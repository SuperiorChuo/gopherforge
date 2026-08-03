package system

import (
	"context"
	"errors"
	"testing"

	"github.com/go-admin-kit/services/system/internal/pkg/tenant"
)

// 白名单 / 租户上下文校验在 DB 访问前完成，nil db 即可测。

func TestTenantSettingRejectsNonConfigurableKey(t *testing.T) {
	svc := NewSettingServiceWithDB(nil)
	ctx := tenant.WithContext(context.Background(), 5)
	_, err := svc.UpsertTenantSettingContext(ctx, "oidc.signing_key", map[string]any{"x": 1})
	if !errors.Is(err, ErrTenantSettingNotConfigurable) {
		t.Fatalf("err = %v, want ErrTenantSettingNotConfigurable", err)
	}
	if err := svc.DeleteTenantSettingContext(ctx, "security.policy"); !errors.Is(err, ErrTenantSettingNotConfigurable) {
		t.Fatalf("delete err = %v, want ErrTenantSettingNotConfigurable", err)
	}
}

func TestTenantSettingRequiresTenantContext(t *testing.T) {
	svc := NewSettingServiceWithDB(nil)
	// 后台/无租户上下文：租户设置不能写
	if _, err := svc.UpsertTenantSettingContext(context.Background(), "ai.provider", map[string]any{"x": 1}); !errors.Is(err, ErrTenantContextRequired) {
		t.Fatalf("upsert err = %v, want ErrTenantContextRequired", err)
	}
	if _, err := svc.ListTenantSettingsContext(context.Background()); !errors.Is(err, ErrTenantContextRequired) {
		t.Fatalf("list err = %v, want ErrTenantContextRequired", err)
	}
}

func TestTenantSettingRejectsInvalidKey(t *testing.T) {
	svc := NewSettingServiceWithDB(nil)
	ctx := tenant.WithContext(context.Background(), 5)
	_, err := svc.UpsertTenantSettingContext(ctx, "bad key", map[string]any{"x": 1})
	if !errors.Is(err, ErrInvalidSystemSettingKey) {
		t.Fatalf("err = %v, want ErrInvalidSystemSettingKey", err)
	}
}
