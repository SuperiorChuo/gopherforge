package runtimeconfig

import (
	"context"
	"errors"
	"strings"
	"sync"
	"time"

	"github.com/go-admin-kit/services/ai/internal/config"
	systemdao "github.com/go-admin-kit/services/ai/internal/dao/system"
	"github.com/go-admin-kit/services/ai/internal/model"
	"github.com/go-admin-kit/services/ai/internal/pkg/database"
	"gorm.io/gorm"
)

// AIProviderSettingKey is the system_settings row that overrides the AI_*
// environment variables. Managed from the console settings page.
const AIProviderSettingKey = "ai.provider"

// AIProviderSettings mirrors config.AIConfig: DB values layer over the
// environment, field by field. An empty APIKey after layering means the
// service is unconfigured and AI endpoints return 503.
type AIProviderSettings struct {
	Provider      string
	BaseURL       string
	APIKey        string
	ChatModel     string
	EmbedModel    string
	EmbedProvider string
	EmbedBaseURL  string
	EmbedAPIKey   string
}

// Configured reports whether the primary provider has credentials.
func (s AIProviderSettings) Configured() bool {
	return strings.TrimSpace(s.APIKey) != ""
}

type AIProviderReader interface {
	AIProviderSettings(ctx context.Context) AIProviderSettings
}

type AIProviderInvalidator interface {
	Refresh(ctx context.Context) error
}

type AIProviderStore interface {
	GetByKeyContext(ctx context.Context, key string) (*model.SystemSetting, error)
}

type CachedAIProviderReader struct {
	store AIProviderStore
	ttl   time.Duration

	mu        sync.RWMutex
	settings  AIProviderSettings
	expiresAt time.Time
	loaded    bool
}

func NewCachedAIProviderReader(store AIProviderStore, ttl time.Duration) *CachedAIProviderReader {
	if ttl <= 0 {
		ttl = 30 * time.Second
	}
	return &CachedAIProviderReader{store: store, ttl: ttl}
}

var (
	defaultAIProviderOnce   sync.Once
	defaultAIProviderReader *CachedAIProviderReader
)

func DefaultAIProviderReader() *CachedAIProviderReader {
	defaultAIProviderOnce.Do(func() {
		defaultAIProviderReader = NewCachedAIProviderReader(defaultAIProviderStore{}, 30*time.Second)
	})
	return defaultAIProviderReader
}

var (
	aiProviderStoreMu sync.RWMutex
	aiProviderStore   AIProviderStore
)

// SetAIProviderStore installs the store behind DefaultAIProviderReader and
// returns a restore function, mirroring SetSecurityPolicyStore.
func SetAIProviderStore(store AIProviderStore) func() {
	aiProviderStoreMu.Lock()
	previous := aiProviderStore
	aiProviderStore = store
	aiProviderStoreMu.Unlock()

	return func() {
		aiProviderStoreMu.Lock()
		aiProviderStore = previous
		aiProviderStoreMu.Unlock()
	}
}

type defaultAIProviderStore struct{}

func (defaultAIProviderStore) GetByKeyContext(ctx context.Context, key string) (*model.SystemSetting, error) {
	aiProviderStoreMu.RLock()
	store := aiProviderStore
	aiProviderStoreMu.RUnlock()
	if store != nil {
		return store.GetByKeyContext(ctx, key)
	}
	if database.DB == nil {
		return nil, ErrStoreUnavailable
	}
	return systemdao.NewSettingDAO(nil).GetByKeyContext(ctx, key)
}

func (r *CachedAIProviderReader) AIProviderSettings(ctx context.Context) AIProviderSettings {
	if r == nil {
		return AIProviderSettingsFromConfig()
	}
	now := time.Now()
	r.mu.RLock()
	if r.loaded && now.Before(r.expiresAt) {
		settings := r.settings
		r.mu.RUnlock()
		return settings
	}
	r.mu.RUnlock()

	if err := r.Refresh(ctx); err != nil {
		r.mu.RLock()
		if r.loaded {
			settings := r.settings
			r.mu.RUnlock()
			return settings
		}
		r.mu.RUnlock()
		return AIProviderSettingsFromConfig()
	}

	r.mu.RLock()
	settings := r.settings
	r.mu.RUnlock()
	return settings
}

func (r *CachedAIProviderReader) Refresh(ctx context.Context) error {
	if r == nil {
		return nil
	}
	if ctx == nil {
		ctx = context.Background()
	}

	settings := AIProviderSettingsFromConfig()
	var err error
	if r.store != nil {
		var setting *model.SystemSetting
		setting, err = r.store.GetByKeyContext(ctx, AIProviderSettingKey)
		switch {
		case err == nil && setting != nil:
			settings = applyAIProviderSetting(settings, setting.ValueJSON)
		case errors.Is(err, gorm.ErrRecordNotFound):
			err = nil
		}
	}

	if err == nil {
		r.mu.Lock()
		r.settings = settings
		r.expiresAt = time.Now().Add(r.ttl)
		r.loaded = true
		r.mu.Unlock()
	}
	return err
}

// AIProviderSettingsFromConfig maps the static AI_* environment config.
func AIProviderSettingsFromConfig() AIProviderSettings {
	ai := config.Cfg.AI
	return AIProviderSettings{
		Provider:      ai.Provider,
		BaseURL:       ai.BaseURL,
		APIKey:        ai.APIKey,
		ChatModel:     ai.ChatModel,
		EmbedModel:    ai.EmbedModel,
		EmbedProvider: ai.EmbedProvider,
		EmbedBaseURL:  ai.EmbedBaseURL,
		EmbedAPIKey:   ai.EmbedAPIKey,
	}
}

// applyAIProviderSetting layers non-empty DB fields over the env defaults.
// provider 名限定 openai/anthropic，与 config.validate 的约束一致；非法值忽略。
func applyAIProviderSetting(settings AIProviderSettings, value map[string]any) AIProviderSettings {
	if value == nil {
		return settings
	}
	if provider, ok := normalizedProviderName(value["provider"]); ok {
		settings.Provider = provider
	}
	settings.BaseURL = stringSetting(value, "base_url", settings.BaseURL)
	settings.APIKey = stringSetting(value, "api_key", settings.APIKey)
	settings.ChatModel = stringSetting(value, "chat_model", settings.ChatModel)
	settings.EmbedModel = stringSetting(value, "embed_model", settings.EmbedModel)
	if provider, ok := normalizedProviderName(value["embed_provider"]); ok {
		settings.EmbedProvider = provider
	}
	settings.EmbedBaseURL = stringSetting(value, "embed_base_url", settings.EmbedBaseURL)
	settings.EmbedAPIKey = stringSetting(value, "embed_api_key", settings.EmbedAPIKey)
	return settings
}

func normalizedProviderName(value any) (string, bool) {
	raw, ok := value.(string)
	if !ok {
		return "", false
	}
	name := strings.ToLower(strings.TrimSpace(raw))
	if name == "openai" || name == "anthropic" {
		return name, true
	}
	return "", false
}

func stringSetting(value map[string]any, key, fallback string) string {
	if raw, ok := value[key].(string); ok {
		if trimmed := strings.TrimSpace(raw); trimmed != "" {
			return trimmed
		}
	}
	return fallback
}
