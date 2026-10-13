package api

// 数据权限硬隔离测试：self 档只见自己+公海；dept 档见部门树成员+公海；
// all 档不拦。写路径（编辑/详情）越权 403。

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/go-admin-kit/services/crm/internal/store/crm"
)

// callAs 以指定用户身份请求（X-Auth-User-ID 可换）。
func callAs(t *testing.T, r *gin.Engine, method, path string, body any, userID string) (*httptest.ResponseRecorder, envelope) {
	t.Helper()
	var buf bytes.Buffer
	if body != nil {
		if err := json.NewEncoder(&buf).Encode(body); err != nil {
			t.Fatal(err)
		}
	}
	req := httptest.NewRequest(method, path, &buf)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Auth-User-ID", userID)
	req.Header.Set("X-Auth-Username", "u"+userID)
	req.Header.Set("X-Auth-Permissions", "*")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	var env envelope
	_ = json.Unmarshal(w.Body.Bytes(), &env)
	return w, env
}

func jsonField(t *testing.T, data json.RawMessage, key string) string {
	t.Helper()
	var m map[string]any
	if err := json.Unmarshal(data, &m); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	switch v := m[key].(type) {
	case float64:
		return strconv.FormatInt(int64(v), 10)
	case string:
		return v
	}
	t.Fatalf("field %s missing in %s", key, string(data))
	return ""
}

func listNames(t *testing.T, data json.RawMessage) map[string]bool {
	t.Helper()
	var body struct {
		List []struct {
			Name string `json:"name"`
		} `json:"list"`
	}
	if err := json.Unmarshal(data, &body); err != nil {
		t.Fatalf("unmarshal list: %v", err)
	}
	out := map[string]bool{}
	for _, it := range body.List {
		out[it.Name] = true
	}
	return out
}

func firstListID(t *testing.T, data json.RawMessage) string {
	t.Helper()
	var body struct {
		List []struct {
			ID uint64 `json:"id"`
		} `json:"list"`
	}
	if err := json.Unmarshal(data, &body); err != nil || len(body.List) == 0 {
		t.Fatalf("empty list: %s", string(data))
	}
	return strconv.FormatUint(body.List[0].ID, 10)
}

// seedScope 配置一个用户的角色档位与部门归属。
func seedScope(t *testing.T, srv *Server, userID uint64, scope string, deptID uint64) {
	t.Helper()
	db := srv.Store.DB()
	var roleID uint64
	db.Raw(`SELECT COALESCE(MAX(id),0)+1 FROM roles`).Scan(&roleID)
	if err := db.Exec(`INSERT INTO roles (id, data_scope) VALUES (?, ?)`, roleID, scope).Error; err != nil {
		t.Fatalf("seed role: %v", err)
	}
	if err := db.Exec(`INSERT INTO user_roles (user_id, role_id) VALUES (?, ?)`, userID, roleID).Error; err != nil {
		t.Fatalf("seed user_role: %v", err)
	}
	if deptID > 0 {
		if err := db.Exec(`INSERT OR REPLACE INTO users (id, department_id, status) VALUES (?, ?, 1)`, userID, deptID).Error; err != nil {
			t.Fatalf("seed user: %v", err)
		}
	}
	store.InvalidateScopeCache()
}

func TestScopeSelfIsolation(t *testing.T) {
	r, srv := newTestRouterWithServer(t)
	// 用户 9=all（infra 默认）建 3 个客户：自己的、别人的(7)、公海
	for _, body := range []map[string]any{
		{"name": "我的客户", "owner_user_id": 9},
		{"name": "别人的客户", "owner_user_id": 7},
	} {
		if w, _ := call(t, r, http.MethodPost, "/api/v1/crm/customers", body, ""); w.Code != http.StatusOK {
			t.Fatalf("create: %d", w.Code)
		}
	}
	// 公海客户：建了再释放
	_, env := call(t, r, http.MethodPost, "/api/v1/crm/customers", map[string]any{"name": "公海客户", "owner_user_id": 9}, "")
	poolID := jsonField(t, env.Data, "id")
	call(t, r, http.MethodPost, "/api/v1/crm/customers/"+poolID+"/release", map[string]any{}, "")

	// 用户 5 = self 档
	seedScope(t, srv, 5, "self", 0)
	w, env := callAs(t, r, http.MethodGet, "/api/v1/crm/customers?page=1&page_size=50", nil, "5")
	if w.Code != http.StatusOK {
		t.Fatalf("list as self: %d", w.Code)
	}
	names := listNames(t, env.Data)
	if names["别人的客户"] || names["我的客户"] {
		t.Fatalf("self 档不应看到他人客户: %v", names)
	}
	if !names["公海客户"] {
		t.Fatalf("self 档应看到公海: %v", names)
	}

	// 越权详情/编辑 → 403
	var otherID string
	{
		_, envAll := call(t, r, http.MethodGet, "/api/v1/crm/customers?keyword=别人的客户", nil, "")
		otherID = firstListID(t, envAll.Data)
	}
	if w, _ := callAs(t, r, http.MethodGet, "/api/v1/crm/customers/"+otherID, nil, "5"); w.Code != http.StatusForbidden {
		t.Fatalf("越权详情应 403，got %d", w.Code)
	}
	if w, _ := callAs(t, r, http.MethodPut, "/api/v1/crm/customers/"+otherID, map[string]any{"name": "改名"}, "5"); w.Code != http.StatusForbidden {
		t.Fatalf("越权编辑应 403，got %d", w.Code)
	}
}

func TestScopeDeptIsolation(t *testing.T) {
	r, srv := newTestRouterWithServer(t)
	db := srv.Store.DB()
	// 部门树：10（父）← 11（子）；成员：经理 20@10、组员 21@11、外人 30@99
	for _, ddl := range []string{
		`INSERT INTO departments (id, parent_id, status) VALUES (10, 0, 1), (11, 10, 1), (99, 0, 1)`,
		`INSERT OR REPLACE INTO users (id, department_id, status) VALUES (20, 10, 1), (21, 11, 1), (30, 99, 1)`,
	} {
		if err := db.Exec(ddl).Error; err != nil {
			t.Fatalf("ddl: %v", err)
		}
	}
	seedScope(t, srv, 20, "department_and_children", 10)

	// all 档用户 9 建：组员的客户、外人的客户
	call(t, r, http.MethodPost, "/api/v1/crm/customers", map[string]any{"name": "组员客户", "owner_user_id": 21}, "")
	call(t, r, http.MethodPost, "/api/v1/crm/customers", map[string]any{"name": "外部门客户", "owner_user_id": 30}, "")

	w, env := callAs(t, r, http.MethodGet, "/api/v1/crm/customers?page=1&page_size=50", nil, "20")
	if w.Code != http.StatusOK {
		t.Fatalf("list as dept: %d", w.Code)
	}
	names := listNames(t, env.Data)
	if !names["组员客户"] {
		t.Fatalf("dept 档应看到子部门成员客户: %v", names)
	}
	if names["外部门客户"] {
		t.Fatalf("dept 档不应看到外部门客户: %v", names)
	}
}

// self 档商机/合同硬隔离：列表只见自己的；越权详情/编辑/推进 403。
func TestScopeSelfOpportunityContractIsolation(t *testing.T) {
	r, srv := newTestRouterWithServer(t)

	// all 档用户 9 建两个客户 + 各挂商机/合同（owner 分别为 9 和 7）
	mkCustomer := func(name string, owner uint64) uint64 {
		_, env := call(t, r, http.MethodPost, "/api/v1/crm/customers",
			map[string]any{"name": name, "owner_user_id": owner}, "")
		var cu struct {
			ID uint64 `json:"id"`
		}
		decode(t, env.Data, &cu)
		return cu.ID
	}
	mine, others := mkCustomer("我的客户", 5), mkCustomer("别人的客户", 7)

	mkOpp := func(name string, customerID, owner uint64) string {
		w, env := call(t, r, http.MethodPost, "/api/v1/crm/opportunities",
			map[string]any{"customer_id": customerID, "name": name, "owner_user_id": owner}, "")
		if w.Code != http.StatusOK {
			t.Fatalf("create opp: %d %s", w.Code, w.Body.String())
		}
		return jsonField(t, env.Data, "id")
	}
	mkOpp("我的商机", mine, 5)
	otherOppID := mkOpp("别人的商机", others, 7)

	// 合同 owner 由创建人决定（u9），再直改 DB 把"别人的合同"归 7
	mkContract := func(no, title string, customerID uint64) string {
		w, env := call(t, r, http.MethodPost, "/api/v1/crm/contracts",
			map[string]any{"no": no, "title": title, "customer_id": customerID}, "")
		if w.Code != http.StatusOK {
			t.Fatalf("create contract: %d %s", w.Code, w.Body.String())
		}
		return jsonField(t, env.Data, "id")
	}
	myCtID := mkContract("HT-S-001", "我的合同", mine)
	otherCtID := mkContract("HT-S-002", "别人的合同", others)
	srv.Store.DB().Exec("UPDATE crm_contracts SET owner_user_id = 5 WHERE id = ?", myCtID)
	srv.Store.DB().Exec("UPDATE crm_contracts SET owner_user_id = 7 WHERE id = ?", otherCtID)

	// 用户 5 = self 档
	seedScope(t, srv, 5, "self", 0)

	// 商机列表只见自己的
	w, env := callAs(t, r, http.MethodGet, "/api/v1/crm/opportunities?page=1&page_size=50", nil, "5")
	if w.Code != http.StatusOK {
		t.Fatalf("list opps as self: %d", w.Code)
	}
	names := listNames(t, env.Data)
	if names["别人的商机"] || !names["我的商机"] {
		t.Fatalf("self 档商机列表: %v", names)
	}

	// 合同列表只见自己的
	w, env = callAs(t, r, http.MethodGet, "/api/v1/crm/contracts?page=1&page_size=50", nil, "5")
	if w.Code != http.StatusOK {
		t.Fatalf("list contracts as self: %d", w.Code)
	}
	var ctList struct {
		List []struct {
			Title string `json:"title"`
		} `json:"list"`
		Total int64 `json:"total"`
	}
	decode(t, env.Data, &ctList)
	if ctList.Total != 1 || ctList.List[0].Title != "我的合同" {
		t.Fatalf("self 档合同列表: %+v", ctList)
	}

	// 越权商机详情/编辑/推进 → 403
	if w, _ := callAs(t, r, http.MethodGet, "/api/v1/crm/opportunities/"+otherOppID, nil, "5"); w.Code != http.StatusForbidden {
		t.Fatalf("越权商机详情应 403，got %d", w.Code)
	}
	if w, _ := callAs(t, r, http.MethodPut, "/api/v1/crm/opportunities/"+otherOppID,
		map[string]any{"name": "改名"}, "5"); w.Code != http.StatusForbidden {
		t.Fatalf("越权商机编辑应 403，got %d", w.Code)
	}
	if w, _ := callAs(t, r, http.MethodPost, "/api/v1/crm/opportunities/"+otherOppID+"/stage",
		map[string]any{"stage": "demand"}, "5"); w.Code != http.StatusForbidden {
		t.Fatalf("越权商机推进应 403，got %d", w.Code)
	}
}

// 商机转移负责人：self 档只能转给自己（转给他人 403）；all 档不限。
func TestOpportunityOwnerTransferGuard(t *testing.T) {
	r, srv := newTestRouterWithServer(t)
	_, env := call(t, r, http.MethodPost, "/api/v1/crm/customers", map[string]any{"name": "客户", "owner_user_id": 5}, "")
	cid, _ := strconv.ParseUint(jsonField(t, env.Data, "id"), 10, 64)
	w, env := call(t, r, http.MethodPost, "/api/v1/crm/opportunities",
		map[string]any{"customer_id": cid, "name": "商机", "owner_user_id": 5}, "")
	if w.Code != http.StatusOK {
		t.Fatalf("create opp: %d", w.Code)
	}
	oppID := jsonField(t, env.Data, "id")

	seedScope(t, srv, 5, "self", 0)
	// self 档把自己的商机转给 7 → 403
	if w, _ := callAs(t, r, http.MethodPut, "/api/v1/crm/opportunities/"+oppID,
		map[string]any{"owner_user_id": 7}, "5"); w.Code != http.StatusForbidden {
		t.Fatalf("self 档转移他人应 403，got %d", w.Code)
	}
	// all 档（用户 9）可以转
	if w, _ := callAs(t, r, http.MethodPut, "/api/v1/crm/opportunities/"+oppID,
		map[string]any{"owner_user_id": 7}, "9"); w.Code != http.StatusOK {
		t.Fatalf("all 档转移应 200，got %d", w.Code)
	}
}
