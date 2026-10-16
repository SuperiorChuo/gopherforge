package runner

import (
	"bytes"
	"log"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/go-admin-kit/services/visibility/internal/model"
)

type afterHookStore struct {
	*persistenceStore
	reads atomic.Int32
}

func (s *afterHookStore) GetGeoRun(t, id uint64) (*model.GeoRun, error) {
	s.reads.Add(1)
	return s.fakeStore.GetGeoRun(t, id)
}

func TestGeoCompletionWriteFailureDoesNotStartAdvice(t *testing.T) {
	server := fakeLLMServer(t)
	defer server.Close()
	st := &afterHookStore{persistenceStore: &persistenceStore{fakeStore: newFakeStore(), stage: "complete"}}
	run := &model.GeoRun{TenantID: 1, Status: "running"}
	if err := st.CreateGeoRun(run); err != nil {
		t.Fatal(err)
	}
	cfg := AIConfig{BaseURL: server.URL, APIKey: "test-key", Model: "test-model"}
	r := New(st, cfg, nil, 1, nil)
	r.wg.Add(1)
	r.execGeoRun(run.ID, 1, "llm", "selfhost", cfg, []model.GeoPrompt{{ID: 1, Question: "q"}}, nil)
	r.Shutdown()
	if st.hits != 1 || st.reads.Load() != 0 {
		t.Fatalf("completion failure started after-hook: hits=%d reads=%d", st.hits, st.reads.Load())
	}
	assertFailedUpdates(t, st.geoUpdates)
}

func TestFailureStateWriteErrorsAreLoggedAndUnicodeIsBounded(t *testing.T) {
	st := &persistenceStore{fakeStore: newFakeStore(), stage: "failure"}
	r := New(st, AIConfig{}, nil, 1, nil)
	defer r.Shutdown()
	var output bytes.Buffer
	old := log.Writer()
	log.SetOutput(&output)
	defer log.SetOutput(old)
	r.failGeoRun(1, 1, "storage down")
	r.failSerpTask(1, 1, "storage down")
	if st.hits != 2 || !strings.Contains(output.String(), "geo run 1 persist failure state") || !strings.Contains(output.String(), "serp task 1 persist failure state") {
		t.Fatalf("silent failed-state write: hits=%d log=%s", st.hits, output.String())
	}
	if taskError(strings.Repeat("错", 600)) != strings.Repeat("错", 512) || taskError("short") != "short" {
		t.Fatal("task error truncation corrupted Unicode")
	}
}
