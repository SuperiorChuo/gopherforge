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

	botClient := bot.NewClient(bot.Config{
		Enabled:      cfg.AIEnabled,
		BaseURL:      cfg.AIBaseURL,
		APIKey:       cfg.AIAPIKey,
		Model:        cfg.AIModel,
		Timeout:      cfg.AITimeout,
		SystemPrompt: cfg.AISystemPrompt,
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
	}

	r := gin.New()
	r.Use(gin.Recovery(), gin.Logger())

	// health (no auth)
	r.GET("/api/v1/health/live", srv.HealthLive)
	r.GET("/api/v1/health/ready", srv.HealthReady)
	r.GET("/api/v1/im/health/live", srv.HealthLive)
	r.GET("/api/v1/im/health/ready", srv.HealthReady)

	// visitor / widget
	r.GET("/api/v1/im/widget/config", srv.WidgetConfig)
	r.POST("/api/v1/im/visitor/session", srv.VisitorSession)
	r.POST("/api/v1/im/conversations", srv.CreateConversation)
	r.GET("/api/v1/im/conversations/:public_id/messages", srv.ListMessages)
	r.POST("/api/v1/im/conversations/:public_id/messages", srv.SendMessage)
	r.POST("/api/v1/im/conversations/:public_id/transfer_human", srv.TransferHuman)

	// agent (M1 + M3 + M4)
	r.GET("/api/v1/im/agent/me", srv.AgentMe)
	r.PUT("/api/v1/im/agent/presence", srv.AgentPresence)
	r.GET("/api/v1/im/agent/conversations", srv.AgentListConversations)
	r.GET("/api/v1/im/agent/queue", srv.AgentQueue)
	r.GET("/api/v1/im/agent/online", srv.AgentOnlineList)
	r.POST("/api/v1/im/agent/conversations/:public_id/accept", srv.AgentAccept)
	r.POST("/api/v1/im/agent/conversations/:public_id/transfer", srv.AgentTransfer)
	r.POST("/api/v1/im/agent/conversations/:public_id/close", srv.AgentClose)
	r.POST("/api/v1/im/agent/conversations/:public_id/summary", srv.AgentSummary)

	// admin sites (M2 embed config)
	r.GET("/api/v1/im/admin/sites", srv.AdminListSites)
	r.PUT("/api/v1/im/admin/sites/:id", srv.AdminUpdateSite)

	// admin skill groups (M3)
	r.GET("/api/v1/im/admin/skill-groups", srv.AdminListSkillGroups)
	r.POST("/api/v1/im/admin/skill-groups", srv.AdminCreateSkillGroup)
	r.PUT("/api/v1/im/admin/skill-groups/:id", srv.AdminUpdateSkillGroup)
	r.GET("/api/v1/im/admin/skill-groups/:id/agents", srv.AdminListSkillAgents)
	r.POST("/api/v1/im/admin/skill-groups/:id/agents", srv.AdminUpsertAgentSkill)
	r.POST("/api/v1/im/admin/agent-skills", srv.AdminUpsertAgentSkill)
	r.DELETE("/api/v1/im/admin/agent-skills/:id", srv.AdminDeleteAgentSkill)

	// websocket
	r.GET("/im/ws", srv.WebSocket)

	// visitor H5 (M1, non-embed)
	r.StaticFile("/im/visitor", "./web-visitor/index.html")
	r.Static("/im/visitor/static", "./web-visitor")

	// webpage embed widget (M2)
	r.Static("/im/widget", "./web-widget")
	r.StaticFile("/im/widget/demo", "./web-widget/demo.html")

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
