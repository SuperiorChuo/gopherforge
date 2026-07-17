package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/go-admin-kit/services/im/internal/ratelimit"
	"github.com/gorilla/websocket"
)

type wsFrame struct {
	Type      string          `json:"type"`
	RequestID string          `json:"request_id"`
	Payload   json.RawMessage `json:"payload"`
}

func dialWS(t *testing.T, ts *httptest.Server, token string) *websocket.Conn {
	t.Helper()
	url := "ws" + strings.TrimPrefix(ts.URL, "http") + "/im/ws?token=" + token
	conn, resp, err := websocket.DefaultDialer.Dial(url, nil)
	if err != nil {
		code := 0
		if resp != nil {
			code = resp.StatusCode
		}
		t.Fatalf("dial: %v (status %d)", err, code)
	}
	t.Cleanup(func() { _ = conn.Close() })
	// first frame is auth_ok
	f := readFrame(t, conn)
	if f.Type != "auth_ok" {
		t.Fatalf("expected auth_ok, got %s", f.Type)
	}
	return conn
}

func readFrame(t *testing.T, conn *websocket.Conn) wsFrame {
	t.Helper()
	_ = conn.SetReadDeadline(time.Now().Add(3 * time.Second))
	var f wsFrame
	if err := conn.ReadJSON(&f); err != nil {
		t.Fatalf("read frame: %v", err)
	}
	return f
}

// readUntil skips frames until one of the wanted types arrives.
func readUntil(t *testing.T, conn *websocket.Conn, types ...string) wsFrame {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		f := readFrame(t, conn)
		for _, want := range types {
			if f.Type == want {
				return f
			}
		}
	}
	t.Fatalf("no frame of type %v before deadline", types)
	return wsFrame{}
}

func send(t *testing.T, conn *websocket.Conn, v any) {
	t.Helper()
	if err := conn.WriteJSON(v); err != nil {
		t.Fatal(err)
	}
}

func wsServer(t *testing.T) (*httptest.Server, *gin.Engine, *Server) {
	r, srv := newTestRouterWithServer(t)
	ts := httptest.NewServer(r)
	t.Cleanup(ts.Close)
	return ts, r, srv
}

func TestWSRejectsBadToken(t *testing.T) {
	ts, _, _ := wsServer(t)
	url := "ws" + strings.TrimPrefix(ts.URL, "http") + "/im/ws?token=garbage"
	_, resp, err := websocket.DefaultDialer.Dial(url, nil)
	if err == nil {
		t.Fatal("dial with bad token succeeded")
	}
	if resp == nil || resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("want 401 handshake, got %+v", resp)
	}
}

func TestWSSubscribeForbiddenForOtherGuest(t *testing.T) {
	ts, r, _ := wsServer(t)
	tokenA := guestSession(t, r, "ws-a")
	tokenB := guestSession(t, r, "ws-b")
	convA := createConversation(t, r, tokenA)

	conn := dialWS(t, ts, tokenB)
	send(t, conn, gin.H{"type": "conversation.subscribe", "payload": gin.H{"conversation_public_id": convA}})
	f := readUntil(t, conn, "error")
	if !strings.Contains(string(f.Payload), "forbidden") {
		t.Fatalf("expected forbidden, got %s", f.Payload)
	}
}

func TestWSSendAckAndBroadcast(t *testing.T) {
	ts, r, _ := wsServer(t)
	token := guestSession(t, r, "ws-send")
	conv := createConversation(t, r, token)

	// second connection subscribes and should receive the broadcast
	watcher := dialWS(t, ts, token)
	send(t, watcher, gin.H{"type": "conversation.subscribe", "payload": gin.H{"conversation_public_id": conv}})
	readUntil(t, watcher, "subscribed")

	sender := dialWS(t, ts, token)
	payload := gin.H{
		"conversation_public_id": conv,
		"client_msg_id":          "ws-c1",
		"msg_type":               "text",
		"content":                gin.H{"text": "hello ws"},
	}
	send(t, sender, gin.H{"type": "message.send", "request_id": "r1", "payload": payload})

	ack := readUntil(t, sender, "message.ack")
	var ackData struct {
		ClientMsgID string `json:"client_msg_id"`
		Seq         int64  `json:"seq"`
		ID          uint64 `json:"id"`
	}
	if err := json.Unmarshal(ack.Payload, &ackData); err != nil {
		t.Fatal(err)
	}
	if ackData.ClientMsgID != "ws-c1" || ackData.Seq == 0 {
		t.Fatalf("bad ack %+v", ackData)
	}

	bc := readUntil(t, watcher, "message.new")
	if !strings.Contains(string(bc.Payload), "hello ws") {
		t.Fatalf("broadcast missing message: %s", bc.Payload)
	}

	// idempotent resend: same client_msg_id → same seq/id, no second broadcast
	send(t, sender, gin.H{"type": "message.send", "request_id": "r2", "payload": payload})
	ack2 := readUntil(t, sender, "message.ack")
	var ack2Data struct {
		Seq int64  `json:"seq"`
		ID  uint64 `json:"id"`
	}
	_ = json.Unmarshal(ack2.Payload, &ack2Data)
	if ack2Data.Seq != ackData.Seq || ack2Data.ID != ackData.ID {
		t.Fatalf("resend not idempotent: first={%d %d} second={%d %d}",
			ackData.Seq, ackData.ID, ack2Data.Seq, ack2Data.ID)
	}
	// watcher must NOT see a duplicate message.new for the replay
	_ = watcher.SetReadDeadline(time.Now().Add(400 * time.Millisecond))
	var extra wsFrame
	for {
		if err := watcher.ReadJSON(&extra); err != nil {
			break // timeout = no duplicate, good
		}
		if extra.Type == "message.new" && strings.Contains(string(extra.Payload), "hello ws") {
			t.Fatal("replay re-broadcast message.new")
		}
	}
}

func TestWSVisitorSendRateLimited(t *testing.T) {
	ts, r, srv := wsServer(t)
	token := guestSession(t, r, "ws-rl")
	conv := createConversation(t, r, token)
	srv.Limits = &Limits{
		Session: ratelimit.New(0.0001, 5),
		Writes:  ratelimit.New(0.0001, 2),
		Uploads: ratelimit.New(0.0001, 5),
	}

	conn := dialWS(t, ts, token)
	for i := 0; i < 3; i++ {
		send(t, conn, gin.H{"type": "message.send", "request_id": "rr", "payload": gin.H{
			"conversation_public_id": conv,
			"client_msg_id":          "rl-" + string(rune('a'+i)),
			"content":                gin.H{"text": "x"},
		}})
	}
	f := readUntil(t, conn, "error")
	if !strings.Contains(string(f.Payload), "rate limited") {
		t.Fatalf("expected rate limited error, got %s", f.Payload)
	}
}
