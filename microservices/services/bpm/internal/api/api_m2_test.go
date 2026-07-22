package api

// M2 HTTP 用例：抄送箱（列表 / unread_only / 标已读幂等 / 越权拒绝）与
// 转办 / 退回 / 重提端点全链路（含 GetTask 动作列表演变）。

import (
	"encoding/json"
	"fmt"
	"net/http"
	"testing"
	"time"
)

// 链：start → 一级(用户2) → 二级(用户3, allowBackPrev) → 抄送(用户9) → 结束。
func TestHTTPM2CcAndActions(t *testing.T) {
	r, _ := newTestServer(t, nil)

	tree := map[string]any{
		"version": 1,
		"start": map[string]any{
			"id": "n-start", "name": "发起", "type": "start",
			"next": map[string]any{
				"id": "n-a1", "name": "一级", "type": "approval",
				"multiMode": "OR",
				"assignee":  map[string]any{"type": "users", "userIds": []uint64{2}},
				"next": map[string]any{
					"id": "n-a2", "name": "二级", "type": "approval",
					"multiMode": "OR", "allowBackPrev": true,
					"assignee": map[string]any{"type": "users", "userIds": []uint64{3}},
					"next": map[string]any{
						"id": "n-cc", "name": "抄送财务", "type": "cc",
						"targets": map[string]any{"type": "users", "userIds": []uint64{9}},
					},
				},
			},
		},
	}
	w, env := call(t, r, "POST", "/api/v1/bpm/definitions", map[string]any{
		"key": "m2_flow", "name": "M2 流程", "biz_type": "demo", "node_tree": tree,
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
	w, env = call(t, r, "POST", "/api/v1/bpm/internal/instances", map[string]any{
		"definition_key": "m2_flow", "title": "M2 测试单",
		"biz_type": "demo", "biz_id": "77", "initiator_id": 1,
	}, internalHdr())
	if w.Code != 200 {
		t.Fatalf("start: %d %s", w.Code, w.Body.String())
	}
	var started struct {
		InstanceID uint64 `json:"instance_id"`
	}
	mustDecode(t, env.Data, &started)

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

	// 一级任务动作：approve/reject/transfer/return_start（无上一审批节点）
	task1 := todoOf("2")
	_, env = call(t, r, "GET", fmt.Sprintf("/api/v1/bpm/tasks/%d", task1), nil, asUser("2"))
	var detail struct {
		Actions []string `json:"actions"`
	}
	mustDecode(t, env.Data, &detail)
	if contains(detail.Actions, "return_prev") || !contains(detail.Actions, "transfer") {
		t.Fatalf("一级动作: %+v", detail.Actions)
	}
	// 转办 2 → 5，新人可同意
	if w, _ := call(t, r, "POST", fmt.Sprintf("/api/v1/bpm/tasks/%d/transfer", task1),
		map[string]any{"target_user_id": 5, "comment": "转办"}, asUser("2")); w.Code != 200 {
		t.Fatalf("transfer: %d %s", w.Code, w.Body.String())
	}
	if w, _ := call(t, r, "POST", fmt.Sprintf("/api/v1/bpm/tasks/%d/approve", task1),
		map[string]any{}, asUser("5")); w.Code != 200 {
		t.Fatalf("approve after transfer: %d %s", w.Code, w.Body.String())
	}

	// 二级任务动作：含 return_prev（allowBackPrev 且存在上一审批节点）
	task2 := todoOf("3")
	_, env = call(t, r, "GET", fmt.Sprintf("/api/v1/bpm/tasks/%d", task2), nil, asUser("3"))
	mustDecode(t, env.Data, &detail)
	if !contains(detail.Actions, "return_prev") {
		t.Fatalf("二级动作应含 return_prev: %+v", detail.Actions)
	}
	// 退回发起人 → 发起人重提任务动作 = resubmit
	if w, _ := call(t, r, "POST", fmt.Sprintf("/api/v1/bpm/tasks/%d/return", task2),
		map[string]any{"to": "start", "comment": "退回补材料"}, asUser("3")); w.Code != 200 {
		t.Fatalf("return: %d %s", w.Code, w.Body.String())
	}
	resubmitTask := todoOf("1")
	_, env = call(t, r, "GET", fmt.Sprintf("/api/v1/bpm/tasks/%d", resubmitTask), nil, asUser("1"))
	mustDecode(t, env.Data, &detail)
	if len(detail.Actions) != 1 || detail.Actions[0] != "resubmit" {
		t.Fatalf("重提任务动作: %+v", detail.Actions)
	}
	// 重提（带新快照）→ 一级重新出任务 → 走完到抄送
	if w, _ := call(t, r, "POST", fmt.Sprintf("/api/v1/bpm/instances/%d/resubmit", started.InstanceID),
		map[string]any{"form_snapshot": map[string]any{"amount_cents": 999}}, asUser("1")); w.Code != 200 {
		t.Fatalf("resubmit: %d %s", w.Code, w.Body.String())
	}
	if w, _ := call(t, r, "POST", fmt.Sprintf("/api/v1/bpm/tasks/%d/approve", todoOf("2")),
		map[string]any{}, asUser("2")); w.Code != 200 {
		t.Fatalf("round2 lvl1: %d", w.Code)
	}
	if w, _ := call(t, r, "POST", fmt.Sprintf("/api/v1/bpm/tasks/%d/approve", todoOf("3")),
		map[string]any{}, asUser("3")); w.Code != 200 {
		t.Fatalf("round2 lvl2: %d", w.Code)
	}

	// 抄送箱：用户 9 一条未读（行含契约字段）
	_, env = call(t, r, "GET", "/api/v1/bpm/cc/my?unread_only=true", nil, asUser("9"))
	var cc struct {
		List []struct {
			ID            uint64     `json:"id"`
			InstanceID    uint64     `json:"instance_id"`
			InstanceTitle string     `json:"instance_title"`
			NodeName      string     `json:"node_name"`
			InitiatorID   uint64     `json:"initiator_id"`
			ReadAt        *time.Time `json:"read_at"`
		} `json:"list"`
		Total int64 `json:"total"`
	}
	mustDecode(t, env.Data, &cc)
	if cc.Total != 1 || cc.List[0].InstanceTitle != "M2 测试单" ||
		cc.List[0].NodeName != "抄送财务" || cc.List[0].InitiatorID != 1 ||
		cc.List[0].ReadAt != nil {
		t.Fatalf("抄送箱: %+v", cc)
	}
	ccID := cc.List[0].ID
	// 他人标已读 → 403
	if w, _ := call(t, r, "POST", fmt.Sprintf("/api/v1/bpm/cc/%d/read", ccID), nil, asUser("2")); w.Code != http.StatusForbidden {
		t.Fatalf("越权标已读应 403, got %d", w.Code)
	}
	// 本人标已读，幂等
	for i := 0; i < 2; i++ {
		if w, _ := call(t, r, "POST", fmt.Sprintf("/api/v1/bpm/cc/%d/read", ccID), nil, asUser("9")); w.Code != 200 {
			t.Fatalf("标已读第 %d 次: %d %s", i+1, w.Code, w.Body.String())
		}
	}
	// unread_only 过滤后为空；全量仍可见且 read_at 非空
	_, env = call(t, r, "GET", "/api/v1/bpm/cc/my?unread_only=true", nil, asUser("9"))
	mustDecode(t, env.Data, &cc)
	if cc.Total != 0 {
		t.Fatalf("已读后 unread_only 应为空: %+v", cc)
	}
	_, env = call(t, r, "GET", "/api/v1/bpm/cc/my", nil, asUser("9"))
	mustDecode(t, env.Data, &cc)
	if cc.Total != 1 || cc.List[0].ReadAt == nil {
		t.Fatalf("全量抄送: %+v", cc)
	}
}

func contains(list []string, s string) bool {
	for _, v := range list {
		if v == s {
			return true
		}
	}
	return false
}

func mustDecode(t *testing.T, raw []byte, v any) {
	t.Helper()
	if err := json.Unmarshal(raw, v); err != nil {
		t.Fatalf("decode: %v (%s)", err, raw)
	}
}
