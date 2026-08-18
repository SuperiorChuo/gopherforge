package runtimeconfig

import (
	"context"
	"errors"
	"sync"
	"time"

	model "github.com/go-admin-kit/services/shared/pkg/model"
	"gorm.io/gorm"
)

// SettingStore reads one system setting. Services own the concrete DAO so
// runtimeconfig does not depend on a service package.
type SettingStore interface {
	GetByKeyContext(ctx context.Context, key string) (*model.SystemSetting, error)
}

// CachedSettingReader contains the shared TTL, refresh, and stale-value
// fallback state machine used by runtime configuration readers. The value type
// and its setting decoder stay service-owned because their fields differ.
type CachedSettingReader[T any] struct {
	store    SettingStore
	key      string
	ttl      time.Duration
	fallback func() T
	apply    func(T, map[string]any) T

	mu        sync.RWMutex
	value     T
	expiresAt time.Time
	loaded    bool
}

func NewCachedSettingReader[T any](store SettingStore, key string, ttl time.Duration, fallback func() T, apply func(T, map[string]any) T) *CachedSettingReader[T] {
	if ttl <= 0 {
		ttl = 30 * time.Second
	}
	return &CachedSettingReader[T]{
		store:    store,
		key:      key,
		ttl:      ttl,
		fallback: fallback,
		apply:    apply,
	}
}

// Value returns a fresh cached value, retaining the last good value when a
// refresh fails and falling back to the static configuration before the first
// successful load.
func (r *CachedSettingReader[T]) Value(ctx context.Context) T {
	if r == nil {
		return r.fallbackValue()
	}

	now := time.Now()
	r.mu.RLock()
	if r.loaded && now.Before(r.expiresAt) {
		value := r.value
		r.mu.RUnlock()
		return value
	}
	r.mu.RUnlock()

	if err := r.Refresh(ctx); err != nil {
		r.mu.RLock()
		if r.loaded {
			value := r.value
			r.mu.RUnlock()
			return value
		}
		r.mu.RUnlock()
		return r.fallbackValue()
	}

	r.mu.RLock()
	value := r.value
	r.mu.RUnlock()
	return value
}

func (r *CachedSettingReader[T]) Refresh(ctx context.Context) error {
	if r == nil {
		return nil
	}
	if ctx == nil {
		ctx = context.Background()
	}

	value := r.fallbackValue()
	var err error
	if r.store != nil {
		var setting *model.SystemSetting
		setting, err = r.store.GetByKeyContext(ctx, r.key)
		switch {
		case err == nil && setting != nil && r.apply != nil:
			value = r.apply(value, setting.ValueJSON)
		case errors.Is(err, gorm.ErrRecordNotFound):
			err = nil
		}
	}

	if err == nil {
		r.mu.Lock()
		r.value = value
		r.expiresAt = time.Now().Add(r.ttl)
		r.loaded = true
		r.mu.Unlock()
	}
	return err
}

func (r *CachedSettingReader[T]) fallbackValue() T {
	if r != nil && r.fallback != nil {
		return r.fallback()
	}
	var zero T
	return zero
}
