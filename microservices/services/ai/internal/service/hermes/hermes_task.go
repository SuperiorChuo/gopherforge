package service

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/go-admin-kit/services/ai/internal/dao"
	localmodel "github.com/go-admin-kit/services/ai/internal/model"
	"github.com/go-admin-kit/services/shared/pkg/tenant"
	"gorm.io/gorm"
)

// Hermes 第二期：多 Agent 协作任务（仅控制面编排）。
//
// 决策见 docs/hermes-multi-agent-phase2.md。本期不动物理链路：company_cluster
// 策略不可应用到任何真实设备（manager isExactP1Toolset 只收 P1 四件套），
// 因此集群/任务策略是「创建、指派、追踪」的控制面概念。

const (
	maxHermesTaskDescriptionLen = 10000
	maxHermesTaskRunResultLen   = 4000
)

// hermesClusterSlugPattern 是 kanban board slug 白名单：小写字母数字 + _ -，
// 无点（会成为 board 目录名）。与 manager 侧 kanbanBoardSlugPattern 一致。
var hermesClusterSlugPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]{0,63}$`)

// hermesWorkerNamePattern 是 kanban worker profile 名白名单：小写字母数字 +
// . _ -，会成为文件系统目录名。与 manager 侧 kanbanWorkerNamePattern 一致。
var hermesWorkerNamePattern = regexp.MustCompile(`^[a-z0-9][a-z0-9._-]{0,62}$`)

// errHermesClusterPolicyNotDeployable 是 company_cluster 策略不可下发到设备的
// 边界说明。manager 未放行 dispatcher 工具集前，任何把该 profile 策略当作
// 设备/实例策略的尝试都必须失败，防止 delegation 工具集进入签名期望状态。
var errHermesClusterPolicyNotDeployable = errors.New(
	"company_cluster policy cannot be applied to a device until the manager accepts its toolset",
)

// ---------------------------------------------------------------------------
// 集群
// ---------------------------------------------------------------------------

type HermesClusterRequest struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	// Slug 非空 = 启用 kanban 通道（board 目录名）；空 = push 通道。
	Slug     string `json:"slug"`
	PolicyID uint64 `json:"policy_id"`
	Enabled  bool   `json:"enabled"`
}

func (s *HermesService) CreateCluster(ctx context.Context, actorID uint, request HermesClusterRequest) (*localmodel.HermesCluster, error) {
	if actorID == 0 {
		return nil, fmt.Errorf("%w: missing actor", ErrHermesInvalidInput)
	}
	request.Name = strings.TrimSpace(request.Name)
	request.Description = strings.TrimSpace(request.Description)
	request.Slug = strings.TrimSpace(request.Slug)
	if len(request.Name) == 0 || len(request.Name) > 80 {
		return nil, fmt.Errorf("%w: invalid cluster name", ErrHermesInvalidInput)
	}
	if len(request.Description) > maxHermesTaskDescriptionLen {
		return nil, fmt.Errorf("%w: description too long", ErrHermesInvalidInput)
	}
	if err := validateHermesClusterSlug(request.Slug); err != nil {
		return nil, err
	}
	// slug 在租户内唯一（非空时）。服务层先查一遍：迁移里的部分唯一索引兜底
	// Postgres，sqlite 测试靠这层。
	if request.Slug != "" {
		exists, err := s.clusterSlugExists(ctx, request.Slug, 0)
		if err != nil {
			return nil, err
		}
		if exists {
			return nil, fmt.Errorf("%w: cluster slug already exists", ErrHermesInvalidInput)
		}
	}
	if request.PolicyID > 0 {
		if err := s.validateClusterPolicy(ctx, request.PolicyID); err != nil {
			return nil, err
		}
	}
	cluster := &localmodel.HermesCluster{
		TenantID:    tenant.FromContextOrDefault(ctx),
		Name:        request.Name,
		Description: request.Description,
		Slug:        request.Slug,
		PolicyID:    request.PolicyID,
		Enabled:     request.Enabled,
		CreatedBy:   actorID,
		UpdatedBy:   actorID,
		CreatedAt:   s.now().UTC(),
		UpdatedAt:   s.now().UTC(),
	}
	if err := s.dao.CreateCluster(ctx, cluster); err != nil {
		return nil, err
	}
	return cluster, nil
}

func (s *HermesService) ListClusters(ctx context.Context, page, pageSize int) ([]dao.HermesClusterView, int64, error) {
	page, pageSize = normalizeHermesPage(page, pageSize)
	return s.dao.ListClusters(ctx, page, pageSize)
}

func (s *HermesService) GetCluster(ctx context.Context, id uint64) (*dao.HermesClusterView, []localmodel.HermesInstance, error) {
	cluster, err := s.dao.GetCluster(ctx, id)
	if err != nil {
		return nil, nil, err
	}
	instances, err := s.dao.ListInstancesByCluster(ctx, id)
	if err != nil {
		return nil, nil, err
	}
	view := &dao.HermesClusterView{HermesCluster: *cluster, InstanceCount: int64(len(instances))}
	return view, instances, nil
}

func (s *HermesService) UpdateCluster(ctx context.Context, actorID uint, id uint64, request HermesClusterRequest) error {
	if actorID == 0 {
		return fmt.Errorf("%w: missing actor", ErrHermesInvalidInput)
	}
	request.Name = strings.TrimSpace(request.Name)
	request.Description = strings.TrimSpace(request.Description)
	request.Slug = strings.TrimSpace(request.Slug)
	if len(request.Name) == 0 || len(request.Name) > 80 {
		return fmt.Errorf("%w: invalid cluster name", ErrHermesInvalidInput)
	}
	if err := validateHermesClusterSlug(request.Slug); err != nil {
		return err
	}
	if request.PolicyID > 0 {
		if err := s.validateClusterPolicy(ctx, request.PolicyID); err != nil {
			return err
		}
	}
	// slug 变化会换 board 目录（旧 board 成孤儿）；集群有 active 任务时拒绝改
	current, err := s.dao.GetCluster(ctx, id)
	if err != nil {
		return err
	}
	if current.Slug != request.Slug {
		if request.Slug != "" {
			exists, err := s.clusterSlugExists(ctx, request.Slug, id)
			if err != nil {
				return err
			}
			if exists {
				return fmt.Errorf("%w: cluster slug already exists", ErrHermesInvalidInput)
			}
		}
		activeCount, err := s.countActiveTasksByCluster(ctx, id)
		if err != nil {
			return err
		}
		if activeCount > 0 {
			return fmt.Errorf("%w: cannot change slug while cluster has active tasks", ErrHermesInvalidInput)
		}
	}
	now := s.now().UTC()
	return s.dao.UpdateCluster(ctx, id, map[string]any{
		"name":        request.Name,
		"description": request.Description,
		"slug":        request.Slug,
		"policy_id":   request.PolicyID,
		"enabled":     request.Enabled,
		"updated_by":  actorID,
		"updated_at":  now,
	})
}

// validateHermesClusterSlug 校验 kanban board slug：空串合法（push 通道），
// 非空必须匹配白名单（小写字母数字 + _ -，无点，会成为 board 目录名）。
func validateHermesClusterSlug(slug string) error {
	if slug == "" {
		return nil
	}
	if !hermesClusterSlugPattern.MatchString(slug) {
		return fmt.Errorf("%w: invalid cluster slug", ErrHermesInvalidInput)
	}
	return nil
}

func (s *HermesService) countActiveTasksByCluster(ctx context.Context, clusterID uint64) (int64, error) {
	var total int64
	err := s.dao.DB().WithContext(ctx).Model(&localmodel.HermesTask{}).
		Where("cluster_id = ? AND status = ?", clusterID, localmodel.HermesTaskStatusActive).
		Count(&total).Error
	return total, err
}

// clusterSlugExists 检查租户内是否已有同名 slug（excludeID 排除自己，用于更新）。
func (s *HermesService) clusterSlugExists(ctx context.Context, slug string, excludeID uint64) (bool, error) {
	var total int64
	query := s.dao.DB().WithContext(ctx).Model(&localmodel.HermesCluster{}).
		Where("slug = ?", slug)
	if excludeID > 0 {
		query = query.Where("id <> ?", excludeID)
	}
	err := query.Count(&total).Error
	return total > 0, err
}

// slugifyHermesWorkerName 把实例名转成 kanban worker profile 名（安全白名单）。
// 空实例名或转不出合法名返回空串（调用方跳过该 worker）。
func slugifyHermesWorkerName(name string) string {
	name = strings.TrimSpace(strings.ToLower(name))
	if name == "" {
		return ""
	}
	var builder strings.Builder
	for _, r := range name {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9', r == '.', r == '_', r == '-':
			builder.WriteRune(r)
		default:
			builder.WriteRune('-')
		}
	}
	slug := builder.String()
	slug = strings.Trim(slug, ".-")
	if slug == "" {
		return ""
	}
	if len(slug) > 63 {
		slug = slug[:63]
	}
	if !hermesWorkerNamePattern.MatchString(slug) {
		return ""
	}
	return slug
}

func (s *HermesService) DeleteCluster(ctx context.Context, actorID uint, id uint64) error {
	if actorID == 0 {
		return fmt.Errorf("%w: missing actor", ErrHermesInvalidInput)
	}
	instanceCount, err := s.dao.CountInstancesInCluster(ctx, id)
	if err != nil {
		return err
	}
	if instanceCount > 0 {
		return fmt.Errorf("%w: cluster still has instances", ErrHermesInvalidInput)
	}
	taskCount, err := s.dao.CountTasksByCluster(ctx, id)
	if err != nil {
		return err
	}
	if taskCount > 0 {
		return fmt.Errorf("%w: cluster still has tasks", ErrHermesInvalidInput)
	}
	return s.dao.DeleteCluster(ctx, id)
}

// validateClusterPolicy 校验集群引用的策略必须是 company_cluster profile。
// company_cluster 是唯一可开放 dispatcher 的 profile；引用 company 策略会让
// 集群/任务派发语义与端侧能力错位，直接拒绝。
func (s *HermesService) validateClusterPolicy(ctx context.Context, policyID uint64) error {
	policy, err := s.dao.GetPolicy(ctx, policyID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return fmt.Errorf("%w: policy not found", ErrHermesInvalidInput)
		}
		return err
	}
	if policy.Profile != localmodel.HermesProfileCompanyCluster {
		return fmt.Errorf("%w: cluster policy must use company_cluster profile", ErrHermesInvalidInput)
	}
	return nil
}

// ---------------------------------------------------------------------------
// 任务
// ---------------------------------------------------------------------------

type HermesTaskRequest struct {
	Name        string `json:"name"`
	Description string `json:"description"` // 任务正文（明文，必填）
	ClusterID   uint64 `json:"cluster_id"`
	PolicyID    uint64 `json:"policy_id"` // 0 = 沿用集群策略
}

func (s *HermesService) CreateTask(ctx context.Context, actorID uint, request HermesTaskRequest) (*localmodel.HermesTask, error) {
	if actorID == 0 {
		return nil, fmt.Errorf("%w: missing actor", ErrHermesInvalidInput)
	}
	request.Name = strings.TrimSpace(request.Name)
	request.Description = strings.TrimSpace(request.Description)
	if len(request.Name) == 0 || len(request.Name) > 120 {
		return nil, fmt.Errorf("%w: invalid task name", ErrHermesInvalidInput)
	}
	if len(request.Description) == 0 {
		return nil, fmt.Errorf("%w: task description is required", ErrHermesInvalidInput)
	}
	if len(request.Description) > maxHermesTaskDescriptionLen {
		return nil, fmt.Errorf("%w: description too long", ErrHermesInvalidInput)
	}
	if request.ClusterID > 0 {
		if _, err := s.dao.GetCluster(ctx, request.ClusterID); err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return nil, fmt.Errorf("%w: cluster not found", ErrHermesInvalidInput)
			}
			return nil, err
		}
	}
	if request.PolicyID > 0 {
		if err := s.validateClusterPolicy(ctx, request.PolicyID); err != nil {
			return nil, err
		}
	}
	now := s.now().UTC()
	task := &localmodel.HermesTask{
		TenantID:    tenant.FromContextOrDefault(ctx),
		Name:        request.Name,
		Description: request.Description,
		Status:      localmodel.HermesTaskStatusPending,
		ClusterID:   request.ClusterID,
		PolicyID:    request.PolicyID,
		CreatedBy:   actorID,
		UpdatedBy:   actorID,
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	if err := s.dao.CreateTask(ctx, task); err != nil {
		return nil, err
	}
	return task, nil
}

// HermesTaskView 是任务列表视图，带集群名与执行进度。
type HermesTaskView struct {
	localmodel.HermesTask
	ClusterName string `json:"cluster_name"`
	RunTotal    int64  `json:"run_total"`
	RunDone     int64  `json:"run_done"` // completed 的 run 数
}

func (s *HermesService) ListTasks(ctx context.Context, page, pageSize int, status string) ([]HermesTaskView, int64, error) {
	page, pageSize = normalizeHermesPage(page, pageSize)
	status = strings.TrimSpace(strings.ToLower(status))
	if status != "" {
		switch status {
		case localmodel.HermesTaskStatusPending, localmodel.HermesTaskStatusActive,
			localmodel.HermesTaskStatusCompleted, localmodel.HermesTaskStatusCancelled:
		default:
			return nil, 0, fmt.Errorf("%w: invalid task status", ErrHermesInvalidInput)
		}
	}
	tasks, total, err := s.dao.ListTasks(ctx, page, pageSize, status)
	if err != nil {
		return nil, 0, err
	}
	views := make([]HermesTaskView, 0, len(tasks))
	// 批量加载集群名与进度，避免 N+1
	clusterIDs := make(map[uint64]struct{}, len(tasks))
	taskIDs := make([]uint64, 0, len(tasks))
	for i := range tasks {
		if tasks[i].ClusterID > 0 {
			clusterIDs[tasks[i].ClusterID] = struct{}{}
		}
		taskIDs = append(taskIDs, tasks[i].ID)
	}
	clusterNames := make(map[uint64]string, len(clusterIDs))
	for id := range clusterIDs {
		if cluster, err := s.dao.GetCluster(ctx, id); err == nil {
			clusterNames[id] = cluster.Name
		}
	}
	runTotals, runDones := s.taskRunProgress(ctx, taskIDs)
	for i := range tasks {
		view := HermesTaskView{HermesTask: tasks[i]}
		if tasks[i].ClusterID > 0 {
			view.ClusterName = clusterNames[tasks[i].ClusterID]
		}
		view.RunTotal = runTotals[tasks[i].ID]
		view.RunDone = runDones[tasks[i].ID]
		views = append(views, view)
	}
	return views, total, nil
}

// taskRunProgress 一次性统计一批任务的 run 总数与完成数。
func (s *HermesService) taskRunProgress(ctx context.Context, taskIDs []uint64) (map[uint64]int64, map[uint64]int64) {
	totals := make(map[uint64]int64, len(taskIDs))
	dones := make(map[uint64]int64, len(taskIDs))
	for _, id := range taskIDs {
		runs, err := s.dao.ListTaskRuns(ctx, id)
		if err != nil {
			continue
		}
		totals[id] = int64(len(runs))
		for _, run := range runs {
			if run.Status == localmodel.HermesTaskRunStatusCompleted {
				dones[id]++
			}
		}
	}
	return totals, dones
}

// HermesTaskRunView 是执行记录视图，带实例名。
type HermesTaskRunView struct {
	localmodel.HermesTaskRun
	InstanceName string `json:"instance_name"`
}

type HermesTaskDetail struct {
	Task    *localmodel.HermesTask    `json:"task"`
	Cluster *localmodel.HermesCluster `json:"cluster,omitempty"`
	Policy  *localmodel.HermesPolicy  `json:"policy,omitempty"`
	Runs    []HermesTaskRunView       `json:"runs"`
}

func (s *HermesService) GetTask(ctx context.Context, id uint64) (*HermesTaskDetail, error) {
	task, err := s.dao.GetTask(ctx, id)
	if err != nil {
		return nil, err
	}
	detail := &HermesTaskDetail{Task: task, Runs: make([]HermesTaskRunView, 0)}
	if task.ClusterID > 0 {
		if cluster, err := s.dao.GetCluster(ctx, task.ClusterID); err == nil {
			detail.Cluster = cluster
		}
	}
	policyID := task.PolicyID
	if policyID == 0 && detail.Cluster != nil {
		policyID = detail.Cluster.PolicyID
	}
	if policyID > 0 {
		if policy, err := s.dao.GetPolicy(ctx, policyID); err == nil {
			detail.Policy = policy
		}
	}
	runs, err := s.dao.ListTaskRuns(ctx, id)
	if err != nil {
		return nil, err
	}
	instanceNames := make(map[uint64]string, len(runs))
	for _, run := range runs {
		if instance, err := s.dao.GetInstance(ctx, run.InstanceID); err == nil {
			instanceNames[run.InstanceID] = instance.Name
		}
	}
	for _, run := range runs {
		detail.Runs = append(detail.Runs, HermesTaskRunView{
			HermesTaskRun: run,
			InstanceName:  instanceNames[run.InstanceID],
		})
	}
	return detail, nil
}

// ActivateTask 把任务从 pending 置为 active，并为集群内启用实例批量建 run。
// 策略解析：task.PolicyID（显式）或 cluster.PolicyID（沿用），必须是
// company_cluster profile；校验失败则拒绝激活，保证 dispatcher 语义只在受控
// 策略下开放。
func (s *HermesService) ActivateTask(ctx context.Context, actorID uint, taskID uint64) (*localmodel.HermesTask, error) {
	if actorID == 0 {
		return nil, fmt.Errorf("%w: missing actor", ErrHermesInvalidInput)
	}
	task, err := s.dao.GetTask(ctx, taskID)
	if err != nil {
		return nil, err
	}
	if task.Status != localmodel.HermesTaskStatusPending {
		return nil, fmt.Errorf("%w: task is not pending", ErrHermesInvalidInput)
	}
	if task.ClusterID == 0 {
		return nil, fmt.Errorf("%w: task has no target cluster", ErrHermesInvalidInput)
	}
	cluster, err := s.dao.GetCluster(ctx, task.ClusterID)
	if err != nil {
		return nil, err
	}
	if !cluster.Enabled {
		return nil, fmt.Errorf("%w: target cluster is disabled", ErrHermesInvalidInput)
	}
	policyID := task.PolicyID
	if policyID == 0 {
		policyID = cluster.PolicyID
	}
	if policyID > 0 {
		if err := s.validateClusterPolicy(ctx, policyID); err != nil {
			return nil, err
		}
	}
	instances, err := s.dao.ListInstancesByCluster(ctx, task.ClusterID)
	if err != nil {
		return nil, err
	}
	// kanban 模式（集群 slug 非空）：worker 角色实例（无设备）不建 run——真正
	// 的 worker 由 gateway dispatcher 按 assignee 派生，只有绑定设备的 lead
	// 实例拿 Kanban 指令驱动整个 fan-out。push 模式（空 slug）：全部实例建 run。
	instanceIDs := make([]uint64, 0, len(instances))
	for _, instance := range instances {
		if cluster.Slug != "" && instance.DeviceID == "" {
			continue
		}
		instanceIDs = append(instanceIDs, instance.ID)
	}
	if err := s.dao.ActivateTaskWithRuns(ctx, taskID, instanceIDs, s.now().UTC(), actorID); err != nil {
		return nil, err
	}
	return s.dao.GetTask(ctx, taskID)
}

func (s *HermesService) CompleteTask(ctx context.Context, actorID uint, taskID uint64) error {
	if actorID == 0 {
		return fmt.Errorf("%w: missing actor", ErrHermesInvalidInput)
	}
	now := s.now().UTC()
	// 完成前收尾未终态 run：任务标完成时还没执行完的 run 诚实置 failed，避免
	// 任务显示完成而 run 停在 assigned/running 的矛盾。
	if err := s.reconcileHermesTaskRuns(ctx, taskID, "任务完成时未执行完", now); err != nil {
		return err
	}
	return s.dao.SetTaskStatus(ctx, taskID, localmodel.HermesTaskStatusActive, localmodel.HermesTaskStatusCompleted, now, actorID)
}

// reconcileHermesTaskRuns 把任务的非终态 run（assigned/running）统一置为 failed
// 并写服务端注记，保持 run 状态机无 cancelled 枚举的一致性。
func (s *HermesService) reconcileHermesTaskRuns(ctx context.Context, taskID uint64, note string, now time.Time) error {
	runs, err := s.dao.ListTaskRuns(ctx, taskID)
	if err != nil {
		return err
	}
	for _, run := range runs {
		if run.Status == localmodel.HermesTaskRunStatusAssigned || run.Status == localmodel.HermesTaskRunStatusRunning {
			if err := s.dao.SetTaskRunStatus(ctx, run.ID, run.Status, localmodel.HermesTaskRunStatusFailed, now, note); err != nil {
				return err
			}
		}
	}
	return nil
}

// CancelTask 取消任务：pending/active → cancelled。非终态 run（assigned/running）
// 一并置为 failed 并写服务端注记。
func (s *HermesService) CancelTask(ctx context.Context, actorID uint, taskID uint64) error {
	if actorID == 0 {
		return fmt.Errorf("%w: missing actor", ErrHermesInvalidInput)
	}
	now := s.now().UTC()
	task, err := s.dao.GetTask(ctx, taskID)
	if err != nil {
		return err
	}
	switch task.Status {
	case localmodel.HermesTaskStatusPending:
		return s.dao.SetTaskStatus(ctx, taskID, localmodel.HermesTaskStatusPending, localmodel.HermesTaskStatusCancelled, now, actorID)
	case localmodel.HermesTaskStatusActive:
		if err := s.reconcileHermesTaskRuns(ctx, taskID, "任务取消，未执行", now); err != nil {
			return err
		}
		return s.dao.SetTaskStatus(ctx, taskID, localmodel.HermesTaskStatusActive, localmodel.HermesTaskStatusCancelled, now, actorID)
	default:
		return fmt.Errorf("%w: task is already in a terminal state", ErrHermesInvalidInput)
	}
}

// ---------------------------------------------------------------------------
// 任务执行记录
// ---------------------------------------------------------------------------

type HermesTaskRunTransitionRequest struct {
	Status string `json:"status"` // running | completed | failed
	Result string `json:"result"` // 结果摘要
}

// UpdateTaskRun 推进单条 run 状态。允许 assigned→running|failed、
// running→completed|failed；completed/failed 为终态。result 在 completed/failed
// 时必填（结果摘要），running 时可留空。
func (s *HermesService) UpdateTaskRun(ctx context.Context, actorID uint, runID uint64, request HermesTaskRunTransitionRequest) error {
	if actorID == 0 {
		return fmt.Errorf("%w: missing actor", ErrHermesInvalidInput)
	}
	run, err := s.dao.GetTaskRun(ctx, runID)
	if err != nil {
		return err
	}
	request.Status = strings.TrimSpace(strings.ToLower(request.Status))
	request.Result = strings.TrimSpace(request.Result)

	var fromOK bool
	switch request.Status {
	case localmodel.HermesTaskRunStatusRunning:
		fromOK = run.Status == localmodel.HermesTaskRunStatusAssigned
	case localmodel.HermesTaskRunStatusCompleted:
		fromOK = run.Status == localmodel.HermesTaskRunStatusRunning
	case localmodel.HermesTaskRunStatusFailed:
		fromOK = run.Status == localmodel.HermesTaskRunStatusAssigned || run.Status == localmodel.HermesTaskRunStatusRunning
	default:
		return fmt.Errorf("%w: unsupported run status", ErrHermesInvalidInput)
	}
	if !fromOK {
		return fmt.Errorf("%w: cannot transition run from %s to %s", ErrHermesInvalidInput, run.Status, request.Status)
	}
	if request.Status != localmodel.HermesTaskRunStatusRunning && request.Result == "" {
		return fmt.Errorf("%w: result is required for %s", ErrHermesInvalidInput, request.Status)
	}
	if len(request.Result) > maxHermesTaskRunResultLen {
		return fmt.Errorf("%w: result too long", ErrHermesInvalidInput)
	}
	return s.dao.SetTaskRunStatus(ctx, runID, run.Status, request.Status, s.now().UTC(), request.Result)
}

// ReportDeviceTaskRun 是设备端执行完任务后的结果回传（三期 manager 下发通道）。
// 设备鉴权中间件已解析出 deviceID；这里校验 run→instance→device 归属，防止
// 一台设备替别的设备/实例回填结果。
//
// 与 UpdateTaskRun 的区别：设备路径是同步单次执行，run 直接从 assigned 进终态
// （completed/failed），不需要中间的 running 步；console 管理路径保留原有更严
// 的流转规则。
func (s *HermesService) ReportDeviceTaskRun(ctx context.Context, deviceID string, runID uint64, status, result string) error {
	status = strings.TrimSpace(strings.ToLower(status))
	result = strings.TrimSpace(result)
	switch status {
	case localmodel.HermesTaskRunStatusCompleted, localmodel.HermesTaskRunStatusFailed:
	default:
		return fmt.Errorf("%w: unsupported run status", ErrHermesInvalidInput)
	}
	if result == "" {
		return fmt.Errorf("%w: result is required", ErrHermesInvalidInput)
	}
	if len(result) > maxHermesTaskRunResultLen {
		return fmt.Errorf("%w: result too long", ErrHermesInvalidInput)
	}
	if runID == 0 {
		return fmt.Errorf("%w: invalid run id", ErrHermesInvalidInput)
	}

	run, err := s.dao.GetTaskRun(ctx, runID)
	if err != nil {
		return err
	}
	if run.Status != localmodel.HermesTaskRunStatusAssigned {
		return fmt.Errorf("%w: run is not pending", ErrHermesInvalidInput)
	}
	instance, err := s.dao.GetInstance(ctx, run.InstanceID)
	if err != nil {
		return err
	}
	if instance.DeviceID != deviceID {
		return fmt.Errorf("%w: run is not owned by this device", ErrHermesInvalidInput)
	}
	return s.dao.SetTaskRunStatus(ctx, runID, localmodel.HermesTaskRunStatusAssigned, status, s.now().UTC(), result)
}
