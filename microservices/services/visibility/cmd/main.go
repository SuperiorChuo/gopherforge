package main

import (
	"github.com/go-admin-kit/services/shared/pkg/observability"

	"context"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	sharedaudit "github.com/go-admin-kit/services/shared/pkg/audittrail"
	"github.com/go-admin-kit/services/shared/pkg/graceful"
	"github.com/go-admin-kit/services/shared/pkg/jobbeat"
	"github.com/go-admin-kit/services/shared/pkg/metrics"
	"github.com/go-admin-kit/services/shared/pkg/notifyclient"
	"github.com/go-admin-kit/services/visibility/internal/api/visibility"
	"github.com/go-admin-kit/services/visibility/internal/config"
	"github.com/go-admin-kit/services/visibility/internal/model"
	"github.com/go-admin-kit/services/visibility/internal/runner"
	"github.com/go-admin-kit/services/visibility/internal/serp"
	"github.com/go-admin-kit/services/visibility/internal/store/visibility"
	"github.com/robfig/cron/v3"
)

func main() {
	cfg := config.Load()
	if cfg.AppEnv != "development" {
		gin.SetMode(gin.ReleaseMode)
	}

	st, err := store.Open(cfg.DSN())
	if err != nil {
		log.Fatalf("db: %v", err)
	}
	st.SetIdentity(cfg.IdentityAPIBase, cfg.IdentityInternalToken)

	// B3：品牌/关键词/GEO 题库管理写审计（采集 run/result 体量大，不注册）
	if err := sharedaudit.Register(st.DB(), sharedaudit.Config{Targets: []sharedaudit.Target{
		{Model: &model.Brand{}, Table: "vis_brands", TargetType: "vis_brand", TenantField: "tenant_id",
			SnapshotFields: []string{"ID", "TenantID", "Name", "Aliases", "Kind", "Website", "Note", "Status", "CreatedAt", "UpdatedAt"}},
		{Model: &model.Keyword{}, Table: "vis_keywords", TargetType: "vis_keyword", TenantField: "tenant_id",
			SnapshotFields: []string{"ID", "TenantID", "Keyword", "Locale", "TargetURL", "Status", "Note", "CreatedAt", "UpdatedAt"}},
		{Model: &model.GeoPrompt{}, Table: "vis_geo_prompts", TargetType: "vis_geo_prompt", TenantField: "tenant_id",
			SnapshotFields: []string{"ID", "TenantID", "Category", "Question", "Engine", "Status", "CreatedAt", "UpdatedAt"}},
	}}); err != nil {
		log.Fatalf("audit trail: %v", err)
	}

	// 启动恢复：上次进程遗留的 running 任务全部标 failed。
	if err := st.FailStaleTasks("服务重启，任务中断"); err != nil {
		log.Printf("fail stale tasks: %v", err)
	}

	notify := notifyclient.New(cfg.NotifyAPIBase, cfg.NotifyInternalToken)
	if notify.Enabled() {
		log.Printf("visibility notify: enabled → %s", cfg.NotifyAPIBase)
	} else {
		log.Printf("visibility notify: disabled (set NOTIFY_INTERNAL_TOKEN to enable)")
	}
	run := runner.New(st, runner.AIConfig{
		BaseURL: cfg.AIBaseURL,
		APIKey:  cfg.AIAPIKey,
		Model:   cfg.AIModel,
	}, serp.CollectorsByNames(cfg.SerpEngines), cfg.SerpMaxPages, notify)
	// Perplexity/Gemini 真实引擎（可选）：配置可达渠道和有效密钥后启用。
	run.SetExtraEngines(
		runner.AIConfig{BaseURL: cfg.PerplexityBaseURL, APIKey: cfg.PerplexityAPIKey, Model: cfg.PerplexityModel},
		runner.AIConfig{BaseURL: cfg.GeminiBaseURL, APIKey: cfg.GeminiAPIKey, Model: cfg.GeminiModel},
	)

	var cr *cron.Cron
	if cfg.SerpEnabled || cfg.GeoCronEnabled {
		// SkipIfStillRunning：serp 全程串行 + 逐词延迟可能跑数小时，周期到点未完则跳过本轮防并发抓取
		cr = cron.New(cron.WithChain(cron.SkipIfStillRunning(cron.DiscardLogger)))
		started := false
		if cfg.SerpEnabled {
			if _, err := cr.AddFunc(cfg.SerpCron, func() {
				start := time.Now()
				run.RunDailySerp(st)
				jobbeat.Report(st.DB(), jobbeat.Run{
					Key: "visibility.serp_daily", Service: "visibility-service",
					Description: "SERP 收录排名采集（cron " + cfg.SerpCron + "）",
					IntervalSec: 86400, StartedAt: start,
				})
			}); err != nil {
				log.Printf("serp cron %q invalid: %v", cfg.SerpCron, err)
			} else {
				started = true
				log.Printf("serp cron scheduled: %s", cfg.SerpCron)
			}
		}
		if cfg.GeoCronEnabled {
			if _, err := cr.AddFunc(cfg.GeoCron, func() {
				start := time.Now()
				run.RunWeeklyGeo(st)
				jobbeat.Report(st.DB(), jobbeat.Run{
					Key: "visibility.geo_check", Service: "visibility-service",
					Description: "GEO 提及检测 + 建议 + 站内信（cron " + cfg.GeoCron + "）",
					// 代码默认周检、本环境 env 覆盖为日检——间隔按宽的周检算，避免日检环境误报 stale 需知：改回周检也不误报
					IntervalSec: 7 * 86400, StartedAt: start,
				})
			}); err != nil {
				log.Printf("geo cron %q invalid: %v", cfg.GeoCron, err)
			} else {
				started = true
				log.Printf("geo cron scheduled: %s", cfg.GeoCron)
			}
		}
		if started {
			cr.Start()
		}
	}

	if cfg.DemoMode && cfg.ProductionMode() {
		log.Printf("WARNING: VISIBILITY_DEMO_MODE is on under APP_ENV=production — 模拟检测、演示排行榜与演示数据播种端点均已启用，用户可能把模板拼的假数据当成真实检测结果")
	}
	log.Printf("visibility demo mode: %v (stub 检测 / 排行榜演示回落 / seed-demo 端点)", cfg.DemoMode)
	srv := &api.Server{
		Store:     st,
		Runner:    run,
		Secret:    cfg.JWTSecret,
		AIBaseURL: cfg.AIBaseURL,
		AIAPIKey:  cfg.AIAPIKey,
		AIModel:   cfg.AIModel,
		DemoMode:  cfg.DemoMode,
	}

	// OpenTelemetry tracing (noop when TRACING_ENABLED != "true").
	shutdownTracing, err := observability.InitTracerFromEnv(context.Background(), "visibility")
	if err != nil {
		log.Printf("tracing init: %v", err)
	}

	r := gin.New()
	r.Use(observability.GinTracing("visibility", "request_id"))
	// HTTP 指标（GET /metrics，Prometheus 抓取）；先于 Logger 注册，抓取不刷访问日志
	metrics.Install(r)
	// 健康探针每 10s 一次；成功探测不进访问日志，失败（>=400）仍记录
	r.Use(gin.Recovery(), sharedaudit.AuditHeaderMiddleware(), gin.LoggerWithConfig(gin.LoggerConfig{
		Skip: func(c *gin.Context) bool {
			p := c.Request.URL.Path
			return c.Writer.Status() < 400 &&
				(strings.HasSuffix(p, "/health/live") || strings.HasSuffix(p, "/health/ready"))
		},
	}))
	srv.RegisterRoutes(r)

	httpSrv := &http.Server{
		Addr:              ":" + cfg.AppPort,
		Handler:           r,
		ReadHeaderTimeout: 10 * time.Second,
	}
	go func() {
		log.Printf("visibility-service listening on :%s", cfg.AppPort)
		if err := httpSrv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("listen: %v", err)
		}
	}()

	// LIFO：http → cron → runner → tracing
	sh := graceful.New(graceful.WithTimeout(15 * time.Second))
	sh.Register("tracing", func(ctx context.Context) error {
		return shutdownTracing(ctx)
	})
	sh.Register("runner", func(ctx context.Context) error {
		run.Shutdown()
		return nil
	})
	sh.Register("cron", func(ctx context.Context) error {
		if cr != nil {
			stopCtx := cr.Stop()
			select {
			case <-stopCtx.Done():
			case <-ctx.Done():
			}
		}
		return nil
	})
	sh.Register("http", func(ctx context.Context) error {
		return httpSrv.Shutdown(ctx)
	})
	if err := sh.WaitAndShutdown(); err != nil {
		log.Printf("visibility graceful shutdown: %v", err)
	}
}
