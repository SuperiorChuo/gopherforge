// Package jobbeat 上报分布式后台任务心跳到 ops_job_heartbeats（任务中心）。
// 各服务进程内的循环任务每轮跑完调用 Report 一次；写入软失败（只记日志，
// 绝不影响任务本身）——表未建（如单测 sqlite 未迁移）或库抖动都不炸循环。
//
// 用法（循环体每轮收尾）：
//
//	defer jobbeat.Report(db, jobbeat.Run{
//	    Key: "cc.balance_warn", Service: "cc-service",
//	    Description: "余额预警/欠费冻结扫描", IntervalSec: 3600,
//	    StartedAt: start, Err: err,
//	})
//
// job_key 约定 <服务>.<任务>；shell cron 侧不经本包，直接 psql upsert 同表
// （见 scripts/ops/*.sh）。
//
// 注意：构建上下文不含 shared 的服务（im/cc/crm/mp/notify/ticket/bpm/visibility）
// 各自持有 internal/jobbeat 同源副本（先例见 iploc/metrics），改动须同步。
package jobbeat

import (
	"log"
	"time"

	"gorm.io/gorm"
)

// Run 一轮任务执行的上报载荷。
type Run struct {
	Key         string // 全局唯一，<服务>.<任务>
	Service     string
	Description string
	IntervalSec int64 // 期望间隔（秒），聚合侧据此判超期；0=不判
	StartedAt   time.Time
	Err         error
}

// Report upsert 一行心跳。软失败：任何错误只记日志。
func Report(db *gorm.DB, r Run) {
	if db == nil || r.Key == "" {
		return
	}
	status, lastErr := "ok", ""
	failInc := 0
	if r.Err != nil {
		status, lastErr = "error", r.Err.Error()
		failInc = 1
	}
	if r.StartedAt.IsZero() {
		r.StartedAt = time.Now()
	}
	durMS := time.Since(r.StartedAt).Milliseconds()
	// PG 与 sqlite（单测）都认这个 ON CONFLICT 语法。
	err := db.Exec(`
INSERT INTO ops_job_heartbeats
  (job_key, service, description, interval_sec, last_run_at, last_status, last_error, last_duration_ms, runs, fails, updated_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, 1, ?, ?)
ON CONFLICT (job_key) DO UPDATE SET
  service = EXCLUDED.service,
  description = EXCLUDED.description,
  interval_sec = EXCLUDED.interval_sec,
  last_run_at = EXCLUDED.last_run_at,
  last_status = EXCLUDED.last_status,
  last_error = EXCLUDED.last_error,
  last_duration_ms = EXCLUDED.last_duration_ms,
  runs = ops_job_heartbeats.runs + 1,
  fails = ops_job_heartbeats.fails + ?,
  updated_at = EXCLUDED.updated_at`,
		r.Key, r.Service, r.Description, r.IntervalSec, time.Now(), status, lastErr, durMS,
		failInc, time.Now(), failInc).Error
	if err != nil {
		log.Printf("jobbeat: report %s failed (soft): %v", r.Key, err)
	}
}
