package system

import (
	"encoding/csv"
	"encoding/json"
	"fmt"
	"time"

	"github.com/gin-gonic/gin"
	service "github.com/go-admin-kit/services/audit/internal/service/system"
	"github.com/go-admin-kit/services/shared/pkg/response"
)

// AuditLogAPI exposes independent business audit logs.
type AuditLogAPI struct {
	logService service.AuditLogService
}

func NewAuditLogAPI() *AuditLogAPI {
	return &AuditLogAPI{
		logService: service.AuditLogService{},
	}
}

// NewAuditLogAPIWithService creates an AuditLogAPI instance from an injected service.
func NewAuditLogAPIWithService(logService service.AuditLogService) *AuditLogAPI {
	return &AuditLogAPI{logService: logService}
}

func (a *AuditLogAPI) GetAuditLogs(c *gin.Context) {
	var req service.AuditLogListRequest
	if err := c.ShouldBindQuery(&req); err != nil {
		response.BadRequest(c, "invalid query parameters")
		return
	}

	result, err := a.logService.ListLogsContext(c.Request.Context(), req)
	if err != nil {
		internalServerError(c, "failed to list audit logs", err)
		return
	}

	response.Success(c, result)
}

// ExportAuditLogs streams audit rows as CSV with the same filters as the list.
// before/after JSON snapshots are marshalled into single columns. Audit logs
// are the compliance surface — no department data-scope is applied (platform
// admins read the full tenant set, matching how the plugin writes rows).
func (a *AuditLogAPI) ExportAuditLogs(c *gin.Context) {
	var req service.AuditLogListRequest
	if err := c.ShouldBindQuery(&req); err != nil {
		response.BadRequest(c, "invalid query parameters")
		return
	}

	logs, err := a.logService.ExportLogsContext(c.Request.Context(), req)
	if err != nil {
		internalServerError(c, "failed to export audit logs", err)
		return
	}

	filename := fmt.Sprintf("audit_logs_%s.csv", time.Now().Format("20060102150405"))
	c.Header("Content-Type", "text/csv; charset=utf-8")
	c.Header("Content-Disposition", fmt.Sprintf("attachment; filename=%s", filename))

	// UTF-8 BOM: Excel on Windows otherwise reads Chinese columns as mojibake.
	_, _ = c.Writer.WriteString("\xEF\xBB\xBF")

	writer := csv.NewWriter(c.Writer)
	defer writer.Flush()

	_ = writer.Write([]string{"操作者", "动作", "目标类型", "目标 ID", "变更前", "变更后", "摘要", "时间"})
	for _, log := range logs {
		before := snapshotJSON(log.BeforeJSON)
		after := snapshotJSON(log.AfterJSON)
		_ = writer.Write([]string{
			log.ActorID,
			log.Action,
			log.TargetType,
			log.TargetID,
			before,
			after,
			log.Summary,
			log.CreatedAt.Format("2006-01-02 15:04:05"),
		})
	}
}

func snapshotJSON(v map[string]any) string {
	if len(v) == 0 {
		return ""
	}
	raw, err := json.Marshal(v)
	if err != nil {
		return fmt.Sprintf("%v", v)
	}
	return string(raw)
}
