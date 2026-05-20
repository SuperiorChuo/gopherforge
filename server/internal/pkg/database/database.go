package database

import (
	"fmt"

	"github.com/go-admin-kit/server/internal/config"
	"github.com/go-admin-kit/server/internal/pkg/logger"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
)

var (
	DB *gorm.DB
)

// InitDatabase 初始化数据库连接
func InitDatabase() error {
	cfg := config.Cfg.Database

	dsn := cfg.GetDSN()

	// 连接数据库
	db, err := gorm.Open(mysql.Open(dsn), &gorm.Config{
		// 使用默认日志配置
	})
	if err != nil {
		return fmt.Errorf("failed to connect database: %w", err)
	}

	// 获取通用数据库对象 sql.DB 以配置连接池
	sqlDB, err := db.DB()
	if err != nil {
		return fmt.Errorf("failed to get database instance: %w", err)
	}

	// 设置连接池参数
	sqlDB.SetMaxIdleConns(cfg.MaxIdleConns)
	sqlDB.SetMaxOpenConns(cfg.MaxOpenConns)

	DB = db
	logger.Info("✅ 数据库连接成功",
		logger.String("主机", fmt.Sprintf("%s:%d", cfg.Host, cfg.Port)),
		logger.String("数据库", cfg.DBName),
	)

	return nil
}
