package hub

import (
	"strings"
	"testing"
	"time"
)

func recv(t *testing.T, ch chan []byte) string {
	t.Helper()
	select {
	case b := <-ch:
		return string(b)
	case <-time.After(time.Second):
		t.Fatal("no message within 1s")
		return ""
	}
}

func TestLocalPublishSubscribe(t *testing.T) {
	h := New()
	ch := Subscribe2(t, h, "c1")
	h.Publish("c1", map[string]string{"type": "x"})
	if got := recv(t, ch); !strings.Contains(got, `"type":"x"`) {
		t.Fatalf("got %s", got)
	}
	// 其他会话不得收到
	other := Subscribe2(t, h, "c2")
	h.Publish("c1", map[string]string{"type": "y"})
	select {
	case b := <-other:
		t.Fatalf("cross-conversation leak: %s", b)
	case <-time.After(100 * time.Millisecond):
	}
}

func Subscribe2(t *testing.T, h *Hub, pid string) chan []byte {
	t.Helper()
	ch := h.Subscribe(pid)
	t.Cleanup(func() {
		defer func() { _ = recover() }() // 清理顺序的重复 close 保护
		h.Unsubscribe(pid, ch)
	})
	return ch
}

// 安装远端发布器后，Publish 必须恰好经过它一次，
// 本地投递仅通过 Dispatch（broker 接收）发生——不重复。
func TestRemoteRoutingNoDuplicates(t *testing.T) {
	h := New()
	ch := Subscribe2(t, h, "c1")
	calls := 0
	h.SetRemote(func(pid string, b []byte) error {
		calls++
		h.Dispatch(pid, b) // 环回，模拟 broker 回显
		return nil
	})
	h.Publish("c1", map[string]string{"type": "x"})
	recv(t, ch)
	select {
	case b := <-ch:
		t.Fatalf("duplicate delivery: %s", b)
	case <-time.After(100 * time.Millisecond):
	}
	if calls != 1 {
		t.Fatalf("remote called %d times", calls)
	}
}

// broker 故障降级为直接本地分发。
func TestRemoteFailureFallsBackLocal(t *testing.T) {
	h := New()
	ch := Subscribe2(t, h, "c1")
	h.SetRemote(func(string, []byte) error { return errFake })
	h.Publish("c1", map[string]string{"type": "x"})
	if got := recv(t, ch); !strings.Contains(got, `"type":"x"`) {
		t.Fatalf("fallback missing: %s", got)
	}
}

var errFake = &fakeErr{}

type fakeErr struct{}

func (*fakeErr) Error() string { return "broker down" }

func TestAgentHubRemote(t *testing.T) {
	ah := NewAgentHub()
	ch := ah.Subscribe(7)
	t.Cleanup(func() {
		defer func() { _ = recover() }()
		ah.Unsubscribe(7, ch)
	})
	calls := 0
	ah.SetRemote(func(tid uint64, b []byte) error { calls++; ah.Dispatch(tid, b); return nil })
	ah.Publish(7, map[string]string{"type": "queue.updated"})
	if got := recv(t, ch); !strings.Contains(got, "queue.updated") {
		t.Fatalf("got %s", got)
	}
	if calls != 1 {
		t.Fatalf("remote called %d times", calls)
	}
}

// 租户隔离：在租户 A 下订阅的坐席绝不能收到
// 租户 B 的事件，无论本地还是经 broker 路径。
func TestAgentHubTenantIsolation(t *testing.T) {
	ah := NewAgentHub()
	chA := ah.Subscribe(1)
	chB := ah.Subscribe(2)
	t.Cleanup(func() {
		defer func() { _ = recover() }()
		ah.Unsubscribe(1, chA)
		ah.Unsubscribe(2, chB)
	})

	ah.Publish(2, map[string]string{"type": "conversation.updated", "who": "tenantB"})
	if got := recv(t, chB); !strings.Contains(got, "tenantB") {
		t.Fatalf("tenant B missed own event: %s", got)
	}
	select {
	case b := <-chA:
		t.Fatalf("cross-tenant leak to tenant A: %s", b)
	case <-time.After(100 * time.Millisecond):
	}

	// broker 路径也保持分片
	ah.SetRemote(func(tid uint64, b []byte) error { ah.Dispatch(tid, b); return nil })
	ah.Publish(1, map[string]string{"type": "queue.updated", "who": "tenantA"})
	if got := recv(t, chA); !strings.Contains(got, "tenantA") {
		t.Fatalf("tenant A missed own event: %s", got)
	}
	select {
	case b := <-chB:
		t.Fatalf("cross-tenant leak to tenant B via remote: %s", b)
	case <-time.After(100 * time.Millisecond):
	}
}

// 遗留的 tenant 0 在两端都归一化为默认租户（1）。
func TestAgentHubZeroTenantNormalized(t *testing.T) {
	ah := NewAgentHub()
	ch := ah.Subscribe(0) // 存于 tenant 1 之下
	t.Cleanup(func() {
		defer func() { _ = recover() }()
		ah.Unsubscribe(0, ch)
	})
	ah.Publish(1, map[string]string{"type": "x"})
	if got := recv(t, ch); !strings.Contains(got, `"type":"x"`) {
		t.Fatalf("zero-tenant subscriber missed default-tenant event: %s", got)
	}
}
