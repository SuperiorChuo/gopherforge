package monitor

import (
	"context"
	"database/sql"
	"fmt"
	"strconv"

	"github.com/go-admin-kit/server/internal/config"
	monitordao "github.com/go-admin-kit/server/internal/dao/monitor"
	"gorm.io/gorm"
)

type mySQLDAO interface {
	ConnectionStatsContext(ctx context.Context) (sql.DBStats, error)
	GetVersionContext(ctx context.Context) (string, error)
	GetCurrentDatabaseContext(ctx context.Context) (string, error)
	GetServerStatsContext(ctx context.Context) (monitordao.MySQLServerStats, error)
	GetTableStatsContext(ctx context.Context, dbName string) (monitordao.MySQLTableStats, error)
}

// MySQLService reports PostgreSQL server health. The historical name is kept
// so the /monitor/mysql API surface stays stable for existing clients.
type MySQLService struct {
	dao mySQLDAO
}

// NewMySQLServiceWithDB builds a MySQLService backed by an injected database handle.
func NewMySQLServiceWithDB(db *gorm.DB) *MySQLService {
	return &MySQLService{dao: monitordao.NewMySQLDAO(db)}
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

	serverStats, err := s.dao.GetServerStatsContext(ctx)
	if err != nil {
		return nil, err
	}

	tableStats, err := s.dao.GetTableStatsContext(ctx, config.Cfg.Database.DBName)
	if err != nil {
		return nil, err
	}

	uptimeSeconds := serverStats.UptimeSeconds
	questions := serverStats.Commits + serverStats.Rollbacks

	data := make(map[string]any)
	data["status"] = "ok"
	data["version"] = version
	data["uptime"] = fmt.Sprintf("%d", uptimeSeconds)
	data["uptime_seconds"] = uptimeSeconds

	data["database"] = map[string]any{
		"host":        config.Cfg.Database.Host,
		"port":        config.Cfg.Database.Port,
		"name":        firstNonEmpty(currentDatabase, config.Cfg.Database.DBName),
		"charset":     "UTF8",
		"collation":   "",
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
		"threads_connected":    serverStats.Connections,
		"threads_running":      serverStats.ActiveConnections,
		"max_connections":      serverStats.MaxConnections,
		"max_used_connections": serverStats.Connections,
		"total_connections":    serverStats.Connections,
	}

	data["queries"] = map[string]any{
		"questions":    questions,
		"qps":          calculateRate(questions, uptimeSeconds),
		"slow_queries": int64(0),
		"selects":      serverStats.RowsReturned,
		"inserts":      serverStats.RowsInserted,
		"updates":      serverStats.RowsUpdated,
		"deletes":      serverStats.RowsDeleted,
	}

	// PostgreSQL does not expose per-database network byte counters;
	// report buffer I/O so the traffic panel stays meaningful.
	blocksReadBytes := serverStats.BlocksRead * 8192
	blocksHitBytes := serverStats.BlocksHit * 8192
	data["traffic"] = map[string]any{
		"bytes_received":       blocksReadBytes,
		"bytes_sent":           blocksHitBytes,
		"bytes_received_human": formatBytes(blocksReadBytes),
		"bytes_sent_human":     formatBytes(blocksHitBytes),
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
