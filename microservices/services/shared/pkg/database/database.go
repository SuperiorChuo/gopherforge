package database

import (
	"database/sql"
	"fmt"
	"time"

	"github.com/go-admin-kit/services/shared/pkg/logger"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

var (
	DB *gorm.DB
)

// Config describes Postgres connection settings for infrastructure services.
// DSN is supplied by the caller so service-specific extras such as search_path
// stay in each service config.
type Config struct {
	DSN                    string
	Host                   string
	Port                   int
	DBName                 string
	MaxIdleConns           int
	MaxOpenConns           int
	ConnMaxLifetimeSeconds int
	ConnMaxIdleTimeSeconds int
}

// InitDatabase initializes the process-wide GORM client.
func InitDatabase(cfg Config) error {
	// PrepareStmt caches prepared statements per connection, saving the
	// parse/plan round-trip on every query. SkipDefaultTransaction is left
	// off on purpose: it would drop the implicit transaction around
	// association writes, which is a semantic change.
	db, err := gorm.Open(postgres.Open(cfg.DSN), &gorm.Config{PrepareStmt: true})
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

func applyConnectionPoolConfig(sqlDB *sql.DB, cfg Config) {
	sqlDB.SetMaxIdleConns(cfg.MaxIdleConns)
	sqlDB.SetMaxOpenConns(cfg.MaxOpenConns)
	sqlDB.SetConnMaxLifetime(effectiveDuration(cfg.ConnMaxLifetimeSeconds, 5*time.Minute))
	sqlDB.SetConnMaxIdleTime(effectiveDuration(cfg.ConnMaxIdleTimeSeconds, 3*time.Minute))
}

func effectiveDuration(seconds int, fallback time.Duration) time.Duration {
	if seconds <= 0 {
		return fallback
	}
	return time.Duration(seconds) * time.Second
}
