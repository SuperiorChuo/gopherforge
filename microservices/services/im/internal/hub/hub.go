package hub

import (
	"encoding/json"
	"sync"
)

// Hub fans conversation events out to WS subscribers. Single instance uses
// in-process dispatch; with a remote publisher set (NATS), Publish goes
// through the broker and every instance dispatches locally on receipt.
type Hub struct {
	mu   sync.RWMutex
	subs map[string]map[chan []byte]struct{} // conversation public_id -> set of chans
	// remote, when set, replaces local dispatch on the publish side.
	remote func(convPublicID string, b []byte) error
}

func New() *Hub {
	return &Hub{subs: make(map[string]map[chan []byte]struct{})}
}

// SetRemote installs a cross-instance publisher (see ConnectNATS).
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
			return // local delivery happens via the broker subscription
		}
		// broker down: degrade to local so the single surviving instance keeps working
	}
	h.Dispatch(convPublicID, b)
}

// Dispatch delivers to local subscribers only (broker receipt path).
func (h *Hub) Dispatch(convPublicID string, b []byte) {
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

// AgentHub broadcasts desk-wide updates (all agents), same remote semantics.
type AgentHub struct {
	mu     sync.RWMutex
	subs   map[chan []byte]struct{}
	remote func(b []byte) error
}

func NewAgentHub() *AgentHub {
	return &AgentHub{subs: make(map[chan []byte]struct{})}
}

func (h *AgentHub) SetRemote(fn func(b []byte) error) {
	h.mu.Lock()
	h.remote = fn
	h.mu.Unlock()
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
	remote := h.remote
	h.mu.RUnlock()
	if remote != nil {
		if remote(b) == nil {
			return
		}
	}
	h.Dispatch(b)
}

func (h *AgentHub) Dispatch(b []byte) {
	h.mu.RLock()
	defer h.mu.RUnlock()
	for ch := range h.subs {
		select {
		case ch <- b:
		default:
		}
	}
}
