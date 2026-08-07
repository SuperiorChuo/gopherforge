package system

import (
	"context"
	"fmt"
	"time"

	dao "github.com/go-admin-kit/services/audit/internal/dao/system"
	"github.com/go-admin-kit/services/audit/internal/model"
	"github.com/go-admin-kit/services/shared/pkg/jobbeat"
	"github.com/go-admin-kit/services/shared/pkg/logger"
	"github.com/go-admin-kit/services/shared/pkg/notifyclient"
	"gorm.io/gorm"
)

// SecurityDetectorOptions tunes the audit-trail anomaly detector.
type SecurityDetectorOptions struct {
	// ScanInterval between detection passes (default 60s).
	ScanInterval time.Duration
	// Window is how far back each pass looks (default 10min).
	Window time.Duration
	// WriteThreshold: same actor writes >= N within window → high_volume_write.
	WriteThreshold int
	// PermissionThreshold: same actor permission actions >= N → permission_storm.
	PermissionThreshold int
	// FailureThreshold: same actor failing actions >= N → failure_burst.
	FailureThreshold int
	// NotifyUserID is the platform admin who receives in-console alerts (default 1).
	NotifyUserID uint
	// NotifyURL is the frontend security-events page link in notifications.
	NotifyURL string
}

func (o *SecurityDetectorOptions) fillDefaults() {
	if o.ScanInterval <= 0 {
		o.ScanInterval = 60 * time.Second
	}
	if o.Window <= 0 {
		o.Window = 10 * time.Minute
	}
	if o.WriteThreshold <= 0 {
		o.WriteThreshold = 20
	}
	if o.PermissionThreshold <= 0 {
		o.PermissionThreshold = 5
	}
	if o.FailureThreshold <= 0 {
		o.FailureThreshold = 10
	}
	if o.NotifyUserID == 0 {
		o.NotifyUserID = 1
	}
	if o.NotifyURL == "" {
		o.NotifyURL = "/system/security-events"
	}
}

// SecurityDetectorService scans recent audit_logs for abnormal patterns and
// records + notifies. Mirrors StartLogRetentionCleaner's lifecycle (ticker +
// jobbeat + ctx cancel).
type SecurityDetectorService struct {
	db         *gorm.DB
	auditDAO   dao.AuditLogDAO
	eventDAO   dao.SecurityEventDAO
	notify     *notifyclient.Client
	opts       SecurityDetectorOptions
	ruleLabels map[string]string
}

func NewSecurityDetectorService(db *gorm.DB, notify *notifyclient.Client, opts SecurityDetectorOptions) *SecurityDetectorService {
	opts.fillDefaults()
	return &SecurityDetectorService{
		db:       db,
		auditDAO: *dao.NewAuditLogDAO(db),
		eventDAO: *dao.NewSecurityEventDAO(db),
		notify:   notify,
		opts:     opts,
		ruleLabels: map[string]string{
			"high_volume_write": "写入操作激增",
			"permission_storm":  "权限变更风暴",
			"failure_burst":     "失败操作激增",
		},
	}
}

// StartSecurityEventDetector launches the background detector.
func StartSecurityEventDetector(ctx context.Context, db *gorm.DB, notify *notifyclient.Client, opts SecurityDetectorOptions) *SecurityDetectorService {
	svc := NewSecurityDetectorService(db, notify, opts)
	go svc.run(ctx)
	return svc
}

func (s *SecurityDetectorService) run(ctx context.Context) {
	s.detect(ctx)
	ticker := time.NewTicker(s.opts.ScanInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.detect(ctx)
		}
	}
}

func (s *SecurityDetectorService) detect(ctx context.Context) {
	now := time.Now()
	from := now.Add(-s.opts.Window)

	// Rule 1: high-volume writes.
	if rows, err := s.auditDAO.CountActorActionsWithinContext(ctx, from, now,
		"action NOT LIKE 'read%' AND action NOT LIKE 'get%' AND action NOT LIKE 'list%' AND action NOT LIKE 'query%' AND action NOT LIKE 'check%' AND action NOT LIKE 'export%'"); err == nil {
		s.evaluateCounts(ctx, "high_volume_write", "warning", s.opts.WriteThreshold, rows)
	} else {
		logDetectError("high_volume_write", err)
	}

	// Rule 2: permission changes.
	if rows, err := s.auditDAO.CountActorActionsWithinContext(ctx, from, now,
		"action ILIKE 'assign%' OR action ILIKE 'grant%' OR action ILIKE 'revoke%' OR action ILIKE 'delete role%' OR action ILIKE 'delete permission%' OR action ILIKE 'bind%'"); err == nil {
		s.evaluateCounts(ctx, "permission_storm", "critical", s.opts.PermissionThreshold, rows)
	} else {
		logDetectError("permission_storm", err)
	}

	// Rule 3: failure burst.
	if rows, err := s.auditDAO.CountActorActionsWithinContext(ctx, from, now,
		"action ILIKE '%fail%' OR action ILIKE '%denied%' OR action ILIKE '%error%' OR summary ILIKE '%fail%' OR summary ILIKE '%denied%'"); err == nil {
		s.evaluateCounts(ctx, "failure_burst", "warning", s.opts.FailureThreshold, rows)
	} else {
		logDetectError("failure_burst", err)
	}

	// Heartbeat so the ops job center can see the detector is alive.
	jobbeat.Report(s.db, jobbeat.Run{
		Key:         "audit.security_detector",
		Service:     "audit-service",
		Description: "审计异常模式检测（写入激增/权限风暴/失败激增）",
		IntervalSec: int64(s.opts.ScanInterval.Seconds()),
		StartedAt:   now,
	})
}

func (s *SecurityDetectorService) evaluateCounts(ctx context.Context, rule, severity string, threshold int, rows []dao.ActorActionCount) {
	for _, row := range rows {
		if row.ActorID == "" || row.Count < int64(threshold) {
			continue
		}
		// Dedupe: one event+notification per actor per rule per window.
		if hit, err := s.eventDAO.RecentRuleHitContext(ctx, rule, row.ActorID, time.Now().Add(-s.opts.Window)); err == nil && hit {
			continue
		} else if err != nil {
			logDetectError(rule, err)
			continue
		}

		event := &model.SecurityEvent{
			TenantID:   1,
			Rule:       rule,
			Severity:   severity,
			Summary:    fmt.Sprintf("%s：操作者 %s 在 %v 内执行 %d 次%s", s.ruleLabels[rule], row.ActorID, s.opts.Window, row.Count, ruleSuffix(rule)),
			ActorID:    row.ActorID,
			ActorType:  "operator",
			Target:     rule,
			OccurredAt: time.Now(),
		}
		if err := s.eventDAO.CreateContext(ctx, event); err != nil {
			logDetectError(rule, err)
			continue
		}
		s.notifyEvent(ctx, event)
	}
}

func (s *SecurityDetectorService) notifyEvent(ctx context.Context, event *model.SecurityEvent) {
	if s.notify == nil || !s.notify.Enabled() {
		return
	}
	if _, err := s.notify.Send(ctx, notifyclient.SendInput{
		TenantID:     uint64(event.TenantID),
		UserID:       uint64(s.opts.NotifyUserID),
		TemplateCode: "security_alert",
		Type:         "security",
		RefType:      "security_event",
		RefID:        fmt.Sprintf("%d", event.ID),
		Title:        fmt.Sprintf("[安全事件] %s", s.ruleLabels[event.Rule]),
		Content:      event.Summary,
		Link:         s.opts.NotifyURL,
	}); err != nil && logger.Logger != nil {
		logger.Warn("security event notify failed", logger.String("rule", event.Rule), logger.Err(err))
	}
	_ = s.eventDAO.MarkNotifiedContext(ctx, event.ID)
}

func ruleSuffix(rule string) string {
	switch rule {
	case "high_volume_write":
		return "写入操作"
	case "permission_storm":
		return "权限变更操作"
	case "failure_burst":
		return "失败操作"
	}
	return "操作"
}

func logDetectError(rule string, err error) {
	if logger.Logger != nil {
		logger.Warn("security detector rule failed", logger.String("rule", rule), logger.Err(err))
	}
}
