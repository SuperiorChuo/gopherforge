package service

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/go-admin-kit/services/ai/internal/dao"
	localmodel "github.com/go-admin-kit/services/ai/internal/model"
	"github.com/go-admin-kit/services/shared/pkg/tenant"
)

// M6 员工自助升级：自助绑定守卫、目录、desired-state 集群∪个人并集、个人用量。

func TestSelfBindSkillRejectsUnapproved(t *testing.T) {
	svc, _, ctx, _ := setupHermesServiceTest(t, true)
	pending := createHermesTestSkill(t, svc, ctx, "pending-skill", "1.0.0")

	// pending 拒
	if err := svc.SelfBindSkill(ctx, 1, pending.ID); !errors.Is(err, ErrHermesInvalidInput) {
		t.Fatalf("pending skill bind error = %v, want invalid input", err)
	}
	// 不存在的技能拒
	if err := svc.SelfBindSkill(ctx, 1, 99999); !errors.Is(err, ErrHermesInvalidInput) {
		t.Fatalf("missing skill bind error = %v, want invalid input", err)
	}
	// 审批后可绑
	if err := svc.ApproveSkill(ctx, 1, pending.ID); err != nil {
		t.Fatalf("approve: %v", err)
	}
	if err := svc.SelfBindSkill(ctx, 1, pending.ID); err != nil {
		t.Fatalf("approved skill bind: %v", err)
	}
}

func TestSelfBindSkillIdempotent(t *testing.T) {
	svc, store, ctx, _ := setupHermesServiceTest(t, true)
	skill := createHermesTestSkill(t, svc, ctx, "idem-skill", "1.0.0")
	if err := svc.ApproveSkill(ctx, 1, skill.ID); err != nil {
		t.Fatalf("approve: %v", err)
	}
	if err := svc.SelfBindSkill(ctx, 1, skill.ID); err != nil {
		t.Fatalf("bind 1: %v", err)
	}
	if err := svc.SelfBindSkill(ctx, 1, skill.ID); err != nil {
		t.Fatalf("bind 2 must be idempotent, got %v", err)
	}
	var count int64
	if err := store.DB().WithContext(ctx).Model(&localmodel.HermesUserSkill{}).Count(&count).Error; err != nil {
		t.Fatalf("count: %v", err)
	}
	if count != 1 {
		t.Fatalf("bindings = %d, want 1", count)
	}
}

func TestSelfBindSkillCrossTenantNotFound(t *testing.T) {
	svc, _, ctx, _ := setupHermesServiceTest(t, true)
	skill := createHermesTestSkill(t, svc, ctx, "t1-skill", "1.0.0")
	if err := svc.ApproveSkill(ctx, 1, skill.ID); err != nil {
		t.Fatalf("approve: %v", err)
	}
	// 租户 2 的会话看不到租户 1 的技能（tenant 插件隔离 → not found）
	otherCtx := tenant.WithContext(context.Background(), 2)
	if err := svc.SelfBindSkill(otherCtx, 1, skill.ID); !errors.Is(err, ErrHermesInvalidInput) {
		t.Fatalf("cross-tenant bind error = %v, want invalid input (not found)", err)
	}
}

func TestListSkillCatalogApprovedOnlyWithInstalledFlag(t *testing.T) {
	svc, _, ctx, _ := setupHermesServiceTest(t, true)
	approved := createHermesTestSkill(t, svc, ctx, "approved-skill", "1.0.0")
	if err := svc.ApproveSkill(ctx, 1, approved.ID); err != nil {
		t.Fatalf("approve: %v", err)
	}
	createHermesTestSkill(t, svc, ctx, "still-pending", "1.0.0")

	if err := svc.SelfBindSkill(ctx, 1, approved.ID); err != nil {
		t.Fatalf("bind: %v", err)
	}
	items, total, err := svc.ListSkillCatalog(ctx, 1, 1, 20)
	if err != nil {
		t.Fatalf("catalog: %v", err)
	}
	if total != 1 || len(items) != 1 {
		t.Fatalf("catalog = %d/%d, want only the approved skill", len(items), total)
	}
	if items[0].Name != "approved-skill" || !items[0].Installed {
		t.Fatalf("catalog item = %+v, want approved-skill installed", items[0])
	}
	// 未安装用户视角 installed=false
	items2, _, err := svc.ListSkillCatalog(ctx, 2, 1, 20)
	if err != nil {
		t.Fatalf("catalog user2: %v", err)
	}
	if items2[0].Installed {
		t.Fatalf("user 2 should not have installed flag")
	}
}

// M6 核心回归：无集群设备此前恒零技能，用户自装后 desired-state 携带。
func TestDesiredStateIncludesUserSkillsForNonClusterDevice(t *testing.T) {
	svc, store, ctx, _ := setupHermesServiceTest(t, true)
	policy := createHermesTestPolicy(t, svc, ctx, "company-policy", "company-model")
	createHermesTestEntitlement(t, svc, ctx, true, policy.ID)
	device := enrollHermesTestDevice(t, svc, store, ctx) // user 1, 无实例无集群

	skill := createHermesTestSkill(t, svc, ctx, "self-skill", "1.0.0")
	if err := svc.ApproveSkill(ctx, 1, skill.ID); err != nil {
		t.Fatalf("approve: %v", err)
	}
	if err := svc.SelfBindSkill(ctx, 1, skill.ID); err != nil {
		t.Fatalf("self bind: %v", err)
	}

	state, err := svc.DesiredState(ctx, device)
	if err != nil {
		t.Fatalf("desired state: %v", err)
	}
	if len(state.Skills) != 1 || state.Skills[0].Name != "self-skill" {
		t.Fatalf("desired skills = %+v, want [self-skill]", state.Skills)
	}
	if state.Skills[0].SourceURL == "" || len(state.Skills[0].SHA256) != 64 {
		t.Fatalf("skill payload incomplete: %+v", state.Skills[0])
	}

	// 已装技能被 revoke 后 desired-state 掉出
	if err := svc.RevokeSkill(ctx, 1, skill.ID); err != nil {
		t.Fatalf("revoke: %v", err)
	}
	state, err = svc.DesiredState(ctx, device)
	if err != nil {
		t.Fatalf("desired state after revoke: %v", err)
	}
	if len(state.Skills) != 0 {
		t.Fatalf("revoked skill must drop out, got %+v", state.Skills)
	}
}

// 并集去重：集群同名优先 + 结果按名排序。
func TestDesiredStateSkillsUnionDedup(t *testing.T) {
	svc, store, ctx, _ := setupHermesServiceTest(t, true)
	companyPolicy := createHermesTestPolicy(t, svc, ctx, "company-policy", "company-model")
	createHermesTestEntitlement(t, svc, ctx, true, companyPolicy.ID)
	clusterPolicy := createHermesTestClusterPolicy(t, svc, ctx, "cluster-policy")
	cluster := createHermesTestCluster(t, svc, ctx, "union-cluster", clusterPolicy.ID)

	// 集群绑 shared@1.0.0
	sharedV1 := createHermesTestSkill(t, svc, ctx, "shared", "1.0.0")
	if err := svc.ApproveSkill(ctx, 1, sharedV1.ID); err != nil {
		t.Fatalf("approve v1: %v", err)
	}
	if err := svc.BindClusterSkill(ctx, 1, cluster.ID, sharedV1.ID); err != nil {
		t.Fatalf("cluster bind: %v", err)
	}
	// 用户绑 shared@2.0.0（同名不同版）+ 独有 zeta
	sharedV2 := createHermesTestSkill(t, svc, ctx, "shared", "2.0.0")
	if err := svc.ApproveSkill(ctx, 1, sharedV2.ID); err != nil {
		t.Fatalf("approve v2: %v", err)
	}
	zeta := createHermesTestSkill(t, svc, ctx, "zeta", "1.0.0")
	if err := svc.ApproveSkill(ctx, 1, zeta.ID); err != nil {
		t.Fatalf("approve zeta: %v", err)
	}
	if err := svc.SelfBindSkill(ctx, 1, sharedV2.ID); err != nil {
		t.Fatalf("self bind shared v2: %v", err)
	}
	if err := svc.SelfBindSkill(ctx, 1, zeta.ID); err != nil {
		t.Fatalf("self bind zeta: %v", err)
	}

	device := enrollHermesTestDevice(t, svc, store, ctx)
	if _, err := svc.CreateInstance(ctx, 1, HermesInstanceRequest{
		Name: "lead", Role: "lead", OwnerUserID: device.UserID, DeviceID: device.DeviceID, ClusterID: cluster.ID, Enabled: true,
	}); err != nil {
		t.Fatalf("create instance: %v", err)
	}

	state, err := svc.DesiredState(ctx, device)
	if err != nil {
		t.Fatalf("desired state: %v", err)
	}
	if len(state.Skills) != 2 {
		t.Fatalf("union skills = %+v, want 2 (shared dedup + zeta)", state.Skills)
	}
	// 排序：shared < zeta；shared 用集群的 1.0.0（管理员意图优先）
	if state.Skills[0].Name != "shared" || state.Skills[0].Version != "1.0.0" {
		t.Fatalf("skills[0] = %+v, want shared@1.0.0 (cluster wins)", state.Skills[0])
	}
	if state.Skills[1].Name != "zeta" {
		t.Fatalf("skills[1] = %+v, want zeta", state.Skills[1])
	}
}

// 无用户技能时 desired-state 字节稳定（skills omitempty 不出现）。
func TestDesiredStateByteStableWhenNoUserSkills(t *testing.T) {
	svc, store, ctx, _ := setupHermesServiceTest(t, true)
	policy := createHermesTestPolicy(t, svc, ctx, "company-policy", "company-model")
	createHermesTestEntitlement(t, svc, ctx, true, policy.ID)
	device := enrollHermesTestDevice(t, svc, store, ctx)

	state, err := svc.DesiredState(ctx, device)
	if err != nil {
		t.Fatalf("desired state: %v", err)
	}
	raw, err := json.Marshal(state)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var back map[string]any
	if err := json.Unmarshal(raw, &back); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if _, exists := back["skills"]; exists {
		t.Fatalf("no-skill payload must omit skills key: %s", raw)
	}
}

// MyUsage 只算本人（user_id 过滤）。
func TestMyUsageScopedToUser(t *testing.T) {
	_, store, ctx, _ := setupHermesServiceTest(t, true)
	db := store.DB()
	if err := db.AutoMigrate(&localmodel.AIUsageLog{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	now := time.Date(2026, 8, 4, 12, 0, 0, 0, time.UTC)
	usageSvc := NewHermesUsageService(dao.NewHermesUsageDAO(db), func() time.Time { return now })

	tid := uint64(1)
	seed := func(userID uint64, tokens int) {
		tk := tokens
		uid := userID
		if err := db.Create(&localmodel.AIUsageLog{
			ModelName: "m", UserID: &uid, TenantID: &tid, DeviceID: "dev-" + string(rune('a'+userID)),
			Operation: dao.HermesProxyOperation, Status: "success", TotalTokens: &tk,
			CreatedAt: now.Add(-time.Hour),
		}).Error; err != nil {
			t.Fatalf("seed: %v", err)
		}
	}
	seed(1, 100)
	seed(1, 150)
	seed(2, 9999) // 他人调用不该被算入

	mine, err := usageSvc.MyUsage(ctx, 1, 7)
	if err != nil {
		t.Fatalf("my usage: %v", err)
	}
	if mine.Summary.TotalRequests != 2 || mine.Summary.TotalTokens != 250 {
		t.Fatalf("my usage = %+v, want 2 requests / 250 tokens (他人调用漏进来了)", mine.Summary)
	}
	if len(mine.Daily) != 1 || mine.Daily[0].TotalRequests != 2 {
		t.Fatalf("daily = %+v, want one day with 2 requests", mine.Daily)
	}
}

// 能力预览：deviceView 返回 Capabilities（技能/工具集；MCP 仅名字）。
func TestDeviceViewCarriesCapabilities(t *testing.T) {
	svc, store, ctx, _ := setupHermesServiceTest(t, true)
	policy := createHermesTestPolicy(t, svc, ctx, "company-policy", "company-model")
	createHermesTestEntitlement(t, svc, ctx, true, policy.ID)
	device := enrollHermesTestDevice(t, svc, store, ctx)

	skill := createHermesTestSkill(t, svc, ctx, "cap-skill", "1.0.0")
	if err := svc.ApproveSkill(ctx, 1, skill.ID); err != nil {
		t.Fatalf("approve: %v", err)
	}
	if err := svc.SelfBindSkill(ctx, 1, skill.ID); err != nil {
		t.Fatalf("bind: %v", err)
	}

	view, err := svc.deviceView(ctx, *device)
	if err != nil {
		t.Fatalf("device view: %v", err)
	}
	if view.Capabilities == nil {
		t.Fatal("capabilities missing")
	}
	if len(view.Capabilities.Skills) != 1 || view.Capabilities.Skills[0].Name != "cap-skill" {
		t.Fatalf("capabilities skills = %+v, want [cap-skill]", view.Capabilities.Skills)
	}
	if len(view.Capabilities.Toolsets) == 0 {
		t.Fatalf("capabilities toolsets empty")
	}
}
