package system

import (
	"context"
	"errors"
	"strings"

	"github.com/gin-gonic/gin"
	dao "github.com/go-admin-kit/server/internal/dao/system"
	"github.com/go-admin-kit/server/internal/model"
)

const (
	defaultAuditActorType = "operator"
	defaultAuditActorID   = "web-console"
	defaultAuditLimit     = 200
	maxAuditPageSize      = 500
)

// AuditLogService handles independent business audit log behavior.
type AuditLogService struct {
	logDAO dao.AuditLogDAO
}

type AuditLogListRequest struct {
	Page       int    `form:"page" json:"page"`
	PageSize   int    `form:"page_size" json:"page_size"`
	Limit      int    `form:"limit" json:"limit"`
	Action     string `form:"action" json:"action"`
	TargetType string `form:"target_type" json:"target_type"`
	TargetID   string `form:"target_id" json:"target_id"`
	View       string `form:"view" json:"view"`
	Keyword    string `form:"keyword" json:"keyword"`
	SortBy     string `form:"sort_by" json:"sort_by"`
	SortOrder  string `form:"sort_order" json:"sort_order"`
}

type AuditRecordRequest struct {
	ActorType  string         `json:"actor_type"`
	ActorID    string         `json:"actor_id"`
	Action     string         `json:"action"`
	TargetType string         `json:"target_type"`
	TargetID   string         `json:"target_id"`
	Before     map[string]any `json:"before"`
	After      map[string]any `json:"after"`
	Summary    string         `json:"summary"`
}

func (s *AuditLogService) Record(c *gin.Context, req AuditRecordRequest) error {
	ctx := context.Background()
	if c != nil && c.Request != nil {
		ctx = c.Request.Context()
	}
	return s.RecordContext(ctx, c, req)
}

func (s *AuditLogService) RecordContext(ctx context.Context, c *gin.Context, req AuditRecordRequest) error {
	log := buildAuditLog(c, req)
	if log.Action == "" {
		return errors.New("action is required")
	}
	if log.TargetType == "" {
		return errors.New("target_type is required")
	}
	if log.TargetID == "" {
		return errors.New("target_id is required")
	}
	return s.logDAO.CreateLogContext(ctx, log)
}

// Deprecated: use ListLogsContext instead.
func (s *AuditLogService) ListLogs(req AuditLogListRequest) (dao.AuditLogListResult, error) {
	return s.ListLogsContext(context.Background(), req)
}

func (s *AuditLogService) ListLogsContext(ctx context.Context, req AuditLogListRequest) (dao.AuditLogListResult, error) {
	normalized := NormalizeAuditLogListRequest(req)
	return s.logDAO.ListLogsContext(ctx, dao.AuditLogListQuery{
		Page:       normalized.Page,
		PageSize:   normalized.PageSize,
		Action:     normalized.Action,
		TargetType: normalized.TargetType,
		TargetID:   normalized.TargetID,
		View:       normalized.View,
		Keyword:    normalized.Keyword,
		SortBy:     normalized.SortBy,
		SortOrder:  normalized.SortOrder,
	})
}

func NormalizeAuditLogListRequest(req AuditLogListRequest) AuditLogListRequest {
	req.Action = strings.TrimSpace(req.Action)
	req.TargetType = strings.TrimSpace(req.TargetType)
	req.TargetID = strings.TrimSpace(req.TargetID)
	req.Keyword = strings.TrimSpace(req.Keyword)
	req.View = NormalizeAuditView(req.View)
	req.SortBy = NormalizeAuditSortBy(req.SortBy)
	req.SortOrder = NormalizeAuditSortOrder(req.SortOrder)

	pageSize := req.PageSize
	if pageSize == 0 {
		pageSize = req.Limit
	}
	if pageSize == 0 {
		pageSize = defaultAuditLimit
	}
	if pageSize < 1 {
		pageSize = 1
	}
	if pageSize > maxAuditPageSize {
		pageSize = maxAuditPageSize
	}
	req.PageSize = pageSize

	if req.Page < 1 {
		req.Page = 1
	}
	return req
}

func NormalizeAuditView(value string) string {
	switch strings.ToUpper(strings.TrimSpace(value)) {
	case "ALL":
		return "ALL"
	default:
		return "ALL"
	}
}

func NormalizeAuditSortBy(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "id", "action", "target_type", "target_id", "actor_id":
		return strings.ToLower(strings.TrimSpace(value))
	default:
		return "created_at"
	}
}

func NormalizeAuditSortOrder(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "asc":
		return "asc"
	case "desc":
		return "desc"
	default:
		return "desc"
	}
}

func normalizeAuditRecord(log *model.AuditLog) {
	log.Action = strings.TrimSpace(log.Action)
	log.TargetType = strings.TrimSpace(log.TargetType)
	log.TargetID = strings.TrimSpace(log.TargetID)
	log.ActorType = strings.TrimSpace(log.ActorType)
	log.ActorID = strings.TrimSpace(log.ActorID)
	log.Summary = strings.TrimSpace(log.Summary)
	if log.ActorType == "" {
		log.ActorType = defaultAuditActorType
	}
	if log.ActorID == "" {
		log.ActorID = defaultAuditActorID
	}
}

func buildAuditLog(c *gin.Context, req AuditRecordRequest) *model.AuditLog {
	actorType, actorID := resolveAuditActor(c)
	if value := strings.TrimSpace(req.ActorType); value != "" {
		actorType = value
	}
	if value := strings.TrimSpace(req.ActorID); value != "" {
		actorID = value
	}

	log := &model.AuditLog{
		ActorType:  actorType,
		ActorID:    actorID,
		Action:     req.Action,
		TargetType: req.TargetType,
		TargetID:   req.TargetID,
		BeforeJSON: req.Before,
		AfterJSON:  req.After,
		Summary:    req.Summary,
	}
	normalizeAuditRecord(log)
	return log
}

func resolveAuditActor(c *gin.Context) (string, string) {
	actorType := defaultAuditActorType
	actorID := defaultAuditActorID
	if c == nil {
		return actorType, actorID
	}
	if value, ok := c.Get("audit_actor_type"); ok {
		if typed, ok := value.(string); ok && strings.TrimSpace(typed) != "" {
			actorType = strings.TrimSpace(typed)
		}
	}
	if value, ok := c.Get("audit_actor_id"); ok {
		if typed, ok := value.(string); ok && strings.TrimSpace(typed) != "" {
			actorID = strings.TrimSpace(typed)
		}
	}
	return actorType, actorID
}
