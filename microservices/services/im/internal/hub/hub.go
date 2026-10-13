package hub

import (
	"encoding/json"
	"sync"
)

// Hub 将会话事件扇出给 WS 订阅者。单实例使用进程内分发；
// 配置了远端发布器（NATS）时，Publish 经 broker 发出，
// 每个实例收到后在本地分发。
type Hub struct {
	mu   sync.RWMutex
	subs map[string]map[chan []byte]struct{} // 会话 public_id -> channel 集合
	// remote 一旦设置，就在发布侧取代本地分发。
	remote func(convPublicID string, b []byte) error
}

func New() *Hub {
	return &Hub{subs: make(map[string]map[chan []byte]struct{})}
}

// SetRemote 安装跨实例发布器（见 ConnectNATS）。
func (h *Hub) SetRemote(fn func(convPublicID string, b []byte) error) {
	h.mu.Lock()
	h.remote = fn
	h.mu.Unlock()
}

func (h *Hub) Subscribe(convPublicID string) chan []byte {
	ch := make(chan []byte, 32)
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.subs[convPublicID] == nil {
		h.subs[convPublicID] = make(map[chan []byte]struct{})
	}
	h.subs[convPublicID][ch] = struct{}{}
	return ch
}

func (h *Hub) Unsubscribe(convPublicID string, ch chan []byte) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if set, ok := h.subs[convPublicID]; ok {
		delete(set, ch)
		if len(set) == 0 {
			delete(h.subs, convPublicID)
		}
	}
	close(ch)
}

func (h *Hub) Publish(convPublicID string, v any) {
	b, err := json.Marshal(v)
	if err != nil {
		return
	}
	h.mu.RLock()
	remote := h.remote
	h.mu.RUnlock()
	if remote != nil {
		if remote(convPublicID, b) == nil {
			return // 本地投递经由 broker 订阅完成
		}
		// broker 不可用：降级为本地分发，让幸存的单个实例继续工作
	}
	h.Dispatch(convPublicID, b)
}

// Dispatch 只投递给本地订阅者（broker 接收路径）。
func (h *Hub) Dispatch(convPublicID string, b []byte) {
	h.mu.RLock()
	defer h.mu.RUnlock()
	for ch := range h.subs[convPublicID] {
		select {
		case ch <- b:
		default:
			// 慢消费者时丢弃
		}
	}
}

// AgentHub 将工作台级别的更新广播给坐席，按 tenant 分片，
// 使一个租户的事件（会话预览、队列状态、已读游标…）绝不会
// 到达其他租户的坐席。远端语义与 Hub 相同。
type AgentHub struct {
	mu     sync.RWMutex
	subs   map[uint64]map[chan []byte]struct{} // tenant id -> channel 集合
	remote func(tenantID uint64, b []byte) error
}

func NewAgentHub() *AgentHub {
	return &AgentHub{subs: make(map[uint64]map[chan []byte]struct{})}
}

// normalizeTenant 将零值/遗留 tenant 映射到默认租户（id=1），
// 与 authjwt.NormalizeTenantID 保持一致，使发布与订阅两侧口径一致。
func normalizeTenant(id uint64) uint64 {
	if id == 0 {
		return 1
	}
	return id
}

func (h *AgentHub) SetRemote(fn func(tenantID uint64, b []byte) error) {
	h.mu.Lock()
	h.remote = fn
	h.mu.Unlock()
}

func (h *AgentHub) Subscribe(tenantID uint64) chan []byte {
	tenantID = normalizeTenant(tenantID)
	ch := make(chan []byte, 32)
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.subs[tenantID] == nil {
		h.subs[tenantID] = make(map[chan []byte]struct{})
	}
	h.subs[tenantID][ch] = struct{}{}
	return ch
}

func (h *AgentHub) Unsubscribe(tenantID uint64, ch chan []byte) {
	tenantID = normalizeTenant(tenantID)
	h.mu.Lock()
	defer h.mu.Unlock()
	if set, ok := h.subs[tenantID]; ok {
		delete(set, ch)
		if len(set) == 0 {
			delete(h.subs, tenantID)
		}
	}
	close(ch)
}

// Publish 只把 v 扇出给该租户的坐席。tenantID 必须取自事件所属实体
// （会话/坐席声明），绝不能取自载荷本身。
func (h *AgentHub) Publish(tenantID uint64, v any) {
	tenantID = normalizeTenant(tenantID)
	b, err := json.Marshal(v)
	if err != nil {
		return
	}
	h.mu.RLock()
	remote := h.remote
	h.mu.RUnlock()
	if remote != nil {
		if remote(tenantID, b) == nil {
			return
		}
	}
	h.Dispatch(tenantID, b)
}

func (h *AgentHub) Dispatch(tenantID uint64, b []byte) {
	tenantID = normalizeTenant(tenantID)
	h.mu.RLock()
	defer h.mu.RUnlock()
	for ch := range h.subs[tenantID] {
		select {
		case ch <- b:
		default:
		}
	}
}
