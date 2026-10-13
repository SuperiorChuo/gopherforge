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
	crmv1 "github.com/go-admin-kit/services/api/gen/crm/v1"
	"github.com/go-admin-kit/services/crm/internal/api/crm"
	"github.com/go-admin-kit/services/crm/internal/bpmclient"
	"github.com/go-admin-kit/services/crm/internal/config"
	"github.com/go-admin-kit/services/crm/internal/model"
	"github.com/go-admin-kit/services/crm/internal/store/crm"
	"github.com/go-admin-kit/services/shared/pkg/auditevents"
	sharedaudit "github.com/go-admin-kit/services/shared/pkg/audittrail"
	"github.com/go-admin-kit/services/shared/pkg/graceful"
	"github.com/go-admin-kit/services/shared/pkg/grpcx"
	"github.com/go-admin-kit/services/shared/pkg/jobbeat"
	"github.com/go-admin-kit/services/shared/pkg/metrics"
	"github.com/go-admin-kit/services/shared/pkg/notifyclient"
	"github.com/go-admin-kit/services/shared/pkg/outbox"
)

// startCrmGRPC 启动 crm 号码识客 gRPC 服务并注册 Consul（Phase 3）。
// ctx 取消时执行 GracefulStop 并注销 Consul 注册，保证优雅退出。
func startCrmGRPC(ctx context.Context, cfg config.Config, st *store.Store, srv *api.Server) {
	port := 9081
	if v := os.Getenv("GRPC_PORT"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			port = n
		}
	}
	lis, err := net.Listen("tcp", fmt.Sprintf(":%d", port))
	if err != nil {
		log.Printf("crm grpc listen failed: %v", err)
		return
	}
	grpcSrv := grpcx.NewServer()
	crmv1.RegisterCrmServiceServer(grpcSrv, api.NewCrmGRPC(st))
	go func() { _ = grpcSrv.Serve(lis) }()

	if consulAddr := os.Getenv("CONSUL_ADDR"); consulAddr != "disabled" {
		host := grpcx.LocalIP()
		if host == "" {
			host = "127.0.0.1"
		}
		if deregister, regErr := grpcx.Register(consulAddr, grpcx.Instance{
			ServiceName: "crm-service",
			Host:        host,
			Port:        port,
		}); regErr == nil {
			defer deregister()
		}
	}

	<-ctx.Done()
	grpcSrv.GracefulStop()
}

func main() {
	cfg := config.Load()
	if cfg.AppEnv != "development" {
		gin.SetMode(gin.ReleaseMode)
	}

	// 主 ctx：关停时取消，驱动后台 goroutine 与 gRPC GracefulStop。
	ctx, stop := context.WithCancel(context.Background())

	st, err := store.Open(cfg.DSN())
	if err != nil {
		log.Fatalf("db: %v", err)
	}
	st.SetIdentity(cfg.IdentityAPIBase, cfg.IdentityInternalToken)

	// Phase 5 试点：CRM 审计变更走事务 Outbox（audit-service worker 投 NATS）。
	// 仅当 public.outbox_events 已迁移才开启，避免未 migrate 时业务写全失败。
	if outbox.TableReady(st.DB()) {
		outbox.EnableTransactional()
		log.Printf("crm audit trail: transactional outbox enabled")
	} else {
		log.Printf("crm audit trail: outbox table not ready（跑 000072 迁移后生效），沿用直发 NATS/直写")
	}

	// 数据变更审计：GORM 插件（与 auth/identity/system 共用 audit_logs）。只对
	// 显式配置的核心 CRM 表做单行写审计；actor/tenant 由 AuditHeaderMiddleware
	// 从 X-Auth-* 头注入请求 ctx，后台/批量写（无 actor）自动跳过。SnapshotFields
	// 为 Go 结构体字段名，Table 必须匹配模型 TableName()——任一不匹配启动即失败。
	if err := sharedaudit.Register(st.DB(), sharedaudit.Config{
		Targets: []sharedaudit.Target{
			{
				Model: &model.Customer{}, Table: "crm_customers", TargetType: "crm_customer", TenantField: "tenant_id",
				SnapshotFields: []string{
					"ID", "TenantID", "Name", "Industry", "Level", "Phone", "Address",
					"Source", "OwnerUserID", "IMVisitorID", "LastFollowAt", "PooledAt",
					"Note", "CreatedAt", "UpdatedAt",
				},
			},
			{
				Model: &model.Lead{}, Table: "crm_leads", TargetType: "crm_lead", TenantField: "tenant_id",
				SnapshotFields: []string{
					"ID", "TenantID", "Name", "Company", "Phone", "Email", "Source",
					"Status", "OwnerUserID", "DepartmentID", "IMVisitorID", "Note",
					"CustomerID", "CreatedAt", "UpdatedAt",
				},
			},
			{
				Model: &model.Contact{}, Table: "crm_contacts", TargetType: "crm_contact", TenantField: "tenant_id",
				SnapshotFields: []string{
					"ID", "TenantID", "CustomerID", "Name", "Title", "Phone", "Email",
					"IsPrimary", "CreatedAt", "UpdatedAt",
				},
			},
			{
				Model: &model.Contract{}, Table: "crm_contracts", TargetType: "crm_contract", TenantField: "tenant_id",
				SnapshotFields: []string{
					"ID", "TenantID", "No", "Title", "CustomerID", "OpportunityID",
					"AmountCents", "Status", "SignedAt", "StartAt", "EndAt", "OwnerUserID",
					"Note", "CreatedAt", "UpdatedAt",
				},
			},
			{
				Model: &model.Payment{}, Table: "crm_payments", TargetType: "crm_payment", TenantField: "tenant_id",
				SnapshotFields: []string{
					"ID", "TenantID", "ContractID", "AmountCents", "PaidAt", "Method",
					"Note", "Status", "ByUserID", "CreatedAt",
				},
			},
			{
				Model: &model.FollowUp{}, Table: "crm_followups", TargetType: "crm_followup", TenantField: "tenant_id",
				SnapshotFields: []string{
					"ID", "TenantID", "TargetType", "TargetID", "Content", "NextAt",
					"ByUserID", "CreatedAt",
				},
			},
			// B3：商机 / 公海规则 / 标签（管理面单行写）
			{
				Model: &model.Opportunity{}, Table: "crm_opportunities", TargetType: "crm_opportunity", TenantField: "tenant_id",
				SnapshotFields: []string{
					"ID", "TenantID", "CustomerID", "Name", "AmountCents", "Stage", "Probability",
					"ExpectedCloseAt", "OwnerUserID", "LostReason", "WonAt", "LostAt", "Note",
					"CreatedAt", "UpdatedAt",
				},
			},
			{
				Model: &model.PoolRule{}, Table: "crm_pool_rules", TargetType: "crm_pool_rule", TenantField: "tenant_id",
				SnapshotFields: []string{
					"ID", "TenantID", "Level", "IdleDays", "ProtectActive", "Enabled",
					"UpdatedBy", "CreatedAt", "UpdatedAt",
				},
			},
			{
				Model: &model.Tag{}, Table: "crm_tags", TargetType: "crm_tag", TenantField: "tenant_id",
				SnapshotFields: []string{"ID", "TenantID", "Name", "Color", "CreatedAt", "UpdatedAt"},
			},
		},
	}); err != nil {
		log.Fatalf("audit trail plugin: %v", err)
	}

	// Phase 2D：审计事件化——init 发布器。
	if err := auditevents.Init(cfg.NATSURL); err != nil {
		log.Printf("audit events publisher init failed（回退直写）: %v", err)
	}

	notify := notifyclient.New(cfg.NotifyAPIBase, cfg.NotifyInternalToken)
	if notify.Enabled() {
		log.Printf("crm notify: enabled → %s", cfg.NotifyAPIBase)
		go runFollowupDueLoop(ctx, st, notify)
	} else {
		log.Printf("crm notify: disabled (set NOTIFY_INTERNAL_TOKEN to enable)")
	}

	bpm := bpmclient.New(cfg.BpmAPIBase, cfg.BpmInternalToken)
	if cfg.BPMEnable && bpm.Enabled() {
		log.Printf("crm bpm: approval enabled → %s (contract flow %s, payment flow %s)",
			cfg.BpmAPIBase, cfg.ContractFlowKey, cfg.PaymentFlowKey)
	} else {
		log.Printf("crm bpm: approval disabled (BPM_ENABLE=%v, client=%v)", cfg.BPMEnable, bpm.Enabled())
	}

	srv := &api.Server{
		Store: st, Secret: cfg.JWTSecret, InternalToken: cfg.InternalToken,
		BPMEnable: cfg.BPMEnable, BPM: bpm,
		ContractFlowKey: cfg.ContractFlowKey, PaymentFlowKey: cfg.PaymentFlowKey,
	}

	// 公海引擎：每小时按租户规则回收超期未跟进客户
	go func() {
		// slight delay so notify-service is up
		select {
		case <-time.After(20 * time.Second):
		case <-ctx.Done():
			return
		}
		ticker := time.NewTicker(time.Hour)
		defer ticker.Stop()
		for {
			tenants, err := st.TenantIDs()
			if err != nil {
				log.Printf("crm pool: tenants: %v", err)
			} else {
				for _, tid := range tenants {
					recycled, err := st.RecycleIdleCustomers(tid, 200)
					if err != nil {
						log.Printf("crm pool: tenant %d recycle: %v", tid, err)
						continue
					}
					if len(recycled) > 0 {
						log.Printf("crm pool: tenant %d recycled %d customers", tid, len(recycled))
						if notify.Enabled() {
							notifyRecycled(notify, recycled)
						}
					}
				}
			}
			// Phase 5：清理 24h 外幂等键
			if n, err := st.PurgeIdempotencyKeys(0); err != nil {
				log.Printf("crm idempotency purge: %v", err)
			} else if n > 0 {
				log.Printf("crm idempotency purge: deleted %d keys", n)
			}
			select {
			case <-ticker.C:
			case <-ctx.Done():
				return
			}
		}
	}()
	log.Printf("crm pool engine: hourly recycle enabled")

	// OpenTelemetry tracing (noop when TRACING_ENABLED != "true").
	shutdownTracing, err := observability.InitTracerFromEnv(context.Background(), "crm")
	if err != nil {
		log.Fatalf("tracing initialization failed: %v", err)
	}

	r := gin.New()
	r.Use(observability.GinTracing("crm", "request_id"))
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

	// Phase 3：crm 号码识客 gRPC server（经 Consul 发现，供 cc 调）。
	go startCrmGRPC(ctx, cfg, st, srv)

	httpSrv := &http.Server{
		Addr:              ":" + cfg.AppPort,
		Handler:           r,
		ReadHeaderTimeout: 10 * time.Second,
	}
	go func() {
		log.Printf("crm-service listening on :%s", cfg.AppPort)
		if err := httpSrv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("listen: %v", err)
		}
	}()

	// LIFO：http → lifecycle（gRPC/workers）→ tracing
	sh := graceful.New(graceful.WithTimeout(10 * time.Second))
	sh.Register("tracing", func(c context.Context) error {
		if shutdownTracing == nil {
			return nil
		}
		return shutdownTracing(c)
	})
	sh.Register("lifecycle", func(c context.Context) error {
		stop()
		return nil
	})
	sh.Register("http", func(c context.Context) error {
		return httpSrv.Shutdown(c)
	})
	if err := sh.WaitAndShutdown(); err != nil {
		log.Printf("crm graceful shutdown: %v", err)
	}
}

func runFollowupDueLoop(ctx context.Context, st *store.Store, notify *notifyclient.Client) {
	select {
	case <-time.After(25 * time.Second):
	case <-ctx.Done():
		return
	}
	ticker := time.NewTicker(3 * time.Minute)
	defer ticker.Stop()
	for {
		start := time.Now()
		scanFollowupDue(st, notify)
		jobbeat.Report(st.DB(), jobbeat.Run{
			Key: "crm.followup_due", Service: "crm-service",
			Description: "CRM 回访到期提醒扫描", IntervalSec: 180, StartedAt: start,
		})
		select {
		case <-ticker.C:
		case <-ctx.Done():
			return
		}
	}
}

func scanFollowupDue(st *store.Store, notify *notifyclient.Client) {
	list, err := st.ListDueFollowUps(100)
	if err != nil {
		log.Printf("crm followup-due list: %v", err)
		return
	}
	if len(list) == 0 {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	created, skipped, failed := 0, 0, 0
	for i := range list {
		f := &list[i]
		uid := f.ByUserID
		if uid == 0 {
			// fallback: customer/lead/opp owner
			uid = ownerOfTarget(st, f.TargetType, f.TargetID, f.TenantID)
		}
		if uid == 0 {
			continue
		}
		snippet := strings.TrimSpace(f.Content)
		if len([]rune(snippet)) > 80 {
			snippet = string([]rune(snippet)[:80]) + "…"
		}
		link := followupLink(f.TargetType, f.TargetID)
		name := f.TargetName
		if name == "" {
			name = f.TargetType
		}
		res, err := notify.Send(ctx, notifyclient.SendInput{
			TenantID:     f.TenantID,
			UserID:       uid,
			TemplateCode: "crm.followup_due",
			Type:         "notice",
			RefType:      "crm_followup",
			RefID:        strconv.FormatUint(f.ID, 10),
			Vars: map[string]string{
				"target_name": name,
				"target_type": f.TargetType,
				"target_id":   strconv.FormatUint(f.TargetID, 10),
				"snippet":     snippet,
				"link":        link,
			},
		})
		if err != nil {
			failed++
			log.Printf("crm followup-due id=%d: %v", f.ID, err)
			continue
		}
		if res != nil {
			created += res.Created
			skipped += res.Skipped
		}
	}
	if created > 0 || failed > 0 {
		log.Printf("crm followup-due: scanned=%d created=%d skipped=%d failed=%d",
			len(list), created, skipped, failed)
	}
}

func ownerOfTarget(st *store.Store, targetType string, targetID, tenantID uint64) uint64 {
	switch targetType {
	case "customer":
		if c, err := st.GetCustomer(targetID, tenantID); err == nil {
			return c.OwnerUserID
		}
	case "lead":
		if l, err := st.GetLead(targetID, tenantID); err == nil {
			return l.OwnerUserID
		}
	case "opportunity":
		if o, err := st.GetOpportunity(targetID, tenantID); err == nil {
			return o.OwnerUserID
		}
	}
	return 0
}

func followupLink(targetType string, targetID uint64) string {
	id := strconv.FormatUint(targetID, 10)
	switch targetType {
	case "customer":
		return "/crm/customers?keyword=" + id
	case "lead":
		return "/crm/leads?keyword=" + id
	case "opportunity":
		return "/crm/pipeline"
	default:
		return "/crm/customers"
	}
}

func notifyRecycled(notify *notifyclient.Client, list []store.RecycledCustomer) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	for _, r := range list {
		if r.FromOwner == 0 {
			continue
		}
		_, err := notify.Send(ctx, notifyclient.SendInput{
			TenantID:     r.TenantID,
			UserID:       r.FromOwner,
			TemplateCode: "crm.customer_recycled",
			Type:         "notice",
			RefType:      "crm_customer_recycle",
			RefID:        strconv.FormatUint(r.ID, 10),
			Vars: map[string]string{
				"name":        r.Name,
				"customer_id": strconv.FormatUint(r.ID, 10),
			},
		})
		if err != nil {
			log.Printf("crm recycle-notify customer=%d: %v", r.ID, err)
		}
	}
}
