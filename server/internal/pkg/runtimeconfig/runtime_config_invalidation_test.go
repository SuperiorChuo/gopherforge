package runtimeconfig

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	miniredis "github.com/alicebob/miniredis/v2"
	"github.com/go-admin-kit/server/internal/model"
	redisstore "github.com/go-admin-kit/server/internal/pkg/redis"
	goredis "github.com/redis/go-redis/v9"
	"gorm.io/gorm"
)

func TestPublishInvalidationPublishesSupportedKeysOnly(t *testing.T) {
	setupRuntimeConfigRedisClient(t)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	messages := make(chan string, 2)
	subscriber, err := redisstore.StartSubscriber(ctx, RuntimeConfigInvalidationChannel, func(_ context.Context, payload string) {
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

	if err := PublishInvalidation(ctx, SecurityPolicySettingKey); err != nil {
		t.Fatalf("PublishInvalidation(security.policy) error = %v", err)
	}
	assertRuntimeConfigMessage(t, ctx, messages, SecurityPolicySettingKey)

	if err := PublishInvalidation(ctx, EmailNotificationSettingKey); err != nil {
		t.Fatalf("PublishInvalidation(notification.email) error = %v", err)
	}
	assertRuntimeConfigMessage(t, ctx, messages, EmailNotificationSettingKey)

	if err := PublishInvalidation(ctx, "oauth.github"); err != nil {
		t.Fatalf("PublishInvalidation(unknown) error = %v", err)
	}
	select {
	case got := <-messages:
		t.Fatalf("unexpected invalidation message for unsupported key: %q", got)
	case <-time.After(100 * time.Millisecond):
	}
}

func TestRefreshByKeyDispatchesSupportedKeysOnly(t *testing.T) {
	store := &countingRuntimeConfigStore{}
	installDefaultRuntimeConfigReaders(t, store)

	if err := RefreshByKey(context.Background(), SecurityPolicySettingKey); err != nil {
		t.Fatalf("RefreshByKey(security.policy) error = %v", err)
	}
	if err := RefreshByKey(context.Background(), EmailNotificationSettingKey); err != nil {
		t.Fatalf("RefreshByKey(notification.email) error = %v", err)
	}
	if err := RefreshByKey(context.Background(), "oauth.github"); err != nil {
		t.Fatalf("RefreshByKey(unknown) error = %v", err)
	}

	if got := store.securityCalls.Load(); got != 1 {
		t.Fatalf("security policy refresh calls = %d, want 1", got)
	}
	if got := store.emailCalls.Load(); got != 1 {
		t.Fatalf("email notification refresh calls = %d, want 1", got)
	}
}

func TestStartInvalidationListenerRefreshesPublishedSupportedKeys(t *testing.T) {
	setupRuntimeConfigRedisClient(t)
	store := &countingRuntimeConfigStore{}
	installDefaultRuntimeConfigReaders(t, store)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	subscriber, err := StartInvalidationListener(ctx)
	if err != nil {
		t.Fatalf("StartInvalidationListener() error = %v", err)
	}
	defer func() {
		if err := subscriber.Close(); err != nil {
			t.Fatalf("close subscriber: %v", err)
		}
	}()

	if err := redisstore.PublishString(ctx, RuntimeConfigInvalidationChannel, "oauth.github"); err != nil {
		t.Fatalf("publish unsupported key: %v", err)
	}
	time.Sleep(100 * time.Millisecond)
	if got := store.securityCalls.Load() + store.emailCalls.Load(); got != 0 {
		t.Fatalf("unsupported key refresh calls = %d, want 0", got)
	}

	if err := redisstore.PublishString(ctx, RuntimeConfigInvalidationChannel, SecurityPolicySettingKey); err != nil {
		t.Fatalf("publish security policy key: %v", err)
	}
	waitForRuntimeConfigCall(t, ctx, "security policy refresh", func() bool {
		return store.securityCalls.Load() == 1
	})

	if err := redisstore.PublishString(ctx, RuntimeConfigInvalidationChannel, EmailNotificationSettingKey); err != nil {
		t.Fatalf("publish email notification key: %v", err)
	}
	waitForRuntimeConfigCall(t, ctx, "email notification refresh", func() bool {
		return store.emailCalls.Load() == 1
	})
}

func setupRuntimeConfigRedisClient(t *testing.T) {
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

func installDefaultRuntimeConfigReaders(t *testing.T, store *countingRuntimeConfigStore) {
	t.Helper()

	previousSecurityReader := defaultSecurityPolicyReader
	previousEmailReader := defaultEmailNotificationReader

	defaultSecurityPolicyOnce = sync.Once{}
	defaultSecurityPolicyReader = NewCachedSecurityPolicyReader(store, time.Minute)
	defaultSecurityPolicyOnce.Do(func() {})
	defaultEmailNotificationOnce = sync.Once{}
	defaultEmailNotificationReader = NewCachedEmailNotificationReader(store, time.Minute)
	defaultEmailNotificationOnce.Do(func() {})

	t.Cleanup(func() {
		defaultSecurityPolicyReader = previousSecurityReader
		defaultEmailNotificationReader = previousEmailReader

		defaultSecurityPolicyOnce = sync.Once{}
		if previousSecurityReader != nil {
			defaultSecurityPolicyOnce.Do(func() {})
		}
		defaultEmailNotificationOnce = sync.Once{}
		if previousEmailReader != nil {
			defaultEmailNotificationOnce.Do(func() {})
		}
	})
}

func assertRuntimeConfigMessage(t *testing.T, ctx context.Context, messages <-chan string, want string) {
	t.Helper()

	select {
	case got := <-messages:
		if got != want {
			t.Fatalf("message = %q, want %q", got, want)
		}
	case <-ctx.Done():
		t.Fatalf("subscriber did not receive %q invalidation", want)
	}
}

func waitForRuntimeConfigCall(t *testing.T, ctx context.Context, name string, called func() bool) {
	t.Helper()

	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()
	for {
		if called() {
			return
		}
		select {
		case <-ctx.Done():
			t.Fatalf("%s was not observed before timeout", name)
		case <-ticker.C:
		}
	}
}

type countingRuntimeConfigStore struct {
	securityCalls atomic.Int32
	emailCalls    atomic.Int32
}

func (s *countingRuntimeConfigStore) GetByKeyContext(ctx context.Context, key string) (*model.SystemSetting, error) {
	switch key {
	case SecurityPolicySettingKey:
		s.securityCalls.Add(1)
	case EmailNotificationSettingKey:
		s.emailCalls.Add(1)
	}
	return nil, gorm.ErrRecordNotFound
}
