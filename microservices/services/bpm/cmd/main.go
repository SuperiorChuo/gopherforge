// bpm-service 入口：轻量审批流引擎（M2）。
// 定义/实例/任务/日志 AutoMigrate 自管表；推进为同步事务内函数调用；
// 终态经 HTTP 回调业务方（BPM_CALLBACK_<BIZTYPE> 注册）；站内信经
// notify internal API（未配 token 静默跳过）。超时提醒 ticker（M2）：
// 与 crm followup-due / ticket overdue 扫描同构，周期由
// BPM_TIMEOUT_SCAN_INTERVAL 控制（默认 5m），随进程优雅退出。
package main

import (
	"github.com/go-admin-kit/services/shared/pkg/observability"

	"context"
	"encoding/json"
	"fmt"
	bpmv1 "github.com/go-admin-kit/services/api/gen/bpm/v1"
	"github.com/go-admin-kit/services/shared/pkg/grpcx"
	"log"
	"net"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/go-admin-kit/services/bpm/internal/api"
	"github.com/go-admin-kit/services/bpm/internal/callback"
	"github.com/go-admin-kit/services/bpm/internal/config"
	"github.com/go-admin-kit/services/bpm/internal/engine"
	"github.com/go-admin-kit/services/bpm/internal/model"
	"github.com/go-admin-kit/services/bpm/internal/store"
	sharedaudit "github.com/go-admin-kit/services/shared/pkg/audittrail"
	"github.com/go-admin-kit/services/shared/pkg/graceful"
	"github.com/go-admin-kit/services/shared/pkg/idempotency"
	"github.com/go-admin-kit/services/shared/pkg/jobbeat"
	"github.com/go-admin-kit/services/shared/pkg/metrics"
	"github.com/go-admin-kit/services/shared/pkg/notifyclient"
)

// startBpmGRPC 启动 bpm 发起审批 gRPC 服务并注册 Consul（Phase 3）。
// ctx 取消时 GracefulStop 并注销 Consul。
func startBpmGRPC(ctx context.Context, cfg config.Config, srv *api.Server) {
	port := 9086
	if v := os.Getenv("GRPC_PORT"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			port = n
		}
	}
	lis, err := net.Listen("tcp", fmt.Sprintf(":%d", port))
	if err != nil {
		log.Printf("bpm grpc listen failed: %v", err)
		return
	}
	grpcSrv := grpcx.NewServer()
	bpmv1.RegisterBpmServiceServer(grpcSrv, api.NewBpmGRPC(srv))
	go func() { _ = grpcSrv.Serve(lis) }()

	var deregister func()
	if consulAddr := os.Getenv("CONSUL_ADDR"); consulAddr != "disabled" {
		host := grpcx.LocalIP()
		if host == "" {
			host = "127.0.0.1"
		}
		if d, regErr := grpcx.Register(consulAddr, grpcx.Instance{
			ServiceName: "bpm-service",
			Host:        host,
			Port:        port,
		}); regErr == nil {
			deregister = d
		}
	}
	<-ctx.Done()
	grpcSrv.GracefulStop()
	if deregister != nil {
		deregister()
	}
}

func main() {
	cfg := config.Load()
	if cfg.AppEnv != "development" {
		gin.SetMode(gin.ReleaseMode)
	}

	st, err := store.Open(cfg.DSN())
	if err != nil {
		log.Fatalf("db: %v", err)
	}

	// 数据变更审计：流程定义为单行写审计目标（管理面 Create/Update/Publish/
	// NewVersion/Suspend 均已带 ctx + 事务）。实例/任务由引擎在事务内批量推进
	// （AND 会签一次多条 Task），撞 MaxRows=1 安全模型，刻意不注册。
	if err := sharedaudit.Register(st.DB(), sharedaudit.Config{Targets: []sharedaudit.Target{
		{Model: &model.ProcessDefinition{}, Table: "bpm_process_definition", TargetType: "bpm_definition", TenantField: "tenant_id",
			SnapshotFields: []string{"ID", "TenantID", "Key", "Name", "Version", "Status", "BizType", "Remark"}},
	}}); err != nil {
		log.Fatalf("audit trail plugin registration failed: %v", err)
	}

	notify := notifyclient.New(cfg.NotifyAPIBase, cfg.NotifyInternalToken)
	if notify.Enabled() {
		log.Printf("bpm notify: enabled → %s", cfg.NotifyAPIBase)
	} else {
		log.Printf("bpm notify: disabled (set NOTIFY_INTERNAL_TOKEN to enable)")
	}

	cb := callback.New(callback.TargetsFromEnv(), cfg.CallbackToken)
	log.Printf("bpm callback: %d biz_type target(s) registered", cb.Targets())

	eng := engine.New(st.DB())
	eng.SetIdentity(cfg.IdentityAPIBase, cfg.IdentityInternalToken)
	srv := &api.Server{
		Store:         st,
		Engine:        eng,
		Secret:        cfg.JWTSecret,
		InternalToken: cfg.InternalToken,
		Notify:        notify,
	}

	// 超时 ticker（收官项升级）：常开——自动通过/拒绝不依赖通知通道；
	// 提醒策略的任务仅在 notify 启用时记录并发信（无通道不吞提醒，留待
	// 通道开启后补发）。
	scanCtx, stopScan := context.WithCancel(context.Background())
	go runTimeoutLoop(scanCtx, srv, st, notify, cfg.TimeoutScanInterval)
	log.Printf("bpm timeout: scan enabled, interval=%s remind=%v", cfg.TimeoutScanInterval, notify.Enabled())
	go runCallbackLoop(scanCtx, st, cb)
	log.Printf("bpm callback: persistent worker enabled")

	// OpenTelemetry tracing (noop when TRACING_ENABLED != "true").
	shutdownTracing, err := observability.InitTracerFromEnv(context.Background(), "bpm")
	if err != nil {
		log.Printf("tracing init: %v", err)
	}

	r := gin.New()
	r.Use(observability.GinTracing("bpm", "request_id"))
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

	// Phase 3：bpm 发起审批 gRPC server（经 Consul 发现，供 crm 调）。
	go startBpmGRPC(scanCtx, cfg, srv)

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

	// LIFO：http → workers/gRPC → tracing
	sh := graceful.New(graceful.WithTimeout(10 * time.Second))
	sh.Register("tracing", func(ctx context.Context) error {
		return shutdownTracing(ctx)
	})
	sh.Register("workers", func(ctx context.Context) error {
		stopScan()
		return nil
	})
	sh.Register("http", func(ctx context.Context) error {
		return httpSrv.Shutdown(ctx)
	})
	if err := sh.WaitAndShutdown(); err != nil {
		log.Printf("bpm graceful shutdown: %v", err)
	}
}

const callbackMaxAttempts = 8

func runCallbackLoop(ctx context.Context, st *store.Store, dispatcher *callback.Dispatcher) {
	ticker := time.NewTicker(15 * time.Second)
	defer ticker.Stop()
	// 幂等键清理不必每 15s：累计到约 1h 跑一次
	var sincePurge time.Time
	for {
		start := time.Now()
		err := deliverCallbacksOnce(ctx, st, dispatcher)
		jobbeat.Report(st.DB(), jobbeat.Run{
			Key: "bpm.biz_callbacks", Service: "bpm-service",
			Description: "审批终态业务回调投递", IntervalSec: 15,
			StartedAt: start, Err: err,
		})
		if sincePurge.IsZero() || time.Since(sincePurge) >= time.Hour {
			if n, perr := idempotency.PurgeOlderThan(st.DB(), "bpm_idempotency_key", idempotency.DefaultTTL); perr != nil {
				log.Printf("bpm idempotency purge: %v", perr)
			} else if n > 0 {
				log.Printf("bpm idempotency purge: deleted %d keys", n)
			}
			sincePurge = time.Now()
		}
		select {
		case <-ticker.C:
		case <-ctx.Done():
			return
		}
	}
}

func deliverCallbacksOnce(ctx context.Context, st *store.Store, dispatcher *callback.Dispatcher) error {
	jobs, err := st.ClaimCallbackJobs(50, 5*time.Minute)
	if err != nil {
		return err
	}
	var firstErr error
	for i := range jobs {
		job := &jobs[i]
		var inst model.ProcessInstance
		if err := st.DB().Where("id = ? AND tenant_id = ?", job.InstanceID, job.TenantID).
			First(&inst).Error; err != nil {
			if firstErr == nil {
				firstErr = err
			}
			_ = st.RetryCallbackJob(job.ID, job.Attempts+1, callbackMaxAttempts, "instance load: "+err.Error())
			continue
		}
		finishedAt := ""
		if inst.FinishedAt != nil {
			finishedAt = inst.FinishedAt.Format(time.RFC3339)
		}
		err := dispatcher.Deliver(ctx, inst.TenantID, callback.Payload{
			InstanceID: inst.ID, DefinitionKey: inst.DefinitionKey,
			BizType: inst.BizType, BizID: inst.BizID, Result: inst.Status,
			FormSnapshot: json.RawMessage(inst.FormSnapshot), FinishedAt: finishedAt,
		})
		if err == nil {
			if finishErr := st.FinishCallbackJob(job.ID); finishErr != nil && firstErr == nil {
				firstErr = finishErr
			}
			continue
		}
		log.Printf("bpm callback: 投递失败 instance=%d biz=%s/%s attempt=%d: %v",
			inst.ID, inst.BizType, inst.BizID, job.Attempts+1, err)
		if firstErr == nil {
			firstErr = err
		}
		_ = st.RetryCallbackJob(job.ID, job.Attempts+1, callbackMaxAttempts, err.Error())
	}
	return firstErr
}

// runTimeoutLoop 扫描 pending 且 timeout_at 已到、未提醒过的任务：按节点
// timeoutAction 分派——remind 记 reminded_at 并发 bpm.task_timeout（notify
// 未启用则跳过留待补发）；auto_pass/auto_reject 由引擎以系统身份执行，
// 效果经 applyEffects 完整分发（下一节点待办通知 + 终态回调）。
func runTimeoutLoop(ctx context.Context, srv *api.Server, st *store.Store, notify *notifyclient.Client, interval time.Duration) {
	// 错峰启动，等 notify-service 起来（与 crm/ticket 同惯例）
	select {
	case <-time.After(20 * time.Second):
	case <-ctx.Done():
		return
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		start := time.Now()
		scanTimeoutDue(ctx, srv, st, notify)
		jobbeat.Report(st.DB(), jobbeat.Run{
			Key: "bpm.timeout_scan", Service: "bpm-service",
			Description: "审批任务超时扫描（提醒/自动通过/自动拒绝）",
			IntervalSec: int64(interval / time.Second), StartedAt: start,
		})
		select {
		case <-ticker.C:
		case <-ctx.Done():
			return
		}
	}
}

func scanTimeoutDue(ctx context.Context, srv *api.Server, st *store.Store, notify *notifyclient.Client) {
	rows, err := st.ListTimeoutDue(100)
	if err != nil {
		log.Printf("bpm timeout list: %v", err)
		return
	}
	if len(rows) == 0 {
		return
	}
	sendCtx, cancel := context.WithTimeout(ctx, 45*time.Second)
	defer cancel()
	reminded, acted, failed := 0, 0, 0
	for _, row := range rows {
		outcome, eff, err := srv.Engine.HandleTimeout(row.TenantID, row.ID)
		if err != nil {
			failed++
			log.Printf("bpm timeout handle task=%d: %v", row.ID, err)
			continue
		}
		switch outcome {
		case engine.TimeoutOutcomePass, engine.TimeoutOutcomeReject:
			acted++
			srv.ApplyEffects(eff)
			continue
		case engine.TimeoutOutcomeSkip:
			continue
		}
		// remind：notify 未启用时不记 reminded_at（通道开启后补发）
		if !notify.Enabled() {
			continue
		}
		hours := int(row.TimeoutAt.Sub(row.CreatedAt).Hours() + 0.5)
		if hours < 1 {
			hours = 1
		}
		first, err := st.MarkTaskReminded(row, hours)
		if err != nil {
			failed++
			log.Printf("bpm timeout mark task=%d: %v", row.ID, err)
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
			log.Printf("bpm timeout notify task=%d: %v", row.ID, err)
		}
	}
	if reminded > 0 || acted > 0 || failed > 0 {
		log.Printf("bpm timeout: scanned=%d reminded=%d auto_acted=%d failed=%d",
			len(rows), reminded, acted, failed)
	}
}
