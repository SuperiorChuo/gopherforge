package system

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/go-admin-kit/services/shared/pkg/audittrail"
	"github.com/go-admin-kit/services/shared/pkg/exportproof"
	"github.com/go-admin-kit/services/system/internal/edgecert"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

type fakeEdgeCertService struct {
	list            []edgecert.PublicView
	listErr         error
	capabilities    edgecert.Capabilities
	upsertResult    *edgecert.PublicView
	upsertErr       error
	queueResult     *edgecert.Task
	queueErr        error
	tasks           []edgecert.Task
	tasksErr        error
	certificate     string
	domain          string
	certificateErr  error
	privateKey      []byte
	privateKeyErr   error
	deleteErr       error
	queuedID        uint64
	queuedKind      string
	queuedBy        uint64
	certificateCall int
	privateKeyCall  int
	sequence        *[]string
	upsertInput     edgecert.UpsertInput
}

func (f *fakeEdgeCertService) List(context.Context) ([]edgecert.PublicView, error) {
	return f.list, f.listErr
}

func (f *fakeEdgeCertService) UpsertDraft(_ context.Context, input edgecert.UpsertInput) (*edgecert.PublicView, error) {
	f.upsertInput = input
	return f.upsertResult, f.upsertErr
}

func (f *fakeEdgeCertService) QueueTask(_ context.Context, id uint64, kind string, requestedBy uint64) (*edgecert.Task, error) {
	f.queuedID, f.queuedKind, f.queuedBy = id, kind, requestedBy
	return f.queueResult, f.queueErr
}

func (f *fakeEdgeCertService) ListTasks(context.Context, uint64, int) ([]edgecert.Task, error) {
	return f.tasks, f.tasksErr
}

func (f *fakeEdgeCertService) Capabilities() edgecert.Capabilities { return f.capabilities }

func (f *fakeEdgeCertService) ExportCertificate(context.Context, uint64) (string, string, error) {
	f.certificateCall++
	if f.sequence != nil {
		*f.sequence = append(*f.sequence, "certificate")
	}
	return f.certificate, f.domain, f.certificateErr
}

func (f *fakeEdgeCertService) ExportPrivateKey(context.Context, uint64) ([]byte, string, error) {
	f.privateKeyCall++
	if f.sequence != nil {
		*f.sequence = append(*f.sequence, "private-key")
	}
	return f.privateKey, f.domain, f.privateKeyErr
}

func (f *fakeEdgeCertService) Delete(context.Context, uint64) error { return f.deleteErr }

type fakeEdgeCertProofConsumer struct {
	err      error
	token    string
	binding  exportproof.Binding
	calls    int
	sequence *[]string
}

func (f *fakeEdgeCertProofConsumer) Consume(_ context.Context, token string, binding exportproof.Binding) error {
	f.calls++
	f.token, f.binding = token, binding
	if f.sequence != nil {
		*f.sequence = append(*f.sequence, "proof")
	}
	return f.err
}

func TestEdgeCertListUsesNestedSafeDTOAndActiveTask(t *testing.T) {
	now := time.Now().UTC()
	service := &fakeEdgeCertService{list: []edgecert.PublicView{{
		ID: 7, Domain: "admin.example.com", Email: "ops@example.com", Provider: "letsencrypt",
		Status: edgecert.StatusIssued, HasCert: true, CertFingerprintSHA256: "abc",
		DeploymentMode: edgecert.DeploymentModeExternal, DeploymentProvider: "caddy",
		DeploymentStatus: edgecert.DeploymentStatusExternal, AutoRenewEnabled: false,
		ServingStatus:    edgecert.ServingStatusUnreachable,
		ServingErrorCode: "tls_probe_failed", ServingErrorMessage: "dial /run/secrets/private token=leak",
		LastError: "provider token=leak", CreatedAt: now, UpdatedAt: now,
		ActiveTask: &edgecert.Task{
			ID: 11, CertificateID: 7, Kind: edgecert.TaskKindRenew, Status: edgecert.TaskStatusFailed,
			ErrorCode: "task_failed", ErrorMessage: "provider token=leak", CreatedAt: now,
		},
	}}}
	api := newEdgeCertAPI(service, nil, nil)
	recorder := performEdgeCertRequest(t, api.List, http.MethodGet, nil, nil, nil)
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", recorder.Code, recorder.Body.String())
	}
	body := recorder.Body.String()
	for _, wanted := range []string{`"certificate":{"status":"issued","has_certificate":true`, `"active_task":{"id":11`, `"renewal":{"status":"failed"`} {
		if !strings.Contains(body, wanted) {
			t.Fatalf("body missing %q: %s", wanted, body)
		}
	}
	for _, forbidden := range []string{"/run/secrets", "token=leak", "private_key_pem"} {
		if strings.Contains(body, forbidden) {
			t.Fatalf("unsafe value %q leaked: %s", forbidden, body)
		}
	}
}

func TestEdgeCertCapabilitiesFailClosedAndEnableOnlyReadyOperations(t *testing.T) {
	db := openEdgeCertTestDB(t)
	service := &fakeEdgeCertService{capabilities: edgecert.Capabilities{
		EncryptionConfigured: true, AsyncTasks: true, DurableHTTP01: true,
		DeploymentModes:  []string{edgecert.DeploymentModeExternal, edgecert.DeploymentModeTraefikFile},
		AutomaticRenewal: true,
	}}
	proofs := &fakeEdgeCertProofConsumer{}
	api := newEdgeCertAPI(service, proofs, db)
	api.deploymentConfigured = true
	recorder := performEdgeCertRequest(t, api.Capabilities, http.MethodGet, nil, nil, nil)
	if recorder.Code != http.StatusOK || strings.Count(recorder.Body.String(), `"enabled":true`) != 5 {
		t.Fatalf("ready capabilities = %d %s", recorder.Code, recorder.Body.String())
	}

	api = newEdgeCertAPI(service, nil, db)
	api.deploymentConfigured = false
	recorder = performEdgeCertRequest(t, api.Capabilities, http.MethodGet, nil, nil, nil)
	if !strings.Contains(recorder.Body.String(), `"export":{"enabled":false`) ||
		!strings.Contains(recorder.Body.String(), `"deploy":{"enabled":false`) {
		t.Fatalf("capabilities did not fail closed: %s", recorder.Body.String())
	}
}

func TestEdgeCertCreatePreservesManagementConfiguration(t *testing.T) {
	now := time.Now().UTC()
	service := &fakeEdgeCertService{upsertResult: &edgecert.PublicView{
		ID: 7, Domain: "admin.example.com", Email: "ops@example.com",
		DeploymentMode: edgecert.DeploymentModeExternal, DeploymentProvider: "caddy",
		CreatedAt: now, UpdatedAt: now,
	}}
	api := newEdgeCertAPI(service, nil, nil)
	recorder := performEdgeCertRequest(t, api.Create, http.MethodPost, map[string]any{
		"domain": "admin.example.com", "email": "ops@example.com",
		"deployment_mode": "external", "deployment_provider": "caddy",
		"auto_renew_enabled": false,
	}, nil, nil)
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, body=%s", recorder.Code, recorder.Body.String())
	}
	if service.upsertInput.DeploymentMode != edgecert.DeploymentModeExternal || service.upsertInput.DeploymentProvider != "caddy" {
		t.Fatalf("management config dropped: %+v", service.upsertInput)
	}
}

func TestEdgeCertCreateRejectsUnsupportedProviderWithoutInternalError(t *testing.T) {
	service := &fakeEdgeCertService{upsertErr: errors.New("unsupported external deployment provider")}
	api := newEdgeCertAPI(service, nil, nil)
	recorder := performEdgeCertRequest(t, api.Create, http.MethodPost, map[string]any{
		"domain": "admin.example.com", "email": "ops@example.com",
		"deployment_mode": "external", "deployment_provider": "evil",
	}, nil, nil)
	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400; body=%s", recorder.Code, recorder.Body.String())
	}
}

func TestEdgeCertCreateProtectsProviderRecoveryState(t *testing.T) {
	service := &fakeEdgeCertService{upsertErr: fmt.Errorf("hidden provider detail: %w", edgecert.ErrProviderReconcileRequired)}
	api := newEdgeCertAPI(service, nil, nil)
	recorder := performEdgeCertRequest(t, api.Create, http.MethodPost, map[string]any{
		"domain": "admin.example.com", "email": "ops@example.com",
		"deployment_mode": "external", "deployment_provider": "caddy",
	}, nil, nil)
	assertProviderReconcileConflict(t, recorder)
}

func TestEdgeCertQueueReturnsHTTP202WithSuccessEnvelope(t *testing.T) {
	now := time.Now().UTC()
	service := &fakeEdgeCertService{queueResult: &edgecert.Task{
		ID: 9, CertificateID: 7, Kind: edgecert.TaskKindIssue,
		Status: edgecert.TaskStatusQueued, Step: "queued", CreatedAt: now,
	}}
	api := newEdgeCertAPI(service, nil, nil)
	recorder := performEdgeCertRequest(t, api.Issue, http.MethodPost, nil,
		gin.Params{{Key: "id", Value: "7"}}, map[string]any{"user_id": uint(42)})
	if recorder.Code != http.StatusAccepted {
		t.Fatalf("status = %d, body = %s", recorder.Code, recorder.Body.String())
	}
	if !strings.Contains(recorder.Body.String(), `"code":200`) || !strings.Contains(recorder.Body.String(), `"task":{"id":9`) {
		t.Fatalf("unexpected envelope: %s", recorder.Body.String())
	}
	if service.queuedID != 7 || service.queuedKind != edgecert.TaskKindIssue || service.queuedBy != 42 {
		t.Fatalf("queue args = id:%d kind:%q user:%d", service.queuedID, service.queuedKind, service.queuedBy)
	}
}

func TestEdgeCertQueueMarksSequentialDuplicateAsReused(t *testing.T) {
	db := openEdgeCertTestDB(t)
	if err := db.AutoMigrate(&edgecert.Task{}); err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	task := edgecert.Task{
		CertificateID: 7, Kind: edgecert.TaskKindIssue, Status: edgecert.TaskStatusQueued,
		Step: "queued", Environment: edgecert.EnvironmentStaging, RunAfter: now,
		CreatedAt: now, UpdatedAt: now,
	}
	if err := db.Create(&task).Error; err != nil {
		t.Fatal(err)
	}
	service := &fakeEdgeCertService{queueResult: &task}
	api := newEdgeCertAPI(service, nil, db)
	recorder := performEdgeCertRequest(t, api.Issue, http.MethodPost, nil,
		gin.Params{{Key: "id", Value: "7"}}, map[string]any{"user_id": uint(42)})
	if recorder.Code != http.StatusAccepted || !strings.Contains(recorder.Body.String(), `"reused":true`) {
		t.Fatalf("duplicate enqueue = %d %s", recorder.Code, recorder.Body.String())
	}
	if service.queueResult.ID != task.ID {
		t.Fatalf("task ID changed: got %d want %d", service.queueResult.ID, task.ID)
	}
}

func TestEdgeCertQueueRequiresManualProviderReconciliation(t *testing.T) {
	service := &fakeEdgeCertService{queueErr: fmt.Errorf(
		"provider order https://ca.invalid/private-order: %w",
		edgecert.ErrProviderReconcileRequired,
	)}
	api := newEdgeCertAPI(service, nil, nil)
	recorder := performEdgeCertRequest(t, api.Issue, http.MethodPost, nil,
		gin.Params{{Key: "id", Value: "7"}}, map[string]any{"user_id": uint(42)})

	if recorder.Code != http.StatusConflict {
		t.Fatalf("status = %d, want 409; body = %s", recorder.Code, recorder.Body.String())
	}
	body := recorder.Body.String()
	for _, wanted := range []string{"CA 订单状态不确定", "人工核对 CA 订单", "请勿重试创建"} {
		if !strings.Contains(body, wanted) {
			t.Fatalf("response missing %q: %s", wanted, body)
		}
	}
	if strings.Contains(body, "ca.invalid") || strings.Contains(body, "private-order") {
		t.Fatalf("provider detail leaked: %s", body)
	}
}

func TestEdgeCertExportConsumesSessionBoundProofAuditsThenWrites(t *testing.T) {
	sequence := []string{}
	key := []byte("PRIVATE-KEY-MATERIAL")
	service := &fakeEdgeCertService{
		certificate: "PUBLIC-CERT", domain: "admin.example.com", privateKey: key, sequence: &sequence,
	}
	proofs := &fakeEdgeCertProofConsumer{sequence: &sequence}
	api := newEdgeCertAPI(service, proofs, openEdgeCertTestDB(t))
	var auditRecord audittrail.ActionRecord
	api.recordAction = func(_ context.Context, _ *gorm.DB, record audittrail.ActionRecord) error {
		sequence = append(sequence, "audit")
		auditRecord = record
		return nil
	}
	recorder := performEdgeCertRequest(t, api.Export, http.MethodPost, map[string]any{
		"step_up_proof": "proof-1", "confirm_domain": "admin.example.com",
	}, gin.Params{{Key: "id", Value: "7"}}, map[string]any{
		"user_id": uint(42), "session_id": "session-123",
	})
	if recorder.Code != http.StatusOK || recorder.Body.String() != "PRIVATE-KEY-MATERIAL" {
		t.Fatalf("export = %d %q", recorder.Code, recorder.Body.String())
	}
	if !reflect.DeepEqual(sequence, []string{"certificate", "proof", "private-key", "audit"}) {
		t.Fatalf("security sequence = %#v", sequence)
	}
	wantBinding := exportproof.Binding{
		UserID: 42, SessionID: "session-123", ResourceType: exportproof.ResourceTypeEdgeCertificate,
		ResourceID: 7, Audience: exportproof.AudienceEdgeCertificateExport,
	}
	if proofs.token != "proof-1" || proofs.binding != wantBinding {
		t.Fatalf("proof consume = token:%q binding:%+v", proofs.token, proofs.binding)
	}
	if auditRecord.Action != "export" || auditRecord.TargetType != edgeCertificateAuditTarget || auditRecord.TargetID != "7" {
		t.Fatalf("audit = %+v", auditRecord)
	}
	for header, wanted := range map[string]string{
		"Cache-Control": "no-store, private, max-age=0", "Pragma": "no-cache", "Expires": "0",
		"X-Content-Type-Options": "nosniff", "Vary": "Authorization, Cookie",
	} {
		if got := recorder.Header().Get(header); got != wanted {
			t.Fatalf("%s = %q, want %q", header, got, wanted)
		}
	}
	if !strings.Contains(recorder.Header().Get("Content-Disposition"), "admin.example.com.private-key.pem") {
		t.Fatalf("Content-Disposition = %q", recorder.Header().Get("Content-Disposition"))
	}
	for i, value := range key {
		if value != 0 {
			t.Fatalf("private key byte %d was not cleared", i)
		}
	}
}

func TestEdgeCertExportFailsClosedBeforeSecretOutput(t *testing.T) {
	tests := []struct {
		name          string
		body          map[string]any
		proofErr      error
		auditErr      error
		wantStatus    int
		wantProofCall int
		wantKeyCall   int
	}{
		{name: "domain mismatch", body: map[string]any{"step_up_proof": "proof", "confirm_domain": "ADMIN.EXAMPLE.COM"}, wantStatus: http.StatusBadRequest},
		{name: "proof rejected", body: map[string]any{"step_up_proof": "proof", "confirm_domain": "admin.example.com"}, proofErr: exportproof.ErrProofInvalid, wantStatus: http.StatusForbidden, wantProofCall: 1},
		{name: "audit unavailable", body: map[string]any{"step_up_proof": "proof", "confirm_domain": "admin.example.com"}, auditErr: errors.New("audit down"), wantStatus: http.StatusInternalServerError, wantProofCall: 1, wantKeyCall: 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			key := []byte("NEVER-LEAK-THIS-KEY")
			service := &fakeEdgeCertService{certificate: "PUBLIC", domain: "admin.example.com", privateKey: key}
			proofs := &fakeEdgeCertProofConsumer{err: tt.proofErr}
			api := newEdgeCertAPI(service, proofs, openEdgeCertTestDB(t))
			api.recordAction = func(context.Context, *gorm.DB, audittrail.ActionRecord) error { return tt.auditErr }
			recorder := performEdgeCertRequest(t, api.Export, http.MethodPost, tt.body,
				gin.Params{{Key: "id", Value: "7"}}, map[string]any{"user_id": uint(42), "session_id": "session-123"})
			if recorder.Code != tt.wantStatus {
				t.Fatalf("status = %d, body = %s", recorder.Code, recorder.Body.String())
			}
			if proofs.calls != tt.wantProofCall || service.privateKeyCall != tt.wantKeyCall {
				t.Fatalf("calls = proof:%d key:%d", proofs.calls, service.privateKeyCall)
			}
			if strings.Contains(recorder.Body.String(), "NEVER-LEAK-THIS-KEY") {
				t.Fatalf("private key leaked in error: %s", recorder.Body.String())
			}
			if tt.wantKeyCall == 1 {
				for i, value := range key {
					if value != 0 {
						t.Fatalf("private key byte %d was not cleared on failure", i)
					}
				}
			}
		})
	}
}

func TestEdgeCertCertificateAndLegacyDownloadNeverExposePrivateKey(t *testing.T) {
	service := &fakeEdgeCertService{
		certificate: "-----BEGIN CERTIFICATE-----\nPUBLIC\n-----END CERTIFICATE-----\n",
		domain:      "admin.example.com", privateKey: []byte("PRIVATE"),
	}
	api := newEdgeCertAPI(service, nil, nil)
	certificate := performEdgeCertRequest(t, api.Certificate, http.MethodGet, nil,
		gin.Params{{Key: "id", Value: "7"}}, nil)
	if certificate.Code != http.StatusOK || strings.Contains(certificate.Body.String(), "PRIVATE") || service.privateKeyCall != 0 {
		t.Fatalf("certificate response = %d %q, key calls = %d", certificate.Code, certificate.Body.String(), service.privateKeyCall)
	}
	legacy := performEdgeCertRequest(t, api.Download, http.MethodGet, nil,
		gin.Params{{Key: "id", Value: "7"}}, nil)
	if legacy.Code != http.StatusGone || service.privateKeyCall != 0 || strings.Contains(legacy.Body.String(), "PRIVATE") {
		t.Fatalf("legacy response = %d %q, key calls = %d", legacy.Code, legacy.Body.String(), service.privateKeyCall)
	}
}

func TestEdgeCertGetTaskIsBoundToCertificateAndSanitizesFailure(t *testing.T) {
	db := openEdgeCertTestDB(t)
	if err := db.AutoMigrate(&edgecert.Task{}); err != nil {
		t.Fatal(err)
	}
	task := edgecert.Task{
		CertificateID: 7, Kind: edgecert.TaskKindProbe, Status: edgecert.TaskStatusFailed,
		Step: "failed", Environment: edgecert.EnvironmentProduction, ErrorCode: "task_failed",
		ErrorMessage: "dial /run/secrets/key token=leak", RunAfter: time.Now(), CreatedAt: time.Now(), UpdatedAt: time.Now(),
	}
	if err := db.Create(&task).Error; err != nil {
		t.Fatal(err)
	}
	api := newEdgeCertAPI(&fakeEdgeCertService{}, nil, db)
	wrongCertificate := performEdgeCertRequest(t, api.GetTask, http.MethodGet, nil, gin.Params{
		{Key: "id", Value: "8"}, {Key: "taskId", Value: strconvUint(task.ID)},
	}, nil)
	if wrongCertificate.Code != http.StatusNotFound {
		t.Fatalf("cross-certificate task lookup = %d %s", wrongCertificate.Code, wrongCertificate.Body.String())
	}
	found := performEdgeCertRequest(t, api.GetTask, http.MethodGet, nil, gin.Params{
		{Key: "id", Value: "7"}, {Key: "taskId", Value: strconvUint(task.ID)},
	}, nil)
	if found.Code != http.StatusOK || strings.Contains(found.Body.String(), "/run/secrets") || strings.Contains(found.Body.String(), "token=leak") {
		t.Fatalf("task response = %d %s", found.Code, found.Body.String())
	}
}

func TestEdgeCertTaskViewPreservesSafeProviderUncertainty(t *testing.T) {
	view := toEdgeCertTaskView(edgecert.Task{
		ID: 9, CertificateID: 7, Kind: edgecert.TaskKindIssue,
		Status: edgecert.TaskStatusFailed, ErrorCode: "provider_state_uncertain",
		ErrorMessage: "provider URL and raw response must not be returned",
	})
	if view.ErrorCode != "provider_state_uncertain" {
		t.Fatalf("error code = %q", view.ErrorCode)
	}
	if !strings.Contains(view.ErrorHint, "核对") || strings.Contains(view.ErrorMessage, "provider URL") {
		t.Fatalf("unsafe or unhelpful task failure view: %#v", view)
	}
}

func TestEdgeCertACMEChallengeReadsDurableUnexpiredToken(t *testing.T) {
	db := openEdgeCertTestDB(t)
	if err := db.AutoMigrate(&edgecert.Challenge{}); err != nil {
		t.Fatal(err)
	}
	challenge := edgecert.Challenge{
		Token: "token-1", CertificateID: 7, KeyAuthorization: "token-1.signature",
		ExpiresAt: time.Now().Add(time.Minute), CreatedAt: time.Now(),
	}
	if err := db.Create(&challenge).Error; err != nil {
		t.Fatal(err)
	}
	api := newEdgeCertAPI(&fakeEdgeCertService{}, nil, db)
	found := performEdgeCertRequest(t, api.ACMEChallenge, http.MethodGet, nil,
		gin.Params{{Key: "token", Value: "token-1"}}, nil)
	if found.Code != http.StatusOK || found.Body.String() != "token-1.signature" {
		t.Fatalf("challenge = %d %q", found.Code, found.Body.String())
	}
	missing := performEdgeCertRequest(t, api.ACMEChallenge, http.MethodGet, nil,
		gin.Params{{Key: "token", Value: "missing"}}, nil)
	if missing.Code != http.StatusNotFound {
		t.Fatalf("missing challenge = %d %q", missing.Code, missing.Body.String())
	}
}

func TestEdgeCertDeleteMapsProtectedStatesToConflict(t *testing.T) {
	for _, err := range []error{edgecert.ErrCertificateBusy, edgecert.ErrCertificateDeployed} {
		api := newEdgeCertAPI(&fakeEdgeCertService{deleteErr: err}, nil, nil)
		recorder := performEdgeCertRequest(t, api.Delete, http.MethodDelete, nil,
			gin.Params{{Key: "id", Value: "7"}}, nil)
		if recorder.Code != http.StatusConflict {
			t.Fatalf("error %v mapped to %d: %s", err, recorder.Code, recorder.Body.String())
		}
	}
	api := newEdgeCertAPI(&fakeEdgeCertService{deleteErr: fmt.Errorf("hidden provider detail: %w", edgecert.ErrProviderReconcileRequired)}, nil, nil)
	recorder := performEdgeCertRequest(t, api.Delete, http.MethodDelete, nil,
		gin.Params{{Key: "id", Value: "7"}}, nil)
	assertProviderReconcileConflict(t, recorder)
}

func assertProviderReconcileConflict(t *testing.T, recorder *httptest.ResponseRecorder) {
	t.Helper()
	if recorder.Code != http.StatusConflict {
		t.Fatalf("status = %d, want 409; body = %s", recorder.Code, recorder.Body.String())
	}
	body := recorder.Body.String()
	for _, wanted := range []string{"CA 订单状态不确定", "人工核对 CA 订单", "请勿重试创建"} {
		if !strings.Contains(body, wanted) {
			t.Fatalf("response missing %q: %s", wanted, body)
		}
	}
	if strings.Contains(body, "hidden provider detail") {
		t.Fatalf("provider detail leaked: %s", body)
	}
}

func performEdgeCertRequest(
	t *testing.T,
	handler gin.HandlerFunc,
	method string,
	body any,
	params gin.Params,
	values map[string]any,
) *httptest.ResponseRecorder {
	t.Helper()
	gin.SetMode(gin.TestMode)
	payload := []byte(nil)
	if body != nil {
		var err error
		payload, err = json.Marshal(body)
		if err != nil {
			t.Fatal(err)
		}
	}
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Params = params
	c.Request = httptest.NewRequest(method, "/api/v1/edge-certs", bytes.NewReader(payload))
	c.Request.Header.Set("Content-Type", "application/json")
	for key, value := range values {
		c.Set(key, value)
	}
	handler(c)
	return recorder
}

func openEdgeCertTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	dsn := "file:" + strings.NewReplacer("/", "_", " ", "_").Replace(t.Name()) + "?mode=memory&cache=shared"
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	return db
}

func strconvUint(value uint64) string {
	return strconv.FormatUint(value, 10)
}
