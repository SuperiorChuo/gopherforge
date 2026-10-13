package dao

import (
	"context"
	"fmt"
	"time"

	localmodel "github.com/go-admin-kit/services/ai/internal/model"
	"github.com/go-admin-kit/services/shared/pkg/fullread"
	"gorm.io/gorm"
)

// Hermes M5：Cron 定时任务的 DAO。挂在 *HermesDAO 上（同类型分文件）。
// 与既有 hermes DAO 约定一致：空结果 make([]T,0)、无 FK、状态流转带来源守卫。

// ---------------------------------------------------------------------------
// 定时任务 hermes_cron_jobs
// ---------------------------------------------------------------------------

func (d *HermesDAO) CreateCronJob(ctx context.Context, job *localmodel.HermesCronJob) error {
	return d.db.WithContext(ctx).Create(job).Error
}

func (d *HermesDAO) GetCronJob(ctx context.Context, id uint64) (*localmodel.HermesCronJob, error) {
	var job localmodel.HermesCronJob
	err := d.db.WithContext(ctx).First(&job, id).Error
	return &job, err
}

// HermesCronJobView 是定时任务列表视图，带累计触发次数与最近触发时间。
type HermesCronJobView struct {
	localmodel.HermesCronJob
	RunCount  int64      `json:"run_count"`
	LastRunAt *time.Time `json:"last_run_at,omitempty"`
}

func (d *HermesDAO) ListCronJobs(ctx context.Context, page, pageSize int) ([]HermesCronJobView, int64, error) {
	views := make([]HermesCronJobView, 0)
	var total int64
	query := d.db.WithContext(ctx).Model(&localmodel.HermesCronJob{})
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	err := query.
		Select(`hermes_cron_jobs.*,
			(SELECT COUNT(*) FROM hermes_cron_runs r
			 WHERE r.tenant_id = hermes_cron_jobs.tenant_id
			   AND r.job_id = hermes_cron_jobs.id) AS run_count,
			(SELECT r2.created_at FROM hermes_cron_runs r2
			 WHERE r2.tenant_id = hermes_cron_jobs.tenant_id
			   AND r2.job_id = hermes_cron_jobs.id
			 ORDER BY r2.created_at DESC LIMIT 1) AS last_run_at`).
		Order("hermes_cron_jobs.updated_at DESC, hermes_cron_jobs.id DESC").
		Offset((page - 1) * pageSize).Limit(pageSize).
		Scan(&views).Error
	return views, total, err
}

func (d *HermesDAO) UpdateCronJob(ctx context.Context, id uint64, updates map[string]any) error {
	result := d.db.WithContext(ctx).Model(&localmodel.HermesCronJob{}).
		Where("id = ?", id).
		Updates(updates)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected != 1 {
		return fmt.Errorf("update Hermes cron job %d: %w", id, gorm.ErrRecordNotFound)
	}
	return nil
}

func (d *HermesDAO) DeleteCronJob(ctx context.Context, id uint64) error {
	return d.db.WithContext(ctx).Delete(&localmodel.HermesCronJob{}, id).Error
}

// ListEnabledCronJobs 返回租户启用的任务，供 buildDesiredStateCron 注入签名 desired-state。
func (d *HermesDAO) ListEnabledCronJobs(ctx context.Context) ([]localmodel.HermesCronJob, error) {
	return fullread.All[localmodel.HermesCronJob](d.db.WithContext(ctx).
		Where("enabled = ?", true).
		Order("id ASC"))
}

// ---------------------------------------------------------------------------
// 触发审计 hermes_cron_runs（设备写入，控制面只读）
// ---------------------------------------------------------------------------

func (d *HermesDAO) CreateCronRun(ctx context.Context, run *localmodel.HermesCronRun) error {
	return d.db.WithContext(ctx).Create(run).Error
}

func (d *HermesDAO) ListCronRuns(ctx context.Context, jobID uint64, page, pageSize int) ([]localmodel.HermesCronRun, int64, error) {
	runs := make([]localmodel.HermesCronRun, 0)
	var total int64
	query := d.db.WithContext(ctx).Model(&localmodel.HermesCronRun{}).Where("job_id = ?", jobID)
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	err := query.
		Order("created_at DESC, id DESC").
		Offset((page - 1) * pageSize).Limit(pageSize).
		Find(&runs).Error
	return runs, total, err
}
