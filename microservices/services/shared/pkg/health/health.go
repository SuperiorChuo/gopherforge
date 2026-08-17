// Package health provides a shared, dependency-injected health check API.
// Services inject their *gorm.DB / *redis.Client; no service-specific
// imports leak into this package.
package health

import (
	"context"
	"database/sql"
	"net/http"
	"reflect"
	"runtime"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/go-admin-kit/services/shared/pkg/response"
	goredis "github.com/redis/go-redis/v9"
)

const dependencyUnavailableMessage = "unavailable"

// RedisPingClient is the Redis command subset used for health checks.
type RedisPingClient interface {
	Ping(ctx context.Context) *goredis.StatusCmd
}

// DatabaseClient is the database command subset used for health checks.
// *gorm.DB satisfies this interface.
type DatabaseClient interface {
	DB() (*sql.DB, error)
}

// API exposes /health, /health/live, /health/ready and /health/check.
type API struct {
	databaseClient DatabaseClient
	redisClient    RedisPingClient
}

// New creates an API with no clients. CheckDependencies then reports
// database/redis as uninitialized.
func New() *API {
	return &API{}
}

// NewWithRedisClient creates an API with an injected Redis client.
func NewWithRedisClient(client RedisPingClient) *API {
	return &API{redisClient: client}
}

// NewWithDatabaseClient creates an API with an injected database client.
func NewWithDatabaseClient(client DatabaseClient) *API {
	return &API{databaseClient: client}
}

// NewWithClients creates an API with injected database and Redis clients.
// A nil client is treated as uninitialized at check time.
func NewWithClients(databaseClient DatabaseClient, redisClient RedisPingClient) *API {
	return &API{databaseClient: databaseClient, redisClient: redisClient}
}

// Health returns a lightweight health snapshot.
func (a *API) Health(c *gin.Context) {
	response.Success(c, gin.H{
		"status":  "ok",
		"time":    time.Now().Format(time.RFC3339),
		"runtime": runtimeSnapshot(),
	})
}

// Liveness reports whether the process is alive (no dependency probes).
func (a *API) Liveness(c *gin.Context) {
	response.Success(c, gin.H{
		"status":  "alive",
		"time":    time.Now().Format(time.RFC3339),
		"runtime": runtimeSnapshot(),
	})
}

// Readiness reports 503 when any dependency is unreachable.
func (a *API) Readiness(c *gin.Context) {
	h := a.CheckDependencies()
	if h["status"] != "ok" {
		c.JSON(http.StatusServiceUnavailable, response.Response{
			Code:      http.StatusServiceUnavailable,
			Message:   "service unavailable",
			ErrorCode: response.ErrorCodeServiceUnavailable,
			Data:      h,
		})
		return
	}
	response.Success(c, h)
}

// HealthCheck returns dependency details without affecting the HTTP status.
func (a *API) HealthCheck(c *gin.Context) {
	response.Success(c, a.CheckDependencies())
}

// CheckDependencies probes database and Redis. Raw dependency errors are
// replaced with "unavailable" so health endpoints never leak credentials.
func (a *API) CheckDependencies() gin.H {
	health := gin.H{
		"status":    "ok",
		"timestamp": time.Now().Format(time.RFC3339),
		"runtime":   runtimeSnapshot(),
		"services":  gin.H{},
	}
	services := health["services"].(gin.H)

	dbCheck := gin.H{"status": "ok"}
	dbStart := time.Now()
	databaseClient := a.databaseStatusClient()
	if databaseClient == nil {
		dbCheck["status"] = "error"
		dbCheck["error"] = "database not initialized"
		health["status"] = "degraded"
	} else if sqlDB, err := databaseClient.DB(); err == nil {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := sqlDB.PingContext(ctx); err != nil {
			dbCheck["status"] = "error"
			dbCheck["error"] = dependencyUnavailableMessage
			health["status"] = "degraded"
		}
		stats := sqlDB.Stats()
		dbCheck["ping_latency_ms"] = float64(time.Since(dbStart)) / float64(time.Millisecond)
		dbCheck["pool"] = gin.H{
			"open_connections":     stats.OpenConnections,
			"in_use":               stats.InUse,
			"idle":                 stats.Idle,
			"wait_count":           stats.WaitCount,
			"wait_duration_ms":     float64(stats.WaitDuration) / float64(time.Millisecond),
			"max_idle_closed":      stats.MaxIdleClosed,
			"max_idle_time_closed": stats.MaxIdleTimeClosed,
			"max_lifetime_closed":  stats.MaxLifetimeClosed,
		}
	} else {
		dbCheck["status"] = "error"
		dbCheck["error"] = dependencyUnavailableMessage
		health["status"] = "degraded"
	}
	services["database"] = dbCheck

	redisCheck := gin.H{"status": "ok"}
	redisStart := time.Now()
	redisClient := a.redisPingClient()
	if redisClient == nil {
		redisCheck["status"] = "error"
		redisCheck["error"] = "redis not initialized"
		health["status"] = "degraded"
	} else {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := redisClient.Ping(ctx).Err(); err != nil {
			redisCheck["status"] = "error"
			redisCheck["error"] = dependencyUnavailableMessage
			health["status"] = "degraded"
		}
	}
	redisCheck["ping_latency_ms"] = float64(time.Since(redisStart)) / float64(time.Millisecond)
	services["redis"] = redisCheck

	return health
}

func (a *API) databaseStatusClient() DatabaseClient {
	if a != nil && !isNilClient(a.databaseClient) {
		return a.databaseClient
	}
	return nil
}

func (a *API) redisPingClient() RedisPingClient {
	if a != nil && !isNilClient(a.redisClient) {
		return a.redisClient
	}
	return nil
}

func isNilClient(client any) bool {
	if client == nil {
		return true
	}
	value := reflect.ValueOf(client)
	switch value.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		return value.IsNil()
	default:
		return false
	}
}

func runtimeSnapshot() gin.H {
	return gin.H{
		"go_version": runtime.Version(),
		"os":         runtime.GOOS,
		"arch":       runtime.GOARCH,
		"compiler":   runtime.Compiler,
	}
}
