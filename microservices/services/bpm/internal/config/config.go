// Package config 提供 bpm-service 的纯环境变量配置（轻量服务约定，
// 不携带核心服务的完整配置框架）。
package config

import (
	"fmt"
	"os"
	"strings"
	"time"
)

type Config struct {
	AppPort    string
	AppEnv     string
	DBHost     string
	DBPort     string
	DBUser     string
	DBPassword string
	DBName     string
	DBSSLMode  string
	JWTSecret  string
	// InternalToken 校验业务方内网调用 bpm internal 端点的
	// X-Internal-Token；空=禁用 internal 端点（返回 503），不裸奔。
	InternalToken string
	// CallbackToken 终态回调业务方时携带的 X-Internal-Token（业务侧内部
	// 端点鉴权）；空=回调不带鉴权头。
	CallbackToken string
	// NotifyAPIBase + NotifyInternalToken：新待办/抄送/终态站内信；
	// token 空=静默跳过通知（不阻断审批主流程）。
	NotifyAPIBase       string
	NotifyInternalToken string
	// TimeoutScanInterval 超时提醒扫描周期（BPM_TIMEOUT_SCAN_INTERVAL，
	// time.ParseDuration 语法，默认 5m）。
	TimeoutScanInterval time.Duration
}

func Load() Config {
	return Config{
		AppPort:             getenv("APP_PORT", "8096"),
		AppEnv:              getenv("APP_ENV", "development"),
		DBHost:              getenv("DB_HOST", "127.0.0.1"),
		DBPort:              getenv("DB_PORT", "5432"),
		DBUser:              getenv("DB_USER", "postgres"),
		DBPassword:          getenv("DB_PASSWORD", "123456"),
		DBName:              getenv("DB_NAME", "go_admin_kit"),
		DBSSLMode:           getenv("DB_SSLMODE", "disable"),
		JWTSecret:           getenv("JWT_SECRET", "local-dev-secret-change-me-32-chars"),
		InternalToken:       getenv("BPM_INTERNAL_TOKEN", ""),
		CallbackToken:       getenv("BPM_CALLBACK_TOKEN", ""),
		NotifyAPIBase:       getenv("NOTIFY_API_BASE", ""),
		NotifyInternalToken: getenv("NOTIFY_INTERNAL_TOKEN", ""),
		TimeoutScanInterval: getenvDuration("BPM_TIMEOUT_SCAN_INTERVAL", 5*time.Minute),
	}
}

func getenvDuration(key string, fallback time.Duration) time.Duration {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		if d, err := time.ParseDuration(v); err == nil && d > 0 {
			return d
		}
	}
	return fallback
}

func (c Config) DSN() string {
	return fmt.Sprintf("host=%s port=%s user=%s password=%s dbname=%s sslmode=%s TimeZone=Asia/Shanghai",
		c.DBHost, c.DBPort, c.DBUser, c.DBPassword, c.DBName, c.DBSSLMode)
}

func getenv(key, fallback string) string {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		return v
	}
	return fallback
}
