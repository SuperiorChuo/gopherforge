package api

import (
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/go-admin-kit/services/cc/internal/authjwt"
	"github.com/go-admin-kit/services/shared/pkg/wsticket"
	"github.com/gorilla/websocket"
)

const ccWSTicketScope = "cc_ws"

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool { return true },
}

const (
	wsWriteWait  = 10 * time.Second
	wsPongWait   = 60 * time.Second
	wsPingPeriod = (wsPongWait * 9) / 10
)

// CreateWebSocketTicket handles POST /api/v1/cc/ws/ticket — 签发 60 秒一次性
// 握手票据，使浏览器不再将长期 Access JWT 暴露在 /api/v1/cc/ws?token= 日志中。
func (s *Server) CreateWebSocketTicket(c *gin.Context) {
	agent, authorized := s.requireAgent(c)
	if !authorized {
		return
	}
	ticket, err := wsticket.Mint(s.Secret, ccWSTicketScope, agent.UserID, agent.TenantID, agent.Username, wsticket.DefaultTTL)
	if err != nil {
		fail(c, http.StatusInternalServerError, "failed to create websocket ticket")
		return
	}
	ok(c, gin.H{"ticket": ticket})
}

// WebSocket handles GET /api/v1/cc/ws — 坐席浏览器实时话务事件通道（来电弹屏底座）。
//
// 优先消费 POST /api/v1/cc/ws/ticket 签发的 60 秒一次性票据（?ticket=）；
// 保留网关头（X-Auth-User-ID）与 legacy access token（?token= / Bearer）兜底。
func (s *Server) WebSocket(c *gin.Context) {
	var userID, tenantID uint64
	if ticket := strings.TrimSpace(c.Query("ticket")); ticket != "" {
		claims, err := s.wsTicketStore().Consume(s.Secret, ccWSTicketScope, ticket)
		if err != nil {
			fail(c, http.StatusUnauthorized, "invalid websocket ticket")
			return
		}
		userID = claims.UserID
		tenantID = claims.TenantID
	} else if uid := gatewayUserID(c); uid > 0 {
		userID = uid
		tenantID = tenantFromHeader(c)
	} else {
		token := c.Query("token")
		if token == "" {
			token = bearer(c)
		}
		if token == "" {
			fail(c, http.StatusUnauthorized, "missing token")
			return
		}
		claims, err := authjwt.ParseAgent(s.Secret, token)
		if err != nil {
			fail(c, http.StatusUnauthorized, "invalid token")
			return
		}
		userID = claims.UserID
		tenantID = claims.TenantID
	}

	conn, err := upgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		return
	}
	defer conn.Close()

	send, unregister := s.Hub.Register(userID, tenantID)
	defer unregister()

	// 读协程：处理 ping/关闭并驱动 pong 续期。客户端断开由 ReadMessage 出错感知。
	conn.SetReadDeadline(time.Now().Add(wsPongWait))
	conn.SetPongHandler(func(string) error {
		conn.SetReadDeadline(time.Now().Add(wsPongWait))
		return nil
	})
	readErr := make(chan struct{})
	go func() {
		defer close(readErr)
		for {
			if _, _, err := conn.ReadMessage(); err != nil {
				return
			}
		}
	}()

	// 建连回执，让前端知道通道已就绪。
	_ = conn.WriteJSON(gin.H{"type": "connected"})

	ping := time.NewTicker(wsPingPeriod)
	defer ping.Stop()
	for {
		select {
		case b, ok := <-send:
			if !ok {
				return // 被 hub 踢除（慢消费者）
			}
			conn.SetWriteDeadline(time.Now().Add(wsWriteWait))
			if err := conn.WriteMessage(websocket.TextMessage, b); err != nil {
				return
			}
		case <-ping.C:
			conn.SetWriteDeadline(time.Now().Add(wsWriteWait))
			if err := conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				return
			}
		case <-readErr:
			return
		}
	}
}

// gatewayUserID 复用 requireAgent 的头解析逻辑，但不写响应（升级端点无 envelope）。
func gatewayUserID(c *gin.Context) uint64 {
	return uintFrom(c.GetHeader("X-Auth-User-ID"))
}
