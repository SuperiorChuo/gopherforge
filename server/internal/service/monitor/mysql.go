package monitor

import (
	"context"
	"database/sql"
	"fmt"
	"strconv"

	"github.com/go-admin-kit/server/internal/config"
	monitordao "github.com/go-admin-kit/server/internal/dao/monitor"
)

type mySQLDAO interface {
	ConnectionStatsContext(ctx context.Context) (sql.DBStats, error)
	GetVersionContext(ctx context.Context) (string, error)
	GetCurrentDatabaseContext(ctx context.Context) (string, error)
	GetNameValuesContext(ctx context.Context, query string) (map[string]string, error)
	GetTableStatsContext(ctx context.Context, dbName string) (monitordao.MySQLTableStats, error)
}

type MySQLService struct {
	dao mySQLDAO
}

func NewMySQLService() *MySQLService {
	return &MySQLService{dao: monitordao.NewMySQLDAO()}
}

// GetMySQLInfo returns MySQL information.
// Deprecated: use GetMySQLInfoContext instead.
func (s *MySQLService) GetMySQLInfo() (map[string]any, error) {
	return s.GetMySQLInfoContext(context.Background())
}

func (s *MySQLService) GetMySQLInfoContext(ctx context.Context) (map[string]any, error) {
	stats, err := s.dao.ConnectionStatsContext(ctx)
	if err != nil {
		return nil, err
	}

	// Load version and runtime information.
	version, err := s.dao.GetVersionContext(ctx)
	if err != nil {
		return nil, err
	}

	currentDatabase, err := s.dao.GetCurrentDatabaseContext(ctx)
	if err != nil {
		return nil, err
	}

	status, err := s.dao.GetNameValuesContext(ctx, "SHOW GLOBAL STATUS")
	if err != nil {
		return nil, err
	}
	variables, err := s.dao.GetNameValuesContext(ctx, "SHOW GLOBAL VARIABLES")
	if err != nil {
		return nil, err
	}

	tableStats, err := s.dao.GetTableStatsContext(ctx, config.Cfg.Database.DBName)
	if err != nil {
		return nil, err
	}

	uptimeSeconds := parseInt64(status["Uptime"])
	questions := parseInt64(status["Questions"])

	data := make(map[string]any)
	data["status"] = "ok"
	data["version"] = version
	data["uptime"] = fmt.Sprintf("%d", uptimeSeconds)
	data["uptime_seconds"] = uptimeSeconds

	data["database"] = map[string]any{
		"host":        config.Cfg.Database.Host,
		"port":        config.Cfg.Database.Port,
		"name":        firstNonEmpty(currentDatabase, config.Cfg.Database.DBName),
		"charset":     variables["character_set_database"],
		"collation":   variables["collation_database"],
		"table_count": tableStats.TableCount,
		"size_bytes":  tableStats.DatabaseSize,
		"size":        formatBytes(tableStats.DatabaseSize),
	}

	data["connections"] = map[string]any{
		"max_open_conns":       stats.MaxOpenConnections,
		"open_conns":           stats.OpenConnections,
		"in_use":               stats.InUse,
		"idle":                 stats.Idle,
		"wait_count":           stats.WaitCount,
		"wait_duration":        stats.WaitDuration.String(),
		"threads_connected":    parseInt64(status["Threads_connected"]),
		"threads_running":      parseInt64(status["Threads_running"]),
		"max_connections":      parseInt64(variables["max_connections"]),
		"max_used_connections": parseInt64(status["Max_used_connections"]),
		"total_connections":    parseInt64(status["Connections"]),
	}

	data["queries"] = map[string]any{
		"questions":    questions,
		"qps":          calculateRate(questions, uptimeSeconds),
		"slow_queries": parseInt64(status["Slow_queries"]),
		"selects":      parseInt64(status["Com_select"]),
		"inserts":      parseInt64(status["Com_insert"]),
		"updates":      parseInt64(status["Com_update"]),
		"deletes":      parseInt64(status["Com_delete"]),
	}

	bytesReceived := parseInt64(status["Bytes_received"])
	bytesSent := parseInt64(status["Bytes_sent"])
	data["traffic"] = map[string]any{
		"bytes_received":       bytesReceived,
		"bytes_sent":           bytesSent,
		"bytes_received_human": formatBytes(bytesReceived),
		"bytes_sent_human":     formatBytes(bytesSent),
	}

	return data, nil
}

func parseInt64(value string) int64 {
	if value == "" {
		return 0
	}
	parsed, err := strconv.ParseInt(value, 10, 64)
	if err != nil {
		return 0
	}
	return parsed
}

func calculateRate(total, seconds int64) float64 {
	if total <= 0 || seconds <= 0 {
		return 0
	}
	rate := float64(total) / float64(seconds)
	return float64(int(rate*100)) / 100
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}

func formatBytes(bytes int64) string {
	const unit = 1024
	if bytes < unit {
		return fmt.Sprintf("%d B", bytes)
	}
	div, exp := int64(unit), 0
	for n := bytes / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.2f %ciB", float64(bytes)/float64(div), "KMGTPE"[exp])
}
