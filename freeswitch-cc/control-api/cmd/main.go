package main

import (
	"log"
	"net/http"
	"time"

	"github.com/SuperiorChuo/go-freeswitch-cc/control-api/internal/api"
	"github.com/SuperiorChuo/go-freeswitch-cc/control-api/internal/config"
	"github.com/SuperiorChuo/go-freeswitch-cc/control-api/internal/esl"
	"github.com/SuperiorChuo/go-freeswitch-cc/control-api/internal/store"
	"github.com/SuperiorChuo/go-freeswitch-cc/control-api/internal/webhook"
	"github.com/gin-gonic/gin"
)

func main() {
	cfg := config.Load()
	gin.SetMode(gin.ReleaseMode)

	st, err := store.Open(cfg.DBDSN)
	if err != nil {
		log.Fatalf("db: %v", err)
	}
	eslClient := esl.New(cfg.ESLHost, cfg.ESLPort, cfg.ESLPassword)
	hook := webhook.New(cfg.WebhookURL, cfg.WebhookSecret)

	srv := &api.Server{
		Cfg:   cfg,
		ESL:   eslClient,
		Store: st,
		Hook:  hook,
	}

	httpSrv := &http.Server{
		Addr:              ":" + cfg.Port,
		Handler:           srv.Router(),
		ReadHeaderTimeout: 5 * time.Second,
	}
	log.Printf("control-api listening on :%s (esl=%s:%s)", cfg.Port, cfg.ESLHost, cfg.ESLPort)
	if err := httpSrv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatal(err)
	}
}
