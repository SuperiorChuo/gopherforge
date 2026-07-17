package api

import (
	"bytes"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-admin-kit/services/im/internal/ratelimit"
)

func TestRateLimits(t *testing.T) {
	r, srv := newTestRouterWithServer(t)
	// negligible refill so only burst matters in-test
	srv.Limits = &Limits{
		Session: ratelimit.New(0.0001, 2),
		Writes:  ratelimit.New(0.0001, 3),
		Uploads: ratelimit.New(0.0001, 1),
	}

	sessionBody := map[string]any{"app_key": "demo", "parent_origin": seededOrigin}

	t.Run("session per IP", func(t *testing.T) {
		for i := 0; i < 2; i++ {
			if w, _ := do(t, r, http.MethodPost, "/api/v1/im/visitor/session", sessionBody, nil); w.Code != http.StatusOK {
				t.Fatalf("session %d: %d", i+1, w.Code)
			}
		}
		if w, _ := do(t, r, http.MethodPost, "/api/v1/im/visitor/session", sessionBody, nil); w.Code != http.StatusTooManyRequests {
			t.Fatalf("3rd session: want 429, got %d", w.Code)
		}
	})

	t.Run("writes per visitor", func(t *testing.T) {
		srv.Limits.Session = ratelimit.New(0.0001, 5) // room for this subtest's session
		token := guestSession(t, r, "rl-guest")
		conv := createConversation(t, r, token) // write #1
		auth := map[string]string{"Authorization": "Bearer " + token}
		body := map[string]any{"content": map[string]any{"text": "hi"}}
		path := "/api/v1/im/conversations/" + conv + "/messages"
		for i := 0; i < 2; i++ { // writes #2 #3
			if w, _ := do(t, r, http.MethodPost, path, body, auth); w.Code != http.StatusOK {
				t.Fatalf("message %d: %d", i+1, w.Code)
			}
		}
		if w, _ := do(t, r, http.MethodPost, path, body, auth); w.Code != http.StatusTooManyRequests {
			t.Fatalf("4th write: want 429, got %d", w.Code)
		}
	})

	t.Run("uploads per sender", func(t *testing.T) {
		srv.Limits.Session = ratelimit.New(0.0001, 5)
		token := guestSession(t, r, "rl-upload-guest")
		upload := func() int {
			var buf bytes.Buffer
			mw := multipart.NewWriter(&buf)
			fw, err := mw.CreateFormFile("file", "a.txt")
			if err != nil {
				t.Fatal(err)
			}
			_, _ = fw.Write([]byte("hello"))
			_ = mw.Close()
			req := httptest.NewRequest(http.MethodPost, "http://"+testHost+"/api/v1/im/attachments", &buf)
			req.Host = testHost
			req.Header.Set("Content-Type", mw.FormDataContentType())
			req.Header.Set("Authorization", "Bearer "+token)
			w := httptest.NewRecorder()
			r.ServeHTTP(w, req)
			return w.Code
		}
		if code := upload(); code != http.StatusOK {
			t.Fatalf("1st upload: %d", code)
		}
		if code := upload(); code != http.StatusTooManyRequests {
			t.Fatalf("2nd upload: want 429, got %d", code)
		}
	})
}

func TestReadCursorsAndUnread(t *testing.T) {
	r := newTestRouter(t)
	token := guestSession(t, r, "read-guest")
	conv := createConversation(t, r, token)
	auth := map[string]string{"Authorization": "Bearer " + token}
	agentAuth := map[string]string{"X-Auth-User-ID": "9", "X-Auth-Username": "agent9"}
	msgPath := "/api/v1/im/conversations/" + conv + "/messages"

	send := func(i byte) {
		body := map[string]any{"client_msg_id": "rm-" + string('0'+i), "content": map[string]any{"text": "msg"}}
		if w, _ := do(t, r, http.MethodPost, msgPath, body, auth); w.Code != http.StatusOK {
			t.Fatalf("send %d: %d", i, w.Code)
		}
	}
	send(1)
	send(2)

	listUnread := func() int64 {
		w, env := do(t, r, http.MethodGet, "/api/v1/im/agent/conversations?scope=all", nil, agentAuth)
		if w.Code != http.StatusOK {
			t.Fatalf("agent list: %d %s", w.Code, w.Body.String())
		}
		var data struct {
			List []struct {
				PublicID    string `json:"public_id"`
				UnreadCount int64  `json:"unread_count"`
			} `json:"list"`
		}
		decodeInto(t, env.Data, &data)
		for _, cv := range data.List {
			if cv.PublicID == conv {
				return cv.UnreadCount
			}
		}
		t.Fatalf("conversation %s not in agent list", conv)
		return -1
	}

	if n := listUnread(); n != 2 {
		t.Fatalf("agent unread = %d, want 2", n)
	}

	// agent marks read up to latest
	if w, _ := do(t, r, http.MethodPost, "/api/v1/im/agent/conversations/"+conv+"/read",
		map[string]any{}, agentAuth); w.Code != http.StatusOK {
		t.Fatalf("agent mark read failed")
	}
	if n := listUnread(); n != 0 {
		t.Fatalf("agent unread after read = %d, want 0", n)
	}

	// conversation payload carries cursors; visitor auto-read own sends
	w, env := do(t, r, http.MethodGet, msgPath, nil, auth)
	if w.Code != http.StatusOK {
		t.Fatal("list messages")
	}
	var data struct {
		Conversation struct {
			AgentLastReadSeq   int64 `json:"agent_last_read_seq"`
			VisitorLastReadSeq int64 `json:"visitor_last_read_seq"`
		} `json:"conversation"`
		Messages []struct {
			Seq int64 `json:"seq"`
		} `json:"messages"`
	}
	decodeInto(t, env.Data, &data)
	last := data.Messages[len(data.Messages)-1].Seq
	if data.Conversation.AgentLastReadSeq != last {
		t.Fatalf("agent cursor %d, want %d", data.Conversation.AgentLastReadSeq, last)
	}
	if data.Conversation.VisitorLastReadSeq != last {
		t.Fatalf("visitor cursor %d (auto-read on send), want %d", data.Conversation.VisitorLastReadSeq, last)
	}

	// monotonic: lower seq must not regress the cursor
	if w, _ := do(t, r, http.MethodPost, "/api/v1/im/conversations/"+conv+"/read",
		map[string]any{"seq": 1}, auth); w.Code != http.StatusOK {
		t.Fatal("visitor mark read")
	}
	w, env = do(t, r, http.MethodGet, msgPath, nil, auth)
	decodeInto(t, env.Data, &data)
	if data.Conversation.VisitorLastReadSeq != last {
		t.Fatalf("cursor regressed to %d", data.Conversation.VisitorLastReadSeq)
	}

	// 越权: another guest cannot mark read
	other := guestSession(t, r, "read-intruder")
	if w, _ := do(t, r, http.MethodPost, "/api/v1/im/conversations/"+conv+"/read",
		map[string]any{}, map[string]string{"Authorization": "Bearer " + other}); w.Code != http.StatusForbidden {
		t.Fatalf("intruder mark read: want 403, got %d", w.Code)
	}
}
