package store

import (
	"os"
	"strconv"
	"time"

	"gorm.io/gorm"
)

// applyConnPool 给连接池立护栏：database/sql 默认 MaxOpenConns 无上限、
// MaxIdleConns 仅 2——突发流量会无节制向 PG 建连（PG 侧每连接一个进程），
// 平时又频繁建断连，两头都坏。默认值按全体服务副本数对齐 PG max_connections
// 预算（各服务口径一致），可用环境变量覆盖。
func applyConnPool(db *gorm.DB) {
	sqlDB, err := db.DB()
	if err != nil {
		return
	}
	sqlDB.SetMaxOpenConns(poolEnvInt("DB_MAX_OPEN_CONNS", 10))
	sqlDB.SetMaxIdleConns(poolEnvInt("DB_MAX_IDLE_CONNS", 5))
	sqlDB.SetConnMaxLifetime(time.Duration(poolEnvInt("DB_CONN_MAX_LIFETIME_SECONDS", 300)) * time.Second)
	sqlDB.SetConnMaxIdleTime(time.Duration(poolEnvInt("DB_CONN_MAX_IDLE_TIME_SECONDS", 180)) * time.Second)
}

func poolEnvInt(key string, fallback int) int {
	v, err := strconv.Atoi(os.Getenv(key))
	if err != nil || v <= 0 {
		return fallback
	}
	return v
}
