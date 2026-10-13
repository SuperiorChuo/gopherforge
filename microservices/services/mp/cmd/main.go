package main

import (
	"github.com/go-admin-kit/services/shared/pkg/observability"

	"context"
	"errors"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/go-admin-kit/services/mp/internal/aiclient"
	"github.com/go-admin-kit/services/mp/internal/api/mp"
	"github.com/go-admin-kit/services/mp/internal/config"
	"github.com/go-admin-kit/services/mp/internal/content"
	"github.com/go-admin-kit/services/mp/internal/publisher"
	"github.com/go-admin-kit/services/mp/internal/publisher/wechat"
	"github.com/go-admin-kit/services/mp/internal/redisx"
	"github.com/go-admin-kit/services/mp/internal/store/mp"
	"github.com/go-admin-kit/services/mp/internal/wxclient"
	sharedaudit "github.com/go-admin-kit/services/shared/pkg/audittrail"
	"github.com/go-admin-kit/services/shared/pkg/graceful"
	"github.com/go-admin-kit/services/shared/pkg/metrics"
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

	// B3：管理面账号/菜单/文章单行写审计（粉丝同步与消息流水体量大，不注册）
	if err := sharedaudit.Register(st.DB(), sharedaudit.Config{Targets: []sharedaudit.Target{
		{Model: &store.MpAccount{}, Table: "mp_accounts", TargetType: "mp_account", TenantField: "tenant_id",
			SnapshotFields: []string{"ID", "TenantID", "AppID", "Name", "Type", "GhID", "SecretRef", "AESKeyRef", "EncryptMode", "AutoPublish", "Status", "CreatedAt", "UpdatedAt"}},
		{Model: &store.MpMenu{}, Table: "mp_menus", TargetType: "mp_menu", TenantField: "tenant_id",
			SnapshotFields: []string{"ID", "TenantID", "AccountID", "Buttons", "Published", "CreatedAt", "UpdatedAt"}},
		{Model: &store.MpArticle{}, Table: "mp_articles", TargetType: "mp_article", TenantField: "tenant_id",
			SnapshotFields: []string{"ID", "TenantID", "AccountID", "Title", "Author", "Status", "PublishID", "CreatedBy", "CreatedAt", "UpdatedAt"}},
	}}); err != nil {
		log.Fatalf("audit trail: %v", err)
	}

	// 内容中台建表（posts / post_publications / channel_accounts），与微信专属 mp_*
	// 表同库。失败不阻断微信链路，仅记录（社媒功能降级）。
	var contentStore *content.Store
	if err := content.Migrate(st.DB()); err != nil {
		log.Printf("内容中台迁移失败，社媒发布功能降级：%v", err)
	} else {
		contentStore = content.NewStore(st.DB())
	}

	srv := &api.Server{Store: st, Secret: cfg.JWTSecret, Content: contentStore}

	// AI 客户端（M5 生成文章/封面）：ai-service 内网地址配了才启用。
	if cfg.AIBaseURL != "" {
		srv.AI = aiclient.New(cfg.AIBaseURL)
	}

	// 微信 API 客户端：Redis 就绪时启用 access_token 中控 + 主动能力（M2/M4）。
	// Redis 不通不阻断启动——回调收发（M1）不依赖 token，主动能力 handler 判空降级。
	rdb := redisx.New(redisx.Config{
		Host: cfg.RedisHost, Port: cfg.RedisPort,
		Password: cfg.RedisPassword, DB: cfg.RedisDB,
	})
	if err := redisx.Health(context.Background(), rdb); err != nil {
		log.Printf("redis 不可用，微信主动能力降级（回调收发不受影响）：%v", err)
	} else {
		resolver := wxclient.EnvSecretResolver{}
		tm := wxclient.NewTokenManager(redisx.NewTokenCache(rdb), resolver)
		srv.WX = wxclient.NewClient(tm)
		srv.Secrets = resolver
		// EncodingAESKey 从名为 account.AESKeyRef 的环境变量解析（安全模式回调）。
		srv.AESKeyForAcct = func(acc *store.MpAccount) string {
			if acc == nil || acc.AESKeyRef == "" {
				return ""
			}
			return os.Getenv(acc.AESKeyRef)
		}
	}

	// 渠道 adapter 注册表（社媒重构阶段B）：微信 adapter 需 WX 客户端就绪才注册；
	// 后续小红书/抖音等 adapter 在此追加 Register。
	reg := publisher.NewRegistry()
	if srv.WX != nil {
		reg.Register(wechat.New(st, srv.WX))
	}
	srv.Publishers = reg

	workerCtx, stopWorkers := context.WithCancel(context.Background())
	go runPublishResultLoop(workerCtx, srv)
	if srv.WX != nil {
		go runFanSyncLoop(workerCtx, srv)
	}

	// OpenTelemetry tracing (noop when TRACING_ENABLED != "true").
	shutdownTracing, err := observability.InitTracerFromEnv(context.Background(), "mp")
	if err != nil {
		log.Printf("tracing init: %v", err)
	}

	r := gin.New()
	r.Use(observability.GinTracing("mp", "request_id"))
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
		log.Printf("mp-service listening on :%s", cfg.AppPort)
		if err := httpSrv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("listen: %v", err)
		}
	}()

	sh := graceful.New(graceful.WithTimeout(10 * time.Second))
	sh.Register("tracing", func(ctx context.Context) error {
		return shutdownTracing(ctx)
	})
	sh.Register("workers", func(ctx context.Context) error {
		stopWorkers()
		return nil
	})
	sh.Register("http", func(ctx context.Context) error {
		return httpSrv.Shutdown(ctx)
	})
	if err := sh.WaitAndShutdown(); err != nil {
		log.Printf("mp graceful shutdown: %v", err)
	}
}

func runFanSyncLoop(ctx context.Context, srv *api.Server) {
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()
	for {
		if err := srv.ReconcileFanSyncJobs(ctx, 2); err != nil && !errors.Is(err, context.Canceled) {
			log.Printf("mp fan synchronization failed: %v", err)
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

func runPublishResultLoop(ctx context.Context, srv *api.Server) {
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()
	for {
		if err := srv.ReconcilePublishResultJobs(50); err != nil {
			log.Printf("mp publish result reconciliation failed: %v", err)
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}
