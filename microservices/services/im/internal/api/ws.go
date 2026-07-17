package api

import (
	"encoding/json"
	"net/http"
	"sync"

	"github.com/gin-gonic/gin"
	"github.com/go-admin-kit/services/im/internal/authjwt"
	"github.com/go-admin-kit/services/im/internal/model"
	"github.com/go-admin-kit/services/im/internal/store"
	"github.com/gorilla/websocket"
)

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool { return true },
}

type wsClientMsg struct {
	Type      string          `json:"type"`
	RequestID string          `json:"request_id"`
	Payload   json.RawMessage `json:"payload"`
}

type wsSendPayload struct {
	ConversationPublicID string         `json:"conversation_public_id"`
	ClientMsgID          string         `json:"client_msg_id"`
	MsgType              string         `json:"msg_type"`
	Content              map[string]any `json:"content"`
}

// GET /im/ws?token=
func (s *Server) WebSocket(c *gin.Context) {
	token := c.Query("token")
	if token == "" {
		token = bearer(c)
	}
	if token == "" {
		Fail(c, http.StatusUnauthorized, "missing token")
		return
	}

	var (
		isGuest     bool
		visitorID   uint64
		agentID     uint64
		agentTenant uint64
	)
	if g, err := authjwt.ParseGuest(s.Secret, token); err == nil {
		isGuest = true
		visitorID = g.VisitorID
	} else if a, err := authjwt.ParseAgent(s.Secret, token); err == nil {
		agentID = a.UserID
		// Prefer gateway header when WS is proxied; else JWT claim (default 1).
		agentTenant = resolveAgentTenantID(c, a.TenantID)
	} else {
		Fail(c, http.StatusUnauthorized, "invalid token")
		return
	}

	conn, err := upgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		return
	}
	defer conn.Close()

	out := make(chan []byte, 64)
	var once sync.Once
	closeOut := func() { once.Do(func() { close(out) }) }
	defer closeOut()

	// writer
	go func() {
		for b := range out {
			if err := conn.WriteMessage(websocket.TextMessage, b); err != nil {
				return
			}
		}
	}()

	writeJSON := func(v any) {
		b, err := json.Marshal(v)
		if err != nil {
			return
		}
		select {
		case out <- b:
		default:
		}
	}

	// fan-in subscriptions
	var mu sync.Mutex
	var unsubs []func()

	addSub := func(ch <-chan []byte, unsub func()) {
		mu.Lock()
		unsubs = append(unsubs, unsub)
		mu.Unlock()
		go func() {
			for b := range ch {
				select {
				case out <- b:
				default:
				}
			}
		}()
	}

	defer func() {
		mu.Lock()
		for _, u := range unsubs {
			u()
		}
		mu.Unlock()
	}()

	if !isGuest {
		ch := s.AgentHub.Subscribe()
		addSub(ch, func() { s.AgentHub.Unsubscribe(ch) })
	}

	writeJSON(gin.H{
		"type": "auth_ok",
		"payload": gin.H{
			"role": map[bool]string{true: "visitor", false: "agent"}[isGuest],
		},
	})

	for {
		_, data, err := conn.ReadMessage()
		if err != nil {
			break
		}
		var msg wsClientMsg
		if err := json.Unmarshal(data, &msg); err != nil {
			writeJSON(gin.H{"type": "error", "payload": gin.H{"message": "bad json"}})
			continue
		}
		switch msg.Type {
		case "ping":
			writeJSON(gin.H{"type": "pong"})
		case "conversation.subscribe":
			var p struct {
				ConversationPublicID string `json:"conversation_public_id"`
			}
			_ = json.Unmarshal(msg.Payload, &p)
			if p.ConversationPublicID == "" {
				continue
			}
			conv, err := s.Store.GetConversationByPublicID(p.ConversationPublicID)
			if err != nil {
				writeJSON(gin.H{"type": "error", "payload": gin.H{"message": "conversation not found"}})
				continue
			}
			if isGuest && conv.VisitorID != visitorID {
				writeJSON(gin.H{"type": "error", "payload": gin.H{"message": "forbidden"}})
				continue
			}
			if !isGuest && conv.TenantID != 0 && conv.TenantID != agentTenant {
				writeJSON(gin.H{"type": "error", "payload": gin.H{"message": "forbidden"}})
				continue
			}
			ch := s.Hub.Subscribe(p.ConversationPublicID)
			pid := p.ConversationPublicID
			addSub(ch, func() { s.Hub.Unsubscribe(pid, ch) })
			writeJSON(gin.H{"type": "subscribed", "payload": gin.H{"conversation_public_id": p.ConversationPublicID}})
		case "message.send":
			var p wsSendPayload
			if err := json.Unmarshal(msg.Payload, &p); err != nil || p.ConversationPublicID == "" || p.Content == nil {
				writeJSON(gin.H{"type": "error", "request_id": msg.RequestID, "payload": gin.H{"message": "bad payload"}})
				continue
			}
			if p.MsgType == "" {
				p.MsgType = "text"
			}
			conv, err := s.Store.GetConversationByPublicID(p.ConversationPublicID)
			if err != nil {
				writeJSON(gin.H{"type": "error", "request_id": msg.RequestID, "payload": gin.H{"message": "conversation not found"}})
				continue
			}
			var senderType string
			var senderID uint64
			if isGuest {
				if conv.VisitorID != visitorID {
					writeJSON(gin.H{"type": "error", "request_id": msg.RequestID, "payload": gin.H{"message": "forbidden"}})
					continue
				}
				senderType = "visitor"
				senderID = visitorID
			} else {
				if conv.TenantID != 0 && conv.TenantID != agentTenant {
					writeJSON(gin.H{"type": "error", "request_id": msg.RequestID, "payload": gin.H{"message": "forbidden"}})
					continue
				}
				senderType = "agent"
				senderID = agentID
				if conv.Status == "queued" || conv.AgentUserID == nil {
					_ = s.Store.AssignConversation(conv, agentID)
				}
			}
			var clientID *string
			if p.ClientMsgID != "" {
				clientID = &p.ClientMsgID
			}
			m := &model.Message{
				ConversationID: conv.ID,
				ClientMsgID:    clientID,
				SenderType:     senderType,
				SenderID:       &senderID,
				MsgType:        p.MsgType,
				Content:        store.JSONText(p.Content),
			}
			if err := s.Store.CreateMessage(m); err != nil {
				writeJSON(gin.H{"type": "error", "request_id": msg.RequestID, "payload": gin.H{"message": err.Error()}})
				continue
			}
			// 幂等重放：只回 ack（同 seq/id），不重播事件、不再触发 bot
			if m.Replayed {
				writeJSON(gin.H{
					"type":       "message.ack",
					"request_id": msg.RequestID,
					"payload":    gin.H{"client_msg_id": p.ClientMsgID, "seq": m.Seq, "id": m.ID},
				})
				continue
			}
			conv2, _ := s.Store.GetConversationByPublicID(p.ConversationPublicID)
			outMsg := gin.H{
				"type": "message.new",
				"payload": gin.H{
					"message":                m,
					"conversation_public_id": p.ConversationPublicID,
				},
			}
			s.Hub.Publish(p.ConversationPublicID, outMsg)
			s.AgentHub.Publish(gin.H{"type": "conversation.updated", "payload": conv2})
			writeJSON(gin.H{
				"type":       "message.ack",
				"request_id": msg.RequestID,
				"payload":    gin.H{"client_msg_id": p.ClientMsgID, "seq": m.Seq, "id": m.ID},
			})
			// M4 bot path (same as HTTP send)
			if senderType == "visitor" && conv2 != nil {
				text := ""
				if t, ok := p.Content["text"].(string); ok {
					text = t
				}
				s.afterVisitorMessage(conv2, text)
			}
		}
	}
}
