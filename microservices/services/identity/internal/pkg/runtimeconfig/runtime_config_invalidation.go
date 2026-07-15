package runtimeconfig

import (
	"context"

	redisstore "github.com/go-admin-kit/services/identity/internal/pkg/redis"
)

// RuntimeConfigInvalidationChannel matches the monolith's channel so cache
// invalidations published by either service refresh both processes.
const RuntimeConfigInvalidationChannel = "go_admin_kit:runtime_config:invalidate"

func PublishInvalidation(ctx context.Context, key string) error {
	if !isRuntimeConfigInvalidationKey(key) {
		return nil
	}
	return redisstore.PublishString(ctx, RuntimeConfigInvalidationChannel, key)
}

func RefreshByKey(ctx context.Context, key string) error {
	switch key {
	case SecurityPolicySettingKey:
		return DefaultSecurityPolicyReader().Refresh(ctx)
	default:
		// The monolith also refreshes the email notification reader here; the
		// auth service does not consume email notification settings, so other
		// keys are ignored.
		return nil
	}
}

func StartInvalidationListener(ctx context.Context) (*redisstore.StringSubscriber, error) {
	return redisstore.StartSubscriber(ctx, RuntimeConfigInvalidationChannel, func(ctx context.Context, payload string) {
		if isRuntimeConfigInvalidationKey(payload) {
			_ = RefreshByKey(ctx, payload)
		}
	})
}

func isRuntimeConfigInvalidationKey(key string) bool {
	return key == SecurityPolicySettingKey
}
