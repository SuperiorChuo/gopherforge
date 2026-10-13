package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/go-admin-kit/services/cc/internal/store/cc"
	"gorm.io/gorm"
)

func newQcTestServer(t *testing.T) (*Server, *gin.Engine) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(&store.Call{}, &store.CCTranscript{}, &store.CCQcKeyword{}, &store.CCQcResult{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	srv := &Server{Store: store.NewWithDB(db), Secret: "test-jwt-secret"}
	r := gin.New()
	r.GET("/api/v1/cc/qc/keywords", srv.QcKeywords)
	r.POST("/api/v1/cc/qc/keywords", srv.QcCreateKeyword)
	r.DELETE("/api/v1/cc/qc/keywords/:id", srv.QcDeleteKeyword)
	r.GET("/api/v1/cc/qc/results", srv.QcResults)
	return srv, r
}

func gwReq(method, path, body string) *http.Request {
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Auth-User-ID", "1") // 网关头鉴权
	return req
}

func TestQcKeywordCRUD(t *testing.T) {
	_, r := newQcTestServer(t)

	// 创建违禁词
	w := httptest.NewRecorder()
	r.ServeHTTP(w, gwReq(http.MethodPost, "/api/v1/cc/qc/keywords", `{"word":"投诉","type":"forbidden","remark":"负面"}`))
	if w.Code != http.StatusOK {
		t.Fatalf("create forbidden: %d body=%s", w.Code, w.Body.String())
	}
	// 创建必说词
	w = httptest.NewRecorder()
	r.ServeHTTP(w, gwReq(http.MethodPost, "/api/v1/cc/qc/keywords", `{"word":"您好","type":"required"}`))
	if w.Code != http.StatusOK {
		t.Fatalf("create required: %d body=%s", w.Code, w.Body.String())
	}
	// 重复词冲突
	w = httptest.NewRecorder()
	r.ServeHTTP(w, gwReq(http.MethodPost, "/api/v1/cc/qc/keywords", `{"word":"您好","type":"required"}`))
	if w.Code != http.StatusConflict {
		t.Fatalf("dup keyword: %d want 409", w.Code)
	}
	// 非法类型
	w = httptest.NewRecorder()
	r.ServeHTTP(w, gwReq(http.MethodPost, "/api/v1/cc/qc/keywords", `{"word":"x","type":"bogus"}`))
	if w.Code != http.StatusBadRequest {
		t.Fatalf("bad type: %d want 400", w.Code)
	}

	// 列表应有 2 条
	w = httptest.NewRecorder()
	r.ServeHTTP(w, gwReq(http.MethodGet, "/api/v1/cc/qc/keywords", ""))
	var listResp struct {
		Data struct {
			Total int `json:"total"`
		} `json:"data"`
	}
	_ = json.Unmarshal(w.Body.Bytes(), &listResp)
	if listResp.Data.Total != 2 {
		t.Fatalf("keyword total = %d, want 2; body=%s", listResp.Data.Total, w.Body.String())
	}
}

func TestRunQcScoring(t *testing.T) {
	srv, _ := newQcTestServer(t)
	ctx := t.Context()

	seed := []store.CCQcKeyword{
		{TenantID: 1, Word: "退款", Type: "forbidden"},
		{TenantID: 1, Word: "您好", Type: "required"},
		{TenantID: 1, Word: "感谢来电", Type: "required"},
	}
	for i := range seed {
		if err := srv.Store.CreateKeyword(ctx, &seed[i]); err != nil {
			t.Fatal(err)
		}
	}

	// 文本含 1 次违禁词「退款」(-20)，有「您好」，缺「感谢来电」(-10) → 100-20-10=70
	score, hits, err := srv.runQc(ctx, 1, "call-x", "您好，关于退款问题我们会处理")
	if err != nil {
		t.Fatal(err)
	}
	if score != 70 {
		t.Fatalf("score = %d, want 70; hits=%+v", score, hits)
	}

	// 违禁词命中多次导致负分被夹到 0
	score2, _, err := srv.runQc(ctx, 1, "call-y", "退款退款退款退款退款退款")
	if err != nil {
		t.Fatal(err)
	}
	if score2 != 0 {
		t.Fatalf("score2 = %d, want 0 (floor)", score2)
	}

	// 结果按 call_id 幂等：重跑覆盖不新增
	var cnt int64
	srv.Store.DB().Model(&store.CCQcResult{}).Count(&cnt)
	if cnt != 2 {
		t.Fatalf("qc result rows = %d, want 2 (idempotent by call_id)", cnt)
	}
}
