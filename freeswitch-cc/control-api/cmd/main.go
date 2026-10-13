package main

import (
	"context"
	"log"
	"net/http"
	"os/signal"
	"syscall"
	"time"

	"github.com/SuperiorChuo/go-freeswitch-cc/control-api/internal/api"
	"github.com/SuperiorChuo/go-freeswitch-cc/control-api/internal/bridge"
	"github.com/SuperiorChuo/go-freeswitch-cc/control-api/internal/config"
	"github.com/SuperiorChuo/go-freeswitch-cc/control-api/internal/esl"
	"github.com/SuperiorChuo/go-freeswitch-cc/control-api/internal/extdir"
	"github.com/SuperiorChuo/go-freeswitch-cc/control-api/internal/metrics"
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
	// 动态配置自愈：DB → 各共享卷全量物化（卷可能是新建/被模板覆盖过的）
	if exts, err := st.ListExtensions(); err != nil {
		log.Printf("list extensions: %v", err)
	} else if err := extdir.Sync(cfg.ExtDir, exts); err != nil {
		log.Printf("extdir sync: %v", err)
	}
	if gws, err := st.ListGateways(); err != nil {
		log.Printf("list gateways: %v", err)
	} else if err := extdir.SyncGateways(cfg.GwDir, cfg.DialplanDir, gws); err != nil {
		log.Printf("gateway sync: %v", err)
	}
	if ivrs, err := st.ListIVRMenus(); err != nil {
		log.Printf("list ivrs: %v", err)
	} else if err := extdir.SyncIVRs(cfg.IVRDir, cfg.DialplanDir, ivrs); err != nil {
		log.Printf("ivr sync: %v", err)
	}
	if queues, err := st.ListCCQueues(); err != nil {
		log.Printf("list cc queues: %v", err)
	} else if agents, err := st.ListQueueAgents(""); err != nil {
		log.Printf("list queue agents: %v", err)
	} else if err := extdir.SyncCCQueues(cfg.CCQueueDir, cfg.DialplanDir, queues, agents); err != nil {
		log.Printf("cc queue sync: %v", err)
	}
	if rooms, err := st.ListConferences(); err != nil {
		log.Printf("list conferences: %v", err)
	} else if err := extdir.SyncConferences(cfg.DialplanDir, rooms); err != nil {
		log.Printf("conference sync: %v", err)
	}
	if routes, err := st.ListTimeRoutes(); err != nil {
		log.Printf("list time routes: %v", err)
	} else if err := extdir.SyncTimeRoutes(cfg.DialplanDir, routes); err != nil {
		log.Printf("time route sync: %v", err)
	}
	if groups, err := st.ListRingGroups(); err != nil {
		log.Printf("list ring groups: %v", err)
	} else if err := extdir.SyncRingGroups(cfg.DialplanDir, groups); err != nil {
		log.Printf("ring group sync: %v", err)
	}
	if bl, err := st.ListBlacklist(); err != nil {
		log.Printf("list blacklist: %v", err)
	} else {
		numbers := make([]string, 0, len(bl))
		for _, e := range bl {
			numbers = append(numbers, e.Number)
		}
		if err := extdir.RenderBlacklist(cfg.BlacklistDir, numbers); err != nil {
			log.Printf("blacklist sync: %v", err)
		}
	}
	eslClient := esl.New(cfg.ESLHost, cfg.ESLPort, cfg.ESLPassword)
	hook := webhook.New(cfg.WebhookURL, cfg.WebhookSecret)

	// FS-M2: persistent ESL subscription → call.* webhooks + local CDR.
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	br := bridge.New(st, hook)
	var srv *api.Server
	handler := func(ev map[string]string) {
		br.Handle(ev)
		if srv != nil {
			srv.GuardOutbound(ev)
			srv.HandleEarlyMedia(ev)
		}
	}
	listener := esl.NewListener(cfg.ESLHost, cfg.ESLPort, cfg.ESLPassword, bridge.Events, handler)

	// 批次 N：Prometheus 指标采集器（每 10s 一轮 ESL，写入 gauge）。
	m := metrics.New()
	collector := metrics.NewCollector(eslClient, m, 10*time.Second)
	go collector.Run(ctx)

	srv = &api.Server{
		Cfg:      cfg,
		ESL:      eslClient,
		Store:    st,
		Hook:     hook,
		Listener: listener,
		Metrics:  m,
	}
	go listener.Run(ctx)

	httpSrv := &http.Server{
		Addr:              ":" + cfg.Port,
		Handler:           srv.Router(),
		ReadHeaderTimeout: 5 * time.Second,
	}
	go func() {
		log.Printf("control-api listening on :%s (esl=%s:%s, webhook=%q)", cfg.Port, cfg.ESLHost, cfg.ESLPort, cfg.WebhookURL)
		if err := httpSrv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatal(err)
		}
	}()

	<-ctx.Done()
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_ = httpSrv.Shutdown(shutdownCtx)
}
