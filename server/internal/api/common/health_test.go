package common

import (
	"context"
	"errors"
	"net"
	"strings"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/gin-gonic/gin"
	"github.com/go-admin-kit/server/internal/pkg/database"
	internalredis "github.com/go-admin-kit/server/internal/pkg/redis"
	goredis "github.com/redis/go-redis/v9"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
)

func TestCheckDependenciesDoesNotExposeDatabasePingError(t *testing.T) {
	rawErr := "dial tcp 10.9.8.7:3306: access denied for user root with password topsecret"
	setupHealthTestRedisNil(t)
	setupHealthTestDBPingError(t, errors.New(rawErr))

	health := NewHealthAPI().checkDependencies()

	if health["status"] != "degraded" {
		t.Fatalf("status = %v, want degraded", health["status"])
	}
	databaseCheck := healthService(t, health, "database")
	if databaseCheck["status"] != "error" {
		t.Fatalf("database status = %v, want error", databaseCheck["status"])
	}
	if databaseCheck["error"] != "unavailable" {
		t.Fatalf("database error = %v, want unavailable", databaseCheck["error"])
	}
	if strings.Contains(toString(databaseCheck["error"]), rawErr) {
		t.Fatalf("database error exposes raw dependency error: %v", databaseCheck["error"])
	}
	if _, ok := databaseCheck["ping_latency_ms"]; !ok {
		t.Fatal("database ping latency is missing")
	}
	if _, ok := databaseCheck["pool"]; !ok {
		t.Fatal("database pool stats are missing")
	}
}

func TestCheckDependenciesDoesNotExposeRedisPingError(t *testing.T) {
	rawErr := "dial tcp redis.internal:6379: auth token topsecret rejected"
	setupHealthTestDBNil(t)
	setupHealthTestRedisPingError(t, errors.New(rawErr))

	health := NewHealthAPI().checkDependencies()

	if health["status"] != "degraded" {
		t.Fatalf("status = %v, want degraded", health["status"])
	}
	redisCheck := healthService(t, health, "redis")
	if redisCheck["status"] != "error" {
		t.Fatalf("redis status = %v, want error", redisCheck["status"])
	}
	if redisCheck["error"] != "unavailable" {
		t.Fatalf("redis error = %v, want unavailable", redisCheck["error"])
	}
	if strings.Contains(toString(redisCheck["error"]), rawErr) {
		t.Fatalf("redis error exposes raw dependency error: %v", redisCheck["error"])
	}
	if _, ok := redisCheck["ping_latency_ms"]; !ok {
		t.Fatal("redis ping latency is missing")
	}
}

func setupHealthTestDBPingError(t *testing.T, pingErr error) {
	t.Helper()

	oldDB := database.DB
	sqlDB, mock, err := sqlmock.New(sqlmock.MonitorPingsOption(true))
	if err != nil {
		t.Fatalf("open sqlmock db: %v", err)
	}
	mock.ExpectPing().WillReturnError(pingErr)

	db, err := gorm.Open(mysql.New(mysql.Config{
		Conn:                      sqlDB,
		SkipInitializeWithVersion: true,
	}), &gorm.Config{DisableAutomaticPing: true})
	if err != nil {
		t.Fatalf("open gorm sqlmock db: %v", err)
	}
	database.DB = db

	t.Cleanup(func() {
		if err := mock.ExpectationsWereMet(); err != nil {
			t.Fatalf("unmet database expectations: %v", err)
		}
		_ = sqlDB.Close()
		database.DB = oldDB
	})
}

func setupHealthTestDBNil(t *testing.T) {
	t.Helper()

	oldDB := database.DB
	database.DB = nil
	t.Cleanup(func() {
		database.DB = oldDB
	})
}

func setupHealthTestRedisPingError(t *testing.T, pingErr error) {
	t.Helper()

	oldClient := internalredis.Client
	client := goredis.NewClient(&goredis.Options{
		Addr: "redis.internal:6379",
		Dialer: func(context.Context, string, string) (net.Conn, error) {
			return nil, pingErr
		},
		MaxRetries:    -1,
		DialerRetries: 1,
	})
	internalredis.Client = client

	t.Cleanup(func() {
		_ = client.Close()
		internalredis.Client = oldClient
	})
}

func setupHealthTestRedisNil(t *testing.T) {
	t.Helper()

	oldClient := internalredis.Client
	internalredis.Client = nil
	t.Cleanup(func() {
		internalredis.Client = oldClient
	})
}

func healthService(t *testing.T, health gin.H, name string) gin.H {
	t.Helper()

	services, ok := health["services"].(gin.H)
	if !ok {
		t.Fatalf("services = %T, want gin.H", health["services"])
	}
	service, ok := services[name].(gin.H)
	if !ok {
		t.Fatalf("services[%q] = %T, want gin.H", name, services[name])
	}
	return service
}

func toString(value any) string {
	text, _ := value.(string)
	return text
}
