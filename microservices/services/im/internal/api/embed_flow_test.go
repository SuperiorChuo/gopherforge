package api

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/go-admin-kit/services/im/internal/authjwt"
	"github.com/go-admin-kit/services/im/internal/bot"
	"github.com/go-admin-kit/services/im/internal/hub"
	"github.com/go-admin-kit/services/im/internal/store"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

const (
	testSecret = "test-secret-at-least-32-characters!!"
	testHost   = "im.test"
	// seeded into the demo site allow-list by store.seed()
	seededOrigin = "http://localhost:3000"
	evilOrigin   = "https://evil.example.com"
)

var testDBSeq atomic.Int64

// newTestRouter runs the real route table against an in-memory DB, so tests
// exercise exactly what main.go serves. AIEnabled stays false → conversations
// queue instead of bot_serving and no bot goroutines race the test.
func newTestRouter(t *testing.T) *gin.Engine {
	r, _ := newTestRouterWithServer(t)
	return r
}

func newTestRouterWithServer(t *testing.T) (*gin.Engine, *Server) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	dsn := fmt.Sprintf("file:imtest%d?mode=memory&cache=shared", testDBSeq.Add(1))
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	// one connection keeps the shared in-memory DB alive for the whole test
	sqlDB.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = sqlDB.Close() })

	st, err := store.NewWithDB(db)
	if err != nil {
		t.Fatalf("store: %v", err)
	}
	srv := &Server{
		Store:     st,
		Hub:       hub.New(),
		AgentHub:  hub.NewAgentHub(),
		Secret:    testSecret,
		UploadDir: t.TempDir(),
	}
	r := gin.New()
	srv.RegisterRoutes(r)
	return r, srv
}

type envelope struct {
	Code    int             `json:"code"`
	Message string          `json:"message"`
	Data    json.RawMessage `json:"data"`
}

func do(t *testing.T, r *gin.Engine, method, path string, body any, headers map[string]string) (*httptest.ResponseRecorder, envelope) {
	t.Helper()
	var buf bytes.Buffer
	if body != nil {
		if err := json.NewEncoder(&buf).Encode(body); err != nil {
			t.Fatal(err)
		}
	}
	req := httptest.NewRequest(method, "http://"+testHost+path, &buf)
	req.Host = testHost
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	var env envelope
	_ = json.Unmarshal(w.Body.Bytes(), &env)
	return w, env
}

func decodeInto(t *testing.T, raw json.RawMessage, v any) {
	t.Helper()
	if err := json.Unmarshal(raw, v); err != nil {
		t.Fatalf("decode data: %v (raw=%s)", err, raw)
	}
}

// guestSession runs POST /visitor/session from an allowed origin and returns
// the guest token.
func guestSession(t *testing.T, r *gin.Engine, guestKey string) string {
	t.Helper()
	w, env := do(t, r, http.MethodPost, "/api/v1/im/visitor/session", map[string]any{
		"app_key":       "demo",
		"guest_key":     guestKey,
		"parent_origin": seededOrigin,
	}, nil)
	if w.Code != http.StatusOK {
		t.Fatalf("visitor session: status %d body %s", w.Code, w.Body.String())
	}
	var data struct {
		GuestToken string `json:"guest_token"`
		VisitorID  uint64 `json:"visitor_id"`
	}
	decodeInto(t, env.Data, &data)
	if data.GuestToken == "" || data.VisitorID == 0 {
		t.Fatalf("empty session payload: %s", env.Data)
	}
	return data.GuestToken
}

func createConversation(t *testing.T, r *gin.Engine, token string) string {
	t.Helper()
	w, env := do(t, r, http.MethodPost, "/api/v1/im/conversations", map[string]any{},
		map[string]string{"Authorization": "Bearer " + token})
	if w.Code != http.StatusOK {
		t.Fatalf("create conversation: status %d body %s", w.Code, w.Body.String())
	}
	var conv struct {
		PublicID string `json:"public_id"`
	}
	decodeInto(t, env.Data, &conv)
	if conv.PublicID == "" {
		t.Fatalf("no public_id: %s", env.Data)
	}
	return conv.PublicID
}

func TestWidgetConfigOriginPolicy(t *testing.T) {
	r := newTestRouter(t)

	t.Run("allowed origin", func(t *testing.T) {
		w, env := do(t, r, http.MethodGet, "/api/v1/im/widget/config?app_key=demo&parent_origin="+seededOrigin, nil, nil)
		if w.Code != http.StatusOK {
			t.Fatalf("status %d body %s", w.Code, w.Body.String())
		}
		var data struct {
			AppKey  string `json:"app_key"`
			Snippet string `json:"snippet"`
		}
		decodeInto(t, env.Data, &data)
		if data.AppKey != "demo" {
			t.Fatalf("app_key %q", data.AppKey)
		}
		if want := "http://" + testHost + "/im/widget/widget.js"; !bytes.Contains([]byte(data.Snippet), []byte(want)) {
			t.Fatalf("snippet %q missing %q", data.Snippet, want)
		}
	})

	t.Run("forged origin denied", func(t *testing.T) {
		w, _ := do(t, r, http.MethodGet, "/api/v1/im/widget/config?app_key=demo&parent_origin="+evilOrigin, nil, nil)
		if w.Code != http.StatusForbidden {
			t.Fatalf("want 403, got %d body %s", w.Code, w.Body.String())
		}
	})

	t.Run("same-host origin bypasses whitelist", func(t *testing.T) {
		// demo page served from the IM host itself is never whitelisted explicitly
		w, _ := do(t, r, http.MethodGet, "/api/v1/im/widget/config?app_key=demo&parent_origin=http://"+testHost, nil, nil)
		if w.Code != http.StatusOK {
			t.Fatalf("want 200, got %d body %s", w.Code, w.Body.String())
		}
	})

	t.Run("unknown app_key", func(t *testing.T) {
		w, _ := do(t, r, http.MethodGet, "/api/v1/im/widget/config?app_key=nope&parent_origin="+seededOrigin, nil, nil)
		if w.Code != http.StatusNotFound {
			t.Fatalf("want 404, got %d", w.Code)
		}
	})
}

func TestVisitorSessionOriginPolicy(t *testing.T) {
	r := newTestRouter(t)

	t.Run("allowed origin mints parseable guest token", func(t *testing.T) {
		tok := guestSession(t, r, "guest-a")
		claims, err := authjwt.ParseGuest(testSecret, tok)
		if err != nil {
			t.Fatalf("minted token does not parse: %v", err)
		}
		if claims.GuestKey != "guest-a" {
			t.Fatalf("guest key %q", claims.GuestKey)
		}
	})

	t.Run("forged origin denied", func(t *testing.T) {
		w, _ := do(t, r, http.MethodPost, "/api/v1/im/visitor/session", map[string]any{
			"app_key":       "demo",
			"parent_origin": evilOrigin,
		}, nil)
		if w.Code != http.StatusForbidden {
			t.Fatalf("want 403, got %d body %s", w.Code, w.Body.String())
		}
	})

	t.Run("null origin from file page allowed", func(t *testing.T) {
		w, _ := do(t, r, http.MethodPost, "/api/v1/im/visitor/session", map[string]any{
			"app_key": "demo",
		}, map[string]string{"Origin": "null"})
		if w.Code != http.StatusOK {
			t.Fatalf("want 200, got %d body %s", w.Code, w.Body.String())
		}
	})

	t.Run("forged Origin header without parent_origin denied", func(t *testing.T) {
		w, _ := do(t, r, http.MethodPost, "/api/v1/im/visitor/session", map[string]any{
			"app_key": "demo",
		}, map[string]string{"Origin": evilOrigin})
		if w.Code != http.StatusForbidden {
			t.Fatalf("want 403, got %d body %s", w.Code, w.Body.String())
		}
	})
}

func TestGuestConversationIsolation(t *testing.T) {
	r := newTestRouter(t)
	tokenA := guestSession(t, r, "guest-a")
	tokenB := guestSession(t, r, "guest-b")
	convA := createConversation(t, r, tokenA)

	msgPath := "/api/v1/im/conversations/" + convA + "/messages"
	msgBody := map[string]any{"msg_type": "text", "content": map[string]any{"text": "hello"}}

	t.Run("owner reads own conversation", func(t *testing.T) {
		w, _ := do(t, r, http.MethodGet, msgPath, nil, map[string]string{"Authorization": "Bearer " + tokenA})
		if w.Code != http.StatusOK {
			t.Fatalf("want 200, got %d body %s", w.Code, w.Body.String())
		}
	})

	t.Run("other guest cannot read", func(t *testing.T) {
		w, _ := do(t, r, http.MethodGet, msgPath, nil, map[string]string{"Authorization": "Bearer " + tokenB})
		if w.Code != http.StatusForbidden {
			t.Fatalf("want 403, got %d body %s", w.Code, w.Body.String())
		}
	})

	t.Run("other guest cannot send", func(t *testing.T) {
		w, _ := do(t, r, http.MethodPost, msgPath, msgBody, map[string]string{"Authorization": "Bearer " + tokenB})
		if w.Code != http.StatusForbidden {
			t.Fatalf("want 403, got %d body %s", w.Code, w.Body.String())
		}
	})

	t.Run("other guest cannot transfer to human", func(t *testing.T) {
		w, _ := do(t, r, http.MethodPost, "/api/v1/im/conversations/"+convA+"/transfer_human",
			map[string]any{}, map[string]string{"Authorization": "Bearer " + tokenB})
		if w.Code != http.StatusForbidden {
			t.Fatalf("want 403, got %d body %s", w.Code, w.Body.String())
		}
	})

	t.Run("owner sends", func(t *testing.T) {
		w, _ := do(t, r, http.MethodPost, msgPath, msgBody, map[string]string{"Authorization": "Bearer " + tokenA})
		if w.Code != http.StatusOK {
			t.Fatalf("want 200, got %d body %s", w.Code, w.Body.String())
		}
	})

	t.Run("no token rejected", func(t *testing.T) {
		w, _ := do(t, r, http.MethodPost, "/api/v1/im/conversations", map[string]any{}, nil)
		if w.Code != http.StatusUnauthorized {
			t.Fatalf("want 401, got %d body %s", w.Code, w.Body.String())
		}
	})

	t.Run("expired token rejected", func(t *testing.T) {
		expired, err := authjwt.MintGuest(testSecret, 1, 1, "guest-a", -time.Minute)
		if err != nil {
			t.Fatal(err)
		}
		w, _ := do(t, r, http.MethodGet, msgPath, nil, map[string]string{"Authorization": "Bearer " + expired})
		// guest parse fails → falls through to agent auth, which also fails
		if w.Code != http.StatusUnauthorized {
			t.Fatalf("want 401, got %d body %s", w.Code, w.Body.String())
		}
	})
}

func TestClientMsgIDIdempotency(t *testing.T) {
	r := newTestRouter(t)
	token := guestSession(t, r, "guest-a")
	conv := createConversation(t, r, token)

	msgPath := "/api/v1/im/conversations/" + conv + "/messages"
	body := map[string]any{
		"client_msg_id": "retry-1",
		"msg_type":      "text",
		"content":       map[string]any{"text": "hello"},
	}
	auth := map[string]string{"Authorization": "Bearer " + token}

	type msg struct {
		ID  uint64 `json:"id"`
		Seq int64  `json:"seq"`
	}
	var first, second msg

	w, env := do(t, r, http.MethodPost, msgPath, body, auth)
	if w.Code != http.StatusOK {
		t.Fatalf("first send: %d %s", w.Code, w.Body.String())
	}
	decodeInto(t, env.Data, &first)

	// widget 网络重试：同 client_msg_id 重发必须返回首次结果
	w, env = do(t, r, http.MethodPost, msgPath, body, auth)
	if w.Code != http.StatusOK {
		t.Fatalf("retry send: %d %s", w.Code, w.Body.String())
	}
	decodeInto(t, env.Data, &second)

	if first.ID != second.ID || first.Seq != second.Seq {
		t.Fatalf("retry not idempotent: first={id:%d seq:%d} second={id:%d seq:%d}",
			first.ID, first.Seq, second.ID, second.Seq)
	}

	w, env = do(t, r, http.MethodGet, msgPath, nil, auth)
	if w.Code != http.StatusOK {
		t.Fatalf("list: %d", w.Code)
	}
	var listData struct {
		Messages []struct {
			MsgType string `json:"msg_type"`
		} `json:"messages"`
	}
	decodeInto(t, env.Data, &listData)
	texts := 0
	for _, m := range listData.Messages {
		if m.MsgType == "text" {
			texts++
		}
	}
	if texts != 1 {
		t.Fatalf("want exactly 1 text message after retry, got %d", texts)
	}

	// 不同 client_msg_id 正常追加
	body["client_msg_id"] = "retry-2"
	w, env = do(t, r, http.MethodPost, msgPath, body, auth)
	if w.Code != http.StatusOK {
		t.Fatalf("second message: %d", w.Code)
	}
	var third msg
	decodeInto(t, env.Data, &third)
	if third.Seq <= first.Seq {
		t.Fatalf("new message seq %d should advance past %d", third.Seq, first.Seq)
	}
}

// countMessages returns per-sender_type text-message counts in a conversation.
func countMessages(t *testing.T, r *gin.Engine, conv, token string) map[string]int {
	t.Helper()
	w, env := do(t, r, http.MethodGet, "/api/v1/im/conversations/"+conv+"/messages", nil,
		map[string]string{"Authorization": "Bearer " + token})
	if w.Code != http.StatusOK {
		t.Fatalf("list: %d %s", w.Code, w.Body.String())
	}
	var data struct {
		Messages []struct {
			SenderType string `json:"sender_type"`
			MsgType    string `json:"msg_type"`
		} `json:"messages"`
	}
	decodeInto(t, env.Data, &data)
	counts := map[string]int{}
	for _, m := range data.Messages {
		if m.MsgType == "text" {
			counts[m.SenderType]++
		}
	}
	return counts
}

// Regression: a client_msg_id retry must not trigger a second bot reply
// (caught by e2e against real PG before the Replayed guard existed).
func TestRetryDoesNotDoubleBotReply(t *testing.T) {
	r, srv := newTestRouterWithServer(t)
	srv.AIEnabled = true
	srv.Bot = &bot.Stub{}

	token := guestSession(t, r, "guest-bot")
	conv := createConversation(t, r, token)

	msgPath := "/api/v1/im/conversations/" + conv + "/messages"
	body := map[string]any{
		"client_msg_id": "bot-retry-1",
		"msg_type":      "text",
		"content":       map[string]any{"text": "你们几点上班？"},
	}
	auth := map[string]string{"Authorization": "Bearer " + token}

	if w, _ := do(t, r, http.MethodPost, msgPath, body, auth); w.Code != http.StatusOK {
		t.Fatalf("first send: %d", w.Code)
	}
	// bot replies async; poll until it lands
	deadline := time.Now().Add(3 * time.Second)
	for countMessages(t, r, conv, token)["bot"] == 0 {
		if time.Now().After(deadline) {
			t.Fatal("bot never replied to first send")
		}
		time.Sleep(20 * time.Millisecond)
	}

	// retry the same client_msg_id, give a would-be second reply time to land
	if w, _ := do(t, r, http.MethodPost, msgPath, body, auth); w.Code != http.StatusOK {
		t.Fatalf("retry send: %d", w.Code)
	}
	time.Sleep(300 * time.Millisecond)

	counts := countMessages(t, r, conv, token)
	if counts["visitor"] != 1 {
		t.Fatalf("visitor messages = %d, want 1", counts["visitor"])
	}
	if counts["bot"] != 1 {
		t.Fatalf("bot replied %d times after retry, want 1", counts["bot"])
	}
}
