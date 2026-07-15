package runtimeconfig

import (
	"context"

	redisstore "github.com/go-admin-kit/server/internal/pkg/redis"
)

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
	case EmailNotificationSettingKey:
		return DefaultEmailNotificationReader().Refresh(ctx)
	default:
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
	switch key {
	case SecurityPolicySettingKey, EmailNotificationSettingKey:
		return true
	default:
		return false
	}
}
