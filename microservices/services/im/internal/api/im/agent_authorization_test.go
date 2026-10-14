package api

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/go-admin-kit/services/im/internal/authjwt"
	"github.com/golang-jwt/jwt/v5"
	"github.com/gorilla/websocket"
)

func signedAgentTestToken(t *testing.T) string {
	t.Helper()
	token, err := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"user_id": 42, "tenant_id": 1, "username": "test-agent",
		"token_type": "access", "exp": time.Now().Add(time.Hour).Unix(),
	}).SignedString([]byte(testSecret))
	if err != nil {
		t.Fatal(err)
	}
	return token
}

func testAgentOwner(t *testing.T, srv *Server, token, permissions string, status int) *atomic.Int32 {
	t.Helper()
	calls := &atomic.Int32{}
	owner := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		if r.Method != http.MethodGet || r.URL.Path != "/internal/verify" || r.URL.RawQuery != "" || r.Header.Get("Authorization") != "Bearer "+token {
			t.Error("owner received unexpected verification request")
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		w.Header().Set("X-Auth-User-ID", "42")
		w.Header().Set("X-Auth-Tenant-ID", "1")
		w.Header().Set("X-Auth-Permissions", permissions)
		w.WriteHeader(status)
	}))
	t.Cleanup(owner.Close)
	srv.AgentVerifier = authjwt.NewVerifier(owner.URL)
	return calls
}

func TestWSAgentRequiresOwnerPermission(t *testing.T) {
	for _, tc := range []struct {
		name, permissions string
		ownerStatus, want int
	}{
		{"no IM permission", "system:user:list", 200, 403},
		{"no permissions", "", 200, 403},
		{"refresh expired or revoked token", "*", 401, 401},
		{"owner unavailable", "*", 503, 503},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ts, _, srv := wsServer(t)
			token := signedAgentTestToken(t)
			calls := testAgentOwner(t, srv, token, tc.permissions, tc.ownerStatus)
			// A conflicting privileged cookie/header must not authorize the query token.
			headers := http.Header{
				"X-Auth-User-Id": {"99"}, "X-Auth-Tenant-Id": {"2"}, "X-Auth-Permissions": {"*"},
			}
			conn, resp, err := websocket.DefaultDialer.Dial("ws"+strings.TrimPrefix(ts.URL, "http")+"/im/ws?token="+token, headers)
			if conn != nil {
				_ = conn.Close()
			}
			if resp != nil {
				defer resp.Body.Close()
			}
			if err == nil || resp == nil || resp.StatusCode != tc.want {
				got := 0
				if resp != nil {
					got = resp.StatusCode
				}
				t.Fatalf("agent handshake status=%d want=%d; invalid/unprivileged agent entered websocket", got, tc.want)
			}
			if calls.Load() != 1 {
				t.Fatalf("owner checks=%d, want 1", calls.Load())
			}
		})
	}
}

func TestMixedIMEndpointsRequireAgentPermission(t *testing.T) {
	r, srv := newTestRouterWithServer(t)
	guest := guestSession(t, r, "agent-authz-guest")
	conv := createConversation(t, r, guest)
	token := signedAgentTestToken(t)
	testAgentOwner(t, srv, token, "system:user:list", http.StatusOK)
	for _, tc := range []struct {
		method, path string
		body         any
	}{
		{http.MethodGet, "/api/v1/im/conversations/" + conv + "/messages", nil},
		{http.MethodPost, "/api/v1/im/conversations/" + conv + "/messages", gin.H{"content": gin.H{"text": "must not write"}}},
		{http.MethodPost, "/api/v1/im/attachments", nil},
	} {
		t.Run(tc.path, func(t *testing.T) {
			w, _ := do(t, r, tc.method, tc.path, tc.body, map[string]string{"Authorization": "Bearer " + token})
			if w.Code != http.StatusForbidden {
				t.Fatalf("agent without IM permission got %d, want 403", w.Code)
			}
		})
	}
	w, _ := do(t, r, http.MethodGet, "/api/v1/im/conversations/"+conv+"/messages", nil,
		map[string]string{"X-Auth-User-ID": "42", "X-Auth-Tenant-ID": "1", "X-Auth-Permissions": "system:user:list"})
	if w.Code != http.StatusForbidden {
		t.Fatalf("trusted identity without IM permission got %d, want 403", w.Code)
	}
}

func TestWSAuthorizedAgentAndGuestFlows(t *testing.T) {
	for _, permission := range []string{"im:agent:access", "im:admin:manage", "*"} {
		t.Run(permission, func(t *testing.T) {
			ts, r, srv := wsServer(t)
			token := signedAgentTestToken(t)
			calls := testAgentOwner(t, srv, token, permission, http.StatusOK)
			guest := guestSession(t, r, "authorized-agent-guest")
			conv := createConversation(t, r, guest)
			visitor := dialWS(t, ts, guest)
			send(t, visitor, gin.H{"type": "conversation.subscribe", "payload": gin.H{"conversation_public_id": conv}})
			readUntil(t, visitor, "subscribed")
			agent := dialWS(t, ts, token)
			if calls.Load() != 1 {
				t.Fatalf("owner checks=%d, want 1; guest must not be checked as access token", calls.Load())
			}
			send(t, agent, gin.H{"type": "conversation.subscribe", "payload": gin.H{"conversation_public_id": conv}})
			readUntil(t, agent, "subscribed")
			send(t, agent, gin.H{"type": "message.send", "request_id": "authorized", "payload": gin.H{
				"conversation_public_id": conv, "client_msg_id": "authorized-1", "content": gin.H{"text": "authorized agent message"},
			}})
			readUntil(t, agent, "message.ack")
			broadcast := readUntil(t, visitor, "message.new")
			if !strings.Contains(string(broadcast.Payload), "authorized agent message") || !strings.Contains(string(broadcast.Payload), `"sender_type":"agent"`) {
				t.Fatalf("unexpected agent broadcast: %s", broadcast.Payload)
			}
			stored, err := srv.Store.GetConversationByPublicID(conv)
			if err != nil || stored.AgentUserID == nil || *stored.AgentUserID != 42 {
				t.Fatalf("authorized agent assignment missing: conversation=%+v error=%v", stored, err)
			}
		})
	}
}
