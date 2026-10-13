// Package hub fans real-time call events out to agent browsers over WebSocket.
// 按租户分片投递：呼叫事件含主被叫号码/客户名，BroadcastTenant 只投本租户连接，
// 防跨租户泄露；BroadcastAll 仅留给无敏感字段的全局运维事件。
// 参照 im-service 的 AgentHub（每连接一个 send chan、慢消费者丢弃/踢除），精简掉
// 跨实例 NATS 分发——cc 单实例即可，事件源是本进程 webhook。
package hub

import (
	"encoding/json"
	"sync"
)

// Event 是推给坐席浏览器的实时话务事件。字段与 control-api ESL webhook 对齐，
// 额外富化 CallerName（来电弹屏直接显示客户名）。
type Event struct {
	Type       string `json:"type"` // call.ringing | call.answered | call.hangup
	CallID     string `json:"call_id"`
	Direction  string `json:"direction"` // inbound | outbound
	Caller     string `json:"caller"`
	Callee     string `json:"callee"`
	AgentExt   string `json:"agent_ext,omitempty"`
	Queue      string `json:"queue,omitempty"`
	CallerName string `json:"caller_name,omitempty"` // cc_contacts 命中的客户名，可空
	BillSec    int    `json:"bill_sec,omitempty"`    // call.hangup 时带通话时长
	TS         int64  `json:"ts"`                    // 事件毫秒时间戳
}

// conn 是一条已鉴权的 WS 连接。send 满即视为慢消费者，广播时踢除。
type conn struct {
	userID   uint64
	tenantID uint64
	send     chan []byte
}

// Hub 维护活动连接注册表，按租户分片广播。goroutine 安全。
type Hub struct {
	mu    sync.RWMutex
	conns map[*conn]struct{}
}

func New() *Hub {
	return &Hub{conns: make(map[*conn]struct{})}
}

// Register 登记一条连接（携带租户归属，BroadcastTenant 据此分片），返回其
// send 通道与注销函数。缓冲区 32：短时突发不阻塞广播；持续跟不上（满）则被踢除。
func (h *Hub) Register(userID, tenantID uint64) (<-chan []byte, func()) {
	c := &conn{userID: userID, tenantID: tenantID, send: make(chan []byte, 32)}
	h.mu.Lock()
	h.conns[c] = struct{}{}
	h.mu.Unlock()
	return c.send, func() { h.remove(c) }
}

func (h *Hub) remove(c *conn) {
	h.mu.Lock()
	if _, ok := h.conns[c]; ok {
		delete(h.conns, c)
		close(c.send)
	}
	h.mu.Unlock()
}

// BroadcastTenant 把事件序列化后只投给指定租户的连接（呼叫事件含号码/客户名，
// 必须分片防跨租户泄露）。慢消费者（send 已满）当即踢除并关闭其通道——绝不阻塞
// 调用方（webhook 是热路径）。
func (h *Hub) BroadcastTenant(tenantID uint64, ev Event) {
	h.broadcast(func(c *conn) bool { return c.tenantID == tenantID }, ev)
}

// BroadcastAll 全量广播。仅用于无敏感字段的全局运维事件（如系统公告），
// 呼叫事件一律走 BroadcastTenant。
func (h *Hub) BroadcastAll(ev Event) {
	h.broadcast(func(*conn) bool { return true }, ev)
}

func (h *Hub) broadcast(match func(*conn) bool, ev Event) {
	b, err := json.Marshal(ev)
	if err != nil {
		return
	}
	var slow []*conn
	h.mu.RLock()
	for c := range h.conns {
		if !match(c) {
			continue
		}
		select {
		case c.send <- b:
		default:
			slow = append(slow, c) // 满 → 慢连接，记下待踢
		}
	}
	h.mu.RUnlock()
	for _, c := range slow {
		h.remove(c)
	}
}

// Count 返回当前活动连接数（供健康检查/调试）。
func (h *Hub) Count() int {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return len(h.conns)
}
