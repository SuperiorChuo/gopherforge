// Package connpool 提供连接池护栏：database/sql 默认 MaxOpenConns 无上限、
// MaxIdleConns 仅 2——突发流量会无节制向 PG 建连（PG 侧每连接一个进程），
// 平时又频繁建断连，两头都坏。默认值按全体服务副本数对齐 PG max_connections
// 预算（各服务口径一致），可用环境变量覆盖。
package connpool

import (
	"os"
	"strconv"
	"time"

	"gorm.io/gorm"
)

// DefaultMaxOpenConns 与 DefaultMaxIdleConns 按 16 微服务全栈副本总数与 PG
// max_connections=300 对齐（预留 start-first 滚动更新瞬时双倍副本与迁移裕量）。
const (
	DefaultMaxOpenConns = 10
	DefaultMaxIdleConns = 5
)

// Apply 给连接池立护栏。默认 10/5，高流量服务可用
// DB_MAX_OPEN_CONNS / DB_MAX_IDLE_CONNS 环境变量按需放大。
func Apply(db *gorm.DB) {
	sqlDB, err := db.DB()
	if err != nil {
		return
	}
	sqlDB.SetMaxOpenConns(envInt("DB_MAX_OPEN_CONNS", DefaultMaxOpenConns))
	sqlDB.SetMaxIdleConns(envInt("DB_MAX_IDLE_CONNS", DefaultMaxIdleConns))
	sqlDB.SetConnMaxLifetime(time.Duration(envInt("DB_CONN_MAX_LIFETIME_SECONDS", 300)) * time.Second)
	sqlDB.SetConnMaxIdleTime(time.Duration(envInt("DB_CONN_MAX_IDLE_TIME_SECONDS", 180)) * time.Second)
}

func envInt(key string, fallback int) int {
	v, err := strconv.Atoi(os.Getenv(key))
	if err != nil || v <= 0 {
		return fallback
	}
	return v
}
