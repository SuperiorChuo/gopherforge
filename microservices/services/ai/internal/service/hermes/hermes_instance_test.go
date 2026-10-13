package service

import (
	"context"
	"errors"
	"testing"

	"github.com/go-admin-kit/services/ai/internal/dao"
	localmodel "github.com/go-admin-kit/services/ai/internal/model"
)

// enrollHermesTestDevice 走完整注册链拿到一台已登记设备，供实例测试绑定。
func enrollHermesTestDevice(t *testing.T, svc *HermesService, store *dao.HermesDAO, ctx context.Context) *localmodel.HermesDevice {
	t.Helper()
	authorization, _ := createHermesTestAuthorization(t, svc)
	if err := svc.ApproveDeviceAuthorization(ctx, 1, 1, authorization.UserCode); err != nil {
		t.Fatalf("approve authorization: %v", err)
	}
	token, err := svc.ExchangeDeviceCode(context.Background(), authorization.DeviceCode)
	if err != nil {
		t.Fatalf("exchange device code: %v", err)
	}
	device, err := store.GetDevice(ctx, token.DeviceID)
	if err != nil {
		t.Fatalf("get device: %v", err)
	}
	return device
}

func createHermesTestPolicy(t *testing.T, svc *HermesService, ctx context.Context, name, modelName string) *localmodel.HermesPolicy {
	t.Helper()
	policy, err := svc.CreatePolicy(ctx, 1, CreateHermesPolicyRequest{
		Name:         name,
		DesiredState: localmodel.HermesDesiredStateActive,
		Toolsets:     localmodel.HermesToolsets{"clarify", "memory", "no_mcp", "todo"},
		ModelName:    modelName,
		ModelBaseURL: "https://control.example/api/v1/ai/hermes/proxy/v1",
		LeaseSeconds: 3600,
	})
	if err != nil {
		t.Fatalf("create policy %s: %v", name, err)
	}
	return policy
}

// 策略解析必须逐层回落：实例 → 权益 → 最新。存量设备没有实例，升级后期望状态
// 必须一字不变——这条是本次改造的兼容性底线。
func TestDesiredStateResolvesPolicyThroughInstanceThenEntitlement(t *testing.T) {
	svc, store, ctx, _ := setupHermesServiceTest(t, true)

	entitlementPolicy := createHermesTestPolicy(t, svc, ctx, "entitlement-policy", "model-from-entitlement")
	instancePolicy := createHermesTestPolicy(t, svc, ctx, "instance-policy", "model-from-instance")

	createHermesTestEntitlement(t, svc, ctx, true, entitlementPolicy.ID)
	device := enrollHermesTestDevice(t, svc, store, ctx)

	// ① 无实例：走权益策略（存量设备的行为）
	state, err := svc.DesiredState(ctx, device)
	if err != nil {
		t.Fatalf("desired state without instance: %v", err)
	}
	if state.Model.Model != "model-from-entitlement" {
		t.Fatalf("without instance model = %q, want model-from-entitlement", state.Model.Model)
	}

	// ② 有实例但 policy_id=0：仍回落权益策略（迁移给存量设备建的就是这种）
	instance, err := svc.CreateInstance(ctx, 1, HermesInstanceRequest{
		Name: "agent-a", Role: "dev", OwnerUserID: 1, DeviceID: device.DeviceID, Enabled: true,
	})
	if err != nil {
		t.Fatalf("create instance: %v", err)
	}
	state, err = svc.DesiredState(ctx, device)
	if err != nil {
		t.Fatalf("desired state with pass-through instance: %v", err)
	}
	if state.Model.Model != "model-from-entitlement" {
		t.Fatalf("pass-through instance model = %q, want model-from-entitlement", state.Model.Model)
	}

	// ③ 实例绑了自己的策略：覆盖权益策略——这一步证明实例层有真实运行效果
	if err := svc.UpdateInstance(ctx, 1, instance.ID, HermesInstanceRequest{
		Name: "agent-a", Role: "dev", OwnerUserID: 1, DeviceID: device.DeviceID,
		PolicyID: instancePolicy.ID, Enabled: true,
	}); err != nil {
		t.Fatalf("bind instance policy: %v", err)
	}
	state, err = svc.DesiredState(ctx, device)
	if err != nil {
		t.Fatalf("desired state with instance policy: %v", err)
	}
	if state.Model.Model != "model-from-instance" {
		t.Fatalf("instance policy model = %q, want model-from-instance", state.Model.Model)
	}
}

// 停用实例必须让节点停跑。只查启用实例的话，停用会静默退回权益策略继续跑，
// 与管理员意图相反——这是 GetInstanceByDevice 刻意连停用实例一起返回的原因。
func TestDesiredStateStopsWhenInstanceDisabled(t *testing.T) {
	svc, store, ctx, _ := setupHermesServiceTest(t, true)

	policy := createHermesTestPolicy(t, svc, ctx, "active-policy", "model-x")
	createHermesTestEntitlement(t, svc, ctx, true, policy.ID)
	device := enrollHermesTestDevice(t, svc, store, ctx)

	instance, err := svc.CreateInstance(ctx, 1, HermesInstanceRequest{
		Name: "agent-a", Role: "dev", OwnerUserID: 1, DeviceID: device.DeviceID, Enabled: true,
	})
	if err != nil {
		t.Fatalf("create instance: %v", err)
	}
	if err := svc.UpdateInstance(ctx, 1, instance.ID, HermesInstanceRequest{
		Name: "agent-a", Role: "dev", OwnerUserID: 1, DeviceID: device.DeviceID, Enabled: false,
	}); err != nil {
		t.Fatalf("disable instance: %v", err)
	}

	state, err := svc.DesiredState(ctx, device)
	if err != nil {
		t.Fatalf("desired state: %v", err)
	}
	if state.DesiredState != localmodel.HermesDesiredStateStopped {
		t.Fatalf("desired state = %q, want stopped", state.DesiredState)
	}
}

// 单实例守卫：schema 允许一节点多实例，但 manager 还只跑一个，所以服务层拦。
// 未绑节点的实例不占运行位，可以随便多建。
func TestCreateInstanceEnforcesOneEnabledInstancePerDevice(t *testing.T) {
	svc, store, ctx, _ := setupHermesServiceTest(t, true)

	policy := createHermesTestPolicy(t, svc, ctx, "p", "model-x")
	createHermesTestEntitlement(t, svc, ctx, true, policy.ID)
	device := enrollHermesTestDevice(t, svc, store, ctx)

	if _, err := svc.CreateInstance(ctx, 1, HermesInstanceRequest{
		Name: "agent-a", Role: "dev", OwnerUserID: 1, DeviceID: device.DeviceID, Enabled: true,
	}); err != nil {
		t.Fatalf("create first instance: %v", err)
	}

	_, err := svc.CreateInstance(ctx, 1, HermesInstanceRequest{
		Name: "agent-b", Role: "qa", OwnerUserID: 1, DeviceID: device.DeviceID, Enabled: true,
	})
	if !errors.Is(err, ErrHermesInstanceLimit) {
		t.Fatalf("second enabled instance error = %v, want ErrHermesInstanceLimit", err)
	}

	// 停用的第二个实例可以建：它不占运行位
	if _, err := svc.CreateInstance(ctx, 1, HermesInstanceRequest{
		Name: "agent-b", Role: "qa", OwnerUserID: 1, DeviceID: device.DeviceID, Enabled: false,
	}); err != nil {
		t.Fatalf("create disabled second instance: %v", err)
	}

	// 未绑节点的实例也可以随便建
	if _, err := svc.CreateInstance(ctx, 1, HermesInstanceRequest{
		Name: "agent-floating", Role: "doc", OwnerUserID: 1, Enabled: true,
	}); err != nil {
		t.Fatalf("create unassigned instance: %v", err)
	}
}

// 更新实例时必须把自己排除在守卫计数之外，否则改自己的名字都会被判成
// 「该节点第二个实例」。
func TestUpdateInstanceExcludesItselfFromGuard(t *testing.T) {
	svc, store, ctx, _ := setupHermesServiceTest(t, true)

	policy := createHermesTestPolicy(t, svc, ctx, "p", "model-x")
	createHermesTestEntitlement(t, svc, ctx, true, policy.ID)
	device := enrollHermesTestDevice(t, svc, store, ctx)

	instance, err := svc.CreateInstance(ctx, 1, HermesInstanceRequest{
		Name: "agent-a", Role: "dev", OwnerUserID: 1, DeviceID: device.DeviceID, Enabled: true,
	})
	if err != nil {
		t.Fatalf("create instance: %v", err)
	}
	if err := svc.UpdateInstance(ctx, 1, instance.ID, HermesInstanceRequest{
		Name: "agent-a-renamed", Role: "dev", OwnerUserID: 1, DeviceID: device.DeviceID, Enabled: true,
	}); err != nil {
		t.Fatalf("rename own instance: %v", err)
	}
}

// 未绑节点的实例不能影响任何设备的期望状态。
func TestUnassignedInstanceDoesNotAffectDevices(t *testing.T) {
	svc, store, ctx, _ := setupHermesServiceTest(t, true)

	entitlementPolicy := createHermesTestPolicy(t, svc, ctx, "entitlement-policy", "model-from-entitlement")
	otherPolicy := createHermesTestPolicy(t, svc, ctx, "other-policy", "model-should-not-apply")
	createHermesTestEntitlement(t, svc, ctx, true, entitlementPolicy.ID)
	device := enrollHermesTestDevice(t, svc, store, ctx)

	if _, err := svc.CreateInstance(ctx, 1, HermesInstanceRequest{
		Name: "floating", Role: "dev", OwnerUserID: 1, PolicyID: otherPolicy.ID, Enabled: true,
	}); err != nil {
		t.Fatalf("create unassigned instance: %v", err)
	}

	state, err := svc.DesiredState(ctx, device)
	if err != nil {
		t.Fatalf("desired state: %v", err)
	}
	if state.Model.Model != "model-from-entitlement" {
		t.Fatalf("model = %q, want model-from-entitlement (unassigned instance must not apply)", state.Model.Model)
	}
}

// 绑定不存在的节点或策略要在写入前挡下，否则实例会指向一台永远收不到期望
// 状态的机器。
func TestCreateInstanceRejectsUnknownDeviceAndPolicy(t *testing.T) {
	svc, _, ctx, _ := setupHermesServiceTest(t, true)

	if _, err := svc.CreateInstance(ctx, 1, HermesInstanceRequest{
		Name: "bad-device", Role: "dev", OwnerUserID: 1, DeviceID: "no-such-device", Enabled: true,
	}); !errors.Is(err, ErrHermesInvalidInput) {
		t.Fatalf("unknown device error = %v, want ErrHermesInvalidInput", err)
	}

	if _, err := svc.CreateInstance(ctx, 1, HermesInstanceRequest{
		Name: "bad-policy", Role: "dev", OwnerUserID: 1, PolicyID: 9999, Enabled: true,
	}); !errors.Is(err, ErrHermesInvalidInput) {
		t.Fatalf("unknown policy error = %v, want ErrHermesInvalidInput", err)
	}
}
