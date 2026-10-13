package main

import (
	"github.com/go-admin-kit/services/shared/pkg/observability"

	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/go-admin-kit/services/pay/internal/api/pay"
	"github.com/go-admin-kit/services/pay/internal/channel"
	"github.com/go-admin-kit/services/pay/internal/config"
	"github.com/go-admin-kit/services/pay/internal/model"
	"github.com/go-admin-kit/services/pay/internal/store/pay"
	sharedaudit "github.com/go-admin-kit/services/shared/pkg/audittrail"
	"github.com/go-admin-kit/services/shared/pkg/graceful"
	"github.com/go-admin-kit/services/shared/pkg/jobbeat"
	"github.com/go-admin-kit/services/shared/pkg/metrics"
)

func main() {
	cfg := config.Load()
	if cfg.AppEnv != "development" {
		gin.SetMode(gin.ReleaseMode)
	}
	if cfg.AppEnv == "production" && cfg.InternalToken == "" {
		log.Printf("pay internal endpoints: disabled (set PAY_INTERNAL_TOKEN to enable)")
	}

	st, err := store.Open(cfg.DSN())
	if err != nil {
		log.Fatalf("db: %v", err)
	}

	// 数据变更审计：订单/退款/渠道为单行写审计目标。管理面写路径在
	// WithContext(ctx)+Transaction 内执行（见 internal/store），插件随事务
	// 同生共死；后台批量循环（关单/回调）无 actor 上下文，插件静默跳过。
	if err := sharedaudit.Register(st.DB(), sharedaudit.Config{Targets: []sharedaudit.Target{
		{Model: &model.Order{}, Table: "pay_orders", TargetType: "pay_order", TenantField: "tenant_id",
			SnapshotFields: []string{"id", "tenant_id", "order_no", "order_token", "biz_type", "biz_id", "subject", "amount_cents", "refunded_cents", "status", "channel_id", "channel_code", "channel_trade_no", "paid_at", "expire_at", "created_at", "updated_at"}},
		{Model: &model.Refund{}, Table: "pay_refunds", TargetType: "pay_refund", TenantField: "tenant_id",
			SnapshotFields: []string{"id", "tenant_id", "refund_no", "order_id", "order_no", "amount_cents", "reason", "status", "channel_refund_no", "operator_user_id", "finished_at", "created_at", "updated_at"}},
		{Model: &model.Channel{}, Table: "pay_channels", TargetType: "pay_channel", TenantField: "tenant_id",
			SnapshotFields: []string{"id", "tenant_id", "code", "name", "status", "fee_rate_bps", "credentials_enc", "key_version", "remark", "created_at", "updated_at"},
			FieldMasks:     map[string]string{"credentials_enc": "token"}},
	}}); err != nil {
		log.Fatalf("audit trail plugin registration failed: %v", err)
	}

	// 渠道注册表 = 本部署的真实支付能力。mock 是演示渠道，生产默认不注册，
	// 于是渠道下拉、收银台渠道列表、模拟支付端点会同步消失（见 api.RegisterRoutes）。
	adapters := make([]channel.Channel, 0, 1)
	if cfg.DemoMode {
		adapters = append(adapters, channel.Mock{})
	}
	registry := channel.NewRegistry(adapters...)
	if cfg.DemoMode && cfg.ProductionMode() {
		log.Printf("WARNING: PAY_DEMO_MODE is on under APP_ENV=production — 收银台模拟支付端点已挂载，任何拿到收银台链接的人都能把订单标成已支付")
	}
	if len(registry.Codes()) == 0 {
		log.Printf("pay channels: none registered — 微信/支付宝 adapter 尚未实现，本部署无法收款（演示环境可设 PAY_DEMO_MODE=true 启用 mock 渠道）")
	}
	srv := &api.Server{
		Store: st, Secret: cfg.JWTSecret, Registry: registry,
		InternalToken: cfg.InternalToken, OrderTTL: cfg.OrderTTL,
	}

	// 补偿循环：过期关单 + 业务回调投递（SKIP LOCKED 抢单，多副本安全）
	loopCtx, loopCancel := context.WithCancel(context.Background())
	go runExpireLoop(loopCtx, st)
	go runCallbackLoop(loopCtx, st, cfg)

	// OpenTelemetry tracing (noop when TRACING_ENABLED != "true").
	shutdownTracing, err := observability.InitTracerFromEnv(context.Background(), "pay")
	if err != nil {
		log.Printf("tracing init: %v", err)
	}

	r := gin.New()
	r.Use(observability.GinTracing("pay", "request_id"))
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
	srv.RegisterRoutes(r)

	httpSrv := &http.Server{
		Addr:              ":" + cfg.AppPort,
		Handler:           r,
		ReadHeaderTimeout: 10 * time.Second,
	}
	go func() {
		log.Printf("pay-service listening on :%s (channels: %v)", cfg.AppPort, registry.Codes())
		if err := httpSrv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("listen: %v", err)
		}
	}()

	sh := graceful.New(graceful.WithTimeout(10 * time.Second))
	sh.Register("tracing", func(ctx context.Context) error {
		return shutdownTracing(ctx)
	})
	sh.Register("workers", func(ctx context.Context) error {
		loopCancel()
		return nil
	})
	sh.Register("http", func(ctx context.Context) error {
		return httpSrv.Shutdown(ctx)
	})
	if err := sh.WaitAndShutdown(); err != nil {
		log.Printf("pay graceful shutdown: %v", err)
	}
}

// runExpireLoop 每分钟批量关过期单。
func runExpireLoop(ctx context.Context, st *store.Store) {
	select {
	case <-time.After(20 * time.Second):
	case <-ctx.Done():
		return
	}
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()
	for {
		start := time.Now()
		closed, err := st.ExpireDueOrders(200)
		if closed > 0 || err != nil {
			log.Printf("pay expire-loop: closed=%d err=%v", closed, err)
		}
		jobbeat.Report(st.DB(), jobbeat.Run{
			Key: "pay.expire_orders", Service: "pay-service",
			Description: "过期支付单批量关单", IntervalSec: 60,
			StartedAt: start, Err: err,
		})
		select {
		case <-ticker.C:
		case <-ctx.Done():
			return
		}
	}
}

// runCallbackLoop 业务终态回调投递：每 15s 抢一批到期任务，POST 业务方注册端点。
func runCallbackLoop(ctx context.Context, st *store.Store, cfg config.Config) {
	select {
	case <-time.After(10 * time.Second):
	case <-ctx.Done():
		return
	}
	client := &http.Client{Timeout: 10 * time.Second}
	ticker := time.NewTicker(15 * time.Second)
	defer ticker.Stop()
	for {
		start := time.Now()
		err := deliverCallbacksOnce(ctx, st, cfg, client)
		jobbeat.Report(st.DB(), jobbeat.Run{
			Key: "pay.biz_callbacks", Service: "pay-service",
			Description: "支付终态业务回调投递", IntervalSec: 15,
			StartedAt: start, Err: err,
		})
		select {
		case <-ticker.C:
		case <-ctx.Done():
			return
		}
	}
}

func deliverCallbacksOnce(ctx context.Context, st *store.Store, cfg config.Config, client *http.Client) error {
	jobs, err := st.ClaimCallbackJobs(50, 5*time.Minute)
	if err != nil {
		log.Printf("pay callback-loop claim: %v", err)
		return err
	}
	for i := range jobs {
		deliverOne(ctx, st, cfg, client, &jobs[i])
	}
	return nil
}

func deliverOne(ctx context.Context, st *store.Store, cfg config.Config, client *http.Client, job *model.CallbackJob) {
	var o model.Order
	if err := st.DB().First(&o, job.OrderID).Error; err != nil {
		_ = st.RetryCallbackJob(job.ID, job.Attempts+1, cfg.CallbackMaxAttempts, "order load: "+err.Error())
		return
	}
	target := cfg.CallbackTargets[o.BizType]
	if target == "" {
		_ = st.RetryCallbackJob(job.ID, job.Attempts+1, cfg.CallbackMaxAttempts,
			"callback target not registered for biz_type="+o.BizType)
		return
	}

	payload := map[string]any{
		"kind":           job.Kind,
		"tenant_id":      o.TenantID,
		"biz_type":       o.BizType,
		"biz_id":         o.BizID,
		"order_no":       o.OrderNo,
		"status":         o.Status,
		"amount_cents":   o.AmountCents,
		"refunded_cents": o.RefundedCents,
		"paid_at":        o.PaidAt,
	}
	if job.Kind == "refund" && job.RefundID > 0 {
		var rf model.Refund
		if err := st.DB().First(&rf, job.RefundID).Error; err == nil {
			payload["refund_no"] = rf.RefundNo
			payload["refund_status"] = rf.Status
			payload["refund_amount_cents"] = rf.AmountCents
		}
	}
	body, _ := json.Marshal(payload)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, target, bytes.NewReader(body))
	if err != nil {
		_ = st.RetryCallbackJob(job.ID, job.Attempts+1, cfg.CallbackMaxAttempts, err.Error())
		return
	}
	req.Header.Set("Content-Type", "application/json")
	if cfg.CallbackToken != "" {
		req.Header.Set("X-Internal-Token", cfg.CallbackToken)
	}
	resp, err := client.Do(req)
	if err != nil {
		_ = st.RetryCallbackJob(job.ID, job.Attempts+1, cfg.CallbackMaxAttempts, err.Error())
		return
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 4096))
	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		_ = st.FinishCallbackJob(job.ID)
		return
	}
	_ = st.RetryCallbackJob(job.ID, job.Attempts+1, cfg.CallbackMaxAttempts,
		fmt.Sprintf("http %d", resp.StatusCode))
}
