package api

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/go-admin-kit/services/im/internal/authjwt"
	"github.com/go-admin-kit/services/im/internal/ratelimit"
)

// Limits throttles the publicly reachable embed endpoints. Buckets are
// in-process (IM is single-replica; see ratelimit package note).
type Limits struct {
	// Session guards guest-session minting, keyed by client IP (pre-auth).
	Session *ratelimit.Limiter
	// Writes guards conversation/message writes, keyed by visitor/agent.
	Writes *ratelimit.Limiter
	// Uploads guards attachment uploads, keyed by visitor/agent.
	Uploads *ratelimit.Limiter
}

func DefaultLimits() *Limits {
	return &Limits{
		Session: ratelimit.New(0.2, 5),  // 12/min per IP
		Writes:  ratelimit.New(1, 5),    // typing pace per sender
		Uploads: ratelimit.New(0.02, 5), // ~72/h per sender
	}
}

func tooMany(c *gin.Context) {
	Fail(c, http.StatusTooManyRequests, "too many requests, slow down")
	c.Abort()
}

// senderKey identifies the caller: visitor id > agent id > client IP.
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

// allowVisitorWS is the WS-path counterpart of limitWrites for guest senders.
func (s *Server) allowVisitorWS(visitorID uint64) bool {
	return s.Limits == nil || s.Limits.Writes.Allow("v:"+itoa64(visitorID))
}
