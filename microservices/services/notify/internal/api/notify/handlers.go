package api

import (
	"context"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/go-admin-kit/services/notify/internal/channel"
	"github.com/go-admin-kit/services/notify/internal/store/notify"
)

func (s *Server) ListMessages(c *gin.Context) {
	u, okU := s.requireUser(c)
	if !okU {
		return
	}
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	ps, _ := strconv.Atoi(c.DefaultQuery("page_size", "20"))
	unread := c.Query("unread") == "1" || strings.EqualFold(c.Query("unread"), "true")
	list, total, err := s.Store.ListMessages(u.TenantID, u.UserID, store.ListFilter{
		UnreadOnly: unread,
		Type:       strings.TrimSpace(c.Query("type")),
		Page:       page,
		PageSize:   ps,
	})
	if err != nil {
		fail(c, http.StatusInternalServerError, err.Error())
		return
	}
	ok(c, gin.H{"list": list, "total": total})
}

func (s *Server) UnreadCount(c *gin.Context) {
	u, okU := s.requireUser(c)
	if !okU {
		return
	}
	n, err := s.Store.UnreadCount(u.TenantID, u.UserID)
	if err != nil {
		fail(c, http.StatusInternalServerError, err.Error())
		return
	}
	ok(c, gin.H{"count": n})
}

func (s *Server) MarkRead(c *gin.Context) {
	u, okU := s.requireUser(c)
	if !okU {
		return
	}
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil || id == 0 {
		fail(c, http.StatusBadRequest, "invalid id")
		return
	}
	if err := s.Store.MarkRead(u.TenantID, u.UserID, id); err != nil {
		fail(c, http.StatusInternalServerError, err.Error())
		return
	}
	ok(c, gin.H{"ok": true})
}

func (s *Server) MarkAllRead(c *gin.Context) {
	u, okU := s.requireUser(c)
	if !okU {
		return
	}
	n, err := s.Store.MarkAllRead(u.TenantID, u.UserID)
	if err != nil {
		fail(c, http.StatusInternalServerError, err.Error())
		return
	}
	ok(c, gin.H{"updated": n})
}

// ChannelStatus GET /api/v1/notify/channels — inbox always on; email/sms depend on env.
func (s *Server) ChannelStatus(c *gin.Context) {
	if _, okU := s.requireUser(c); !okU {
		return
	}
	ok(c, s.channelStatus())
}

// ListTemplates GET /api/v1/notify/templates
func (s *Server) ListTemplates(c *gin.Context) {
	u, okU := s.requireUser(c)
	if !okU {
		return
	}
	list, err := s.Store.ListTemplates(u.TenantID)
	if err != nil {
		fail(c, http.StatusInternalServerError, err.Error())
		return
	}
	ok(c, gin.H{"list": list, "channels": s.channelStatus()})
}

// GetTemplate GET /api/v1/notify/templates/:id
func (s *Server) GetTemplate(c *gin.Context) {
	u, okU := s.requireUser(c)
	if !okU {
		return
	}
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil || id == 0 {
		fail(c, http.StatusBadRequest, "invalid id")
		return
	}
	t, err := s.Store.GetTemplate(id, u.TenantID)
	if err != nil {
		fail(c, http.StatusNotFound, "template not found")
		return
	}
	ok(c, t)
}

type updateTemplateReq struct {
	TitleTpl *string `json:"title_tpl"`
	BodyTpl  *string `json:"body_tpl"`
	LinkTpl  *string `json:"link_tpl"`
	Channels *string `json:"channels"`
	Enabled  *bool   `json:"enabled"`
}

// UpdateTemplate PUT /api/v1/notify/templates/:id
func (s *Server) UpdateTemplate(c *gin.Context) {
	u, okU := s.requireUser(c)
	if !okU {
		return
	}
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil || id == 0 {
		fail(c, http.StatusBadRequest, "invalid id")
		return
	}
	var req updateTemplateReq
	if err := c.ShouldBindJSON(&req); err != nil {
		fail(c, http.StatusBadRequest, "invalid body")
		return
	}
	t, err := s.Store.UpdateTemplate(c.Request.Context(), id, u.TenantID, store.TemplateUpdate{
		TitleTpl: req.TitleTpl,
		BodyTpl:  req.BodyTpl,
		LinkTpl:  req.LinkTpl,
		Channels: req.Channels,
		Enabled:  req.Enabled,
	})
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			fail(c, http.StatusNotFound, err.Error())
			return
		}
		fail(c, http.StatusBadRequest, err.Error())
		return
	}
	ok(c, t)
}

type previewReq struct {
	Vars map[string]string `json:"vars"`
}

// PreviewTemplate POST /api/v1/notify/templates/:id/preview
func (s *Server) PreviewTemplate(c *gin.Context) {
	u, okU := s.requireUser(c)
	if !okU {
		return
	}
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil || id == 0 {
		fail(c, http.StatusBadRequest, "invalid id")
		return
	}
	var req previewReq
	_ = c.ShouldBindJSON(&req)
	tpl, err := s.Store.GetTemplate(id, u.TenantID)
	if err != nil {
		fail(c, http.StatusNotFound, "template not found")
		return
	}
	vars := req.Vars
	if len(vars) == 0 {
		vars = defaultSampleVars(tpl.Code)
	}
	title, body, link, _, err := s.Store.PreviewTemplate(id, u.TenantID, vars)
	if err != nil {
		fail(c, http.StatusNotFound, "template not found")
		return
	}
	ok(c, gin.H{
		"code":     tpl.Code,
		"title":    title,
		"content":  body,
		"link":     link,
		"channels": channel.ParseChannels(tpl.Channels),
	})
}

// TestSendTemplate POST /api/v1/notify/templates/:id/test-send
// Sends a rendered message to the current user (for ops smoke).
func (s *Server) TestSendTemplate(c *gin.Context) {
	u, okU := s.requireUser(c)
	if !okU {
		return
	}
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil || id == 0 {
		fail(c, http.StatusBadRequest, "invalid id")
		return
	}
	var req previewReq
	_ = c.ShouldBindJSON(&req)
	tpl, err := s.Store.GetTemplate(id, u.TenantID)
	if err != nil {
		fail(c, http.StatusNotFound, "template not found")
		return
	}
	if !tpl.Enabled {
		fail(c, http.StatusBadRequest, "template disabled")
		return
	}
	vars := req.Vars
	if vars == nil {
		vars = defaultSampleVars(tpl.Code)
	}
	// unique ref so tests don't get 24h-deduped forever
	refID := "test-" + strconv.FormatInt(time.Now().UnixNano(), 10)
	res, err := s.Store.Send(c.Request.Context(), store.SendInput{
		TenantID:     u.TenantID,
		UserID:       u.UserID,
		TemplateCode: tpl.Code,
		Type:         typeForCode(tpl.Code),
		RefType:      "template_test",
		RefID:        refID,
		Vars:         vars,
		Channels:     channel.ParseChannels(tpl.Channels),
	})
	if err != nil {
		fail(c, http.StatusBadRequest, err.Error())
		return
	}
	// external fan-out
	var delivered []channel.Result
	if s.Dispatcher != nil && res != nil {
		ctx, cancel := context.WithTimeout(c.Request.Context(), 15*time.Second)
		defer cancel()
		contact := s.Store.GetUserContact(c.Request.Context(), u.TenantID, u.UserID)
		payload := channel.Payload{Title: res.Title, Content: res.Content, Link: res.Link}
		var msgID uint64
		if res.Message != nil {
			msgID = res.Message.ID
		}
		// 运维手动点测：仍同步投递，控制台要当场看到 sent/failed 明细。
		delivered = deliverSyncLogs(ctx, s.Dispatcher, s.Store,
			u.TenantID, u.UserID, msgID, tpl.Code, "template_test", refID,
			res.Channels, contact, payload)
	}
	ok(c, gin.H{
		"message":   res.Message,
		"title":     res.Title,
		"content":   res.Content,
		"link":      res.Link,
		"channels":  res.Channels,
		"delivered": delivered,
	})
}

func typeForCode(code string) string {
	switch {
	case strings.HasPrefix(code, "ticket."):
		return "ticket"
	case strings.HasPrefix(code, "im."):
		return "im"
	case strings.HasPrefix(code, "crm."):
		return "notice"
	default:
		return "notice"
	}
}

func defaultSampleVars(code string) map[string]string {
	// enough for seeded templates
	base := map[string]string{
		"ticket_id":   "1",
		"ticket_no":   "TK-DEMO",
		"title":       "演示工单",
		"snippet":     "这是一条测试内容",
		"public_id":   "demo-uuid",
		"reason":      "test",
		"score":       "2",
		"hours_left":  "3",
		"target_name": "演示客户",
		"target_type": "customer",
		"target_id":   "1",
		"link":        "/crm/customers",
		"name":        "演示客户",
		"customer_id": "1",
	}
	_ = code
	return base
}

// ListMyDeliveryLogs GET /api/v1/notify/deliveries — own external delivery attempts.
func (s *Server) ListMyDeliveryLogs(c *gin.Context) {
	u, okU := s.requireUser(c)
	if !okU {
		return
	}
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "20"))
	list, err := s.Store.ListDeliveryLogs(u.TenantID, u.UserID, limit)
	if err != nil {
		fail(c, http.StatusInternalServerError, err.Error())
		return
	}
	ok(c, gin.H{"list": list})
}

// AdminListDeliveryLogs GET /api/v1/notify/deliveries/all —— 租户内全部
// 发送日志（邮件管理面，路线图第 12 项）；仅平台管理员（网关注入头判定，
// 与 bpm 管理视图同口径）。
func (s *Server) AdminListDeliveryLogs(c *gin.Context) {
	u, okU := s.requireUser(c)
	if !okU {
		return
	}
	if c.GetHeader("X-Auth-Platform-Admin") != "1" {
		fail(c, http.StatusForbidden, "仅平台管理员可查看全部发送日志")
		return
	}
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "20"))
	list, total, err := s.Store.ListAllDeliveryLogs(u.TenantID,
		c.Query("channel"), c.Query("status"), c.Query("keyword"), page, pageSize)
	if err != nil {
		fail(c, http.StatusInternalServerError, err.Error())
		return
	}
	ok(c, gin.H{"list": list, "total": total})
}

type internalSendReq struct {
	TenantID     uint64            `json:"tenant_id"`
	UserID       uint64            `json:"user_id"`
	UserIDs      []uint64          `json:"user_ids"` // batch
	TemplateCode string            `json:"template_code"`
	Type         string            `json:"type"`
	RefType      string            `json:"ref_type"`
	RefID        string            `json:"ref_id"`
	Vars         map[string]string `json:"vars"`
	Title        string            `json:"title"`
	Content      string            `json:"content"`
	Link         string            `json:"link"`
	// Channels optional override e.g. ["inbox","email","sms"]
	Channels []string `json:"channels"`
}

// InternalSend POST /api/v1/notify/internal/send
func (s *Server) InternalSend(c *gin.Context) {
	if !s.requireInternal(c) {
		return
	}
	var req internalSendReq
	if err := c.ShouldBindJSON(&req); err != nil {
		fail(c, http.StatusBadRequest, "invalid body")
		return
	}
	users := req.UserIDs
	if req.UserID > 0 {
		users = append(users, req.UserID)
	}
	if len(users) == 0 {
		fail(c, http.StatusBadRequest, "user_id or user_ids required")
		return
	}
	if req.TenantID == 0 {
		req.TenantID = 1
	}

	// 批量路径：模板解析 1 次 + 租户校验 1 条 + 去重 1 条 + 站内信 1 条批量
	// INSERT，替代原来 per-user 的 5 条查询。跨租户收件人仍被拒且不落库
	// （rejected 计数），24h 内重复仍被抑制（skipped 计数）。
	res, err := s.Store.SendBatch(c.Request.Context(), store.BatchSendInput{
		TenantID:     req.TenantID,
		UserIDs:      users,
		TemplateCode: req.TemplateCode,
		Type:         req.Type,
		RefType:      req.RefType,
		RefID:        req.RefID,
		Vars:         req.Vars,
		Title:        req.Title,
		Content:      req.Content,
		Link:         req.Link,
		Channels:     req.Channels,
	})
	if err != nil {
		fail(c, http.StatusBadRequest, err.Error())
		return
	}
	for _, uid := range res.RejectedUserIDs {
		log.Printf("notify internal send: user %d not in tenant %d, skipped (ref=%s/%s)",
			uid, req.TenantID, req.RefType, req.RefID)
	}

	// 外部渠道（email/sms）异步投递：阻塞的 SMTP / 短信外呼移出请求路径，
	// 站内信已落库故接口可以先返回。queued = 已入队的收件人数，
	// dropped = 队列满被丢弃（有日志与计数，不静默）。
	queued, dropped := 0, 0
	external := channel.Has(res.Rendered.Channels, "email") || channel.Has(res.Rendered.Channels, "sms")
	if external && s.Outbox != nil {
		payload := channel.Payload{
			TenantID: req.TenantID, TemplateCode: res.Rendered.Code, Params: req.Vars,
			Title: res.Rendered.Title, Content: res.Rendered.Content, Link: res.Rendered.Link,
		}
		for _, rec := range res.Recipients {
			okQ := s.Outbox.Enqueue(outboxJob{
				tenantID:  req.TenantID,
				userID:    rec.UserID,
				messageID: rec.MessageID,
				code:      res.Rendered.Code,
				refType:   req.RefType,
				refID:     req.RefID,
				channels:  res.Rendered.Channels,
				contact:   rec.Contact,
				payload:   payload,
			})
			if okQ {
				queued++
			} else {
				dropped++
			}
		}
	}

	// 返回体向后兼容：created / skipped / rejected 语义不变；delivered 在
	// 异步化后不再于返回时可知，保留为空数组（调用方只读计数，见各服务
	// notifyclient.SendResult），新增 queued / dropped 表达异步投递状态。
	ok(c, gin.H{
		"created":   res.Created,
		"skipped":   res.Skipped,
		"rejected":  res.Rejected,
		"delivered": []channel.Result{},
		"queued":    queued,
		"dropped":   dropped,
	})
}
