package hub

import (
	"encoding/json"
	"testing"
	"time"
)

func recvEvent(t *testing.T, ch <-chan []byte) Event {
	t.Helper()
	select {
	case b := <-ch:
		var ev Event
		if err := json.Unmarshal(b, &ev); err != nil {
			t.Fatalf("unmarshal event: %v", err)
		}
		return ev
	case <-time.After(time.Second):
		t.Fatal("no event within 1s")
		return Event{}
	}
}

func assertNoEvent(t *testing.T, ch <-chan []byte) {
	t.Helper()
	select {
	case b := <-ch:
		t.Fatalf("unexpected event: %s", string(b))
	case <-time.After(200 * time.Millisecond):
		// 符合预期：无投递
	}
}

func TestRegisterBroadcastTenant(t *testing.T) {
	h := New()
	ch, unregister := h.Register(7, 1)
	defer unregister()

	if h.Count() != 1 {
		t.Fatalf("count = %d, want 1", h.Count())
	}
	h.BroadcastTenant(1, Event{Type: "call.ringing", CallID: "c1", Caller: "13800000000"})
	ev := recvEvent(t, ch)
	if ev.Type != "call.ringing" || ev.CallID != "c1" || ev.Caller != "13800000000" {
		t.Fatalf("got %+v", ev)
	}
}

// 租户分片：A 租户的事件绝不投给 B 租户的连接（呼叫事件含手机号/客户名）。
func TestBroadcastTenantIsolation(t *testing.T) {
	h := New()
	chA, unA := h.Register(1, 2) // 租户 2 的坐席
	defer unA()
	chB, unB := h.Register(2, 3) // 租户 3 的坐席
	defer unB()

	h.BroadcastTenant(2, Event{Type: "call.ringing", CallID: "c-t2", Caller: "13800000000"})
	if a := recvEvent(t, chA); a.CallID != "c-t2" {
		t.Fatalf("tenant-2 conn got %+v", a)
	}
	assertNoEvent(t, chB)

	h.BroadcastTenant(3, Event{Type: "call.answered", CallID: "c-t3"})
	if b := recvEvent(t, chB); b.CallID != "c-t3" {
		t.Fatalf("tenant-3 conn got %+v", b)
	}
	assertNoEvent(t, chA)
}

// 同租户多连接照常扇出。
func TestBroadcastTenantFanOut(t *testing.T) {
	h := New()
	chA, unA := h.Register(1, 5)
	defer unA()
	chB, unB := h.Register(2, 5)
	defer unB()

	h.BroadcastTenant(5, Event{Type: "call.answered", CallID: "c2"})
	if a := recvEvent(t, chA); a.CallID != "c2" {
		t.Fatalf("conn A got %+v", a)
	}
	if b := recvEvent(t, chB); b.CallID != "c2" {
		t.Fatalf("conn B got %+v", b)
	}
}

// BroadcastAll 跨租户全播（仅限无敏感字段的全局运维事件）。
func TestBroadcastAllReachesAllTenants(t *testing.T) {
	h := New()
	chA, unA := h.Register(1, 2)
	defer unA()
	chB, unB := h.Register(2, 3)
	defer unB()

	h.BroadcastAll(Event{Type: "system.notice", CallID: "n1"})
	if a := recvEvent(t, chA); a.CallID != "n1" {
		t.Fatalf("conn A got %+v", a)
	}
	if b := recvEvent(t, chB); b.CallID != "n1" {
		t.Fatalf("conn B got %+v", b)
	}
}

func TestUnregisterClosesChannel(t *testing.T) {
	h := New()
	ch, unregister := h.Register(3, 1)
	unregister()
	if h.Count() != 0 {
		t.Fatalf("count = %d, want 0 after unregister", h.Count())
	}
	// 通道应已关闭：读取立即返回零值且 ok=false
	select {
	case _, ok := <-ch:
		if ok {
			t.Fatal("channel should be closed and drained")
		}
	case <-time.After(time.Second):
		t.Fatal("closed channel must not block")
	}
	// 二次注销不应 panic
	unregister()
}

// 慢消费者（不读 send）在缓冲填满后被广播踢除并关闭通道。
func TestSlowConnEvicted(t *testing.T) {
	h := New()
	ch, unregister := h.Register(4, 1)
	defer unregister()

	// 缓冲区 32；连发 40 条不读，撑满后该连接被踢。
	for i := 0; i < 40; i++ {
		h.BroadcastTenant(1, Event{Type: "call.ringing", CallID: "flood"})
	}
	// 给踢除一点时间（广播同步踢，但保险起见轮询）。
	deadline := time.Now().Add(time.Second)
	for h.Count() != 0 && time.Now().Before(deadline) {
		h.BroadcastTenant(1, Event{Type: "call.ringing", CallID: "flood"})
	}
	if h.Count() != 0 {
		t.Fatalf("slow conn not evicted, count = %d", h.Count())
	}
	// 被踢后通道最终关闭：排空缓冲后应读到 ok=false。
	for {
		select {
		case _, ok := <-ch:
			if !ok {
				return // 关闭到达，符合预期
			}
		case <-time.After(time.Second):
			t.Fatal("evicted channel never closed")
		}
	}
}
