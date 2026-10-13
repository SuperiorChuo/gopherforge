package manager

import (
	"crypto/ed25519"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/go-admin-kit/clients/hermes-manager/internal/protocol"
	"github.com/go-admin-kit/clients/hermes-manager/internal/state"
)

// Kanban 里程碑全链路：company_cluster 签名 desired-state 带 Kanban 指令 →
// Sync 收敛 → ensureKanbanGateway（首次重启）→ ApplyWorkerProfiles →
// EnsureKanbanBoard → CreateKanbanTask（幂等）→ ShowKanbanTask done → 回传。

func newKanbanSyncFixture(t *testing.T, desired protocol.UnsignedDesiredState, showStatus, showSummary string) (*Manager, *state.Store, *fakeAdapter, *httptest.Server, *sync.Mutex, *[]map[string]any) {
	t.Helper()
	now := time.Now().UTC()
	controlPublic, controlPrivate, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatalf("generate control key: %v", err)
	}
	payload, _ := json.Marshal(desired)
	signed := protocol.DesiredState{
		UnsignedDesiredState: desired,
		Signature:            base64.RawStdEncoding.EncodeToString(ed25519.Sign(controlPrivate, payload)),
	}

	var mu sync.Mutex
	var taskRunReports []map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		switch request.URL.Path {
		case "/api/v1/ai/hermes/device/desired-state":
			_ = json.NewEncoder(writer).Encode(protocol.Envelope[protocol.DesiredState]{Code: http.StatusOK, Message: "success", Data: signed})
		case "/api/v1/ai/hermes/device/model-token":
			_ = json.NewEncoder(writer).Encode(protocol.Envelope[protocol.ModelToken]{Code: http.StatusOK, Message: "success", Data: protocol.ModelToken{Token: "t", ExpiresAt: now.Add(30 * time.Minute)}})
		case "/api/v1/ai/hermes/device/task-runs":
			var report map[string]any
			_ = json.NewDecoder(request.Body).Decode(&report)
			mu.Lock()
			taskRunReports = append(taskRunReports, report)
			mu.Unlock()
			_ = json.NewEncoder(writer).Encode(protocol.Envelope[map[string]bool]{Code: http.StatusOK, Message: "success", Data: map[string]bool{"ok": true}})
		case "/api/v1/ai/hermes/device/events":
			_ = json.NewEncoder(writer).Encode(protocol.Envelope[map[string]bool]{Code: http.StatusOK, Message: "success", Data: map[string]bool{"ok": true}})
		default:
			http.NotFound(writer, request)
		}
	}))
	t.Cleanup(server.Close)

	store, _ := state.NewStore(t.TempDir())
	devicePublic, _, _ := store.EnsureDeviceKey()
	config := state.Config{
		ServerURL: server.URL, DeviceID: "11111111-2222-4333-8444-555555555555",
		PublicKey: base64.RawStdEncoding.EncodeToString(devicePublic),
		PolicySigningPublicKey: base64.RawStdEncoding.EncodeToString(controlPublic),
		ManagerVersion: Version, HermesVersion: "v2026.7.20",
	}
	_ = store.SaveConfig(&config)

	manager := New(store, server.Client())
	manager.now = func() time.Time { return now }
	adp := &fakeAdapter{installedVer: "v2026.7.20", showStatus: showStatus, showSummary: showSummary}
	manager.SetAdapter(adp)
	return manager, store, adp, server, &mu, &taskRunReports
}

func TestSyncCompanyClusterDispatchesKanban(t *testing.T) {
	desired := protocol.UnsignedDesiredState{
		SchemaVersion:  1,
		Revision:       11,
		DesiredState:   "active",
		LeaseExpiresAt: time.Now().UTC().Add(time.Hour),
		Profile:        "company_cluster",
		Hermes: protocol.DesiredStateArtifact{
			ID: 1, Version: "v2026.7.20", SHA256: strings.Repeat("a", 64), SourceURL: "https://example.invalid/h.tar.gz",
		},
		Model:    protocol.DesiredStateModel{Mode: "company_proxy", BaseURL: "https://control.example/model", Model: "m"},
		Toolsets: []string{"clarify", "memory", "no_mcp", "todo", "delegation", "skills", "mcp"},
		Kanban: &protocol.DesiredStateKanban{
			TaskID: 101, RunID: 202, BoardSlug: "payments",
			Title: "Q3 支付调研", Body: "调研并输出", Assignee: "research-a", Model: "m",
			Workers: []protocol.DesiredStateKanbanWorker{{Name: "research-a"}, {Name: "research-b"}},
		},
	}
	manager, store, adp, _, mu, reports := newKanbanSyncFixture(t, desired, "done", "调研完成")

	if _, err := manager.Sync(t.Context()); err != nil {
		t.Fatalf("sync kanban desired state: %v", err)
	}

	// worker profile 渲染
	if len(adp.workerProfileNames) != 2 || adp.workerProfileNames[0] != "research-a" || adp.workerProfileNames[1] != "research-b" {
		t.Fatalf("worker profiles = %v, want [research-a research-b]", adp.workerProfileNames)
	}
	// board + create + show
	if adp.ensuredBoardSlug != "payments" {
		t.Fatalf("board slug = %q, want payments", adp.ensuredBoardSlug)
	}
	if adp.createdKanban.Assignee != "research-a" || adp.createdKanban.BoardSlug != "payments" {
		t.Fatalf("created kanban = %+v", adp.createdKanban)
	}
	if adp.createdKanbanKey != "gak-kanban-101-202" {
		t.Fatalf("idempotency key = %q, want gak-kanban-101-202", adp.createdKanbanKey)
	}

	// 幂等持久化
	saved, _ := store.LoadConfig()
	if saved.KanbanReportedRunID != 202 {
		t.Fatalf("kanban_reported_run_id = %d, want 202", saved.KanbanReportedRunID)
	}
	if saved.KanbanConfigDigest != saved.ConfigDigest {
		t.Fatalf("kanban_config_digest should equal config digest")
	}

	// 控制面收到 completed
	mu.Lock()
	copied := append([]map[string]any(nil), *reports...)
	mu.Unlock()
	if len(copied) != 1 {
		t.Fatalf("task-runs reports = %d, want 1", len(copied))
	}
	if copied[0]["run_id"] != float64(202) || copied[0]["status"] != "completed" || copied[0]["result"] != "调研完成" {
		t.Fatalf("report = %+v", copied[0])
	}
}

// 跨 sync 轮询：第一次 show 未 done → 不回传；第二次 done → 回传一次；第三次（已记）不再 create/show。
func TestSyncKanbanPollsAcrossSyncs(t *testing.T) {
	desired := protocol.UnsignedDesiredState{
		SchemaVersion: 1, Revision: 11, DesiredState: "active",
		LeaseExpiresAt: time.Now().UTC().Add(time.Hour),
		Profile:        "company_cluster",
		Hermes:         protocol.DesiredStateArtifact{ID: 1, Version: "v2026.7.20", SHA256: strings.Repeat("a", 64), SourceURL: "https://e.invalid/h.tar.gz"},
		Model:          protocol.DesiredStateModel{Mode: "company_proxy", BaseURL: "https://control.example/model", Model: "m"},
		Toolsets:       []string{"clarify", "memory", "no_mcp", "todo", "delegation"},
		Kanban:         &protocol.DesiredStateKanban{TaskID: 1, RunID: 2, BoardSlug: "b", Title: "t", Body: "x", Assignee: "w", Workers: []protocol.DesiredStateKanbanWorker{{Name: "w"}}},
	}
	// showStatus 在两次调用间从 ready 变 done
	manager, store, adp, _, mu, reports := newKanbanSyncFixture(t, desired, "ready", "")
	adp.showCalls = 0

	if _, err := manager.Sync(t.Context()); err != nil {
		t.Fatalf("first sync: %v", err)
	}
	saved, _ := store.LoadConfig()
	if saved.KanbanReportedRunID != 0 {
		t.Fatalf("ready task must not report, got run %d", saved.KanbanReportedRunID)
	}
	if adp.showCalls != 1 {
		t.Fatalf("show calls = %d, want 1", adp.showCalls)
	}

	// 第二次：done
	adp.showStatus = "done"
	adp.showSummary = "完成了"
	if _, err := manager.Sync(t.Context()); err != nil {
		t.Fatalf("second sync: %v", err)
	}
	saved, _ = store.LoadConfig()
	if saved.KanbanReportedRunID != 2 {
		t.Fatalf("done task must report run 2, got %d", saved.KanbanReportedRunID)
	}
	mu.Lock()
	reportCount := len(*reports)
	mu.Unlock()
	if reportCount != 1 {
		t.Fatalf("reports = %d, want 1", reportCount)
	}

	// 第三次：已记，不再 create/show/report
	beforeCalls := adp.showCalls
	beforeCreates := countCall(adp.calls, "create_kanban_task")
	if _, err := manager.Sync(t.Context()); err != nil {
		t.Fatalf("third sync: %v", err)
	}
	if adp.showCalls != beforeCalls {
		t.Fatalf("third sync should not re-show, calls %d → %d", beforeCalls, adp.showCalls)
	}
	if countCall(adp.calls, "create_kanban_task") != beforeCreates {
		t.Fatal("third sync should not re-create")
	}
}

// 建任务失败：发 task_dispatch failed 事件、不记 run、下轮重试。
func TestSyncKanbanCreateFailureRetries(t *testing.T) {
	desired := protocol.UnsignedDesiredState{
		SchemaVersion: 1, Revision: 11, DesiredState: "active",
		LeaseExpiresAt: time.Now().UTC().Add(time.Hour),
		Profile:        "company_cluster",
		Hermes:         protocol.DesiredStateArtifact{ID: 1, Version: "v2026.7.20", SHA256: strings.Repeat("a", 64), SourceURL: "https://e.invalid/h.tar.gz"},
		Model:          protocol.DesiredStateModel{Mode: "company_proxy", BaseURL: "https://control.example/model", Model: "m"},
		Toolsets:       []string{"clarify", "memory", "no_mcp", "todo", "delegation"},
		Kanban:         &protocol.DesiredStateKanban{TaskID: 1, RunID: 2, BoardSlug: "b", Title: "t", Body: "x", Assignee: "w", Workers: []protocol.DesiredStateKanbanWorker{{Name: "w"}}},
	}
	manager, store, adp, _, mu, reports := newKanbanSyncFixture(t, desired, "done", "")
	adp.createKanbanErr = errTaskPushFail

	if _, err := manager.Sync(t.Context()); err != nil {
		t.Fatalf("sync with create failure should not fail convergence: %v", err)
	}
	saved, _ := store.LoadConfig()
	if saved.KanbanReportedRunID != 0 {
		t.Fatalf("failed create must not record run, got %d", saved.KanbanReportedRunID)
	}
	mu.Lock()
	count := len(*reports)
	mu.Unlock()
	if count != 0 {
		t.Fatalf("failed create must not report, got %d", count)
	}
}

// worker 渲染失败：不建 board/任务。
func TestSyncKanbanWorkerProfileFailure(t *testing.T) {
	desired := protocol.UnsignedDesiredState{
		SchemaVersion: 1, Revision: 11, DesiredState: "active",
		LeaseExpiresAt: time.Now().UTC().Add(time.Hour),
		Profile:        "company_cluster",
		Hermes:         protocol.DesiredStateArtifact{ID: 1, Version: "v2026.7.20", SHA256: strings.Repeat("a", 64), SourceURL: "https://e.invalid/h.tar.gz"},
		Model:          protocol.DesiredStateModel{Mode: "company_proxy", BaseURL: "https://control.example/model", Model: "m"},
		Toolsets:       []string{"clarify", "memory", "no_mcp", "todo", "delegation"},
		Kanban:         &protocol.DesiredStateKanban{TaskID: 1, RunID: 2, BoardSlug: "b", Title: "t", Body: "x", Assignee: "w", Workers: []protocol.DesiredStateKanbanWorker{{Name: "w"}}},
	}
	manager, store, adp, _, mu, reports := newKanbanSyncFixture(t, desired, "done", "")
	adp.workerProfileErr = errTaskPushFail

	if _, err := manager.Sync(t.Context()); err != nil {
		t.Fatalf("sync with worker profile failure: %v", err)
	}
	saved, _ := store.LoadConfig()
	if saved.KanbanReportedRunID != 0 {
		t.Fatalf("worker failure must not record run")
	}
	if adp.ensuredBoardSlug != "" {
		t.Fatalf("worker failure must not ensure board")
	}
	mu.Lock()
	count := len(*reports)
	mu.Unlock()
	if count != 0 {
		t.Fatalf("worker failure must not report")
	}
}

// 首次 kanban sync 重启 gateway（config 摘要变）；稳态不重启。
func TestSyncKanbanRestartsGatewayOnConfigDigestChange(t *testing.T) {
	desired := protocol.UnsignedDesiredState{
		SchemaVersion: 1, Revision: 11, DesiredState: "active",
		LeaseExpiresAt: time.Now().UTC().Add(time.Hour),
		Profile:        "company_cluster",
		Hermes:         protocol.DesiredStateArtifact{ID: 1, Version: "v2026.7.20", SHA256: strings.Repeat("a", 64), SourceURL: "https://e.invalid/h.tar.gz"},
		Model:          protocol.DesiredStateModel{Mode: "company_proxy", BaseURL: "https://control.example/model", Model: "m"},
		Toolsets:       []string{"clarify", "memory", "no_mcp", "todo", "delegation"},
		Kanban:         &protocol.DesiredStateKanban{TaskID: 1, RunID: 2, BoardSlug: "b", Title: "t", Body: "x", Assignee: "w", Workers: []protocol.DesiredStateKanbanWorker{{Name: "w"}}},
	}
	manager, store, adp, _, _, _ := newKanbanSyncFixture(t, desired, "done", "")

	// 第一次：摘要变 → Restart
	if _, err := manager.Sync(t.Context()); err != nil {
		t.Fatalf("first sync: %v", err)
	}
	saved, _ := store.LoadConfig()
	if saved.KanbanConfigDigest != saved.ConfigDigest {
		t.Fatal("kanban_config_digest should be set after first sync")
	}
	restartCount := countCall(adp.calls, "restart")
	if restartCount < 1 {
		t.Fatalf("expected a restart on first kanban sync, calls=%v", adp.calls)
	}

	// 第二次：摘要一致 → 不重启
	before := countCall(adp.calls, "restart")
	if _, err := manager.Sync(t.Context()); err != nil {
		t.Fatalf("second sync: %v", err)
	}
	if countCall(adp.calls, "restart") != before {
		t.Fatal("steady-state sync should not restart gateway")
	}
}

func countCall(calls []string, target string) int {
	count := 0
	for _, c := range calls {
		if c == target {
			count++
		}
	}
	return count
}
