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

// 三期全链路：company_cluster 签名 desired-state（dispatcher 工具集 + Task）
// → Sync 收敛 → ApplyAPIServerKey 注入 → PushTask 用对任务 → LastPushedRunID
// 幂等持久化 → 控制面收到 task-runs completed。

func TestSyncCompanyClusterDeliversTask(t *testing.T) {
	now := time.Now().UTC()
	controlPublic, controlPrivate, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatalf("generate control key: %v", err)
	}

	desired := protocol.UnsignedDesiredState{
		SchemaVersion:  1,
		Revision:       10,
		DesiredState:   "active",
		LeaseExpiresAt: now.Add(time.Hour),
		Profile:        "company_cluster",
		Hermes: protocol.DesiredStateArtifact{
			ID:        1,
			Version:   "v2026.7.20",
			SHA256:    strings.Repeat("a", 64),
			SourceURL: "https://example.invalid/hermes.tar.gz",
		},
		Model:    protocol.DesiredStateModel{Mode: "company_proxy", BaseURL: "https://control.example/model", Model: "cluster-model"},
		Toolsets: []string{"clarify", "memory", "no_mcp", "todo", "delegation", "skills", "mcp"},
		Task: &protocol.DesiredStateTask{
			TaskID:      101,
			RunID:       202,
			Description: "三期任务：拆解并输出",
			Model:       "cluster-model",
		},
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
			// active + company_proxy 需要 token 才能收敛；发一个长期有效 token
			_ = json.NewEncoder(writer).Encode(protocol.Envelope[protocol.ModelToken]{
				Code: http.StatusOK, Message: "success",
				Data: protocol.ModelToken{Token: "test-model-token", ExpiresAt: now.Add(30 * time.Minute)},
			})
		case "/api/v1/ai/hermes/device/task-runs":
			var report map[string]any
			if err := json.NewDecoder(request.Body).Decode(&report); err != nil {
				writer.WriteHeader(http.StatusBadRequest)
				return
			}
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

	store, err := state.NewStore(t.TempDir())
	if err != nil {
		t.Fatalf("new store: %v", err)
	}
	devicePublic, _, err := store.EnsureDeviceKey()
	if err != nil {
		t.Fatalf("ensure device key: %v", err)
	}
	config := state.Config{
		ServerURL:              server.URL,
		DeviceID:               "11111111-2222-4333-8444-555555555555",
		PublicKey:              base64.RawStdEncoding.EncodeToString(devicePublic),
		PolicySigningPublicKey: base64.RawStdEncoding.EncodeToString(controlPublic),
		ManagerVersion:         Version,
		HermesVersion:          "v2026.7.20",
		ActualState:            "stopped",
		HealthStatus:           "healthy",
	}
	if err := store.SaveConfig(&config); err != nil {
		t.Fatalf("save config: %v", err)
	}

	manager := New(store, server.Client())
	manager.now = func() time.Time { return now }
	adp := &fakeAdapter{installedVer: "v2026.7.20", running: false, pushTaskResult: "任务执行完毕"}
	manager.SetAdapter(adp)

	if _, err := manager.Sync(t.Context()); err != nil {
		t.Fatalf("sync cluster desired state: %v", err)
	}

	// 收敛后必须注入 API server key 并推任务
	if adp.appliedAPIServerKey == "" {
		t.Fatal("apply_api_server should have been called with a non-empty key")
	}
	if adp.pushedTask.RunID != 202 || adp.pushedTask.TaskID != 101 {
		t.Fatalf("pushed task = %+v, want run 202 / task 101", adp.pushedTask)
	}

	// 幂等持久化：LastPushedRunID 必须已落盘
	saved, err := store.LoadConfig()
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	if saved.LastPushedRunID != 202 {
		t.Fatalf("last_pushed_run_id = %d, want 202", saved.LastPushedRunID)
	}
	if saved.APIServerFingerprint == "" {
		t.Fatal("api_server_fingerprint should be set after sync")
	}
	if saved.APIServerKey == "" {
		t.Fatal("api_server_key should be persisted in manager.json")
	}

	// 控制面必须收到 task-runs completed + result
	mu.Lock()
	reports := append([]map[string]any(nil), taskRunReports...)
	mu.Unlock()
	if len(reports) != 1 {
		t.Fatalf("task-runs reports = %d, want 1", len(reports))
	}
	if reports[0]["run_id"] != float64(202) || reports[0]["status"] != "completed" || reports[0]["result"] != "任务执行完毕" {
		t.Fatalf("task-runs report = %+v, want run 202 / completed / 任务执行完毕", reports[0])
	}
}

// 推任务失败：不记 LastPushedRunID（下轮重试）、设备不失败收敛、不误回传。
func TestSyncClusterTaskPushFailureRetries(t *testing.T) {
	now := time.Now().UTC()
	controlPublic, controlPrivate, _ := ed25519.GenerateKey(nil)
	desired := protocol.UnsignedDesiredState{
		SchemaVersion:  1,
		Revision:       10,
		DesiredState:   "active",
		LeaseExpiresAt: now.Add(time.Hour),
		Profile:        "company_cluster",
		Hermes: protocol.DesiredStateArtifact{
			ID: 1, Version: "v2026.7.20", SHA256: strings.Repeat("a", 64), SourceURL: "https://example.invalid/h.tar.gz",
		},
		Model:    protocol.DesiredStateModel{Mode: "company_proxy", BaseURL: "https://control.example/model", Model: "m"},
		Toolsets: []string{"clarify", "memory", "no_mcp", "todo", "delegation"},
		Task:     &protocol.DesiredStateTask{TaskID: 1, RunID: 2, Description: "任务"},
	}
	payload, _ := json.Marshal(desired)
	signed := protocol.DesiredState{
		UnsignedDesiredState: desired,
		Signature:            base64.RawStdEncoding.EncodeToString(ed25519.Sign(controlPrivate, payload)),
	}

	var mu sync.Mutex
	taskRunCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		switch request.URL.Path {
		case "/api/v1/ai/hermes/device/desired-state":
			_ = json.NewEncoder(writer).Encode(protocol.Envelope[protocol.DesiredState]{Code: http.StatusOK, Message: "success", Data: signed})
		case "/api/v1/ai/hermes/device/model-token":
			_ = json.NewEncoder(writer).Encode(protocol.Envelope[protocol.ModelToken]{Code: http.StatusOK, Message: "success", Data: protocol.ModelToken{Token: "t", ExpiresAt: now.Add(30 * time.Minute)}})
		case "/api/v1/ai/hermes/device/task-runs":
			mu.Lock()
			taskRunCount++
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
	adp := &fakeAdapter{installedVer: "v2026.7.20", pushTaskErr: errTaskPushFail}
	manager.SetAdapter(adp)

	if _, err := manager.Sync(t.Context()); err != nil {
		t.Fatalf("sync with push failure should not fail convergence: %v", err)
	}
	saved, _ := store.LoadConfig()
	if saved.LastPushedRunID != 0 {
		t.Fatalf("failed push must not record run id, got %d", saved.LastPushedRunID)
	}
	mu.Lock()
	count := taskRunCount
	mu.Unlock()
	if count != 0 {
		t.Fatalf("failed push must not report task-runs, got %d", count)
	}
}

var errTaskPushFail = &adapterError{}

// adapterError 仅用于让 PushTask 失败，避免引入真实错误类型。
type adapterError struct{}

func (e *adapterError) Error() string { return "task push failed" }
