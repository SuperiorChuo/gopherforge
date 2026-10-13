// Package store persists tickets. AutoMigrate experimental track (same as im/crm).
// All queries force tenant_id isolation.
package store

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	connpool "github.com/go-admin-kit/services/shared/pkg/connpool"
	"github.com/go-admin-kit/services/shared/pkg/fullread"
	"github.com/go-admin-kit/services/ticket/internal/model"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

var (
	ErrInvalidStatus   = errors.New("非法状态")
	ErrInvalidPriority = errors.New("非法优先级")
	ErrInvalidSource   = errors.New("非法来源")
	ErrBadTransition   = errors.New("不允许的状态流转")
	ErrEmptyTitle      = errors.New("标题不能为空")
)

type Store struct {
	db *gorm.DB
}

func Open(dsn string) (*Store, error) {
	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{PrepareStmt: true})
	if err != nil {
		return nil, err
	}
	connpool.Apply(db)
	return NewWithDB(db)
}

// NewWithDB wraps an existing gorm.DB (tests inject sqlite in-memory).
func NewWithDB(db *gorm.DB) (*Store, error) {
	s := &Store{db: db}
	if err := db.AutoMigrate(&model.Ticket{}, &model.TicketEvent{}, &model.NotificationJob{}); err != nil {
		return nil, err
	}
	return s, nil
}

func (s *Store) EnqueueNotification(tenantID, userID uint64, templateCode, refType, refID string, vars map[string]string) error {
	payload, err := json.Marshal(vars)
	if err != nil {
		return err
	}
	key := strings.Join([]string{fmt.Sprint(tenantID), templateCode, refType, refID, fmt.Sprint(userID)}, ":")
	job := model.NotificationJob{TenantID: tenantID, DedupeKey: key, UserID: userID,
		TemplateCode: templateCode, RefType: refType, RefID: refID, VarsJSON: payload,
		Status: "pending", NextAt: time.Now()}
	return s.db.Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "dedupe_key"}}, DoNothing: true}).Create(&job).Error
}

func (s *Store) ClaimNotificationJobs(limit int, lease time.Duration) ([]model.NotificationJob, error) {
	if limit <= 0 || limit > 100 {
		limit = 50
	}
	var jobs []model.NotificationJob
	now := time.Now()
	err := s.db.Transaction(func(tx *gorm.DB) error {
		q := tx.Where("status = ? AND next_at <= ?", "pending", now).Order("id").Limit(limit)
		if tx.Dialector.Name() == "postgres" {
			q = q.Clauses(clause.Locking{Strength: "UPDATE", Options: "SKIP LOCKED"})
		}
		if err := q.Find(&jobs).Error; err != nil || len(jobs) == 0 {
			return err
		}
		ids := make([]uint64, len(jobs))
		for i := range jobs {
			ids[i] = jobs[i].ID
		}
		return tx.Model(&model.NotificationJob{}).Where("id IN ?", ids).Update("next_at", now.Add(lease)).Error
	})
	return jobs, err
}

func (s *Store) FinishNotificationJob(id uint64) error {
	return s.db.Delete(&model.NotificationJob{}, id).Error
}

func (s *Store) RetryNotificationJob(id uint64, attempts, maxAttempts int, lastErr string) error {
	if len(lastErr) > 480 {
		lastErr = lastErr[:480]
	}
	updates := map[string]any{"attempts": attempts, "last_error": lastErr}
	if attempts >= maxAttempts {
		updates["status"] = "dead"
	} else {
		shift := attempts - 1
		if shift < 0 {
			shift = 0
		}
		if shift > 6 {
			shift = 6
		}
		updates["next_at"] = time.Now().Add(time.Duration(1<<uint(shift)) * time.Minute)
	}
	return s.db.Model(&model.NotificationJob{}).Where("id = ?", id).Updates(updates).Error
}

func (s *Store) DB() *gorm.DB { return s.db }

func tenantQ(db *gorm.DB, tenantID uint64) *gorm.DB {
	return db.Where("tenant_id = ?", tenantID)
}

type Page struct {
	Page     int
	PageSize int
}

func (p Page) clamp() (offset, limit int) {
	if p.Page <= 0 {
		p.Page = 1
	}
	if p.PageSize <= 0 || p.PageSize > 100 {
		p.PageSize = 20
	}
	return (p.Page - 1) * p.PageSize, p.PageSize
}

// ---------- validation helpers ----------

func NormalizeStatus(s string) (string, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return model.StatusOpen, nil
	}
	switch s {
	case model.StatusOpen, model.StatusInProgress, model.StatusResolved, model.StatusClosed:
		return s, nil
	default:
		return "", ErrInvalidStatus
	}
}

func NormalizePriority(p string) (string, error) {
	p = strings.TrimSpace(p)
	if p == "" {
		return model.PriorityNormal, nil
	}
	switch p {
	case model.PriorityLow, model.PriorityNormal, model.PriorityHigh, model.PriorityUrgent:
		return p, nil
	default:
		return "", ErrInvalidPriority
	}
}

func NormalizeSource(src string) (string, error) {
	src = strings.TrimSpace(src)
	if src == "" {
		return model.SourceManual, nil
	}
	switch src {
	case model.SourceManual, model.SourceIM, model.SourceCall, model.SourceOther:
		return src, nil
	default:
		return "", ErrInvalidSource
	}
}

// CanTransition returns whether from → to is allowed.
func CanTransition(from, to string) bool {
	if from == to {
		return true
	}
	switch from {
	case model.StatusOpen:
		return to == model.StatusInProgress || to == model.StatusResolved || to == model.StatusClosed
	case model.StatusInProgress:
		return to == model.StatusOpen || to == model.StatusResolved || to == model.StatusClosed
	case model.StatusResolved:
		return to == model.StatusClosed || to == model.StatusInProgress
	case model.StatusClosed:
		return to == model.StatusOpen // reopen
	default:
		return false
	}
}

// ---------- filters ----------

type TicketFilter struct {
	Status             string
	Priority           string
	Source             string
	AssigneeUserID     uint64
	CreatorUserID      uint64
	CustomerID         uint64
	ConversationPublic string
	Keyword            string
	// Scope: all | mine | unassigned
	Scope string
	Me    uint64
	// OverdueOnly: open/in_progress past DueAt
	OverdueOnly bool
	Page
}

func (s *Store) ListTickets(tenantID uint64, f TicketFilter) ([]model.Ticket, int64, error) {
	q := tenantQ(s.db.Model(&model.Ticket{}), tenantID)
	if f.Status != "" {
		q = q.Where("status = ?", f.Status)
	}
	if f.Priority != "" {
		q = q.Where("priority = ?", f.Priority)
	}
	if f.Source != "" {
		q = q.Where("source = ?", f.Source)
	}
	if f.AssigneeUserID > 0 {
		q = q.Where("assignee_user_id = ?", f.AssigneeUserID)
	}
	if f.CreatorUserID > 0 {
		q = q.Where("creator_user_id = ?", f.CreatorUserID)
	}
	if f.CustomerID > 0 {
		q = q.Where("customer_id = ?", f.CustomerID)
	}
	if f.ConversationPublic != "" {
		q = q.Where("conversation_public_id = ?", f.ConversationPublic)
	}
	switch f.Scope {
	case "mine":
		q = q.Where("assignee_user_id = ?", f.Me)
	case "unassigned":
		q = q.Where("assignee_user_id = 0")
	}
	if f.OverdueOnly {
		now := time.Now()
		q = q.Where("due_at IS NOT NULL AND due_at < ? AND status IN ?", now,
			[]string{model.StatusOpen, model.StatusInProgress})
	}
	if kw := strings.TrimSpace(f.Keyword); kw != "" {
		like := "%" + kw + "%"
		q = q.Where(
			"ticket_no ILIKE ? OR title ILIKE ? OR contact_name ILIKE ? OR contact_phone ILIKE ? OR description ILIKE ?",
			like, like, like, like, like,
		)
	}
	var total int64
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	offset, limit := f.Page.clamp()
	var list []model.Ticket
	err := q.Order("id DESC").Offset(offset).Limit(limit).Find(&list).Error
	return list, total, err
}

func (s *Store) GetTicket(id, tenantID uint64) (*model.Ticket, error) {
	var t model.Ticket
	err := tenantQ(s.db, tenantID).Where("id = ?", id).First(&t).Error
	if err != nil {
		return nil, err
	}
	return &t, nil
}

type CreateTicketInput struct {
	Title                string
	Description          string
	Status               string
	Priority             string
	Source               string
	CreatorUserID        uint64
	AssigneeUserID       uint64
	CustomerID           uint64
	ConversationPublicID string
	IMVisitorID          uint64
	ContactName          string
	ContactPhone         string
	DueAt                *time.Time
}

func (s *Store) CreateTicket(ctx context.Context, tenantID uint64, in CreateTicketInput) (*model.Ticket, error) {
	title := strings.TrimSpace(in.Title)
	if title == "" {
		return nil, ErrEmptyTitle
	}
	status, err := NormalizeStatus(in.Status)
	if err != nil {
		return nil, err
	}
	priority, err := NormalizePriority(in.Priority)
	if err != nil {
		return nil, err
	}
	source, err := NormalizeSource(in.Source)
	if err != nil {
		return nil, err
	}
	assignee := in.AssigneeUserID
	if assignee == 0 {
		assignee = in.CreatorUserID
	}

	// M1.5：未显式指定截止时间时，按优先级套默认 SLA
	dueAt := in.DueAt
	slaApplied := false
	if dueAt == nil {
		if d := DefaultDueAt(priority, time.Now()); d != nil {
			dueAt = d
			slaApplied = true
		}
	}

	t := &model.Ticket{
		TenantID:             tenantID,
		TicketNo:             "pending",
		Title:                title,
		Description:          strings.TrimSpace(in.Description),
		Status:               status,
		Priority:             priority,
		Source:               source,
		CreatorUserID:        in.CreatorUserID,
		AssigneeUserID:       assignee,
		CustomerID:           in.CustomerID,
		ConversationPublicID: strings.TrimSpace(in.ConversationPublicID),
		IMVisitorID:          in.IMVisitorID,
		ContactName:          strings.TrimSpace(in.ContactName),
		ContactPhone:         strings.TrimSpace(in.ContactPhone),
		DueAt:                dueAt,
	}

	err = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(t).Error; err != nil {
			return err
		}
		t.TicketNo = fmt.Sprintf("TK-%d", t.ID)
		if err := tx.Model(t).Update("ticket_no", t.TicketNo).Error; err != nil {
			return err
		}
		content := "创建工单"
		if slaApplied && t.DueAt != nil {
			content = fmt.Sprintf("创建工单（SLA 截止 %s）", t.DueAt.Format("2006-01-02 15:04"))
		}
		ev := model.TicketEvent{
			TenantID: tenantID,
			TicketID: t.ID,
			Kind:     model.EventCreated,
			Content:  content,
			ToStatus: t.Status,
			ByUserID: in.CreatorUserID,
		}
		return tx.Create(&ev).Error
	})
	if err != nil {
		return nil, err
	}
	return t, nil
}

type UpdateTicketInput struct {
	Title        *string
	Description  *string
	Priority     *string
	CustomerID   *uint64
	ContactName  *string
	ContactPhone *string
	DueAt        **time.Time // pointer to pointer: nil=no change; &nil=clear; &time=set
	ByUserID     uint64
}

func (s *Store) UpdateTicket(ctx context.Context, id, tenantID uint64, in UpdateTicketInput) (*model.Ticket, error) {
	t, err := s.GetTicket(id, tenantID)
	if err != nil {
		return nil, err
	}
	changed := []string{}
	if in.Title != nil {
		title := strings.TrimSpace(*in.Title)
		if title == "" {
			return nil, ErrEmptyTitle
		}
		if title != t.Title {
			t.Title = title
			changed = append(changed, "标题")
		}
	}
	if in.Description != nil {
		t.Description = strings.TrimSpace(*in.Description)
		changed = append(changed, "描述")
	}
	if in.Priority != nil {
		p, err := NormalizePriority(*in.Priority)
		if err != nil {
			return nil, err
		}
		if p != t.Priority {
			t.Priority = p
			changed = append(changed, "优先级→"+p)
		}
	}
	if in.CustomerID != nil {
		t.CustomerID = *in.CustomerID
		changed = append(changed, "客户")
	}
	if in.ContactName != nil {
		t.ContactName = strings.TrimSpace(*in.ContactName)
		changed = append(changed, "联系人")
	}
	if in.ContactPhone != nil {
		t.ContactPhone = strings.TrimSpace(*in.ContactPhone)
		changed = append(changed, "电话")
	}
	if in.DueAt != nil {
		t.DueAt = *in.DueAt
		changed = append(changed, "截止时间")
	}
	if len(changed) == 0 {
		return t, nil
	}
	err = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Save(t).Error; err != nil {
			return err
		}
		return tx.Create(&model.TicketEvent{
			TenantID: tenantID,
			TicketID: t.ID,
			Kind:     model.EventUpdate,
			Content:  "更新：" + strings.Join(changed, "、"),
			ByUserID: in.ByUserID,
		}).Error
	})
	if err != nil {
		return nil, err
	}
	return t, nil
}

func (s *Store) ChangeStatus(ctx context.Context, id, tenantID, byUserID uint64, toStatus, note string) (*model.Ticket, error) {
	to, err := NormalizeStatus(toStatus)
	if err != nil {
		return nil, err
	}
	t, err := s.GetTicket(id, tenantID)
	if err != nil {
		return nil, err
	}
	if !CanTransition(t.Status, to) {
		return nil, fmt.Errorf("%w: %s → %s", ErrBadTransition, t.Status, to)
	}
	from := t.Status
	if from == to {
		return t, nil
	}
	now := time.Now()
	t.Status = to
	note = strings.TrimSpace(note)
	if to == model.StatusResolved {
		t.ResolvedAt = &now
		if note != "" {
			t.ResolveNote = note
		}
	}
	if to == model.StatusClosed {
		t.ClosedAt = &now
		if note != "" {
			t.ResolveNote = note
		}
	}
	if to == model.StatusOpen || to == model.StatusInProgress {
		// reopen clears closed stamp
		if from == model.StatusClosed || from == model.StatusResolved {
			t.ClosedAt = nil
		}
	}
	err = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Save(t).Error; err != nil {
			return err
		}
		content := fmt.Sprintf("%s → %s", from, to)
		if note != "" {
			content += "：" + note
		}
		return tx.Create(&model.TicketEvent{
			TenantID:   tenantID,
			TicketID:   t.ID,
			Kind:       model.EventStatus,
			Content:    content,
			FromStatus: from,
			ToStatus:   to,
			ByUserID:   byUserID,
		}).Error
	})
	if err != nil {
		return nil, err
	}
	return t, nil
}

func (s *Store) Assign(ctx context.Context, id, tenantID, byUserID, assigneeUserID uint64) (*model.Ticket, error) {
	t, err := s.GetTicket(id, tenantID)
	if err != nil {
		return nil, err
	}
	from := t.AssigneeUserID
	if from == assigneeUserID {
		return t, nil
	}
	t.AssigneeUserID = assigneeUserID
	// auto move open → in_progress when first assigned to someone
	statusChanged := false
	fromStatus := t.Status
	if t.Status == model.StatusOpen && assigneeUserID > 0 {
		t.Status = model.StatusInProgress
		statusChanged = true
	}
	err = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Save(t).Error; err != nil {
			return err
		}
		content := fmt.Sprintf("指派 %d → %d", from, assigneeUserID)
		if err := tx.Create(&model.TicketEvent{
			TenantID: tenantID,
			TicketID: t.ID,
			Kind:     model.EventAssign,
			Content:  content,
			ByUserID: byUserID,
		}).Error; err != nil {
			return err
		}
		if statusChanged {
			return tx.Create(&model.TicketEvent{
				TenantID:   tenantID,
				TicketID:   t.ID,
				Kind:       model.EventStatus,
				Content:    fmt.Sprintf("%s → %s（指派自动推进）", fromStatus, t.Status),
				FromStatus: fromStatus,
				ToStatus:   t.Status,
				ByUserID:   byUserID,
			}).Error
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return t, nil
}

func (s *Store) AddComment(ctx context.Context, id, tenantID, byUserID uint64, content string) (*model.TicketEvent, error) {
	content = strings.TrimSpace(content)
	if content == "" {
		return nil, errors.New("评论不能为空")
	}
	if _, err := s.GetTicket(id, tenantID); err != nil {
		return nil, err
	}
	ev := &model.TicketEvent{
		TenantID: tenantID,
		TicketID: id,
		Kind:     model.EventComment,
		Content:  content,
		ByUserID: byUserID,
	}
	if err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		return tx.Create(ev).Error
	}); err != nil {
		return nil, err
	}
	return ev, nil
}

func (s *Store) ListEvents(ticketID, tenantID uint64) ([]model.TicketEvent, error) {
	return fullread.All[model.TicketEvent](tenantQ(s.db.Model(&model.TicketEvent{}), tenantID).
		Where("ticket_id = ?", ticketID).
		Order("id ASC"))
}

// ListOverdueForNotify returns open/in_progress tickets past due_at (for inbox push).
// limit caps scan size per cron tick.
func (s *Store) ListOverdueForNotify(limit int) ([]model.Ticket, error) {
	if limit <= 0 {
		limit = 100
	}
	now := time.Now()
	var list []model.Ticket
	err := s.db.Where(
		"due_at IS NOT NULL AND due_at < ? AND status IN ?",
		now, []string{model.StatusOpen, model.StatusInProgress},
	).Order("due_at ASC").Limit(limit).Find(&list).Error
	return list, err
}

// ListDueSoonForNotify returns open/in_progress tickets whose due_at is within
// the next withinHours (not yet overdue). Used for SLA pre-warning.
func (s *Store) ListDueSoonForNotify(withinHours, limit int) ([]model.Ticket, error) {
	if withinHours <= 0 {
		withinHours = 4
	}
	if limit <= 0 {
		limit = 100
	}
	now := time.Now()
	until := now.Add(time.Duration(withinHours) * time.Hour)
	var list []model.Ticket
	err := s.db.Where(
		"due_at IS NOT NULL AND due_at > ? AND due_at <= ? AND status IN ?",
		now, until, []string{model.StatusOpen, model.StatusInProgress},
	).Order("due_at ASC").Limit(limit).Find(&list).Error
	return list, err
}

// Stats returns open-ish counters for dashboard widgets / global badge.
type Stats struct {
	Open        int64 `json:"open"`
	InProgress  int64 `json:"in_progress"`
	Resolved    int64 `json:"resolved"`
	Closed      int64 `json:"closed"`
	Overdue     int64 `json:"overdue"`
	MineOpen    int64 `json:"mine_open"`
	MineOverdue int64 `json:"mine_overdue"`
}

func (s *Store) Stats(tenantID, me uint64) (Stats, error) {
	var st Stats
	base := tenantQ(s.db.Model(&model.Ticket{}), tenantID)
	count := func(dest *int64, conds ...any) error {
		q := base.Session(&gorm.Session{})
		if len(conds) > 0 {
			q = q.Where(conds[0], conds[1:]...)
		}
		return q.Count(dest).Error
	}
	if err := count(&st.Open, "status = ?", model.StatusOpen); err != nil {
		return st, err
	}
	if err := count(&st.InProgress, "status = ?", model.StatusInProgress); err != nil {
		return st, err
	}
	if err := count(&st.Resolved, "status = ?", model.StatusResolved); err != nil {
		return st, err
	}
	if err := count(&st.Closed, "status = ?", model.StatusClosed); err != nil {
		return st, err
	}
	now := time.Now()
	openish := []string{model.StatusOpen, model.StatusInProgress}
	if err := count(&st.Overdue,
		"due_at IS NOT NULL AND due_at < ? AND status IN ?",
		now, openish); err != nil {
		return st, err
	}
	if me > 0 {
		if err := count(&st.MineOpen,
			"assignee_user_id = ? AND status IN ?",
			me, openish); err != nil {
			return st, err
		}
		if err := count(&st.MineOverdue,
			"assignee_user_id = ? AND due_at IS NOT NULL AND due_at < ? AND status IN ?",
			me, now, openish); err != nil {
			return st, err
		}
	}
	return st, nil
}
