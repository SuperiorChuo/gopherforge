package channel

import (
	"errors"
	"reflect"
	"testing"
)

// 注册表是"本部署支持哪些渠道"的唯一事实源：空注册表必须如实返回空，
// 而不是让调用方以为还有个 mock 兜底。
func TestEmptyRegistryExposesNoChannels(t *testing.T) {
	r := NewRegistry()
	if got := r.Codes(); len(got) != 0 {
		t.Fatalf("Codes() = %v, want empty for a registry with no adapters", got)
	}
	if r.Has(MockCode) {
		t.Fatal("Has(mock) = true on an empty registry")
	}
	if _, err := r.Get(MockCode); !errors.Is(err, ErrUnknownChannel) {
		t.Fatalf("Get(mock) error = %v, want ErrUnknownChannel", err)
	}
}

func TestRegistryReportsRegisteredChannels(t *testing.T) {
	r := NewRegistry(Mock{})
	if !r.Has(MockCode) {
		t.Fatal("Has(mock) = false after registering the mock channel")
	}
	if got := r.Codes(); !reflect.DeepEqual(got, []string{MockCode}) {
		t.Fatalf("Codes() = %v, want [%s]", got, MockCode)
	}
}
