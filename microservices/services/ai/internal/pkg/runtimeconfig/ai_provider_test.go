package runtimeconfig

import (
	"context"
	"testing"
	"time"

	"github.com/go-admin-kit/services/ai/internal/config"
	model "github.com/go-admin-kit/services/shared/pkg/model"
	"github.com/go-admin-kit/services/shared/pkg/tenant"
	"gorm.io/gorm"
)

func withStubAIConfig(t *testing.T) {
	t.Helper()
	oldAI := config.Cfg.AI
	config.Cfg.AI = config.AIConfig{
		Provider:   "openai",
		BaseURL:    "https://env.example.com/v1",
		APIKey:     "env-key",
		ChatModel:  "env-chat",
		EmbedModel: "env-embed",
	}
	t.Cleanup(func() {
		config.Cfg.AI = oldAI
	})
}

func TestAIProviderReaderFallsBackToStaticConfig(t *testing.T) {
	withStubAIConfig(t)

	reader := NewCachedAIProviderReader(&stubAIProviderStore{err: gorm.ErrRecordNotFound}, time.Minute)
	settings := reader.AIProviderSettings(context.Background())

	if settings.Provider != "openai" || settings.APIKey != "env-key" || settings.ChatModel != "env-chat" {
		t.Fatalf("settings = %#v, want static config values", settings)
	}
	if !settings.Configured() {
		t.Fatalf("Configured() = false, want true with env API key")
	}
}

func TestAIProviderReaderLayersSettingOverConfig(t *testing.T) {
	withStubAIConfig(t)

	store := &stubAIProviderStore{setting: &model.SystemSetting{
		SettingKey: AIProviderSettingKey,
		ValueJSON: map[string]any{
			"provider":   "anthropic",
			"api_key":    "db-key",
			"chat_model": "db-chat",
			// base_url 留空 → 回落环境变量
			"base_url": "",
		},
	}}
	reader := NewCachedAIProviderReader(store, time.Minute)
	settings := reader.AIProviderSettings(context.Background())

	if settings.Provider != "anthropic" || settings.APIKey != "db-key" || settings.ChatModel != "db-chat" {
		t.Fatalf("settings = %#v, want DB overrides", settings)
	}
	if settings.BaseURL != "https://env.example.com/v1" {
		t.Fatalf("BaseURL = %q, want env fallback for empty DB field", settings.BaseURL)
	}
	if settings.EmbedModel != "env-embed" {
		t.Fatalf("EmbedModel = %q, want env fallback for absent DB field", settings.EmbedModel)
	}
}

func TestAIProviderReaderIgnoresInvalidProviderName(t *testing.T) {
	withStubAIConfig(t)

	store := &stubAIProviderStore{setting: &model.SystemSetting{
		SettingKey: AIProviderSettingKey,
		ValueJSON:  map[string]any{"provider": "not-a-provider"},
	}}
	reader := NewCachedAIProviderReader(store, time.Minute)

	if got := reader.AIProviderSettings(context.Background()).Provider; got != "openai" {
		t.Fatalf("Provider = %q, want invalid name ignored", got)
	}
}

func TestAIProviderReaderCachesAndRefreshes(t *testing.T) {
	withStubAIConfig(t)

	store := &stubAIProviderStore{setting: &model.SystemSetting{
		SettingKey: AIProviderSettingKey,
		ValueJSON:  map[string]any{"chat_model": "gen-1"},
	}}
	reader := NewCachedAIProviderReader(store, time.Hour)

	if got := reader.AIProviderSettings(context.Background()).ChatModel; got != "gen-1" {
		t.Fatalf("initial chat model = %q, want gen-1", got)
	}
	store.setting.ValueJSON = map[string]any{"chat_model": "gen-2"}
	if got := reader.AIProviderSettings(context.Background()).ChatModel; got != "gen-1" {
		t.Fatalf("cached chat model = %q, want gen-1 before refresh", got)
	}
	if store.calls != 1 {
		t.Fatalf("store calls = %d, want cache hit after first read", store.calls)
	}

	if err := reader.Refresh(context.Background()); err != nil {
		t.Fatalf("Refresh() error = %v", err)
	}
	if got := reader.AIProviderSettings(context.Background()).ChatModel; got != "gen-2" {
		t.Fatalf("refreshed chat model = %q, want gen-2", got)
	}
}

type stubAIProviderStore struct {
	setting *model.SystemSetting
	err     error
	calls   int
}

func (s *stubAIProviderStore) GetByKeyContext(_ context.Context, _ string) (*model.SystemSetting, error) {
	s.calls++
	if s.err != nil {
		return nil, s.err
	}
	return s.setting, nil
}

type stubTenantSettingStore struct {
	setting *model.TenantSetting
	err     error
}

func (s stubTenantSettingStore) GetByKeyContext(_ context.Context, tenantID uint, _ string) (*model.TenantSetting, error) {
	if s.err != nil {
		return nil, s.err
	}
	if s.setting == nil || s.setting.TenantID != tenantID {
		return nil, gorm.ErrRecordNotFound
	}
	return s.setting, nil
}

func TestAIProviderReaderAppliesTenantOverride(t *testing.T) {
	withStubAIConfig(t)

	reader := NewCachedAIProviderReader(&stubAIProviderStore{err: gorm.ErrRecordNotFound}, time.Minute)
	reader.tenantStore = stubTenantSettingStore{setting: &model.TenantSetting{
		TenantID: 5, SettingKey: AIProviderSettingKey,
		ValueJSON: map[string]any{"api_key": "tenant-key", "chat_model": "tenant-chat"},
	}}

	// 显式租户上下文 → 应用租户覆盖（层叠在平台默认之上）
	ctx := tenant.WithContext(context.Background(), 5)
	settings := reader.AIProviderSettings(ctx)
	if settings.APIKey != "tenant-key" || settings.ChatModel != "tenant-chat" {
		t.Fatalf("tenant ctx settings = %#v, want tenant override", settings)
	}

	// 后台/无租户上下文 → 平台默认（env），不查租户覆盖
	settings = reader.AIProviderSettings(context.Background())
	if settings.APIKey != "env-key" {
		t.Fatalf("background ctx settings = %#v, want env default", settings)
	}
}
