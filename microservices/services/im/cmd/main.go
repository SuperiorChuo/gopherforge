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

	srv := &api.Server{
		Store:    st,
		Hub:      hub.New(),
		AgentHub: hub.NewAgentHub(),
		Secret:   cfg.JWTSecret,
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

	// agent
	r.GET("/api/v1/im/agent/conversations", srv.AgentListConversations)
	r.POST("/api/v1/im/agent/conversations/:public_id/accept", srv.AgentAccept)
	r.POST("/api/v1/im/agent/conversations/:public_id/close", srv.AgentClose)

	// websocket
	r.GET("/im/ws", srv.WebSocket)

	// visitor H5 (M1, non-embed)
	r.StaticFile("/im/visitor", "./web-visitor/index.html")
	r.Static("/im/visitor/static", "./web-visitor")

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
