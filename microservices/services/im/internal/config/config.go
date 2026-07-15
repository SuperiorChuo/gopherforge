package config

import (
	"os"
	"strconv"
	"strings"
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
	CORSOrigins []string
}

func Load() Config {
	return Config{
		AppPort:     getenv("APP_PORT", "8088"),
		AppEnv:      getenv("APP_ENV", "development"),
		DBHost:      getenv("DB_HOST", "127.0.0.1"),
		DBPort:      getenv("DB_PORT", "5432"),
		DBUser:      getenv("DB_USER", "postgres"),
		DBPassword:  getenv("DB_PASSWORD", "123456"),
		DBName:      getenv("DB_NAME", "go_admin_kit"),
		DBSSLMode:   getenv("DB_SSLMODE", "disable"),
		JWTSecret:   getenv("JWT_SECRET", "local-dev-secret-change-me-32-chars"),
		CORSOrigins: splitCSV(getenv("CORS_ALLOW_ORIGINS", "http://localhost:8000,http://localhost:3000,http://127.0.0.1:3000")),
	}
}

func (c Config) DSN() string {
	return "host=" + c.DBHost +
		" user=" + c.DBUser +
		" password=" + c.DBPassword +
		" dbname=" + c.DBName +
		" port=" + c.DBPort +
		" sslmode=" + c.DBSSLMode +
		" TimeZone=Asia/Shanghai"
}

func getenv(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}

func splitCSV(s string) []string {
	parts := strings.Split(s, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}

func EnvInt(k string, def int) int {
	v := os.Getenv(k)
	if v == "" {
		return def
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return def
	}
	return n
}
