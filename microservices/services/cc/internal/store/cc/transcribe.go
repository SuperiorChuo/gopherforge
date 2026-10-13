package store

import (
	"context"
	"errors"
	"time"

	"gorm.io/gorm"
)

// 录音转写异步任务队列（cc_transcribe_jobs）。
//
// 为什么要队列：转写要把整段 wav 从媒体面搬到 ASR，单请求秒级到分钟级。
// 同步做的话请求线程被 ASR 的等待占住，且历史实现把整个 wav 读进内存
// （100MB/请求，服务限额 256M，两个并发即 OOM）。改成入队 + 后台单/少量
// worker 串行消费，配合 io.Pipe 流式转发，内存占用与录音大小解耦。
//
// 进程重启：running 的任务不会自愈（没有分布式锁，也不需要），由
// RequeueStaleTranscribeJobs 把超过阈值仍在 running 的任务改回 pending
// 重新排队，attempts 已经累加，超过 MaxAttempts 直接判失败不再重试。

// TranscribeJobStatus 取值。
const (
	TranscribeStatusPending = "pending"
	TranscribeStatusRunning = "running"
	TranscribeStatusDone    = "done"
	TranscribeStatusFailed  = "failed"
)

// CCTranscribeJob 一次转写+质检任务。按 (tenant_id, call_id) 唯一：同一通话
// 重复提交复用同一行（进行中直接返回原任务，已完成/已失败则重置为 pending 重跑）。
type CCTranscribeJob struct {
	ID       uint64 `gorm:"primaryKey" json:"id"`
	TenantID uint64 `gorm:"not null;default:1;uniqueIndex:ux_cc_transcribe_jobs_tenant_call" json:"tenant_id"`
	CallID   string `gorm:"size:64;not null;uniqueIndex:ux_cc_transcribe_jobs_tenant_call" json:"call_id"`
	// RecordingFile 入队时已校验过的录音文件名（basename），worker 直接用，
	// 不再重查话单——归属校验在入队时按租户做过。
	RecordingFile string `gorm:"size:160;not null" json:"recording_file"`
	Status        string `gorm:"size:16;not null;default:pending;index" json:"status"`
	Attempts      int    `gorm:"not null;default:0" json:"attempts"`
	MaxAttempts   int    `gorm:"not null;default:3" json:"max_attempts"`
	Error         string `gorm:"size:512" json:"error,omitempty"`
	// RequestedBy 提交人 user_id（排障用）。
	RequestedBy uint64 `json:"requested_by"`
	// Score 完成时的质检得分（未完成为空）。
	Score      *int       `json:"score,omitempty"`
	StartedAt  *time.Time `json:"started_at,omitempty"`
	FinishedAt *time.Time `json:"finished_at,omitempty"`
	CreatedAt  time.Time  `json:"created_at"`
	UpdatedAt  time.Time  `json:"updated_at"`
}

func (CCTranscribeJob) TableName() string { return "cc_transcribe_jobs" }

// ErrTranscribeQueueFull 待处理任务过多（保护 ASR 与本进程）。
var ErrTranscribeQueueFull = errors.New("transcribe queue is full")

// EnqueueTranscribeJob 幂等入队。返回任务与 created（true=新建或重置，
// false=已有进行中的同通话任务，直接复用）。queueCap>0 时 pending 数达到
// 上限且需要新建时返回 ErrTranscribeQueueFull。
func (s *Store) EnqueueTranscribeJob(ctx context.Context, tenantID uint64, callID, recFile string, requestedBy uint64, maxAttempts, queueCap int) (*CCTranscribeJob, bool, error) {
	tenantID = EnsureTenant(tenantID)
	if maxAttempts <= 0 {
		maxAttempts = 3
	}
	var job CCTranscribeJob
	err := s.db.WithContext(ctx).
		Where("tenant_id = ? AND call_id = ?", tenantID, callID).First(&job).Error
	switch {
	case err == nil:
		if job.Status == TranscribeStatusPending || job.Status == TranscribeStatusRunning {
			return &job, false, nil
		}
	case !errors.Is(err, gorm.ErrRecordNotFound):
		return nil, false, err
	}

	if queueCap > 0 {
		var pending int64
		if err := s.db.WithContext(ctx).Model(&CCTranscribeJob{}).
			Where("status IN ?", []string{TranscribeStatusPending, TranscribeStatusRunning}).
			Count(&pending).Error; err != nil {
			return nil, false, err
		}
		if pending >= int64(queueCap) {
			return nil, false, ErrTranscribeQueueFull
		}
	}

	if job.ID == 0 {
		job = CCTranscribeJob{
			TenantID: tenantID, CallID: callID, RecordingFile: recFile,
			Status: TranscribeStatusPending, MaxAttempts: maxAttempts, RequestedBy: requestedBy,
		}
		if err := s.db.WithContext(ctx).Create(&job).Error; err != nil {
			return nil, false, err
		}
		return &job, true, nil
	}
	// 已完成/已失败 → 重置重跑（attempts 归零，清错误与结果）。
	now := time.Now()
	if err := s.db.WithContext(ctx).Model(&CCTranscribeJob{}).Where("id = ?", job.ID).
		Updates(map[string]any{
			"recording_file": recFile, "status": TranscribeStatusPending,
			"attempts": 0, "max_attempts": maxAttempts, "error": "",
			"score": nil, "started_at": nil, "finished_at": nil,
			"requested_by": requestedBy, "updated_at": now,
		}).Error; err != nil {
		return nil, false, err
	}
	if err := s.db.WithContext(ctx).First(&job, job.ID).Error; err != nil {
		return nil, false, err
	}
	return &job, true, nil
}

// GetTranscribeJob 按 ID 查任务，强制租户归属（跨租户查返回 ErrRecordNotFound）。
func (s *Store) GetTranscribeJob(ctx context.Context, tenantID, id uint64) (*CCTranscribeJob, error) {
	var job CCTranscribeJob
	if err := s.db.WithContext(ctx).
		Where("id = ? AND tenant_id = ?", id, EnsureTenant(tenantID)).First(&job).Error; err != nil {
		return nil, err
	}
	return &job, nil
}

// GetTranscribeJobByCall 按通话查最近一次任务（前端轮询用）。
func (s *Store) GetTranscribeJobByCall(ctx context.Context, tenantID uint64, callID string) (*CCTranscribeJob, error) {
	var job CCTranscribeJob
	if err := s.db.WithContext(ctx).
		Where("tenant_id = ? AND call_id = ?", EnsureTenant(tenantID), callID).First(&job).Error; err != nil {
		return nil, err
	}
	return &job, nil
}

// ClaimTranscribeJob 取一条 pending 任务并原子置为 running（attempts+1）。
// 队列为空返回 (nil, nil)。多 worker/多副本下靠 UPDATE ... WHERE status='pending'
// 的 RowsAffected 判胜出，抢输的下一轮再取。
func (s *Store) ClaimTranscribeJob(ctx context.Context) (*CCTranscribeJob, error) {
	for attempt := 0; attempt < 5; attempt++ {
		var job CCTranscribeJob
		err := s.db.WithContext(ctx).Where("status = ?", TranscribeStatusPending).
			Order("id ASC").First(&job).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		if err != nil {
			return nil, err
		}
		now := time.Now()
		res := s.db.WithContext(ctx).Model(&CCTranscribeJob{}).
			Where("id = ? AND status = ?", job.ID, TranscribeStatusPending).
			Updates(map[string]any{
				"status": TranscribeStatusRunning, "attempts": gorm.Expr("attempts + 1"),
				"started_at": now, "updated_at": now,
			})
		if res.Error != nil {
			return nil, res.Error
		}
		if res.RowsAffected == 0 {
			continue // 被别人抢走，换下一条
		}
		if err := s.db.WithContext(ctx).First(&job, job.ID).Error; err != nil {
			return nil, err
		}
		return &job, nil
	}
	return nil, nil
}

// FinishTranscribeJob 标记成功并留存质检得分。
func (s *Store) FinishTranscribeJob(ctx context.Context, id uint64, score int) error {
	now := time.Now()
	return s.db.WithContext(ctx).Model(&CCTranscribeJob{}).Where("id = ?", id).
		Updates(map[string]any{
			"status": TranscribeStatusDone, "error": "", "score": score,
			"finished_at": now, "updated_at": now,
		}).Error
}

// FailTranscribeJob 记一次失败：attempts < max_attempts 时退回 pending 重试，
// 否则判 failed。返回 willRetry。
func (s *Store) FailTranscribeJob(ctx context.Context, id uint64, reason string) (bool, error) {
	var job CCTranscribeJob
	if err := s.db.WithContext(ctx).First(&job, id).Error; err != nil {
		return false, err
	}
	if len(reason) > 500 {
		reason = reason[:500]
	}
	now := time.Now()
	updates := map[string]any{"error": reason, "updated_at": now}
	willRetry := job.Attempts < job.MaxAttempts
	if willRetry {
		updates["status"] = TranscribeStatusPending
		updates["started_at"] = nil
	} else {
		updates["status"] = TranscribeStatusFailed
		updates["finished_at"] = now
	}
	if err := s.db.WithContext(ctx).Model(&CCTranscribeJob{}).Where("id = ?", id).
		Updates(updates).Error; err != nil {
		return false, err
	}
	return willRetry, nil
}

// RequeueStaleTranscribeJobs 把卡在 running 超过 olderThan 的任务退回 pending
// （进程被 kill / 部署滚动重启时任务不会永久悬挂）；attempts 已用满的直接判失败。
// 返回 (重排数, 判失败数)。
func (s *Store) RequeueStaleTranscribeJobs(ctx context.Context, olderThan time.Duration) (int64, int64, error) {
	cutoff := time.Now().Add(-olderThan)
	now := time.Now()
	dead := s.db.WithContext(ctx).Model(&CCTranscribeJob{}).
		Where("status = ? AND started_at < ? AND attempts >= max_attempts", TranscribeStatusRunning, cutoff).
		Updates(map[string]any{
			"status": TranscribeStatusFailed, "error": "任务中断且重试已用尽（进程重启或超时）",
			"finished_at": now, "updated_at": now,
		})
	if dead.Error != nil {
		return 0, 0, dead.Error
	}
	res := s.db.WithContext(ctx).Model(&CCTranscribeJob{}).
		Where("status = ? AND started_at < ?", TranscribeStatusRunning, cutoff).
		Updates(map[string]any{
			"status": TranscribeStatusPending, "started_at": nil,
			"error": "任务中断，已重新排队（进程重启或超时）", "updated_at": now,
		})
	return res.RowsAffected, dead.RowsAffected, res.Error
}
