package api

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/go-admin-kit/services/im/internal/authjwt"
	jwtlib "github.com/golang-jwt/jwt/v5"
	"github.com/gorilla/websocket"
)

func TestIMWebSocketTicketSingleUseAndTokenTypeGuard(t *testing.T) {
	ts, _, srv := wsServer(t)

	// 1. Mint one-time ticket through /api/v1/im/agent/ws/ticket.
	req, _ := http.NewRequest(http.MethodPost, ts.URL+"/api/v1/im/agent/ws/ticket", nil)
	req.Header.Set("X-Auth-User-ID", "9")
	req.Header.Set("X-Auth-Username", "agent9")
	req.Header.Set("X-Auth-Tenant-ID", "1")
	req.Header.Set("X-Auth-Permissions", "im:agent:access")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("POST ticket: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("POST ticket status = %d, want 200", resp.StatusCode)
	}
	var env struct {
		Data struct {
			Ticket string `json:"ticket"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&env); err != nil {
		t.Fatalf("decode ticket response: %v", err)
	}
	if env.Data.Ticket == "" {
		t.Fatal("expected non-empty ticket")
	}

	// 2. Ticket must NOT be accepted as a REST Bearer access token.
	if _, err := authjwt.ParseAgent(srv.Secret, env.Data.Ticket); err == nil {
		t.Fatal("ParseAgent accepted a ws_ticket token; want rejection")
	}

	// 3. First WS dial with ?ticket= succeeds.
	wsURL := "ws" + strings.TrimPrefix(ts.URL, "http") + "/im/ws?ticket=" + env.Data.Ticket
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("first IM WS dial failed: %v", err)
	}
	_ = conn.Close()

	// 4. Replaying the same ticket fails with 401.
	_, replayResp, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err == nil {
		t.Fatal("second IM WS dial with replayed ticket succeeded; want 401")
	}
	if replayResp == nil || replayResp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("replay status = %v, want 401", replayResp)
	}

	// 5. Refresh token must be rejected by ParseAgent.
	refreshTok, err := jwtlib.NewWithClaims(jwtlib.SigningMethodHS256, authjwt.AgentClaims{
		UserID: 9, TenantID: 1, TokenType: "refresh",
		RegisteredClaims: jwtlib.RegisteredClaims{
			ExpiresAt: jwtlib.NewNumericDate(time.Now().Add(time.Hour)),
		},
	}).SignedString([]byte(srv.Secret))
	if err != nil {
		t.Fatalf("sign refresh token: %v", err)
	}
	if _, err := authjwt.ParseAgent(srv.Secret, refreshTok); err == nil {
		t.Fatal("ParseAgent accepted a refresh token; want rejection")
	}
}
