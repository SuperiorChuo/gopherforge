package api

import (
	"errors"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/go-admin-kit/services/ticket/internal/model"
	"github.com/go-admin-kit/services/ticket/internal/store/ticket"
	"gorm.io/gorm"
)

func notFoundOr(c *gin.Context, err error, msg string) {
	if errors.Is(err, gorm.ErrRecordNotFound) {
		fail(c, http.StatusNotFound, msg)
		return
	}
	fail(c, http.StatusBadRequest, err.Error())
}

func (s *Server) ListTickets(c *gin.Context) {
	u, authed := s.requireUser(c)
	if !authed {
		return
	}
	assignee, _ := strconv.ParseUint(c.Query("assignee_user_id"), 10, 64)
	creator, _ := strconv.ParseUint(c.Query("creator_user_id"), 10, 64)
	customer, _ := strconv.ParseUint(c.Query("customer_id"), 10, 64)
	overdue := c.Query("overdue") == "1" || strings.EqualFold(c.Query("overdue"), "true")
	list, total, err := s.Store.ListTickets(u.TenantID, store.TicketFilter{
		Status:             c.Query("status"),
		Priority:           c.Query("priority"),
		Source:             c.Query("source"),
		AssigneeUserID:     assignee,
		CreatorUserID:      creator,
		CustomerID:         customer,
		ConversationPublic: c.Query("conversation_public_id"),
		Keyword:            c.Query("keyword"),
		Scope:              c.Query("scope"),
		Me:                 u.UserID,
		OverdueOnly:        overdue,
		Page:               pageOf(c),
	})
	if err != nil {
		fail(c, http.StatusInternalServerError, err.Error())
		return
	}
	ok(c, gin.H{"list": list, "total": total})
}

type createReq struct {
	Title                string  `json:"title"`
	Description          string  `json:"description"`
	Priority             string  `json:"priority"`
	Source               string  `json:"source"`
	AssigneeUserID       uint64  `json:"assignee_user_id"`
	CustomerID           uint64  `json:"customer_id"`
	ConversationPublicID string  `json:"conversation_public_id"`
	IMVisitorID          uint64  `json:"im_visitor_id"`
	ContactName          string  `json:"contact_name"`
	ContactPhone         string  `json:"contact_phone"`
	DueAt                *string `json:"due_at"` // RFC3339
}

func parseDue(s *string) (*time.Time, error) {
	if s == nil || strings.TrimSpace(*s) == "" {
		return nil, nil
	}
	t, err := time.Parse(time.RFC3339, strings.TrimSpace(*s))
	if err != nil {
		// also accept date-only
		t, err = time.ParseInLocation("2006-01-02", strings.TrimSpace(*s), time.Local)
		if err != nil {
			return nil, err
		}
	}
	return &t, nil
}

func (s *Server) CreateTicket(c *gin.Context) {
	u, authed := s.requireUser(c)
	if !authed {
		return
	}
	var req createReq
	if err := c.ShouldBindJSON(&req); err != nil {
		fail(c, http.StatusBadRequest, "invalid body")
		return
	}
	due, err := parseDue(req.DueAt)
	if err != nil {
		fail(c, http.StatusBadRequest, "due_at 格式无效")
		return
	}
	source := req.Source
	if source == "" && req.ConversationPublicID != "" {
		source = "im"
	}
	tk, err := s.Store.CreateTicket(c.Request.Context(), u.TenantID, store.CreateTicketInput{
		Title:                req.Title,
		Description:          req.Description,
		Priority:             req.Priority,
		Source:               source,
		CreatorUserID:        u.UserID,
		AssigneeUserID:       req.AssigneeUserID,
		CustomerID:           req.CustomerID,
		ConversationPublicID: req.ConversationPublicID,
		IMVisitorID:          req.IMVisitorID,
		ContactName:          req.ContactName,
		ContactPhone:         req.ContactPhone,
		DueAt:                due,
	})
	if err != nil {
		fail(c, http.StatusBadRequest, err.Error())
		return
	}
	s.pushAssigned(tk, u.UserID)
	ok(c, tk)
}

func (s *Server) GetTicket(c *gin.Context) {
	u, authed := s.requireUser(c)
	if !authed {
		return
	}
	id, valid := pathID(c, "id")
	if !valid {
		return
	}
	tk, err := s.Store.GetTicket(id, u.TenantID)
	if err != nil {
		notFoundOr(c, err, "工单不存在")
		return
	}
	events, err := s.Store.ListEvents(id, u.TenantID)
	if err != nil {
		fail(c, http.StatusInternalServerError, err.Error())
		return
	}
	ok(c, gin.H{"ticket": tk, "events": events})
}

type updateReq struct {
	Title        *string `json:"title"`
	Description  *string `json:"description"`
	Priority     *string `json:"priority"`
	CustomerID   *uint64 `json:"customer_id"`
	ContactName  *string `json:"contact_name"`
	ContactPhone *string `json:"contact_phone"`
	// DueAt: omit = no change; empty string = clear; RFC3339/date = set
	DueAt *string `json:"due_at"`
}

func (s *Server) UpdateTicket(c *gin.Context) {
	u, authed := s.requireUser(c)
	if !authed {
		return
	}
	id, valid := pathID(c, "id")
	if !valid {
		return
	}
	var req updateReq
	if err := c.ShouldBindJSON(&req); err != nil {
		fail(c, http.StatusBadRequest, "invalid body")
		return
	}
	in := store.UpdateTicketInput{
		Title: req.Title, Description: req.Description, Priority: req.Priority,
		CustomerID: req.CustomerID, ContactName: req.ContactName, ContactPhone: req.ContactPhone,
		ByUserID: u.UserID,
	}
	if req.DueAt != nil {
		if strings.TrimSpace(*req.DueAt) == "" {
			var nilTime *time.Time
			in.DueAt = &nilTime
		} else {
			d, err := parseDue(req.DueAt)
			if err != nil {
				fail(c, http.StatusBadRequest, "due_at 格式无效")
				return
			}
			in.DueAt = &d
		}
	}
	tk, err := s.Store.UpdateTicket(c.Request.Context(), id, u.TenantID, in)
	if err != nil {
		notFoundOr(c, err, "工单不存在")
		return
	}
	ok(c, tk)
}

type statusReq struct {
	Status string `json:"status"`
	Note   string `json:"note"`
}

func (s *Server) ChangeStatus(c *gin.Context) {
	u, authed := s.requireUser(c)
	if !authed {
		return
	}
	id, valid := pathID(c, "id")
	if !valid {
		return
	}
	var req statusReq
	if err := c.ShouldBindJSON(&req); err != nil || strings.TrimSpace(req.Status) == "" {
		fail(c, http.StatusBadRequest, "status 必填")
		return
	}
	tk, err := s.Store.ChangeStatus(c.Request.Context(), id, u.TenantID, u.UserID, req.Status, req.Note)
	if err != nil {
		notFoundOr(c, err, "工单不存在")
		return
	}
	ok(c, tk)
}

type assignReq struct {
	AssigneeUserID uint64 `json:"assignee_user_id"`
}

func (s *Server) Assign(c *gin.Context) {
	u, authed := s.requireUser(c)
	if !authed {
		return
	}
	id, valid := pathID(c, "id")
	if !valid {
		return
	}
	var req assignReq
	if err := c.ShouldBindJSON(&req); err != nil {
		fail(c, http.StatusBadRequest, "invalid body")
		return
	}
	tk, err := s.Store.Assign(c.Request.Context(), id, u.TenantID, u.UserID, req.AssigneeUserID)
	if err != nil {
		notFoundOr(c, err, "工单不存在")
		return
	}
	s.pushAssigned(tk, u.UserID)
	ok(c, tk)
}

// pushAssigned 给被指派人写站内信（自指派跳过；失败只打日志）。
func (s *Server) pushAssigned(tk *model.Ticket, byUserID uint64) {
	if s.Store == nil || tk == nil || tk.AssigneeUserID == 0 {
		return
	}
	if tk.AssigneeUserID == byUserID {
		return
	}
	ticketID := tk.ID
	tenantID := tk.TenantID
	uid := tk.AssigneeUserID
	ticketNo := tk.TicketNo
	title := tk.Title
	if err := s.Store.EnqueueNotification(tenantID, uid, "ticket.assigned", "ticket",
		strconv.FormatUint(ticketID, 10), map[string]string{
			"ticket_id": strconv.FormatUint(ticketID, 10),
			"ticket_no": ticketNo,
			"title":     title,
		}); err != nil {
		log.Printf("ticket assigned-notify enqueue id=%d user=%d: %v", ticketID, uid, err)
	}
}

type commentReq struct {
	Content string `json:"content"`
}

func (s *Server) AddComment(c *gin.Context) {
	u, authed := s.requireUser(c)
	if !authed {
		return
	}
	id, valid := pathID(c, "id")
	if !valid {
		return
	}
	var req commentReq
	if err := c.ShouldBindJSON(&req); err != nil {
		fail(c, http.StatusBadRequest, "invalid body")
		return
	}
	ev, err := s.Store.AddComment(c.Request.Context(), id, u.TenantID, u.UserID, req.Content)
	if err != nil {
		notFoundOr(c, err, "工单不存在")
		return
	}
	if tk, err := s.Store.GetTicket(id, u.TenantID); err == nil {
		s.pushComment(tk, u.UserID, ev.ID, req.Content)
	}
	ok(c, ev)
}

// pushComment 评论推给当前处理人（评论人自己跳过）。
func (s *Server) pushComment(tk *model.Ticket, byUserID, eventID uint64, content string) {
	if s.Store == nil || tk == nil || tk.AssigneeUserID == 0 {
		return
	}
	if tk.AssigneeUserID == byUserID {
		return
	}
	snippet := strings.TrimSpace(content)
	if len([]rune(snippet)) > 80 {
		snippet = string([]rune(snippet)[:80]) + "…"
	}
	ticketID := tk.ID
	tenantID := tk.TenantID
	uid := tk.AssigneeUserID
	ticketNo := tk.TicketNo
	title := tk.Title
	if err := s.Store.EnqueueNotification(tenantID, uid, "ticket.comment", "ticket_comment",
		strconv.FormatUint(eventID, 10), map[string]string{
			"ticket_id": strconv.FormatUint(ticketID, 10),
			"ticket_no": ticketNo,
			"title":     title,
			"snippet":   snippet,
		}); err != nil {
		log.Printf("ticket comment-notify enqueue id=%d user=%d: %v", ticketID, uid, err)
	}
}

func (s *Server) ListEvents(c *gin.Context) {
	u, authed := s.requireUser(c)
	if !authed {
		return
	}
	id, valid := pathID(c, "id")
	if !valid {
		return
	}
	if _, err := s.Store.GetTicket(id, u.TenantID); err != nil {
		notFoundOr(c, err, "工单不存在")
		return
	}
	events, err := s.Store.ListEvents(id, u.TenantID)
	if err != nil {
		fail(c, http.StatusInternalServerError, err.Error())
		return
	}
	ok(c, gin.H{"list": events})
}

func (s *Server) Stats(c *gin.Context) {
	u, authed := s.requireUser(c)
	if !authed {
		return
	}
	st, err := s.Store.Stats(u.TenantID, u.UserID)
	if err != nil {
		fail(c, http.StatusInternalServerError, err.Error())
		return
	}
	ok(c, st)
}

// SLAPolicy returns active default SLA hours by priority (for UI copy).
func (s *Server) SLAPolicy(c *gin.Context) {
	if _, authed := s.requireUser(c); !authed {
		return
	}
	ok(c, store.CurrentSLAPolicy())
}
