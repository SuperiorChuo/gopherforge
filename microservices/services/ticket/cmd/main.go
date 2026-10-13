package main

import (
	"github.com/go-admin-kit/services/shared/pkg/observability"

	"context"
	"encoding/json"
	"log"
	"math"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	sharedaudit "github.com/go-admin-kit/services/shared/pkg/audittrail"
	"github.com/go-admin-kit/services/shared/pkg/graceful"
	"github.com/go-admin-kit/services/shared/pkg/jobbeat"
	"github.com/go-admin-kit/services/shared/pkg/metrics"
	"github.com/go-admin-kit/services/shared/pkg/notifyclient"
	"github.com/go-admin-kit/services/ticket/internal/api/ticket"
	"github.com/go-admin-kit/services/ticket/internal/config"
	"github.com/go-admin-kit/services/ticket/internal/model"
	"github.com/go-admin-kit/services/ticket/internal/store/ticket"
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
	if err := sharedaudit.Register(st.DB(), sharedaudit.Config{
		Targets: []sharedaudit.Target{
			{Model: &model.Ticket{}, Table: "ticket_tickets", TargetType: "ticket", TenantField: "tenant_id",
				SnapshotFields: []string{
					"ID", "TenantID", "TicketNo", "Title", "Description", "Status", "Priority",
					"Source", "CreatorUserID", "AssigneeUserID", "CustomerID", "ConversationPublicID",
					"IMVisitorID", "ContactName", "ContactPhone", "DueAt", "ResolvedAt", "ClosedAt",
					"ResolveNote", "CreatedAt", "UpdatedAt",
				}},
			{Model: &model.TicketEvent{}, Table: "ticket_events", TargetType: "ticket_event", TenantField: "tenant_id",
				SnapshotFields: []string{
					"ID", "TenantID", "TicketID", "Kind", "Content", "FromStatus", "ToStatus",
					"ByUserID", "CreatedAt",
				}},
		},
	}); err != nil {
		log.Fatalf("audit trail: %v", err)
	}

	notify := notifyclient.New(cfg.NotifyAPIBase, cfg.NotifyInternalToken)
	if notify.Enabled() {
		log.Printf("ticket notify: enabled → %s", cfg.NotifyAPIBase)
	} else {
		log.Printf("ticket notify: disabled (set NOTIFY_INTERNAL_TOKEN to enable)")
	}
	loopCtx, loopCancel := context.WithCancel(context.Background())
	if notify.Enabled() {
		go runTicketNotifyLoop(loopCtx, st, notify)
		go runNotificationOutbox(loopCtx, st, notify)
	}

	srv := &api.Server{Store: st, Secret: cfg.JWTSecret, Notify: notify}

	// OpenTelemetry tracing (noop when TRACING_ENABLED != "true").
	shutdownTracing, err := observability.InitTracerFromEnv(context.Background(), "ticket")
	if err != nil {
		log.Printf("tracing init: %v", err)
	}

	r := gin.New()
	r.Use(observability.GinTracing("ticket", "request_id"))
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
		log.Printf("ticket-service listening on :%s", cfg.AppPort)
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
		log.Printf("ticket graceful shutdown: %v", err)
	}
}

// runTicketNotifyLoop: overdue + SLA pre-warning every 2 minutes.
func runTicketNotifyLoop(ctx context.Context, st *store.Store, notify *notifyclient.Client) {
	select {
	case <-time.After(15 * time.Second):
	case <-ctx.Done():
		return
	}
	ticker := time.NewTicker(2 * time.Minute)
	defer ticker.Stop()
	for {
		start := time.Now()
		scanOverdueOnce(st, notify)
		scanDueSoonOnce(st, notify, 4) // 4h window before due
		jobbeat.Report(st.DB(), jobbeat.Run{
			Key: "ticket.sla_notify", Service: "ticket-service",
			Description: "工单超时/SLA 预警扫描", IntervalSec: 120, StartedAt: start,
		})
		select {
		case <-ticker.C:
		case <-ctx.Done():
			return
		}
	}
}

func runNotificationOutbox(ctx context.Context, st *store.Store, notify *notifyclient.Client) {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()
	for {
		jobs, err := st.ClaimNotificationJobs(50, time.Minute)
		if err != nil {
			log.Printf("ticket notify outbox claim: %v", err)
		}
		for i := range jobs {
			job := &jobs[i]
			var vars map[string]string
			if err := json.Unmarshal(job.VarsJSON, &vars); err != nil {
				_ = st.RetryNotificationJob(job.ID, job.Attempts+1, 8, "decode vars: "+err.Error())
				continue
			}
			_, err := notify.Send(ctx, notifyclient.SendInput{TenantID: job.TenantID, UserID: job.UserID,
				TemplateCode: job.TemplateCode, Type: "ticket", RefType: job.RefType, RefID: job.RefID, Vars: vars})
			if err == nil {
				_ = st.FinishNotificationJob(job.ID)
			} else {
				_ = st.RetryNotificationJob(job.ID, job.Attempts+1, 8, err.Error())
			}
		}
		select {
		case <-ticker.C:
		case <-ctx.Done():
			return
		}
	}
}

func ticketNotifyUser(t *model.Ticket) uint64 {
	if t.AssigneeUserID > 0 {
		return t.AssigneeUserID
	}
	return t.CreatorUserID
}

func scanOverdueOnce(st *store.Store, notify *notifyclient.Client) {
	list, err := st.ListOverdueForNotify(100)
	if err != nil {
		log.Printf("ticket overdue-notify list: %v", err)
		return
	}
	if len(list) == 0 {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	created, skipped, failed := 0, 0, 0
	for i := range list {
		t := &list[i]
		uid := ticketNotifyUser(t)
		if uid == 0 {
			continue
		}
		res, err := notify.Send(ctx, notifyclient.SendInput{
			TenantID:     t.TenantID,
			UserID:       uid,
			TemplateCode: "ticket.overdue",
			Type:         "ticket",
			RefType:      "ticket",
			RefID:        strconv.FormatUint(t.ID, 10),
			Vars: map[string]string{
				"ticket_id": strconv.FormatUint(t.ID, 10),
				"ticket_no": t.TicketNo,
				"title":     t.Title,
			},
		})
		if err != nil {
			failed++
			log.Printf("ticket overdue-notify id=%d: %v", t.ID, err)
			continue
		}
		if res != nil {
			created += res.Created
			skipped += res.Skipped
		}
	}
	if created > 0 || failed > 0 {
		log.Printf("ticket overdue-notify: scanned=%d created=%d skipped=%d failed=%d",
			len(list), created, skipped, failed)
	}
}

func scanDueSoonOnce(st *store.Store, notify *notifyclient.Client, withinHours int) {
	list, err := st.ListDueSoonForNotify(withinHours, 100)
	if err != nil {
		log.Printf("ticket sla-warn list: %v", err)
		return
	}
	if len(list) == 0 {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	now := time.Now()
	created, skipped, failed := 0, 0, 0
	for i := range list {
		t := &list[i]
		uid := ticketNotifyUser(t)
		if uid == 0 || t.DueAt == nil {
			continue
		}
		hoursLeft := int(math.Ceil(t.DueAt.Sub(now).Hours()))
		if hoursLeft < 1 {
			hoursLeft = 1
		}
		// ref_id includes window day so re-warn next day if still open; 24h dedupe per day
		dayKey := t.DueAt.Format("20060102")
		res, err := notify.Send(ctx, notifyclient.SendInput{
			TenantID:     t.TenantID,
			UserID:       uid,
			TemplateCode: "ticket.sla_warning",
			Type:         "ticket",
			RefType:      "ticket_sla_warn",
			RefID:        strconv.FormatUint(t.ID, 10) + "-" + dayKey,
			Vars: map[string]string{
				"ticket_id":  strconv.FormatUint(t.ID, 10),
				"ticket_no":  t.TicketNo,
				"title":      t.Title,
				"hours_left": strconv.Itoa(hoursLeft),
			},
		})
		if err != nil {
			failed++
			log.Printf("ticket sla-warn id=%d: %v", t.ID, err)
			continue
		}
		if res != nil {
			created += res.Created
			skipped += res.Skipped
		}
	}
	if created > 0 || failed > 0 {
		log.Printf("ticket sla-warn: scanned=%d created=%d skipped=%d failed=%d",
			len(list), created, skipped, failed)
	}
}
