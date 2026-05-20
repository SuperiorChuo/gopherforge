package common

import (
	"context"
	"net/http"
	"runtime"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/go-admin-kit/server/internal/middleware"
	"github.com/go-admin-kit/server/internal/pkg/database"
	"github.com/go-admin-kit/server/internal/pkg/redis"
	"github.com/go-admin-kit/server/internal/pkg/response"
)

// HealthAPI 健康检查API
type HealthAPI struct{}

// NewHealthAPI 创建HealthAPI实例
func NewHealthAPI() *HealthAPI {
	return &HealthAPI{}
}

// Health 健康检查
func (a *HealthAPI) Health(c *gin.Context) {
	response.Success(c, gin.H{
		"status":  "ok",
		"time":    time.Now().Format(time.RFC3339),
		"runtime": runtimeSnapshot(),
	})
}

// Liveness 进程存活检查。
func (a *HealthAPI) Liveness(c *gin.Context) {
	response.Success(c, gin.H{
		"status":  "alive",
		"time":    time.Now().Format(time.RFC3339),
		"runtime": runtimeSnapshot(),
	})
}

// Readiness 依赖就绪检查。
func (a *HealthAPI) Readiness(c *gin.Context) {
	health := a.checkDependencies()
	if health["status"] != "ok" {
		c.JSON(http.StatusServiceUnavailable, response.Response{
			Code:    503,
			Message: "service unavailable",
			Data:    health,
		})
		return
	}
	response.Success(c, health)
}

// HealthCheck 详细健康检查
func (a *HealthAPI) HealthCheck(c *gin.Context) {
	response.Success(c, a.checkDependencies())
}

// MetricsSnapshot 返回 JSON 格式的进程内 HTTP 指标。
func (a *HealthAPI) MetricsSnapshot(c *gin.Context) {
	response.Success(c, middleware.MetricsSnapshot())
}

// PrometheusMetrics 返回 Prometheus 文本格式指标。
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
	if database.DB == nil {
		dbCheck["status"] = "error"
		dbCheck["error"] = "database not initialized"
		health["status"] = "degraded"
		services["database"] = dbCheck
	} else if sqlDB, err := database.DB.DB(); err == nil {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := sqlDB.PingContext(ctx); err != nil {
			dbCheck["status"] = "error"
			dbCheck["error"] = err.Error()
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
		dbCheck["error"] = err.Error()
		health["status"] = "degraded"
		services["database"] = dbCheck
	}
	services["database"] = dbCheck

	redisCheck := gin.H{
		"status": "ok",
	}
	redisStart := time.Now()
	if redis.Client == nil {
		redisCheck["status"] = "error"
		redisCheck["error"] = "redis not initialized"
		health["status"] = "degraded"
	} else {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := redis.Client.Ping(ctx).Err(); err != nil {
			redisCheck["status"] = "error"
			redisCheck["error"] = err.Error()
			health["status"] = "degraded"
		}
	}
	redisCheck["ping_latency_ms"] = float64(time.Since(redisStart)) / float64(time.Millisecond)
	services["redis"] = redisCheck

	return health
}

func runtimeSnapshot() gin.H {
	return gin.H{
		"go_version": runtime.Version(),
		"os":         runtime.GOOS,
		"arch":       runtime.GOARCH,
		"compiler":   runtime.Compiler,
	}
}
