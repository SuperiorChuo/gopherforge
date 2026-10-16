package api

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/go-admin-kit/services/visibility/internal/model"
	"github.com/go-admin-kit/services/visibility/internal/runner"
	store "github.com/go-admin-kit/services/visibility/internal/store/visibility"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func coreServer(t *testing.T) (*Server, *gin.Engine) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "visibility.db")), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatal(err)
	}
	conn, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	conn.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = conn.Close() })
	st, err := store.NewWithDB(db)
	if err != nil {
		t.Fatal(err)
	}
	run := runner.New(st, runner.AIConfig{}, nil, 1, nil)
	t.Cleanup(run.Shutdown)
	s := &Server{Store: st, Runner: run, Secret: "local-test"}
	r := gin.New()
	s.RegisterRoutes(r)
	return s, r
}

func coreRequest(t *testing.T, r http.Handler, method, path string, body any, tenant uint64, permission string, want int) json.RawMessage {
	t.Helper()
	var raw []byte
	if v, ok := body.(string); ok {
		raw = []byte(v)
	} else if body != nil {
		var err error
		raw, err = json.Marshal(body)
		if err != nil {
			t.Fatal(err)
		}
	}
	req := httptest.NewRequest(method, "/api/v1/visibility"+path, bytes.NewReader(raw))
	req.Header.Set("Content-Type", "application/json")
	if tenant > 0 {
		req.Header.Set("X-Auth-User-ID", "10")
		req.Header.Set("X-Auth-Tenant-ID", fmt.Sprint(tenant))
		req.Header.Set("X-Auth-Permissions", permission)
	}
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != want {
		t.Fatalf("%s %s: status=%d want=%d body=%s", method, path, w.Code, want, w.Body.String())
	}
	var envelope struct {
		Code int             `json:"code"`
		Data json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &envelope); err != nil {
		t.Fatal(err)
	}
	if envelope.Code != want {
		t.Fatalf("envelope=%s", w.Body.String())
	}
	return envelope.Data
}

func TestCoreRoutesPermissionsAndTenantCRUD(t *testing.T) {
	for _, tc := range []struct {
		path, field   string
		input, update any
	}{
		{"brands", "name", map[string]any{"name": " Alpha ", "kind": "brand", "aliases": "A"}, map[string]any{"name": "Beta", "status": 0}},
		{"keywords", "keyword", map[string]any{"keyword": " Alpha ", "rank": 3}, map[string]any{"keyword": "Beta", "rank": 0, "status": "paused"}},
		{"geo/prompts", "question", map[string]any{"question": " Alpha "}, map[string]any{"question": "Beta", "status": 0}},
	} {
		t.Run(tc.path, func(t *testing.T) {
			_, r := coreServer(t)
			path := "/" + tc.path
			coreRequest(t, r, "GET", path, nil, 0, "", 401)
			coreRequest(t, r, "POST", path, tc.input, 1, "visibility:read", 403)
			coreRequest(t, r, "POST", path, "{", 1, "visibility:write", 400)
			data := coreRequest(t, r, "POST", path, tc.input, 1, "visibility:write", 200)
			var created struct {
				ID       uint64
				TenantID uint64 `json:"tenant_id"`
			}
			if err := json.Unmarshal(data, &created); err != nil {
				t.Fatal(err)
			}
			if created.ID == 0 || created.TenantID != 1 {
				t.Fatalf("created=%s", data)
			}
			item := fmt.Sprintf("%s/%d", path, created.ID)
			coreRequest(t, r, "PUT", item, tc.update, 2, "*", 404)
			coreRequest(t, r, "DELETE", item, nil, 2, "*", 404)
			foreign := coreRequest(t, r, "GET", path, nil, 2, "visibility:read", 200)
			var list struct {
				Total int
				List  []map[string]any
			}
			if err := json.Unmarshal(foreign, &list); err != nil {
				t.Fatal(err)
			}
			if list.Total != 0 || len(list.List) != 0 {
				t.Fatalf("tenant leak=%s", foreign)
			}
			coreRequest(t, r, "PUT", path+"/bad", tc.update, 1, "visibility:write", 400)
			coreRequest(t, r, "PUT", item, tc.update, 1, "visibility:write", 200)
			data = coreRequest(t, r, "GET", path+"?page=1&page_size=1", nil, 1, "visibility:read", 200)
			if err := json.Unmarshal(data, &list); err != nil {
				t.Fatal(err)
			}
			if list.Total != 1 || len(list.List) != 1 || list.List[0][tc.field] != "Beta" {
				t.Fatalf("update/list=%s", data)
			}
			coreRequest(t, r, "DELETE", item, nil, 1, "visibility:write", 200)
			coreRequest(t, r, "DELETE", item, nil, 1, "visibility:write", 404)
		})
	}
}

func TestBatchWritesRollBackAndDeduplicate(t *testing.T) {
	for _, tc := range []struct{ path, table, field string }{
		{"brands", "vis_brands", "name"}, {"keywords", "vis_keywords", "keyword"}, {"geo/prompts", "vis_geo_prompts", "question"},
	} {
		t.Run(tc.path, func(t *testing.T) {
			s, r := coreServer(t)
			hits := 0
			sentinel := errors.New("injected second batch row failure")
			if err := s.Store.DB().Callback().Create().Before("gorm:create").Register("test:batch_failure", func(tx *gorm.DB) {
				if tx.Statement.Table == tc.table {
					hits++
					if hits == 2 {
						tx.AddError(sentinel)
					}
				}
			}); err != nil {
				t.Fatal(err)
			}
			body := map[string]any{"items": []map[string]any{{tc.field: "Alpha"}, {tc.field: "Beta"}}}
			coreRequest(t, r, "POST", "/"+tc.path+"/batch", body, 1, "visibility:write", 500)
			var count int64
			if err := s.Store.DB().Table(tc.table).Count(&count).Error; err != nil {
				t.Fatal(err)
			}
			if hits != 2 || count != 0 {
				t.Fatalf("partial batch persisted: hits=%d rows=%d", hits, count)
			}
			if err := s.Store.DB().Callback().Create().Remove("test:batch_failure"); err != nil {
				t.Fatal(err)
			}
			coreRequest(t, r, "POST", "/"+tc.path, map[string]any{tc.field: "Alpha"}, 2, "*", 200)
			body = map[string]any{"items": []map[string]any{{tc.field: " Alpha "}, {tc.field: "alpha"}, {tc.field: " "}, {tc.field: "Beta"}}}
			data := coreRequest(t, r, "POST", "/"+tc.path+"/batch", body, 1, "visibility:write", 200)
			var result struct{ Created, Skipped int }
			if err := json.Unmarshal(data, &result); err != nil {
				t.Fatal(err)
			}
			if result.Created != 2 || result.Skipped != 1 {
				t.Fatalf("counts=%s", data)
			}
			data = coreRequest(t, r, "POST", "/"+tc.path+"/batch", body, 1, "visibility:write", 200)
			if err := json.Unmarshal(data, &result); err != nil {
				t.Fatal(err)
			}
			if result.Created != 0 || result.Skipped != 3 {
				t.Fatalf("repeat=%s", data)
			}
			coreRequest(t, r, "POST", "/"+tc.path+"/batch", `{"items":[]}`, 1, "*", 400)
			items := make([]map[string]string, 101)
			for i := range items {
				items[i] = map[string]string{tc.field: "x"}
			}
			coreRequest(t, r, "POST", "/"+tc.path+"/batch", map[string]any{"items": items}, 1, "*", 400)
		})
	}
}

func TestBrandBatchUsesNewAliasesForDeduplication(t *testing.T) {
	_, r := coreServer(t)
	data := coreRequest(t, r, "POST", "/brands/batch", `{"items":[{"name":"Alpha","aliases":" A，别名、Other "},{"name":"other"},{"name":"别名"}]}`, 1, "*", 200)
	var counts struct{ Created, Skipped int }
	if err := json.Unmarshal(data, &counts); err != nil {
		t.Fatal(err)
	}
	if counts.Created != 1 || counts.Skipped != 2 {
		t.Fatalf("new aliases ignored: %s", data)
	}
}

func TestReportReadFailuresAreNotEmptySuccess(t *testing.T) {
	for _, path := range []string{"/geo/runs/2", "/geo/runs/2/compare?with=1"} {
		t.Run(path, func(t *testing.T) {
			s, r := coreServer(t)
			for i := uint64(1); i <= 2; i++ {
				if err := s.Store.CreateGeoRun(&model.GeoRun{ID: i, TenantID: 1, Status: "done"}); err != nil {
					t.Fatal(err)
				}
			}
			hits := 0
			if err := s.Store.DB().Callback().Query().Before("gorm:query").Register("test:results_failure", func(tx *gorm.DB) {
				if tx.Statement.Table == "vis_geo_results" {
					hits++
					tx.AddError(errors.New("results unavailable"))
				}
			}); err != nil {
				t.Fatal(err)
			}
			coreRequest(t, r, "GET", path, nil, 1, "visibility:read", 500)
			if hits != 1 {
				t.Fatalf("read failure hits=%d", hits)
			}
		})
	}
}

func TestMalformedTaskRequestsDoNotStartWork(t *testing.T) {
	s, r := coreServer(t)
	s.DemoMode = true
	if err := s.Store.CreateGeoPrompt(context.Background(), &model.GeoPrompt{TenantID: 1, Question: "q"}); err != nil {
		t.Fatal(err)
	}
	if err := s.Store.CreateKeyword(context.Background(), &model.Keyword{TenantID: 1, Keyword: "q"}); err != nil {
		t.Fatal(err)
	}
	coreRequest(t, r, "POST", "/geo/runs", `{"mode":"stub","note":42}`, 1, "*", 400)
	coreRequest(t, r, "POST", "/serp/checks", `{"keyword_id":"wrong"}`, 1, "*", 400)
	var n int64
	for _, table := range []string{"vis_geo_runs", "vis_serp_tasks"} {
		if err := s.Store.DB().Table(table).Count(&n).Error; err != nil {
			t.Fatal(err)
		}
		if n != 0 {
			t.Fatalf("malformed body started work: %s=%d", table, n)
		}
	}
}
