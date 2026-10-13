package api

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/go-admin-kit/services/im/internal/authjwt"
	"github.com/go-admin-kit/services/im/internal/ratelimit"
)

// Limits 对可公开访问的嵌入端点进行限流。令牌桶为
// 进程内实现（IM 为单副本；参见 ratelimit 包说明）。
type Limits struct {
	// Session 保护访客会话创建，按客户端 IP 限流（认证前）。
	Session *ratelimit.Limiter
	// Writes 保护会话/消息写入，按访客/坐席限流。
	Writes *ratelimit.Limiter
	// Uploads 保护附件上传，按访客/坐席限流。
	Uploads *ratelimit.Limiter
}

func DefaultLimits() *Limits {
	return &Limits{
		Session: ratelimit.New(0.2, 5),  // 每 IP 12 次/分
		Writes:  ratelimit.New(1, 5),    // 每个发送者的输入节奏
		Uploads: ratelimit.New(0.02, 5), // 每个发送者约 72 次/时
	}
}

func tooMany(c *gin.Context) {
	Fail(c, http.StatusTooManyRequests, "too many requests, slow down")
	c.Abort()
}

// senderKey 标识调用者：访客 id > 坐席 id > 客户端 IP。
func (s *Server) senderKey(c *gin.Context) string {
	if tok := bearer(c); tok != "" {
		if g, err := authjwt.ParseGuest(s.Secret, tok); err == nil {
			return "v:" + itoa64(g.VisitorID)
		}
		if a, err := authjwt.ParseAgent(s.Secret, tok); err == nil {
			return "a:" + itoa64(a.UserID)
		}
	}
	if uid := c.GetHeader("X-Auth-User-ID"); uid != "" {
		return "a:" + uid
	}
	return "ip:" + c.ClientIP()
}

func itoa64(v uint64) string {
	if v == 0 {
		return "0"
	}
	var buf [20]byte
	i := len(buf)
	for v > 0 {
		i--
		buf[i] = byte('0' + v%10)
		v /= 10
	}
	return string(buf[i:])
}

func (s *Server) limitSession() gin.HandlerFunc {
	return func(c *gin.Context) {
		if s.Limits == nil || s.Limits.Session.Allow("ip:"+c.ClientIP()) {
			return
		}
		tooMany(c)
	}
}

func (s *Server) limitWrites() gin.HandlerFunc {
	return func(c *gin.Context) {
		if s.Limits == nil || s.Limits.Writes.Allow(s.senderKey(c)) {
			return
		}
		tooMany(c)
	}
}

func (s *Server) limitUploads() gin.HandlerFunc {
	return func(c *gin.Context) {
		if s.Limits == nil || s.Limits.Uploads.Allow(s.senderKey(c)) {
			return
		}
		tooMany(c)
	}
}

// allowVisitorWS 是 WS 路径下针对访客发送者、对应 limitWrites 的实现。
func (s *Server) allowVisitorWS(visitorID uint64) bool {
	return s.Limits == nil || s.Limits.Writes.Allow("v:"+itoa64(visitorID))
}
