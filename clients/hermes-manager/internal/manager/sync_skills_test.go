package manager

import (
	"archive/tar"
	"compress/gzip"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"bytes"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/go-admin-kit/clients/hermes-manager/internal/protocol"
	"github.com/go-admin-kit/clients/hermes-manager/internal/state"
)

// M2 技能市场全链路：company_cluster 签名 desired-state 带 Skills → Sync 收敛
// → DownloadSkill（httptest 下载端点返回真实 tarball）→ ApplySkills → digest
// 落盘 + 重启；二次 Sync no-op；下载失败不失败收敛重试。

func makeSkillTarball(t *testing.T, skillMD string) []byte {
	t.Helper()
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	body := []byte(skillMD)
	if err := tw.WriteHeader(&tar.Header{Name: "SKILL.md", Mode: 0o644, Size: int64(len(body))}); err != nil {
		t.Fatalf("tar header: %v", err)
	}
	if _, err := tw.Write(body); err != nil {
		t.Fatalf("tar write: %v", err)
	}
	if err := tw.Close(); err != nil {
		t.Fatalf("tar close: %v", err)
	}
	if err := gz.Close(); err != nil {
		t.Fatalf("gz close: %v", err)
	}
	return buf.Bytes()
}

func newSkillsSyncFixture(t *testing.T, desired protocol.UnsignedDesiredState, skillTarball []byte, downloadFail bool) (*Manager, *state.Store, *fakeAdapter, *httptest.Server) {
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
		case "/api/v1/ai/hermes/device/task-runs":
			_ = json.NewEncoder(writer).Encode(protocol.Envelope[map[string]bool]{Code: http.StatusOK, Message: "success", Data: map[string]bool{"ok": true}})
		case "/api/v1/ai/hermes/device/events":
			_ = json.NewEncoder(writer).Encode(protocol.Envelope[map[string]bool]{Code: http.StatusOK, Message: "success", Data: map[string]bool{"ok": true}})
		default:
			if strings.HasPrefix(request.URL.Path, "/api/v1/ai/hermes/device/skills/") && strings.HasSuffix(request.URL.Path, "/download") {
				if downloadFail {
					writer.WriteHeader(http.StatusServiceUnavailable)
					return
				}
				writer.Header().Set("Content-Type", "application/octet-stream")
				_, _ = writer.Write(skillTarball)
				return
			}
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

func TestSyncCompanyClusterAppliesSkills(t *testing.T) {
	skillMD := "---\nname: weekly-report\ndescription: 周报生成\nversion: 1.0.0\n---\nProcedure: ...\n"
	tarball := makeSkillTarball(t, skillMD)
	sum := sha256.Sum256(tarball)
	sha := hex.EncodeToString(sum[:])

	desired := protocol.UnsignedDesiredState{
		SchemaVersion: 1, Revision: 12, DesiredState: "active",
		LeaseExpiresAt: time.Now().UTC().Add(time.Hour),
		Profile:        "company_cluster",
		Hermes:         protocol.DesiredStateArtifact{ID: 1, Version: "v2026.7.20", SHA256: strings.Repeat("a", 64), SourceURL: "https://e.invalid/h.tar.gz"},
		Model:          protocol.DesiredStateModel{Mode: "company_proxy", BaseURL: "https://control.example/model", Model: "m"},
		Toolsets:       []string{"clarify", "memory", "no_mcp", "todo", "delegation"},
		Skills: []protocol.DesiredStateSkill{{
			ID: 5, Name: "weekly-report", Version: "1.0.0", SHA256: sha,
			SourceURL: "/api/v1/ai/hermes/device/skills/5/download",
		}},
	}
	manager, store, adp, _ := newSkillsSyncFixture(t, desired, tarball, false)

	if _, err := manager.Sync(t.Context()); err != nil {
		t.Fatalf("sync with skills: %v", err)
	}
	if len(adp.appliedSkillBundles) != 1 {
		t.Fatalf("applied skill bundles = %d, want 1", len(adp.appliedSkillBundles))
	}
	if adp.appliedSkillBundles[0].Skill.Name != "weekly-report" {
		t.Fatalf("applied skill = %+v, want weekly-report", adp.appliedSkillBundles[0].Skill)
	}
	// digest 落盘 + 重启
	saved, _ := store.LoadConfig()
	if saved.SkillsAppliedDigest == "" {
		t.Fatal("skills_applied_digest should be set")
	}
	if countCall(adp.calls, "restart") < 1 {
		t.Fatalf("expected a restart on skill install, calls=%v", adp.calls)
	}

	// 二次 Sync：digest 一致 → 不重启不重装
	before := countCall(adp.calls, "restart")
	beforeBundles := len(adp.appliedSkillBundles)
	if _, err := manager.Sync(t.Context()); err != nil {
		t.Fatalf("second sync: %v", err)
	}
	if countCall(adp.calls, "restart") != before {
		t.Fatal("steady-state sync should not restart")
	}
	if len(adp.appliedSkillBundles) != beforeBundles {
		t.Fatal("steady-state sync should not re-apply skills")
	}
}

// 下载失败：不失败收敛、不记 digest、下轮重试。
func TestSyncSkillsDownloadFailureRetries(t *testing.T) {
	desired := protocol.UnsignedDesiredState{
		SchemaVersion: 1, Revision: 12, DesiredState: "active",
		LeaseExpiresAt: time.Now().UTC().Add(time.Hour),
		Profile:        "company_cluster",
		Hermes:         protocol.DesiredStateArtifact{ID: 1, Version: "v2026.7.20", SHA256: strings.Repeat("a", 64), SourceURL: "https://e.invalid/h.tar.gz"},
		Model:          protocol.DesiredStateModel{Mode: "company_proxy", BaseURL: "https://control.example/model", Model: "m"},
		Toolsets:       []string{"clarify", "memory", "no_mcp", "todo", "delegation"},
		Skills: []protocol.DesiredStateSkill{{
			ID: 5, Name: "weekly-report", Version: "1.0.0", SHA256: strings.Repeat("b", 64),
			SourceURL: "/api/v1/ai/hermes/device/skills/5/download",
		}},
	}
	manager, store, adp, _ := newSkillsSyncFixture(t, desired, nil, true)

	if _, err := manager.Sync(t.Context()); err != nil {
		t.Fatalf("sync with download failure should not fail convergence: %v", err)
	}
	saved, _ := store.LoadConfig()
	if saved.SkillsAppliedDigest != "" {
		t.Fatal("failed download must not record digest")
	}
	if len(adp.appliedSkillBundles) != 0 {
		t.Fatal("failed download must not apply skills")
	}
}
