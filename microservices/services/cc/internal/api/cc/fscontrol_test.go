package api

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/go-admin-kit/services/cc/internal/fsclient"
	"github.com/go-admin-kit/services/cc/internal/store/cc"
	"gorm.io/gorm"
)

// fakeControlAPI 校验 token 并按路径回包。
func fakeControlAPI(t *testing.T) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-CC-Token") != "test-token" {
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = w.Write([]byte(`{"error":"unauthorized"}`))
			return
		}
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/v1/registrations":
			_, _ = w.Write([]byte(`{"total":1,"list":[{"reg_user":"1000","network_ip":"192.168.1.5"}]}`))
		case "/v1/channels":
			_, _ = w.Write([]byte(`{"total":0,"list":[]}`))
		case "/v1/calls/originate":
			var body map[string]string
			_ = json.NewDecoder(r.Body).Decode(&body)
			if body["from_ext"] != "1000" || body["to"] != "9196" {
				t.Errorf("originate body = %v", body)
			}
			_, _ = w.Write([]byte(`{"queued":true,"reply":"+OK Job-UUID: j1"}`))
		case "/v1/calls/hangup":
			_, _ = w.Write([]byte(`{"ok":true,"reply":"+OK"}`))
		case "/v1/extensions":
			if r.Method == http.MethodPost {
				_, _ = w.Write([]byte(`{"ok":true,"ext":"2000","reloaded":true}`))
			} else {
				_, _ = w.Write([]byte(`{"total":5,"list":[{"ext":"1000","source":"static"},{"ext":"2000","source":"dynamic"}]}`))
			}
		case "/v1/extensions/2000":
			if r.Method != http.MethodDelete {
				w.WriteHeader(http.StatusMethodNotAllowed)
				return
			}
			_, _ = w.Write([]byte(`{"ok":true,"reloaded":true}`))
		case "/v1/recordings":
			_, _ = w.Write([]byte(`{"total":1,"list":[{"name":"abc.wav","size":1024}]}`))
		case "/v1/recordings/abc.wav":
			w.Header().Set("Content-Type", "audio/wav")
			_, _ = w.Write([]byte("RIFFfakewav"))
		case "/v1/voicemails":
			_, _ = w.Write([]byte(`{"total":1,"list":[{"ext":"2001","file":"msg_a.wav","size":2048}]}`))
		case "/v1/voicemails/2001/msg_a.wav":
			if r.Method == http.MethodDelete {
				_, _ = w.Write([]byte(`{"ok":true}`))
				return
			}
			w.Header().Set("Content-Type", "audio/wav")
			_, _ = w.Write([]byte("RIFFvmwav"))
		default:
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte(`{"error":"not found"}`))
		}
	}))
}

// fsTestSrv 是 newFSTestRouter 最近一次装配的 Server，越权用例种数据用。
var fsTestSrv *Server

func newFSTestRouter(t *testing.T, base string) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.TestMode)
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(&store.Call{}, &store.WebhookLog{}, &store.Agent{},
		&store.CCRate{}, &store.CCBillingAccount{}, &store.CCCallCharge{},
		&store.CCRiskPolicy{}, &store.CCRiskListEntry{}, &store.CCRiskAttempt{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	srv := &Server{Store: store.NewWithDB(db), FS: fsclient.New(base, "test-token"), Secret: "test-jwt-secret"}
	fsTestSrv = srv
	r := gin.New()
	r.GET("/api/v1/cc/fs/registrations", srv.FSRegistrations)
	r.GET("/api/v1/cc/fs/channels", srv.FSChannels)
	r.POST("/api/v1/cc/fs/originate", srv.FSOriginate)
	r.POST("/api/v1/cc/fs/hangup", srv.FSHangup)
	r.GET("/api/v1/cc/fs/extensions", srv.FSExtensions)
	r.POST("/api/v1/cc/fs/extensions", srv.FSCreateExtension)
	r.DELETE("/api/v1/cc/fs/extensions/:ext", srv.FSDeleteExtension)
	r.GET("/api/v1/cc/fs/recordings", srv.FSRecordings)
	r.GET("/api/v1/cc/fs/recordings/:file", srv.FSRecordingFile)
	r.GET("/api/v1/cc/fs/voicemails", srv.FSVoicemails)
	r.GET("/api/v1/cc/fs/voicemails/:ext/:file", srv.FSVoicemailFile)
	r.DELETE("/api/v1/cc/fs/voicemails/:ext/:file", srv.FSVoicemailDelete)
	return r
}

func TestFSExtensionCRUDProxy(t *testing.T) {
	upstream := fakeControlAPI(t)
	defer upstream.Close()
	r := newFSTestRouter(t, upstream.URL)

	// 创建
	w := httptest.NewRecorder()
	req := asAgent(httptest.NewRequest(http.MethodPost, "/api/v1/cc/fs/extensions",
		strings.NewReader(`{"ext":"2000","password":"abcd1234","label":"客服"}`)))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), `"reloaded":true`) {
		t.Fatalf("create: code=%d body=%s", w.Code, w.Body.String())
	}

	// 非法分机号被本层拦截
	w = httptest.NewRecorder()
	req = asAgent(httptest.NewRequest(http.MethodPost, "/api/v1/cc/fs/extensions",
		strings.NewReader(`{"ext":"20 00","password":"abcd1234"}`)))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("invalid ext: code=%d", w.Code)
	}

	// 列表
	w = httptest.NewRecorder()
	r.ServeHTTP(w, asAgent(httptest.NewRequest(http.MethodGet, "/api/v1/cc/fs/extensions", nil)))
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), `"dynamic"`) {
		t.Fatalf("list: code=%d body=%s", w.Code, w.Body.String())
	}

	// 删除
	w = httptest.NewRecorder()
	r.ServeHTTP(w, asAgent(httptest.NewRequest(http.MethodDelete, "/api/v1/cc/fs/extensions/2000", nil)))
	if w.Code != http.StatusOK {
		t.Fatalf("delete: code=%d body=%s", w.Code, w.Body.String())
	}
}

func TestFSRecordingStream(t *testing.T) {
	upstream := fakeControlAPI(t)
	defer upstream.Close()
	r := newFSTestRouter(t, upstream.URL)

	w := httptest.NewRecorder()
	r.ServeHTTP(w, asAgent(httptest.NewRequest(http.MethodGet, "/api/v1/cc/fs/recordings/abc.wav", nil)))
	if w.Code != http.StatusOK || w.Header().Get("Content-Type") != "audio/wav" || !strings.Contains(w.Body.String(), "RIFF") {
		t.Fatalf("stream: code=%d ct=%s", w.Code, w.Header().Get("Content-Type"))
	}

	// 非 wav / 带编码斜杠的文件名都进不来（白名单 400，或路由不匹配 404）
	for _, target := range []string{
		"/api/v1/cc/fs/recordings/abc.mp3",
		"/api/v1/cc/fs/recordings/..%2Fsecret.wav",
	} {
		w = httptest.NewRecorder()
		r.ServeHTTP(w, asAgent(httptest.NewRequest(http.MethodGet, target, nil)))
		if w.Code != http.StatusBadRequest && w.Code != http.StatusNotFound {
			t.Fatalf("%s: code=%d", target, w.Code)
		}
	}
}

func asAgent(req *http.Request) *http.Request {
	req.Header.Set("X-Auth-User-ID", "1")
	req.Header.Set("X-Auth-Tenant-ID", "1")
	return req
}

// asTenantAgent 模拟普通租户（非平台）坐席。
func asTenantAgent(req *http.Request, tenantID string) *http.Request {
	req.Header.Set("X-Auth-User-ID", "9")
	req.Header.Set("X-Auth-Tenant-ID", tenantID)
	return req
}

func TestFSVoicemailProxy(t *testing.T) {
	upstream := fakeControlAPI(t)
	defer upstream.Close()
	r := newFSTestRouter(t, upstream.URL)

	// 列表
	w := httptest.NewRecorder()
	r.ServeHTTP(w, asAgent(httptest.NewRequest(http.MethodGet, "/api/v1/cc/fs/voicemails?ext=2001", nil)))
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), `"file":"msg_a.wav"`) {
		t.Fatalf("list: code=%d body=%s", w.Code, w.Body.String())
	}

	// 非法 ext 本层拦截
	w = httptest.NewRecorder()
	r.ServeHTTP(w, asAgent(httptest.NewRequest(http.MethodGet, "/api/v1/cc/fs/voicemails?ext=abc", nil)))
	if w.Code != http.StatusBadRequest {
		t.Fatalf("bad ext: code=%d", w.Code)
	}

	// 流式播放
	w = httptest.NewRecorder()
	r.ServeHTTP(w, asAgent(httptest.NewRequest(http.MethodGet, "/api/v1/cc/fs/voicemails/2001/msg_a.wav", nil)))
	if w.Code != http.StatusOK || w.Header().Get("Content-Type") != "audio/wav" || !strings.Contains(w.Body.String(), "RIFF") {
		t.Fatalf("stream: code=%d ct=%s", w.Code, w.Header().Get("Content-Type"))
	}

	// 删除
	w = httptest.NewRecorder()
	r.ServeHTTP(w, asAgent(httptest.NewRequest(http.MethodDelete, "/api/v1/cc/fs/voicemails/2001/msg_a.wav", nil)))
	if w.Code != http.StatusOK {
		t.Fatalf("delete: code=%d body=%s", w.Code, w.Body.String())
	}

	// 路径穿越 / 非法名被挡
	for _, target := range []string{
		"/api/v1/cc/fs/voicemails/2001/..%2Fsecret.wav",
		"/api/v1/cc/fs/voicemails/xx/msg_a.wav",
	} {
		w = httptest.NewRecorder()
		r.ServeHTTP(w, asAgent(httptest.NewRequest(http.MethodGet, target, nil)))
		if w.Code != http.StatusBadRequest && w.Code != http.StatusNotFound {
			t.Fatalf("%s: code=%d", target, w.Code)
		}
	}
}

func TestFSRegistrationsProxy(t *testing.T) {
	upstream := fakeControlAPI(t)
	defer upstream.Close()
	r := newFSTestRouter(t, upstream.URL)

	w := httptest.NewRecorder()
	r.ServeHTTP(w, asAgent(httptest.NewRequest(http.MethodGet, "/api/v1/cc/fs/registrations", nil)))
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), `"reg_user":"1000"`) {
		t.Fatalf("code=%d body=%s", w.Code, w.Body.String())
	}

	// 未认证 → 401
	w = httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/v1/cc/fs/registrations", nil))
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("unauthenticated: code=%d", w.Code)
	}
}

func TestFSOriginateProxy(t *testing.T) {
	upstream := fakeControlAPI(t)
	defer upstream.Close()
	r := newFSTestRouter(t, upstream.URL)

	w := httptest.NewRecorder()
	req := asAgent(httptest.NewRequest(http.MethodPost, "/api/v1/cc/fs/originate",
		strings.NewReader(`{"from_ext":"1000","to":"9196"}`)))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), "Job-UUID") {
		t.Fatalf("code=%d body=%s", w.Code, w.Body.String())
	}

	// 非法号码在本层就挡掉
	w = httptest.NewRecorder()
	req = asAgent(httptest.NewRequest(http.MethodPost, "/api/v1/cc/fs/originate",
		strings.NewReader(`{"from_ext":"1000","to":"9196 XML public"}`)))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("invalid to: code=%d", w.Code)
	}
}

func TestFSHangupProxy(t *testing.T) {
	upstream := fakeControlAPI(t)
	defer upstream.Close()
	r := newFSTestRouter(t, upstream.URL)

	w := httptest.NewRecorder()
	req := asAgent(httptest.NewRequest(http.MethodPost, "/api/v1/cc/fs/hangup",
		strings.NewReader(`{"call_id":"fee34799-dc2c-4b04-94aa-4aeec9891d3c"}`)))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("code=%d body=%s", w.Code, w.Body.String())
	}

	w = httptest.NewRecorder()
	req = asAgent(httptest.NewRequest(http.MethodPost, "/api/v1/cc/fs/hangup",
		strings.NewReader(`{"call_id":"x; hupall"}`)))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("invalid call_id: code=%d", w.Code)
	}
}

func TestFSProxyDisabled(t *testing.T) {
	r := newFSTestRouter(t, "") // CC_FS_API_BASE 未配置
	w := httptest.NewRecorder()
	r.ServeHTTP(w, asAgent(httptest.NewRequest(http.MethodGet, "/api/v1/cc/fs/channels", nil)))
	if w.Code != http.StatusBadRequest {
		t.Fatalf("disabled: code=%d body=%s", w.Code, w.Body.String())
	}
}

// ---- 越权收口 ----

// 全平台运维端点（注册/通道/分机/录音清单、分机增删）：普通租户 403，平台租户放行。
func TestFSAdminGatedEndpointsRejectNonPlatformTenant(t *testing.T) {
	upstream := fakeControlAPI(t)
	defer upstream.Close()
	r := newFSTestRouter(t, upstream.URL)

	targets := []struct {
		method, path, body string
	}{
		{http.MethodGet, "/api/v1/cc/fs/registrations", ""},
		{http.MethodGet, "/api/v1/cc/fs/channels", ""},
		{http.MethodGet, "/api/v1/cc/fs/extensions", ""},
		{http.MethodPost, "/api/v1/cc/fs/extensions", `{"ext":"2000","password":"abcd1234"}`},
		{http.MethodDelete, "/api/v1/cc/fs/extensions/2000", ""},
		{http.MethodGet, "/api/v1/cc/fs/recordings", ""},
		{http.MethodGet, "/api/v1/cc/fs/voicemails", ""}, // 不带 ext = 全平台清单
	}
	for _, tc := range targets {
		var body io.Reader
		if tc.body != "" {
			body = strings.NewReader(tc.body)
		}
		w := httptest.NewRecorder()
		req := asTenantAgent(httptest.NewRequest(tc.method, tc.path, body), "2")
		if tc.body != "" {
			req.Header.Set("Content-Type", "application/json")
		}
		r.ServeHTTP(w, req)
		if w.Code != http.StatusForbidden {
			t.Fatalf("%s %s: tenant-2 code=%d, want 403 (body=%s)", tc.method, tc.path, w.Code, w.Body.String())
		}
	}
}

// FSHangup：普通租户强拆非本租户通话被拒；本租户通话放行。
func TestFSHangupTenantOwnership(t *testing.T) {
	upstream := fakeControlAPI(t)
	defer upstream.Close()
	r := newFSTestRouter(t, upstream.URL)
	srv := fsTestSrv // newFSTestRouter 装配的 Server（用于种数据）

	const callT2 = "fee34799-dc2c-4b04-94aa-4aeec9891d3c"
	if err := srv.Store.DB().Create(&store.Call{TenantID: 2, CallID: callT2, Status: "answered"}).Error; err != nil {
		t.Fatalf("seed call: %v", err)
	}

	post := func(tenant, callID string) *httptest.ResponseRecorder {
		w := httptest.NewRecorder()
		req := asTenantAgent(httptest.NewRequest(http.MethodPost, "/api/v1/cc/fs/hangup",
			strings.NewReader(`{"call_id":"`+callID+`"}`)), tenant)
		req.Header.Set("Content-Type", "application/json")
		r.ServeHTTP(w, req)
		return w
	}

	// 租户 3 拆租户 2 的通话 → 403
	if w := post("3", callT2); w.Code != http.StatusForbidden {
		t.Fatalf("cross-tenant hangup: code=%d body=%s", w.Code, w.Body.String())
	}
	// 租户 2 拆自己的通话 → 放行
	if w := post("2", callT2); w.Code != http.StatusOK {
		t.Fatalf("own hangup: code=%d body=%s", w.Code, w.Body.String())
	}
	// 平台租户拆任意通话 → 放行
	w := httptest.NewRecorder()
	req := asAgent(httptest.NewRequest(http.MethodPost, "/api/v1/cc/fs/hangup",
		strings.NewReader(`{"call_id":"`+callT2+`"}`)))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("platform hangup: code=%d body=%s", w.Code, w.Body.String())
	}
}

// 录音下载：普通租户只能下本租户话单里的录音文件。
func TestFSRecordingFileTenantOwnership(t *testing.T) {
	upstream := fakeControlAPI(t)
	defer upstream.Close()
	r := newFSTestRouter(t, upstream.URL)
	srv := fsTestSrv

	if err := srv.Store.DB().Create(&store.Call{
		TenantID: 2, CallID: "rec-own-1", Status: "hangup", Recording: "/var/lib/freeswitch/recordings/abc.wav",
	}).Error; err != nil {
		t.Fatalf("seed call: %v", err)
	}

	// 本租户话单里的录音 → 放行
	w := httptest.NewRecorder()
	r.ServeHTTP(w, asTenantAgent(httptest.NewRequest(http.MethodGet, "/api/v1/cc/fs/recordings/abc.wav", nil), "2"))
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), "RIFF") {
		t.Fatalf("own recording: code=%d", w.Code)
	}
	// 他租户 → 403
	w = httptest.NewRecorder()
	r.ServeHTTP(w, asTenantAgent(httptest.NewRequest(http.MethodGet, "/api/v1/cc/fs/recordings/abc.wav", nil), "3"))
	if w.Code != http.StatusForbidden {
		t.Fatalf("cross-tenant recording: code=%d body=%s", w.Code, w.Body.String())
	}
	// 平台租户 → 放行
	w = httptest.NewRecorder()
	r.ServeHTTP(w, asAgent(httptest.NewRequest(http.MethodGet, "/api/v1/cc/fs/recordings/abc.wav", nil)))
	if w.Code != http.StatusOK {
		t.Fatalf("platform recording: code=%d", w.Code)
	}
}

// 留言：按分机→cc_agents 租户绑定校验（列举/播放/删除）。
func TestFSVoicemailTenantOwnership(t *testing.T) {
	upstream := fakeControlAPI(t)
	defer upstream.Close()
	r := newFSTestRouter(t, upstream.URL)
	srv := fsTestSrv

	// 分机 2001 绑定在租户 2
	if err := srv.Store.DB().Create(&store.Agent{TenantID: 2, UserID: 9, Extension: "2001"}).Error; err != nil {
		t.Fatalf("seed agent: %v", err)
	}

	// 本租户查/播/删 → 放行
	for _, tc := range []struct{ method, path string }{
		{http.MethodGet, "/api/v1/cc/fs/voicemails?ext=2001"},
		{http.MethodGet, "/api/v1/cc/fs/voicemails/2001/msg_a.wav"},
		{http.MethodDelete, "/api/v1/cc/fs/voicemails/2001/msg_a.wav"},
	} {
		w := httptest.NewRecorder()
		r.ServeHTTP(w, asTenantAgent(httptest.NewRequest(tc.method, tc.path, nil), "2"))
		if w.Code != http.StatusOK {
			t.Fatalf("%s %s tenant-2: code=%d body=%s", tc.method, tc.path, w.Code, w.Body.String())
		}
	}
	// 他租户 → 403
	for _, tc := range []struct{ method, path string }{
		{http.MethodGet, "/api/v1/cc/fs/voicemails?ext=2001"},
		{http.MethodGet, "/api/v1/cc/fs/voicemails/2001/msg_a.wav"},
		{http.MethodDelete, "/api/v1/cc/fs/voicemails/2001/msg_a.wav"},
	} {
		w := httptest.NewRecorder()
		r.ServeHTTP(w, asTenantAgent(httptest.NewRequest(tc.method, tc.path, nil), "3"))
		if w.Code != http.StatusForbidden {
			t.Fatalf("%s %s tenant-3: code=%d, want 403", tc.method, tc.path, w.Code)
		}
	}
}

// 外呼：普通租户不能冒用未绑定到本租户的分机发起。
func TestFSOriginateExtensionOwnership(t *testing.T) {
	upstream := fakeControlAPI(t)
	defer upstream.Close()
	r := newFSTestRouter(t, upstream.URL)
	srv := fsTestSrv

	if err := srv.Store.DB().Create(&store.Agent{TenantID: 2, UserID: 9, Extension: "1000"}).Error; err != nil {
		t.Fatalf("seed agent: %v", err)
	}

	post := func(tenant string) *httptest.ResponseRecorder {
		w := httptest.NewRecorder()
		req := asTenantAgent(httptest.NewRequest(http.MethodPost, "/api/v1/cc/fs/originate",
			strings.NewReader(`{"from_ext":"1000","to":"9196"}`)), tenant)
		req.Header.Set("Content-Type", "application/json")
		r.ServeHTTP(w, req)
		return w
	}
	if w := post("2"); w.Code != http.StatusOK {
		t.Fatalf("own ext originate: code=%d body=%s", w.Code, w.Body.String())
	}
	if w := post("3"); w.Code != http.StatusForbidden {
		t.Fatalf("cross-tenant ext originate: code=%d, want 403", w.Code)
	}
}
