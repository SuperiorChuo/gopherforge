package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/go-admin-kit/services/visibility/internal/model"
	"github.com/go-admin-kit/services/visibility/internal/runner"
	"gorm.io/gorm"
)

func TestAdviceHTTPPreservesPreviousDataOnFailure(t *testing.T) {
	for _, stage := range []string{"malformed", "prefixed malformed", "inline malformed", "scalar", "write failure", "success"} {
		t.Run(stage, func(t *testing.T) {
			s, r := coreServer(t)
			var hits atomic.Int32
			ai := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
				hits.Add(1)
				if req.URL.Path != "/v1/chat/completions" || req.Header.Get("Authorization") != "Bearer test-key" {
					t.Errorf("unexpected local AI request: %s", req.URL.Path)
				}
				content := `{"markdown":"new advice","actions":[{"id":"a1","title":"new action","priority":"HIGH"}]}`
				if stage == "malformed" {
					content = `{"markdown":false}`
				} else if stage == "prefixed malformed" {
					content = "建议如下：\n{\"markdown\":\"truncated"
				} else if stage == "inline malformed" {
					content = `建议如下：{ "markdown": "truncated`
				} else if stage == "scalar" {
					content = "null"
				}
				_ = json.NewEncoder(w).Encode(map[string]any{"choices": []any{map[string]any{"message": map[string]string{"content": content}}}})
			}))
			defer ai.Close()
			s.Runner.Shutdown()
			s.Runner = runner.New(s.Store, runner.AIConfig{BaseURL: ai.URL, APIKey: "test-key", Model: "test-model"}, nil, 1, nil)
			defer s.Runner.Shutdown()
			run := &model.GeoRun{TenantID: 1, Status: "done", Advice: "previous", AdviceItemsJSON: `[{"id":"old","title":"old","done":true}]`}
			if err := s.Store.CreateGeoRun(run); err != nil {
				t.Fatal(err)
			}
			if err := s.Store.CreateGeoResult(&model.GeoResult{TenantID: 1, RunID: run.ID, Question: "q", Engine: "selfhost"}); err != nil {
				t.Fatal(err)
			}
			path := fmt.Sprintf("/geo/runs/%d/advice", run.ID)
			coreRequest(t, r, "POST", path, nil, 2, "*", 502)
			if hits.Load() != 0 {
				t.Fatal("foreign run reached AI")
			}
			writes := 0
			if stage == "write failure" {
				if err := s.Store.DB().Callback().Update().Before("gorm:update").Register("test:advice_write", func(tx *gorm.DB) {
					if tx.Statement.Table == "vis_geo_runs" {
						writes++
						tx.AddError(errors.New("advice unavailable"))
					}
				}); err != nil {
					t.Fatal(err)
				}
			}
			want := 502
			if stage == "success" {
				want = 200
			}
			coreRequest(t, r, "POST", path, nil, 1, "visibility:write", want)
			if hits.Load() != 1 {
				t.Fatalf("AI hits=%d", hits.Load())
			}
			got, err := s.Store.GetGeoRun(1, run.ID)
			if err != nil {
				t.Fatal(err)
			}
			if stage != "success" {
				if got.Advice != run.Advice || got.AdviceItemsJSON != run.AdviceItemsJSON {
					t.Fatalf("failed advice replaced previous data: %+v", got)
				}
			} else {
				if got.Advice != "new advice" {
					t.Fatalf("advice=%+v", got)
				}
				var items []model.AdviceItem
				if err := json.Unmarshal([]byte(got.AdviceItemsJSON), &items); err != nil {
					t.Fatal(err)
				}
				if len(items) != 1 || items[0].ID != "a1" || items[0].Priority != "high" {
					t.Fatalf("items=%+v", items)
				}
			}
			if stage == "write failure" && writes != 1 {
				t.Fatalf("write failure not exercised: %d", writes)
			}
		})
	}
}

func TestReportComparisonAndChecklistRemainTenantScoped(t *testing.T) {
	s, r := coreServer(t)
	for _, run := range []model.GeoRun{{ID: 1, TenantID: 1, Status: "done"}, {ID: 2, TenantID: 1, Status: "done"}, {ID: 3, TenantID: 2, Status: "done"}, {ID: 4, TenantID: 1, Status: "running"}} {
		if err := s.Store.CreateGeoRun(&run); err != nil {
			t.Fatal(err)
		}
	}
	for _, res := range []model.GeoResult{
		{TenantID: 1, RunID: 1, Question: "shared", Engine: "selfhost"},
		{TenantID: 1, RunID: 2, Question: "shared", Engine: "selfhost", MentionBrand: true},
		{TenantID: 1, RunID: 2, Question: "fallback", Engine: "llm_fallback_stub", MentionBrand: true},
		{TenantID: 2, RunID: 2, Question: "foreign row", Engine: "selfhost", MentionBrand: true},
	} {
		if err := s.Store.CreateGeoResult(&res); err != nil {
			t.Fatal(err)
		}
	}
	coreRequest(t, r, "GET", "/geo/runs/2", nil, 2, "*", 404)
	coreRequest(t, r, "GET", "/geo/runs/2/compare?with=3", nil, 1, "*", 404)
	coreRequest(t, r, "GET", "/geo/runs/2/compare?with=2", nil, 1, "*", 400)
	coreRequest(t, r, "GET", "/geo/runs/2/compare?with=4", nil, 1, "*", 400)
	coreRequest(t, r, "GET", "/geo/runs/4/compare", nil, 1, "*", 400)
	data := coreRequest(t, r, "GET", "/geo/runs/1/compare", nil, 1, "*", 200)
	var none struct {
		HasPrevious bool `json:"has_previous"`
	}
	if err := json.Unmarshal(data, &none); err != nil {
		t.Fatal(err)
	}
	if none.HasPrevious {
		t.Fatalf("unexpected previous=%s", data)
	}
	data = coreRequest(t, r, "GET", "/geo/runs/2/compare", nil, 1, "*", 200)
	var comparison struct {
		HasPrevious bool `json:"has_previous"`
		Compare     struct {
			Rate    int      `json:"current_rate"`
			Prompts int      `json:"current_prompts"`
			New     []string `json:"newly_mentioned"`
		} `json:"compare"`
	}
	if err := json.Unmarshal(data, &comparison); err != nil {
		t.Fatal(err)
	}
	if !comparison.HasPrevious || comparison.Compare.Rate != 100 || comparison.Compare.Prompts != 1 || len(comparison.Compare.New) != 1 {
		t.Fatalf("comparison=%s", data)
	}
	data = coreRequest(t, r, "GET", "/geo/runs/2", nil, 1, "*", 200)
	var detail struct{ Results []model.GeoResult }
	if err := json.Unmarshal(data, &detail); err != nil {
		t.Fatal(err)
	}
	if len(detail.Results) != 2 {
		t.Fatalf("tenant results=%s", data)
	}
	body := `{"items":[{"id":"1","title":"Fix","done":true}]}`
	coreRequest(t, r, "PUT", "/geo/runs/2/advice-items", body, 2, "*", 404)
	coreRequest(t, r, "PUT", "/geo/runs/2/advice-items", `{"items":[]}`, 1, "*", 400)
	coreRequest(t, r, "PUT", "/geo/runs/2/advice-items", body, 1, "visibility:write", 200)
	got, err := s.Store.GetGeoRun(1, 2)
	if err != nil {
		t.Fatal(err)
	}
	var items []model.AdviceItem
	if err := json.Unmarshal([]byte(got.AdviceItemsJSON), &items); err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 || !items[0].Done {
		t.Fatalf("checklist=%+v", items)
	}
}

func TestGenerationRejectsMalformedInputBeforeAI(t *testing.T) {
	s, r := coreServer(t)
	var hits atomic.Int32
	ai := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) { hits.Add(1); http.Error(w, "must not call AI", 500) }))
	defer ai.Close()
	s.Runner.Shutdown()
	s.Runner = runner.New(s.Store, runner.AIConfig{BaseURL: ai.URL, APIKey: "test-key", Model: "test-model"}, nil, 1, nil)
	defer s.Runner.Shutdown()
	for _, path := range []string{"/brands/generate", "/keywords/generate", "/geo/prompts/generate"} {
		coreRequest(t, r, "POST", path, `{"hint":42}`, 1, "*", 400)
	}
	if hits.Load() != 0 {
		t.Fatalf("malformed request called AI: %d", hits.Load())
	}
}
