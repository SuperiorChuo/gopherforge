package service

import (
	"context"
	"testing"

	localmodel "github.com/go-admin-kit/services/ai/internal/model"
)

// Kanban 里程碑：集群 slug 非空 = kanban 通道。lead 设备拿 Kanban 指令、
// worker 无设备无 Task/Kanban、ActivateTask 只给绑设备实例建 run。

func createHermesTestKanbanCluster(t *testing.T, svc *HermesService, ctx context.Context, name, slug string, policyID uint64) *localmodel.HermesCluster {
	t.Helper()
	cluster, err := svc.CreateCluster(ctx, 1, HermesClusterRequest{
		Name: name, Description: "kanban cluster", Slug: slug, PolicyID: policyID, Enabled: true,
	})
	if err != nil {
		t.Fatalf("create kanban cluster %s: %v", name, err)
	}
	return cluster
}

// 基准：kanban 集群 lead 设备拿 Kanban 指令，worker（无设备）无 Task/Kanban。
func TestBuildDesiredStateEmitsKanbanForLeadDevice(t *testing.T) {
	svc, store, ctx, _ := setupHermesServiceTest(t, true)
	companyPolicy := createHermesTestPolicy(t, svc, ctx, "company-policy", "company-model")
	createHermesTestEntitlement(t, svc, ctx, true, companyPolicy.ID)

	clusterPolicy := createHermesTestClusterPolicy(t, svc, ctx, "cluster-policy")
	cluster := createHermesTestKanbanCluster(t, svc, ctx, "kanban-cluster", "payments", clusterPolicy.ID)

	// lead 实例（绑设备）+ worker 实例（无设备）
	leadDevice := enrollHermesTestDevice(t, svc, store, ctx)
	lead, err := svc.CreateInstance(ctx, 1, HermesInstanceRequest{
		Name: "lead-agent", Role: "lead", OwnerUserID: 1, DeviceID: leadDevice.DeviceID, ClusterID: cluster.ID, Enabled: true,
	})
	if err != nil {
		t.Fatalf("create lead instance: %v", err)
	}
	if _, err := svc.CreateInstance(ctx, 1, HermesInstanceRequest{
		Name: "research-b", Role: "worker", OwnerUserID: 1, DeviceID: "", ClusterID: cluster.ID, Enabled: true,
	}); err != nil {
		t.Fatalf("create worker instance: %v", err)
	}

	task, err := svc.CreateTask(ctx, 1, HermesTaskRequest{
		Name: "kanban-task", Description: "拆解并派发", ClusterID: cluster.ID, PolicyID: clusterPolicy.ID,
	})
	if err != nil {
		t.Fatalf("create task: %v", err)
	}
	if _, err := svc.ActivateTask(ctx, 1, task.ID); err != nil {
		t.Fatalf("activate task: %v", err)
	}

	// lead 设备拿 Kanban，且 Task nil
	state, err := svc.DesiredState(ctx, leadDevice)
	if err != nil {
		t.Fatalf("lead desired state: %v", err)
	}
	if state.Kanban == nil {
		t.Fatal("lead device must carry a kanban directive")
	}
	if state.Task != nil {
		t.Fatal("lead device must NOT carry a push task")
	}
	if state.Kanban.BoardSlug != "payments" {
		t.Fatalf("kanban board slug = %q, want payments", state.Kanban.BoardSlug)
	}
	if state.Kanban.Assignee != "lead-agent" {
		t.Fatalf("kanban assignee = %q, want lead-agent (lead instance slugified)", state.Kanban.Assignee)
	}
	// worker 集含 lead + research-b
	names := map[string]bool{}
	for _, w := range state.Kanban.Workers {
		names[w.Name] = true
	}
	if !names["lead-agent"] || !names["research-b"] {
		t.Fatalf("kanban workers = %v, want lead-agent + research-b", state.Kanban.Workers)
	}
	_ = lead
}

// kanban 模式：ActivateTask 只为绑设备实例建 run（worker 无设备不建）。
func TestActivateTaskKanbanCreatesRunsOnlyForDeviceBound(t *testing.T) {
	svc, store, ctx, _ := setupHermesServiceTest(t, true)
	companyPolicy := createHermesTestPolicy(t, svc, ctx, "company-policy", "company-model")
	createHermesTestEntitlement(t, svc, ctx, true, companyPolicy.ID)

	clusterPolicy := createHermesTestClusterPolicy(t, svc, ctx, "cluster-policy")
	cluster := createHermesTestKanbanCluster(t, svc, ctx, "kanban-runs", "runs", clusterPolicy.ID)

	leadDevice := enrollHermesTestDevice(t, svc, store, ctx)
	if _, err := svc.CreateInstance(ctx, 1, HermesInstanceRequest{
		Name: "lead", Role: "lead", OwnerUserID: 1, DeviceID: leadDevice.DeviceID, ClusterID: cluster.ID, Enabled: true,
	}); err != nil {
		t.Fatalf("create lead: %v", err)
	}
	// 两个 worker（无设备）
	for _, name := range []string{"w1", "w2"} {
		if _, err := svc.CreateInstance(ctx, 1, HermesInstanceRequest{
			Name: name, Role: "worker", OwnerUserID: 1, DeviceID: "", ClusterID: cluster.ID, Enabled: true,
		}); err != nil {
			t.Fatalf("create worker %s: %v", name, err)
		}
	}

	task, err := svc.CreateTask(ctx, 1, HermesTaskRequest{
		Name: "t", Description: "x", ClusterID: cluster.ID, PolicyID: clusterPolicy.ID,
	})
	if err != nil {
		t.Fatalf("create task: %v", err)
	}
	if _, err := svc.ActivateTask(ctx, 1, task.ID); err != nil {
		t.Fatalf("activate: %v", err)
	}
	runs, err := store.ListTaskRuns(ctx, task.ID)
	if err != nil {
		t.Fatalf("list runs: %v", err)
	}
	if len(runs) != 1 {
		t.Fatalf("kanban mode runs = %d, want 1 (only device-bound lead)", len(runs))
	}
}

// 集群 slug 校验：非法 slug 拒、同租户重复 slug 拒、空 slug 合法。
func TestClusterSlugValidation(t *testing.T) {
	svc, _, ctx, _ := setupHermesServiceTest(t, true)

	// 非法 slug（大写）拒
	if _, err := svc.CreateCluster(ctx, 1, HermesClusterRequest{Name: "bad-slug", Slug: "BadSlug", Enabled: true}); err == nil {
		t.Fatal("uppercase slug must be rejected")
	}
	// 非法 slug（空格）拒
	if _, err := svc.CreateCluster(ctx, 1, HermesClusterRequest{Name: "space-slug", Slug: "bad slug", Enabled: true}); err == nil {
		t.Fatal("slug with space must be rejected")
	}

	// 合法 slug
	first, err := svc.CreateCluster(ctx, 1, HermesClusterRequest{Name: "good-slug", Slug: "payments", Enabled: true})
	if err != nil {
		t.Fatalf("create good slug: %v", err)
	}
	// 同租户重复 slug 拒（唯一索引）
	if _, err := svc.CreateCluster(ctx, 1, HermesClusterRequest{Name: "dup-slug", Slug: "payments", Enabled: true}); err == nil {
		t.Fatal("duplicate slug in tenant must be rejected")
	}
	// 空 slug 不冲突（多个空合法）
	if _, err := svc.CreateCluster(ctx, 1, HermesClusterRequest{Name: "empty-1", Enabled: true}); err != nil {
		t.Fatalf("empty slug 1: %v", err)
	}
	if _, err := svc.CreateCluster(ctx, 1, HermesClusterRequest{Name: "empty-2", Enabled: true}); err != nil {
		t.Fatalf("empty slug 2: %v", err)
	}
	_ = first
}

// 实例名 slugify：中文/空格 → 短横线，assignee 属于 worker 集。
func TestKanbanDirectiveWorkersSlugified(t *testing.T) {
	svc, store, ctx, _ := setupHermesServiceTest(t, true)
	companyPolicy := createHermesTestPolicy(t, svc, ctx, "company-policy", "company-model")
	createHermesTestEntitlement(t, svc, ctx, true, companyPolicy.ID)

	clusterPolicy := createHermesTestClusterPolicy(t, svc, ctx, "cluster-policy")
	cluster := createHermesTestKanbanCluster(t, svc, ctx, "slug-cluster", "slug", clusterPolicy.ID)

	leadDevice := enrollHermesTestDevice(t, svc, store, ctx)
	// 实例名含中文和空格 → slugify 后 research-agent-a
	if _, err := svc.CreateInstance(ctx, 1, HermesInstanceRequest{
		Name: "Research Agent A", Role: "lead", OwnerUserID: 1, DeviceID: leadDevice.DeviceID, ClusterID: cluster.ID, Enabled: true,
	}); err != nil {
		t.Fatalf("create lead: %v", err)
	}
	if _, err := svc.CreateInstance(ctx, 1, HermesInstanceRequest{
		Name: "研究助手B", Role: "worker", OwnerUserID: 1, DeviceID: "", ClusterID: cluster.ID, Enabled: true,
	}); err != nil {
		t.Fatalf("create worker: %v", err)
	}

	task, err := svc.CreateTask(ctx, 1, HermesTaskRequest{Name: "t", Description: "x", ClusterID: cluster.ID})
	if err != nil {
		t.Fatalf("create task: %v", err)
	}
	if _, err := svc.ActivateTask(ctx, 1, task.ID); err != nil {
		t.Fatalf("activate: %v", err)
	}

	state, err := svc.DesiredState(ctx, leadDevice)
	if err != nil {
		t.Fatalf("desired state: %v", err)
	}
	if state.Kanban == nil {
		t.Fatal("kanban directive missing")
	}
	// lead 实例 Research Agent A → research-agent-a
	if state.Kanban.Assignee != "research-agent-a" {
		t.Fatalf("assignee = %q, want research-agent-a", state.Kanban.Assignee)
	}
	// worker "研究助手B" → 中文全转 '-' 后只剩 "b"（slugify 语义正确）
	names := map[string]bool{}
	for _, w := range state.Kanban.Workers {
		names[w.Name] = true
	}
	if !names["research-agent-a"] || !names["b"] {
		t.Fatalf("slugified workers = %v, want research-agent-a + b", state.Kanban.Workers)
	}
}
