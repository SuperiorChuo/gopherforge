package monitor

import (
	"context"
	"database/sql"
	"errors"

	"gorm.io/gorm"
)

// MySQLDAO reads PostgreSQL server statistics. The historical name is kept
// so the /monitor/mysql API surface stays stable for existing clients.
type MySQLDAO struct {
	db *gorm.DB
}

type MySQLTableStats struct {
	TableCount   int64 `gorm:"column:table_count"`
	DatabaseSize int64 `gorm:"column:database_size"`
}

type MySQLServerStats struct {
	UptimeSeconds     int64 `gorm:"column:uptime_seconds"`
	Connections       int64 `gorm:"column:connections"`
	ActiveConnections int64 `gorm:"column:active_connections"`
	MaxConnections    int64 `gorm:"column:max_connections"`
	Commits           int64 `gorm:"column:commits"`
	Rollbacks         int64 `gorm:"column:rollbacks"`
	RowsReturned      int64 `gorm:"column:rows_returned"`
	RowsInserted      int64 `gorm:"column:rows_inserted"`
	RowsUpdated       int64 `gorm:"column:rows_updated"`
	RowsDeleted       int64 `gorm:"column:rows_deleted"`
	BlocksRead        int64 `gorm:"column:blocks_read"`
	BlocksHit         int64 `gorm:"column:blocks_hit"`
	TempBytes         int64 `gorm:"column:temp_bytes"`
}

func NewMySQLDAO(db *gorm.DB) *MySQLDAO {
	return &MySQLDAO{db: db}
}

func (d *MySQLDAO) dbWithContext(ctx context.Context) *gorm.DB {
	if ctx == nil {
		ctx = context.Background()
	}
	return d.db.WithContext(ctx)
}

func (d *MySQLDAO) ConnectionStatsContext(ctx context.Context) (sql.DBStats, error) {
	if err := ctx.Err(); err != nil {
		return sql.DBStats{}, err
	}
	if d == nil || d.db == nil {
		return sql.DBStats{}, errors.New("database is not initialized")
	}
	sqlDB, err := d.db.DB()
	if err != nil {
		return sql.DBStats{}, err
	}
	return sqlDB.Stats(), nil
}

func (d *MySQLDAO) GetVersionContext(ctx context.Context) (string, error) {
	var version string
	err := d.dbWithContext(ctx).Raw("SHOW server_version").Scan(&version).Error
	return version, err
}

func (d *MySQLDAO) GetCurrentDatabaseContext(ctx context.Context) (string, error) {
	var currentDatabase string
	err := d.dbWithContext(ctx).Raw("SELECT current_database()").Scan(&currentDatabase).Error
	return currentDatabase, err
}

func (d *MySQLDAO) GetServerStatsContext(ctx context.Context) (MySQLServerStats, error) {
	var stats MySQLServerStats
	err := d.dbWithContext(ctx).Raw(
		`SELECT
		   EXTRACT(EPOCH FROM (now() - pg_postmaster_start_time()))::bigint AS uptime_seconds,
		   (SELECT COUNT(*) FROM pg_stat_activity) AS connections,
		   (SELECT COUNT(*) FROM pg_stat_activity WHERE state = 'active') AS active_connections,
		   current_setting('max_connections')::bigint AS max_connections,
		   s.xact_commit AS commits,
		   s.xact_rollback AS rollbacks,
		   s.tup_returned AS rows_returned,
		   s.tup_inserted AS rows_inserted,
		   s.tup_updated AS rows_updated,
		   s.tup_deleted AS rows_deleted,
		   s.blks_read AS blocks_read,
		   s.blks_hit AS blocks_hit,
		   s.temp_bytes AS temp_bytes
		 FROM pg_stat_database s
		 WHERE s.datname = current_database()`,
	).Scan(&stats).Error
	return stats, err
}

func (d *MySQLDAO) GetTableStatsContext(ctx context.Context, dbName string) (MySQLTableStats, error) {
	var stats MySQLTableStats
	err := d.dbWithContext(ctx).Raw(
		`SELECT
		   (SELECT COUNT(*) FROM information_schema.tables
		    WHERE table_catalog = ? AND table_schema NOT IN ('pg_catalog', 'information_schema')) AS table_count,
		   pg_database_size(?) AS database_size`,
		dbName, dbName,
	).Scan(&stats).Error
	return stats, err
}
