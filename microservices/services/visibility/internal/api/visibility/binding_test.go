package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/go-admin-kit/services/visibility/internal/model"
	"github.com/go-admin-kit/services/visibility/internal/runner"
	"gorm.io/gorm"
)

func TestTrailingJSONNeverStartsWork(t *testing.T) {
	for _, tail := range []string{" garbage", `{"keyword_id":123}`} {
		t.Run(tail, func(t *testing.T) {
			s, r := coreServer(t)
			s.DemoMode = true
			if err := s.Store.CreateGeoPrompt(context.Background(), &model.GeoPrompt{TenantID: 1, Question: "q"}); err != nil {
				t.Fatal(err)
			}
			if err := s.Store.CreateKeyword(context.Background(), &model.Keyword{TenantID: 1, Keyword: "q"}); err != nil {
				t.Fatal(err)
			}
			var hits atomic.Int32
			ai := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) { hits.Add(1); http.Error(w, "unexpected AI", 500) }))
			defer ai.Close()
			s.Runner.Shutdown()
			s.Runner = runner.New(s.Store, runner.AIConfig{BaseURL: ai.URL, APIKey: "test-key", Model: "test-model"}, nil, 1, nil)
			defer s.Runner.Shutdown()
			for _, tc := range []struct{ path, body string }{{"/geo/runs", `{"mode":"stub"}`}, {"/serp/checks", `{"keyword_id":0}`}, {"/brands/generate", `{}`}, {"/keywords/generate", `{}`}, {"/geo/prompts/generate", `{}`}} {
				coreRequest(t, r, "POST", tc.path, tc.body+tail, 1, "*", 400)
			}
			for _, table := range []string{"vis_geo_runs", "vis_serp_tasks"} {
				var n int64
				if err := s.Store.DB().Table(table).Count(&n).Error; err != nil {
					t.Fatal(err)
				}
				if n != 0 {
					t.Fatalf("trailing JSON started task: %s=%d", table, n)
				}
			}
			if hits.Load() != 0 {
				t.Fatalf("trailing JSON called AI: %d", hits.Load())
			}
		})
	}
}

func TestOptionalBodyPreservesEmptyAndSingleJSON(t *testing.T) {
	for _, body := range []string{"", "   ", "{}", "{} \n\t"} {
		t.Run(body, func(t *testing.T) {
			s, r := coreServer(t)
			var hits atomic.Int32
			if err := s.Store.CreateBrand(context.Background(), &model.Brand{TenantID: 1, Name: "Alpha", Kind: "brand"}); err != nil {
				t.Fatal(err)
			}
			ai := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
				hits.Add(1)
				_ = json.NewEncoder(w).Encode(map[string]any{"choices": []any{map[string]any{"message": map[string]string{"content": `[{"name":"Beta","kind":"competitor"}]`}}}})
			}))
			defer ai.Close()
			s.Runner.Shutdown()
			s.Runner = runner.New(s.Store, runner.AIConfig{BaseURL: ai.URL, APIKey: "test-key", Model: "test-model"}, nil, 1, nil)
			defer s.Runner.Shutdown()
			coreRequest(t, r, "POST", "/brands/generate", body, 1, "*", 200)
			if hits.Load() != 1 {
				t.Fatalf("optional body did not reach local AI: %d", hits.Load())
			}
		})
	}
}

func TestReportMainAndPreviousReadFailuresReturn500(t *testing.T) {
	for _, tc := range []struct {
		name, path, table string
		failAt            int
	}{
		{"detail main", "/geo/runs/2", "vis_geo_runs", 1},
		{"compare main", "/geo/runs/2/compare?with=1", "vis_geo_runs", 1},
		{"compare previous main", "/geo/runs/2/compare?with=1", "vis_geo_runs", 2},
		{"compare previous results", "/geo/runs/2/compare?with=1", "vis_geo_results", 2},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s, r := coreServer(t)
			for i := uint64(1); i <= 2; i++ {
				if err := s.Store.CreateGeoRun(&model.GeoRun{ID: i, TenantID: 1, Status: "done"}); err != nil {
					t.Fatal(err)
				}
			}
			hits := 0
			if err := s.Store.DB().Callback().Query().Before("gorm:query").Register("test:report", func(tx *gorm.DB) {
				if tx.Statement.Table == tc.table {
					hits++
					if hits == tc.failAt {
						tx.AddError(errors.New("injected report read failure"))
					}
				}
			}); err != nil {
				t.Fatal(err)
			}
			coreRequest(t, r, "GET", tc.path, nil, 1, "*", 500)
			if hits != tc.failAt {
				t.Fatalf("wrong failure stage: hits=%d", hits)
			}
		})
	}
}
