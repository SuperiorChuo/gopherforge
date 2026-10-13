package api

import (
	"encoding/json"
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

func newRiskRouter(t *testing.T, fsBase string) (*gin.Engine, *Server) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(
		&store.Call{}, &store.Agent{},
		&store.CCRate{}, &store.CCBillingAccount{},
		&store.CCRiskPolicy{}, &store.CCRiskListEntry{}, &store.CCRiskAttempt{},
	); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	srv := &Server{Store: store.NewWithDB(db), FS: fsclient.New(fsBase, "test-token"), Secret: "test-jwt-secret"}
	r := gin.New()
	r.GET("/api/v1/cc/risk/policy", srv.GetRiskPolicy)
	r.PUT("/api/v1/cc/risk/policy", srv.SaveRiskPolicy)
	r.GET("/api/v1/cc/risk/lists", srv.ListRiskLists)
	r.POST("/api/v1/cc/risk/lists", srv.CreateRiskList)
	r.DELETE("/api/v1/cc/risk/lists/:id", srv.DeleteRiskList)
	r.POST("/api/v1/cc/fs/originate", srv.FSOriginate)
	return r, srv
}

func TestRiskPolicyGetCreatesDefaultAndSave(t *testing.T) {
	r, _ := newRiskRouter(t, "")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, asAgent(httptest.NewRequest(http.MethodGet, "/api/v1/cc/risk/policy", nil)))
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), `"max_rate_per_min":0`) {
		t.Fatalf("get default: code=%d body=%s", w.Code, w.Body.String())
	}

	w = httptest.NewRecorder()
	req := asAgent(httptest.NewRequest(http.MethodPut, "/api/v1/cc/risk/policy",
		strings.NewReader(`{"outbound_start":"09:00","outbound_end":"18:00","outbound_weekdays":"1,2,3,4,5","max_calls_per_number_per_hour":5,"blocked_prefixes":"00, 166","max_rate_per_min":120}`)))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), `"blocked_prefixes":"00,166"`) {
		t.Fatalf("save: code=%d body=%s", w.Code, w.Body.String())
	}

	w = httptest.NewRecorder()
	req = asAgent(httptest.NewRequest(http.MethodPut, "/api/v1/cc/risk/policy",
		strings.NewReader(`{"outbound_start":"9:00"}`)))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("bad time: code=%d", w.Code)
	}
}

func TestRiskListCRUDAndConflict(t *testing.T) {
	r, _ := newRiskRouter(t, "")
	post := func(body string) *httptest.ResponseRecorder {
		w := httptest.NewRecorder()
		req := asAgent(httptest.NewRequest(http.MethodPost, "/api/v1/cc/risk/lists", strings.NewReader(body)))
		req.Header.Set("Content-Type", "application/json")
		r.ServeHTTP(w, req)
		return w
	}
	w := post(`{"number":"13800138000","match":"exact","list_type":"black","remark":"投诉"}`)
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), `"id":`) {
		t.Fatalf("create: code=%d body=%s", w.Code, w.Body.String())
	}
	var env struct {
		Data store.CCRiskListEntry `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &env); err != nil || env.Data.ID == 0 {
		t.Fatalf("decode: %v body=%s", err, w.Body.String())
	}
	if w := post(`{"number":"13800138000","match":"exact","list_type":"black"}`); w.Code != http.StatusConflict {
		t.Fatalf("dup: code=%d body=%s", w.Code, w.Body.String())
	}
	if w := post(`{"number":"bad number","match":"exact","list_type":"black"}`); w.Code != http.StatusBadRequest {
		t.Fatalf("bad number: code=%d", w.Code)
	}
	if w := post(`{"number":"00","match":"prefix","list_type":"black"}`); w.Code != http.StatusOK {
		t.Fatalf("prefix 00: code=%d body=%s", w.Code, w.Body.String())
	}

	w = httptest.NewRecorder()
	r.ServeHTTP(w, asAgent(httptest.NewRequest(http.MethodGet, "/api/v1/cc/risk/lists", nil)))
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), `"total":2`) {
		t.Fatalf("list: code=%d body=%s", w.Code, w.Body.String())
	}

	w = httptest.NewRecorder()
	r.ServeHTTP(w, asAgent(httptest.NewRequest(http.MethodDelete, "/api/v1/cc/risk/lists/"+strconvID(env.Data.ID), nil)))
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), `"deleted":true`) {
		t.Fatalf("delete: code=%d body=%s", w.Code, w.Body.String())
	}
}

func strconvID(id uint64) string {
	return strings.TrimLeft(strings.ReplaceAll(jsonNumber(id), `"`, ""), " ")
}

func jsonNumber(id uint64) string {
	b, _ := json.Marshal(id)
	return string(b)
}

func TestOriginateDeniedByBlacklistDoesNotHitFS(t *testing.T) {
	hits := 0
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits++
		w.WriteHeader(http.StatusInternalServerError)
	}))
	t.Cleanup(upstream.Close)
	r, _ := newRiskRouter(t, upstream.URL)

	req := asAgent(httptest.NewRequest(http.MethodPost, "/api/v1/cc/risk/lists",
		strings.NewReader(`{"number":"9196","match":"exact","list_type":"black"}`)))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("seed list: %d %s", w.Code, w.Body.String())
	}

	w = httptest.NewRecorder()
	req = asAgent(httptest.NewRequest(http.MethodPost, "/api/v1/cc/fs/originate",
		strings.NewReader(`{"from_ext":"1000","to":"9196"}`)))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)
	if w.Code != http.StatusForbidden || !strings.Contains(w.Body.String(), "外呼黑名单") {
		t.Fatalf("originate deny: code=%d body=%s", w.Code, w.Body.String())
	}
	if hits != 0 {
		t.Fatalf("control-api hits = %d, want 0", hits)
	}
}

func TestOriginateFrequencySecondCall429(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/calls/originate" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"queued":true,"reply":"+OK Job-UUID: j1"}`))
	}))
	t.Cleanup(upstream.Close)
	r, _ := newRiskRouter(t, upstream.URL)

	req := asAgent(httptest.NewRequest(http.MethodPut, "/api/v1/cc/risk/policy",
		strings.NewReader(`{"max_calls_per_number_per_hour":1}`)))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("save policy: %d %s", w.Code, w.Body.String())
	}

	originate := func() *httptest.ResponseRecorder {
		w := httptest.NewRecorder()
		req := asAgent(httptest.NewRequest(http.MethodPost, "/api/v1/cc/fs/originate",
			strings.NewReader(`{"from_ext":"1000","to":"9196"}`)))
		req.Header.Set("Content-Type", "application/json")
		r.ServeHTTP(w, req)
		return w
	}
	if w := originate(); w.Code != http.StatusOK {
		t.Fatalf("first originate: code=%d body=%s", w.Code, w.Body.String())
	}
	if w := originate(); w.Code != http.StatusTooManyRequests {
		t.Fatalf("second originate: code=%d body=%s", w.Code, w.Body.String())
	}
}
