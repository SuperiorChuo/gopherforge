package api

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/go-admin-kit/services/cc/internal/authjwt"
	"github.com/gorilla/websocket"
)

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool { return true },
}

const (
	wsWriteWait  = 10 * time.Second
	wsPongWait   = 60 * time.Second
	wsPingPeriod = (wsPongWait * 9) / 10
)

// WebSocket handles GET /api/v1/cc/ws — 坐席浏览器实时话务事件通道（来电弹屏底座）。
//
// 鉴权照搬 im-service 的 /im/ws 做法：浏览器 WS 握手带不了 Authorization 头，故
// 用 ?token= 传 access token，本端 authjwt.ParseAgent 校验。该端点在网关侧走
// cc-open 路由（不挂 ForwardAuth），与 webhook/cdr/cid 同列——升级请求不经
// ForwardAuth，token 由本 handler 自校验。gateway 头（X-Auth-User-ID）也兜底放行，
// 便于内网直连/测试。升级端点不走 {code,message,data} envelope。
func (s *Server) WebSocket(c *gin.Context) {
	var userID, tenantID uint64
	if uid := gatewayUserID(c); uid > 0 {
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
