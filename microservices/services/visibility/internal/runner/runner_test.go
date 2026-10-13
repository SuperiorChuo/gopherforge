package runner

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/go-admin-kit/services/visibility/internal/model"
	"github.com/go-admin-kit/services/visibility/internal/serp"
	"github.com/go-admin-kit/services/visibility/internal/store/visibility"
)

type fakeStore struct {
	mu      sync.Mutex
	nextID  uint64
	runs    map[uint64]*model.GeoRun
	results int

	// hooks
	onCreateResult    func(*model.GeoResult) error
	resultEntered     chan struct{}
	resultGate        chan struct{} // 非 nil 时 CreateGeoResult 先等待，用于卡住 goroutine
	createRunEntered  chan struct{}
	createRunGate     chan struct{}
	createRunErr      error
	createTaskEntered chan struct{}
	createTaskGate    chan struct{}
	createTaskErr     error
	findGeoErr        error
	findSerpErr       error
	serpCreates       int
	serpStatus        string
	trackingKeywords  bool
}

func newFakeStore() *fakeStore {
	return &fakeStore{runs: map[uint64]*model.GeoRun{}}
}

func (f *fakeStore) ListActiveGeoPrompts(uint64) ([]model.GeoPrompt, error) {
	return []model.GeoPrompt{{ID: 1, Question: "有什么后台推荐？"}}, nil
}

func (f *fakeStore) ListAllBrands(uint64) ([]model.Brand, error) {
	return []model.Brand{{Name: "MyBrand", Kind: "brand"}}, nil
}

func (f *fakeStore) CreateGeoRun(r *model.GeoRun) error {
	if f.createRunEntered != nil {
		close(f.createRunEntered)
	}
	if f.createRunGate != nil {
		<-f.createRunGate
	}
	if f.createRunErr != nil {
		return f.createRunErr
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	f.nextID++
	r.ID = f.nextID
	cp := *r
	f.runs[r.ID] = &cp
	return nil
}

func (f *fakeStore) UpdateGeoRun(_, id uint64, updates map[string]any) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	r, ok := f.runs[id]
	if !ok {
		return fmt.Errorf("run %d not found", id)
	}
	if v, ok := updates["status"].(string); ok {
		r.Status = v
	}
	if v, ok := updates["error"].(string); ok {
		r.Error = v
	}
	return nil
}

func (f *fakeStore) CreateGeoResult(res *model.GeoResult) error {
	if f.resultEntered != nil {
		close(f.resultEntered)
	}
	if f.resultGate != nil {
		<-f.resultGate
	}
	if f.onCreateResult != nil {
		return f.onCreateResult(res)
	}
	f.mu.Lock()
	f.results++
	f.mu.Unlock()
	return nil
}

func (f *fakeStore) FindRunningGeoRun(uint64) (*model.GeoRun, error) {
	return nil, f.findGeoErr
}

func (f *fakeStore) GetGeoRun(_, id uint64) (*model.GeoRun, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if r, ok := f.runs[id]; ok {
		return r, nil
	}
	return nil, fmt.Errorf("not found")
}

func (f *fakeStore) ListGeoResults(uint64, uint64) ([]model.GeoResult, error) { return nil, nil }

func (f *fakeStore) ListTrackingKeywords(uint64, uint64) ([]model.Keyword, error) {
	if f.trackingKeywords {
		return []model.Keyword{{ID: 1, Keyword: "后台系统", TargetURL: "https://example.com"}}, nil
	}
	return nil, nil
}
func (f *fakeStore) CreateSerpTask(task *model.SerpTask) error {
	if f.createTaskEntered != nil {
		close(f.createTaskEntered)
	}
	if f.createTaskGate != nil {
		<-f.createTaskGate
	}
	if f.createTaskErr != nil {
		return f.createTaskErr
	}
	f.mu.Lock()
	f.nextID++
	task.ID = f.nextID
	f.serpCreates++
	f.mu.Unlock()
	return nil
}
func (f *fakeStore) UpdateSerpTask(_ uint64, _ uint64, updates map[string]any) error {
	f.mu.Lock()
	if status, ok := updates["status"].(string); ok {
		f.serpStatus = status
	}
	f.mu.Unlock()
	return nil
}
func (f *fakeStore) FindRunningSerpTask(uint64) (*model.SerpTask, error) {
	return nil, f.findSerpErr
}
func (f *fakeStore) CreateRankSnapshot(*model.RankSnapshot) error                 { return nil }
func (f *fakeStore) ApplyEngineRanks(uint64, uint64, []store.EngineRank) error    { return nil }
func (f *fakeStore) GetSettingJSON(string) (map[string]any, error)                { return nil, nil }
func (f *fakeStore) ListTenantIDsWithActiveGeoPrompts() ([]uint64, error)         { return nil, nil }
func (f *fakeStore) ListTenantIDsWithTrackingKeywords() ([]uint64, error)         { return nil, nil }
func (f *fakeStore) FindPreviousDoneGeoRun(uint64, uint64) (*model.GeoRun, error) { return nil, nil }
func (f *fakeStore) ListAdminUserIDs(context.Context, uint64) ([]uint64, error)   { return nil, nil }

func (f *fakeStore) serpTaskStatus() string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.serpStatus
}

func (f *fakeStore) runStatus(id uint64) string {
	f.mu.Lock()
	defer f.mu.Unlock()
	if r, ok := f.runs[id]; ok {
		return r.Status
	}
	return ""
}

type shutdownBlockingCollector struct {
	entered chan struct{}
}

func (c *shutdownBlockingCollector) Name() string { return "shutdown-blocking" }
func (c *shutdownBlockingCollector) Fetch(ctx context.Context, _ string, _ int) ([]serp.Item, error) {
	close(c.entered)
	<-ctx.Done()
	return nil, ctx.Err()
}

func waitFor(t *testing.T, timeout time.Duration, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("condition not met before timeout")
}

func assertShutdownStillWaiting(t *testing.T, r *Runner, shutdownDone <-chan struct{}) {
	t.Helper()
	<-r.ctx.Done()
	select {
	case <-shutdownDone:
		t.Fatal("shutdown returned before the registered task completed")
	case <-time.After(25 * time.Millisecond):
	}
}

func TestShutdownWaitsForInFlightGeoSubmission(t *testing.T) {
	fs := newFakeStore()
	fs.createRunEntered = make(chan struct{})
	fs.createRunGate = make(chan struct{})
	r := New(fs, AIConfig{}, nil, 5, nil)

	submitDone := make(chan error, 1)
	go func() {
		_, _, err := r.SubmitGeoRun(1, 1, "stub", "", "")
		submitDone <- err
	}()
	<-fs.createRunEntered

	shutdownDone := make(chan struct{})
	go func() {
		r.Shutdown()
		close(shutdownDone)
	}()

	assertShutdownStillWaiting(t, r, shutdownDone)
	close(fs.createRunGate)

	if err := <-submitDone; err != nil {
		t.Fatalf("in-flight submit: %v", err)
	}
	select {
	case <-shutdownDone:
	case <-time.After(3 * time.Second):
		t.Fatal("shutdown did not wait for the registered task")
	}
}

func TestShutdownWaitsForInFlightSerpSubmission(t *testing.T) {
	fs := newFakeStore()
	fs.trackingKeywords = true
	fs.createTaskEntered = make(chan struct{})
	fs.createTaskGate = make(chan struct{})
	r := New(fs, AIConfig{}, nil, 5, nil)

	submitDone := make(chan error, 1)
	go func() {
		_, _, err := r.SubmitSerpTask(1, 1, 0, "manual")
		submitDone <- err
	}()
	<-fs.createTaskEntered

	shutdownDone := make(chan struct{})
	go func() {
		r.Shutdown()
		close(shutdownDone)
	}()
	assertShutdownStillWaiting(t, r, shutdownDone)
	close(fs.createTaskGate)

	if err := <-submitDone; err != nil {
		t.Fatalf("in-flight submit: %v", err)
	}
	select {
	case <-shutdownDone:
	case <-time.After(3 * time.Second):
		t.Fatal("shutdown did not wait for the registered SERP task")
	}
}

func TestShutdownWaitsForInFlightScheduledSerpSubmission(t *testing.T) {
	fs := newFakeStore()
	fs.trackingKeywords = true
	fs.createTaskEntered = make(chan struct{})
	fs.createTaskGate = make(chan struct{})
	r := New(fs, AIConfig{}, nil, 5, nil)

	submitDone := make(chan error, 1)
	go func() {
		_, _, err := r.submitSerpSync(1)
		submitDone <- err
	}()
	<-fs.createTaskEntered

	shutdownDone := make(chan struct{})
	go func() {
		r.Shutdown()
		close(shutdownDone)
	}()
	assertShutdownStillWaiting(t, r, shutdownDone)
	close(fs.createTaskGate)

	if err := <-submitDone; err != nil {
		t.Fatalf("in-flight scheduled submit: %v", err)
	}
	select {
	case <-shutdownDone:
	case <-time.After(3 * time.Second):
		t.Fatal("shutdown did not wait for the registered scheduled SERP task")
	}
}

func TestGeoCancellationDuringLastResultMarksFailed(t *testing.T) {
	fs := newFakeStore()
	fs.resultEntered = make(chan struct{})
	fs.resultGate = make(chan struct{})
	r := New(fs, AIConfig{}, nil, 5, nil)

	if _, _, err := r.SubmitGeoRun(1, 1, "stub", "", ""); err != nil {
		t.Fatalf("submit Geo run: %v", err)
	}
	<-fs.resultEntered
	shutdownDone := make(chan struct{})
	go func() {
		r.Shutdown()
		close(shutdownDone)
	}()
	<-r.ctx.Done()
	close(fs.resultGate)
	select {
	case <-shutdownDone:
	case <-time.After(3 * time.Second):
		t.Fatal("shutdown did not wait for canceled Geo run")
	}

	if got := fs.runStatus(1); got != "failed" {
		t.Fatalf("run status after cancellation = %q, want failed", got)
	}
}

func TestSerpCancellationDuringLastKeywordMarksFailed(t *testing.T) {
	fs := newFakeStore()
	fs.trackingKeywords = true
	collector := &shutdownBlockingCollector{entered: make(chan struct{})}
	r := New(fs, AIConfig{}, []serp.Collector{collector}, 1, nil)

	if _, _, err := r.SubmitSerpTask(1, 1, 0, "manual"); err != nil {
		t.Fatalf("submit SERP task: %v", err)
	}
	<-collector.entered
	r.Shutdown()

	if got := fs.serpTaskStatus(); got != "failed" {
		t.Fatalf("task status after cancellation = %q, want failed", got)
	}
}

func assertShutdownReturns(t *testing.T, r *Runner) {
	t.Helper()
	done := make(chan struct{})
	go func() {
		r.Shutdown()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("shutdown blocked by a leaked lifecycle registration")
	}
}

func TestSubmitReturnsDedupLookupError(t *testing.T) {
	wantErr := errors.New("dedup lookup failed")

	t.Run("geo", func(t *testing.T) {
		fs := newFakeStore()
		fs.findGeoErr = wantErr
		r := New(fs, AIConfig{}, nil, 5, nil)
		defer r.Shutdown()
		if _, _, err := r.SubmitGeoRun(1, 1, "stub", "", ""); !errors.Is(err, wantErr) {
			t.Fatalf("submit error = %v, want %v", err, wantErr)
		}
		fs.mu.Lock()
		created := len(fs.runs)
		fs.mu.Unlock()
		if created != 0 {
			t.Fatalf("created %d Geo runs after dedup lookup failure", created)
		}
	})

	t.Run("serp", func(t *testing.T) {
		fs := newFakeStore()
		fs.findSerpErr = wantErr
		fs.trackingKeywords = true
		r := New(fs, AIConfig{}, nil, 5, nil)
		defer r.Shutdown()
		if _, _, err := r.SubmitSerpTask(1, 1, 0, "manual"); !errors.Is(err, wantErr) {
			t.Fatalf("submit error = %v, want %v", err, wantErr)
		}
		if fs.serpCreates != 0 {
			t.Fatalf("created %d SERP tasks after dedup lookup failure", fs.serpCreates)
		}
	})

	t.Run("scheduled_serp", func(t *testing.T) {
		fs := newFakeStore()
		fs.findSerpErr = wantErr
		fs.trackingKeywords = true
		r := New(fs, AIConfig{}, nil, 5, nil)
		defer r.Shutdown()
		if _, _, err := r.submitSerpSync(1); !errors.Is(err, wantErr) {
			t.Fatalf("submit error = %v, want %v", err, wantErr)
		}
		if fs.serpCreates != 0 {
			t.Fatalf("created %d scheduled SERP tasks after dedup lookup failure", fs.serpCreates)
		}
	})
}

func TestCreateFailureReleasesLifecycleRegistration(t *testing.T) {
	wantErr := errors.New("create failed")

	t.Run("geo", func(t *testing.T) {
		fs := newFakeStore()
		fs.createRunErr = wantErr
		r := New(fs, AIConfig{}, nil, 5, nil)
		if _, _, err := r.SubmitGeoRun(1, 1, "stub", "", ""); !errors.Is(err, wantErr) {
			t.Fatalf("submit error = %v, want %v", err, wantErr)
		}
		assertShutdownReturns(t, r)
		assertShutdownReturns(t, r) // 重复关闭仍应安全返回。
	})

	t.Run("serp", func(t *testing.T) {
		fs := newFakeStore()
		fs.trackingKeywords = true
		fs.createTaskErr = wantErr
		r := New(fs, AIConfig{}, nil, 5, nil)
		if _, _, err := r.SubmitSerpTask(1, 1, 0, "manual"); !errors.Is(err, wantErr) {
			t.Fatalf("submit error = %v, want %v", err, wantErr)
		}
		assertShutdownReturns(t, r)
	})

	t.Run("scheduled_serp", func(t *testing.T) {
		fs := newFakeStore()
		fs.trackingKeywords = true
		fs.createTaskErr = wantErr
		r := New(fs, AIConfig{}, nil, 5, nil)
		if _, _, err := r.submitSerpSync(1); !errors.Is(err, wantErr) {
			t.Fatalf("submit error = %v, want %v", err, wantErr)
		}
		assertShutdownReturns(t, r)
	})
}

func TestSubmitGeoRunDedup(t *testing.T) {
	fs := newFakeStore()
	fs.resultGate = make(chan struct{}) // 卡住第一个 run 的 goroutine
	r := New(fs, AIConfig{}, nil, 5, nil)
	defer r.Shutdown()

	run1, reused1, err := r.SubmitGeoRun(1, 1, "stub", "", "")
	if err != nil || reused1 {
		t.Fatalf("first submit: run=%+v reused=%v err=%v", run1, reused1, err)
	}
	run2, reused2, err := r.SubmitGeoRun(1, 1, "stub", "", "")
	if err != nil || !reused2 || run2.ID != run1.ID {
		t.Fatalf("second submit should reuse run %d: run=%+v reused=%v err=%v", run1.ID, run2, reused2, err)
	}
	// 不同租户不受影响
	fsOther := fs // 同一个 fake，但 tenant 2 没有 active 记录
	_ = fsOther
	run3, reused3, err := r.SubmitGeoRun(2, 1, "stub", "", "")
	if err != nil || reused3 || run3.ID == run1.ID {
		t.Fatalf("other tenant should get its own run: run=%+v reused=%v err=%v", run3, reused3, err)
	}

	close(fs.resultGate)
	waitFor(t, 3*time.Second, func() bool { return fs.runStatus(run1.ID) == "done" })
	// 完成后可再次提交
	run4, reused4, err := r.SubmitGeoRun(1, 1, "stub", "", "")
	if err != nil || reused4 {
		t.Fatalf("after done, submit should create new run: run=%+v reused=%v err=%v", run4, reused4, err)
	}
	waitFor(t, 3*time.Second, func() bool { return fs.runStatus(run4.ID) == "done" })
}

func TestGeoRunPanicMarksFailed(t *testing.T) {
	fs := newFakeStore()
	fs.onCreateResult = func(*model.GeoResult) error { panic("boom") }
	r := New(fs, AIConfig{}, nil, 5, nil)
	defer r.Shutdown()

	run, _, err := r.SubmitGeoRun(1, 1, "stub", "", "")
	if err != nil {
		t.Fatalf("submit: %v", err)
	}
	waitFor(t, 3*time.Second, func() bool { return fs.runStatus(run.ID) == "failed" })
	fs.mu.Lock()
	errMsg := fs.runs[run.ID].Error
	fs.mu.Unlock()
	if !strings.Contains(errMsg, "panic") {
		t.Fatalf("error should mention panic, got %q", errMsg)
	}
	// panic 后 active 表已清理，可重新提交
	fs.onCreateResult = nil
	run2, reused, err := r.SubmitGeoRun(1, 1, "stub", "", "")
	if err != nil || reused {
		t.Fatalf("resubmit after panic: run=%+v reused=%v err=%v", run2, reused, err)
	}
}
