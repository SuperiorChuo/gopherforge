// Package health provides a shared, dependency-injected health check API for
// all services. Each service wires its own *sql.DB / redis.UniversalClient via
// the constructor; no service-specific imports leak into shared.
//
// Previously 7 infrastructure services each carried a near-identical copy of
// health.go; 9 business services had no readiness/dependency checks at all.
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

// API exposes /health/live, /health/ready, /health and /health/check.
// Zero-value is usable; all dependency checks are skipped when no client
// is injected (ready always reports ok).
type API struct {
	db    *sql.DB
	redis RedisPingClient
}

// New creates a HealthAPI. Nil parameters skip the corresponding check.
func New(db *sql.DB, redis RedisPingClient) *API {
	return &API{db: db, redis: redis}
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
	h := a.checkDependencies()
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
	response.Success(c, a.checkDependencies())
}

func (a *API) checkDependencies() gin.H {
	h := gin.H{
		"status":    "ok",
		"timestamp": time.Now().Format(time.RFC3339),
		"runtime":   runtimeSnapshot(),
		"services":  gin.H{},
	}
	services := h["services"].(gin.H)

	// ---- database ----
	dbCheck := gin.H{"status": "ok"}
	if a == nil || a.db == nil {
		dbCheck["status"] = "ok"
		dbCheck["note"] = "not configured"
	} else {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := a.db.PingContext(ctx); err != nil {
			dbCheck["status"] = "error"
			dbCheck["error"] = dependencyUnavailableMessage
			h["status"] = "degraded"
		}
		stats := a.db.Stats()
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
	}
	services["database"] = dbCheck

	// ---- redis ----
	redisCheck := gin.H{"status": "ok"}
	if a == nil || isNilRedis(a.redis) {
		redisCheck["status"] = "ok"
		redisCheck["note"] = "not configured"
	} else {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := a.redis.Ping(ctx).Err(); err != nil {
			redisCheck["status"] = "error"
			redisCheck["error"] = dependencyUnavailableMessage
			h["status"] = "degraded"
		}
	}
	services["redis"] = redisCheck

	return h
}

func isNilRedis(client any) bool {
	if client == nil {
		return true
	}
	v := reflect.ValueOf(client)
	switch v.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		return v.IsNil()
	}
	return false
}

func runtimeSnapshot() gin.H {
	return gin.H{
		"go_version": runtime.Version(),
		"os":         runtime.GOOS,
		"arch":       runtime.GOARCH,
		"compiler":   runtime.Compiler,
	}
}
