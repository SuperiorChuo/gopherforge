package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/go-admin-kit/services/cc/internal/authjwt"
	"github.com/go-admin-kit/services/cc/internal/hub"
	"github.com/go-admin-kit/services/shared/pkg/wsticket"
	jwtlib "github.com/golang-jwt/jwt/v5"
	"github.com/gorilla/websocket"
)

func TestCCWebSocketTicketSingleUseAndTokenTypeGuard(t *testing.T) {
	srv, r := newTestServer(t, "")
	srv.Secret = "cc-ws-ticket-test-secret-32-chars!"
	srv.Hub = hub.New()
	srv.WSTickets = wsticket.NewStore()
	r.POST("/api/v1/cc/ws/ticket", srv.CreateWebSocketTicket)
	r.GET("/api/v1/cc/ws", srv.WebSocket)

	ts := httptest.NewServer(r)
	defer ts.Close()

	// 1. Mint ticket via authenticated gateway headers.
	req, _ := http.NewRequest(http.MethodPost, ts.URL+"/api/v1/cc/ws/ticket", nil)
	req.Header.Set("X-Auth-User-ID", "11")
	req.Header.Set("X-Auth-Username", "agent11")
	req.Header.Set("X-Auth-Tenant-ID", "2")
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

	// 3. First WebSocket dial with ?ticket= succeeds and receives connected ack.
	wsURL := "ws" + strings.TrimPrefix(ts.URL, "http") + "/api/v1/cc/ws?ticket=" + env.Data.Ticket
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("first WS dial failed: %v", err)
	}
	var ack map[string]any
	if err := conn.ReadJSON(&ack); err != nil || ack["type"] != "connected" {
		t.Fatalf("unexpected ack: %v err=%v", ack, err)
	}
	_ = conn.Close()

	// 4. Replaying the exact same ticket must fail with 401.
	_, replayResp, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err == nil {
		t.Fatal("second WS dial with replayed ticket succeeded; want 401")
	}
	if replayResp == nil || replayResp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("replay status = %v, want 401", replayResp)
	}

	// 5. Refresh token must be rejected by ParseAgent.
	refreshTok, err := jwtlib.NewWithClaims(jwtlib.SigningMethodHS256, authjwt.AgentClaims{
		UserID: 11, TenantID: 2, TokenType: "refresh",
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
