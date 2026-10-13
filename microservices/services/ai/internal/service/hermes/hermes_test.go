package service

import (
	"context"
	"crypto/ed25519"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	aiclient "github.com/go-admin-kit/services/ai/internal/ai"
	"github.com/go-admin-kit/services/ai/internal/dao"
	localmodel "github.com/go-admin-kit/services/ai/internal/model"
	model "github.com/go-admin-kit/services/shared/pkg/model"
	"github.com/go-admin-kit/services/shared/pkg/tenant"
	"gorm.io/gorm"
)

func setupHermesServiceTest(t *testing.T, withSigner bool) (*HermesService, *dao.HermesDAO, context.Context, *HermesPolicySigner) {
	t.Helper()
	dsn := fmt.Sprintf("file:hermes_service_%d?mode=memory&cache=shared", time.Now().UnixNano())
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := tenant.Register(db); err != nil {
		t.Fatalf("register tenant plugin: %v", err)
	}
	if err := db.AutoMigrate(
		&model.User{},
		&localmodel.HermesEntitlement{},
		&localmodel.HermesDeviceCode{},
		&localmodel.HermesDevice{},
		&localmodel.HermesPolicy{},
		&localmodel.HermesArtifact{},
		&localmodel.HermesDeviceEvent{},
		&localmodel.HermesInstance{},
		&localmodel.HermesCluster{},
		&localmodel.HermesTask{},
		&localmodel.HermesTaskRun{},
		&localmodel.HermesSkill{},
		&localmodel.HermesClusterSkill{},
		&localmodel.HermesUserSkill{},
		&localmodel.HermesMcpServer{},
		&localmodel.HermesClusterMcpServer{},
		&localmodel.HermesCronJob{},
		&localmodel.HermesCronRun{},
	); err != nil {
		t.Fatalf("migrate sqlite: %v", err)
	}
	if sqlDB, err := db.DB(); err == nil {
		sqlDB.SetMaxOpenConns(1)
		t.Cleanup(func() { _ = sqlDB.Close() })
	}

	ctx := tenant.WithContext(context.Background(), 1)
	if err := db.WithContext(ctx).Create(&model.User{
		TenantID: 1,
		Username: "hermes-user",
		Password: "unused-test-password-hash",
		Nickname: "Hermes User",
		Status:   1,
	}).Error; err != nil {
		t.Fatalf("create user: %v", err)
	}

	seed := make([]byte, ed25519.SeedSize)
	for index := range seed {
		seed[index] = byte(index + 1)
	}
	signer, err := NewHermesPolicySigner(base64.RawStdEncoding.EncodeToString(seed))
	if err != nil {
		t.Fatalf("create signer: %v", err)
	}
	configuredSigner := signer
	if !withSigner {
		configuredSigner = nil
	}
	store := dao.NewHermesDAO(db)
	svc := NewHermesService(store, configuredSigner, HermesServiceOptions{
		VerificationURI: "https://control.example/profile/hermes",
		Now: func() time.Time {
			return time.Date(2026, 7, 28, 12, 0, 0, 0, time.UTC)
		},
	})
	return svc, store, ctx, signer
}

func createHermesTestEntitlement(t *testing.T, svc *HermesService, ctx context.Context, enabled bool, policyID uint64) {
	t.Helper()
	if err := svc.UpdateEntitlement(ctx, 1, 1, UpdateHermesEntitlementRequest{
		Enabled:     enabled,
		PolicyID:    policyID,
		DeviceLimit: 1,
	}); err != nil {
		t.Fatalf("update entitlement: %v", err)
	}
}

func createHermesTestAuthorization(t *testing.T, svc *HermesService) (*HermesDeviceAuthorizationResponse, ed25519.PrivateKey) {
	t.Helper()
	publicKey, privateKey, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatalf("generate device key: %v", err)
	}
	result, err := svc.CreateDeviceAuthorization(context.Background(), CreateHermesDeviceAuthorizationRequest{
		PublicKey:  base64.RawStdEncoding.EncodeToString(publicKey),
		DeviceName: "Work laptop",
		Platform:   "darwin",
		Arch:       "arm64",
	})
	if err != nil {
		t.Fatalf("create authorization: %v", err)
	}
	return result, privateKey
}

func TestHermesEnrollmentIsSingleUseAndEnforcesDeviceLimit(t *testing.T) {
	svc, _, tenantCtx, _ := setupHermesServiceTest(t, true)
	createHermesTestEntitlement(t, svc, tenantCtx, true, 0)

	authorization, _ := createHermesTestAuthorization(t, svc)
	if err := svc.ApproveDeviceAuthorization(tenantCtx, 1, 1, strings.ToLower(authorization.UserCode)); err != nil {
		t.Fatalf("approve authorization: %v", err)
	}
	token, err := svc.ExchangeDeviceCode(context.Background(), authorization.DeviceCode)
	if err != nil {
		t.Fatalf("exchange device code: %v", err)
	}
	if token.DeviceID == "" || token.PolicySigningPublicKey == "" {
		t.Fatalf("exchange result = %+v, want device identity and signing key", token)
	}
	if _, err := svc.ExchangeDeviceCode(context.Background(), authorization.DeviceCode); !errors.Is(err, dao.ErrHermesAuthorizationDenied) {
		t.Fatalf("second exchange error = %v, want authorization denied", err)
	}

	secondAuthorization, _ := createHermesTestAuthorization(t, svc)
	if err := svc.ApproveDeviceAuthorization(tenantCtx, 1, 1, secondAuthorization.UserCode); err != nil {
		t.Fatalf("approve second authorization: %v", err)
	}
	if _, err := svc.ExchangeDeviceCode(context.Background(), secondAuthorization.DeviceCode); !errors.Is(err, dao.ErrHermesDeviceLimitReached) {
		t.Fatalf("second device exchange error = %v, want device limit", err)
	}
}

func TestHermesEnrollmentRequiresEntitlementAndConfiguredSigner(t *testing.T) {
	svc, store, tenantCtx, signer := setupHermesServiceTest(t, false)
	authorization, _ := createHermesTestAuthorization(t, svc)
	if err := svc.ApproveDeviceAuthorization(tenantCtx, 1, 1, authorization.UserCode); !errors.Is(err, dao.ErrHermesEntitlementDisabled) {
		t.Fatalf("approve without entitlement error = %v, want entitlement disabled", err)
	}

	createHermesTestEntitlement(t, svc, tenantCtx, true, 0)
	if err := svc.ApproveDeviceAuthorization(tenantCtx, 1, 1, authorization.UserCode); err != nil {
		t.Fatalf("approve with entitlement: %v", err)
	}
	if _, err := svc.ExchangeDeviceCode(context.Background(), authorization.DeviceCode); !errors.Is(err, ErrHermesControlPlaneUnavailable) {
		t.Fatalf("exchange without signer error = %v, want unavailable", err)
	}

	configured := NewHermesService(store, signer, HermesServiceOptions{
		Now: svc.now,
	})
	if _, err := configured.ExchangeDeviceCode(context.Background(), authorization.DeviceCode); err != nil {
		t.Fatalf("exchange after configuring signer: %v", err)
	}
}

func TestRevokedDeviceConvergesBeforeActualStateIsConfirmed(t *testing.T) {
	svc, store, ctx, _ := setupHermesServiceTest(t, true)
	currentTime := svc.now().UTC()
	svc.now = func() time.Time { return currentTime }
	policy := createHermesTestPolicy(t, svc, ctx, "revocation-policy", "approved-model")
	createHermesTestEntitlement(t, svc, ctx, true, policy.ID)
	device := enrollHermesTestDevice(t, svc, store, ctx)

	activeHeartbeat := HermesHeartbeatRequest{
		ManagerVersion:        "0.1.0-dev",
		HermesVersion:         "v2026.7.20",
		ActualState:           localmodel.HermesDesiredStateActive,
		HealthStatus:          "healthy",
		AppliedPolicyRevision: policy.Revision,
	}
	if err := svc.RecordHeartbeat(ctx, device.DeviceID, activeHeartbeat); err != nil {
		t.Fatalf("record active heartbeat: %v", err)
	}
	if err := svc.RevokeDevice(ctx, device.DeviceID, nil); err != nil {
		t.Fatalf("revoke device: %v", err)
	}

	revokedDevice, err := store.GetDevice(ctx, device.DeviceID)
	if err != nil {
		t.Fatalf("get revoked device: %v", err)
	}
	if revokedDevice.RevokedAt == nil {
		t.Fatal("revoked_at must be set immediately")
	}
	if revokedDevice.ActualState != localmodel.HermesDesiredStateActive {
		t.Fatalf("actual state before manager confirmation = %q, want active", revokedDevice.ActualState)
	}
	views, total, err := svc.ListDevices(ctx, 1, 20, nil)
	if err != nil {
		t.Fatalf("list devices after revocation: %v", err)
	}
	if total != 1 || len(views) != 1 {
		t.Fatalf("device views total/list = %d/%d, want 1/1", total, len(views))
	}
	if views[0].DesiredState != localmodel.HermesDesiredStateRevoked ||
		views[0].ActualState != localmodel.HermesDesiredStateActive ||
		views[0].ConnectionStatus != "online" {
		t.Fatalf("pending revocation view = %+v", views[0])
	}
	desired, err := svc.DesiredState(ctx, revokedDevice)
	if err != nil {
		t.Fatalf("desired state after revocation: %v", err)
	}
	if desired.DesiredState != localmodel.HermesDesiredStateRevoked {
		t.Fatalf("desired state = %q, want revoked", desired.DesiredState)
	}

	if err := svc.RecordHeartbeat(ctx, device.DeviceID, activeHeartbeat); !errors.Is(err, dao.ErrHermesDeviceRevoked) {
		t.Fatalf("stale active heartbeat error = %v, want device revoked", err)
	}
	if err := svc.RecordEvent(ctx, device.DeviceID, HermesEventRequest{EventType: "start", Result: "succeeded"}); !errors.Is(err, ErrHermesInvalidInput) {
		t.Fatalf("non-revoke event after revocation error = %v, want invalid input", err)
	}
	if err := svc.RecordEvent(ctx, device.DeviceID, HermesEventRequest{EventType: "revoke", Result: "succeeded"}); err != nil {
		t.Fatalf("record revoke confirmation event: %v", err)
	}
	identity, err := store.GetDeviceIdentity(context.Background(), device.DeviceID)
	if err != nil {
		t.Fatalf("get identity before final heartbeat: %v", err)
	}
	if identity.RevocationConfirmed {
		t.Fatal("terminal event alone must not close the final heartbeat channel")
	}
	currentTime = currentTime.Add(time.Second)
	revokedHeartbeat := activeHeartbeat
	revokedHeartbeat.ActualState = localmodel.HermesDesiredStateRevoked
	revokedHeartbeat.HealthStatus = "unknown"
	if err := svc.RecordHeartbeat(ctx, device.DeviceID, revokedHeartbeat); err != nil {
		t.Fatalf("record final revoked heartbeat: %v", err)
	}

	confirmed, err := store.GetDevice(ctx, device.DeviceID)
	if err != nil {
		t.Fatalf("get confirmed revoked device: %v", err)
	}
	if confirmed.ActualState != localmodel.HermesDesiredStateRevoked || confirmed.LastSeenAt == nil {
		t.Fatalf("confirmed device = %+v, want revoked with last_seen_at", confirmed)
	}
	staleSeenAt := svc.now().UTC().Add(-hermesDeviceOnlineWindow - time.Second)
	if err := store.DB().WithContext(ctx).Model(&localmodel.HermesDevice{}).
		Where("device_id = ?", device.DeviceID).
		Update("last_seen_at", staleSeenAt).Error; err != nil {
		t.Fatalf("make heartbeat stale: %v", err)
	}
	views, _, err = svc.ListDevices(ctx, 1, 20, nil)
	if err != nil {
		t.Fatalf("list stale device: %v", err)
	}
	if len(views) != 1 || views[0].ConnectionStatus != "offline" {
		t.Fatalf("stale device views = %+v, want offline", views)
	}
	events, err := store.ListDeviceEvents(ctx, device.DeviceID, 10)
	if err != nil {
		t.Fatalf("list revocation events: %v", err)
	}
	if len(events) < 2 ||
		events[0].EventType != "revoke" || events[0].Result != "succeeded" ||
		events[1].EventType != "revoke" || events[1].Result != "started" {
		t.Fatalf("revocation events = %+v, want manager succeeded then server started", events)
	}
}

func TestPolicyRevokedStateCanBeReportedWithoutDeviceRevocation(t *testing.T) {
	svc, store, ctx, _ := setupHermesServiceTest(t, true)
	currentTime := svc.now().UTC()
	svc.now = func() time.Time { return currentTime }
	policy, err := svc.CreatePolicy(ctx, 1, CreateHermesPolicyRequest{
		Name:         "reversible-revoked-policy",
		DesiredState: localmodel.HermesDesiredStateRevoked,
		ModelName:    "approved-model",
		ModelBaseURL: "https://control.example/api/v1/ai/hermes/proxy/v1",
		Toolsets:     localmodel.HermesToolsets{"clarify", "memory", "no_mcp", "todo"},
		Privacy:      localmodel.HermesPrivacy{},
		LeaseSeconds: 3600,
	})
	if err != nil {
		t.Fatalf("create revoked policy: %v", err)
	}
	createHermesTestEntitlement(t, svc, ctx, true, policy.ID)
	device := enrollHermesTestDevice(t, svc, store, ctx)

	desired, err := svc.DesiredState(ctx, device)
	if err != nil {
		t.Fatalf("get revoked desired state: %v", err)
	}
	if desired.DesiredState != localmodel.HermesDesiredStateRevoked || device.RevokedAt != nil {
		t.Fatalf("desired/device = %q/%v, want reversible policy revocation", desired.DesiredState, device.RevokedAt)
	}
	if err := svc.RecordHeartbeat(ctx, device.DeviceID, HermesHeartbeatRequest{
		ManagerVersion:        "0.1.0-dev",
		HermesVersion:         "v2026.7.20",
		ActualState:           localmodel.HermesDesiredStateRevoked,
		HealthStatus:          "unknown",
		AppliedPolicyRevision: policy.Revision,
	}); err != nil {
		t.Fatalf("record policy-revoked heartbeat: %v", err)
	}

	reported, err := store.GetDevice(ctx, device.DeviceID)
	if err != nil {
		t.Fatalf("get reported device: %v", err)
	}
	if reported.ActualState != localmodel.HermesDesiredStateRevoked || reported.RevokedAt != nil {
		t.Fatalf("reported state/revoked_at = %q/%v, want revoked state without permanent revocation", reported.ActualState, reported.RevokedAt)
	}
	// A policy reconciliation may have emitted the same terminal event before
	// permanent revocation. It must not confirm a later administrator request.
	if err := svc.RecordEvent(ctx, device.DeviceID, HermesEventRequest{EventType: "revoke", Result: "succeeded"}); err != nil {
		t.Fatalf("record policy revoke event: %v", err)
	}
	currentTime = currentTime.Add(time.Second)
	if err := svc.RevokeDevice(ctx, device.DeviceID, nil); err != nil {
		t.Fatalf("permanently revoke policy-revoked device: %v", err)
	}
	identity, err := store.GetDeviceIdentity(context.Background(), device.DeviceID)
	if err != nil {
		t.Fatalf("get pending revoked identity: %v", err)
	}
	if identity.RevocationConfirmed {
		t.Fatal("policy revoke event must not confirm the later permanent revocation")
	}

	if err := svc.RecordEvent(ctx, device.DeviceID, HermesEventRequest{
		EventType: "stop",
		Result:    "failed",
		ErrorCode: "PROCESS_STOP_FAILED",
	}); err != nil {
		t.Fatalf("record revoked stop failure: %v", err)
	}
	if err := svc.RecordHeartbeat(ctx, device.DeviceID, HermesHeartbeatRequest{
		ManagerVersion:        "0.1.0-dev",
		HermesVersion:         "v2026.7.20",
		ActualState:           "error",
		HealthStatus:          "unhealthy",
		AppliedPolicyRevision: policy.Revision,
		LastErrorCode:         "PROCESS_STOP_FAILED",
	}); err != nil {
		t.Fatalf("record revoked error heartbeat: %v", err)
	}
	failed, err := store.GetDevice(ctx, device.DeviceID)
	if err != nil {
		t.Fatalf("get failed revoked device: %v", err)
	}
	if failed.ActualState != "error" || failed.LastErrorCode != "PROCESS_STOP_FAILED" {
		t.Fatalf("failed revoked device = %+v, want visible stop error", failed)
	}

	currentTime = currentTime.Add(time.Second)
	identity, err = store.GetDeviceIdentity(context.Background(), device.DeviceID)
	if err != nil {
		t.Fatalf("get identity before permanent revoke heartbeat: %v", err)
	}
	if identity.RevocationConfirmed {
		t.Fatal("error heartbeat must leave the permanent revocation channel open")
	}
	currentTime = currentTime.Add(time.Second)
	if err := svc.RecordHeartbeat(ctx, device.DeviceID, HermesHeartbeatRequest{
		ManagerVersion:        "0.1.0-dev",
		HermesVersion:         "v2026.7.20",
		ActualState:           localmodel.HermesDesiredStateRevoked,
		HealthStatus:          "unknown",
		AppliedPolicyRevision: policy.Revision,
	}); err != nil {
		t.Fatalf("record permanent revoke heartbeat: %v", err)
	}
	identity, err = store.GetDeviceIdentity(context.Background(), device.DeviceID)
	if err != nil {
		t.Fatalf("get confirmed revoked identity: %v", err)
	}
	if !identity.RevocationConfirmed {
		t.Fatal("post-revocation revoked heartbeat must confirm even when the terminal event was lost")
	}
}

func TestGetMeReturnsEnrichedDeviceViews(t *testing.T) {
	svc, store, ctx, _ := setupHermesServiceTest(t, true)
	policy := createHermesTestPolicy(t, svc, ctx, "employee-view-policy", "approved-model")
	createHermesTestEntitlement(t, svc, ctx, true, policy.ID)
	device := enrollHermesTestDevice(t, svc, store, ctx)

	result, err := svc.GetMe(ctx, device.UserID)
	if err != nil {
		t.Fatalf("get Hermes me: %v", err)
	}
	if len(result.Devices) != 1 || result.Devices[0].DesiredState != localmodel.HermesDesiredStateActive ||
		result.Devices[0].ConnectionStatus != "never" {
		t.Fatalf("Hermes me devices = %+v, want active/never view", result.Devices)
	}
	payload, err := json.Marshal(result)
	if err != nil {
		t.Fatalf("marshal Hermes me: %v", err)
	}
	if !strings.Contains(string(payload), `"desired_state":"active"`) ||
		!strings.Contains(string(payload), `"connection_status":"never"`) {
		t.Fatalf("Hermes me JSON is missing device view fields: %s", payload)
	}
}

func TestRevokeDeviceRollsBackWhenAuditEventFails(t *testing.T) {
	svc, store, ctx, _ := setupHermesServiceTest(t, true)
	policy := createHermesTestPolicy(t, svc, ctx, "revocation-rollback-policy", "approved-model")
	createHermesTestEntitlement(t, svc, ctx, true, policy.ID)
	device := enrollHermesTestDevice(t, svc, store, ctx)

	if err := store.DB().Migrator().DropTable(&localmodel.HermesDeviceEvent{}); err != nil {
		t.Fatalf("drop event table: %v", err)
	}
	if err := svc.RevokeDevice(ctx, device.DeviceID, nil); err == nil {
		t.Fatal("revoke device should fail when its audit event cannot be recorded")
	}

	unchanged, err := store.GetDevice(ctx, device.DeviceID)
	if err != nil {
		t.Fatalf("get device after failed revocation: %v", err)
	}
	if unchanged.RevokedAt != nil {
		t.Fatalf("revoked_at = %v, want rollback after audit event failure", unchanged.RevokedAt)
	}
}

func TestDeviceViewsSurviveRevokedArtifact(t *testing.T) {
	svc, store, ctx, _ := setupHermesServiceTest(t, true)
	artifact, err := svc.CreateArtifact(ctx, CreateHermesArtifactRequest{
		Kind:           "hermes",
		Platform:       "darwin",
		Arch:           "arm64",
		Version:        "v2026.7.20-view-test",
		UpstreamCommit: "1cd8f8c37e152cc6938a567c6d368e95ad1588ef",
		SourceURL:      "https://example.com/hermes-view-test.tar.gz",
		SHA256:         strings.Repeat("c", 64),
	})
	if err != nil {
		t.Fatalf("create artifact: %v", err)
	}
	if err := svc.ApproveArtifact(ctx, artifact.ID, 1); err != nil {
		t.Fatalf("approve artifact: %v", err)
	}
	policy, err := svc.CreatePolicy(ctx, 1, CreateHermesPolicyRequest{
		Name:         "revoked-artifact-policy",
		DesiredState: localmodel.HermesDesiredStateActive,
		ArtifactID:   artifact.ID,
		ModelName:    "approved-model",
		ModelBaseURL: "https://control.example/api/v1/ai/hermes/proxy/v1",
		Toolsets:     localmodel.HermesToolsets{"clarify", "memory", "no_mcp", "todo"},
		Privacy:      localmodel.HermesPrivacy{},
		LeaseSeconds: 3600,
	})
	if err != nil {
		t.Fatalf("create policy: %v", err)
	}
	createHermesTestEntitlement(t, svc, ctx, true, policy.ID)
	device := enrollHermesTestDevice(t, svc, store, ctx)
	if err := svc.RevokeArtifact(ctx, artifact.ID, 1); err != nil {
		t.Fatalf("revoke artifact: %v", err)
	}

	views, total, err := svc.ListDevices(ctx, 1, 20, nil)
	if err != nil {
		t.Fatalf("list devices with revoked artifact: %v", err)
	}
	if total != 1 || len(views) != 1 || views[0].DesiredState != "unknown" {
		t.Fatalf("device views total/list = %d/%+v, want one device with unknown desired state", total, views)
	}
	detail, _, err := svc.GetDevice(ctx, device.DeviceID)
	if err != nil {
		t.Fatalf("get device with revoked artifact: %v", err)
	}
	if detail.DesiredState != "unknown" {
		t.Fatalf("detail desired state = %q, want unknown", detail.DesiredState)
	}
}

func TestHermesPolicyAndDesiredStateAreSignedAndPrivacyLocked(t *testing.T) {
	svc, store, tenantCtx, signer := setupHermesServiceTest(t, true)
	policy, err := svc.CreatePolicy(tenantCtx, 1, CreateHermesPolicyRequest{
		Name:         "P1 company policy",
		DesiredState: localmodel.HermesDesiredStateActive,
		ModelName:    "approved-model",
		ModelBaseURL: "https://control.example/api/v1/ai/hermes/proxy/v1",
		Toolsets:     localmodel.HermesToolsets{"todo", "clarify", "no_mcp", "memory"},
		Privacy:      localmodel.HermesPrivacy{},
		LeaseSeconds: 3600,
	})
	if err != nil {
		t.Fatalf("create policy: %v", err)
	}
	if policy.Revision != 1 || policy.Signature == "" {
		t.Fatalf("policy = %+v, want revision and signature", policy)
	}

	_, err = svc.CreatePolicy(tenantCtx, 1, CreateHermesPolicyRequest{
		Name:         "unsafe",
		DesiredState: localmodel.HermesDesiredStateActive,
		Toolsets:     localmodel.HermesToolsets{"clarify", "memory", "no_mcp", "todo"},
		Privacy:      localmodel.HermesPrivacy{UploadConversations: true},
		LeaseSeconds: 3600,
	})
	if !errors.Is(err, ErrHermesInvalidInput) {
		t.Fatalf("unsafe privacy error = %v, want invalid input", err)
	}

	createHermesTestEntitlement(t, svc, tenantCtx, true, policy.ID)
	authorization, _ := createHermesTestAuthorization(t, svc)
	if err := svc.ApproveDeviceAuthorization(tenantCtx, 1, 1, authorization.UserCode); err != nil {
		t.Fatalf("approve authorization: %v", err)
	}
	token, err := svc.ExchangeDeviceCode(context.Background(), authorization.DeviceCode)
	if err != nil {
		t.Fatalf("exchange device code: %v", err)
	}
	device, err := store.GetDevice(tenantCtx, token.DeviceID)
	if err != nil {
		t.Fatalf("get device: %v", err)
	}
	state, err := svc.DesiredState(tenantCtx, device)
	if err != nil {
		t.Fatalf("desired state: %v", err)
	}
	encoded, err := json.Marshal(state)
	if err != nil {
		t.Fatalf("marshal desired state: %v", err)
	}
	for _, field := range []string{`"schema_version":1`, `"desired_state":"active"`, `"profile":"company"`, `"signature":"`} {
		if !strings.Contains(string(encoded), field) {
			t.Fatalf("desired state JSON %s missing %s", encoded, field)
		}
	}

	unsigned, err := json.Marshal(state.hermesUnsignedDesiredState)
	if err != nil {
		t.Fatalf("marshal unsigned state: %v", err)
	}
	signature, err := base64.RawStdEncoding.DecodeString(state.Signature)
	if err != nil {
		t.Fatalf("decode signature: %v", err)
	}
	if !ed25519.Verify(signer.publicKey, unsigned, signature) {
		t.Fatal("desired state signature did not verify")
	}
}

// 制品录入的校验直接决定「员工电脑会下载并执行什么」，这里把安全相关的
// 几条钉住：摘要格式、https、不可变版本、Hermes 必须钉 commit。
func TestCreateArtifactRejectsUnsafeInput(t *testing.T) {
	svc, _, ctx, _ := setupHermesServiceTest(t, true)

	valid := CreateHermesArtifactRequest{
		Kind:           "hermes",
		Platform:       "linux",
		Arch:           "amd64",
		Version:        "v2026.7.20",
		UpstreamCommit: "1cd8f8c37e152cc6938a567c6d368e95ad1588ef",
		SourceURL:      "https://example.com/hermes.tar.gz",
		SHA256:         strings.Repeat("a", 64),
	}

	if _, err := svc.CreateArtifact(ctx, valid); err != nil {
		t.Fatalf("valid artifact should be accepted: %v", err)
	}

	cases := []struct {
		name   string
		mutate func(*CreateHermesArtifactRequest)
	}{
		{"短摘要", func(r *CreateHermesArtifactRequest) { r.SHA256 = "abc123" }},
		{"非十六进制摘要", func(r *CreateHermesArtifactRequest) { r.SHA256 = strings.Repeat("z", 64) }},
		{"明文 http 来源", func(r *CreateHermesArtifactRequest) { r.SourceURL = "http://example.com/x.tar.gz" }},
		{"file 协议来源", func(r *CreateHermesArtifactRequest) { r.SourceURL = "file:///etc/passwd" }},
		{"移动引用当版本", func(r *CreateHermesArtifactRequest) { r.Version = "latest" }},
		{"main 当版本", func(r *CreateHermesArtifactRequest) { r.Version = "main" }},
		{"Unix 父目录版本", func(r *CreateHermesArtifactRequest) { r.Version = "../profile" }},
		{"Windows 父目录版本", func(r *CreateHermesArtifactRequest) { r.Version = `..\profile` }},
		{"点目录版本", func(r *CreateHermesArtifactRequest) { r.Version = "." }},
		{"隐藏目录版本", func(r *CreateHermesArtifactRequest) { r.Version = ".hidden" }},
		{"尾随点版本", func(r *CreateHermesArtifactRequest) { r.Version = "v1." }},
		{"含空格版本", func(r *CreateHermesArtifactRequest) { r.Version = "v1 beta" }},
		{"hermes 未钉 commit", func(r *CreateHermesArtifactRequest) { r.UpstreamCommit = "" }},
		{"非法 kind", func(r *CreateHermesArtifactRequest) { r.Kind = "shellscript" }},
		{"非法平台", func(r *CreateHermesArtifactRequest) { r.Platform = "solaris" }},
	}
	for index, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req := valid
			req.Version = fmt.Sprintf("%s-case-%d", valid.Version, index) // 避开唯一索引
			tc.mutate(&req)
			if _, err := svc.CreateArtifact(ctx, req); err == nil {
				t.Fatalf("%s 应被拒绝但通过了", tc.name)
			}
		})
	}
}

// 同一 kind/platform/arch 下版本号唯一：防止同版本号指向不同摘要。
func TestCreateArtifactRejectsDuplicateRelease(t *testing.T) {
	svc, _, ctx, _ := setupHermesServiceTest(t, true)
	req := CreateHermesArtifactRequest{
		Kind:      "manager",
		Platform:  "darwin",
		Arch:      "arm64",
		Version:   "v1.0.0",
		SourceURL: "https://example.com/m.tar.gz",
		SHA256:    strings.Repeat("b", 64),
	}
	if _, err := svc.CreateArtifact(ctx, req); err != nil {
		t.Fatalf("first create: %v", err)
	}
	if _, err := svc.CreateArtifact(ctx, req); !errors.Is(err, dao.ErrHermesArtifactDuplicate) {
		t.Fatalf("重复录入应返回 ErrHermesArtifactDuplicate，实际 %v", err)
	}
}

// 运维动作的安全语义：控制面只能下发固定枚举，动作进签名载荷，
// manager 回执终态后清除。这三条一起保证「系统不具备下发任意命令的能力」。
func TestDeviceActionIsEnumeratedSignedAndCleared(t *testing.T) {
	svc, store, ctx, signer := setupHermesServiceTest(t, true)
	policy, err := svc.CreatePolicy(ctx, 1, CreateHermesPolicyRequest{
		Name:         "action policy",
		DesiredState: localmodel.HermesDesiredStateActive,
		Toolsets:     localmodel.HermesToolsets{"clarify", "memory", "no_mcp", "todo"},
		Privacy:      localmodel.HermesPrivacy{},
		LeaseSeconds: 3600,
	})
	if err != nil {
		t.Fatalf("create policy: %v", err)
	}
	createHermesTestEntitlement(t, svc, ctx, true, policy.ID)
	authorization, _ := createHermesTestAuthorization(t, svc)
	if err := svc.ApproveDeviceAuthorization(ctx, 1, 1, authorization.UserCode); err != nil {
		t.Fatalf("approve: %v", err)
	}
	token, err := svc.ExchangeDeviceCode(context.Background(), authorization.DeviceCode)
	if err != nil {
		t.Fatalf("exchange: %v", err)
	}

	// 任意命令必须被拒绝——这是「不提供通用命令通道」的核心保证
	for _, bad := range []string{"rm -rf /", "shell", "exec", "curl http://x | sh", "RETRY_UPDATE; ls"} {
		if err := svc.RequestDeviceAction(ctx, token.DeviceID, bad); !errors.Is(err, ErrHermesInvalidInput) {
			t.Fatalf("action %q should be rejected, got %v", bad, err)
		}
	}

	if err := svc.RequestDeviceAction(ctx, token.DeviceID, "retry_update"); err != nil {
		t.Fatalf("request action: %v", err)
	}

	device, err := store.GetDevice(ctx, token.DeviceID)
	if err != nil {
		t.Fatalf("get device: %v", err)
	}
	state, err := svc.DesiredState(ctx, device)
	if err != nil {
		t.Fatalf("desired state: %v", err)
	}
	if state.PendingAction != "retry_update" {
		t.Fatalf("pending action = %q, want retry_update", state.PendingAction)
	}
	// 动作必须在签名覆盖范围内，否则中间人可以凭空注入动作
	unsigned, _ := json.Marshal(state.hermesUnsignedDesiredState)
	signature, _ := base64.RawStdEncoding.DecodeString(state.Signature)
	if !ed25519.Verify(signer.publicKey, unsigned, signature) {
		t.Fatal("desired state with pending action did not verify")
	}
	if !strings.Contains(string(unsigned), `"pending_action":"retry_update"`) {
		t.Fatalf("signed payload missing pending_action: %s", unsigned)
	}

	// started 不清除：执行中途崩溃时动作要留着
	if err := svc.RecordEvent(ctx, token.DeviceID, HermesEventRequest{EventType: "update", Result: "started"}); err != nil {
		t.Fatalf("record started: %v", err)
	}
	device, _ = store.GetDevice(ctx, token.DeviceID)
	if device.PendingAction != "retry_update" {
		t.Fatalf("started should not clear action, got %q", device.PendingAction)
	}

	// 终态清除
	if err := svc.RecordEvent(ctx, token.DeviceID, HermesEventRequest{EventType: "update", Result: "succeeded"}); err != nil {
		t.Fatalf("record succeeded: %v", err)
	}
	device, _ = store.GetDevice(ctx, token.DeviceID)
	if device.PendingAction != "" {
		t.Fatalf("succeeded should clear action, got %q", device.PendingAction)
	}
}

// 模型代理的全部意义在于「停用可立即强制」。这里钉住三条：token 有效期内
// 员工被停用 → 立即拒绝；设备被吊销 → 立即拒绝；无效 token → 拒绝。
func TestModelProxyEnforcesRevocationImmediately(t *testing.T) {
	svc, store, ctx, _ := setupHermesServiceTest(t, true)
	db := store.DB()
	if err := db.AutoMigrate(&localmodel.HermesModelToken{}); err != nil {
		t.Fatalf("migrate tokens: %v", err)
	}
	tokenDAO := dao.NewHermesModelTokenDAO(db)
	proxy := NewHermesProxyService(svc, tokenDAO, nil, aiclient.Providers{}, nil, nil, nil, func() time.Time {
		return time.Date(2026, 7, 29, 12, 0, 0, 0, time.UTC)
	})
	svc.SetModelTokenRevoker(proxy)

	policy, err := svc.CreatePolicy(ctx, 1, CreateHermesPolicyRequest{
		Name:         "proxy policy",
		DesiredState: localmodel.HermesDesiredStateActive,
		Toolsets:     localmodel.HermesToolsets{"clarify", "memory", "no_mcp", "todo"},
		Privacy:      localmodel.HermesPrivacy{},
		LeaseSeconds: 3600,
	})
	if err != nil {
		t.Fatalf("create policy: %v", err)
	}
	createHermesTestEntitlement(t, svc, ctx, true, policy.ID)
	authorization, _ := createHermesTestAuthorization(t, svc)
	if err := svc.ApproveDeviceAuthorization(ctx, 1, 1, authorization.UserCode); err != nil {
		t.Fatalf("approve: %v", err)
	}
	issued, err := svc.ExchangeDeviceCode(context.Background(), authorization.DeviceCode)
	if err != nil {
		t.Fatalf("exchange: %v", err)
	}
	device, err := store.GetDevice(ctx, issued.DeviceID)
	if err != nil {
		t.Fatalf("get device: %v", err)
	}

	tok, err := proxy.IssueModelToken(ctx, device)
	if err != nil {
		t.Fatalf("issue token: %v", err)
	}
	if !strings.HasPrefix(tok.Token, "hmt_") || tok.Token == "" {
		t.Fatalf("unexpected token %q", tok.Token)
	}

	// 正常路径可用
	if _, _, err := proxy.Authorize(ctx, "Bearer "+tok.Token); err != nil {
		t.Fatalf("authorize should succeed: %v", err)
	}

	// 乱造的 token 必须拒绝
	if _, _, err := proxy.Authorize(ctx, "Bearer hmt_forged"); !errors.Is(err, ErrHermesProxyUnauthorized) {
		t.Fatalf("forged token error = %v, want unauthorized", err)
	}

	// 停用员工权益 → token 还在有效期内，但必须立刻拒绝
	if err := svc.UpdateEntitlement(ctx, 1, 1, UpdateHermesEntitlementRequest{
		Enabled: false, PolicyID: policy.ID, DeviceLimit: 1,
	}); err != nil {
		t.Fatalf("disable entitlement: %v", err)
	}
	if _, _, err := proxy.Authorize(ctx, "Bearer "+tok.Token); err == nil {
		t.Fatal("disabled entitlement must block model access immediately")
	}

	// 恢复权益后换新 token，再验证设备吊销同样立即生效
	if err := svc.UpdateEntitlement(ctx, 1, 1, UpdateHermesEntitlementRequest{
		Enabled: true, PolicyID: policy.ID, DeviceLimit: 1,
	}); err != nil {
		t.Fatalf("re-enable: %v", err)
	}
	tok2, err := proxy.IssueModelToken(ctx, device)
	if err != nil {
		t.Fatalf("issue token 2: %v", err)
	}
	if _, _, err := proxy.Authorize(ctx, "Bearer "+tok2.Token); err != nil {
		t.Fatalf("authorize after re-enable: %v", err)
	}

	// 换发即吊旧：一设备一活跃 token。旧 token 在 TTL 内也必须立即失效，
	// 否则 token 被窃后连「换发止损」都做不到。
	tok3, err := proxy.IssueModelToken(ctx, device)
	if err != nil {
		t.Fatalf("issue token 3: %v", err)
	}
	if _, _, err := proxy.Authorize(ctx, "Bearer "+tok2.Token); err == nil {
		t.Fatal("rotated-out token must be rejected immediately")
	}
	if _, _, err := proxy.Authorize(ctx, "Bearer "+tok3.Token); err != nil {
		t.Fatalf("authorize latest token: %v", err)
	}

	if err := svc.RevokeDevice(ctx, device.DeviceID, nil); err != nil {
		t.Fatalf("revoke device: %v", err)
	}
	if _, _, err := proxy.Authorize(ctx, "Bearer "+tok3.Token); err == nil {
		t.Fatal("revoked device must block model access immediately")
	}
}

func TestModelProxyUsesTheSameInstancePolicyAsDesiredState(t *testing.T) {
	svc, store, ctx, _ := setupHermesServiceTest(t, true)
	db := store.DB()
	if err := db.AutoMigrate(&localmodel.HermesModelToken{}); err != nil {
		t.Fatalf("migrate tokens: %v", err)
	}
	proxy := NewHermesProxyService(
		svc,
		dao.NewHermesModelTokenDAO(db),
		nil,
		aiclient.Providers{},
		nil,
		nil,
		nil,
		func() time.Time { return time.Date(2026, 7, 29, 12, 0, 0, 0, time.UTC) },
	)
	entitlementPolicy := createHermesTestPolicy(t, svc, ctx, "proxy-entitlement", "entitlement-model")
	instancePolicy := createHermesTestPolicy(t, svc, ctx, "proxy-instance", "instance-model")
	createHermesTestEntitlement(t, svc, ctx, true, entitlementPolicy.ID)
	device := enrollHermesTestDevice(t, svc, store, ctx)
	if _, err := svc.CreateInstance(ctx, 1, HermesInstanceRequest{
		Name:        "proxy-agent",
		Role:        "default",
		OwnerUserID: device.UserID,
		DeviceID:    device.DeviceID,
		PolicyID:    instancePolicy.ID,
		Enabled:     true,
	}); err != nil {
		t.Fatalf("create instance: %v", err)
	}
	token, err := proxy.IssueModelToken(ctx, device)
	if err != nil {
		t.Fatalf("issue token: %v", err)
	}
	caller, _, err := proxy.Authorize(ctx, "Bearer "+token.Token)
	if err != nil {
		t.Fatalf("authorize token: %v", err)
	}
	if caller.Policy == nil || caller.Policy.ID != instancePolicy.ID {
		t.Fatalf("proxy policy = %+v, want instance policy %d", caller.Policy, instancePolicy.ID)
	}
}

// 模型授权必须由策略决定，而不是由设备请求体里的 model 字段决定——否则员工
// 改一行本地配置就能越权用别的模型。这里钉住：策略引用的模型被禁用后拒绝，
// 且策略发布时就校验模型引用而不是等设备调用才暴露。
func TestPolicyModelReferenceIsValidatedAndEnforced(t *testing.T) {
	svc, store, ctx, _ := setupHermesServiceTest(t, true)
	db := store.DB()
	if err := db.AutoMigrate(&localmodel.AIModel{}, &localmodel.HermesModelToken{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	modelDAO := dao.NewAIModelDAO(db)
	svc.SetModelLookup(modelDAO)

	// 引用不存在的模型：发布阶段就应被拒
	if _, err := svc.CreatePolicy(ctx, 1, CreateHermesPolicyRequest{
		Name:         "bad model ref",
		DesiredState: localmodel.HermesDesiredStateActive,
		ModelID:      99999,
		Toolsets:     localmodel.HermesToolsets{"clarify", "memory", "no_mcp", "todo"},
		Privacy:      localmodel.HermesPrivacy{},
		LeaseSeconds: 3600,
	}); !errors.Is(err, ErrHermesInvalidInput) {
		t.Fatalf("missing model ref error = %v, want invalid input", err)
	}

	// 引用已禁用的模型：同样应被拒
	disabled := &localmodel.AIModel{
		Name: "disabled-model", DisplayName: "d", Provider: "openai",
		Status: localmodel.AIModelStatusDisabled,
	}
	if err := modelDAO.Create(ctx, disabled); err != nil {
		t.Fatalf("create disabled model: %v", err)
	}
	// Status 有 gorm default:1，零值会被当成未设置而套用默认值，这里显式落盘
	if err := db.Model(&localmodel.AIModel{}).Where("id = ?", disabled.ID).
		Update("status", localmodel.AIModelStatusDisabled).Error; err != nil {
		t.Fatalf("disable model: %v", err)
	}
	if _, err := svc.CreatePolicy(ctx, 1, CreateHermesPolicyRequest{
		Name:         "disabled model ref",
		DesiredState: localmodel.HermesDesiredStateActive,
		ModelID:      disabled.ID,
		Toolsets:     localmodel.HermesToolsets{"clarify", "memory", "no_mcp", "todo"},
		Privacy:      localmodel.HermesPrivacy{},
		LeaseSeconds: 3600,
	}); !errors.Is(err, ErrHermesInvalidInput) {
		t.Fatalf("disabled model ref error = %v, want invalid input", err)
	}

	// 启用的模型可以发布，且 model_name 留空时用模型配置名兜底
	enabled := &localmodel.AIModel{
		Name: "approved-model", DisplayName: "a", Provider: "openai",
		Status: localmodel.AIModelStatusEnabled,
	}
	if err := modelDAO.Create(ctx, enabled); err != nil {
		t.Fatalf("create enabled model: %v", err)
	}
	policy, err := svc.CreatePolicy(ctx, 1, CreateHermesPolicyRequest{
		Name:         "good model ref",
		DesiredState: localmodel.HermesDesiredStateActive,
		ModelID:      enabled.ID,
		Toolsets:     localmodel.HermesToolsets{"clarify", "memory", "no_mcp", "todo"},
		Privacy:      localmodel.HermesPrivacy{},
		LeaseSeconds: 3600,
	})
	if err != nil {
		t.Fatalf("create policy with valid model: %v", err)
	}
	if policy.ModelName != "approved-model" {
		t.Fatalf("model name = %q, want fallback to model config name", policy.ModelName)
	}
}

// 配额守卫：超限时拦下，且判定口径与控制台一致。
func TestQuotaGuardBlocksWhenOverLimit(t *testing.T) {
	svc, store, ctx, _ := setupHermesServiceTest(t, true)
	db := store.DB()
	if err := db.AutoMigrate(&localmodel.AIQuota{}, &localmodel.AIUsageLog{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	_ = svc

	usageDAO := dao.NewAIUsageLogDAO(db)
	quotaDAO := dao.NewAIQuotaDAO(db)
	quotaSvc := NewAIQuotaService(quotaDAO)
	now := time.Date(2026, 7, 29, 12, 0, 0, 0, time.UTC)
	guard := NewHermesQuotaGuard(quotaSvc, usageDAO, func() time.Time { return now })

	// 无配额配置时放行
	if err := guard.Check(ctx, 1, 1); err != nil {
		t.Fatalf("no quota should pass: %v", err)
	}

	limit := 2
	uid := uint64(1)
	if err := quotaSvc.UpsertQuota(ctx, &localmodel.AIQuota{
		Scope: localmodel.AIQuotaScopeUser, ScopeID: &uid,
		DailyRequestLimit: &limit, Enabled: 1, ResetDayOfMonth: 1,
	}); err != nil {
		t.Fatalf("upsert quota: %v", err)
	}

	// 未达上限仍放行
	if err := guard.Check(ctx, 1, 1); err != nil {
		t.Fatalf("under limit should pass: %v", err)
	}

	// 造够用量后必须拦下
	tid := uint64(1)
	for i := 0; i < limit; i++ {
		if err := usageDAO.Create(ctx, &localmodel.AIUsageLog{
			ModelName: "m", UserID: &uid, TenantID: &tid,
			Operation: "hermes_proxy_chat", Status: "success",
			CreatedAt: now.Add(time.Duration(i) * time.Minute),
		}); err != nil {
			t.Fatalf("seed usage: %v", err)
		}
	}
	if err := guard.Check(ctx, 1, 1); !errors.Is(err, ErrHermesQuotaExceeded) {
		t.Fatalf("over limit error = %v, want quota exceeded", err)
	}
}

// 用量统计必须只算 Hermes 代理调用。控制台自身的 AI 调用写在同一张
// ai_usage_logs 表里，不按 operation 过滤就会把两者混算，导致「Hermes 用量」
// 虚高、按设备聚合出现空 device_id 行。
func TestHermesUsageExcludesConsoleCalls(t *testing.T) {
	_, store, ctx, _ := setupHermesServiceTest(t, true)
	db := store.DB()
	if err := db.AutoMigrate(&localmodel.AIUsageLog{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	now := time.Date(2026, 7, 29, 12, 0, 0, 0, time.UTC)
	usageSvc := NewHermesUsageService(dao.NewHermesUsageDAO(db), func() time.Time { return now })

	uid := uint64(1)
	tid := uint64(1)
	seed := func(op, deviceID, status string, tokens int) {
		tk := tokens
		if err := db.Create(&localmodel.AIUsageLog{
			ModelName: "m", UserID: &uid, TenantID: &tid, DeviceID: deviceID,
			Operation: op, Status: status, TotalTokens: &tk,
			CreatedAt: now.Add(-time.Hour),
		}).Error; err != nil {
			t.Fatalf("seed: %v", err)
		}
	}
	// Hermes 代理调用：2 成功 1 失败
	seed(dao.HermesProxyOperation, "dev-a", "success", 100)
	seed(dao.HermesProxyOperation, "dev-a", "success", 150)
	seed(dao.HermesProxyOperation, "dev-b", "error", 0)
	// 控制台自身调用：不该被算进来
	seed("chat", "", "success", 9999)
	seed("image", "", "success", 8888)

	overview, err := usageSvc.Overview(ctx, 7, 10)
	if err != nil {
		t.Fatalf("overview: %v", err)
	}
	if overview.Summary.TotalRequests != 3 {
		t.Fatalf("total requests = %d, want 3 (控制台调用被混算了)", overview.Summary.TotalRequests)
	}
	if overview.Summary.FailedRequests != 1 {
		t.Fatalf("failed = %d, want 1", overview.Summary.FailedRequests)
	}
	if overview.Summary.TotalTokens != 250 {
		t.Fatalf("tokens = %d, want 250 (控制台的 9999/8888 不该计入)", overview.Summary.TotalTokens)
	}
	if overview.Summary.ActiveDevices != 2 {
		t.Fatalf("active devices = %d, want 2", overview.Summary.ActiveDevices)
	}
	// 按设备聚合不该出现空 device_id 行
	for _, d := range overview.Devices {
		if d.DeviceID == "" {
			t.Fatal("按设备聚合出现空 device_id 行，说明控制台调用漏进来了")
		}
	}
	if len(overview.Devices) != 2 {
		t.Fatalf("devices = %d, want 2", len(overview.Devices))
	}

	// 单设备统计
	deviceUsage, err := usageSvc.DeviceUsage(ctx, "dev-a", 7)
	if err != nil {
		t.Fatalf("device usage: %v", err)
	}
	if deviceUsage.Summary.TotalRequests != 2 || deviceUsage.Summary.TotalTokens != 250 {
		t.Fatalf("dev-a summary = %+v, want 2 requests / 250 tokens", deviceUsage.Summary)
	}
	failures, err := usageSvc.DeviceUsage(ctx, "dev-b", 7)
	if err != nil {
		t.Fatalf("dev-b usage: %v", err)
	}
	if len(failures.Failures) != 1 {
		t.Fatalf("dev-b failures = %d, want 1", len(failures.Failures))
	}
}
