package monitor

import (
	"context"
	"database/sql"
	"errors"

	"github.com/go-admin-kit/server/internal/pkg/database"
	"gorm.io/gorm"
)

type MySQLDAO struct {
	db *gorm.DB
}

type MySQLNameValue struct {
	VariableName string `gorm:"column:Variable_name"`
	Value        string `gorm:"column:Value"`
}

type MySQLTableStats struct {
	TableCount   int64 `gorm:"column:table_count"`
	DatabaseSize int64 `gorm:"column:database_size"`
}

func NewMySQLDAO(dbs ...*gorm.DB) *MySQLDAO {
	db := database.DB
	if len(dbs) > 0 {
		db = dbs[0]
	}
	return &MySQLDAO{db: db}
}

func (d *MySQLDAO) dbWithContext(ctx context.Context) *gorm.DB {
	if ctx == nil {
		ctx = context.Background()
	}
	if d != nil && d.db != nil {
		return d.db.WithContext(ctx)
	}
	return database.DB.WithContext(ctx)
}

func (d *MySQLDAO) ConnectionStats() (sql.DBStats, error) {
	return d.ConnectionStatsContext(context.Background())
}

func (d *MySQLDAO) ConnectionStatsContext(ctx context.Context) (sql.DBStats, error) {
	if err := ctx.Err(); err != nil {
		return sql.DBStats{}, err
	}
	db := database.DB
	if d != nil && d.db != nil {
		db = d.db
	}
	if db == nil {
		return sql.DBStats{}, errors.New("database is not initialized")
	}
	sqlDB, err := db.DB()
	if err != nil {
		return sql.DBStats{}, err
	}
	return sqlDB.Stats(), nil
}

func (d *MySQLDAO) GetVersion() (string, error) {
	return d.GetVersionContext(context.Background())
}

func (d *MySQLDAO) GetVersionContext(ctx context.Context) (string, error) {
	var version string
	err := d.dbWithContext(ctx).Raw("SELECT VERSION()").Scan(&version).Error
	return version, err
}

func (d *MySQLDAO) GetCurrentDatabase() (string, error) {
	return d.GetCurrentDatabaseContext(context.Background())
}

func (d *MySQLDAO) GetCurrentDatabaseContext(ctx context.Context) (string, error) {
	var currentDatabase string
	err := d.dbWithContext(ctx).Raw("SELECT DATABASE()").Scan(&currentDatabase).Error
	return currentDatabase, err
}

func (d *MySQLDAO) GetNameValues(query string) (map[string]string, error) {
	return d.GetNameValuesContext(context.Background(), query)
}

func (d *MySQLDAO) GetNameValuesContext(ctx context.Context, query string) (map[string]string, error) {
	var rows []MySQLNameValue
	if err := d.dbWithContext(ctx).Raw(query).Scan(&rows).Error; err != nil {
		return nil, err
	}

	result := make(map[string]string, len(rows))
	for _, row := range rows {
		result[row.VariableName] = row.Value
	}
	return result, nil
}

func (d *MySQLDAO) GetTableStats(dbName string) (MySQLTableStats, error) {
	return d.GetTableStatsContext(context.Background(), dbName)
}

func (d *MySQLDAO) GetTableStatsContext(ctx context.Context, dbName string) (MySQLTableStats, error) {
	var stats MySQLTableStats
	err := d.dbWithContext(ctx).Raw(
		`SELECT COUNT(*) AS table_count, COALESCE(SUM(data_length + index_length), 0) AS database_size
		 FROM information_schema.tables
		 WHERE table_schema = ?`,
		dbName,
	).Scan(&stats).Error
	return stats, err
}
