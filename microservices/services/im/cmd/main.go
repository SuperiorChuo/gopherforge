package main

import (
	"github.com/go-admin-kit/services/shared/pkg/observability"

	"context"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/go-admin-kit/services/im/internal/api/im"
	"github.com/go-admin-kit/services/im/internal/bot"
	"github.com/go-admin-kit/services/im/internal/config"
	"github.com/go-admin-kit/services/im/internal/hub"
	"github.com/go-admin-kit/services/im/internal/kbclient"
	"github.com/go-admin-kit/services/im/internal/model"
	"github.com/go-admin-kit/services/im/internal/sensitive"
	"github.com/go-admin-kit/services/im/internal/settings"
	"github.com/go-admin-kit/services/im/internal/storage"
	"github.com/go-admin-kit/services/im/internal/store/im"
	sharedaudit "github.com/go-admin-kit/services/shared/pkg/audittrail"
	"github.com/go-admin-kit/services/shared/pkg/graceful"
	"github.com/go-admin-kit/services/shared/pkg/metrics"
	"github.com/go-admin-kit/services/shared/pkg/notifyclient"
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
	// 数据变更审计：站点/技能组/坐席技能/快捷回复/敏感词为单行写审计目标。
	// 会话与消息为高频写（MaxRows=1 不适用），不注册；presence 为心跳写不注册。
	if err := sharedaudit.Register(st.DB(), sharedaudit.Config{Targets: []sharedaudit.Target{
		{Model: &model.Site{}, Table: "im_sites", TargetType: "im_site", TenantField: "tenant_id",
			SnapshotFields: []string{"ID", "TenantID", "Name", "AppKey", "AllowedOrigins", "WelcomeText", "Status", "BotEnabled"}},
		{Model: &model.SkillGroup{}, Table: "im_skill_groups", TargetType: "im_skill_group", TenantField: "tenant_id",
			SnapshotFields: []string{"ID", "TenantID", "Name", "Code", "Strategy", "Status"}},
		// im_agent_skills 无 tenant_id 列（租户通过 skill_group_id 间接归属），插件要求直接列，不注册。
		{Model: &model.QuickReply{}, Table: "im_quick_replies", TargetType: "im_quick_reply", TenantField: "tenant_id",
			SnapshotFields: []string{"ID", "TenantID", "Title", "Content", "Sort", "CreatedBy"}},
		{Model: &model.SensitiveWord{}, Table: "im_sensitive_words", TargetType: "im_sensitive_word", TenantField: "tenant_id",
			SnapshotFields: []string{"ID", "TenantID", "Word", "Action", "Tag", "Status", "CreatedBy"}},
	}}); err != nil {
		log.Fatalf("audit trail plugin registration failed: %v", err)
	}

	// 控制台「系统设置 → AI 服务」(ai.provider) 覆盖环境变量，30s TTL 内热生效。
	aiSettings := settings.NewAIProviderReader(st.DB(), 30*time.Second)
	botClient := bot.NewDynamic(bot.Config{
		Enabled:      cfg.AIEnabled,
		BaseURL:      cfg.AIBaseURL,
		APIKey:       cfg.AIAPIKey,
		Model:        cfg.AIModel,
		Timeout:      cfg.AITimeout,
		SystemPrompt: cfg.AISystemPrompt,
	}, func(ctx context.Context) bot.Overrides {
		s := aiSettings.Get(ctx)
		return bot.Overrides{
			Provider:  s.Provider,
			BaseURL:   s.BaseURL,
			APIKey:    s.APIKey,
			ChatModel: s.ChatModel,
		}
	})
	log.Printf("im bot provider=%s ai_enabled=%v", botClient.Name(), cfg.AIEnabled)

	var attStorage storage.Store = storage.NewLocal(cfg.UploadDir)
	if cfg.StorageType == "minio" {
		if m, err := storage.NewMinIO(context.Background(), storage.MinIOConfig{
			Endpoint:  cfg.MinIOEndpoint,
			AccessKey: cfg.MinIOAccessKey,
			SecretKey: cfg.MinIOSecretKey,
			Bucket:    cfg.MinIOBucket,
			UseSSL:    cfg.MinIOUseSSL,
		}); err != nil {
			// 附件是次要能力：MinIO 不可用降级本地盘，不拖垮消息主链路
			log.Printf("im storage: minio unavailable, falling back to local: %v", err)
		} else {
			attStorage = m
		}
	}
	log.Printf("im attachment storage=%s", attStorage.Type())

	convHub := hub.New()
	agentHub := hub.NewAgentHub()
	if cfg.NATSURL != "" {
		if nc, err := hub.ConnectNATS(cfg.NATSURL, convHub, agentHub); err != nil {
			// 广播降级为进程内直投，仅单实例可用
			log.Printf("im hub: nats unavailable, in-process fan-out only: %v", err)
		} else {
			defer nc.Close()
			log.Printf("im hub: nats connected (%s)", cfg.NATSURL)
		}
	}

	var kbVec *kbclient.Client
	if cfg.AIInternalToken != "" {
		kbVec = kbclient.New(cfg.AIServiceURL, cfg.AIInternalToken)
		log.Printf("im kb vector: enabled via %s", cfg.AIServiceURL)
	} else {
		log.Printf("im kb vector: disabled (set AI_INTERNAL_TOKEN to enable)")
	}

	var notifyCli *notifyclient.Client
	if cfg.NotifyInternalToken != "" {
		notifyCli = notifyclient.New(cfg.NotifyAPIBase, cfg.NotifyInternalToken)
		log.Printf("im notify: enabled → %s", cfg.NotifyAPIBase)
	} else {
		log.Printf("im notify: disabled (set NOTIFY_INTERNAL_TOKEN to enable)")
	}

	srv := &api.Server{
		Store:           st,
		Hub:             convHub,
		AgentHub:        agentHub,
		Secret:          cfg.JWTSecret,
		Bot:             botClient,
		BotSystemPrompt: cfg.AISystemPrompt,
		AIEnabled:       cfg.AIEnabled,
		KBVector:        kbVec,
		Notify:          notifyCli,
		Storage:         attStorage,
		UploadDir:       cfg.UploadDir,
		Limits:          api.DefaultLimits(),
		SensitiveWords:  cfg.SensitiveWords,
		Sensitive:       sensitive.NewCache(st),
	}
	if len(cfg.SensitiveWords) > 0 {
		log.Printf("im sensitive words: %d env fallback entries (console word list managed in DB)", len(cfg.SensitiveWords))
	}

	// 空闲超时自动关闭：每分钟扫一轮，关掉长时间无消息的 queued/assigned 会话
	if cfg.IdleCloseMinutes > 0 {
		idle := time.Duration(cfg.IdleCloseMinutes) * time.Minute
		go func() {
			for {
				time.Sleep(time.Minute)
				closed, err := st.CloseIdleConversations(idle, 100)
				if err != nil {
					log.Printf("im idle-close: %v", err)
					continue
				}
				for i := range closed {
					conv := &closed[i]
					payload := gin.H{"type": "conversation.updated", "payload": conv}
					agentHub.Publish(conv.TenantID, payload)
					convHub.Publish(conv.PublicID.String(), payload)
				}
				if len(closed) > 0 {
					log.Printf("im idle-close: closed %d conversations (idle > %s)", len(closed), idle)
				}
			}
		}()
		log.Printf("im idle-close: %d minutes", cfg.IdleCloseMinutes)
	}

	// 保留期清理：每日一次，删除超期 closed 会话（消息 + MinIO/本地附件对象）
	if cfg.RetentionDays > 0 {
		retention := time.Duration(cfg.RetentionDays) * 24 * time.Hour
		go func() {
			for {
				res, err := st.PurgeExpired(retention, 500)
				if err != nil {
					log.Printf("im retention: purge error: %v", err)
				} else if res.Conversations > 0 {
					log.Printf("im retention: purged %d conversations, %d messages, %d attachments",
						res.Conversations, res.Messages, len(res.AttachmentKeys))
				}
				if res != nil {
					for _, key := range res.AttachmentKeys {
						if err := attStorage.Delete(context.Background(), key); err != nil {
							log.Printf("im retention: delete attachment %s: %v", key, err)
						}
					}
				}
				time.Sleep(24 * time.Hour)
			}
		}()
		log.Printf("im retention: %d days (closed conversations)", cfg.RetentionDays)
	}

	// OpenTelemetry 链路追踪（TRACING_ENABLED != "true" 时为空操作 noop）。
	shutdownTracing, err := observability.InitTracerFromEnv(context.Background(), "im")
	if err != nil {
		log.Printf("tracing init: %v", err)
	}

	r := gin.New()
	r.Use(observability.GinTracing("im", "request_id"))
	// HTTP 指标（GET /metrics，Prometheus 抓取）；先于 Logger 注册，抓取不刷访问日志
	metrics.Install(r)
	// 健康探针每 10s 一次；成功探测不进访问日志，失败（>=400）仍记录
	r.Use(gin.Recovery(), gin.LoggerWithConfig(gin.LoggerConfig{
		Skip: func(c *gin.Context) bool {
			p := c.Request.URL.Path
			return c.Writer.Status() < 400 &&
				(strings.HasSuffix(p, "/health/live") || strings.HasSuffix(p, "/health/ready"))
		},
	}))
	r.Use(sharedaudit.AuditHeaderMiddleware())

	srv.RegisterRoutes(r)

	// 访客 H5（M1，非嵌入）
	r.StaticFile("/im/visitor", "./web-visitor/index.html")
	r.Static("/im/visitor/static", "./web-visitor")

	// 网页嵌入组件（M2）
	// Gin 不允许 /im/widget/*filepath 与精确路径共存，demo 页直接走 /im/widget/demo.html
	r.Static("/im/widget", "./web-widget")
	// 附件下载由 RegisterRoutes 的 /im/uploads/*key 处理（对象存储 + 本地回源）

	httpSrv := &http.Server{Addr: ":" + cfg.AppPort, Handler: r}
	go func() {
		log.Printf("im-service listening on :%s", cfg.AppPort)
		if err := httpSrv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("listen: %v", err)
		}
	}()

	sh := graceful.New(graceful.WithTimeout(10 * time.Second))
	sh.Register("tracing", func(ctx context.Context) error {
		return shutdownTracing(ctx)
	})
	sh.Register("http", func(ctx context.Context) error {
		return httpSrv.Shutdown(ctx)
	})
	if err := sh.WaitAndShutdown(); err != nil {
		log.Printf("im graceful shutdown: %v", err)
	}
}
