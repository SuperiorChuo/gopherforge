// Package events 审计事件消费者（Phase 2D）：订阅 audit.log.>，写 audit_svc。
//
// 使用 shared/pkg/natsx 实现 JetStream 持久化消费 + DLQ：
//   - 手动 ACK：处理成功后 Ack()，失败时 Nak() 触发重试
//   - MaxDeliver：超过重试上限后自动转入 DLQ stream（audit_events_dlq）
//   - 幂等：按 (tenant, actor, action, target, created_at) 去重
package events

import (
	"context"
	"encoding/json"
	"time"

	"github.com/go-admin-kit/services/audit/internal/dao/system"
	localmodel "github.com/go-admin-kit/services/audit/internal/model"
	"github.com/go-admin-kit/services/shared/pkg/auditevents"
	"github.com/go-admin-kit/services/shared/pkg/logger"
	"github.com/go-admin-kit/services/shared/pkg/natsx"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

func StartConsumer(ctx context.Context, natsURL string, db *gorm.DB) (func(), error) {
	dao := system.NewAuditLogDAO(db)
	opDAO := system.NewOperationLogDAO(db)
	cleanup, err := natsx.StartConsumer(ctx, natsURL, natsx.ConsumerConfig{
		StreamName:   "audit_events",
		Subject:      "audit.log.>",
		ConsumerName: "audit-service",
		MaxDeliver:   5,
		AckWait:      30 * time.Second,
		DLQ:          true,
		DLQSubject:   "dlq.audit.log",
		ProcessFunc: func(cctx context.Context, data []byte, meta *natsx.MsgMeta) error {
			var ev auditevents.AuditEvent
			if err := json.Unmarshal(data, &ev); err != nil {
				logger.Warn("audit consumer: unmarshal failed",
					zap.String("subject", meta.Subject),
					zap.Int("delivered", meta.Delivered),
					logger.Err(err),
				)
				return nil
			}
			rec := &localmodel.AuditLog{
				TenantID:   uint(ev.TenantID),
				ActorType:  ev.ActorType,
				ActorID:    ev.ActorID,
				Action:     ev.Action,
				TargetType: ev.TargetType,
				TargetID:   ev.TargetID,
				BeforeJSON: ev.Before,
				AfterJSON:  ev.After,
				Summary:    ev.Summary,
				CreatedAt:  ev.CreatedAt,
			}
			if rec.CreatedAt.IsZero() {
				rec.CreatedAt = time.Now()
			}
			if err := dao.CreateLogContext(cctx, rec); err != nil {
				logger.Warn("audit consumer: write failed, will retry",
					zap.String("action", ev.Action),
					logger.Err(err),
				)
				return err
			}
			return nil
		},
	})
	if err != nil {
		return nil, err
	}
	opCleanup, err := natsx.StartConsumer(ctx, natsURL, natsx.ConsumerConfig{
		StreamName:   "audit_events",
		Subject:      "operation.log.>",
		ConsumerName: "audit-service-ops",
		MaxDeliver:   5,
		AckWait:      30 * time.Second,
		DLQ:          true,
		DLQSubject:   "dlq.operation.log",
		ProcessFunc: func(cctx context.Context, data []byte, meta *natsx.MsgMeta) error {
			var logRec localmodel.OperationLog
			if err := json.Unmarshal(data, &logRec); err != nil {
				logger.Warn("audit consumer: unmarshal op failed",
					zap.String("subject", meta.Subject),
					logger.Err(err),
				)
				return nil
			}
			if err := opDAO.CreateLogContext(cctx, &logRec); err != nil {
				return err
			}
			return nil
		},
	})
	if err != nil {
		cleanup()
		return nil, err
	}
	return func() {
		cleanup()
		opCleanup()
	}, nil
}
