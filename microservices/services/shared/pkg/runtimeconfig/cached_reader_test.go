package runtimeconfig

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/go-admin-kit/services/shared/pkg/model"
)

type cachedSettingStore struct {
	setting *model.SystemSetting
	err     error
	calls   atomic.Int32
}

func (s *cachedSettingStore) GetByKeyContext(context.Context, string) (*model.SystemSetting, error) {
	s.calls.Add(1)
	return s.setting, s.err
}

func TestCachedSettingReaderUsesStaticFallbackAndRetainsLastGoodValue(t *testing.T) {
	store := &cachedSettingStore{setting: &model.SystemSetting{ValueJSON: map[string]any{"value": 42}}}
	reader := NewCachedSettingReader(store, "security.policy", time.Hour,
		func() int { return 7 },
		func(value int, payload map[string]any) int {
			if got, ok := payload["value"].(int); ok {
				return got
			}
			return value
		},
	)

	if got := reader.Value(context.Background()); got != 42 {
		t.Fatalf("first value = %d, want 42", got)
	}
	if got := store.calls.Load(); got != 1 {
		t.Fatalf("store calls after first value = %d, want 1", got)
	}
	if got := reader.Value(context.Background()); got != 42 {
		t.Fatalf("cached value = %d, want 42", got)
	}
	if got := store.calls.Load(); got != 1 {
		t.Fatalf("store calls after cached value = %d, want 1", got)
	}

	store.err = errors.New("redis unavailable")
	if err := reader.Refresh(context.Background()); err == nil {
		t.Fatal("Refresh() error = nil, want store error")
	}
	if got := reader.Value(context.Background()); got != 42 {
		t.Fatalf("stale value = %d, want 42", got)
	}
}

func TestCachedSettingReaderFallsBackBeforeFirstSuccessfulLoad(t *testing.T) {
	store := &cachedSettingStore{err: errors.New("store unavailable")}
	reader := NewCachedSettingReader(store, "security.policy", time.Hour,
		func() string { return "static" }, nil)

	if got := reader.Value(context.Background()); got != "static" {
		t.Fatalf("fallback value = %q, want static", got)
	}
}
