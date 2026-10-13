package api

// 覆盖销售管道核心链路：线索转化 → 商机阶段流转 → 合同回款约束 → 统计，
// 以及租户隔离。走真实路由表 + sqlite 内存库（与 im/cc 测试同法）。

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"sync/atomic"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/go-admin-kit/services/crm/internal/store/crm"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

var dbSeq atomic.Int64

func newTestRouter(t *testing.T) *gin.Engine {
	r, _ := newTestRouterWithServer(t)
	return r
}

func mustParseUint64(s string) uint64 {
	v, _ := strconv.ParseUint(s, 10, 64)
	return v
}

func newTestRouterWithServer(t *testing.T) (*gin.Engine, *Server) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	dsn := fmt.Sprintf("file:crmtest%d?mode=memory&cache=shared", dbSeq.Add(1))
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatalf("sqlite: %v", err)
	}
	sqlDB, _ := db.DB()
	sqlDB.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = sqlDB.Close() })
	st, err := store.NewWithDB(db)
	if err != nil {
		t.Fatalf("store: %v", err)
	}
	// Phase 2C：identity owner API mock（查询同一测试 db，反映各测试 seed 的 users/departments）。
	idSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/api/v1/internal/users/contacts":
			var req struct {
				TenantID uint64   `json:"tenant_id"`
				IDs      []uint64 `json:"ids"`
			}
			_ = json.NewDecoder(r.Body).Decode(&req)
			contacts := map[string]any{}
			var rows []struct {
				ID     uint64
				Status int
			}
			_ = db.Table("users").Select("id, status").
				Where("id IN ? AND tenant_id = ?", req.IDs, req.TenantID).Scan(&rows).Error
			for _, rw := range rows {
				if rw.Status != 1 {
					continue
				}
				contacts[fmt.Sprintf("%d", rw.ID)] = map[string]string{"email": "", "phone": ""}
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{"contacts": contacts})
		case r.URL.Path == "/api/v1/internal/users/data-scope":
			id := r.URL.Query().Get("id")
			tid := r.URL.Query().Get("tenant_id")
			var scopes []string
			_ = db.Table("roles").
				Joins("JOIN user_roles ON user_roles.role_id = roles.id").
				Where("user_roles.user_id = ? AND roles.tenant_id = ?", id, tid).
				Pluck("roles.data_scope", &scopes).Error
			ds := "self"
			for _, sc := range scopes {
				if sc == "all" {
					ds = "all"
					break
				}
				if sc == "dept" || sc == "department" || sc == "department_and_children" {
					ds = "dept"
				}
			}
			var deptID uint64
			_ = db.Table("users").Select("COALESCE(department_id,0)").
				Where("id = ? AND tenant_id = ?", id, tid).Limit(1).Scan(&deptID).Error
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{"data_scope": ds, "department_id": deptID})
		case r.URL.Path == "/api/v1/internal/departments/members":
			deptID := r.URL.Query().Get("department_id")
			tid := r.URL.Query().Get("tenant_id")
			// BFS 展开部门树（parent_id），再取树内全部启用成员。
			tree := []uint64{mustParseUint64(deptID)}
			frontier := []uint64{mustParseUint64(deptID)}
			for depth := 0; depth < 10 && len(frontier) > 0; depth++ {
				var children []uint64
				_ = db.Table("departments").
					Where("parent_id IN ? AND tenant_id = ?", frontier, tid).
					Pluck("id", &children).Error
				if len(children) == 0 {
					break
				}
				tree = append(tree, children...)
				frontier = children
			}
			var all []uint64
			_ = db.Table("users").Where("department_id IN ? AND tenant_id = ? AND status = 1", tree, tid).Pluck("id", &all).Error
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{"ids": all})
		case r.URL.Path == "/api/v1/internal/departments/info":
			id := r.URL.Query().Get("id")
			tid := r.URL.Query().Get("tenant_id")
			var n int64
			_ = db.Table("departments").Where("id = ? AND tenant_id = ?", id, tid).Count(&n).Error
			if n == 0 {
				w.WriteHeader(http.StatusNotFound)
				return
			}
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"id":` + id + `}`))
		default:
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{}`))
		}
	}))
	st.SetIdentity(idSrv.URL, "test-internal-token")
	t.Cleanup(idSrv.Close)
	// data scope 依赖的系统表（生产与 system 同库；测试里手建最小结构，
	// tenant_id 默认 1 与请求默认租户对齐），默认用户 9 配 all 角色，
	// 既有测试不受硬隔离影响。
	for _, ddl := range []string{
		`CREATE TABLE IF NOT EXISTS roles (id INTEGER PRIMARY KEY, tenant_id INTEGER NOT NULL DEFAULT 1, data_scope TEXT NOT NULL DEFAULT 'self')`,
		`CREATE TABLE IF NOT EXISTS user_roles (id INTEGER PRIMARY KEY AUTOINCREMENT, user_id INTEGER, role_id INTEGER)`,
		`CREATE TABLE IF NOT EXISTS users (id INTEGER PRIMARY KEY, tenant_id INTEGER NOT NULL DEFAULT 1, username TEXT DEFAULT '', nickname TEXT DEFAULT '', department_id INTEGER DEFAULT 0, status INTEGER DEFAULT 1)`,
		`CREATE TABLE IF NOT EXISTS departments (id INTEGER PRIMARY KEY, tenant_id INTEGER NOT NULL DEFAULT 1, name TEXT DEFAULT '', parent_id INTEGER DEFAULT 0, sort INTEGER DEFAULT 0, status INTEGER DEFAULT 1)`,
		`INSERT INTO roles (id, data_scope) VALUES (1, 'all')`,
		`INSERT INTO user_roles (user_id, role_id) VALUES (9, 1)`,
		`INSERT INTO users (id, status) VALUES (9, 1)`,
	} {
		if err := db.Exec(ddl).Error; err != nil {
			t.Fatalf("scope ddl: %v", err)
		}
	}
	store.InvalidateScopeCache()
	srv := &Server{Store: st, Secret: "test-secret-at-least-32-characters!!"}
	r := gin.New()
	srv.RegisterRoutes(r)
	return r, srv
}

type envelope struct {
	Code    int             `json:"code"`
	Message string          `json:"message"`
	Data    json.RawMessage `json:"data"`
}

func call(t *testing.T, r *gin.Engine, method, path string, body any, tenant string) (*httptest.ResponseRecorder, envelope) {
	t.Helper()
	var buf bytes.Buffer
	if body != nil {
		if err := json.NewEncoder(&buf).Encode(body); err != nil {
			t.Fatal(err)
		}
	}
	req := httptest.NewRequest(method, path, &buf)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Auth-User-ID", "9")
	req.Header.Set("X-Auth-Username", "sales9")
	req.Header.Set("X-Auth-Permissions", "*")
	if tenant != "" {
		req.Header.Set("X-Auth-Tenant-ID", tenant)
	}
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	var env envelope
	_ = json.Unmarshal(w.Body.Bytes(), &env)
	return w, env
}

func decode(t *testing.T, raw json.RawMessage, v any) {
	t.Helper()
	if err := json.Unmarshal(raw, v); err != nil {
		t.Fatalf("decode: %v (%s)", err, raw)
	}
}

func TestLeadToWonPipeline(t *testing.T) {
	r := newTestRouter(t)

	// 1. 建线索
	w, env := call(t, r, http.MethodPost, "/api/v1/crm/leads", map[string]any{
		"name": "张三", "company": "极光科技", "phone": "13800000001", "source": "call",
	}, "")
	if w.Code != http.StatusOK {
		t.Fatalf("create lead: %d %s", w.Code, w.Body.String())
	}
	var lead struct {
		ID uint64 `json:"id"`
	}
	decode(t, env.Data, &lead)

	// 2. 转化：客户+主联系人+商机一步生成
	w, env = call(t, r, http.MethodPost, fmt.Sprintf("/api/v1/crm/leads/%d/convert", lead.ID), map[string]any{
		"opportunity_name": "年度SaaS订阅", "amount_cents": 1200000,
	}, "")
	if w.Code != http.StatusOK {
		t.Fatalf("convert: %d %s", w.Code, w.Body.String())
	}
	var conv struct {
		Customer struct {
			ID   uint64 `json:"id"`
			Name string `json:"name"`
		} `json:"customer"`
		Opportunity struct {
			ID          uint64 `json:"id"`
			Stage       string `json:"stage"`
			Probability int    `json:"probability"`
		} `json:"opportunity"`
	}
	decode(t, env.Data, &conv)
	if conv.Customer.Name != "极光科技" || conv.Opportunity.Stage != "initial" || conv.Opportunity.Probability != 10 {
		t.Fatalf("convert result: %+v", conv)
	}

	// 3. 重复转化被拒
	w, _ = call(t, r, http.MethodPost, fmt.Sprintf("/api/v1/crm/leads/%d/convert", lead.ID), nil, "")
	if w.Code != http.StatusBadRequest {
		t.Fatalf("re-convert status %d", w.Code)
	}

	// 4. 客户详情聚合出主联系人与商机
	w, env = call(t, r, http.MethodGet, fmt.Sprintf("/api/v1/crm/customers/%d", conv.Customer.ID), nil, "")
	if w.Code != http.StatusOK {
		t.Fatalf("detail status %d", w.Code)
	}
	var detail struct {
		Contacts []struct {
			Name      string `json:"name"`
			IsPrimary bool   `json:"is_primary"`
		} `json:"contacts"`
		Opportunities []struct{ ID uint64 } `json:"opportunities"`
	}
	decode(t, env.Data, &detail)
	if len(detail.Contacts) != 1 || !detail.Contacts[0].IsPrimary || detail.Contacts[0].Name != "张三" {
		t.Fatalf("contacts: %+v", detail.Contacts)
	}

	// 5. 阶段推进 initial→proposal→won；won 后不可再动
	oppID := conv.Opportunity.ID
	for _, stage := range []string{"proposal", "won"} {
		w, _ = call(t, r, http.MethodPost, fmt.Sprintf("/api/v1/crm/opportunities/%d/stage", oppID),
			map[string]any{"stage": stage, "note": "推进"}, "")
		if w.Code != http.StatusOK {
			t.Fatalf("stage %s: %d %s", stage, w.Code, w.Body.String())
		}
	}
	w, _ = call(t, r, http.MethodPost, fmt.Sprintf("/api/v1/crm/opportunities/%d/stage", oppID),
		map[string]any{"stage": "lost"}, "")
	if w.Code != http.StatusBadRequest {
		t.Fatalf("stage after won: %d", w.Code)
	}

	// 6. 阶段日志完整（创建+2 次推进）
	w, env = call(t, r, http.MethodGet, fmt.Sprintf("/api/v1/crm/opportunities/%d", oppID), nil, "")
	var oppDetail struct {
		StageLogs []struct {
			ToStage string `json:"to_stage"`
		} `json:"stage_logs"`
	}
	decode(t, env.Data, &oppDetail)
	if len(oppDetail.StageLogs) != 3 || oppDetail.StageLogs[2].ToStage != "won" {
		t.Fatalf("stage logs: %+v", oppDetail.StageLogs)
	}

	// 7. 从商机签合同（金额默认带出）→ 生效 → 回款约束
	w, env = call(t, r, http.MethodPost, "/api/v1/crm/contracts", map[string]any{
		"no": "HT-2026-001", "title": "年度订阅合同", "customer_id": conv.Customer.ID,
		"opportunity_id": oppID,
	}, "")
	if w.Code != http.StatusOK {
		t.Fatalf("contract: %d %s", w.Code, w.Body.String())
	}
	var ct struct {
		ID          uint64 `json:"id"`
		AmountCents int64  `json:"amount_cents"`
	}
	decode(t, env.Data, &ct)
	if ct.AmountCents != 1200000 {
		t.Fatalf("contract amount not inherited: %d", ct.AmountCents)
	}

	// 草拟状态不可回款
	w, _ = call(t, r, http.MethodPost, fmt.Sprintf("/api/v1/crm/contracts/%d/payments", ct.ID),
		map[string]any{"amount_cents": 500000}, "")
	if w.Code != http.StatusBadRequest {
		t.Fatalf("payment on draft: %d", w.Code)
	}
	// 生效后可回款
	w, _ = call(t, r, http.MethodPut, fmt.Sprintf("/api/v1/crm/contracts/%d", ct.ID),
		map[string]any{"status": "active", "signed_at": "2026-07-18"}, "")
	if w.Code != http.StatusOK {
		t.Fatalf("activate: %d %s", w.Code, w.Body.String())
	}
	w, _ = call(t, r, http.MethodPost, fmt.Sprintf("/api/v1/crm/contracts/%d/payments", ct.ID),
		map[string]any{"amount_cents": 500000}, "")
	if w.Code != http.StatusOK {
		t.Fatalf("payment1: %d %s", w.Code, w.Body.String())
	}
	// 超额回款被拒
	w, _ = call(t, r, http.MethodPost, fmt.Sprintf("/api/v1/crm/contracts/%d/payments", ct.ID),
		map[string]any{"amount_cents": 800000}, "")
	if w.Code != http.StatusBadRequest {
		t.Fatalf("over payment: %d %s", w.Code, w.Body.String())
	}
	// 合同编号租户内唯一
	w, _ = call(t, r, http.MethodPost, "/api/v1/crm/contracts", map[string]any{
		"no": "HT-2026-001", "title": "重复编号", "customer_id": conv.Customer.ID,
	}, "")
	if w.Code != http.StatusBadRequest {
		t.Fatalf("dup no: %d", w.Code)
	}

	// 8. 统计：赢单与回款进账
	w, env = call(t, r, http.MethodGet, "/api/v1/crm/stats?days=30", nil, "")
	if w.Code != http.StatusOK {
		t.Fatalf("stats: %d", w.Code)
	}
	var sres struct {
		Stats struct {
			WonCount       int64 `json:"won_count"`
			WonAmountCents int64 `json:"won_amount_cents"`
			PaidCents      int64 `json:"paid_cents"`
			ContractCents  int64 `json:"contract_cents"`
			ConvertedLeads int64 `json:"converted_leads"`
			Owners         []struct {
				OwnerUserID    uint64 `json:"owner_user_id"`
				WonCount       int64  `json:"won_count"`
				WonAmountCents int64  `json:"won_amount_cents"`
				ContractCents  int64  `json:"contract_cents"`
				PaidCents      int64  `json:"paid_cents"`
			} `json:"owners"`
		} `json:"stats"`
	}
	decode(t, env.Data, &sres)
	if sres.Stats.WonCount != 1 || sres.Stats.WonAmountCents != 1200000 ||
		sres.Stats.PaidCents != 500000 || sres.Stats.ContractCents != 1200000 ||
		sres.Stats.ConvertedLeads != 1 {
		t.Fatalf("stats: %+v", sres.Stats)
	}
	// 业绩排行：全链路都由 user 9 完成
	if len(sres.Stats.Owners) != 1 {
		t.Fatalf("owners len = %d", len(sres.Stats.Owners))
	}
	top := sres.Stats.Owners[0]
	if top.OwnerUserID != 9 || top.WonCount != 1 || top.WonAmountCents != 1200000 ||
		top.ContractCents != 1200000 || top.PaidCents != 500000 {
		t.Fatalf("owner stat: %+v", top)
	}
}

func TestTenantIsolation(t *testing.T) {
	r := newTestRouter(t)

	// 租户 1 建客户
	w, env := call(t, r, http.MethodPost, "/api/v1/crm/customers", map[string]any{"name": "一号租户客户"}, "1")
	if w.Code != http.StatusOK {
		t.Fatalf("create: %d", w.Code)
	}
	var cu struct {
		ID uint64 `json:"id"`
	}
	decode(t, env.Data, &cu)

	// 租户 2 看不到、改不了
	w, _ = call(t, r, http.MethodGet, fmt.Sprintf("/api/v1/crm/customers/%d", cu.ID), nil, "2")
	if w.Code != http.StatusNotFound {
		t.Fatalf("cross-tenant read: %d", w.Code)
	}
	w, env = call(t, r, http.MethodGet, "/api/v1/crm/customers", nil, "2")
	var list struct {
		Total int64 `json:"total"`
	}
	decode(t, env.Data, &list)
	if list.Total != 0 {
		t.Fatalf("cross-tenant list total=%d", list.Total)
	}
	// 租户 2 也不能给租户 1 的客户建商机
	w, _ = call(t, r, http.MethodPost, "/api/v1/crm/opportunities",
		map[string]any{"customer_id": cu.ID, "name": "越权商机"}, "2")
	if w.Code != http.StatusBadRequest {
		t.Fatalf("cross-tenant opportunity: %d", w.Code)
	}
}

func TestFollowUps(t *testing.T) {
	r := newTestRouter(t)
	_, env := call(t, r, http.MethodPost, "/api/v1/crm/customers", map[string]any{"name": "跟进客户"}, "")
	var cu struct {
		ID uint64 `json:"id"`
	}
	decode(t, env.Data, &cu)

	w, _ := call(t, r, http.MethodPost, "/api/v1/crm/followups", map[string]any{
		"target_type": "customer", "target_id": cu.ID,
		"content": "电话沟通了需求，下周复访", "next_at": "2026-07-25 10:00",
	}, "")
	if w.Code != http.StatusOK {
		t.Fatalf("followup: %d %s", w.Code, w.Body.String())
	}
	// 不存在的目标被拒
	w, _ = call(t, r, http.MethodPost, "/api/v1/crm/followups", map[string]any{
		"target_type": "lead", "target_id": 99999, "content": "x",
	}, "")
	if w.Code != http.StatusBadRequest {
		t.Fatalf("bad target: %d", w.Code)
	}
	w, env = call(t, r, http.MethodGet, fmt.Sprintf("/api/v1/crm/followups?target_type=customer&target_id=%d", cu.ID), nil, "")
	var fres struct {
		List []struct {
			Content string `json:"content"`
		} `json:"list"`
	}
	decode(t, env.Data, &fres)
	if len(fres.List) != 1 || fres.List[0].Content == "" {
		t.Fatalf("followup list: %+v", fres.List)
	}
}
