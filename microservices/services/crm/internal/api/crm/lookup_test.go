package api

// 来电弹屏号码识别：客户 > 联系人 > 线索 > none，含 +86 前缀归一。

import (
	"fmt"
	"net/http"
	"testing"
)

func TestPhoneLookup(t *testing.T) {
	r := newTestRouter(t)

	// 客户直连号码
	_, env := call(t, r, http.MethodPost, "/api/v1/crm/customers",
		map[string]any{"name": "宏图集团", "phone": "13511112222"}, "")
	var cu struct {
		ID uint64 `json:"id"`
	}
	decode(t, env.Data, &cu)

	// 联系人号码（挂在宏图集团下）
	_, _ = call(t, r, http.MethodPost, fmt.Sprintf("/api/v1/crm/customers/%d/contacts", cu.ID),
		map[string]any{"name": "刘工", "phone": "13733334444"}, "")

	// 未转化线索号码
	_, _ = call(t, r, http.MethodPost, "/api/v1/crm/leads",
		map[string]any{"name": "陌生询价", "phone": "13955556666", "source": "call"}, "")

	cases := []struct {
		phone   string
		matched string
		desc    string
	}{
		{"13511112222", "customer", "客户号码直中"},
		{"+8613511112222", "customer", "+86 前缀尾号归一"},
		{"13733334444", "customer", "联系人号码带出客户"},
		{"13955556666", "lead", "线索号码"},
		{"13800000000", "none", "陌生号码"},
	}
	for _, tc := range cases {
		w, env := call(t, r, http.MethodGet, "/api/v1/crm/lookup?phone="+tc.phone, nil, "")
		if w.Code != http.StatusOK {
			t.Fatalf("%s: status %d", tc.desc, w.Code)
		}
		var res struct {
			Matched  string `json:"matched"`
			Customer *struct {
				Name string `json:"name"`
			} `json:"customer"`
			Contact *struct {
				Name string `json:"name"`
			} `json:"contact"`
		}
		decode(t, env.Data, &res)
		if res.Matched != tc.matched {
			t.Fatalf("%s: matched=%s want %s", tc.desc, res.Matched, tc.matched)
		}
		if tc.matched == "customer" && res.Customer.Name != "宏图集团" {
			t.Fatalf("%s: customer=%v", tc.desc, res.Customer)
		}
	}

	// 空号码 400；跨租户不可见
	w, _ := call(t, r, http.MethodGet, "/api/v1/crm/lookup?phone=", nil, "")
	if w.Code != http.StatusBadRequest {
		t.Fatalf("empty phone: %d", w.Code)
	}
	w, env = call(t, r, http.MethodGet, "/api/v1/crm/lookup?phone=13511112222", nil, "2")
	var cross struct {
		Matched string `json:"matched"`
	}
	decode(t, env.Data, &cross)
	if cross.Matched != "none" {
		t.Fatalf("cross-tenant lookup: %s", cross.Matched)
	}
}
