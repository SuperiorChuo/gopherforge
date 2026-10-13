package dao

import (
	"context"
	"fmt"
	"time"

	localmodel "github.com/go-admin-kit/services/ai/internal/model"
	"github.com/go-admin-kit/services/shared/pkg/fullread"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// Hermes 第二期：多 Agent 协作任务的 DAO。挂在现有 *HermesDAO 上（同类型分文件）。
//
// 与既有 hermes DAO 相同的约定：
//   - 空结果切片一律 make([]T, 0)，保证 JSON 序列化为 [] 而非 null；
//   - 状态流转带「来源状态守卫」（Where status = from），防止并发覆盖；
//   - 无 FK，引用完整性在服务层校验。

// ---------------------------------------------------------------------------
// 集群
// ---------------------------------------------------------------------------

func (d *HermesDAO) CreateCluster(ctx context.Context, cluster *localmodel.HermesCluster) error {
	return d.db.WithContext(ctx).Create(cluster).Error
}

func (d *HermesDAO) GetCluster(ctx context.Context, id uint64) (*localmodel.HermesCluster, error) {
	var cluster localmodel.HermesCluster
	err := d.db.WithContext(ctx).First(&cluster, id).Error
	return &cluster, err
}

// HermesClusterView 是集群列表视图，带当前启用实例数。
type HermesClusterView struct {
	localmodel.HermesCluster
	InstanceCount int64 `json:"instance_count"`
}

func (d *HermesDAO) ListClusters(ctx context.Context, page, pageSize int) ([]HermesClusterView, int64, error) {
	views := make([]HermesClusterView, 0)
	var total int64
	query := d.db.WithContext(ctx).Model(&localmodel.HermesCluster{})
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	err := query.
		Select(`hermes_clusters.*,
			(SELECT COUNT(*) FROM hermes_instances i
			 WHERE i.tenant_id = hermes_clusters.tenant_id
			   AND i.cluster_id = hermes_clusters.id
			   AND i.enabled) AS instance_count`).
		Order("hermes_clusters.updated_at DESC, hermes_clusters.id DESC").
		Offset((page - 1) * pageSize).Limit(pageSize).
		Scan(&views).Error
	return views, total, err
}

func (d *HermesDAO) UpdateCluster(ctx context.Context, id uint64, updates map[string]any) error {
	result := d.db.WithContext(ctx).Model(&localmodel.HermesCluster{}).
		Where("id = ?", id).Updates(updates)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected != 1 {
		return fmt.Errorf("update Hermes cluster: %w", gorm.ErrRecordNotFound)
	}
	return nil
}

func (d *HermesDAO) DeleteCluster(ctx context.Context, id uint64) error {
	result := d.db.WithContext(ctx).Where("id = ?", id).Delete(&localmodel.HermesCluster{})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected != 1 {
		return fmt.Errorf("delete Hermes cluster: %w", gorm.ErrRecordNotFound)
	}
	return nil
}

func (d *HermesDAO) CountInstancesInCluster(ctx context.Context, clusterID uint64) (int64, error) {
	var total int64
	err := d.db.WithContext(ctx).Model(&localmodel.HermesInstance{}).
		Where("cluster_id = ?", clusterID).Count(&total).Error
	return total, err
}

func (d *HermesDAO) CountTasksByCluster(ctx context.Context, clusterID uint64) (int64, error) {
	var total int64
	err := d.db.WithContext(ctx).Model(&localmodel.HermesTask{}).
		Where("cluster_id = ?", clusterID).Count(&total).Error
	return total, err
}

// ListInstancesByCluster 返回集群内启用实例（派发目标）。含未绑节点实例：
// 实例可以建好但 device_id 为空，任务派发后等节点接入再跑。
func (d *HermesDAO) ListInstancesByCluster(ctx context.Context, clusterID uint64) ([]localmodel.HermesInstance, error) {
	return fullread.All[localmodel.HermesInstance](d.db.WithContext(ctx).
		Where("cluster_id = ? AND enabled = ?", clusterID, true).Order("id ASC"))
}

// ---------------------------------------------------------------------------
// 任务
// ---------------------------------------------------------------------------

func (d *HermesDAO) CreateTask(ctx context.Context, task *localmodel.HermesTask) error {
	return d.db.WithContext(ctx).Create(task).Error
}

func (d *HermesDAO) GetTask(ctx context.Context, id uint64) (*localmodel.HermesTask, error) {
	var task localmodel.HermesTask
	err := d.db.WithContext(ctx).First(&task, id).Error
	return &task, err
}

func (d *HermesDAO) ListTasks(ctx context.Context, page, pageSize int, status string) ([]localmodel.HermesTask, int64, error) {
	tasks := make([]localmodel.HermesTask, 0)
	var total int64
	query := d.db.WithContext(ctx).Model(&localmodel.HermesTask{})
	if status != "" {
		query = query.Where("status = ?", status)
	}
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	err := query.Order("updated_at DESC, id DESC").
		Offset((page - 1) * pageSize).Limit(pageSize).Find(&tasks).Error
	return tasks, total, err
}

func (d *HermesDAO) DeleteTask(ctx context.Context, id uint64) error {
	result := d.db.WithContext(ctx).Where("id = ?", id).Delete(&localmodel.HermesTask{})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected != 1 {
		return fmt.Errorf("delete Hermes task: %w", gorm.ErrRecordNotFound)
	}
	return nil
}

func (d *HermesDAO) UpdateTask(ctx context.Context, id uint64, updates map[string]any) error {
	result := d.db.WithContext(ctx).Model(&localmodel.HermesTask{}).
		Where("id = ?", id).Updates(updates)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected != 1 {
		return fmt.Errorf("update Hermes task: %w", gorm.ErrRecordNotFound)
	}
	return nil
}

// SetTaskStatus 带来源状态守卫：只有当前状态为 from 的任务才能转到 to，避免
// 并发下把已取消的任务重新激活。
func (d *HermesDAO) SetTaskStatus(ctx context.Context, id uint64, from, to string, now time.Time, actor uint) error {
	fields := map[string]any{
		"status":     to,
		"updated_at": now,
		"updated_by": actor,
	}
	// 终态补 finished_at（与 SetTaskRunStatus 对 run 的处理一致）
	if to == localmodel.HermesTaskStatusCompleted || to == localmodel.HermesTaskStatusCancelled {
		fields["finished_at"] = now
	}
	result := d.db.WithContext(ctx).Model(&localmodel.HermesTask{}).
		Where("id = ? AND status = ?", id, from).
		Updates(fields)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected != 1 {
		return fmt.Errorf("set Hermes task status %s->%s: %w", from, to, gorm.ErrRecordNotFound)
	}
	return nil
}

// ---------------------------------------------------------------------------
// 任务执行记录
// ---------------------------------------------------------------------------

func (d *HermesDAO) CreateTaskRuns(ctx context.Context, runs []localmodel.HermesTaskRun) error {
	if len(runs) == 0 {
		return nil
	}
	return d.db.WithContext(ctx).Create(&runs).Error
}

func (d *HermesDAO) GetTaskRun(ctx context.Context, id uint64) (*localmodel.HermesTaskRun, error) {
	var run localmodel.HermesTaskRun
	err := d.db.WithContext(ctx).First(&run, id).Error
	return &run, err
}

func (d *HermesDAO) ListTaskRuns(ctx context.Context, taskID uint64) ([]localmodel.HermesTaskRun, error) {
	runs := make([]localmodel.HermesTaskRun, 0)
	err := d.db.WithContext(ctx).
		Where("task_id = ?", taskID).Order("id ASC").Find(&runs).Error
	return runs, err
}

func (d *HermesDAO) ListTaskRunsByInstance(ctx context.Context, instanceID uint64, limit int) ([]localmodel.HermesTaskRun, error) {
	runs := make([]localmodel.HermesTaskRun, 0)
	err := d.db.WithContext(ctx).
		Where("instance_id = ?", instanceID).
		Order("id DESC").Limit(limit).Find(&runs).Error
	return runs, err
}

// ListPendingRunsByInstance 返回实例最旧若干条 assigned run（三期任务下发的
// 候选）。assigned 是按激活顺序入队的；取最旧保证先派先执行。
func (d *HermesDAO) ListPendingRunsByInstance(ctx context.Context, instanceID uint64, limit int) ([]localmodel.HermesTaskRun, error) {
	runs := make([]localmodel.HermesTaskRun, 0)
	err := d.db.WithContext(ctx).
		Where("instance_id = ? AND status = ?", instanceID, localmodel.HermesTaskRunStatusAssigned).
		Order("id ASC").Limit(limit).Find(&runs).Error
	return runs, err
}

// SetTaskRunStatus 带来源状态守卫的流转。result 非空时一并写入；进入 running
// 写 started_at，进入终态（completed/failed）写 finished_at。流转规则由服务层
// 校验，这里只保证「同一条 run 不能被两个并发请求同时改到不同终态」。
func (d *HermesDAO) SetTaskRunStatus(ctx context.Context, id uint64, from, to string, now time.Time, result string) error {
	fields := map[string]any{"status": to, "updated_at": now}
	if result != "" {
		fields["result"] = result
	}
	switch to {
	case localmodel.HermesTaskRunStatusRunning:
		fields["started_at"] = now
	case localmodel.HermesTaskRunStatusCompleted, localmodel.HermesTaskRunStatusFailed:
		fields["finished_at"] = now
	}
	result_ := d.db.WithContext(ctx).Model(&localmodel.HermesTaskRun{}).
		Where("id = ? AND status = ?", id, from).
		Updates(fields)
	if result_.Error != nil {
		return result_.Error
	}
	if result_.RowsAffected != 1 {
		return fmt.Errorf("set Hermes task run status %s->%s: %w", from, to, gorm.ErrRecordNotFound)
	}
	return nil
}

// ActivateTaskWithRuns 原子激活任务：锁任务行 + 校验仍为 pending + 批量建 run
// + 任务置 active，全部在一个事务里。instanceIDs 为空时只把任务置 active，
// 不建 run（集群没有启用实例时允许先把任务标为执行中）。
func (d *HermesDAO) ActivateTaskWithRuns(ctx context.Context, taskID uint64, instanceIDs []uint64, now time.Time, actor uint) error {
	return d.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var task localmodel.HermesTask
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("id = ?", taskID).First(&task).Error; err != nil {
			return err
		}
		if task.Status != localmodel.HermesTaskStatusPending {
			return fmt.Errorf("activate Hermes task: %w", gorm.ErrRecordNotFound)
		}
		if len(instanceIDs) > 0 {
			// 逐条 Create：tenant 插件的 create 钩子对批量 slice 反射设 TenantID
			// 会崩（field.Set 作用在 slice 反射值上），现有 hermes 表都是单条写。
			for _, instanceID := range instanceIDs {
				run := localmodel.HermesTaskRun{
					TenantID:   task.TenantID,
					TaskID:     taskID,
					InstanceID: instanceID,
					Status:     localmodel.HermesTaskRunStatusAssigned,
					CreatedBy:  actor,
					UpdatedBy:  actor,
					CreatedAt:  now,
					UpdatedAt:  now,
				}
				if err := tx.Create(&run).Error; err != nil {
					return err
				}
			}
		}
		return tx.Model(&localmodel.HermesTask{}).
			Where("id = ? AND status = ?", taskID, localmodel.HermesTaskStatusPending).
			Updates(map[string]any{
				"status":     localmodel.HermesTaskStatusActive,
				"started_at": now,
				"updated_at": now,
				"updated_by": actor,
			}).Error
	})
}
