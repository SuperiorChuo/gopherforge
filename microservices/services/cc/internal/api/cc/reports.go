package api

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
)

// 呼叫报表：GET /api/v1/cc/reports/overview?from=&to=（默认今天）。一个端点全返回
// （总话务/接通率/均长/放弃/时段分布/坐席榜/队列榜），数据源纯 cc_calls 聚合。

// parseDay 解析 YYYY-MM-DD（本地时区），失败返回 zero。
func parseDay(s string) (time.Time, bool) {
	if s == "" {
		return time.Time{}, false
	}
	t, err := time.ParseInLocation("2006-01-02", s, time.Local)
	if err != nil {
		return time.Time{}, false
	}
	return t, true
}

// ReportsOverview handles GET /api/v1/cc/reports/overview.
func (s *Server) ReportsOverview(c *gin.Context) {
	agent, authorized := s.requireAgent(c)
	if !authorized {
		return
	}
	now := time.Now()
	// 默认今天 [00:00, 明天 00:00)
	from := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.Local)
	to := from.AddDate(0, 0, 1)
	if d, ok := parseDay(c.Query("from")); ok {
		from = d
	}
	if d, ok := parseDay(c.Query("to")); ok {
		// to 是含当天的截止日：右开区间取 to 的次日 00:00
		to = d.AddDate(0, 0, 1)
	}
	if !to.After(from) {
		to = from.AddDate(0, 0, 1)
	}
	report, err := s.Store.ReportOverview(c.Request.Context(), agent.TenantID, from, to)
	if err != nil {
		fail(c, http.StatusInternalServerError, "build report failed")
		return
	}
	ok(c, report)
}
