package service

import (
	"context"
	"errors"
	"testing"

	localmodel "github.com/go-admin-kit/services/ai/internal/model"
	"gorm.io/gorm"
)

// 第二期多 Agent 协作任务：集群 + 任务 + 执行记录。全部为控制面概念，
// company_cluster 策略本期不可应用到真实设备（manager isExactP1Toolset 未放行）。

func createHermesTestClusterPolicy(t *testing.T, svc *HermesService, ctx context.Context, name string) *localmodel.HermesPolicy {
	t.Helper()
	policy, err := svc.CreatePolicy(ctx, 1, CreateHermesPolicyRequest{
		Profile:      localmodel.HermesProfileCompanyCluster,
		Name:         name,
		DesiredState: localmodel.HermesDesiredStateActive,
		Toolsets:     localmodel.HermesToolsets{"clarify", "memory", "no_mcp", "todo", "delegation", "skills", "mcp"},
		ModelName:    "cluster-model",
		ModelBaseURL: "https://control.example/api/v1/ai/hermes/proxy/v1",
		LeaseSeconds: 3600,
	})
	if err != nil {
		t.Fatalf("create cluster policy %s: %v", name, err)
	}
	return policy
}

func createHermesTestCluster(t *testing.T, svc *HermesService, ctx context.Context, name string, policyID uint64) *localmodel.HermesCluster {
	t.Helper()
	cluster, err := svc.CreateCluster(ctx, 1, HermesClusterRequest{
		Name:        name,
		Description: "test cluster",
		PolicyID:    policyID,
		Enabled:     true,
	})
	if err != nil {
		t.Fatalf("create cluster %s: %v", name, err)
	}
	return cluster
}

func TestCreateClusterRequiresCompanyClusterPolicy(t *testing.T) {
	svc, _, ctx, _ := setupHermesServiceTest(t, true)
	companyPolicy := createHermesTestPolicy(t, svc, ctx, "company-policy", "model")

	_, err := svc.CreateCluster(ctx, 1, HermesClusterRequest{
		Name:     "bad-cluster",
		PolicyID: companyPolicy.ID,
		Enabled:  true,
	})
	if err == nil || !errors.Is(err, ErrHermesInvalidInput) {
		t.Fatalf("cluster referencing company policy: err = %v, want ErrHermesInvalidInput", err)
	}
}

func TestCreateClusterRejectsNonExistentPolicy(t *testing.T) {
	svc, _, ctx, _ := setupHermesServiceTest(t, true)
	_, err := svc.CreateCluster(ctx, 1, HermesClusterRequest{Name: "x", PolicyID: 999, Enabled: true})
	if err == nil || !errors.Is(err, ErrHermesInvalidInput) {
		t.Fatalf("cluster referencing missing policy: err = %v, want ErrHermesInvalidInput", err)
	}
}

func TestDeleteClusterGuardsAgainstInstancesAndTasks(t *testing.T) {
	svc, _, ctx, _ := setupHermesServiceTest(t, true)
	policy := createHermesTestClusterPolicy(t, svc, ctx, "cluster-policy")
	cluster := createHermesTestCluster(t, svc, ctx, "guarded", policy.ID)

	// 集群内放一个未绑节点实例后，删除必须失败
	if _, err := svc.CreateInstance(ctx, 1, HermesInstanceRequest{Name: "member", ClusterID: cluster.ID}); err != nil {
		t.Fatalf("create instance: %v", err)
	}
	err := svc.DeleteCluster(ctx, 1, cluster.ID)
	if err == nil || !errors.Is(err, ErrHermesInvalidInput) {
		t.Fatalf("delete non-empty cluster: err = %v, want ErrHermesInvalidInput", err)
	}

	// 集群下还有任务时，删除也必须失败
	task, err := svc.CreateTask(ctx, 1, HermesTaskRequest{
		Name: "t", Description: "x", ClusterID: cluster.ID,
	})
	if err != nil {
		t.Fatalf("create task: %v", err)
	}
	_ = task
	if err := svc.DeleteCluster(ctx, 1, cluster.ID); err == nil {
		t.Fatal("delete cluster with task should fail")
	}

	// 清掉实例与任务后再删应成功
	instances, _, err := svc.dao.ListInstances(ctx, 1, 10, "")
	if err != nil {
		t.Fatalf("list instances: %v", err)
	}
	for _, instance := range instances {
		if instance.ClusterID == cluster.ID {
			if err := svc.dao.DeleteInstance(ctx, instance.ID); err != nil {
				t.Fatalf("delete instance: %v", err)
			}
		}
	}
	tasks, _, err := svc.dao.ListTasks(ctx, 1, 10, "")
	if err != nil {
		t.Fatalf("list tasks: %v", err)
	}
	for _, item := range tasks {
		if item.ClusterID == cluster.ID {
			if err := svc.dao.DeleteTask(ctx, item.ID); err != nil {
				t.Fatalf("delete task: %v", err)
			}
		}
	}
	if err := svc.DeleteCluster(ctx, 1, cluster.ID); err != nil {
		t.Fatalf("delete empty cluster: %v", err)
	}
}

func TestCreateTaskValidatesClusterAndPolicy(t *testing.T) {
	svc, _, ctx, _ := setupHermesServiceTest(t, true)
	policy := createHermesTestClusterPolicy(t, svc, ctx, "cluster-policy")
	cluster := createHermesTestCluster(t, svc, ctx, "target", policy.ID)

	task, err := svc.CreateTask(ctx, 1, HermesTaskRequest{
		Name:        "task-a",
		Description: "分解与执行一次市场调研",
		ClusterID:   cluster.ID,
		PolicyID:    policy.ID,
	})
	if err != nil {
		t.Fatalf("create task: %v", err)
	}
	if task.Status != localmodel.HermesTaskStatusPending {
		t.Fatalf("new task status = %q, want pending", task.Status)
	}

	// 空正文必须拒绝（任务正文是决策 1 的核心）
	if _, err := svc.CreateTask(ctx, 1, HermesTaskRequest{Name: "empty-desc", ClusterID: cluster.ID}); err == nil {
		t.Fatal("task without description should be rejected")
	}
	// 不存在的集群必须拒绝
	if _, err := svc.CreateTask(ctx, 1, HermesTaskRequest{Name: "no-cluster", Description: "x", ClusterID: 999}); err == nil {
		t.Fatal("task referencing missing cluster should be rejected")
	}
	// company 策略不能作为任务策略（必须 company_cluster）
	companyPolicy := createHermesTestPolicy(t, svc, ctx, "company-policy", "model")
	if _, err := svc.CreateTask(ctx, 1, HermesTaskRequest{
		Name: "company-policy-task", Description: "x", ClusterID: cluster.ID, PolicyID: companyPolicy.ID,
	}); err == nil {
		t.Fatal("task referencing company policy should be rejected")
	}
}

func TestActivateTaskCreatesRunsAndGuardsTransitions(t *testing.T) {
	svc, _, ctx, _ := setupHermesServiceTest(t, true)
	policy := createHermesTestClusterPolicy(t, svc, ctx, "cluster-policy")
	cluster := createHermesTestCluster(t, svc, ctx, "active-cluster", policy.ID)

	// 集群内两个启用实例
	first, err := svc.CreateInstance(ctx, 1, HermesInstanceRequest{Name: "agent-a", ClusterID: cluster.ID, Enabled: true})
	if err != nil {
		t.Fatalf("create agent-a: %v", err)
	}
	second, err := svc.CreateInstance(ctx, 1, HermesInstanceRequest{Name: "agent-b", ClusterID: cluster.ID, Enabled: true})
	if err != nil {
		t.Fatalf("create agent-b: %v", err)
	}

	task, err := svc.CreateTask(ctx, 1, HermesTaskRequest{
		Name: "collab-task", Description: "两个 Agent 协同完成", ClusterID: cluster.ID, PolicyID: policy.ID,
	})
	if err != nil {
		t.Fatalf("create task: %v", err)
	}

	activated, err := svc.ActivateTask(ctx, 1, task.ID)
	if err != nil {
		t.Fatalf("activate task: %v", err)
	}
	if activated.Status != localmodel.HermesTaskStatusActive {
		t.Fatalf("activated task status = %q, want active", activated.Status)
	}

	runs, err := svc.dao.ListTaskRuns(ctx, task.ID)
	if err != nil {
		t.Fatalf("list runs: %v", err)
	}
	if len(runs) != 2 {
		t.Fatalf("runs count = %d, want 2", len(runs))
	}
	seenInstances := map[uint64]bool{}
	for _, run := range runs {
		seenInstances[run.InstanceID] = true
		if run.Status != localmodel.HermesTaskRunStatusAssigned {
			t.Fatalf("run status = %q, want assigned", run.Status)
		}
	}
	if !seenInstances[first.ID] || !seenInstances[second.ID] {
		t.Fatalf("runs should cover both cluster instances")
	}

	// 重复激活必须失败（来源状态守卫）
	if _, err := svc.ActivateTask(ctx, 1, task.ID); err == nil {
		t.Fatal("re-activating an active task should fail")
	}

	// 逐个 run 推进：assigned → running → completed
	for _, run := range runs {
		if err := svc.UpdateTaskRun(ctx, 1, run.ID, HermesTaskRunTransitionRequest{Status: localmodel.HermesTaskRunStatusRunning}); err != nil {
			t.Fatalf("run to running: %v", err)
		}
		if err := svc.UpdateTaskRun(ctx, 1, run.ID, HermesTaskRunTransitionRequest{
			Status: localmodel.HermesTaskRunStatusCompleted,
			Result: "完成了子任务",
		}); err != nil {
			t.Fatalf("run to completed: %v", err)
		}
		// 终态不能再转
		if err := svc.UpdateTaskRun(ctx, 1, run.ID, HermesTaskRunTransitionRequest{
			Status: localmodel.HermesTaskRunStatusFailed,
			Result: "x",
		}); err == nil {
			t.Fatal("transitioning a completed run should fail")
		}
	}

	// 完成整个任务
	if err := svc.CompleteTask(ctx, 1, task.ID); err != nil {
		t.Fatalf("complete task: %v", err)
	}

	detail, err := svc.GetTask(ctx, task.ID)
	if err != nil {
		t.Fatalf("get task detail: %v", err)
	}
	if detail.Task.Status != localmodel.HermesTaskStatusCompleted {
		t.Fatalf("task status = %q, want completed", detail.Task.Status)
	}
	if len(detail.Runs) != 2 {
		t.Fatalf("detail runs = %d, want 2", len(detail.Runs))
	}
	if detail.Cluster == nil || detail.Cluster.Name != "active-cluster" {
		t.Fatalf("detail cluster missing or wrong name")
	}
}

func TestCancelTaskMarksNonTerminalRunsFailed(t *testing.T) {
	svc, _, ctx, _ := setupHermesServiceTest(t, true)
	policy := createHermesTestClusterPolicy(t, svc, ctx, "cluster-policy")
	cluster := createHermesTestCluster(t, svc, ctx, "cancel-cluster", policy.ID)

	// 一个实例保持 assigned，另一个推进到 running
	if _, err := svc.CreateInstance(ctx, 1, HermesInstanceRequest{Name: "agent-a", ClusterID: cluster.ID, Enabled: true}); err != nil {
		t.Fatalf("create agent-a: %v", err)
	}
	if _, err := svc.CreateInstance(ctx, 1, HermesInstanceRequest{Name: "agent-b", ClusterID: cluster.ID, Enabled: true}); err != nil {
		t.Fatalf("create agent-b: %v", err)
	}

	task, err := svc.CreateTask(ctx, 1, HermesTaskRequest{Name: "cancel-me", Description: "会被取消", ClusterID: cluster.ID})
	if err != nil {
		t.Fatalf("create task: %v", err)
	}
	if _, err := svc.ActivateTask(ctx, 1, task.ID); err != nil {
		t.Fatalf("activate: %v", err)
	}
	runs, err := svc.dao.ListTaskRuns(ctx, task.ID)
	if err != nil {
		t.Fatalf("list runs: %v", err)
	}
	if len(runs) != 2 {
		t.Fatalf("runs = %d, want 2", len(runs))
	}
	if err := svc.UpdateTaskRun(ctx, 1, runs[0].ID, HermesTaskRunTransitionRequest{Status: localmodel.HermesTaskRunStatusRunning}); err != nil {
		t.Fatalf("run to running: %v", err)
	}

	if err := svc.CancelTask(ctx, 1, task.ID); err != nil {
		t.Fatalf("cancel task: %v", err)
	}

	after, err := svc.dao.ListTaskRuns(ctx, task.ID)
	if err != nil {
		t.Fatalf("list runs after cancel: %v", err)
	}
	for _, run := range after {
		if run.Status != localmodel.HermesTaskRunStatusFailed {
			t.Fatalf("run %d status after cancel = %q, want failed", run.ID, run.Status)
		}
		if run.Result != "任务取消，未执行" {
			t.Fatalf("run %d result = %q, want cancellation note", run.ID, run.Result)
		}
	}
}

func TestCompanyClusterPolicyCannotApplyToDevice(t *testing.T) {
	svc, store, ctx, _ := setupHermesServiceTest(t, true)
	companyPolicy := createHermesTestPolicy(t, svc, ctx, "company-policy", "model")
	clusterPolicy := createHermesTestClusterPolicy(t, svc, ctx, "cluster-policy")

	// 先建一个 company 策略的权益，让设备能完成注册
	createHermesTestEntitlement(t, svc, ctx, true, companyPolicy.ID)

	// 直接把 company_cluster 策略作为权益策略 → 必须拒绝
	err := svc.UpdateEntitlement(ctx, 1, 1, UpdateHermesEntitlementRequest{
		Enabled: true, PolicyID: clusterPolicy.ID, DeviceLimit: 1,
	})
	if err == nil || !errors.Is(err, ErrHermesInvalidInput) {
		t.Fatalf("entitlement with cluster policy: err = %v, want ErrHermesInvalidInput", err)
	}

	// 直接把 company_cluster 策略作为实例策略 → 必须拒绝
	device := enrollHermesTestDevice(t, svc, store, ctx)
	_, err = svc.CreateInstance(ctx, 1, HermesInstanceRequest{
		Name: "bad-instance", DeviceID: device.DeviceID, PolicyID: clusterPolicy.ID,
	})
	if err == nil || !errors.Is(err, ErrHermesInvalidInput) {
		t.Fatalf("instance with cluster policy: err = %v, want ErrHermesInvalidInput", err)
	}

	// 防御性边界：即使强行把 company_cluster 策略设到权益，buildDesiredState 也必须拒绝
	// （通过绕过 service 守卫直接写库的方式模拟最坏情况）
	ent := &localmodel.HermesEntitlement{
		TenantID: 1, UserID: 1, Enabled: true, PolicyID: clusterPolicy.ID, DeviceLimit: 1,
	}
	if err := store.UpsertEntitlement(ctx, ent); err != nil {
		t.Fatalf("upsert entitlement bypassing guard: %v", err)
	}
	if _, err := svc.DesiredState(ctx, device); err == nil || !errors.Is(err, ErrHermesInvalidInput) {
		t.Fatalf("desired state with cluster policy: err = %v, want ErrHermesInvalidInput", err)
	}
}

func TestCompanyClusterPolicyRequiresDelegationToolset(t *testing.T) {
	svc, _, ctx, _ := setupHermesServiceTest(t, true)
	// 不带 delegation 的 company_cluster 策略必须被拒绝
	_, err := svc.CreatePolicy(ctx, 1, CreateHermesPolicyRequest{
		Profile:      localmodel.HermesProfileCompanyCluster,
		Name:         "cluster-without-delegation",
		DesiredState: localmodel.HermesDesiredStateActive,
		Toolsets:     localmodel.HermesToolsets{"clarify", "memory", "no_mcp", "todo"},
		ModelName:    "cluster-model",
		ModelBaseURL: "https://control.example/api/v1/ai/hermes/proxy/v1",
		LeaseSeconds: 3600,
	})
	if err == nil || !errors.Is(err, ErrHermesInvalidInput) {
		t.Fatalf("cluster policy without delegation: err = %v, want ErrHermesInvalidInput", err)
	}
}

func TestListTasksReturnsEmptySliceNotNull(t *testing.T) {
	svc, _, ctx, _ := setupHermesServiceTest(t, true)
	tasks, total, err := svc.ListTasks(ctx, 1, 20, "")
	if err != nil {
		t.Fatalf("list tasks: %v", err)
	}
	if total != 0 {
		t.Fatalf("total = %d, want 0", total)
	}
	if tasks == nil {
		t.Fatal("empty task list must be non-nil slice")
	}
	if len(tasks) != 0 {
		t.Fatalf("tasks = %d, want 0", len(tasks))
	}
}

func TestGetTaskNotFound(t *testing.T) {
	svc, _, ctx, _ := setupHermesServiceTest(t, true)
	_, err := svc.GetTask(ctx, 999)
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		t.Fatalf("get missing task: err = %v, want gorm.ErrRecordNotFound", err)
	}
}

// 确保空上下文不会 panic（tenant 缺省 1）
func TestHermesTaskTenantDefaultsToOne(t *testing.T) {
	svc, _, _, _ := setupHermesServiceTest(t, true)
	policy, err := svc.CreatePolicy(context.Background(), 1, CreateHermesPolicyRequest{
		Profile:      localmodel.HermesProfileCompanyCluster,
		Name:         "cluster-policy",
		DesiredState: localmodel.HermesDesiredStateActive,
		Toolsets:     localmodel.HermesToolsets{"clarify", "memory", "no_mcp", "todo", "delegation"},
		ModelName:    "m",
		ModelBaseURL: "https://control.example/api/v1/ai/hermes/proxy/v1",
		LeaseSeconds: 3600,
	})
	if err != nil {
		t.Fatalf("create cluster policy: %v", err)
	}
	if policy.Profile != localmodel.HermesProfileCompanyCluster {
		t.Fatalf("policy profile = %q, want company_cluster", policy.Profile)
	}
	if policy.TenantID != 1 {
		t.Fatalf("policy tenant = %d, want 1", policy.TenantID)
	}
}
