package api

import (
	"github.com/gin-gonic/gin"
	"github.com/go-admin-kit/services/im/internal/authjwt"
	"github.com/go-admin-kit/services/im/internal/model"
	"github.com/go-admin-kit/services/im/internal/store/im"
	"github.com/go-admin-kit/services/shared/pkg/iploc"
	"github.com/google/uuid"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// GET /api/v1/im/widget/config?app_key=demo（获取站点嵌入配置）
func (s *Server) WidgetConfig(c *gin.Context) {
	appKey := c.Query("app_key")
	if appKey == "" {
		appKey = "demo"
	}
	site, err := s.Store.GetSiteByAppKey(appKey)
	if err != nil {
		Fail(c, http.StatusNotFound, "site not found")
		return
	}
	// iframe 场景 Origin 是 IM 域；用 parent_origin 校验客户站域名
	origin := parentOrigin(c, "")
	if originDenied(c, site.AllowedOrigins, origin) {
		Fail(c, http.StatusForbidden, "origin denied")
		return
	}
	OK(c, gin.H{
		"app_key":      site.AppKey,
		"name":         site.Name,
		"welcome_text": site.WelcomeText,
		"bot_enabled":  site.BotEnabled,
		"snippet":      embedSnippet(c, site.AppKey),
	})
}

func embedSnippet(c *gin.Context, appKey string) string {
	// 在反向代理后优先使用公网网关主机
	host := c.Request.Host
	scheme := "http"
	if c.Request.TLS != nil || c.GetHeader("X-Forwarded-Proto") == "https" {
		scheme = "https"
	}
	base := scheme + "://" + host
	return `<script src="` + base + `/im/widget/widget.js" data-app-key="` + appKey + `" async></script>`
}

type visitorSessionReq struct {
	AppKey       string `json:"app_key"`
	GuestKey     string `json:"guest_key"`
	DisplayName  string `json:"display_name"`
	ParentOrigin string `json:"parent_origin"`
}

// POST /api/v1/im/visitor/session（创建访客会话）
func (s *Server) VisitorSession(c *gin.Context) {
	var req visitorSessionReq
	if err := c.ShouldBindJSON(&req); err != nil {
		Fail(c, http.StatusBadRequest, "invalid body")
		return
	}
	if req.AppKey == "" {
		req.AppKey = "demo"
	}
	if req.GuestKey == "" {
		req.GuestKey = uuid.NewString()
	}
	site, err := s.Store.GetSiteByAppKey(req.AppKey)
	if err != nil {
		Fail(c, http.StatusNotFound, "site not found")
		return
	}
	origin := parentOrigin(c, req.ParentOrigin)
	if originDenied(c, site.AllowedOrigins, origin) {
		Fail(c, http.StatusForbidden, "origin denied")
		return
	}
	v, err := s.Store.UpsertVisitor(site.ID, req.GuestKey, req.DisplayName)
	if err != nil {
		Fail(c, http.StatusInternalServerError, err.Error())
		return
	}
	token, err := authjwt.MintGuest(s.Secret, v.ID, site.ID, v.GuestKey, 24*time.Hour)
	if err != nil {
		Fail(c, http.StatusInternalServerError, err.Error())
		return
	}
	OK(c, gin.H{
		"guest_token":  token,
		"guest_key":    v.GuestKey,
		"visitor_id":   v.ID,
		"display_name": v.DisplayName,
		"phone":        v.Phone,
		"welcome_text": site.WelcomeText,
	})
}

type createConvReq struct {
	Channel        string         `json:"channel"`
	Context        map[string]any `json:"context"`
	SkillGroupID   *uint64        `json:"skill_group_id"`
	SkillGroupCode string         `json:"skill_group_code"`
}

// enrichVisitorContext 将客户端页面/referrer 上下文与服务端
// IP / UA / 父级 origin 合并，让坐席能看到真实来源与地址。
func enrichVisitorContext(c *gin.Context, ctx map[string]any) map[string]any {
	if ctx == nil {
		ctx = map[string]any{}
	}
	// 服务端权威网络字段（覆盖客户端伪造的 ip）。
	if ip := strings.TrimSpace(c.ClientIP()); ip != "" {
		ctx["client_ip"] = ip
		ctx["ip_location"] = visitorIPLocation(ip)
	}
	if ua := strings.TrimSpace(c.GetHeader("User-Agent")); ua != "" {
		if len(ua) > 512 {
			ua = ua[:512]
		}
		ctx["user_agent"] = ua
	}
	if po := strings.TrimSpace(c.GetHeader("X-Parent-Origin")); po != "" {
		ctx["parent_origin"] = po
	} else if po := strings.TrimSpace(c.Query("parent_origin")); po != "" {
		ctx["parent_origin"] = po
	}
	// 归一化旧客户端的常见别名。
	if _, ok := ctx["page_url"]; !ok {
		if p, ok := ctx["page"].(string); ok && p != "" {
			ctx["page_url"] = p
		}
	}
	return ctx
}

// visitorIPLocation 解析访客地址标签：本机/内网保持粗粒度标签；公网 IP 优先
// 走 ip2region 离线库拿「省 市 ISP」精确归属地，xdb 未部署或查不到时回退「公网」。
func visitorIPLocation(ip string) string {
	coarse := classifyIPLocation(ip)
	if coarse != "公网" {
		return coarse
	}
	if loc := iploc.Lookup(ip); loc != "" {
		return loc
	}
	return coarse
}

// classifyIPLocation 不借助外部地理 API，返回粗粒度标签。
// 公网 IP 保留简单「公网」标签；私网/回环标记为内网。
func classifyIPLocation(ip string) string {
	host := ip
	if i := strings.Index(ip, "%"); i >= 0 {
		host = ip[:i]
	}
	// 去掉端口（若存在，IPv4:port）
	if strings.Count(host, ":") == 1 {
		if h, _, err := net.SplitHostPort(host); err == nil {
			host = h
		}
	}
	parsed := net.ParseIP(host)
	if parsed == nil {
		return ""
	}
	if parsed.IsLoopback() {
		return "本机"
	}
	if parsed.IsPrivate() || parsed.IsLinkLocalUnicast() || parsed.IsLinkLocalMulticast() {
		return "内网"
	}
	return "公网"
}

// POST /api/v1/im/conversations（创建/获取会话）
func (s *Server) CreateConversation(c *gin.Context) {
	guest, ok := s.requireGuest(c)
	if !ok {
		return
	}
	if s.visitorBanned(guest.VisitorID) {
		Fail(c, http.StatusForbidden, "您已被限制发起会话")
		return
	}
	var req createConvReq
	_ = c.ShouldBindJSON(&req)
	req.Context = enrichVisitorContext(c, req.Context)
	ctxJSON := store.JSONText(req.Context)

	site, siteErr := s.Store.GetSite(guest.SiteID)
	var skillID *uint64
	botOn := false
	tenantID := uint64(1)
	if siteErr == nil {
		tenantID = authjwt.NormalizeTenantID(site.TenantID)
		botOn = site.BotEnabled && s.AIEnabled
		if req.SkillGroupID != nil && *req.SkillGroupID > 0 {
			// 只接受站点所属租户内的技能组
			if sg, err := s.Store.GetSkillGroupForTenant(*req.SkillGroupID, tenantID); err == nil {
				skillID = &sg.ID
			}
		} else if req.SkillGroupCode != "" {
			if sg, err := s.Store.GetSkillGroupByCode(tenantID, req.SkillGroupCode); err == nil {
				skillID = &sg.ID
			}
		} else if site.DefaultSkillGroupID != nil {
			skillID = site.DefaultSkillGroupID
		}
	}

	// conversation.tenant_id 继承自 site.tenant_id
	conv, err := s.Store.EnsureOpenConversation(tenantID, guest.SiteID, guest.VisitorID, req.Channel, ctxJSON, skillID, botOn)
	if err != nil {
		Fail(c, http.StatusInternalServerError, err.Error())
		return
	}
	// 在可能的自动分配/系统事件后刷新
	if refreshed, err := s.Store.GetConversationByPublicID(conv.PublicID.String()); err == nil {
		conv = refreshed
	}
	// 非工作时间/离线留言提示：新会话（还没有任何消息）时最多落一条系统提示，
	// 营业时间外优先（更具体），否则退回"无在线坐席"的兜底判断
	if conv.Status == "queued" && conv.LastMessagePreview == "" {
		noticeText := ""
		if siteErr == nil {
			if bh := model.ParseBusinessHours(site.BusinessHours); !bh.WithinAt(time.Now()) {
				noticeText = bh.OffNotice()
			}
		}
		if noticeText == "" {
			if n, err := s.Store.OnlineAgentCount(); err == nil && n == 0 {
				noticeText = "当前坐席均不在线，您可以留言，我们上线后会第一时间回复。"
			}
		}
		if noticeText != "" {
			_ = s.Store.AppendSystemEvent(conv.ID, "offline_notice", map[string]any{"text": noticeText})
			if refreshed, err := s.Store.GetConversationByPublicID(conv.PublicID.String()); err == nil {
				conv = refreshed
			}
		}
	}
	if conv.Status == "queued" {
		conv.QueuePosition = s.Store.QueuePosition(conv)
	}
	s.AgentHub.Publish(conv.TenantID, gin.H{"type": "conversation.updated", "payload": conv})
	s.AgentHub.Publish(conv.TenantID, gin.H{"type": "queue.updated", "payload": gin.H{"skill_group_id": conv.SkillGroupID}})
	OK(c, conv)
}

// GET /api/v1/im/conversations/:public_id/messages（拉取会话消息）
func (s *Server) ListMessages(c *gin.Context) {
	publicID := c.Param("public_id")
	conv, err := s.Store.GetConversationByPublicID(publicID)
	if err != nil {
		Fail(c, http.StatusNotFound, "conversation not found")
		return
	}
	if !s.canAccessConversation(c, conv) {
		return
	}
	after, _ := strconv.ParseInt(c.DefaultQuery("after_seq", "0"), 10, 64)
	list, err := s.Store.ListMessages(conv.ID, after, 100)
	if err != nil {
		Fail(c, http.StatusInternalServerError, err.Error())
		return
	}
	if conv.Status == "queued" {
		conv.QueuePosition = s.Store.QueuePosition(conv)
	}
	OK(c, gin.H{"messages": list, "conversation": conv})
}

type sendMsgReq struct {
	ClientMsgID string         `json:"client_msg_id"`
	MsgType     string         `json:"msg_type"`
	Content     map[string]any `json:"content"`
}

// POST /api/v1/im/conversations/:public_id/messages（访客或坐席发送消息）
func (s *Server) SendMessage(c *gin.Context) {
	publicID := c.Param("public_id")
	conv, err := s.Store.GetConversationByPublicID(publicID)
	if err != nil {
		Fail(c, http.StatusNotFound, "conversation not found")
		return
	}
	var req sendMsgReq
	if err := c.ShouldBindJSON(&req); err != nil || req.Content == nil {
		Fail(c, http.StatusBadRequest, "invalid body")
		return
	}
	if req.MsgType == "" {
		req.MsgType = "text"
	}

	var senderType string
	var senderID uint64

	// 先尝试访客身份
	if tok := bearer(c); tok != "" {
		if g, err := authjwt.ParseGuest(s.Secret, tok); err == nil {
			if g.VisitorID != conv.VisitorID {
				Fail(c, http.StatusForbidden, "not your conversation")
				return
			}
			if s.visitorBanned(g.VisitorID) {
				Fail(c, http.StatusForbidden, "您已被限制发言")
				return
			}
			senderType = "visitor"
			senderID = g.VisitorID
		}
	}
	if senderType == "" {
		agent, ok := s.requireAgent(c)
		if !ok {
			return
		}
		if conv.TenantID != 0 && conv.TenantID != agent.TenantID {
			Fail(c, http.StatusForbidden, "conversation not in your tenant")
			return
		}
		senderType = "agent"
		senderID = agent.UserID
		if conv.Status == "queued" || conv.AgentUserID == nil {
			_ = s.Store.AssignConversation(conv, agent.UserID)
			_ = s.Store.AppendSystemEvent(conv.ID, "assigned", map[string]any{
				"agent_user_id": agent.UserID,
				"auto":          false,
			})
		}
	}

	// 敏感词：仅文本消息，落库前处理（历史/回放均干净）。
	// block 词命中 → 拒发；mask 词命中 → 打码。
	if req.MsgType == "text" {
		if t, ok := req.Content["text"].(string); ok {
			masked, blocked := s.applySensitive(conv.TenantID, t)
			if blocked {
				Fail(c, http.StatusBadRequest, sensitiveBlockedMsg)
				return
			}
			req.Content["text"] = masked
		}
	}
	content := store.JSONText(req.Content)
	var clientID *string
	if req.ClientMsgID != "" {
		clientID = &req.ClientMsgID
	}
	msg := &model.Message{
		ConversationID: conv.ID,
		ClientMsgID:    clientID,
		SenderType:     senderType,
		SenderID:       &senderID,
		MsgType:        req.MsgType,
		Content:        content,
	}
	if err := s.Store.CreateMessage(msg); err != nil {
		Fail(c, http.StatusInternalServerError, err.Error())
		return
	}
	// 幂等重放：直接返回首次落库的消息，不重播事件、不再触发 bot
	if msg.Replayed {
		OK(c, msg)
		return
	}
	// 发消息隐含已读之前的所有消息
	if _, err := s.Store.MarkRead(conv, senderType, msg.Seq); err == nil {
		s.publishRead(conv, senderType, msg.Seq)
	}
	// 刷新会话预览
	convTenant := conv.TenantID
	conv, _ = s.Store.GetConversationByPublicID(publicID)
	payload := gin.H{
		"type": "message.new",
		"payload": gin.H{
			"message":                msg,
			"conversation_public_id": publicID,
		},
	}
	s.Hub.Publish(publicID, payload)
	s.AgentHub.Publish(convTenant, gin.H{"type": "conversation.updated", "payload": conv})

	// M4：访客消息后触发 bot 回复 / 转人工意图
	if senderType == "visitor" && conv != nil {
		text := ""
		if t, ok := req.Content["text"].(string); ok {
			text = t
		}
		s.afterVisitorMessage(conv, text)
	}
	OK(c, msg)
}
