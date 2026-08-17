// Package runtimeconfig contains shared infrastructure for runtime setting
// invalidation. Setting keys and refresh behavior remain owned by each service.
package runtimeconfig

import (
	"context"

	redisstore "github.com/go-admin-kit/services/shared/pkg/redis"
)

// InvalidationChannel is the cross-service channel used for runtime setting
// cache invalidations. Every service may subscribe to the same channel while
// only handling the keys it actually consumes.
const InvalidationChannel = "go_admin_kit:runtime_config:invalidate"

// InvalidationHandler binds service-owned key filtering and refresh behavior to
// the shared Redis lifecycle. A nil IsSupported function means no key is
// supported, which keeps an incompletely wired handler fail-closed.
type InvalidationHandler struct {
	Channel     string
	IsSupported func(string) bool
	Refresh     func(context.Context, string) error
}

// Publish sends a supported setting key to the shared invalidation channel.
// Unsupported keys are ignored so a service cannot publish a setting it does
// not own.
func (h InvalidationHandler) Publish(ctx context.Context, key string) error {
	if !h.supported(key) {
		return nil
	}
	return redisstore.PublishString(ctx, h.channel(), key)
}

// Start subscribes to the shared invalidation channel and refreshes supported
// keys. Refresh errors intentionally remain best-effort, matching the
// existing service behavior: the next TTL expiry or explicit read retries.
func (h InvalidationHandler) Start(ctx context.Context) (*redisstore.StringSubscriber, error) {
	return redisstore.StartSubscriber(ctx, h.channel(), func(refreshCtx context.Context, payload string) {
		if h.supported(payload) && h.Refresh != nil {
			_ = h.Refresh(refreshCtx, payload)
		}
	})
}

func (h InvalidationHandler) channel() string {
	if h.Channel == "" {
		return InvalidationChannel
	}
	return h.Channel
}

func (h InvalidationHandler) supported(key string) bool {
	return h.IsSupported != nil && h.IsSupported(key)
}
