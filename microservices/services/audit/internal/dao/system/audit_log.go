package system

import (
	"context"
	"strings"
	"time"

	"github.com/go-admin-kit/services/audit/internal/model"
	"github.com/go-admin-kit/services/shared/pkg/tenant"
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
	return d.db.WithContext(ctx)
}

type AuditLogListQuery struct {
	Page       int
	PageSize   int
	Action     string
	TargetType string
	TargetID   string
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

func (d *AuditLogDAO) CreateLogContext(ctx context.Context, log *model.AuditLog) error {
	if log != nil {
		log.TenantID = tenant.EnsureID(ctx, log.TenantID)
	}
	return d.dbWithContext(ctx).Create(log).Error
}

func (d *AuditLogDAO) ListLogsContext(ctx context.Context, req AuditLogListQuery) (AuditLogListResult, error) {
	if ctx == nil {
		ctx = context.Background()
	}

	var result AuditLogListResult
	baseQuery := applyAuditBaseFilters(
		tenant.ApplyFilter(d.dbWithContext(ctx).Model(&model.AuditLog{}), ctx),
		req,
	)
	listQuery := baseQuery.Session(&gorm.Session{})

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

	// The list and the summary count the same filtered set, so the count above is
	// reused instead of issuing a second, byte-identical COUNT over the tenant's
	// whole audit_logs table. Should the list ever gain a filter the summary does
	// not share, BuildSummary must go back to counting for itself.
	summary, err := d.BuildSummary(baseQuery.Session(&gorm.Session{}), result.Pagination.Total)
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

// maxExportRows bounds an export so a runaway filter can't dump the whole
// table; the export handler streams rows as they are returned.
const maxExportRows = 50000

// ExportLogsContext returns up to maxExportRows audit rows honouring the same
// filters as the list, without pagination. Audit logs are the compliance
// surface — written tenant-scoped by the plugin and read across tenants by
// platform admins, so no department data-scope is applied here (unlike
// operation-log export, whose rows map to users).
func (d *AuditLogDAO) ExportLogsContext(ctx context.Context, req AuditLogListQuery) ([]model.AuditLog, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	query := applyAuditBaseFilters(
		tenant.ApplyFilter(d.dbWithContext(ctx).Model(&model.AuditLog{}), ctx),
		req,
	)
	var logs []model.AuditLog
	if err := query.
		Order(auditOrderClause(req.SortBy, req.SortOrder)).
		Limit(maxExportRows).
		Find(&logs).Error; err != nil {
		return nil, err
	}
	return logs, nil
}

func (d *AuditLogDAO) BuildSummary(baseQuery *gorm.DB, totalLogs int64) (AuditLogSummary, error) {
	summary := AuditLogSummary{TotalLogs: totalLogs}

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

// ActorActionCount is one actor's write/action count within a window, used by
// the security event detector.
type ActorActionCount struct {
	ActorID string
	Count   int64
}

// CountActorActionsWithinContext counts audit rows per actor within a time
// window, optionally filtered by an action predicate. SQL-side filtering keeps
// the scan cheap; the detector decides thresholds and rules in Go.
func (d *AuditLogDAO) CountActorActionsWithinContext(
	ctx context.Context,
	from, to time.Time,
	actionPredicate string,
	args ...any,
) ([]ActorActionCount, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	q := d.dbWithContext(ctx).
		Model(&model.AuditLog{}).
		Where("created_at >= ? AND created_at <= ?", from, to)
	if actionPredicate != "" {
		q = q.Where(actionPredicate, args...)
	}
	var rows []ActorActionCount
	if err := q.
		Select("actor_id, COUNT(*) AS count").
		Group("actor_id").
		Having("COUNT(*) > 0").
		Scan(&rows).Error; err != nil {
		return nil, err
	}
	return rows, nil
}
