package system

import (
	"context"
	"errors"
	"regexp"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	miniredis "github.com/alicebob/miniredis/v2"
	redisstore "github.com/go-admin-kit/server/internal/pkg/redis"
	"github.com/go-admin-kit/server/internal/pkg/runtimeconfig"
	goredis "github.com/redis/go-redis/v9"
	"gorm.io/gorm"
)

func TestSettingServiceGetSettingContextMapsNotFound(t *testing.T) {
	mock := setupSystemUserServiceContextTestDB(t)
	mock.ExpectQuery(regexp.QuoteMeta("SELECT * FROM `system_settings` WHERE setting_key = ? ORDER BY `system_settings`.`setting_key` LIMIT ?")).
		WithArgs("security.password_policy", 1).
		WillReturnError(gorm.ErrRecordNotFound)

	_, err := (&SettingService{}).GetSettingContext(context.Background(), "security.password_policy")
	if !errors.Is(err, ErrSystemSettingNotFound) {
		t.Fatalf("GetSettingContext() error = %v, want ErrSystemSettingNotFound", err)
	}
}

func TestSettingServiceUpsertSettingContextRejectsInvalidKey(t *testing.T) {
	setupSystemUserServiceContextTestDB(t)

	_, err := (&SettingService{}).UpsertSettingContext(context.Background(), UpsertSettingRequest{
		SettingKey: "security password",
		ValueJSON:  map[string]any{"password_max_age_days": 90},
	})
	if !errors.Is(err, ErrInvalidSystemSettingKey) {
		t.Fatalf("UpsertSettingContext() error = %v, want ErrInvalidSystemSettingKey", err)
	}
}

func TestSettingServiceDeleteSettingContextMapsNotFound(t *testing.T) {
	mock := setupSystemUserServiceContextTestDB(t)
	mock.ExpectBegin()
	mock.ExpectExec(regexp.QuoteMeta("DELETE FROM `system_settings` WHERE setting_key = ?")).
		WithArgs("security.policy").
		WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectCommit()

	err := (&SettingService{}).DeleteSettingContext(context.Background(), "security.policy")
	if !errors.Is(err, ErrSystemSettingNotFound) {
		t.Fatalf("DeleteSettingContext() error = %v, want ErrSystemSettingNotFound", err)
	}
}

func TestSettingServiceUpsertSecurityPolicyRefreshesRuntimeConfig(t *testing.T) {
	mock := setupSystemUserServiceContextTestDB(t)
	mock.ExpectBegin()
	mock.ExpectExec("INSERT INTO `system_settings`").
		WithArgs(runtimeconfig.SecurityPolicySettingKey, sqlmock.AnyArg(), sqlmock.AnyArg()).
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectCommit()

	invalidator := &stubRuntimeConfigInvalidator{}
	_, err := (&SettingService{runtimeInvalidator: invalidator}).UpsertSettingContext(context.Background(), UpsertSettingRequest{
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
	mock := setupSystemUserServiceContextTestDB(t)
	mock.ExpectBegin()
	mock.ExpectExec(regexp.QuoteMeta("DELETE FROM `system_settings` WHERE setting_key = ?")).
		WithArgs(runtimeconfig.SecurityPolicySettingKey).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	invalidator := &stubRuntimeConfigInvalidator{}
	err := (&SettingService{runtimeInvalidator: invalidator}).DeleteSettingContext(context.Background(), runtimeconfig.SecurityPolicySettingKey)
	if err != nil {
		t.Fatalf("DeleteSettingContext() error = %v", err)
	}
	if invalidator.calls != 1 {
		t.Fatalf("runtime invalidator calls = %d, want 1", invalidator.calls)
	}
}

func TestSettingServiceUpsertEmailNotificationRefreshesRuntimeConfig(t *testing.T) {
	mock := setupSystemUserServiceContextTestDB(t)
	mock.ExpectBegin()
	mock.ExpectExec("INSERT INTO `system_settings`").
		WithArgs(runtimeconfig.EmailNotificationSettingKey, sqlmock.AnyArg(), sqlmock.AnyArg()).
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectCommit()

	emailInvalidator := &stubRuntimeConfigInvalidator{}
	_, err := (&SettingService{emailInvalidator: emailInvalidator}).UpsertSettingContext(context.Background(), UpsertSettingRequest{
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

			mock := setupSystemUserServiceContextTestDB(t)
			mock.ExpectBegin()
			mock.ExpectExec("INSERT INTO `system_settings`").
				WithArgs(tt.key, sqlmock.AnyArg(), sqlmock.AnyArg()).
				WillReturnResult(sqlmock.NewResult(1, 1))
			mock.ExpectCommit()

			invalidator := &stubRuntimeConfigInvalidator{}
			service := &SettingService{}
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

	mock := setupSystemUserServiceContextTestDB(t)
	mock.ExpectBegin()
	mock.ExpectExec("INSERT INTO `system_settings`").
		WithArgs(runtimeconfig.SecurityPolicySettingKey, sqlmock.AnyArg(), sqlmock.AnyArg()).
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectCommit()

	invalidator := &stubRuntimeConfigInvalidator{}
	_, err := (&SettingService{runtimeInvalidator: invalidator}).UpsertSettingContext(context.Background(), UpsertSettingRequest{
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
