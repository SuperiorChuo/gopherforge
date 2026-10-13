package manager

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
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

func signedManagerTestDesired(t *testing.T, privateKey ed25519.PrivateKey, now time.Time) *protocol.DesiredState {
	t.Helper()
	unsigned := protocol.UnsignedDesiredState{
		SchemaVersion:  1,
		Revision:       12,
		DesiredState:   "active",
		LeaseExpiresAt: now.Add(5 * time.Minute),
		Profile:        "company",
		Hermes: protocol.DesiredStateArtifact{
			ID:        1,
			Version:   "v2026.7.20",
			Commit:    strings.Repeat("a", 40),
			SHA256:    strings.Repeat("b", 64),
			SourceURL: "https://artifacts.example.invalid/hermes.tar.gz",
		},
		Model: protocol.DesiredStateModel{
			Mode:    "company_proxy",
			BaseURL: "https://models.example.invalid/v1",
			Model:   "managed-model",
		},
		Toolsets: []string{"clarify", "memory", "no_mcp", "todo"},
	}
	payload, err := json.Marshal(unsigned)
	if err != nil {
		t.Fatalf("marshal desired state: %v", err)
	}
	return &protocol.DesiredState{
		UnsignedDesiredState: unsigned,
		Signature:            base64.RawStdEncoding.EncodeToString(ed25519.Sign(privateKey, payload)),
	}
}

func writeManagerTestEnvelope(w http.ResponseWriter, status int, data any, errorCode string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"code":       status,
		"message":    http.StatusText(status),
		"error_code": errorCode,
		"data":       data,
	})
}

func newManagerTestServer(t *testing.T, desired *protocol.DesiredState, failModelToken bool) *httptest.Server {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		switch request.URL.Path {
		case "/api/v1/ai/hermes/device/desired-state":
			writeManagerTestEnvelope(w, http.StatusOK, desired, "")
		case "/api/v1/ai/hermes/device/model-token":
			if failModelToken {
				writeManagerTestEnvelope(w, http.StatusInternalServerError, map[string]any{}, "MODEL_TOKEN_ISSUE_FAILED")
				return
			}
			writeManagerTestEnvelope(w, http.StatusOK, protocol.ModelToken{
				Token: "test-token", ExpiresAt: time.Now().UTC().Add(30 * time.Minute),
			}, "")
		case "/api/v1/ai/hermes/device/events":
			writeManagerTestEnvelope(w, http.StatusOK, map[string]bool{"ok": true}, "")
		default:
			http.NotFound(w, request)
		}
	}))
	t.Cleanup(server.Close)
	return server
}

func newManagerTestStore(
	t *testing.T,
	serverURL string,
	policyPublicKey ed25519.PublicKey,
	configure func(*state.Config),
) *state.Store {
	t.Helper()
	store, err := state.NewStore(t.TempDir())
	if err != nil {
		t.Fatalf("new store: %v", err)
	}
	devicePublicKey, _, err := store.EnsureDeviceKey()
	if err != nil {
		t.Fatalf("ensure device key: %v", err)
	}
	config := &state.Config{
		ServerURL:              serverURL,
		DeviceID:               "device-test-1",
		PublicKey:              base64.RawStdEncoding.EncodeToString(devicePublicKey),
		PolicySigningPublicKey: base64.RawStdEncoding.EncodeToString(policyPublicKey),
		ManagerVersion:         Version,
		ActualState:            "active",
		HealthStatus:           "healthy",
	}
	if configure != nil {
		configure(config)
	}
	if err := store.SaveConfig(config); err != nil {
		t.Fatalf("save config: %v", err)
	}
	return store
}

func TestSyncRestoringActiveDoesNotReportHealthyWhenModelTokenRefreshFails(t *testing.T) {
	now := time.Now().UTC()
	policyPublicKey, policyPrivateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("generate policy signing key: %v", err)
	}
	desired := signedManagerTestDesired(t, policyPrivateKey, now)
	server := newManagerTestServer(t, desired, true)
	store := newManagerTestStore(t, server.URL, policyPublicKey, func(config *state.Config) {
		config.ActualState = "stopped"
		config.HealthStatus = "healthy"
		config.AppliedPolicyRevision = 11
		config.HermesVersion = desired.Hermes.Version
		config.ModelTokenExpiresAt = now.Add(time.Hour)
	})
	adp := &fakeAdapter{installedVer: desired.Hermes.Version, running: false}
	mgr := New(store, server.Client())
	mgr.now = func() time.Time { return now }
	mgr.SetAdapter(adp)

	returnedDesired, syncErr := mgr.Sync(context.Background())

	if returnedDesired == nil || returnedDesired.Revision != desired.Revision {
		t.Fatalf("desired state = %+v, want revision %d", returnedDesired, desired.Revision)
	}
	if syncErr == nil {
		t.Fatal("restoring active without a replacement token must fail convergence")
	}
	config, err := store.LoadConfig()
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	if config.ActualState == "active" && config.HealthStatus == "healthy" {
		t.Fatalf("token refresh failure was reported healthy: %+v", config)
	}
	if config.LastErrorCode != "MODEL_TOKEN_REFRESH_FAILED" {
		t.Fatalf("last error = %q, want MODEL_TOKEN_REFRESH_FAILED", config.LastErrorCode)
	}
}

func TestSyncPersistsUpdateIntentBeforeSwitchingArtifact(t *testing.T) {
	now := time.Now().UTC()
	policyPublicKey, policyPrivateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("generate policy signing key: %v", err)
	}
	desired := signedManagerTestDesired(t, policyPrivateKey, now)
	desired.Model.Mode = "employee"
	payload, err := json.Marshal(desired.UnsignedDesiredState)
	if err != nil {
		t.Fatalf("marshal updated desired state: %v", err)
	}
	desired.Signature = base64.RawStdEncoding.EncodeToString(ed25519.Sign(policyPrivateKey, payload))
	server := newManagerTestServer(t, desired, false)
	store := newManagerTestStore(t, server.URL, policyPublicKey, func(config *state.Config) {
		config.AppliedPolicyRevision = 11
		config.HermesVersion = "v-old"
	})
	intentPersisted := false
	adp := &fakeAdapter{installedVer: "v-old", running: true}
	adp.installHook = func(artifact protocol.DesiredStateArtifact) {
		config, loadErr := store.LoadConfig()
		if loadErr != nil {
			t.Errorf("load config during install: %v", loadErr)
			return
		}
		intentPersisted = config.UpdateInProgressArtifactID == artifact.ID &&
			config.UpdateInProgressPolicyRevision == desired.Revision &&
			config.UpdateInProgressVersion == artifact.Version
	}
	mgr := New(store, server.Client())
	mgr.now = func() time.Time { return now }
	mgr.SetAdapter(adp)

	if _, err := mgr.Sync(context.Background()); err != nil {
		t.Fatalf("sync update: %v", err)
	}
	if !intentPersisted {
		t.Fatal("update intent must be durable before Install stops the old process and switches current.json")
	}
	config, err := store.LoadConfig()
	if err != nil {
		t.Fatalf("load final config: %v", err)
	}
	if config.UpdateInProgressArtifactID != 0 || config.UpdateInProgressPolicyRevision != 0 || config.UpdateInProgressVersion != "" {
		t.Fatalf("successful update must clear durable intent: %+v", config)
	}
}
