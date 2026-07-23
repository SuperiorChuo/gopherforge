package api

// M3+ HTTP 用例：加签 / 委派端点全链路与 GetTask 动作列表演变
// （常规节点含 add_sign/delegate、SEQ 节点不含 add_sign、委派中仅
// delegate_resolve、越权 400）。

import (
	"fmt"
	"testing"
)

// 链：start → 或签(用户2) → 结束。委派 2→5 → 5 办结 → 回 2 → 加签 6 → 2 同意。
func TestHTTPAddSignDelegate(t *testing.T) {
	r, _ := newTestServer(t, nil)

	tree := map[string]any{
		"version": 1,
		"start": map[string]any{
			"id": "n-start", "name": "发起", "type": "start",
			"next": map[string]any{
				"id": "n-a1", "name": "审批", "type": "approval",
				"multiMode": "OR",
				"assignee":  map[string]any{"type": "users", "userIds": []uint64{2}},
			},
		},
	}
	w, env := call(t, r, "POST", "/api/v1/bpm/definitions", map[string]any{
		"key": "asd_flow", "name": "加签委派流程", "biz_type": "demo", "node_tree": tree,
	}, asUser("1"))
	if w.Code != 200 {
		t.Fatalf("create def: %d %s", w.Code, w.Body.String())
	}
	var def struct {
		ID uint64 `json:"id"`
	}
	mustDecode(t, env.Data, &def)
	if w, _ := call(t, r, "POST", fmt.Sprintf("/api/v1/bpm/definitions/%d/publish", def.ID), nil, asUser("1")); w.Code != 200 {
		t.Fatalf("publish: %d %s", w.Code, w.Body.String())
	}
	if w, _ = call(t, r, "POST", "/api/v1/bpm/internal/instances", map[string]any{
		"definition_key": "asd_flow", "title": "加签委派测试单",
		"biz_type": "demo", "biz_id": "88", "initiator_id": 1,
	}, internalHdr()); w.Code != 200 {
		t.Fatalf("start: %d %s", w.Code, w.Body.String())
	}

	todoOf := func(user string) uint64 {
		t.Helper()
		_, env := call(t, r, "GET", "/api/v1/bpm/tasks/todo", nil, asUser(user))
		var todo struct {
			List []struct {
				ID uint64 `json:"id"`
			} `json:"list"`
		}
		mustDecode(t, env.Data, &todo)
		if len(todo.List) == 0 {
			t.Fatalf("用户 %s 无待办", user)
		}
		return todo.List[0].ID
	}
	actionsOf := func(taskID uint64, user string) []string {
		t.Helper()
		_, env := call(t, r, "GET", fmt.Sprintf("/api/v1/bpm/tasks/%d", taskID), nil, asUser(user))
		var detail struct {
			Actions []string `json:"actions"`
		}
		mustDecode(t, env.Data, &detail)
		return detail.Actions
	}

	// 常规审批任务动作含 add_sign / delegate
	task := todoOf("2")
	acts := actionsOf(task, "2")
	if !contains(acts, "add_sign") || !contains(acts, "delegate") {
		t.Fatalf("常规动作应含 add_sign/delegate: %+v", acts)
	}

	// 委派 2 → 5：受托人动作仅 delegate_resolve，且不能直接同意
	if w, _ := call(t, r, "POST", fmt.Sprintf("/api/v1/bpm/tasks/%d/delegate", task),
		map[string]any{"target_user_id": 5, "comment": "帮我核实"}, asUser("2")); w.Code != 200 {
		t.Fatalf("delegate: %d %s", w.Code, w.Body.String())
	}
	if got := actionsOf(todoOf("5"), "5"); len(got) != 1 || got[0] != "delegate_resolve" {
		t.Fatalf("委派中受托人动作: %+v", got)
	}
	if w, _ := call(t, r, "POST", fmt.Sprintf("/api/v1/bpm/tasks/%d/approve", task),
		map[string]any{}, asUser("5")); w.Code != 400 {
		t.Fatalf("委派中 approve 应 400, got %d", w.Code)
	}
	// 空意见办结 400；带意见办结回到 2
	if w, _ := call(t, r, "POST", fmt.Sprintf("/api/v1/bpm/tasks/%d/delegate/resolve", task),
		map[string]any{"comment": ""}, asUser("5")); w.Code != 400 {
		t.Fatalf("空意见办结应 400, got %d", w.Code)
	}
	if w, _ := call(t, r, "POST", fmt.Sprintf("/api/v1/bpm/tasks/%d/delegate/resolve", task),
		map[string]any{"comment": "已核实"}, asUser("5")); w.Code != 200 {
		t.Fatalf("resolve: %d %s", w.Code, w.Body.String())
	}
	if got := actionsOf(todoOf("2"), "2"); !contains(got, "approve") || !contains(got, "add_sign") {
		t.Fatalf("办结后原人动作应恢复: %+v", got)
	}

	// 加签 6：新人出待办；非 assignee 加签 400
	if w, _ := call(t, r, "POST", fmt.Sprintf("/api/v1/bpm/tasks/%d/add-sign", task),
		map[string]any{"user_ids": []uint64{6}, "comment": "拉个人"}, asUser("9")); w.Code != 400 {
		t.Fatalf("非处理人加签应 400, got %d", w.Code)
	}
	if w, _ := call(t, r, "POST", fmt.Sprintf("/api/v1/bpm/tasks/%d/add-sign", task),
		map[string]any{"user_ids": []uint64{6}, "comment": "拉个人"}, asUser("2")); w.Code != 200 {
		t.Fatalf("add-sign: %d %s", w.Code, w.Body.String())
	}
	addedTask := todoOf("6")
	if addedTask == 0 {
		t.Fatal("加签人应有待办")
	}
	// 或签：原人同意即通过
	w, env = call(t, r, "POST", fmt.Sprintf("/api/v1/bpm/tasks/%d/approve", task),
		map[string]any{"comment": "同意"}, asUser("2"))
	if w.Code != 200 {
		t.Fatalf("approve: %d %s", w.Code, w.Body.String())
	}
	var res struct {
		InstanceStatus string `json:"instance_status"`
	}
	mustDecode(t, env.Data, &res)
	if res.InstanceStatus != "approved" {
		t.Fatalf("或签同意后实例应 approved: %s", res.InstanceStatus)
	}
}

// SEQ 依次节点：动作列表不含 add_sign，端点也拒绝。
func TestHTTPAddSignSeqExcluded(t *testing.T) {
	r, _ := newTestServer(t, nil)

	tree := map[string]any{
		"version": 1,
		"start": map[string]any{
			"id": "n-start", "name": "发起", "type": "start",
			"next": map[string]any{
				"id": "n-a1", "name": "依次审批", "type": "approval",
				"multiMode": "SEQ",
				"assignee":  map[string]any{"type": "users", "userIds": []uint64{2, 3}},
			},
		},
	}
	w, env := call(t, r, "POST", "/api/v1/bpm/definitions", map[string]any{
		"key": "asd_seq", "name": "依次流程", "biz_type": "demo", "node_tree": tree,
	}, asUser("1"))
	if w.Code != 200 {
		t.Fatalf("create def: %d %s", w.Code, w.Body.String())
	}
	var def struct {
		ID uint64 `json:"id"`
	}
	mustDecode(t, env.Data, &def)
	if w, _ := call(t, r, "POST", fmt.Sprintf("/api/v1/bpm/definitions/%d/publish", def.ID), nil, asUser("1")); w.Code != 200 {
		t.Fatalf("publish: %d %s", w.Code, w.Body.String())
	}
	if w, _ := call(t, r, "POST", "/api/v1/bpm/internal/instances", map[string]any{
		"definition_key": "asd_seq", "title": "依次测试单",
		"biz_type": "demo", "biz_id": "89", "initiator_id": 1,
	}, internalHdr()); w.Code != 200 {
		t.Fatalf("start: %d %s", w.Code, w.Body.String())
	}

	_, env = call(t, r, "GET", "/api/v1/bpm/tasks/todo", nil, asUser("2"))
	var todo struct {
		List []struct {
			ID uint64 `json:"id"`
		} `json:"list"`
	}
	mustDecode(t, env.Data, &todo)
	if len(todo.List) != 1 {
		t.Fatalf("SEQ 首位待办: %+v", todo)
	}
	taskID := todo.List[0].ID
	_, env = call(t, r, "GET", fmt.Sprintf("/api/v1/bpm/tasks/%d", taskID), nil, asUser("2"))
	var detail struct {
		Actions []string `json:"actions"`
	}
	mustDecode(t, env.Data, &detail)
	if contains(detail.Actions, "add_sign") || !contains(detail.Actions, "delegate") {
		t.Fatalf("SEQ 动作不应含 add_sign（委派仍可用）: %+v", detail.Actions)
	}
	if w, _ := call(t, r, "POST", fmt.Sprintf("/api/v1/bpm/tasks/%d/add-sign", taskID),
		map[string]any{"user_ids": []uint64{5}}, asUser("2")); w.Code != 400 {
		t.Fatalf("SEQ 加签端点应 400, got %d", w.Code)
	}
}
