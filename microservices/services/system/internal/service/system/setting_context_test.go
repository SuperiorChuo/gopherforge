package system

import (
	"context"
	"errors"
	"regexp"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	miniredis "github.com/alicebob/miniredis/v2"
	redisstore "github.com/go-admin-kit/services/system/internal/pkg/redis"
	"github.com/go-admin-kit/services/system/internal/pkg/runtimeconfig"
	goredis "github.com/redis/go-redis/v9"
	"gorm.io/gorm"
)

func TestSettingServiceGetSettingContextMapsNotFound(t *testing.T) {
	db, mock := setupSystemUserServiceContextTestDB(t)
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT * FROM "system_settings" WHERE setting_key = $1 ORDER BY "system_settings"."setting_key" LIMIT $2`)).
		WithArgs("security.password_policy", 1).
		WillReturnError(gorm.ErrRecordNotFound)

	svc := NewSettingServiceWithDB(db)
	_, err := (&svc).GetSettingContext(context.Background(), "security.password_policy")
	if !errors.Is(err, ErrSystemSettingNotFound) {
		t.Fatalf("GetSettingContext() error = %v, want ErrSystemSettingNotFound", err)
	}
}

func TestSettingServiceUpsertSettingContextRejectsInvalidKey(t *testing.T) {
	db, _ := setupSystemUserServiceContextTestDB(t)

	svc := NewSettingServiceWithDB(db)
	_, err := (&svc).UpsertSettingContext(context.Background(), UpsertSettingRequest{
		SettingKey: "security password",
		ValueJSON:  map[string]any{"password_max_age_days": 90},
	})
	if !errors.Is(err, ErrInvalidSystemSettingKey) {
		t.Fatalf("UpsertSettingContext() error = %v, want ErrInvalidSystemSettingKey", err)
	}
}

func TestSettingServiceDeleteSettingContextMapsNotFound(t *testing.T) {
	db, mock := setupSystemUserServiceContextTestDB(t)
	mock.ExpectBegin()
	mock.ExpectExec(regexp.QuoteMeta(`DELETE FROM "system_settings" WHERE setting_key = $1`)).
		WithArgs("security.policy").
		WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectCommit()

	svc := NewSettingServiceWithDB(db)
	err := (&svc).DeleteSettingContext(context.Background(), "security.policy")
	if !errors.Is(err, ErrSystemSettingNotFound) {
		t.Fatalf("DeleteSettingContext() error = %v, want ErrSystemSettingNotFound", err)
	}
}

func TestSettingServiceProtectsOIDCSigningKey(t *testing.T) {
	// The OIDC signing private key must never be readable, writable or deletable
	// through the generic settings API — these checks short-circuit before any DB
	// access, so no sqlmock expectations are set (a stray query would fail).
	db, _ := setupSystemUserServiceContextTestDB(t)
	svc := NewSettingServiceWithDB(db)
	ctx := context.Background()

	if _, err := (&svc).GetSettingContext(ctx, "oidc.signing_key"); !errors.Is(err, ErrProtectedSystemSettingKey) {
		t.Fatalf("GetSettingContext(oidc.signing_key) = %v, want ErrProtectedSystemSettingKey", err)
	}
	if _, err := (&svc).UpsertSettingContext(ctx, UpsertSettingRequest{
		SettingKey: "oidc.signing_key", ValueJSON: map[string]any{"pem": "x"},
	}); !errors.Is(err, ErrProtectedSystemSettingKey) {
		t.Fatalf("UpsertSettingContext(oidc.signing_key) = %v, want ErrProtectedSystemSettingKey", err)
	}
	if err := (&svc).DeleteSettingContext(ctx, "oidc.signing_key"); !errors.Is(err, ErrProtectedSystemSettingKey) {
		t.Fatalf("DeleteSettingContext(oidc.signing_key) = %v, want ErrProtectedSystemSettingKey", err)
	}
	if _, err := (&svc).BatchUpsertSettingsContext(ctx, BatchUpsertSettingsRequest{
		Settings: []UpsertSettingRequest{{SettingKey: "oidc.signing_key", ValueJSON: map[string]any{"pem": "x"}}},
	}); !errors.Is(err, ErrProtectedSystemSettingKey) {
		t.Fatalf("BatchUpsertSettingsContext(oidc.signing_key) = %v, want ErrProtectedSystemSettingKey", err)
	}
}

func TestSettingServiceMasksProtectedKeyInList(t *testing.T) {
	db, mock := setupSystemUserServiceContextTestDB(t)
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT * FROM "system_settings" WHERE setting_key LIKE $1 ORDER BY setting_key ASC`)).
		WithArgs("oidc.%").
		WillReturnRows(sqlmock.NewRows([]string{"setting_key", "value_json", "updated_at"}).
			AddRow("oidc.signing_key", `{"pem":"-----BEGIN RSA PRIVATE KEY-----SECRET"}`, time.Now()))

	svc := NewSettingServiceWithDB(db)
	got, err := (&svc).ListSettingsContext(context.Background(), "oidc")
	if err != nil {
		t.Fatalf("ListSettingsContext() error = %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("expected 1 setting, got %d", len(got))
	}
	if pem, ok := got[0].ValueJSON["pem"]; ok {
		t.Fatalf("list leaked private key material: pem=%v", pem)
	}
	if got[0].ValueJSON["protected"] != true {
		t.Fatalf("protected key not masked: %v", got[0].ValueJSON)
	}
}

func TestSettingServiceUpsertSecurityPolicyRefreshesRuntimeConfig(t *testing.T) {
	db, mock := setupSystemUserServiceContextTestDB(t)
	mock.ExpectBegin()
	mock.ExpectExec("INSERT INTO \"system_settings\"").
		WithArgs(runtimeconfig.SecurityPolicySettingKey, sqlmock.AnyArg(), sqlmock.AnyArg()).
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectCommit()

	invalidator := &stubRuntimeConfigInvalidator{}
	svc := NewSettingServiceWithDB(db)
	svc.runtimeInvalidator = invalidator
	_, err := (&svc).UpsertSettingContext(context.Background(), UpsertSettingRequest{
		SettingKey: runtimeconfig.SecurityPolicySettingKey,
		ValueJSON:  map[string]any{"password_history_count": 3},
	})
	if err != nil {
		t.Fatalf("UpsertSettingContext() error = %v", err)
	}
	if invalidator.calls != 1 {
		t.Fatalf("runtime invalidator calls = %d, want 1", invalidator.calls)
	}
}

func TestSettingServiceDeleteSecurityPolicyRefreshesRuntimeConfig(t *testing.T) {
	db, mock := setupSystemUserServiceContextTestDB(t)
	mock.ExpectBegin()
	mock.ExpectExec(regexp.QuoteMeta(`DELETE FROM "system_settings" WHERE setting_key = $1`)).
		WithArgs(runtimeconfig.SecurityPolicySettingKey).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	invalidator := &stubRuntimeConfigInvalidator{}
	svc := NewSettingServiceWithDB(db)
	svc.runtimeInvalidator = invalidator
	err := (&svc).DeleteSettingContext(context.Background(), runtimeconfig.SecurityPolicySettingKey)
	if err != nil {
		t.Fatalf("DeleteSettingContext() error = %v", err)
	}
	if invalidator.calls != 1 {
		t.Fatalf("runtime invalidator calls = %d, want 1", invalidator.calls)
	}
}

func TestSettingServiceUpsertEmailNotificationRefreshesRuntimeConfig(t *testing.T) {
	db, mock := setupSystemUserServiceContextTestDB(t)
	mock.ExpectBegin()
	mock.ExpectExec("INSERT INTO \"system_settings\"").
		WithArgs(runtimeconfig.EmailNotificationSettingKey, sqlmock.AnyArg(), sqlmock.AnyArg()).
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectCommit()

	emailInvalidator := &stubRuntimeConfigInvalidator{}
	svc := NewSettingServiceWithDB(db)
	svc.emailInvalidator = emailInvalidator
	_, err := (&svc).UpsertSettingContext(context.Background(), UpsertSettingRequest{
		SettingKey: runtimeconfig.EmailNotificationSettingKey,
		ValueJSON:  map[string]any{"enabled": true},
	})
	if err != nil {
		t.Fatalf("UpsertSettingContext() error = %v", err)
	}
	if emailInvalidator.calls != 1 {
		t.Fatalf("email invalidator calls = %d, want 1", emailInvalidator.calls)
	}
}

func TestSettingServiceUpsertRuntimeConfigPublishesInvalidation(t *testing.T) {
	tests := []struct {
		name      string
		key       string
		valueJSON map[string]any
	}{
		{
			name:      "security policy",
			key:       runtimeconfig.SecurityPolicySettingKey,
			valueJSON: map[string]any{"password_history_count": 3},
		},
		{
			name:      "email notification",
			key:       runtimeconfig.EmailNotificationSettingKey,
			valueJSON: map[string]any{"enabled": true},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			setupSettingRuntimeConfigRedisClient(t)

			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			defer cancel()

			messages := make(chan string, 1)
			subscriber, err := redisstore.StartSubscriber(ctx, runtimeconfig.RuntimeConfigInvalidationChannel, func(_ context.Context, payload string) {
				messages <- payload
			})
			if err != nil {
				t.Fatalf("start subscriber: %v", err)
			}
			defer func() {
				if err := subscriber.Close(); err != nil {
					t.Fatalf("close subscriber: %v", err)
				}
			}()

			db, mock := setupSystemUserServiceContextTestDB(t)
			mock.ExpectBegin()
			mock.ExpectExec("INSERT INTO \"system_settings\"").
				WithArgs(tt.key, sqlmock.AnyArg(), sqlmock.AnyArg()).
				WillReturnResult(sqlmock.NewResult(1, 1))
			mock.ExpectCommit()

			invalidator := &stubRuntimeConfigInvalidator{}
			service := NewSettingServiceWithDB(db)
			if tt.key == runtimeconfig.SecurityPolicySettingKey {
				service.runtimeInvalidator = invalidator
			} else {
				service.emailInvalidator = invalidator
			}

			_, err = service.UpsertSettingContext(ctx, UpsertSettingRequest{
				SettingKey: tt.key,
				ValueJSON:  tt.valueJSON,
			})
			if err != nil {
				t.Fatalf("UpsertSettingContext() error = %v", err)
			}
			if invalidator.calls != 1 {
				t.Fatalf("runtime invalidator calls = %d, want 1", invalidator.calls)
			}

			select {
			case got := <-messages:
				if got != tt.key {
					t.Fatalf("invalidation message = %q, want %q", got, tt.key)
				}
			case <-ctx.Done():
				t.Fatalf("did not receive invalidation for %q", tt.key)
			}
		})
	}
}

func TestSettingServiceUpsertRuntimeConfigIgnoresPublishFailure(t *testing.T) {
	oldClient := redisstore.Client
	redisstore.Client = nil
	t.Cleanup(func() {
		redisstore.Client = oldClient
	})

	db, mock := setupSystemUserServiceContextTestDB(t)
	mock.ExpectBegin()
	mock.ExpectExec("INSERT INTO \"system_settings\"").
		WithArgs(runtimeconfig.SecurityPolicySettingKey, sqlmock.AnyArg(), sqlmock.AnyArg()).
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectCommit()

	invalidator := &stubRuntimeConfigInvalidator{}
	svc := NewSettingServiceWithDB(db)
	svc.runtimeInvalidator = invalidator
	_, err := (&svc).UpsertSettingContext(context.Background(), UpsertSettingRequest{
		SettingKey: runtimeconfig.SecurityPolicySettingKey,
		ValueJSON:  map[string]any{"password_history_count": 3},
	})
	if err != nil {
		t.Fatalf("UpsertSettingContext() error = %v", err)
	}
	if invalidator.calls != 1 {
		t.Fatalf("runtime invalidator calls = %d, want 1", invalidator.calls)
	}
}

func TestSettingServiceRefreshRuntimeConfigUsesDetachedContextWhenRequestCanceled(t *testing.T) {
	setupSettingRuntimeConfigRedisClient(t)

	requestCtx, cancelRequest := context.WithCancel(context.Background())
	cancelRequest()

	invalidator := &contextObservingRuntimeConfigInvalidator{}
	(&SettingService{runtimeInvalidator: invalidator}).refreshRuntimeConfigIfNeeded(requestCtx, runtimeconfig.SecurityPolicySettingKey)

	if invalidator.calls != 1 {
		t.Fatalf("runtime invalidator calls = %d, want 1", invalidator.calls)
	}
	if invalidator.ctxErr != nil {
		t.Fatalf("runtime invalidator context error = %v, want nil", invalidator.ctxErr)
	}
}

func TestSettingServiceRefreshRuntimeConfigPublishesWithDetachedContextWhenRequestCanceled(t *testing.T) {
	setupSettingRuntimeConfigRedisClient(t)

	subscribeCtx, cancelSubscribe := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancelSubscribe()

	messages := make(chan string, 1)
	subscriber, err := redisstore.StartSubscriber(subscribeCtx, runtimeconfig.RuntimeConfigInvalidationChannel, func(_ context.Context, payload string) {
		messages <- payload
	})
	if err != nil {
		t.Fatalf("start subscriber: %v", err)
	}
	defer func() {
		if err := subscriber.Close(); err != nil {
			t.Fatalf("close subscriber: %v", err)
		}
	}()

	requestCtx, cancelRequest := context.WithCancel(context.Background())
	cancelRequest()

	(&SettingService{runtimeInvalidator: &stubRuntimeConfigInvalidator{}}).refreshRuntimeConfigIfNeeded(requestCtx, runtimeconfig.SecurityPolicySettingKey)

	select {
	case got := <-messages:
		if got != runtimeconfig.SecurityPolicySettingKey {
			t.Fatalf("invalidation message = %q, want %q", got, runtimeconfig.SecurityPolicySettingKey)
		}
	case <-subscribeCtx.Done():
		t.Fatal("did not receive invalidation from canceled request context")
	}
}

func setupSettingRuntimeConfigRedisClient(t *testing.T) {
	t.Helper()

	store, err := miniredis.Run()
	if err != nil {
		t.Fatalf("start miniredis: %v", err)
	}

	previousClient := redisstore.Client
	redisstore.Client = goredis.NewClient(&goredis.Options{Addr: store.Addr()})

	t.Cleanup(func() {
		_ = redisstore.Client.Close()
		redisstore.Client = previousClient
		store.Close()
	})
}

type stubRuntimeConfigInvalidator struct {
	calls int
}

func (s *stubRuntimeConfigInvalidator) Refresh(ctx context.Context) error {
	s.calls++
	return nil
}

type contextObservingRuntimeConfigInvalidator struct {
	calls  int
	ctxErr error
}

func (s *contextObservingRuntimeConfigInvalidator) Refresh(ctx context.Context) error {
	s.calls++
	s.ctxErr = ctx.Err()
	return nil
}
