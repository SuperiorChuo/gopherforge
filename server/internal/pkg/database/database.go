package database

import (
	"database/sql"
	"fmt"

	"github.com/go-admin-kit/server/internal/config"
	"github.com/go-admin-kit/server/internal/pkg/logger"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
)

var (
	DB *gorm.DB
)

// InitDatabase initializes the database connection.
func InitDatabase() error {
	cfg := config.Cfg.Database

	dsn := cfg.GetDSN()

	db, err := gorm.Open(mysql.Open(dsn), &gorm.Config{})
	if err != nil {
		return fmt.Errorf("failed to connect database: %w", err)
	}

	sqlDB, err := db.DB()
	if err != nil {
		return fmt.Errorf("failed to get database instance: %w", err)
	}

	applyConnectionPoolConfig(sqlDB, cfg)

	DB = db
	logger.Info("database connected",
		logger.String("host", fmt.Sprintf("%s:%d", cfg.Host, cfg.Port)),
		logger.String("database", cfg.DBName),
	)

	return nil
}

func Close() error {
	if DB == nil {
		return nil
	}
	sqlDB, err := DB.DB()
	if err != nil {
		return err
	}
	DB = nil
	return sqlDB.Close()
}

func applyConnectionPoolConfig(sqlDB *sql.DB, cfg config.DatabaseConfig) {
	sqlDB.SetMaxIdleConns(cfg.MaxIdleConns)
	sqlDB.SetMaxOpenConns(cfg.MaxOpenConns)
	sqlDB.SetConnMaxLifetime(cfg.EffectiveConnMaxLifetime())
	sqlDB.SetConnMaxIdleTime(cfg.EffectiveConnMaxIdleTime())
}
