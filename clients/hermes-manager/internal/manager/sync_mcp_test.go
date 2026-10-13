package manager

import (
	"crypto/ed25519"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/go-admin-kit/clients/hermes-manager/internal/protocol"
	"github.com/go-admin-kit/clients/hermes-manager/internal/state"
)

// M3 MCP 管理全链路：company_cluster 签名 desired-state 带 McpServers → Sync
// 收敛 → config.yaml 有 mcp_servers 块 + 无 no_mcp → FetchMcpSecrets 拉密钥 →
// ApplyMcpSecrets 写 .env → 重启一次 → digest 落盘；二次 Sync no-op。

func newMcpSyncFixture(t *testing.T, desired protocol.UnsignedDesiredState, mcpSecrets map[string]string, fetchFail bool) (*Manager, *state.Store, *fakeAdapter, *httptest.Server) {
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

	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		switch request.URL.Path {
		case "/api/v1/ai/hermes/device/desired-state":
			_ = json.NewEncoder(writer).Encode(protocol.Envelope[protocol.DesiredState]{Code: http.StatusOK, Message: "success", Data: signed})
		case "/api/v1/ai/hermes/device/model-token":
			_ = json.NewEncoder(writer).Encode(protocol.Envelope[protocol.ModelToken]{Code: http.StatusOK, Message: "success", Data: protocol.ModelToken{Token: "t", ExpiresAt: now.Add(30 * time.Minute)}})
		case "/api/v1/ai/hermes/device/mcp-secrets":
			if fetchFail {
				writer.WriteHeader(http.StatusServiceUnavailable)
				return
			}
			var req struct {
				EnvKeys []string `json:"env_keys"`
			}
			_ = json.NewDecoder(request.Body).Decode(&req)
			out := map[string]string{}
			for _, key := range req.EnvKeys {
				if val, ok := mcpSecrets[key]; ok {
					out[key] = val
				}
			}
			_ = json.NewEncoder(writer).Encode(protocol.Envelope[map[string]any]{Code: http.StatusOK, Message: "success", Data: map[string]any{"secrets": out}})
		case "/api/v1/ai/hermes/device/task-runs":
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
	adp := &fakeAdapter{installedVer: "v2026.7.20", running: true}
	manager.SetAdapter(adp)
	return manager, store, adp, server
}

func TestSyncCompanyClusterAppliesMcpServers(t *testing.T) {
	desired := protocol.UnsignedDesiredState{
		SchemaVersion: 1, Revision: 13, DesiredState: "active",
		LeaseExpiresAt: time.Now().UTC().Add(time.Hour),
		Profile:        "company_cluster",
		Hermes:         protocol.DesiredStateArtifact{ID: 1, Version: "v2026.7.20", SHA256: strings.Repeat("a", 64), SourceURL: "https://e.invalid/h.tar.gz"},
		Model:          protocol.DesiredStateModel{Mode: "company_proxy", BaseURL: "https://control.example/model", Model: "m"},
		// 含 no_mcp（P1 基线）——渲染时应被剔除
		Toolsets: []string{"clarify", "memory", "no_mcp", "todo", "delegation"},
		McpServers: []protocol.DesiredStateMcpServer{{
			ID: 3, Name: "company-crm", Type: "sse",
			URL: "https://crm.internal/v1/sse", SecretRevision: 1,
			Headers: map[string]string{"Authorization": "Bearer ${MCP_COMPANY_CRM_API_KEY}"},
		}},
	}
	manager, store, adp, _ := newMcpSyncFixture(t, desired,
		map[string]string{"MCP_COMPANY_CRM_API_KEY": "sk-secret"}, false)

	if _, err := manager.Sync(t.Context()); err != nil {
		t.Fatalf("sync with mcp: %v", err)
	}
	// .env 密钥
	if adp.appliedMcpSecrets == nil || adp.appliedMcpSecrets["MCP_COMPANY_CRM_API_KEY"] != "sk-secret" {
		t.Fatalf("applied mcp secrets = %+v, want MCP_COMPANY_CRM_API_KEY=sk-secret", adp.appliedMcpSecrets)
	}
	// digest 落盘 + 重启
	saved, _ := store.LoadConfig()
	if saved.McpServersAppliedDigest == "" {
		t.Fatal("mcp_servers_applied_digest should be set")
	}
	if countCall(adp.calls, "restart") < 1 {
		t.Fatalf("expected a restart on mcp install, calls=%v", adp.calls)
	}

	// 二次 Sync no-op
	before := countCall(adp.calls, "restart")
	if _, err := manager.Sync(t.Context()); err != nil {
		t.Fatalf("second sync: %v", err)
	}
	if countCall(adp.calls, "restart") != before {
		t.Fatal("steady-state sync should not restart")
	}
}

// 拉取密钥失败：不失败收敛、不记 digest、下轮重试。
func TestSyncMcpSecretsFetchFailureRetries(t *testing.T) {
	desired := protocol.UnsignedDesiredState{
		SchemaVersion: 1, Revision: 13, DesiredState: "active",
		LeaseExpiresAt: time.Now().UTC().Add(time.Hour),
		Profile:        "company_cluster",
		Hermes:         protocol.DesiredStateArtifact{ID: 1, Version: "v2026.7.20", SHA256: strings.Repeat("a", 64), SourceURL: "https://e.invalid/h.tar.gz"},
		Model:          protocol.DesiredStateModel{Mode: "company_proxy", BaseURL: "https://control.example/model", Model: "m"},
		Toolsets:       []string{"clarify", "memory", "no_mcp", "todo", "delegation"},
		McpServers: []protocol.DesiredStateMcpServer{{
			ID: 3, Name: "company-crm", Type: "sse", URL: "https://crm.internal/v1/sse",
			Headers: map[string]string{"Authorization": "Bearer ${MCP_COMPANY_CRM_API_KEY}"},
		}},
	}
	manager, store, adp, _ := newMcpSyncFixture(t, desired, nil, true)

	if _, err := manager.Sync(t.Context()); err != nil {
		t.Fatalf("sync with fetch failure should not fail convergence: %v", err)
	}
	saved, _ := store.LoadConfig()
	if saved.McpServersAppliedDigest != "" {
		t.Fatal("failed fetch must not record digest")
	}
	if adp.appliedMcpSecrets != nil {
		t.Fatal("failed fetch must not apply secrets")
	}
}
