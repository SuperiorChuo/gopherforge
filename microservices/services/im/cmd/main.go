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

	srv := &api.Server{
		Store:           st,
		Hub:             hub.New(),
		AgentHub:        hub.NewAgentHub(),
		Secret:          cfg.JWTSecret,
		Bot:             botClient,
		BotSystemPrompt: cfg.AISystemPrompt,
		AIEnabled:       cfg.AIEnabled,
		UploadDir:       cfg.UploadDir,
		Limits:          api.DefaultLimits(),
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
	// 附件静态托管（文件名为 UUID，不可枚举）
	r.Static("/im/uploads", cfg.UploadDir)

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
