package api

import (
	"testing"
	"time"

	"github.com/go-admin-kit/services/monitor/internal/config"
	"github.com/go-admin-kit/services/shared/pkg/jwt"
	sharedapi "github.com/go-admin-kit/services/shared/pkg/sharedapi"
)

func TestConfigureRuntimeJWTUsesLoadedServiceSecret(t *testing.T) {
	original := config.Cfg
	config.Cfg.JWT = config.JWTConfig{
		Secret: "monitor-runtime-test-secret-at-least-32-characters",
		Issuer: "monitor-runtime-test", AccessTokenExpire: 3600, RefreshTokenExpire: 7200,
	}
	restoreJWT := configureRuntimeJWT(sharedapi.Dependencies{})
	t.Cleanup(func() {
		restoreJWT()
		config.Cfg = original
	})

	access, _, err := jwt.GenerateTokenWithTenantPlatformAndAccessTTL(1, "monitor-test", 1, true, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := jwt.TokenID(access); err != nil {
		t.Fatalf("configured token rejected: %v", err)
	}
	restoreJWT()
	if _, err := jwt.TokenID(access); err == nil {
		t.Fatal("token signed with runtime secret remained valid after restoring previous config")
	}
}
