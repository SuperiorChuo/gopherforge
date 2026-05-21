package common

import (
	"context"
	"database/sql"
	"net/http"
	"reflect"
	"runtime"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/go-admin-kit/server/internal/middleware"
	"github.com/go-admin-kit/server/internal/pkg/database"
	redisstore "github.com/go-admin-kit/server/internal/pkg/redis"
	"github.com/go-admin-kit/server/internal/pkg/response"
	goredis "github.com/redis/go-redis/v9"
)

// RedisPingClient is the Redis command subset used by HealthAPI.
type RedisPingClient interface {
	Ping(ctx context.Context) *goredis.StatusCmd
}

// DatabaseClient is the database command subset used by HealthAPI.
type DatabaseClient interface {
	DB() (*sql.DB, error)
}

// HealthAPI handles health check endpoints.
type HealthAPI struct {
	databaseClient DatabaseClient
	redisClient    RedisPingClient
}

const dependencyUnavailableMessage = "unavailable"

// NewHealthAPI creates a HealthAPI instance.
func NewHealthAPI() *HealthAPI {
	return &HealthAPI{}
}

// NewHealthAPIWithRedisClient creates a HealthAPI with an injected Redis client.
func NewHealthAPIWithRedisClient(client RedisPingClient) *HealthAPI {
	return &HealthAPI{redisClient: client}
}

// NewHealthAPIWithDatabaseClient creates a HealthAPI with an injected database client.
func NewHealthAPIWithDatabaseClient(client DatabaseClient) *HealthAPI {
	return &HealthAPI{databaseClient: client}
}

// Health returns a lightweight health snapshot.
func (a *HealthAPI) Health(c *gin.Context) {
	response.Success(c, gin.H{
		"status":  "ok",
		"time":    time.Now().Format(time.RFC3339),
		"runtime": runtimeSnapshot(),
	})
}

// Liveness reports whether the process is alive.
func (a *HealthAPI) Liveness(c *gin.Context) {
	response.Success(c, gin.H{
		"status":  "alive",
		"time":    time.Now().Format(time.RFC3339),
		"runtime": runtimeSnapshot(),
	})
}

// Readiness reports whether dependencies are ready.
func (a *HealthAPI) Readiness(c *gin.Context) {
	health := a.checkDependencies()
	if health["status"] != "ok" {
		c.JSON(http.StatusServiceUnavailable, response.Response{
			Code:      http.StatusServiceUnavailable,
			Message:   "service unavailable",
			ErrorCode: response.ErrorCodeServiceUnavailable,
			Data:      health,
		})
		return
	}
	response.Success(c, health)
}

// HealthCheck returns dependency health details.
func (a *HealthAPI) HealthCheck(c *gin.Context) {
	response.Success(c, a.checkDependencies())
}

// MetricsSnapshot returns in-process HTTP metrics as JSON.
func (a *HealthAPI) MetricsSnapshot(c *gin.Context) {
	response.Success(c, middleware.MetricsSnapshot())
}

// PrometheusMetrics returns metrics in Prometheus text format.
func (a *HealthAPI) PrometheusMetrics(c *gin.Context) {
	c.String(http.StatusOK, middleware.PrometheusMetrics())
}

func (a *HealthAPI) checkDependencies() gin.H {
	health := gin.H{
		"status":    "ok",
		"timestamp": time.Now().Format(time.RFC3339),
		"runtime":   runtimeSnapshot(),
		"services":  gin.H{},
	}

	services := health["services"].(gin.H)

	dbCheck := gin.H{
		"status": "ok",
	}
	dbStart := time.Now()
	databaseClient := a.databaseStatusClient()
	if databaseClient == nil {
		dbCheck["status"] = "error"
		dbCheck["error"] = "database not initialized"
		health["status"] = "degraded"
		services["database"] = dbCheck
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
		services["database"] = dbCheck
	}
	services["database"] = dbCheck

	redisCheck := gin.H{
		"status": "ok",
	}
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

func (a *HealthAPI) databaseStatusClient() DatabaseClient {
	if a != nil && !isNilClient(a.databaseClient) {
		return a.databaseClient
	}
	if database.DB == nil {
		return nil
	}
	return database.DB
}

func (a *HealthAPI) redisPingClient() RedisPingClient {
	if a != nil && !isNilClient(a.redisClient) {
		return a.redisClient
	}
	if redisstore.Client == nil {
		return nil
	}
	return redisstore.Client
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
