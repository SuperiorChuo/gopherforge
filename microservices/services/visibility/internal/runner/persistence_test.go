package runner

import (
	"errors"
	"strings"
	"testing"

	"github.com/go-admin-kit/services/visibility/internal/model"
	"github.com/go-admin-kit/services/visibility/internal/serp"
	store "github.com/go-admin-kit/services/visibility/internal/store/visibility"
)

// These executions are synchronous: no polling can confuse an intermediate
// counter update with completion. Only the injected persistence stage fails.
type persistenceStore struct {
	*fakeStore
	stage                    string
	hits, snapshots, applied int
	geoUpdates, serpUpdates  []map[string]any
}

var persistenceFailure = errors.New("injected persistence failure")

func (s *persistenceStore) fail(stage string) error {
	if s.stage == stage {
		s.hits++
		return persistenceFailure
	}
	return nil
}
func (s *persistenceStore) CreateGeoResult(r *model.GeoResult) error {
	if err := s.fail("result"); err != nil {
		return err
	}
	return s.fakeStore.CreateGeoResult(r)
}
func (s *persistenceStore) UpdateGeoRun(t, id uint64, u map[string]any) error {
	stage := "progress"
	if u["status"] == "done" {
		stage = "complete"
	} else if u["status"] == "failed" {
		stage = "failure"
	}
	if err := s.fail(stage); err != nil {
		return err
	}
	s.geoUpdates = append(s.geoUpdates, u)
	return s.fakeStore.UpdateGeoRun(t, id, u)
}
func (s *persistenceStore) CreateRankSnapshot(*model.RankSnapshot) error {
	if err := s.fail("snapshot"); err != nil {
		return err
	}
	s.snapshots++
	return nil
}
func (s *persistenceStore) ApplyEngineRanks(uint64, uint64, []store.EngineRank) error {
	if err := s.fail("ranks"); err != nil {
		return err
	}
	s.applied++
	return nil
}
func (s *persistenceStore) UpdateSerpTask(t, id uint64, u map[string]any) error {
	stage := "progress"
	if u["status"] == "done" {
		stage = "complete"
	} else if u["status"] == "failed" {
		stage = "failure"
	}
	if err := s.fail(stage); err != nil {
		return err
	}
	s.serpUpdates = append(s.serpUpdates, u)
	return s.fakeStore.UpdateSerpTask(t, id, u)
}
func assertFailedUpdates(t *testing.T, updates []map[string]any) {
	t.Helper()
	if len(updates) == 0 {
		t.Fatal("no state updates")
	}
	last := updates[len(updates)-1]
	text, _ := last["error"].(string)
	if last["status"] != "failed" || last["finished_at"] == nil || !strings.Contains(text, persistenceFailure.Error()) {
		t.Fatalf("persistence failure falsely completed: updates=%v", updates)
	}
	for _, u := range updates {
		if u["status"] == "done" {
			t.Fatalf("persisted success after failure: %v", updates)
		}
	}
}
func TestGeoPersistenceFailuresDoNotComplete(t *testing.T) {
	for _, stage := range []string{"result", "progress", "complete"} {
		t.Run(stage, func(t *testing.T) {
			fs := &persistenceStore{fakeStore: newFakeStore(), stage: stage}
			run := &model.GeoRun{TenantID: 1, Status: "running"}
			if err := fs.CreateGeoRun(run); err != nil {
				t.Fatal(err)
			}
			r := New(fs, AIConfig{}, nil, 1, nil)
			defer r.Shutdown()
			r.wg.Add(1)
			r.execGeoRun(run.ID, 1, "stub", "selfhost", AIConfig{}, []model.GeoPrompt{{ID: 1, Question: "q"}}, nil)
			if fs.hits != 1 {
				t.Fatalf("injection not exercised: %d", fs.hits)
			}
			assertFailedUpdates(t, fs.geoUpdates)
			if stage == "result" {
				for _, u := range fs.geoUpdates {
					if n, ok := u["done_count"].(int); ok && n > 0 {
						t.Fatalf("missing result counted: %v", u)
					}
				}
			}
		})
	}
}
func TestSerpPersistenceFailuresDoNotComplete(t *testing.T) {
	for _, stage := range []string{"snapshot", "ranks", "progress", "complete", "missing-target-snapshot"} {
		t.Run(stage, func(t *testing.T) {
			fs := &persistenceStore{fakeStore: newFakeStore(), stage: stage}
			kw := model.Keyword{ID: 1, Keyword: "q", TargetURL: "https://example.com"}
			if stage == "missing-target-snapshot" {
				fs.stage = "snapshot"
				kw.TargetURL = ""
			}
			r := New(fs, AIConfig{}, []serp.Collector{&fakeCollector{name: "bing", items: []serp.Item{{Position: 2, URL: "https://example.com"}}}}, 1, nil)
			defer r.Shutdown()
			r.execSerpTask(1, 1, []model.Keyword{kw})
			if fs.hits != 1 {
				t.Fatalf("injection not exercised: %d", fs.hits)
			}
			assertFailedUpdates(t, fs.serpUpdates)
			if fs.stage == "snapshot" && fs.applied != 0 {
				t.Fatalf("rank updated without history: %d", fs.applied)
			}
			if fs.stage == "ranks" {
				for _, u := range fs.serpUpdates {
					if n, ok := u["ok_count"].(int); ok && n > 0 {
						t.Fatalf("failed rank counted successful: %v", u)
					}
				}
			}
		})
	}
}
