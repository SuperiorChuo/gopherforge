package api

import (
	"errors"
	"github.com/gin-gonic/gin"
	"github.com/go-admin-kit/services/im/internal/authjwt"
	"github.com/go-admin-kit/services/im/internal/bot"
	"github.com/go-admin-kit/services/im/internal/hub"
	"github.com/go-admin-kit/services/im/internal/kbclient"
	"github.com/go-admin-kit/services/im/internal/sensitive"
	"github.com/go-admin-kit/services/im/internal/storage"
	"github.com/go-admin-kit/services/im/internal/store/im"
	"github.com/go-admin-kit/services/shared/pkg/gatewayauth"
	"github.com/go-admin-kit/services/shared/pkg/notifyclient"
	"gorm.io/gorm"
	"net/http"
	"net/url"
	"strconv"
	"strings"
)

type Server struct {
	Store    *store.Store
	Hub      *hub.Hub
	AgentHub *hub.AgentHub
	Secret   string
	// AgentVerifier checks tokens and IM permissions with the auth owner on
	// mixed guest/agent endpoints, including query-token WebSockets.
	AgentVerifier *authjwt.Verifier
	// Bot 是可选的 AI / stub 客户端（M4）。
	Bot             bot.Client
	BotSystemPrompt string
	AIEnabled       bool
	// KBVector 可选的 ai-service 向量检索（M1.5）；nil = 仅关键词。
	KBVector *kbclient.Client
	// Notify 可选的站内信推送（转人工站内信）；nil = 禁用。
	Notify *notifyclient.Client
	// Storage 持有附件对象（栈内为 MinIO，开发环境为本地）。
	Storage storage.Store
	// UploadDir 是遗留本地目录，作为对象存储前上传文件
	// 的读回退保留。
	UploadDir string
	// Limits 对公开嵌入端点做频率限制；nil 禁用（单元测试）。
	Limits *Limits
	// SensitiveWords 对文本消息中命中的子串打码（M6）。
	// 进程级 env 兜底词，仅打码；控制台词库见 Sensitive。
	SensitiveWords []string
	// Sensitive 是基于数据库的租户词库缓存（控制台管理、
	// 打码/拦截、热生效）。nil = 仅 env 词。
	Sensitive *sensitive.Cache
}

func OK(c *gin.Context, data any) {
	// 与脚手架前端约定：业务成功 code == 200
	c.JSON(http.StatusOK, gin.H{"code": 200, "message": "ok", "data": data})
}

func errorsIsNotFound(err error) bool {
	return err != nil && errors.Is(err, gorm.ErrRecordNotFound)
}

func Fail(c *gin.Context, httpCode int, msg string) {
	c.JSON(httpCode, gin.H{"code": httpCode, "message": msg, "data": nil})
}

func bearer(c *gin.Context) string {
	h := c.GetHeader("Authorization")
	if strings.HasPrefix(strings.ToLower(h), "bearer ") {
		return strings.TrimSpace(h[7:])
	}
	return ""
}

func (s *Server) requireGuest(c *gin.Context) (*authjwt.GuestClaims, bool) {
	tok := bearer(c)
	if tok == "" {
		Fail(c, http.StatusUnauthorized, "missing guest token")
		return nil, false
	}
	claims, err := authjwt.ParseGuest(s.Secret, tok)
	if err != nil {
		Fail(c, http.StatusUnauthorized, "invalid guest token")
		return nil, false
	}
	return claims, true
}

// resolveAgentTenantID 优先取 X-Auth-Tenant-ID（网关），其次 JWT claim，否则默认 1。
func resolveAgentTenantID(c *gin.Context, jwtTenant uint64) uint64 {
	if h := c.GetHeader("X-Auth-Tenant-ID"); h != "" {
		if n, err := strconv.ParseUint(h, 10, 64); err == nil && n > 0 {
			return n
		}
	}
	return authjwt.NormalizeTenantID(jwtTenant)
}

// agentPermitted reports whether the trusted gateway identity carries an IM
// agent/admin permission. These headers are only trustworthy behind the
// gateway's auth-chain hop (auth-sanitize clears client-supplied X-Auth-*).
func agentPermitted(c *gin.Context) bool {
	granted := gatewayauth.ParsePermissions(c.GetHeader("X-Auth-Permissions"))
	for _, code := range []string{"im:agent:access", "im:admin:manage", "*"} {
		if _, ok := granted[code]; ok {
			return true
		}
	}
	return false
}

// verifyAgentToken validates an access token with its owner (auth-service).
// A locally valid signature never grants the agent role by itself: revoked or
// rotated tokens and valid users without IM permissions must not enter agent
// flows. Failures close with the owner's verdict (401/403) or 503.
func (s *Server) verifyAgentToken(c *gin.Context, token string) (*authjwt.AgentClaims, bool) {
	// Cheap local sanity check first: malformed, expired or wrong-secret tokens
	// are rejected without a network call. A locally valid signature is not an
	// authorization — the owner's verdict below is what grants the agent role.
	if _, err := authjwt.ParseAgent(s.Secret, token); err != nil {
		Fail(c, http.StatusUnauthorized, "invalid agent token")
		return nil, false
	}
	if s.AgentVerifier == nil {
		Fail(c, http.StatusServiceUnavailable, "agent verification unavailable")
		return nil, false
	}
	claims, err := s.AgentVerifier.VerifyAgent(c.Request.Context(), token)
	switch {
	case err == nil:
		return claims, true
	case errors.Is(err, authjwt.ErrForbidden):
		Fail(c, http.StatusForbidden, "insufficient permissions")
	case errors.Is(err, authjwt.ErrUnauthorized):
		Fail(c, http.StatusUnauthorized, "invalid agent token")
	default:
		Fail(c, http.StatusServiceUnavailable, "agent verification unavailable")
	}
	return nil, false
}

func (s *Server) requireAgent(c *gin.Context) (*authjwt.AgentClaims, bool) {
	// Bearer tokens are always verified with their owner, even when the
	// gateway already filled identity headers: the shared visitor/agent
	// endpoints are reachable outside auth-chain, where headers can be forged.
	if tok := bearer(c); tok != "" {
		return s.verifyAgentToken(c, tok)
	}
	// Cookie-console sessions arrive without a bearer; only the auth-chain hop
	// may assert their identity headers, and they need the same IM permission.
	if uid := c.GetHeader("X-Auth-User-ID"); uid != "" {
		if !agentPermitted(c) {
			Fail(c, http.StatusForbidden, "insufficient permissions")
			return nil, false
		}
		id, err := strconv.ParseUint(uid, 10, 64)
		if err == nil && id > 0 {
			return &authjwt.AgentClaims{
				UserID:   id,
				Username: c.GetHeader("X-Auth-Username"),
				TenantID: resolveAgentTenantID(c, 0),
			}, true
		}
	}
	Fail(c, http.StatusUnauthorized, "missing agent token")
	return nil, false
}

// sameHostOrigin 报告 origin 是否指向服务本请求的主机
// （demo 页与自托管嵌入）；这些场景无需加入白名单。
func sameHostOrigin(c *gin.Context, origin string) bool {
	u, err := url.Parse(origin)
	if err != nil || u.Host == "" {
		return false
	}
	return strings.EqualFold(u.Host, c.Request.Host)
}

func originDenied(c *gin.Context, allowedJSON, origin string) bool {
	if origin == "" || origin == "null" {
		return false
	}
	if sameHostOrigin(c, origin) {
		return false
	}
	return !store.OriginAllowed(allowedJSON, origin)
}

func parentOrigin(c *gin.Context, bodyOrigin string) string {
	if bodyOrigin != "" {
		return bodyOrigin
	}
	if v := c.Query("parent_origin"); v != "" {
		return v
	}
	if v := c.GetHeader("X-Parent-Origin"); v != "" {
		return v
	}
	return c.GetHeader("Origin")
}

// GET /api/v1/im/health/*（健康探针）
func (s *Server) HealthLive(c *gin.Context) { c.JSON(http.StatusOK, gin.H{"status": "ok"}) }

func (s *Server) HealthReady(c *gin.Context) {
	if s.Store == nil || s.Store.Ping(c.Request.Context()) != nil {
		Fail(c, http.StatusServiceUnavailable, "database unavailable")
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "ready"})
}
