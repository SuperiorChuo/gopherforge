package store

import (
	"context"
	"fmt"
	"testing"

	"github.com/go-admin-kit/services/im/internal/model"
)

// assignedConversation 造一条已分配给指定坐席的会话。
func assignedConversation(t *testing.T, s *Store, tenantID, agentUserID uint64) {
	t.Helper()
	v, err := s.UpsertVisitor(tenantID, fmt.Sprintf("batch-guest-%d", retDBSeq.Add(1)), "")
	if err != nil {
		t.Fatal(err)
	}
	c, err := s.CreateConversation(tenantID, 1, v.ID, "h5", "{}", nil, false)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.db.Model(c).Updates(map[string]any{
		"agent_user_id": agentUserID,
		"status":        "assigned",
	}).Error; err != nil {
		t.Fatal(err)
	}
}

// 批量在接数与逐条 CountAssignedForAgent 必须完全一致（含 0 负载坐席）。
func TestCountAssignedForAgentsMatchesPerAgent(t *testing.T) {
	s := newTestStore(t)
	for i := 0; i < 3; i++ {
		assignedConversation(t, s, 1, 101)
	}
	assignedConversation(t, s, 1, 102)
	// 关闭态会话不该计入
	v, _ := s.UpsertVisitor(1, "batch-closed", "")
	c, _ := s.CreateConversation(1, 1, v.ID, "h5", "{}", nil, false)
	if err := s.db.Model(c).Updates(map[string]any{
		"agent_user_id": 103, "status": "closed",
	}).Error; err != nil {
		t.Fatal(err)
	}

	ids := []uint64{101, 102, 103, 104}
	got, err := s.CountAssignedForAgents(ids)
	if err != nil {
		t.Fatal(err)
	}
	for _, id := range ids {
		want, err := s.CountAssignedForAgent(id)
		if err != nil {
			t.Fatal(err)
		}
		if got[id] != want {
			t.Errorf("agent %d: batch = %d, per-agent = %d", id, got[id], want)
		}
	}
	if got[101] != 3 || got[102] != 1 || got[103] != 0 || got[104] != 0 {
		t.Fatalf("unexpected counts: %v", got)
	}
}

func TestCountAssignedForAgentsEmptyAndZeroIDs(t *testing.T) {
	s := newTestStore(t)
	for name, ids := range map[string][]uint64{
		"nil":   nil,
		"empty": {},
		"zeros": {0, 0},
	} {
		got, err := s.CountAssignedForAgents(ids)
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if len(got) != 0 {
			t.Errorf("%s: got %v, want empty", name, got)
		}
	}
}

// 批量 presence 与逐条 GetPresence 一致，缺行的坐席同样补 offline。
func TestGetPresenceMapMatchesGetPresence(t *testing.T) {
	s := newTestStore(t)
	if _, err := s.UpsertPresence(201, "online", "alice"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.UpsertPresence(202, "busy", "bob"); err != nil {
		t.Fatal(err)
	}

	ids := []uint64{201, 202, 203}
	got, err := s.GetPresenceMap(ids)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 3 {
		t.Fatalf("got %d entries, want 3", len(got))
	}
	for _, id := range ids {
		want, err := s.GetPresence(id)
		if err != nil {
			t.Fatal(err)
		}
		g := got[id]
		if g == nil {
			t.Fatalf("agent %d missing from batch result", id)
		}
		if g.AgentUserID != want.AgentUserID || g.Status != want.Status || g.DisplayName != want.DisplayName {
			t.Errorf("agent %d: batch = %+v, per-agent = %+v", id, g, want)
		}
	}
	// 无 presence 行的坐席补 offline 占位，与 GetPresence 同义
	if got[203].Status != "offline" {
		t.Errorf("agent 203 status = %q, want offline", got[203].Status)
	}
	// 重复 id 不产生重复项
	dup, err := s.GetPresenceMap([]uint64{201, 201, 202})
	if err != nil {
		t.Fatal(err)
	}
	if len(dup) != 2 {
		t.Fatalf("dedup: got %d entries, want 2", len(dup))
	}
}

// 批量技能组坐席数与逐组 ListAgentSkills 长度一致。
func TestCountAgentSkillsByGroupMatchesPerGroup(t *testing.T) {
	s := newTestStore(t)
	g1, err := s.CreateSkillGroup(context.Background(), SkillGroupInput{TenantID: 1, Name: "g1", Code: "g1", Strategy: "round_robin", Status: 1})
	if err != nil {
		t.Fatal(err)
	}
	g2, err := s.CreateSkillGroup(context.Background(), SkillGroupInput{TenantID: 1, Name: "g2", Code: "g2", Strategy: "round_robin", Status: 1})
	if err != nil {
		t.Fatal(err)
	}
	g3, err := s.CreateSkillGroup(context.Background(), SkillGroupInput{TenantID: 1, Name: "g3", Code: "g3", Strategy: "round_robin", Status: 1})
	if err != nil {
		t.Fatal(err)
	}
	for _, uid := range []uint64{301, 302} {
		if _, err := s.UpsertAgentSkill(context.Background(), uid, g1.ID, 5, 1); err != nil {
			t.Fatal(err)
		}
	}
	// 停用绑定也计入：与原来 ListAgentSkills(g.ID, 0) 无 status 过滤保持一致
	if _, err := s.UpsertAgentSkill(context.Background(), 303, g2.ID, 5, 2); err != nil {
		t.Fatal(err)
	}

	ids := []uint64{g1.ID, g2.ID, g3.ID}
	got, err := s.CountAgentSkillsByGroup(ids)
	if err != nil {
		t.Fatal(err)
	}
	for _, id := range ids {
		skills, err := s.ListAgentSkills(id, 0)
		if err != nil {
			t.Fatal(err)
		}
		if got[id] != len(skills) {
			t.Errorf("group %d: batch = %d, per-group = %d", id, got[id], len(skills))
		}
	}
	if got[g1.ID] != 2 || got[g2.ID] != 1 || got[g3.ID] != 0 {
		t.Fatalf("unexpected counts: %v", got)
	}
}

// presence 读必须按租户收口：不能返回别的租户的坐席。
func TestListPresenceFiltersByTenant(t *testing.T) {
	s := newTestStore(t)
	gA, err := s.CreateSkillGroup(context.Background(), SkillGroupInput{TenantID: 1, Name: "tenant1-group", Code: "tenant1-group", Strategy: "round_robin", Status: 1})
	if err != nil {
		t.Fatal(err)
	}
	gB, err := s.CreateSkillGroup(context.Background(), SkillGroupInput{TenantID: 2, Name: "tenant2-group", Code: "tenant2-group", Strategy: "round_robin", Status: 1})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.UpsertPresence(401, "online", "t1-agent"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.UpsertPresence(402, "online", "t2-agent"); err != nil {
		t.Fatal(err)
	}
	// 无任何技能组绑定的坐席：不属于任何租户，租户视图里不该出现
	if _, err := s.UpsertPresence(403, "online", "orphan"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.UpsertAgentSkill(context.Background(), 401, gA.ID, 5, 1); err != nil {
		t.Fatal(err)
	}
	if _, err := s.UpsertAgentSkill(context.Background(), 402, gB.ID, 5, 1); err != nil {
		t.Fatal(err)
	}

	t1, err := s.ListPresence(1, "online", "busy")
	if err != nil {
		t.Fatal(err)
	}
	if len(t1) != 1 || t1[0].AgentUserID != 401 {
		t.Fatalf("tenant 1 presence = %+v, want only agent 401", t1)
	}
	t2, err := s.ListPresence(2, "online", "busy")
	if err != nil {
		t.Fatal(err)
	}
	if len(t2) != 1 || t2[0].AgentUserID != 402 {
		t.Fatalf("tenant 2 presence = %+v, want only agent 402", t2)
	}
	// 状态过滤仍生效
	if _, err := s.UpsertPresence(401, "offline", "t1-agent"); err != nil {
		t.Fatal(err)
	}
	if got, err := s.ListPresence(1, "online", "busy"); err != nil {
		t.Fatal(err)
	} else if len(got) != 0 {
		t.Fatalf("offline agent leaked into online filter: %+v", got)
	}
	// tenantID == 0 是内部全平台口径，仍能看到全部（且带上限）
	all, err := s.ListPresence(0)
	if err != nil {
		t.Fatal(err)
	}
	if len(all) != 3 {
		t.Fatalf("platform-wide presence = %d rows, want 3", len(all))
	}
}

// 跨技能组批量取排队会话，等价于逐组 ListAgentConversations(scope=queue) 的并集，
// 且不跨租户。
func TestListQueuedForSkillGroups(t *testing.T) {
	s := newTestStore(t)
	g1, err := s.CreateSkillGroup(context.Background(), SkillGroupInput{TenantID: 1, Name: "q1", Code: "q1", Strategy: "round_robin", Status: 1})
	if err != nil {
		t.Fatal(err)
	}
	g2, err := s.CreateSkillGroup(context.Background(), SkillGroupInput{TenantID: 1, Name: "q2", Code: "q2", Strategy: "round_robin", Status: 1})
	if err != nil {
		t.Fatal(err)
	}
	other, err := s.CreateSkillGroup(context.Background(), SkillGroupInput{TenantID: 2, Name: "q-other", Code: "q-other", Strategy: "round_robin", Status: 1})
	if err != nil {
		t.Fatal(err)
	}

	queued := func(tenantID, sgID uint64) uint64 {
		t.Helper()
		v, err := s.UpsertVisitor(tenantID, fmt.Sprintf("q-guest-%d", retDBSeq.Add(1)), "")
		if err != nil {
			t.Fatal(err)
		}
		c, err := s.CreateConversation(tenantID, 1, v.ID, "h5", "{}", &sgID, false)
		if err != nil {
			t.Fatal(err)
		}
		if err := s.db.Model(c).Update("status", "queued").Error; err != nil {
			t.Fatal(err)
		}
		return c.ID
	}
	a := queued(1, g1.ID)
	b := queued(1, g2.ID)
	foreign := queued(2, other.ID)

	got, err := s.ListQueuedForSkillGroups(1, []uint64{g1.ID, g2.ID}, 50)
	if err != nil {
		t.Fatal(err)
	}
	ids := map[uint64]bool{}
	for i := range got {
		ids[got[i].ID] = true
	}
	if !ids[a] || !ids[b] {
		t.Fatalf("missing queued conversations: got %v, want %d and %d", ids, a, b)
	}
	if ids[foreign] {
		t.Fatal("cross-tenant conversation leaked into queued batch")
	}
	if len(got) != 2 {
		t.Fatalf("got %d conversations, want 2", len(got))
	}
	// 与逐组查询的并集一致
	perGroup := map[uint64]bool{}
	for _, sgID := range []uint64{g1.ID, g2.ID} {
		list, err := s.ListAgentConversations(1, 0, "queue", sgID, 20)
		if err != nil {
			t.Fatal(err)
		}
		for i := range list {
			perGroup[list[i].ID] = true
		}
	}
	if len(perGroup) != len(ids) {
		t.Fatalf("batch = %v, per-group union = %v", ids, perGroup)
	}
	for id := range perGroup {
		if !ids[id] {
			t.Errorf("conversation %d in per-group union but not in batch", id)
		}
	}
	// 空组列表不查库
	if got, err := s.ListQueuedForSkillGroups(1, nil, 50); err != nil || len(got) != 0 {
		t.Fatalf("empty group list: got %v err %v", got, err)
	}
}

// 无上限 Find 收口后仍要返回全部正常规模数据（limit 不误伤）。
func TestAdminListsRespectLimit(t *testing.T) {
	s := newTestStore(t)
	for i := 0; i < 5; i++ {
		name := fmt.Sprintf("lim-%d", i)
		if _, err := s.CreateSkillGroup(context.Background(), SkillGroupInput{
			TenantID: 1, Name: name, Code: name, Strategy: "round_robin", Status: 1,
		}); err != nil {
			t.Fatal(err)
		}
	}
	groups, err := s.ListSkillGroups(1)
	if err != nil {
		t.Fatal(err)
	}
	if len(groups) < 5 || len(groups) > adminListLimit {
		t.Fatalf("ListSkillGroups returned %d groups", len(groups))
	}
	var total int64
	if err := s.db.Model(&model.SkillGroup{}).Where("tenant_id = ?", 1).Count(&total).Error; err != nil {
		t.Fatal(err)
	}
	if int64(len(groups)) != total {
		t.Fatalf("ListSkillGroups returned %d, tenant has %d", len(groups), total)
	}
}
