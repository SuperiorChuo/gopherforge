package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/go-admin-kit/services/im/internal/api"
	"github.com/go-admin-kit/services/im/internal/bot"
	"github.com/go-admin-kit/services/im/internal/config"
	"github.com/go-admin-kit/services/im/internal/hub"
	"github.com/go-admin-kit/services/im/internal/settings"
	"github.com/go-admin-kit/services/im/internal/storage"
	"github.com/go-admin-kit/services/im/internal/store"
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

	srv := &api.Server{
		Store:           st,
		Hub:             convHub,
		AgentHub:        agentHub,
		Secret:          cfg.JWTSecret,
		Bot:             botClient,
		BotSystemPrompt: cfg.AISystemPrompt,
		AIEnabled:       cfg.AIEnabled,
		Storage:         attStorage,
		UploadDir:       cfg.UploadDir,
		Limits:          api.DefaultLimits(),
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

	r := gin.New()
	r.Use(gin.Recovery(), gin.Logger())

	srv.RegisterRoutes(r)

	// visitor H5 (M1, non-embed)
	r.StaticFile("/im/visitor", "./web-visitor/index.html")
	r.Static("/im/visitor/static", "./web-visitor")

	// webpage embed widget (M2)
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

	ch := make(chan os.Signal, 1)
	signal.Notify(ch, syscall.SIGINT, syscall.SIGTERM)
	<-ch
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_ = httpSrv.Shutdown(ctx)
}
