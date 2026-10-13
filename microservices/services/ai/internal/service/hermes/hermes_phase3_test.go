package service

import (
	"errors"
	"strings"
	"testing"

	localmodel "github.com/go-admin-kit/services/ai/internal/model"
)

// 三期：manager 下发通道。验证 company_cluster 策略经集群归属真正到设备、
// desired-state 携带 assigned run 作为任务意图、设备端结果回传归属校验。

// 三期核心：设备实例入启用集群且集群绑了 company_cluster 策略 → desired-state
// 用集群策略（dispatcher 工具集 + cluster profile），而非被守卫拒绝。
func TestClusterDeviceGetsCompanyClusterDesiredState(t *testing.T) {
	svc, store, ctx, _ := setupHermesServiceTest(t, true)
	companyPolicy := createHermesTestPolicy(t, svc, ctx, "company-policy", "company-model")
	createHermesTestEntitlement(t, svc, ctx, true, companyPolicy.ID)

	clusterPolicy := createHermesTestClusterPolicy(t, svc, ctx, "cluster-policy")
	cluster := createHermesTestCluster(t, svc, ctx, "worker-cluster", clusterPolicy.ID)

	device := enrollHermesTestDevice(t, svc, store, ctx)
	inst, err := svc.CreateInstance(ctx, 1, HermesInstanceRequest{
		Name: "worker-agent", Role: "worker", OwnerUserID: 1,
		DeviceID: device.DeviceID, ClusterID: cluster.ID, Enabled: true,
	})
	if err != nil {
		t.Fatalf("create worker instance: %v", err)
	}

	state, err := svc.DesiredState(ctx, device)
	if err != nil {
		t.Fatalf("desired state for cluster device: %v", err)
	}
	if state.Profile != localmodel.HermesProfileCompanyCluster {
		t.Fatalf("cluster device profile = %q, want company_cluster", state.Profile)
	}
	if state.Model.Model != clusterPolicy.ModelName {
		t.Fatalf("cluster device model = %q, want cluster policy %q", state.Model.Model, clusterPolicy.ModelName)
	}
	// dispatcher 工具集必须随签名状态下发（manager 三期已放行）
	hasDelegation := false
	for _, tool := range state.Toolsets {
		if tool == "delegation" {
			hasDelegation = true
		}
	}
	if !hasDelegation {
		t.Fatalf("cluster device toolsets = %v, want delegation present", state.Toolsets)
	}
	_ = inst
}

// 集群内实例待执行 run 时，desired-state 必须携带 Task 意图。
func TestBuildDesiredStateCarriesAssignedTask(t *testing.T) {
	svc, store, ctx, _ := setupHermesServiceTest(t, true)
	companyPolicy := createHermesTestPolicy(t, svc, ctx, "company-policy", "company-model")
	createHermesTestEntitlement(t, svc, ctx, true, companyPolicy.ID)

	clusterPolicy := createHermesTestClusterPolicy(t, svc, ctx, "cluster-policy")
	cluster := createHermesTestCluster(t, svc, ctx, "task-cluster", clusterPolicy.ID)

	device := enrollHermesTestDevice(t, svc, store, ctx)
	inst, err := svc.CreateInstance(ctx, 1, HermesInstanceRequest{
		Name: "task-worker", Role: "worker", OwnerUserID: 1,
		DeviceID: device.DeviceID, ClusterID: cluster.ID, Enabled: true,
	})
	if err != nil {
		t.Fatalf("create worker instance: %v", err)
	}

	task, err := svc.CreateTask(ctx, 1, HermesTaskRequest{
		Name: "phase3-task", Description: "三期的任务正文", ClusterID: cluster.ID, PolicyID: clusterPolicy.ID,
	})
	if err != nil {
		t.Fatalf("create task: %v", err)
	}
	if _, err := svc.ActivateTask(ctx, 1, task.ID); err != nil {
		t.Fatalf("activate task: %v", err)
	}

	state, err := svc.DesiredState(ctx, device)
	if err != nil {
		t.Fatalf("desired state: %v", err)
	}
	if state.Task == nil {
		t.Fatal("cluster device with assigned run must carry a task intent")
	}
	if state.Task.TaskID != task.ID {
		t.Fatalf("task intent task_id = %d, want %d", state.Task.TaskID, task.ID)
	}
	if state.Task.Description != "三期的任务正文" {
		t.Fatalf("task intent description = %q, want 三期的任务正文", state.Task.Description)
	}
	if state.Task.Model != clusterPolicy.ModelName {
		t.Fatalf("task intent model = %q, want cluster policy model", state.Task.Model)
	}
	_ = inst
}

// 没有 pending run 时 Task 必须为 nil（P1 payload 字节不变）。
func TestBuildDesiredStateOmitsTaskWhenNoPendingRun(t *testing.T) {
	svc, store, ctx, _ := setupHermesServiceTest(t, true)
	companyPolicy := createHermesTestPolicy(t, svc, ctx, "company-policy", "company-model")
	createHermesTestEntitlement(t, svc, ctx, true, companyPolicy.ID)

	clusterPolicy := createHermesTestClusterPolicy(t, svc, ctx, "cluster-policy")
	cluster := createHermesTestCluster(t, svc, ctx, "no-task-cluster", clusterPolicy.ID)

	device := enrollHermesTestDevice(t, svc, store, ctx)
	if _, err := svc.CreateInstance(ctx, 1, HermesInstanceRequest{
		Name: "idle-worker", Role: "worker", OwnerUserID: 1,
		DeviceID: device.DeviceID, ClusterID: cluster.ID, Enabled: true,
	}); err != nil {
		t.Fatalf("create instance: %v", err)
	}

	state, err := svc.DesiredState(ctx, device)
	if err != nil {
		t.Fatalf("desired state: %v", err)
	}
	if state.Task != nil {
		t.Fatalf("no pending run should omit task, got %+v", state.Task)
	}
}

// 设备端结果回传：run 归属校验 + 终态直转 + 非本设备拒绝。
func TestReportDeviceTaskRun(t *testing.T) {
	svc, store, ctx, _ := setupHermesServiceTest(t, true)
	companyPolicy := createHermesTestPolicy(t, svc, ctx, "company-policy", "company-model")
	createHermesTestEntitlement(t, svc, ctx, true, companyPolicy.ID)

	clusterPolicy := createHermesTestClusterPolicy(t, svc, ctx, "cluster-policy")
	cluster := createHermesTestCluster(t, svc, ctx, "report-cluster", clusterPolicy.ID)

	device := enrollHermesTestDevice(t, svc, store, ctx)
	inst, err := svc.CreateInstance(ctx, 1, HermesInstanceRequest{
		Name: "report-worker", Role: "worker", OwnerUserID: 1,
		DeviceID: device.DeviceID, ClusterID: cluster.ID, Enabled: true,
	})
	if err != nil {
		t.Fatalf("create instance: %v", err)
	}

	task, err := svc.CreateTask(ctx, 1, HermesTaskRequest{
		Name: "report-task", Description: "结果回传任务", ClusterID: cluster.ID, PolicyID: clusterPolicy.ID,
	})
	if err != nil {
		t.Fatalf("create task: %v", err)
	}
	if _, err := svc.ActivateTask(ctx, 1, task.ID); err != nil {
		t.Fatalf("activate task: %v", err)
	}
	runs, err := store.ListTaskRuns(ctx, task.ID)
	if err != nil || len(runs) != 1 {
		t.Fatalf("runs = %v (err %v), want 1", runs, err)
	}
	run := runs[0]

	// 正常回传：assigned → completed + result
	if err := svc.ReportDeviceTaskRun(ctx, device.DeviceID, run.ID, "completed", "任务执行完毕"); err != nil {
		t.Fatalf("report completed: %v", err)
	}
	after, _ := store.GetTaskRun(ctx, run.ID)
	if after.Status != localmodel.HermesTaskRunStatusCompleted || after.Result != "任务执行完毕" {
		t.Fatalf("run after report = %s/%q, want completed/任务执行完毕", after.Status, after.Result)
	}

	// 非本设备拒绝（run 属于 device，用别的 deviceID 报）
	if err := svc.ReportDeviceTaskRun(ctx, "00000000-other-4000-8000-0000000000ff", run.ID, "failed", "x"); err == nil {
		t.Fatal("reporting from a different device must be rejected")
	}

	// 已终态再报拒绝
	if err := svc.ReportDeviceTaskRun(ctx, device.DeviceID, run.ID, "failed", "x"); err == nil {
		t.Fatal("re-reporting a terminal run must be rejected")
	}

	// 非法状态拒绝
	if err := svc.ReportDeviceTaskRun(ctx, device.DeviceID, run.ID, "running", "x"); err == nil {
		t.Fatal("device path must reject non-terminal status")
	}

	// 空结果拒绝
	if err := svc.ReportDeviceTaskRun(ctx, device.DeviceID, run.ID, "failed", ""); err == nil {
		t.Fatal("device path must reject empty result")
	}
	_ = inst
}

// DB 绕过（cluster 策略直接放到权益上）仍被防御不变式拒绝。
func TestClusterPolicyOnEntitlementStillRejected(t *testing.T) {
	svc, store, ctx, _ := setupHermesServiceTest(t, true)
	companyPolicy := createHermesTestPolicy(t, svc, ctx, "company-policy", "company-model")
	createHermesTestEntitlement(t, svc, ctx, true, companyPolicy.ID)

	clusterPolicy := createHermesTestClusterPolicy(t, svc, ctx, "cluster-policy")

	device := enrollHermesTestDevice(t, svc, store, ctx)

	// 绕过 service 守卫直接写库：把 cluster 策略设到权益
	ent := &localmodel.HermesEntitlement{
		TenantID: 1, UserID: 1, Enabled: true, PolicyID: clusterPolicy.ID, DeviceLimit: 1,
	}
	if err := store.UpsertEntitlement(ctx, ent); err != nil {
		t.Fatalf("upsert entitlement bypassing guard: %v", err)
	}
	_, err := svc.DesiredState(ctx, device)
	if err == nil {
		t.Fatal("desired state with cluster policy on entitlement must be rejected")
	}
	if !errors.Is(err, ErrHermesInvalidInput) {
		t.Fatalf("err = %v, want ErrHermesInvalidInput", err)
	}
	if !strings.Contains(err.Error(), "company_cluster") {
		t.Fatalf("err = %v, want company_cluster mention", err)
	}
}
