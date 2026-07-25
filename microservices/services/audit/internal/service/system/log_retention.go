package system

import (
	"context"
	"errors"
	"time"

	"github.com/go-admin-kit/services/shared/pkg/jobbeat"
	"github.com/go-admin-kit/services/shared/pkg/logger"
	"gorm.io/gorm"
)

// 日志保留策略：按保留天数周期清理操作日志与登录日志（跨全部租户）。
//
//   - 默认关闭（AUDIT_LOG_RETENTION_DAYS<=0）——绝不隐式删数据，开关权在部署方；
//   - 业务审计日志（audit_logs）刻意不在清理范围：它是合规取证面，
//     要清只能走显式运维操作；
//   - 每轮经 shared/pkg/jobbeat 上报心跳（job_key: audit.log_retention），
//     在控制台「定时任务 → 服务任务心跳」可见，删除失败会标 error。

// LogRetentionOptions 保留策略参数。
type LogRetentionOptions struct {
	// RetentionDays <=0 表示关闭。
	RetentionDays int
	// ScanInterval <=0 时取默认一天。
	ScanInterval time.Duration
}

// StartLogRetentionCleaner 启动后台清理协程；关闭状态返回 false（不起协程、
// 不上报心跳）。ctx 取消即退出；单轮删除是独立 DELETE，中途终止无残留状态。
func StartLogRetentionCleaner(
	ctx context.Context,
	db *gorm.DB,
	opSvc *OperationLogService,
	loginSvc *LoginLogService,
	opts LogRetentionOptions,
) bool {
	if opts.RetentionDays <= 0 {
		return false
	}
	if opts.ScanInterval <= 0 {
		opts.ScanInterval = 24 * time.Hour
	}
	go func() {
		ticker := time.NewTicker(opts.ScanInterval)
		defer ticker.Stop()
		for {
			runLogRetentionOnce(ctx, db, opSvc, loginSvc, opts)
			select {
			case <-ticker.C:
			case <-ctx.Done():
				return
			}
		}
	}()
	return true
}

func runLogRetentionOnce(
	ctx context.Context,
	db *gorm.DB,
	opSvc *OperationLogService,
	loginSvc *LoginLogService,
	opts LogRetentionOptions,
) {
	start := time.Now()
	opDeleted, opErr := opSvc.ClearLogsAllTenantsContext(ctx, opts.RetentionDays)
	loginDeleted, loginErr := loginSvc.ClearLogsAllTenantsContext(ctx, opts.RetentionDays)
	err := errors.Join(opErr, loginErr)
	if err != nil {
		logger.Warn("log retention cleanup failed",
			logger.Int("retention_days", opts.RetentionDays),
			logger.Err(err))
	} else {
		logger.Info("log retention cleanup done",
			logger.Int("retention_days", opts.RetentionDays),
			logger.Int64("operation_logs_deleted", opDeleted),
			logger.Int64("login_logs_deleted", loginDeleted))
	}
	jobbeat.Report(db, jobbeat.Run{
		Key:         "audit.log_retention",
		Service:     "audit-service",
		Description: "操作/登录日志按保留天数清理（audit_logs 不清理）",
		IntervalSec: int64(opts.ScanInterval / time.Second),
		StartedAt:   start,
		Err:         err,
	})
}
