package system

import (
	"context"
	"strings"

	"github.com/go-admin-kit/server/internal/model"
	"github.com/go-admin-kit/server/internal/pkg/database"
	"gorm.io/gorm"
)

// AuditLogDAO is the data access layer for independent business audit logs.
type AuditLogDAO struct {
	db *gorm.DB
}

func NewAuditLogDAO(db *gorm.DB) *AuditLogDAO {
	return &AuditLogDAO{db: db}
}

func (d *AuditLogDAO) dbWithContext(ctx context.Context) *gorm.DB {
	if ctx == nil {
		ctx = context.Background()
	}
	if d != nil && d.db != nil {
		return d.db.WithContext(ctx)
	}
	return database.DB.WithContext(ctx)
}

type AuditLogListQuery struct {
	Page       int
	PageSize   int
	Action     string
	TargetType string
	TargetID   string
	View       string
	Keyword    string
	SortBy     string
	SortOrder  string
}

type AuditLogListResult struct {
	Items      []model.AuditLog   `json:"items"`
	Pagination AuditLogPagination `json:"pagination"`
	Summary    AuditLogSummary    `json:"summary"`
	Facets     AuditLogFacets     `json:"facets"`
}

type AuditLogPagination struct {
	Page       int   `json:"page"`
	PageSize   int   `json:"page_size"`
	Total      int64 `json:"total"`
	TotalPages int   `json:"total_pages"`
	HasPrev    bool  `json:"has_prev"`
	HasNext    bool  `json:"has_next"`
}

type AuditLogFacets struct {
	Actions     []string `json:"actions"`
	TargetTypes []string `json:"target_types"`
	ActorTypes  []string `json:"actor_types"`
}

type AuditLogSummary struct {
	TotalLogs           int64                      `json:"total_logs"`
	DistinctActions     int64                      `json:"distinct_actions"`
	DistinctTargetTypes int64                      `json:"distinct_target_types"`
	DistinctActorIDs    int64                      `json:"distinct_actor_ids"`
	ActionBreakdown     []AuditLogBreakdownSummary `json:"action_breakdown"`
}

type AuditLogBreakdownSummary struct {
	Action string `json:"action"`
	Count  int64  `json:"count"`
}

// Deprecated: use CreateLogContext instead.
func (d *AuditLogDAO) CreateLog(log *model.AuditLog) error {
	return d.CreateLogContext(context.Background(), log)
}

func (d *AuditLogDAO) CreateLogContext(ctx context.Context, log *model.AuditLog) error {
	return d.dbWithContext(ctx).Create(log).Error
}

// Deprecated: use ListLogsContext instead.
func (d *AuditLogDAO) ListLogs(req AuditLogListQuery) (AuditLogListResult, error) {
	return d.ListLogsContext(context.Background(), req)
}

func (d *AuditLogDAO) ListLogsContext(ctx context.Context, req AuditLogListQuery) (AuditLogListResult, error) {
	if ctx == nil {
		ctx = context.Background()
	}

	var result AuditLogListResult
	baseQuery := applyAuditBaseFilters(d.dbWithContext(ctx).Model(&model.AuditLog{}), req)
	listQuery := applyAuditViewFilter(baseQuery.Session(&gorm.Session{}), req.View)

	if err := listQuery.Count(&result.Pagination.Total).Error; err != nil {
		return result, err
	}

	totalPages := calculateAuditTotalPages(result.Pagination.Total, req.PageSize)
	page := req.Page
	if page > totalPages {
		page = totalPages
	}
	offset := (page - 1) * req.PageSize

	if err := listQuery.
		Order(auditOrderClause(req.SortBy, req.SortOrder)).
		Limit(req.PageSize).
		Offset(offset).
		Find(&result.Items).Error; err != nil {
		return result, err
	}

	summary, err := d.BuildSummary(baseQuery.Session(&gorm.Session{}))
	if err != nil {
		return result, err
	}
	facets, err := d.BuildFacets(baseQuery.Session(&gorm.Session{}))
	if err != nil {
		return result, err
	}

	result.Pagination = AuditLogPagination{
		Page:       page,
		PageSize:   req.PageSize,
		Total:      result.Pagination.Total,
		TotalPages: totalPages,
		HasPrev:    page > 1,
		HasNext:    page < totalPages,
	}
	result.Summary = summary
	result.Facets = facets
	return result, nil
}

func (d *AuditLogDAO) BuildSummary(baseQuery *gorm.DB) (AuditLogSummary, error) {
	var summary AuditLogSummary
	if err := baseQuery.Session(&gorm.Session{}).Count(&summary.TotalLogs).Error; err != nil {
		return summary, err
	}

	var err error
	if summary.DistinctActions, err = countAuditDistinct(baseQuery.Session(&gorm.Session{}), "action"); err != nil {
		return summary, err
	}
	if summary.DistinctTargetTypes, err = countAuditDistinct(baseQuery.Session(&gorm.Session{}), "target_type"); err != nil {
		return summary, err
	}
	if summary.DistinctActorIDs, err = countAuditDistinct(baseQuery.Session(&gorm.Session{}), "actor_id"); err != nil {
		return summary, err
	}

	if err := baseQuery.Session(&gorm.Session{}).
		Select("action, COUNT(*) as count").
		Where("action IS NOT NULL AND action <> ''").
		Group("action").
		Order("count DESC, action ASC").
		Find(&summary.ActionBreakdown).Error; err != nil {
		return summary, err
	}

	return summary, nil
}

func (d *AuditLogDAO) BuildFacets(baseQuery *gorm.DB) (AuditLogFacets, error) {
	var facets AuditLogFacets
	var err error
	if facets.Actions, err = distinctAuditValues(baseQuery.Session(&gorm.Session{}), "action"); err != nil {
		return facets, err
	}
	if facets.TargetTypes, err = distinctAuditValues(baseQuery.Session(&gorm.Session{}), "target_type"); err != nil {
		return facets, err
	}
	if facets.ActorTypes, err = distinctAuditValues(baseQuery.Session(&gorm.Session{}), "actor_type"); err != nil {
		return facets, err
	}
	return facets, nil
}

func applyAuditBaseFilters(query *gorm.DB, req AuditLogListQuery) *gorm.DB {
	if req.Action != "" {
		query = query.Where("action = ?", req.Action)
	}
	if req.TargetType != "" {
		query = query.Where("target_type = ?", req.TargetType)
	}
	if req.TargetID != "" {
		query = query.Where("target_id = ?", req.TargetID)
	}
	if req.Keyword != "" {
		pattern := "%" + strings.ToLower(req.Keyword) + "%"
		query = query.Where(
			"LOWER(target_id) LIKE ? OR LOWER(summary) LIKE ? OR LOWER(actor_id) LIKE ?",
			pattern,
			pattern,
			pattern,
		)
	}
	return query
}

func applyAuditViewFilter(query *gorm.DB, view string) *gorm.DB {
	return query
}

func auditOrderClause(sortBy, sortOrder string) string {
	column := "created_at"
	switch sortBy {
	case "id", "action", "target_type", "target_id", "actor_id":
		column = sortBy
	}
	order := "DESC"
	if sortOrder == "asc" {
		order = "ASC"
	}
	if column == "id" || column == "created_at" {
		return column + " " + order + ", id " + order
	}
	return "LOWER(" + column + ") " + order + ", id " + order
}

func calculateAuditTotalPages(total int64, pageSize int) int {
	if pageSize <= 0 {
		return 1
	}
	pages := int(total) / pageSize
	if int(total)%pageSize > 0 {
		pages++
	}
	if pages < 1 {
		return 1
	}
	return pages
}

func countAuditDistinct(query *gorm.DB, column string) (int64, error) {
	var count int64
	err := query.
		Where(column + " IS NOT NULL AND " + column + " <> ''").
		Distinct(column).
		Count(&count).Error
	return count, err
}

func distinctAuditValues(query *gorm.DB, column string) ([]string, error) {
	var values []string
	err := query.
		Distinct(column).
		Where(column+" IS NOT NULL AND "+column+" <> ''").
		Order(column+" ASC").
		Pluck(column, &values).Error
	return values, err
}
