package runner

import (
	"bytes"
	"log"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/go-admin-kit/services/visibility/internal/model"
)

type executionFailureStore struct {
	*persistenceStore
	resultCalls, results int
	hook                 string
	hookReads            atomic.Int32
}

func (s *executionFailureStore) CreateGeoResult(result *model.GeoResult) error {
	s.resultCalls++
	if s.hook == "double failure" && s.resultCalls == 2 {
		return persistenceFailure
	}
	s.results++
	return s.persistenceStore.CreateGeoResult(result)
}
func (s *executionFailureStore) GetGeoRun(tenant, id uint64) (*model.GeoRun, error) {
	s.hookReads.Add(1)
	if s.hook == "panic" {
		panic("advice hook panic")
	}
	return nil, persistenceFailure
}

func TestGeoPartialResultsAndFailedStateWriteRemainVisible(t *testing.T) {
	server := fakeLLMServer(t)
	defer server.Close()
	st := &executionFailureStore{persistenceStore: &persistenceStore{fakeStore: newFakeStore(), stage: "failure"}, hook: "double failure"}
	run := &model.GeoRun{TenantID: 1, Status: "running"}
	if err := st.CreateGeoRun(run); err != nil {
		t.Fatal(err)
	}
	cfg := AIConfig{BaseURL: server.URL, APIKey: "test-key", Model: "test-model"}
	r := New(st, cfg, nil, 1, nil)
	var output bytes.Buffer
	old := log.Writer()
	log.SetOutput(&output)
	defer log.SetOutput(old)
	r.geoActive[1] = run.ID
	r.wg.Add(1)
	r.execGeoRun(run.ID, 1, "llm", "selfhost", cfg, []model.GeoPrompt{{ID: 1, Question: "q1"}, {ID: 2, Question: "q2"}}, nil)
	r.Shutdown()
	got, err := st.fakeStore.GetGeoRun(1, run.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != "running" || st.results != 1 || st.resultCalls != 2 || st.hits != 1 || st.hookReads.Load() != 0 || len(r.geoActive) != 0 || !strings.Contains(output.String(), "persist failure state") {
		t.Fatalf("double failure hidden: status=%s results=%d attempts=%d failedWrites=%d hooks=%d active=%d log=%s", got.Status, st.results, st.resultCalls, st.hits, st.hookReads.Load(), len(r.geoActive), output.String())
	}
	if len(st.geoUpdates) != 1 || st.geoUpdates[0]["done_count"] != 1 {
		t.Fatalf("partial progress falsely advanced: %v", st.geoUpdates)
	}
}

func TestAfterHookFailuresNeverUndoCompletedRun(t *testing.T) {
	for _, stage := range []string{"read error", "panic"} {
		t.Run(stage, func(t *testing.T) {
			server := fakeLLMServer(t)
			defer server.Close()
			st := &executionFailureStore{persistenceStore: &persistenceStore{fakeStore: newFakeStore()}, hook: stage}
			run := &model.GeoRun{TenantID: 1, Status: "running"}
			if err := st.CreateGeoRun(run); err != nil {
				t.Fatal(err)
			}
			cfg := AIConfig{BaseURL: server.URL, APIKey: "test-key", Model: "test-model"}
			r := New(st, cfg, nil, 1, nil)
			var output bytes.Buffer
			old := log.Writer()
			log.SetOutput(&output)
			defer log.SetOutput(old)
			r.geoActive[1] = run.ID
			r.wg.Add(1)
			r.execGeoRun(run.ID, 1, "llm", "selfhost", cfg, []model.GeoPrompt{{ID: 1, Question: "q"}}, nil)
			r.Shutdown()
			got, err := st.fakeStore.GetGeoRun(1, run.ID)
			if err != nil {
				t.Fatal(err)
			}
			marker := "auto advice"
			if stage == "panic" {
				marker = "after-hook panic"
			}
			if got.Status != "done" || st.hookReads.Load() != 1 || len(r.geoActive) != 0 || !strings.Contains(output.String(), marker) {
				t.Fatalf("hook changed completed task: status=%s hooks=%d active=%d log=%s", got.Status, st.hookReads.Load(), len(r.geoActive), output.String())
			}
		})
	}
}
