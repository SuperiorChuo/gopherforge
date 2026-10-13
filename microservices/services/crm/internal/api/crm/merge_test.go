package api

// 客户合并去重测试：同号码检测成组、合并迁移子实体与留痕、越权合并 403。

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

func TestDupDetectAndMerge(t *testing.T) {
	r, _ := newTestRouterWithServer(t)

	// 两个同号码客户（一个带格式差异）+ 一个无关客户
	_, e1 := call(t, r, http.MethodPost, "/api/v1/crm/customers", map[string]any{
		"name": "宏图集团", "phone": "+86 138-0000-1111", "industry": "制造",
	}, "")
	idA := jsonField(t, e1.Data, "id")
	_, e2 := call(t, r, http.MethodPost, "/api/v1/crm/customers", map[string]any{
		"name": "宏图集团有限公司", "phone": "13800001111", "address": "上海",
	}, "")
	idB := jsonField(t, e2.Data, "id")
	call(t, r, http.MethodPost, "/api/v1/crm/customers", map[string]any{"name": "无关客户", "phone": "13900002222"}, "")

	// B 名下挂联系人 + 商机 + 跟进
	call(t, r, http.MethodPost, "/api/v1/crm/customers/"+idB+"/contacts", map[string]any{"name": "李四", "phone": "13800001111"}, "")
	call(t, r, http.MethodPost, "/api/v1/crm/opportunities", map[string]any{"customer_id": jsonNum(t, idB), "name": "B的商机", "amount_cents": 500000}, "")
	call(t, r, http.MethodPost, "/api/v1/crm/followups", map[string]any{"target_type": "customer", "target_id": jsonNum(t, idB), "content": "B的跟进"}, "")

	// 检测：同号码应成组
	w, env := call(t, r, http.MethodGet, "/api/v1/crm/dup-groups", nil, "")
	if w.Code != http.StatusOK {
		t.Fatalf("dup-groups: %d", w.Code)
	}
	var dg struct {
		Groups []struct {
			Kind      string `json:"kind"`
			Customers []struct {
				ID uint64 `json:"id"`
			} `json:"customers"`
		} `json:"groups"`
		Total int `json:"total"`
	}
	if err := json.Unmarshal(env.Data, &dg); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if dg.Total < 1 {
		t.Fatalf("应检出至少一组重复，got %d", dg.Total)
	}
	foundPhoneGroup := false
	for _, g := range dg.Groups {
		if g.Kind == "phone" && len(g.Customers) == 2 {
			foundPhoneGroup = true
		}
	}
	if !foundPhoneGroup {
		t.Fatalf("应有号码重复组: %s", string(env.Data))
	}

	// 合并 B → A
	w, env = call(t, r, http.MethodPost, "/api/v1/crm/customers/merge", map[string]any{
		"primary_id": jsonNum(t, idA), "source_ids": []any{jsonNum(t, idB)},
	}, "")
	if w.Code != http.StatusOK {
		t.Fatalf("merge: %d %s", w.Code, string(env.Data))
	}

	// 副客户没了，主客户详情继承了联系人/商机/跟进/地址补齐
	if w, _ := call(t, r, http.MethodGet, "/api/v1/crm/customers/"+idB, nil, ""); w.Code != http.StatusNotFound {
		t.Fatalf("副客户应已删除，got %d", w.Code)
	}
	_, ed := call(t, r, http.MethodGet, "/api/v1/crm/customers/"+idA, nil, "")
	var detail struct {
		Customer struct {
			Address string `json:"address"`
		} `json:"customer"`
		Contacts      []any `json:"contacts"`
		Opportunities []any `json:"opportunities"`
		FollowUps     []any `json:"follow_ups"`
	}
	if err := json.Unmarshal(ed.Data, &detail); err != nil {
		t.Fatalf("detail: %v", err)
	}
	if len(detail.Contacts) != 1 || len(detail.Opportunities) != 1 || len(detail.FollowUps) != 1 {
		t.Fatalf("子实体应迁到主客户: contacts=%d opps=%d follows=%d",
			len(detail.Contacts), len(detail.Opportunities), len(detail.FollowUps))
	}
	if detail.Customer.Address != "上海" {
		t.Fatalf("主客户空地址应被补齐，got %q", detail.Customer.Address)
	}

	// 合并后同组应消失
	_, env = call(t, r, http.MethodGet, "/api/v1/crm/dup-groups", nil, "")
	var dg2 struct {
		Total int `json:"total"`
	}
	_ = json.Unmarshal(env.Data, &dg2)
	if dg2.Total != 0 {
		t.Fatalf("合并后不应再有重复组，got %d", dg2.Total)
	}
}

func TestMergeScopeForbidden(t *testing.T) {
	r, srv := newTestRouterWithServer(t)
	// all 档用户 9 建两个同号码客户：一个归自己，一个归 7
	_, e1 := call(t, r, http.MethodPost, "/api/v1/crm/customers", map[string]any{"name": "甲", "phone": "13811112222", "owner_user_id": 5}, "")
	_, e2 := call(t, r, http.MethodPost, "/api/v1/crm/customers", map[string]any{"name": "乙", "phone": "138 1111 2222", "owner_user_id": 7}, "")
	idA, idB := jsonField(t, e1.Data, "id"), jsonField(t, e2.Data, "id")

	seedScope(t, srv, 5, "self", 0)
	// self 档用户 5 试图把 7 的客户合并进自己 → 403
	w, _ := callAs(t, r, http.MethodPost, "/api/v1/crm/customers/merge", map[string]any{
		"primary_id": jsonNum(t, idA), "source_ids": []any{jsonNum(t, idB)},
	}, "5")
	if w.Code != http.StatusForbidden {
		t.Fatalf("越权合并应 403，got %d", w.Code)
	}
	// dup-groups 对 self 档也不应暴露含他人客户的组
	_, env := callAs(t, r, http.MethodGet, "/api/v1/crm/dup-groups", nil, "5")
	var dg struct {
		Total int `json:"total"`
	}
	_ = json.Unmarshal(env.Data, &dg)
	if dg.Total != 0 {
		t.Fatalf("self 档不应看到含他人客户的重复组，got %d", dg.Total)
	}
}

func TestMergeRejectsTooManySourceIDs(t *testing.T) {
	r := newTestRouter(t)
	sourceIDs := make([]uint64, 101)
	for i := range sourceIDs {
		sourceIDs[i] = uint64(i + 2)
	}
	w, env := call(t, r, http.MethodPost, "/api/v1/crm/customers/merge", map[string]any{
		"primary_id": 1, "source_ids": sourceIDs,
	}, "")
	if w.Code != http.StatusBadRequest || !strings.Contains(env.Message, "100") {
		t.Fatalf("oversized merge = %d %q, want 400 with limit", w.Code, env.Message)
	}
}

func TestMergeDeduplicatesSourceIDs(t *testing.T) {
	r := newTestRouter(t)
	_, primary := call(t, r, http.MethodPost, "/api/v1/crm/customers", map[string]any{"name": "主客户"}, "")
	_, source := call(t, r, http.MethodPost, "/api/v1/crm/customers", map[string]any{"name": "副客户"}, "")
	primaryID, sourceID := jsonField(t, primary.Data, "id"), jsonField(t, source.Data, "id")

	w, env := call(t, r, http.MethodPost, "/api/v1/crm/customers/merge", map[string]any{
		"primary_id": jsonNum(t, primaryID),
		"source_ids": []any{jsonNum(t, sourceID), jsonNum(t, sourceID)},
	}, "")
	if w.Code != http.StatusOK {
		t.Fatalf("deduplicated merge = %d %q", w.Code, env.Message)
	}
	if !strings.Contains(string(env.Data), `"merged":1`) {
		t.Fatalf("merged count must use unique source ids: %s", env.Data)
	}
}

func jsonNum(t *testing.T, s string) float64 {
	t.Helper()
	var v float64
	if err := json.Unmarshal([]byte(s), &v); err != nil {
		t.Fatalf("num %q: %v", s, err)
	}
	return v
}
