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
	// other conversation must not receive
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
		defer func() { _ = recover() }() // double-close guard for cleanup order
		h.Unsubscribe(pid, ch)
	})
	return ch
}

// With a remote publisher installed, Publish must go through it exactly once
// and local delivery happens only via Dispatch (broker receipt) — no doubles.
func TestRemoteRoutingNoDuplicates(t *testing.T) {
	h := New()
	ch := Subscribe2(t, h, "c1")
	calls := 0
	h.SetRemote(func(pid string, b []byte) error {
		calls++
		h.Dispatch(pid, b) // loopback like a broker echo
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

// Broker failure degrades to direct local dispatch.
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
	ch := ah.Subscribe()
	t.Cleanup(func() {
		defer func() { _ = recover() }()
		ah.Unsubscribe(ch)
	})
	calls := 0
	ah.SetRemote(func(b []byte) error { calls++; ah.Dispatch(b); return nil })
	ah.Publish(map[string]string{"type": "queue.updated"})
	if got := recv(t, ch); !strings.Contains(got, "queue.updated") {
		t.Fatalf("got %s", got)
	}
	if calls != 1 {
		t.Fatalf("remote called %d times", calls)
	}
}
