// bpm-service 入口：轻量审批流引擎（M1）。
// 定义/实例/任务/日志 AutoMigrate 自管表；推进为同步事务内函数调用；
// 终态经 HTTP 回调业务方（BPM_CALLBACK_<BIZTYPE> 注册）；站内信经
// notify internal API（未配 token 静默跳过）。超时提醒 ticker 属 M2。
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
	"github.com/go-admin-kit/services/bpm/internal/api"
	"github.com/go-admin-kit/services/bpm/internal/callback"
	"github.com/go-admin-kit/services/bpm/internal/config"
	"github.com/go-admin-kit/services/bpm/internal/engine"
	"github.com/go-admin-kit/services/bpm/internal/notifyclient"
	"github.com/go-admin-kit/services/bpm/internal/store"
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

	notify := notifyclient.New(cfg.NotifyAPIBase, cfg.NotifyInternalToken)
	if notify.Enabled() {
		log.Printf("bpm notify: enabled → %s", cfg.NotifyAPIBase)
	} else {
		log.Printf("bpm notify: disabled (set NOTIFY_INTERNAL_TOKEN to enable)")
	}

	cb := callback.New(callback.TargetsFromEnv(), cfg.CallbackToken)
	log.Printf("bpm callback: %d biz_type target(s) registered", cb.Targets())

	srv := &api.Server{
		Store:         st,
		Engine:        engine.New(st.DB()),
		Secret:        cfg.JWTSecret,
		InternalToken: cfg.InternalToken,
		Notify:        notify,
		Callback:      cb,
	}

	r := gin.New()
	r.Use(gin.Recovery(), gin.Logger())
	srv.RegisterRoutes(r)

	httpSrv := &http.Server{
		Addr:              ":" + cfg.AppPort,
		Handler:           r,
		ReadHeaderTimeout: 10 * time.Second,
	}
	go func() {
		log.Printf("bpm-service listening on :%s", cfg.AppPort)
		if err := httpSrv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("listen: %v", err)
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_ = httpSrv.Shutdown(ctx)
}
