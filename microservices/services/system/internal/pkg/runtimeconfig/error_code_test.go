package runtimeconfig

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/go-admin-kit/services/system/internal/model"
)

type stubErrorCodeStore struct {
	codes []model.ErrorCode
	err   error
	calls int
}

func (s *stubErrorCodeStore) GetAllEnabledContext(_ context.Context) ([]model.ErrorCode, error) {
	s.calls++
	if s.err != nil {
		return nil, s.err
	}
	return s.codes, nil
}

func TestErrorCodeReaderCachesWithinTTL(t *testing.T) {
	store := &stubErrorCodeStore{codes: []model.ErrorCode{
		{Code: "DICT_TYPE_NOT_FOUND", Message: "字典类型不存在", Status: 1},
	}}
	reader := NewCachedErrorCodeReader(store, time.Minute)

	message, ok := reader.GetMessage(context.Background(), "DICT_TYPE_NOT_FOUND")
	if !ok || message != "字典类型不存在" {
		t.Fatalf("GetMessage() = (%q, %v), want custom message hit", message, ok)
	}

	// TTL 内第二次读取必须走内存缓存，不再查库
	if _, ok := reader.GetMessage(context.Background(), "DICT_TYPE_NOT_FOUND"); !ok {
		t.Fatal("GetMessage() second call missed cache")
	}
	if store.calls != 1 {
		t.Fatalf("store calls = %d, want 1 (cached within TTL)", store.calls)
	}

	// 未配置的错误码返回 miss，调用方回落默认文案
	if message, ok := reader.GetMessage(context.Background(), "NOT_CONFIGURED"); ok {
		t.Fatalf("GetMessage(unknown) = (%q, true), want miss", message)
	}
}

func TestErrorCodeReaderRefreshesAfterExpiry(t *testing.T) {
	store := &stubErrorCodeStore{codes: []model.ErrorCode{
		{Code: "NOTICE_NOT_FOUND", Message: "旧文案", Status: 1},
	}}
	reader := NewCachedErrorCodeReader(store, time.Minute)

	if message, _ := reader.GetMessage(context.Background(), "NOTICE_NOT_FOUND"); message != "旧文案" {
		t.Fatalf("initial message = %q, want 旧文案", message)
	}

	// 模拟控制台改文案 + TTL 到期：下一次读取应重新查库拿到新文案（热生效）
	store.codes = []model.ErrorCode{{Code: "NOTICE_NOT_FOUND", Message: "新文案", Status: 1}}
	reader.mu.Lock()
	reader.expiresAt = time.Now().Add(-time.Second)
	reader.mu.Unlock()

	message, ok := reader.GetMessage(context.Background(), "NOTICE_NOT_FOUND")
	if !ok || message != "新文案" {
		t.Fatalf("GetMessage() after expiry = (%q, %v), want refreshed 新文案", message, ok)
	}
	if store.calls != 2 {
		t.Fatalf("store calls = %d, want 2 (refreshed after expiry)", store.calls)
	}
}

func TestErrorCodeReaderServesStaleOnRefreshFailure(t *testing.T) {
	store := &stubErrorCodeStore{codes: []model.ErrorCode{
		{Code: "FORBIDDEN", Message: "权限不足", Status: 1},
	}}
	reader := NewCachedErrorCodeReader(store, time.Minute)

	if _, ok := reader.GetMessage(context.Background(), "FORBIDDEN"); !ok {
		t.Fatal("initial GetMessage() missed")
	}

	// 缓存过期且数据库故障：降级返回旧缓存，错误路径不被拖垮
	store.err = errors.New("db down")
	reader.mu.Lock()
	reader.expiresAt = time.Now().Add(-time.Second)
	reader.mu.Unlock()

	message, ok := reader.GetMessage(context.Background(), "FORBIDDEN")
	if !ok || message != "权限不足" {
		t.Fatalf("GetMessage() with store failure = (%q, %v), want stale cache", message, ok)
	}
}

func TestErrorCodeReaderMissesWhenNeverLoaded(t *testing.T) {
	store := &stubErrorCodeStore{err: errors.New("db down")}
	reader := NewCachedErrorCodeReader(store, time.Minute)

	if message, ok := reader.GetMessage(context.Background(), "ANY"); ok {
		t.Fatalf("GetMessage() = (%q, true), want miss when store unavailable", message)
	}
}
