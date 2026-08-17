package runtimeconfig

import (
	"context"

	redisstore "github.com/go-admin-kit/services/shared/pkg/redis"
	sharedruntimeconfig "github.com/go-admin-kit/services/shared/pkg/runtimeconfig"
)

const RuntimeConfigInvalidationChannel = sharedruntimeconfig.InvalidationChannel

func PublishInvalidation(ctx context.Context, key string) error {
	return sharedruntimeconfig.InvalidationHandler{
		Channel: RuntimeConfigInvalidationChannel, IsSupported: isRuntimeConfigInvalidationKey, Refresh: RefreshByKey,
	}.Publish(ctx, key)
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
	return sharedruntimeconfig.InvalidationHandler{
		Channel: RuntimeConfigInvalidationChannel, IsSupported: isRuntimeConfigInvalidationKey, Refresh: RefreshByKey,
	}.Start(ctx)
}

func isRuntimeConfigInvalidationKey(key string) bool {
	return key == SecurityPolicySettingKey
}
