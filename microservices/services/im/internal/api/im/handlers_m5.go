package api

// M5 处理器：访客 CSAT 评分、坐席侧访客名片/备注、快捷回复。

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/go-admin-kit/services/im/internal/model"
	"github.com/go-admin-kit/services/im/internal/store/im"
	"github.com/go-admin-kit/services/shared/pkg/notifyclient"
)

// ---------- 访客评分（CSAT）----------

type rateReq struct {
	Score   int16  `json:"score"`
	Comment string `json:"comment"`
}

// VisitorRateConversation 处理 POST /api/v1/im/conversations/:public_id/rating。
// 仅限已关闭会话，每个会话只能评一次。
func (s *Server) VisitorRateConversation(c *gin.Context) {
	guest, ok := s.requireGuest(c)
	if !ok {
		return
	}
	conv, err := s.Store.GetConversationByPublicID(c.Param("public_id"))
	if err != nil {
		Fail(c, http.StatusNotFound, "conversation not found")
		return
	}
	if conv.VisitorID != guest.VisitorID {
		Fail(c, http.StatusForbidden, "not your conversation")
		return
	}
	var req rateReq
	if err := c.ShouldBindJSON(&req); err != nil {
		Fail(c, http.StatusBadRequest, "invalid body")
		return
	}
	if len(req.Comment) > 500 {
		req.Comment = req.Comment[:500]
	}
	if err := s.Store.RateConversation(conv, req.Score, req.Comment); err != nil {
		Fail(c, http.StatusBadRequest, err.Error())
		return
	}
	_ = s.Store.AppendSystemEvent(conv.ID, "rated", map[string]any{"score": req.Score})
	if refreshed, err := s.Store.GetConversationByPublicID(conv.PublicID.String()); err == nil {
		conv = refreshed
	}
	s.AgentHub.Publish(conv.TenantID, gin.H{"type": "conversation.updated", "payload": conv})
	s.Hub.Publish(conv.PublicID.String(), gin.H{"type": "conversation.updated", "payload": conv})
	// 低分即时推站内信（≤3 星 → 接待坐席）
	s.pushLowRating(conv, req.Score, req.Comment)
	OK(c, gin.H{"rated": true, "score": req.Score})
}

// pushLowRating 访客 CSAT ≤3 时通知接待坐席；失败只打日志。
func (s *Server) pushLowRating(conv *model.Conversation, score int16, comment string) {
	if s.Notify == nil || !s.Notify.Enabled() || conv == nil {
		return
	}
	if score <= 0 || score > 3 {
		return
	}
	if conv.AgentUserID == nil || *conv.AgentUserID == 0 {
		return
	}
	uid := *conv.AgentUserID
	publicID := conv.PublicID.String()
	tenantID := conv.TenantID
	snippet := strings.TrimSpace(comment)
	if snippet == "" {
		snippet = "无文字评价"
	}
	if len([]rune(snippet)) > 80 {
		snippet = string([]rune(snippet)[:80]) + "…"
	}
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
		defer cancel()
		_, err := s.Notify.Send(ctx, notifyclient.SendInput{
			TenantID:     tenantID,
			UserID:       uid,
			TemplateCode: "im.low_rating",
			Type:         "im",
			RefType:      "im_rating",
			RefID:        publicID,
			Vars: map[string]string{
				"score":     strconv.Itoa(int(score)),
				"public_id": publicID[:8],
				"snippet":   snippet,
			},
		})
		if err != nil {
			log.Printf("im low-rating notify conv=%s: %v", publicID, err)
		}
	}()
}

// ---------- 坐席访客名片 ----------

// parseConversationContext 将 conversation.context 的 JSON 解码为供坐席使用的 map。
func parseConversationContext(raw string) map[string]any {
	raw = strings.TrimSpace(raw)
	if raw == "" || raw == "null" {
		return map[string]any{}
	}
	var m map[string]any
	if err := json.Unmarshal([]byte(raw), &m); err != nil || m == nil {
		return map[string]any{}
	}
	return m
}

// AgentGetVisitor 处理 GET /api/v1/im/agent/conversations/:public_id/visitor。
func (s *Server) AgentGetVisitor(c *gin.Context) {
	agent, ok := s.requireAgent(c)
	if !ok {
		return
	}
	conv, ok := s.requireAgentConversation(c, agent)
	if !ok {
		return
	}
	visitor, err := s.Store.GetVisitor(conv.VisitorID)
	if err != nil {
		Fail(c, http.StatusNotFound, "visitor not found")
		return
	}
	count, _ := s.Store.CountVisitorConversations(visitor.ID)
	siteName := ""
	if site, err := s.Store.GetSite(conv.SiteID); err == nil {
		siteName = site.Name
	}
	OK(c, gin.H{
		"visitor":            visitor,
		"conversation_count": count,
		"context":            parseConversationContext(conv.Context),
		"site_name":          siteName,
		"channel":            conv.Channel,
	})
}

type visitorNoteReq struct {
	Note string `json:"note"`
}

// AgentSetVisitorNote 处理 PUT /api/v1/im/agent/conversations/:public_id/visitor-note。
func (s *Server) AgentSetVisitorNote(c *gin.Context) {
	agent, ok := s.requireAgent(c)
	if !ok {
		return
	}
	conv, ok := s.requireAgentConversation(c, agent)
	if !ok {
		return
	}
	var req visitorNoteReq
	if err := c.ShouldBindJSON(&req); err != nil {
		Fail(c, http.StatusBadRequest, "invalid body")
		return
	}
	if len(req.Note) > 2000 {
		req.Note = req.Note[:2000]
	}
	if err := s.Store.UpdateVisitorNote(conv.VisitorID, req.Note); err != nil {
		Fail(c, http.StatusInternalServerError, err.Error())
		return
	}
	OK(c, gin.H{"saved": true})
}

type visitorContactReq struct {
	DisplayName *string `json:"display_name"`
	Phone       *string `json:"phone"`
	Email       *string `json:"email"`
	Wechat      *string `json:"wechat"`
	// ClearPhone 为 true 时强制将电话置空。
	ClearPhone  bool `json:"clear_phone"`
	ClearEmail  bool `json:"clear_email"`
	ClearWechat bool `json:"clear_wechat"`
}

// applyVisitorContactReq 校验并应用访客的联系方式字段。
func (s *Server) applyVisitorContactReq(visitorID uint64, req visitorContactReq) (*model.Visitor, string) {
	patch := store.VisitorProfilePatch{}
	hasAny := false

	if req.DisplayName != nil {
		dn := strings.TrimSpace(*req.DisplayName)
		if dn != "" {
			patch.DisplayName = &dn
			hasAny = true
		}
	}
	if req.Phone != nil {
		phone := normalizePhone(*req.Phone)
		if phone == "" && strings.TrimSpace(*req.Phone) != "" {
			return nil, "电话格式不正确"
		}
		if phone != "" {
			patch.Phone = &phone
			hasAny = true
		}
	}
	if req.Email != nil {
		email := normalizeEmail(*req.Email)
		if email == "" && strings.TrimSpace(*req.Email) != "" {
			return nil, "邮箱格式不正确"
		}
		if email != "" {
			patch.Email = &email
			hasAny = true
		}
	}
	if req.Wechat != nil {
		wx := normalizeWechat(*req.Wechat)
		if wx == "" && strings.TrimSpace(*req.Wechat) != "" {
			return nil, "微信号格式不正确"
		}
		if wx != "" {
			patch.Wechat = &wx
			hasAny = true
		}
	}

	if req.ClearPhone {
		_ = s.Store.ClearVisitorField(visitorID, "phone")
		hasAny = true
	}
	if req.ClearEmail {
		_ = s.Store.ClearVisitorField(visitorID, "email")
		hasAny = true
	}
	if req.ClearWechat {
		_ = s.Store.ClearVisitorField(visitorID, "wechat")
		hasAny = true
	}

	if !hasAny {
		return nil, "请填写联系方式"
	}
	if patch.DisplayName != nil || patch.Phone != nil || patch.Email != nil || patch.Wechat != nil {
		if err := s.Store.UpdateVisitorProfilePatch(visitorID, patch); err != nil {
			return nil, err.Error()
		}
	}
	v, err := s.Store.GetVisitor(visitorID)
	if err != nil {
		return nil, err.Error()
	}
	return v, ""
}

// VisitorUpdateProfile 处理 PUT /api/v1/im/visitor/profile —— 访客留下联系方式。
func (s *Server) VisitorUpdateProfile(c *gin.Context) {
	guest, ok := s.requireGuest(c)
	if !ok {
		return
	}
	var req visitorContactReq
	if err := c.ShouldBindJSON(&req); err != nil {
		Fail(c, http.StatusBadRequest, "invalid body")
		return
	}
	v, errMsg := s.applyVisitorContactReq(guest.VisitorID, req)
	if errMsg != "" {
		if strings.Contains(errMsg, "格式") || strings.Contains(errMsg, "请填写") {
			Fail(c, http.StatusBadRequest, errMsg)
			return
		}
		Fail(c, http.StatusInternalServerError, errMsg)
		return
	}
	OK(c, gin.H{"visitor": v})
}

// AgentSetVisitorContact 处理 PUT /api/v1/im/agent/conversations/:public_id/visitor-contact。
func (s *Server) AgentSetVisitorContact(c *gin.Context) {
	agent, ok := s.requireAgent(c)
	if !ok {
		return
	}
	conv, ok := s.requireAgentConversation(c, agent)
	if !ok {
		return
	}
	var req visitorContactReq
	if err := c.ShouldBindJSON(&req); err != nil {
		Fail(c, http.StatusBadRequest, "invalid body")
		return
	}
	v, errMsg := s.applyVisitorContactReq(conv.VisitorID, req)
	if errMsg != "" {
		if strings.Contains(errMsg, "格式") || strings.Contains(errMsg, "请填写") {
			Fail(c, http.StatusBadRequest, errMsg)
			return
		}
		Fail(c, http.StatusInternalServerError, errMsg)
		return
	}
	OK(c, gin.H{"visitor": v})
}

// normalizePhone 只保留数字与常见分隔符，最长 32 字符。
func normalizePhone(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	var b strings.Builder
	for _, r := range raw {
		switch {
		case r >= '0' && r <= '9':
			b.WriteRune(r)
		case r == '+' || r == '-' || r == ' ' || r == '(' || r == ')':
			if r == '+' && b.Len() == 0 {
				b.WriteRune(r)
			}
		default:
		}
	}
	out := b.String()
	digits := strings.TrimPrefix(out, "+")
	n := len(digits)
	if n < 7 || n > 15 {
		return ""
	}
	for _, r := range digits {
		if r < '0' || r > '9' {
			return ""
		}
	}
	if len(out) > 32 {
		out = out[:32]
	}
	return out
}

func normalizeEmail(raw string) string {
	raw = strings.TrimSpace(strings.ToLower(raw))
	if raw == "" {
		return ""
	}
	if len(raw) > 128 {
		return ""
	}
	// 简单的类 RFC5322 校验
	at := strings.LastIndex(raw, "@")
	if at < 1 || at >= len(raw)-3 {
		return ""
	}
	local, domain := raw[:at], raw[at+1:]
	if strings.Contains(local, " ") || strings.Contains(domain, " ") {
		return ""
	}
	if !strings.Contains(domain, ".") {
		return ""
	}
	for _, r := range raw {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') ||
			r == '@' || r == '.' || r == '_' || r == '%' || r == '+' || r == '-' {
			continue
		}
		return ""
	}
	return raw
}

func normalizeWechat(raw string) string {
	raw = strings.TrimSpace(raw)
	raw = strings.ReplaceAll(raw, " ", "")
	if raw == "" {
		return ""
	}
	// 手机号当微信号
	if p := normalizePhone(raw); p != "" {
		return p
	}
	// 6-20 个字符，字母开头，字母数字 _ -
	if len(raw) < 6 || len(raw) > 20 {
		return ""
	}
	if raw[0] < 'A' || (raw[0] > 'Z' && raw[0] < 'a') || raw[0] > 'z' {
		// 也允许首字符是 unicode 字母？这里按 ASCII 处理
		r0 := rune(raw[0])
		if !((r0 >= 'a' && r0 <= 'z') || (r0 >= 'A' && r0 <= 'Z')) {
			return ""
		}
	}
	for _, r := range raw {
		ok := (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') ||
			(r >= '0' && r <= '9') || r == '_' || r == '-'
		if !ok {
			return ""
		}
	}
	return raw
}

// ---------- 快捷回复 ----------

type quickReplyReq struct {
	Title   *string `json:"title"`
	Content *string `json:"content"`
	Sort    *int    `json:"sort"`
}

// AgentListQuickReplies 处理 GET /api/v1/im/agent/quick-replies。
func (s *Server) AgentListQuickReplies(c *gin.Context) {
	agent, ok := s.requireAgent(c)
	if !ok {
		return
	}
	list, err := s.Store.ListQuickReplies(agent.TenantID)
	if err != nil {
		Fail(c, http.StatusInternalServerError, err.Error())
		return
	}
	OK(c, gin.H{"list": list})
}

// AgentCreateQuickReply 处理 POST /api/v1/im/agent/quick-replies。
func (s *Server) AgentCreateQuickReply(c *gin.Context) {
	agent, ok := s.requireAgent(c)
	if !ok {
		return
	}
	var req quickReplyReq
	if err := c.ShouldBindJSON(&req); err != nil || req.Title == nil || req.Content == nil {
		Fail(c, http.StatusBadRequest, "title and content are required")
		return
	}
	qr, err := s.Store.CreateQuickReply(c.Request.Context(), agent.TenantID, agent.UserID, *req.Title, *req.Content)
	if err != nil {
		Fail(c, http.StatusBadRequest, err.Error())
		return
	}
	OK(c, qr)
}

// AgentUpdateQuickReply 处理 PUT /api/v1/im/agent/quick-replies/:id。
func (s *Server) AgentUpdateQuickReply(c *gin.Context) {
	agent, ok := s.requireAgent(c)
	if !ok {
		return
	}
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil || id == 0 {
		Fail(c, http.StatusBadRequest, "invalid id")
		return
	}
	var req quickReplyReq
	if err := c.ShouldBindJSON(&req); err != nil {
		Fail(c, http.StatusBadRequest, "invalid body")
		return
	}
	qr, err := s.Store.UpdateQuickReply(c.Request.Context(), id, agent.TenantID, req.Title, req.Content, req.Sort)
	if err != nil {
		if errorsIsNotFound(err) {
			Fail(c, http.StatusNotFound, "quick reply not found")
			return
		}
		Fail(c, http.StatusBadRequest, err.Error())
		return
	}
	OK(c, qr)
}

// AgentDeleteQuickReply 处理 DELETE /api/v1/im/agent/quick-replies/:id。
func (s *Server) AgentDeleteQuickReply(c *gin.Context) {
	agent, ok := s.requireAgent(c)
	if !ok {
		return
	}
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil || id == 0 {
		Fail(c, http.StatusBadRequest, "invalid id")
		return
	}
	if err := s.Store.DeleteQuickReply(c.Request.Context(), id, agent.TenantID); err != nil {
		if errorsIsNotFound(err) {
			Fail(c, http.StatusNotFound, "quick reply not found")
			return
		}
		Fail(c, http.StatusInternalServerError, err.Error())
		return
	}
	OK(c, gin.H{"deleted": true})
}
