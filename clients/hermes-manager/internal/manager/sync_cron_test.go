package manager

import (
	"crypto/ed25519"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/go-admin-kit/clients/hermes-manager/internal/protocol"
	"github.com/go-admin-kit/clients/hermes-manager/internal/state"
)

var errCronPushFailed = errors.New("cron push failed")

// M5 全链路：company_cluster 签名 desired-state 带 Cron 任务 → Sync →
// dispatchCron 评估命中 → PushTask 推提示词 → POST /device/cron-runs 回传 →
// CronLastRuns 分钟槽幂等 → 二次 Sync 无动作。

func newCronSyncFixture(t *testing.T, desired protocol.UnsignedDesiredState, now time.Time) (*Manager, *state.Store, *fakeAdapter, *sync.Mutex, *[]map[string]any) {
	t.Helper()
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
	var cronRunReports []map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		switch request.URL.Path {
		case "/api/v1/ai/hermes/device/desired-state":
			_ = json.NewEncoder(writer).Encode(protocol.Envelope[protocol.DesiredState]{Code: http.StatusOK, Message: "success", Data: signed})
		case "/api/v1/ai/hermes/device/model-token":
			_ = json.NewEncoder(writer).Encode(protocol.Envelope[protocol.ModelToken]{Code: http.StatusOK, Message: "success", Data: protocol.ModelToken{Token: "t", ExpiresAt: now.Add(30 * time.Minute)}})
		case "/api/v1/ai/hermes/device/cron-runs":
			var report map[string]any
			_ = json.NewDecoder(request.Body).Decode(&report)
			mu.Lock()
			cronRunReports = append(cronRunReports, report)
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
		PublicKey:            base64.RawStdEncoding.EncodeToString(devicePublic),
		PolicySigningPublicKey: base64.RawStdEncoding.EncodeToString(controlPublic),
		ManagerVersion:       Version,
		HermesVersion:        "v2026.7.20",
		ActualState:          "active",
		APIServerKey:         "test-loopback-key",
	}
	_ = store.SaveConfig(&config)

	manager := New(store, server.Client())
	manager.now = func() time.Time { return now }
	adp := &fakeAdapter{installedVer: "v2026.7.20", pushTaskResult: "日报已生成"}
	manager.SetAdapter(adp)
	return manager, store, adp, &mu, &cronRunReports
}

func TestSyncCompanyClusterDispatchesCron(t *testing.T) {
	// 周一 09:00：命中 0 9 * * 1-5
	monday9 := time.Date(2026, 8, 3, 9, 0, 0, 0, time.UTC)
	desired := protocol.UnsignedDesiredState{
		SchemaVersion:  1,
		Revision:       21,
		DesiredState:   "active",
		LeaseExpiresAt: time.Now().UTC().Add(time.Hour),
		Profile:        "company_cluster",
		Hermes: protocol.DesiredStateArtifact{
			ID: 1, Version: "v2026.7.20", SHA256: strings.Repeat("a", 64), SourceURL: "https://example.invalid/h.tar.gz",
		},
		Model:    protocol.DesiredStateModel{Mode: "company_proxy", BaseURL: "https://control.example/model", Model: "m"},
		Toolsets: []string{"clarify", "memory", "no_mcp", "todo", "delegation", "skills", "mcp"},
		Cron: []protocol.DesiredStateCronJob{
			{ID: 5, Name: "daily-report", CronExpression: "0 9 * * 1-5", Prompt: "生成昨日工作日报", Enabled: true},
		},
	}
	manager, store, adp, mu, reports := newCronSyncFixture(t, desired, monday9)

	// 第一次 Sync：命中并回传
	_, err := manager.Sync(t.Context())
	if err != nil {
		t.Fatalf("sync 1: %v", err)
	}
	mu.Lock()
	if len(*reports) != 1 {
		mu.Unlock()
		t.Fatalf("cron-runs reports = %d, want 1", len(*reports))
	}
	report := (*reports)[0]
	mu.Unlock()
	if report["job_id"] != float64(5) || report["status"] != "completed" {
		t.Fatalf("report = %v, want job 5 completed", report)
	}
	if adp.pushedTask.Description != "生成昨日工作日报" {
		t.Fatalf("pushed task = %+v, want daily prompt", adp.pushedTask)
	}

	// 幂等：CronLastRuns 已记槽
	cfg, err := store.LoadConfig()
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	if cfg.CronLastRuns["5"] != monday9.Unix()/60 {
		t.Fatalf("CronLastRuns = %v, want slot %d", cfg.CronLastRuns, monday9.Unix()/60)
	}

	// 第二次 Sync（同一 now）：不再推送/上报
	manager.Sync(t.Context())
	mu.Lock()
	if len(*reports) != 1 {
		mu.Unlock()
		t.Fatalf("second sync produced %d reports, want still 1", len(*reports))
	}
	mu.Unlock()
}

func TestSyncCronPushFailureDoesNotRecord(t *testing.T) {
	// 推送失败：不记槽、不上报；发 cron_dispatch 事件（events 端点静默接受）
	monday9 := time.Date(2026, 8, 3, 9, 0, 0, 0, time.UTC)
	desired := protocol.UnsignedDesiredState{
		SchemaVersion:  1,
		Revision:       22,
		DesiredState:   "active",
		LeaseExpiresAt: time.Now().UTC().Add(time.Hour),
		Profile:        "company_cluster",
		Hermes: protocol.DesiredStateArtifact{
			ID: 1, Version: "v2026.7.20", SHA256: strings.Repeat("a", 64), SourceURL: "https://example.invalid/h.tar.gz",
		},
		Model:    protocol.DesiredStateModel{Mode: "company_proxy", BaseURL: "https://control.example/model", Model: "m"},
		Toolsets: []string{"clarify", "memory", "no_mcp", "todo", "delegation", "skills", "mcp"},
		Cron: []protocol.DesiredStateCronJob{
			{ID: 6, Name: "daily", CronExpression: "0 9 * * 1-5", Prompt: "p", Enabled: true},
		},
	}
	manager, store, adp, _, _ := newCronSyncFixture(t, desired, monday9)
	adp.pushTaskErr = errCronPushFailed

	manager.Sync(t.Context())

	cfg, err := store.LoadConfig()
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	if _, recorded := cfg.CronLastRuns["6"]; recorded {
		t.Fatalf("push failure must not record slot, got %v", cfg.CronLastRuns)
	}
}
