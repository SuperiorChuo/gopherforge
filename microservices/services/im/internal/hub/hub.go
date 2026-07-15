package hub

import (
	"encoding/json"
	"sync"
)

// Hub is an in-process pub/sub for M1 (single instance).
type Hub struct {
	mu   sync.RWMutex
	subs map[string]map[chan []byte]struct{} // conversation public_id -> set of chans
}

func New() *Hub {
	return &Hub{subs: make(map[string]map[chan []byte]struct{})}
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
	defer h.mu.RUnlock()
	for ch := range h.subs[convPublicID] {
		select {
		case ch <- b:
		default:
			// drop if slow consumer
		}
	}
}

// AgentBroadcast for agent desk listing updates (all agents).
type AgentHub struct {
	mu   sync.RWMutex
	subs map[chan []byte]struct{}
}

func NewAgentHub() *AgentHub {
	return &AgentHub{subs: make(map[chan []byte]struct{})}
}

func (h *AgentHub) Subscribe() chan []byte {
	ch := make(chan []byte, 32)
	h.mu.Lock()
	h.subs[ch] = struct{}{}
	h.mu.Unlock()
	return ch
}

func (h *AgentHub) Unsubscribe(ch chan []byte) {
	h.mu.Lock()
	delete(h.subs, ch)
	h.mu.Unlock()
	close(ch)
}

func (h *AgentHub) Publish(v any) {
	b, err := json.Marshal(v)
	if err != nil {
		return
	}
	h.mu.RLock()
	defer h.mu.RUnlock()
	for ch := range h.subs {
		select {
		case ch <- b:
		default:
		}
	}
}
