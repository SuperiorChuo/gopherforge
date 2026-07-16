// Package settings reads console-managed runtime settings from the shared
// system_settings table. IM has no Redis subscriber, so freshness relies on
// a short TTL cache: console edits take effect within ttl (default 30s).
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

// AIProviderSettingKey matches the row written by the console settings page
// (system-service) and consumed by ai-service.
const AIProviderSettingKey = "ai.provider"

// AIProvider is the subset of the ai.provider setting the IM bot consumes.
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

// Get returns the current DB overrides; zero value when the row is absent or
// the store is unreachable (callers fall back to env config).
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
	var raw []byte
	err := r.db.WithContext(ctx).
		Raw("SELECT value_json FROM system_settings WHERE setting_key = ?", AIProviderSettingKey).
		Scan(&raw).Error
	if err != nil {
		return AIProvider{}, err
	}
	if len(raw) == 0 {
		// 行不存在：合法状态，返回零值让调用方回落环境变量
		return AIProvider{}, nil
	}
	var settings AIProvider
	if err := json.Unmarshal(raw, &settings); err != nil {
		return AIProvider{}, err
	}
	settings.Provider = strings.ToLower(strings.TrimSpace(settings.Provider))
	settings.BaseURL = strings.TrimSpace(settings.BaseURL)
	settings.APIKey = strings.TrimSpace(settings.APIKey)
	settings.ChatModel = strings.TrimSpace(settings.ChatModel)
	return settings, nil
}
