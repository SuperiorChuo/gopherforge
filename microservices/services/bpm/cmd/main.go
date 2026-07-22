// bpm-service 入口：轻量审批流引擎（M2）。
// 定义/实例/任务/日志 AutoMigrate 自管表；推进为同步事务内函数调用；
// 终态经 HTTP 回调业务方（BPM_CALLBACK_<BIZTYPE> 注册）；站内信经
// notify internal API（未配 token 静默跳过）。超时提醒 ticker（M2）：
// 与同类到期扫描 ticker 同构，周期由
// BPM_TIMEOUT_SCAN_INTERVAL 控制（默认 5m），随进程优雅退出。
package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strconv"
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

	// 超时提醒 ticker（M2）：notify 未启用时不扫描——先记 reminded 再发信，
	// 若在无通道时扫描会把提醒静默吞掉。
	scanCtx, stopScan := context.WithCancel(context.Background())
	defer stopScan()
	if notify.Enabled() {
		go runTimeoutRemindLoop(scanCtx, st, notify, cfg.TimeoutScanInterval)
		log.Printf("bpm timeout-remind: enabled, interval=%s", cfg.TimeoutScanInterval)
	} else {
		log.Printf("bpm timeout-remind: disabled (notify not enabled)")
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
	stopScan()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_ = httpSrv.Shutdown(ctx)
}

// runTimeoutRemindLoop 扫描 pending 且 timeout_at 已到、未提醒过的任务，
// 逐条发 bpm.task_timeout 站内信并回填 reminded_at（条件更新保证只提醒
// 一次），写 timeout_remind 日志（operator=0）。ctx 取消即退出。
func runTimeoutRemindLoop(ctx context.Context, st *store.Store, notify *notifyclient.Client, interval time.Duration) {
	// 错峰启动，等 notify-service 起来（错峰惯例）
	select {
	case <-time.After(20 * time.Second):
	case <-ctx.Done():
		return
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		scanTimeoutDue(ctx, st, notify)
		select {
		case <-ticker.C:
		case <-ctx.Done():
			return
		}
	}
}

func scanTimeoutDue(ctx context.Context, st *store.Store, notify *notifyclient.Client) {
	rows, err := st.ListTimeoutDue(100)
	if err != nil {
		log.Printf("bpm timeout-remind list: %v", err)
		return
	}
	if len(rows) == 0 {
		return
	}
	sendCtx, cancel := context.WithTimeout(ctx, 45*time.Second)
	defer cancel()
	reminded, failed := 0, 0
	for _, row := range rows {
		hours := int(row.TimeoutAt.Sub(row.CreatedAt).Hours() + 0.5)
		if hours < 1 {
			hours = 1
		}
		first, err := st.MarkTaskReminded(row, hours)
		if err != nil {
			failed++
			log.Printf("bpm timeout-remind mark task=%d: %v", row.ID, err)
			continue
		}
		if !first { // 并发/重复扫描已提醒过
			continue
		}
		reminded++
		taskID := strconv.FormatUint(row.ID, 10)
		if _, err := notify.Send(sendCtx, notifyclient.SendInput{
			TenantID:     row.TenantID,
			UserID:       row.AssigneeID,
			TemplateCode: "bpm.task_timeout",
			Type:         "bpm",
			RefType:      "bpm_task",
			RefID:        taskID,
			Vars: map[string]string{
				"instance_title": row.InstanceTitle,
				"node_name":      row.NodeName,
				"hours":          strconv.Itoa(hours),
				"task_id":        taskID,
			},
			Title:   "审批超时提醒：" + row.InstanceTitle,
			Content: "节点「" + row.NodeName + "」的待办已超过 " + strconv.Itoa(hours) + " 小时未处理，请尽快审批",
			Link:    "/bpm/todo?taskId=" + taskID,
		}); err != nil {
			// 通知失败不回滚 reminded_at（notify 自带 24h 去重，重发意义有限；
			// 与全仓"通知失败不阻断"惯例一致），仅日志。
			log.Printf("bpm timeout-remind notify task=%d: %v", row.ID, err)
		}
	}
	if reminded > 0 || failed > 0 {
		log.Printf("bpm timeout-remind: scanned=%d reminded=%d failed=%d", len(rows), reminded, failed)
	}
}
