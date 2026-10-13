package main

import (
	"github.com/go-admin-kit/services/shared/pkg/observability"

	"context"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	notifyv1 "github.com/go-admin-kit/services/api/gen/notify/v1"
	"github.com/go-admin-kit/services/notify/internal/api/notify"
	"github.com/go-admin-kit/services/notify/internal/channel"
	"github.com/go-admin-kit/services/notify/internal/config"
	"github.com/go-admin-kit/services/notify/internal/model"
	"github.com/go-admin-kit/services/notify/internal/runtimeconfig"
	"github.com/go-admin-kit/services/notify/internal/store/notify"
	sharedaudit "github.com/go-admin-kit/services/shared/pkg/audittrail"
	"github.com/go-admin-kit/services/shared/pkg/graceful"
	"github.com/go-admin-kit/services/shared/pkg/grpcx"
	"github.com/go-admin-kit/services/shared/pkg/metrics"
)

// startNotifyGRPC 启动 notify 发信 gRPC 服务并注册 Consul（Phase 3）。
// ctx 取消时 GracefulStop 并注销 Consul。
func startNotifyGRPC(ctx context.Context, cfg config.Config, st *store.Store, srv *api.Server) {
	port := 9085
	if v := os.Getenv("GRPC_PORT"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			port = n
		}
	}
	lis, err := net.Listen("tcp", fmt.Sprintf(":%d", port))
	if err != nil {
		log.Printf("notify grpc listen failed: %v", err)
		return
	}
	grpcSrv := grpcx.NewServer()
	notifyv1.RegisterNotifyServiceServer(grpcSrv, api.NewNotifyGRPC(srv))
	go func() { _ = grpcSrv.Serve(lis) }()

	var deregister func()
	if consulAddr := os.Getenv("CONSUL_ADDR"); consulAddr != "disabled" {
		host := grpcx.LocalIP()
		if host == "" {
			host = "127.0.0.1"
		}
		if d, regErr := grpcx.Register(consulAddr, grpcx.Instance{
			ServiceName: "notify-service",
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

	st, err := store.Open(cfg.DSN(), cfg.DefaultChannels)
	if err != nil {
		log.Fatalf("db: %v", err)
	}
	st.SetIdentity(cfg.IdentityAPIBase, cfg.IdentityInternalToken)

	// B3：模板管理写审计（消息/投递日志体量大，不注册）
	if err := sharedaudit.Register(st.DB(), sharedaudit.Config{Targets: []sharedaudit.Target{
		{Model: &model.Template{}, Table: "notify_templates", TargetType: "notify_template", TenantField: "tenant_id",
			SnapshotFields: []string{"ID", "TenantID", "Code", "TitleTpl", "BodyTpl", "LinkTpl", "Channel", "Channels", "Enabled", "CreatedAt", "UpdatedAt"}},
	}}); err != nil {
		log.Fatalf("audit trail: %v", err)
	}

	// SMTP 账号 DB 热配置（system_settings key=notify.smtp）：控制台改配置
	// 30s 内热生效；env（NOTIFY_SMTP_*）兜底（邮件管理面，路线图第 12 项）。
	smtpReader := runtimeconfig.NewSMTPReader(st.DB(), channel.SMTPConfig{
		Host:     cfg.SMTPHost,
		Port:     cfg.SMTPPort,
		User:     cfg.SMTPUser,
		Password: cfg.SMTPPassword,
		From:     cfg.SMTPFrom,
	}, 30*time.Second)
	// system-service 统一持有供应商配置、模板和发送日志；未配置内部契约时，
	// 演示环境仍可显式使用只写日志的 stub。
	smsMode := cfg.EffectiveSMSMode()
	if smsMode != cfg.SMSMode {
		log.Printf("WARNING: NOTIFY_SMS_MODE=%s ignored — 短信 stub 只写日志、不投递真实短信，非演示环境不启用（演示站可设 NOTIFY_DEMO_MODE=true）；短信通道将如实报 skipped", cfg.SMSMode)
	}
	var smsSender channel.SMSSender = channel.NewSystemSMS(cfg.SystemSMSURL, cfg.SystemSMSToken, &http.Client{Timeout: 10 * time.Second})
	if !smsSender.Enabled() {
		smsSender = channel.NewStubSMS(smsMode)
	}
	disp := &channel.Dispatcher{
		Email: channel.NewDynamicSMTPEmail(smtpReader.Current),
		SMS:   smsSender,
		Voice: channel.NewCCVoice(cfg.CCAPIBase, cfg.CCInternalToken),
	}
	log.Printf("notify channels: email=%v sms=%v voice=%v provider=%s", disp.Email.Enabled(), disp.SMS.Enabled(), disp.Voice.Enabled(), disp.SMS.Name())

	// 外发队列：email/sms 的阻塞外呼从 internal/send 请求路径移出，异步跑。
	outbox := api.NewOutbox(st, disp, api.OutboxOptions{
		Workers:   cfg.OutboxWorkers,
		QueueSize: cfg.OutboxQueueSize,
	})

	srv := &api.Server{
		Store:         st,
		Secret:        cfg.JWTSecret,
		InternalToken: cfg.InternalToken,
		AlertUserID:   cfg.AlertUserID,
		Dispatcher:    disp,
		Outbox:        outbox,
	}

	// OpenTelemetry tracing (noop when TRACING_ENABLED != "true").
	shutdownTracing, err := observability.InitTracerFromEnv(context.Background(), "notify")
	if err != nil {
		log.Printf("tracing init: %v", err)
	}

	lifecycleCtx, stopLifecycle := context.WithCancel(context.Background())

	r := gin.New()
	r.Use(observability.GinTracing("notify", "request_id"))
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

	// Phase 3：notify 发信 gRPC server（经 Consul 发现，供 bpm/ticket/crm/im/cc/pay 调）。
	go startNotifyGRPC(lifecycleCtx, cfg, st, srv)

	httpSrv := &http.Server{
		Addr:              ":" + cfg.AppPort,
		Handler:           r,
		ReadHeaderTimeout: 10 * time.Second,
	}
	go func() {
		log.Printf("notify-service listening on :%s (internal=%v)", cfg.AppPort, cfg.InternalToken != "")
		if err := httpSrv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("listen: %v", err)
		}
	}()

	// LIFO：http → outbox 排空 → gRPC lifecycle → tracing
	sh := graceful.New(graceful.WithTimeout(15 * time.Second))
	sh.Register("tracing", func(ctx context.Context) error {
		return shutdownTracing(ctx)
	})
	sh.Register("grpc-lifecycle", func(ctx context.Context) error {
		stopLifecycle()
		return nil
	})
	sh.Register("outbox", func(ctx context.Context) error {
		return outbox.Shutdown(ctx)
	})
	sh.Register("http", func(ctx context.Context) error {
		return httpSrv.Shutdown(ctx)
	})
	if err := sh.WaitAndShutdown(); err != nil {
		log.Printf("notify graceful shutdown: %v", err)
	}
}
