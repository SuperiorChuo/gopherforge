package monitor

import (
	"fmt"
	"strconv"

	"github.com/go-admin-kit/server/internal/config"
	"github.com/go-admin-kit/server/internal/pkg/database"
)

type MySQLService struct{}

type mysqlNameValue struct {
	VariableName string `gorm:"column:Variable_name"`
	Value        string `gorm:"column:Value"`
}

func NewMySQLService() *MySQLService {
	return &MySQLService{}
}

// GetMySQLInfo 获取 MySQL 信息
func (s *MySQLService) GetMySQLInfo() (map[string]interface{}, error) {
	db, err := database.DB.DB()
	if err != nil {
		return nil, err
	}

	stats := db.Stats()

	// 获取版本和运行时信息
	var version string
	database.DB.Raw("SELECT VERSION()").Scan(&version)

	var currentDatabase string
	database.DB.Raw("SELECT DATABASE()").Scan(&currentDatabase)

	status := getMySQLNameValues("SHOW GLOBAL STATUS")
	variables := getMySQLNameValues("SHOW GLOBAL VARIABLES")

	var tableStats struct {
		TableCount   int64 `gorm:"column:table_count"`
		DatabaseSize int64 `gorm:"column:database_size"`
	}
	database.DB.Raw(
		`SELECT COUNT(*) AS table_count, COALESCE(SUM(data_length + index_length), 0) AS database_size
		 FROM information_schema.tables
		 WHERE table_schema = ?`,
		config.Cfg.Database.DBName,
	).Scan(&tableStats)

	uptimeSeconds := parseInt64(status["Uptime"])
	questions := parseInt64(status["Questions"])

	data := make(map[string]interface{})
	data["status"] = "ok"
	data["version"] = version
	data["uptime"] = fmt.Sprintf("%d", uptimeSeconds)
	data["uptime_seconds"] = uptimeSeconds

	data["database"] = map[string]interface{}{
		"host":        config.Cfg.Database.Host,
		"port":        config.Cfg.Database.Port,
		"name":        firstNonEmpty(currentDatabase, config.Cfg.Database.DBName),
		"charset":     variables["character_set_database"],
		"collation":   variables["collation_database"],
		"table_count": tableStats.TableCount,
		"size_bytes":  tableStats.DatabaseSize,
		"size":        formatBytes(tableStats.DatabaseSize),
	}

	data["connections"] = map[string]interface{}{
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

	data["queries"] = map[string]interface{}{
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
	data["traffic"] = map[string]interface{}{
		"bytes_received":       bytesReceived,
		"bytes_sent":           bytesSent,
		"bytes_received_human": formatBytes(bytesReceived),
		"bytes_sent_human":     formatBytes(bytesSent),
	}

	return data, nil
}

func getMySQLNameValues(query string) map[string]string {
	var rows []mysqlNameValue
	result := make(map[string]string)
	if err := database.DB.Raw(query).Scan(&rows).Error; err != nil {
		return result
	}
	for _, row := range rows {
		result[row.VariableName] = row.Value
	}
	return result
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
