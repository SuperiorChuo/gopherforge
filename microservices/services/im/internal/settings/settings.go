// 包 settings 从共享的 system_settings 表读取控制台托管的运行时设置。
// IM 没有 Redis 订阅者，因此新鲜度依赖短 TTL 缓存：控制台修改在 ttl
// （默认 30s）内生效。
package settings

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"time"

	"gorm.io/gorm"
)

// AIProviderSettingKey 对应控制台设置页（system-service）写入、
// 由 ai-service 消费的那一行。
const AIProviderSettingKey = "ai.provider"

// AIProvider 是 IM bot 消费的 ai.provider 设置的子集。
// IM 的 bot 只支持 OpenAI 兼容协议，anthropic 配置对 IM 视为不可用。
type AIProvider struct {
	Provider  string `json:"provider"`
	BaseURL   string `json:"base_url"`
	APIKey    string `json:"api_key"`
	ChatModel string `json:"chat_model"`
}

type AIProviderReader struct {
	db  *gorm.DB
	ttl time.Duration

	mu        sync.RWMutex
	cached    AIProvider
	expiresAt time.Time
	loaded    bool
}

func NewAIProviderReader(db *gorm.DB, ttl time.Duration) *AIProviderReader {
	if ttl <= 0 {
		ttl = 30 * time.Second
	}
	return &AIProviderReader{db: db, ttl: ttl}
}

// Get 返回当前 DB 覆盖；行不存在或存储不可达时返回零值
// （调用方回落到 env 配置）。
func (r *AIProviderReader) Get(ctx context.Context) AIProvider {
	now := time.Now()
	r.mu.RLock()
	if r.loaded && now.Before(r.expiresAt) {
		cached := r.cached
		r.mu.RUnlock()
		return cached
	}
	r.mu.RUnlock()

	settings, err := r.load(ctx)
	if err != nil {
		r.mu.RLock()
		cached := r.cached
		loaded := r.loaded
		r.mu.RUnlock()
		if loaded {
			return cached
		}
		return AIProvider{}
	}

	r.mu.Lock()
	r.cached = settings
	r.expiresAt = now.Add(r.ttl)
	r.loaded = true
	r.mu.Unlock()
	return settings
}

func (r *AIProviderReader) load(ctx context.Context) (AIProvider, error) {
	if r.db == nil {
		return AIProvider{}, errors.New("db not initialized")
	}
	// json 列扫成 string（gorm Raw().Scan 对 []byte 会按整型切片处理而报错）
	var raw string
	err := r.db.WithContext(ctx).
		Raw("SELECT value_json FROM system_settings WHERE setting_key = ?", AIProviderSettingKey).
		Scan(&raw).Error
	if err != nil {
		return AIProvider{}, err
	}
	if raw == "" {
		// 行不存在：合法状态，返回零值让调用方回落环境变量
		return AIProvider{}, nil
	}
	var settings AIProvider
	if err := json.Unmarshal([]byte(raw), &settings); err != nil {
		return AIProvider{}, err
	}
	settings.Provider = strings.ToLower(strings.TrimSpace(settings.Provider))
	settings.BaseURL = strings.TrimSpace(settings.BaseURL)
	settings.APIKey = strings.TrimSpace(settings.APIKey)
	settings.ChatModel = strings.TrimSpace(settings.ChatModel)
	return settings, nil
}
