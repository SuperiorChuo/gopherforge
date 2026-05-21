package auth

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/go-admin-kit/server/internal/config"
	"github.com/go-admin-kit/server/internal/model"
	jwtpkg "github.com/go-admin-kit/server/internal/pkg/jwt"
)

func TestHashSummaryMatchesConsoleSessionRules(t *testing.T) {
	if got := hashSummary("   "); got != "" {
		t.Fatalf("blank hash = %q, want empty", got)
	}

	got := hashSummary("127.0.0.1")
	if len(got) != 64 {
		t.Fatalf("hash length = %d, want 64", len(got))
	}
	if got != hashSummary(" 127.0.0.1 ") {
		t.Fatal("hash should trim whitespace before hashing")
	}
}

func TestTruncateRunesPreservesRuneBoundaries(t *testing.T) {
	got := truncateRunes("abc\u4e16\u754c", 4)
	if got != "abc\u4e16" {
		t.Fatalf("truncateRunes = %q, want abc\\u4e16", got)
	}
}

func TestConsoleSessionServiceValidateActiveSessionContextHonorsCanceledContext(t *testing.T) {
	setupAuthServiceContextTestDB(t)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := (ConsoleSessionService{}).ValidateActiveSessionContext(ctx, "session-1", "alice")
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("ValidateActiveSessionContext() error = %v, want context.Canceled", err)
	}
}

func TestBuildConsoleSessionUsesTokenExpiryAndUserMetadata(t *testing.T) {
	setConsoleSessionJWTTestConfig(t)
	accessToken, refreshToken, err := jwtpkg.GenerateTokenWithAccessTTL(42, "alice", 90*time.Minute)
	if err != nil {
		t.Fatalf("GenerateTokenWithAccessTTL() error = %v", err)
	}

	session := BuildConsoleSession(&model.User{
		ID:                 42,
		Username:           "alice",
		Nickname:           "Alice Admin",
		Avatar:             "https://example.test/avatar.png",
		MustChangePassword: true,
		Roles: []model.Role{
			{Code: "operator"},
			{Code: "admin"},
		},
	}, []string{"settings.read"}, accessToken, refreshToken)

	if !session.Authenticated || !session.AuthEnabled {
		t.Fatalf("session auth flags = authenticated:%v enabled:%v, want both true", session.Authenticated, session.AuthEnabled)
	}
	if session.AccessToken != accessToken || session.RefreshToken != refreshToken {
		t.Fatal("session did not preserve issued tokens")
	}
	if session.TTLSec <= 0 || session.TTLSec > int((90*time.Minute).Seconds()) {
		t.Fatalf("session ttl = %d, want positive and within token TTL", session.TTLSec)
	}
	if session.User.ID != 42 || session.User.DisplayName != "Alice Admin" || session.User.ActorID != "alice" {
		t.Fatalf("session user = %#v, want profile fields from user", session.User)
	}
	if session.User.Role != "admin" || len(session.User.Roles) != 2 || session.User.Roles[0] != "admin" || session.User.Roles[1] != "operator" {
		t.Fatalf("session roles = role:%q roles:%v, want sorted role codes", session.User.Role, session.User.Roles)
	}
	if len(session.User.Permissions) != 1 || session.User.Permissions[0] != "settings.read" {
		t.Fatalf("session permissions = %v, want supplied permissions", session.User.Permissions)
	}
	if !session.User.MustChangePassword {
		t.Fatal("session user should preserve must_change_password")
	}
}

func TestBuildConsoleSessionFallsBackToUsernameDisplayName(t *testing.T) {
	session := BuildConsoleSession(&model.User{Username: "bob"}, nil, "", "")

	if session.User.DisplayName != "bob" {
		t.Fatalf("display name = %q, want username fallback", session.User.DisplayName)
	}
	if session.User.Role != "operator" {
		t.Fatalf("role = %q, want operator fallback", session.User.Role)
	}
}

func setConsoleSessionJWTTestConfig(t *testing.T) {
	t.Helper()

	oldConfig := config.Cfg.JWT
	config.Cfg.JWT = config.JWTConfig{
		Secret:               "unit-test-secret-at-least-32-characters",
		AccessTokenExpire:    3600,
		RefreshTokenExpire:   7200,
		RefreshTokenRotation: true,
		Issuer:               "unit-test",
	}

	t.Cleanup(func() {
		config.Cfg.JWT = oldConfig
	})
}
