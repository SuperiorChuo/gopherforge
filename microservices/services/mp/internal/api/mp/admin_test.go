package api

// 后台接口单测：账号 CRUD + 消息/粉丝列表。用 X-Auth-* 头模拟网关鉴权，
// sqlite in-memory 隔离每个用例。重点覆盖 tenant 隔离与密钥明文不外泄。

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/go-admin-kit/services/mp/internal/store/mp"
	"gorm.io/gorm"
)

// newAdminServer 建一个不预置账号的服务器（账号由用例自建）。
func newAdminServer(t *testing.T) (*Server, *gin.Engine) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	st, err := store.NewWithDB(db)
	if err != nil {
		t.Fatalf("migrate: %v", err)
	}
	srv := &Server{Store: st, Secret: "test-jwt-secret"}
	r := gin.New()
	srv.RegisterRoutes(r)
	return srv, r
}

// doAuthed 发一个带网关鉴权头的请求。tenant 传 0 时不带 tenant 头（默认租户 1）。
func doAuthed(r *gin.Engine, method, path, body string, tenantID uint64) *httptest.ResponseRecorder {
	var rd *bytes.Reader
	if body != "" {
		rd = bytes.NewReader([]byte(body))
	} else {
		rd = bytes.NewReader(nil)
	}
	req := httptest.NewRequest(method, path, rd)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Auth-User-ID", "42")
	req.Header.Set("X-Auth-Username", "tester")
	req.Header.Set("X-Auth-Permissions", "*")
	if tenantID != 0 {
		req.Header.Set("X-Auth-Tenant-ID", fmt.Sprintf("%d", tenantID))
	}
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

// envelope 解析 {code,message,data} 信封，data 保留成 json.RawMessage。
type envelope struct {
	Code    int             `json:"code"`
	Message string          `json:"message"`
	Data    json.RawMessage `json:"data"`
}

func decodeEnvelope(t *testing.T, w *httptest.ResponseRecorder) envelope {
	t.Helper()
	var e envelope
	if err := json.Unmarshal(w.Body.Bytes(), &e); err != nil {
		t.Fatalf("decode envelope: %v (body=%s)", err, w.Body.String())
	}
	return e
}

// createAccount 建账号并返回其 id。
func createAccount(t *testing.T, r *gin.Engine, appID, name string, tenantID uint64) uint64 {
	t.Helper()
	body := fmt.Sprintf(`{"app_id":%q,"name":%q,"type":"service","gh_id":"gh_x","secret_ref":"MP_SECRET_%s","token":"tok123","aes_key_ref":"MP_AESKEY_%s","encrypt_mode":"plain","auto_publish":true}`,
		appID, name, appID, appID)
	w := doAuthed(r, http.MethodPost, "/api/v1/mp/accounts", body, tenantID)
	if w.Code != http.StatusOK {
		t.Fatalf("create account: code=%d body=%s", w.Code, w.Body.String())
	}
	var out struct {
		ID uint64 `json:"id"`
	}
	e := decodeEnvelope(t, w)
	if err := json.Unmarshal(e.Data, &out); err != nil {
		t.Fatalf("decode account: %v", err)
	}
	if out.ID == 0 {
		t.Fatalf("created account id is 0: %s", w.Body.String())
	}
	return out.ID
}

func TestAccountCRUDLifecycle(t *testing.T) {
	_, r := newAdminServer(t)
	id := createAccount(t, r, "wxCRUD001", "运营A号", 1)

	// 列表可见
	w := doAuthed(r, http.MethodGet, "/api/v1/mp/accounts", "", 1)
	if w.Code != http.StatusOK {
		t.Fatalf("list: code=%d", w.Code)
	}
	e := decodeEnvelope(t, w)
	var list struct {
		List     []map[string]any `json:"list"`
		Total    int64            `json:"total"`
		Page     int              `json:"page"`
		PageSize int              `json:"page_size"`
	}
	if err := json.Unmarshal(e.Data, &list); err != nil {
		t.Fatalf("decode list: %v", err)
	}
	if list.Total != 1 || len(list.List) != 1 {
		t.Fatalf("list total=%d len=%d, want 1", list.Total, len(list.List))
	}
	if list.Page != 1 || list.PageSize != 20 {
		t.Fatalf("list page=%d size=%d, want 1/20", list.Page, list.PageSize)
	}

	// 详情
	w = doAuthed(r, http.MethodGet, fmt.Sprintf("/api/v1/mp/accounts/%d", id), "", 1)
	if w.Code != http.StatusOK {
		t.Fatalf("get: code=%d body=%s", w.Code, w.Body.String())
	}

	// 改名 + 改 type
	upd := `{"name":"运营A号改","type":"subscribe","gh_id":"gh_new","encrypt_mode":"compat","auto_publish":false,"status":"active"}`
	w = doAuthed(r, http.MethodPut, fmt.Sprintf("/api/v1/mp/accounts/%d", id), upd, 1)
	if w.Code != http.StatusOK {
		t.Fatalf("update: code=%d body=%s", w.Code, w.Body.String())
	}
	e = decodeEnvelope(t, w)
	var updated struct {
		Name        string `json:"name"`
		Type        string `json:"type"`
		AppID       string `json:"app_id"`
		EncryptMode string `json:"encrypt_mode"`
	}
	_ = json.Unmarshal(e.Data, &updated)
	if updated.Name != "运营A号改" || updated.Type != "subscribe" || updated.EncryptMode != "compat" {
		t.Fatalf("update not applied: %+v", updated)
	}
	if updated.AppID != "wxCRUD001" {
		t.Fatalf("app_id should be immutable, got %q", updated.AppID)
	}

	// 删
	w = doAuthed(r, http.MethodDelete, fmt.Sprintf("/api/v1/mp/accounts/%d", id), "", 1)
	if w.Code != http.StatusOK {
		t.Fatalf("delete: code=%d body=%s", w.Code, w.Body.String())
	}
	// 再删返回 404
	w = doAuthed(r, http.MethodDelete, fmt.Sprintf("/api/v1/mp/accounts/%d", id), "", 1)
	if w.Code != http.StatusNotFound {
		t.Fatalf("delete missing: code=%d, want 404", w.Code)
	}
}

func TestAccountAppIDConflict(t *testing.T) {
	_, r := newAdminServer(t)
	createAccount(t, r, "wxDUP001", "第一个", 1)

	body := `{"app_id":"wxDUP001","name":"重复号","type":"service","encrypt_mode":"plain"}`
	w := doAuthed(r, http.MethodPost, "/api/v1/mp/accounts", body, 1)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("dup app_id: code=%d, want 400 (body=%s)", w.Code, w.Body.String())
	}
	e := decodeEnvelope(t, w)
	if !strings.Contains(e.Message, "wxDUP001") {
		t.Fatalf("conflict message should mention app_id: %q", e.Message)
	}
}

func TestAccountInvalidEnums(t *testing.T) {
	_, r := newAdminServer(t)
	// 非法 type
	w := doAuthed(r, http.MethodPost, "/api/v1/mp/accounts",
		`{"app_id":"wxBADTYPE","name":"x","type":"bogus","encrypt_mode":"plain"}`, 1)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("bad type: code=%d, want 400", w.Code)
	}
	// 非法 encrypt_mode
	w = doAuthed(r, http.MethodPost, "/api/v1/mp/accounts",
		`{"app_id":"wxBADENC","name":"x","type":"service","encrypt_mode":"rot13"}`, 1)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("bad encrypt_mode: code=%d, want 400", w.Code)
	}
	// 缺 app_id
	w = doAuthed(r, http.MethodPost, "/api/v1/mp/accounts",
		`{"name":"x","type":"service","encrypt_mode":"plain"}`, 1)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("missing app_id: code=%d, want 400", w.Code)
	}
	// 非法 status
	w = doAuthed(r, http.MethodPost, "/api/v1/mp/accounts",
		`{"app_id":"wxBADSTATUS","name":"x","type":"service","encrypt_mode":"plain","status":"paused"}`, 1)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("bad status: code=%d, want 400", w.Code)
	}
	id := createAccount(t, r, "wxUPDATESTATUS", "状态校验号", 1)
	w = doAuthed(r, http.MethodPut, fmt.Sprintf("/api/v1/mp/accounts/%d", id),
		`{"name":"状态校验号","status":"paused"}`, 1)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("bad update status: code=%d, want 400", w.Code)
	}
}

func TestAccountTenantIsolation(t *testing.T) {
	_, r := newAdminServer(t)
	id1 := createAccount(t, r, "wxT1", "租户1的号", 1)
	createAccount(t, r, "wxT2", "租户2的号", 2)

	// 租户 2 列表看不到租户 1 的账号
	w := doAuthed(r, http.MethodGet, "/api/v1/mp/accounts", "", 2)
	e := decodeEnvelope(t, w)
	var list struct {
		List  []map[string]any `json:"list"`
		Total int64            `json:"total"`
	}
	_ = json.Unmarshal(e.Data, &list)
	if list.Total != 1 || len(list.List) != 1 {
		t.Fatalf("tenant2 list total=%d, want 1 (isolation leak)", list.Total)
	}
	if list.List[0]["app_id"] != "wxT2" {
		t.Fatalf("tenant2 sees wrong account: %v", list.List[0]["app_id"])
	}

	// 租户 2 取不到租户 1 的账号详情（404）
	w = doAuthed(r, http.MethodGet, fmt.Sprintf("/api/v1/mp/accounts/%d", id1), "", 2)
	if w.Code != http.StatusNotFound {
		t.Fatalf("cross-tenant get: code=%d, want 404", w.Code)
	}

	// 租户 2 删不到租户 1 的账号（404）
	w = doAuthed(r, http.MethodDelete, fmt.Sprintf("/api/v1/mp/accounts/%d", id1), "", 2)
	if w.Code != http.StatusNotFound {
		t.Fatalf("cross-tenant delete: code=%d, want 404", w.Code)
	}
}

func TestAccountListHidesSecrets(t *testing.T) {
	_, r := newAdminServer(t)
	// token 是敏感明文，appsecret/aeskey 只存引用名。断言明文 token 不出现在响应里。
	body := `{"app_id":"wxSECRET","name":"密钥号","type":"service","secret_ref":"MP_SECRET_REF","token":"TEST_PLAINTEXT_TOKEN_XYZ","aes_key_ref":"MP_AESKEY_REF","encrypt_mode":"safe"}`
	w := doAuthed(r, http.MethodPost, "/api/v1/mp/accounts", body, 1)
	if w.Code != http.StatusOK {
		t.Fatalf("create: code=%d body=%s", w.Code, w.Body.String())
	}
	if strings.Contains(w.Body.String(), "TEST_PLAINTEXT_TOKEN_XYZ") {
		t.Fatalf("create response leaked token: %s", w.Body.String())
	}

	w = doAuthed(r, http.MethodGet, "/api/v1/mp/accounts", "", 1)
	raw := w.Body.String()
	if strings.Contains(raw, "TEST_PLAINTEXT_TOKEN_XYZ") {
		t.Fatalf("list response leaked token plaintext: %s", raw)
	}
	// 引用名允许回显（前端要展示配置状态）。
	if !strings.Contains(raw, "MP_SECRET_REF") {
		t.Fatalf("secret_ref should be visible: %s", raw)
	}
}

func TestListMessagesTenantIsolationAndFilter(t *testing.T) {
	srv, r := newAdminServer(t)
	acc1 := createAccount(t, r, "wxMSG1", "号1", 1)

	// 直接向 store 塞消息（模拟回调留档）。
	_ = srv.Store.LogMessage(&store.MpMessage{
		TenantID: 1, AccountID: acc1, OpenID: "openA", Direction: "in",
		MsgType: "text", Content: "hi", MsgID: "m1",
	})
	_ = srv.Store.LogMessage(&store.MpMessage{
		TenantID: 1, AccountID: acc1, OpenID: "openA", Direction: "out",
		MsgType: "text", Content: "收到：hi",
	})
	// 另一租户的消息不应出现。
	_ = srv.Store.LogMessage(&store.MpMessage{
		TenantID: 2, AccountID: 999, OpenID: "openB", Direction: "in",
		MsgType: "text", Content: "别人的", MsgID: "m2",
	})

	// tenant 1 全部
	w := doAuthed(r, http.MethodGet, "/api/v1/mp/messages", "", 1)
	e := decodeEnvelope(t, w)
	var list struct {
		List  []map[string]any `json:"list"`
		Total int64            `json:"total"`
	}
	_ = json.Unmarshal(e.Data, &list)
	if list.Total != 2 {
		t.Fatalf("tenant1 messages total=%d, want 2", list.Total)
	}

	// direction 过滤
	w = doAuthed(r, http.MethodGet, "/api/v1/mp/messages?direction=in", "", 1)
	e = decodeEnvelope(t, w)
	_ = json.Unmarshal(e.Data, &list)
	if list.Total != 1 {
		t.Fatalf("direction=in total=%d, want 1", list.Total)
	}

	// tenant 2 只看到自己的
	w = doAuthed(r, http.MethodGet, "/api/v1/mp/messages", "", 2)
	e = decodeEnvelope(t, w)
	_ = json.Unmarshal(e.Data, &list)
	if list.Total != 1 {
		t.Fatalf("tenant2 messages total=%d, want 1 (isolation)", list.Total)
	}
}

func TestListFansEmptyThenSeeded(t *testing.T) {
	srv, r := newAdminServer(t)
	acc := createAccount(t, r, "wxFAN1", "粉丝号", 1)

	// 本阶段初始为空。
	w := doAuthed(r, http.MethodGet, "/api/v1/mp/fans", "", 1)
	e := decodeEnvelope(t, w)
	var list struct {
		List  []map[string]any `json:"list"`
		Total int64            `json:"total"`
	}
	_ = json.Unmarshal(e.Data, &list)
	if list.Total != 0 {
		t.Fatalf("fans initial total=%d, want 0", list.Total)
	}

	// 直接塞一条粉丝（模拟 M4 拉取），验证列表与 keyword 过滤。
	if err := srv.Store.DB().Create(&store.MpFan{
		TenantID: 1, AccountID: acc, OpenID: "fan_open_1", Nickname: "张三",
		Subscribed: true,
	}).Error; err != nil {
		t.Fatalf("seed fan: %v", err)
	}
	if err := srv.Store.DB().Create(&store.MpFan{
		TenantID: 2, AccountID: 999, OpenID: "fan_other", Nickname: "李四",
		Subscribed: true,
	}).Error; err != nil {
		t.Fatalf("seed other fan: %v", err)
	}

	w = doAuthed(r, http.MethodGet, "/api/v1/mp/fans", "", 1)
	e = decodeEnvelope(t, w)
	_ = json.Unmarshal(e.Data, &list)
	if list.Total != 1 {
		t.Fatalf("tenant1 fans total=%d, want 1 (isolation)", list.Total)
	}

	// keyword 匹配 nickname
	w = doAuthed(r, http.MethodGet, "/api/v1/mp/fans?keyword=张", "", 1)
	e = decodeEnvelope(t, w)
	_ = json.Unmarshal(e.Data, &list)
	if list.Total != 1 {
		t.Fatalf("fans keyword=张 total=%d, want 1", list.Total)
	}
	// keyword 不匹配
	w = doAuthed(r, http.MethodGet, "/api/v1/mp/fans?keyword=王五", "", 1)
	e = decodeEnvelope(t, w)
	_ = json.Unmarshal(e.Data, &list)
	if list.Total != 0 {
		t.Fatalf("fans keyword=王五 total=%d, want 0", list.Total)
	}
}

func TestAdminRequiresAuth(t *testing.T) {
	_, r := newAdminServer(t)
	// 无鉴权头 → 401
	req := httptest.NewRequest(http.MethodGet, "/api/v1/mp/accounts", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("unauthenticated list: code=%d, want 401", w.Code)
	}
}
