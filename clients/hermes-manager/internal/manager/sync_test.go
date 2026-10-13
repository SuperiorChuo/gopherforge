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

	"github.com/go-admin-kit/clients/hermes-manager/internal/adapter"
	"github.com/go-admin-kit/clients/hermes-manager/internal/protocol"
	"github.com/go-admin-kit/clients/hermes-manager/internal/state"
)

type syncFixtureOptions struct {
	modelMode          string
	issuedToken        *protocol.ModelToken
	tokenEndpointError bool
	config             state.Config
	desired            protocol.UnsignedDesiredState
}

func newSyncFixture(t *testing.T, options syncFixtureOptions) (*Manager, *state.Store, *fakeAdapter, *int) {
	t.Helper()
	now := time.Now().UTC()
	if options.desired.SchemaVersion == 0 {
		options.desired = protocol.UnsignedDesiredState{
			SchemaVersion:  1,
			Revision:       9,
			DesiredState:   "active",
			LeaseExpiresAt: now.Add(time.Hour),
			Profile:        "company",
			Hermes: protocol.DesiredStateArtifact{
				ID:        1,
				Version:   "v2026.7.20",
				SHA256:    strings.Repeat("a", 64),
				SourceURL: "https://example.invalid/hermes.tar.gz",
			},
			Model:    protocol.DesiredStateModel{Mode: options.modelMode, BaseURL: "https://control.example/model", Model: "approved-model"},
			Toolsets: []string{"clarify", "memory", "no_mcp", "todo"},
		}
	}
	controlPublic, controlPrivate, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatalf("generate control key: %v", err)
	}
	payload, err := json.Marshal(options.desired)
	if err != nil {
		t.Fatalf("marshal desired state: %v", err)
	}
	signed := protocol.DesiredState{
		UnsignedDesiredState: options.desired,
		Signature:            base64.RawStdEncoding.EncodeToString(ed25519.Sign(controlPrivate, payload)),
	}
	tokenCalls := 0
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		switch request.URL.Path {
		case "/api/v1/ai/hermes/device/desired-state":
			_ = json.NewEncoder(writer).Encode(protocol.Envelope[protocol.DesiredState]{Code: http.StatusOK, Message: "success", Data: signed})
		case "/api/v1/ai/hermes/device/model-token":
			tokenCalls++
			if options.tokenEndpointError {
				writer.WriteHeader(http.StatusServiceUnavailable)
				_ = json.NewEncoder(writer).Encode(protocol.Envelope[map[string]any]{Code: http.StatusServiceUnavailable, Message: "temporarily unavailable", ErrorCode: "HERMES_MODEL_TOKEN_UNAVAILABLE"})
				return
			}
			if options.issuedToken == nil {
				t.Errorf("unexpected model-token request")
				writer.WriteHeader(http.StatusInternalServerError)
				return
			}
			_ = json.NewEncoder(writer).Encode(protocol.Envelope[protocol.ModelToken]{Code: http.StatusOK, Message: "success", Data: *options.issuedToken})
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
	options.config.ServerURL = server.URL
	options.config.DeviceID = "11111111-2222-4333-8444-555555555555"
	options.config.PublicKey = base64.RawStdEncoding.EncodeToString(devicePublic)
	options.config.PolicySigningPublicKey = base64.RawStdEncoding.EncodeToString(controlPublic)
	options.config.ManagerVersion = Version
	if err := store.SaveConfig(&options.config); err != nil {
		t.Fatalf("save config: %v", err)
	}

	manager := New(store, server.Client())
	manager.now = func() time.Time { return now }
	adp := &fakeAdapter{installedVer: options.config.HermesVersion, running: options.config.ActualState == "active"}
	manager.SetAdapter(adp)
	return manager, store, adp, &tokenCalls
}

func TestSyncFailsClosedWhenRequiredModelTokenRefreshFails(t *testing.T) {
	manager, store, adp, tokenCalls := newSyncFixture(t, syncFixtureOptions{
		modelMode:          "company_proxy",
		tokenEndpointError: true,
		config: state.Config{
			AppliedPolicyRevision: 8,
			ActualState:           "stopped",
			HealthStatus:          "healthy",
			HermesVersion:         "v2026.7.20",
			ModelTokenExpiresAt:   time.Now().Add(time.Hour),
		},
	})

	if _, err := manager.Sync(t.Context()); err == nil {
		t.Fatal("missing required token must make sync fail")
	}
	config, err := store.LoadConfig()
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	if *tokenCalls != 1 || config.ActualState != "stopped" || config.HealthStatus != "unhealthy" ||
		config.LastErrorCode != modelTokenRefreshFailed || config.AppliedPolicyRevision != 8 {
		t.Fatalf("fail-closed config/token calls = %+v / %d", config, *tokenCalls)
	}
	if callIndex(adp.calls, "start") >= 0 || callIndex(adp.calls, "install") >= 0 || callIndex(adp.calls, "apply_profile") >= 0 {
		t.Fatalf("missing required token must not start convergence: %v", adp.calls)
	}
}

func TestSyncReportsEarlyTokenRenewalFailureWithoutStoppingValidSession(t *testing.T) {
	expiresAt := time.Now().Add(5 * time.Minute)
	manager, store, adp, _ := newSyncFixture(t, syncFixtureOptions{
		modelMode:          "company_proxy",
		tokenEndpointError: true,
		config: state.Config{
			AppliedPolicyRevision: 9,
			ActualState:           "active",
			HealthStatus:          "healthy",
			HermesVersion:         "v2026.7.20",
			ModelTokenExpiresAt:   expiresAt,
		},
	})

	if _, err := manager.Sync(t.Context()); err == nil {
		t.Fatal("degraded token renewal must make sync fail")
	}
	config, err := store.LoadConfig()
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	if config.ActualState != "active" || config.HealthStatus != "degraded" || config.LastErrorCode != modelTokenRefreshFailed {
		t.Fatalf("renewal failure config = %+v", config)
	}
	if !config.ModelTokenExpiresAt.Equal(expiresAt) {
		t.Fatalf("still-valid token expiry changed from %v to %v", expiresAt, config.ModelTokenExpiresAt)
	}
	if callIndex(adp.calls, "stop") >= 0 || callIndex(adp.calls, "restart") >= 0 {
		t.Fatalf("early renewal failure must keep the valid session running: %v", adp.calls)
	}
}

func TestSyncStopsAndRetriesWhenIssuedTokenCannotBeApplied(t *testing.T) {
	oldExpiry := time.Now().Add(-time.Minute)
	newExpiry := time.Now().Add(30 * time.Minute)
	manager, store, adp, _ := newSyncFixture(t, syncFixtureOptions{
		modelMode:   "company_proxy",
		issuedToken: &protocol.ModelToken{Token: "fresh-token", ExpiresAt: newExpiry},
		config: state.Config{
			AppliedPolicyRevision: 9,
			ActualState:           "active",
			HealthStatus:          "healthy",
			HermesVersion:         "v2026.7.20",
			ModelTokenExpiresAt:   oldExpiry,
		},
	})
	adp.profileErr = adapter.Coded(adapter.ErrCodeProfileFailed, http.ErrAbortHandler)

	if _, err := manager.Sync(t.Context()); err == nil {
		t.Fatal("unapplied issued token must make sync fail")
	}
	config, err := store.LoadConfig()
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	if config.ActualState != "stopped" || config.HealthStatus != "unhealthy" || config.LastErrorCode != adapter.ErrCodeProfileFailed {
		t.Fatalf("unapplied token config = %+v", config)
	}
	if !config.ModelTokenExpiresAt.Equal(oldExpiry) {
		t.Fatalf("unusable new token expiry was persisted: old=%v got=%v new=%v", oldExpiry, config.ModelTokenExpiresAt, newExpiry)
	}
	if callIndex(adp.calls, "stop") < 0 || adp.running {
		t.Fatalf("process using revoked old token was not stopped: %v", adp.calls)
	}
}

func TestSyncPersistsUpdateCheckpointBeforeInstall(t *testing.T) {
	manager, store, adp, _ := newSyncFixture(t, syncFixtureOptions{
		modelMode: "byo",
		config: state.Config{
			AppliedPolicyRevision: 8,
			ActualState:           "active",
			HealthStatus:          "healthy",
			HermesVersion:         "v-old",
		},
	})
	adp.installHook = func(artifact protocol.DesiredStateArtifact) {
		config, err := store.LoadConfig()
		if err != nil {
			t.Fatalf("load checkpoint during install: %v", err)
		}
		if config.UpdateInProgressArtifactID != artifact.ID ||
			config.UpdateInProgressPolicyRevision != 9 || config.UpdateInProgressVersion != artifact.Version {
			t.Fatalf("install began without durable checkpoint: %+v", config)
		}
	}

	if _, err := manager.Sync(t.Context()); err != nil {
		t.Fatalf("sync: %v", err)
	}
	config, err := store.LoadConfig()
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	if config.UpdateInProgressArtifactID != 0 || config.UpdateInProgressPolicyRevision != 0 || config.UpdateInProgressVersion != "" {
		t.Fatalf("successful update did not clear checkpoint: %+v", config)
	}
}
