package runtimeconfig

import (
	"context"
	"sync"
	"time"

	systemdao "github.com/go-admin-kit/services/system/internal/dao/system"
	"github.com/go-admin-kit/services/system/internal/model"
	"github.com/go-admin-kit/services/system/internal/pkg/database"
)

// ErrorCodeStore 提供全量启用错误码的读取能力（由 DAO 实现）。
type ErrorCodeStore interface {
	GetAllEnabledContext(ctx context.Context) ([]model.ErrorCode, error)
}

// ErrorCodeInvalidator 供写路径在改动后立即刷新本进程缓存。
type ErrorCodeInvalidator interface {
	Refresh(ctx context.Context) error
}

// CachedErrorCodeReader 错误码文案读取器：30s TTL 内存缓存。
// 控制台修改错误码后，本进程最迟 TTL 到期后的下一次读取热生效，无需重启；
// system 服务自身的写路径还会主动 Refresh，做到本进程即时生效。
type CachedErrorCodeReader struct {
	store ErrorCodeStore
	ttl   time.Duration

	mu        sync.RWMutex
	messages  map[string]string
	expiresAt time.Time
	loaded    bool
}

// NewCachedErrorCodeReader 构建读取器；ttl<=0 时使用默认 30s。
func NewCachedErrorCodeReader(store ErrorCodeStore, ttl time.Duration) *CachedErrorCodeReader {
	if ttl <= 0 {
		ttl = 30 * time.Second
	}
	return &CachedErrorCodeReader{store: store, ttl: ttl}
}

var (
	defaultErrorCodeReaderOnce sync.Once
	defaultErrorCodeReader     *CachedErrorCodeReader
)

// DefaultErrorCodeReader 进程内默认读取器单例（30s TTL）。
func DefaultErrorCodeReader() *CachedErrorCodeReader {
	defaultErrorCodeReaderOnce.Do(func() {
		defaultErrorCodeReader = NewCachedErrorCodeReader(defaultErrorCodeStore{}, 30*time.Second)
	})
	return defaultErrorCodeReader
}

var (
	errorCodeStoreMu sync.RWMutex
	errorCodeStore   ErrorCodeStore
)

// SetErrorCodeStore 安装 DefaultErrorCodeReader 背后的 store 并返回恢复函数。
// 默认读取器按次解析 store，未安装时回退到全局数据库句柄，因此不接线也可用。
func SetErrorCodeStore(store ErrorCodeStore) func() {
	errorCodeStoreMu.Lock()
	previous := errorCodeStore
	errorCodeStore = store
	errorCodeStoreMu.Unlock()

	return func() {
		errorCodeStoreMu.Lock()
		errorCodeStore = previous
		errorCodeStoreMu.Unlock()
	}
}

type defaultErrorCodeStore struct{}

func (defaultErrorCodeStore) GetAllEnabledContext(ctx context.Context) ([]model.ErrorCode, error) {
	errorCodeStoreMu.RLock()
	store := errorCodeStore
	errorCodeStoreMu.RUnlock()
	if store != nil {
		return store.GetAllEnabledContext(ctx)
	}
	if database.DB == nil {
		return nil, ErrStoreUnavailable
	}
	return systemdao.NewErrCodeDAO(database.DB).GetAllEnabledContext(ctx)
}

// GetMessage 按错误码取自定义文案；命中启用中的记录返回 (文案, true)，
// 否则返回 ("", false)，调用方应继续使用默认文案。
func (r *CachedErrorCodeReader) GetMessage(ctx context.Context, code string) (string, bool) {
	if r == nil || code == "" {
		return "", false
	}
	now := time.Now()
	r.mu.RLock()
	if r.loaded && now.Before(r.expiresAt) {
		message, ok := r.messages[code]
		r.mu.RUnlock()
		return message, ok && message != ""
	}
	r.mu.RUnlock()

	// 缓存过期或未加载：刷新失败时降级用旧缓存，避免错误路径反被拖垮
	if err := r.Refresh(ctx); err != nil {
		r.mu.RLock()
		defer r.mu.RUnlock()
		if r.loaded {
			message, ok := r.messages[code]
			return message, ok && message != ""
		}
		return "", false
	}

	r.mu.RLock()
	defer r.mu.RUnlock()
	message, ok := r.messages[code]
	return message, ok && message != ""
}

// Refresh 立即从 store 重建缓存并续期 TTL。
func (r *CachedErrorCodeReader) Refresh(ctx context.Context) error {
	if r == nil {
		return nil
	}
	if ctx == nil {
		ctx = context.Background()
	}

	codes, err := r.store.GetAllEnabledContext(ctx)
	if err != nil {
		return err
	}

	messages := make(map[string]string, len(codes))
	for _, item := range codes {
		if item.Code == "" || item.Message == "" {
			continue
		}
		messages[item.Code] = item.Message
	}

	r.mu.Lock()
	r.messages = messages
	r.expiresAt = time.Now().Add(r.ttl)
	r.loaded = true
	r.mu.Unlock()
	return nil
}
