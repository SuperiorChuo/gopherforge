package system

import (
	"context"
	"errors"
	"mime"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/go-admin-kit/services/shared/pkg/audittrail"
	"github.com/go-admin-kit/services/shared/pkg/database"
	"github.com/go-admin-kit/services/shared/pkg/exportproof"
	"github.com/go-admin-kit/services/shared/pkg/response"
	"github.com/go-admin-kit/services/system/internal/edgecert"
	"gorm.io/gorm"
)

const (
	edgeCertificateAuditTarget = "edge_tls_certificate"
	edgeCertificateAuditTenant = uint(1)
	edgeCertificateJSONLimit   = int64(16 << 10)
)

type edgeCertService interface {
	List(context.Context) ([]edgecert.PublicView, error)
	UpsertDraft(context.Context, edgecert.UpsertInput) (*edgecert.PublicView, error)
	QueueTask(context.Context, uint64, string, uint64) (*edgecert.Task, error)
	ListTasks(context.Context, uint64, int) ([]edgecert.Task, error)
	Capabilities() edgecert.Capabilities
	ExportCertificate(context.Context, uint64) (string, string, error)
	ExportPrivateKey(context.Context, uint64) ([]byte, string, error)
	Delete(context.Context, uint64) error
}

type edgeCertProofConsumer interface {
	Consume(context.Context, string, exportproof.Binding) error
}

type edgeCertActionRecorder func(context.Context, *gorm.DB, audittrail.ActionRecord) error

// EdgeCertAPI exposes the certificate lifecycle without ever serializing secret
// material into the normal JSON response envelope.
type EdgeCertAPI struct {
	svc                  edgeCertService
	proofs               edgeCertProofConsumer
	db                   *gorm.DB
	recordAction         edgeCertActionRecorder
	deploymentConfigured bool
}

var configuredEdgeCertAPI struct {
	sync.RWMutex
	service *edgecert.Service
	proofs  *exportproof.Store
}

// ConfigureEdgeCertAPI installs the keyring/deployer-aware runtime assembled by
// main before routes are registered. A missing proof store or keyring is kept as
// a fail-closed runtime state and is reflected by Capabilities.
func ConfigureEdgeCertAPI(service *edgecert.Service, proofs *exportproof.Store) {
	configuredEdgeCertAPI.Lock()
	configuredEdgeCertAPI.service = service
	configuredEdgeCertAPI.proofs = proofs
	configuredEdgeCertAPI.Unlock()
}

func NewEdgeCertAPI() *EdgeCertAPI {
	return NewEdgeCertAPIWithDB(database.DB)
}

// NewEdgeCertAPIWithDB keeps dependency-injected route composition usable in
// tests and embedded deployments while still preferring the fully configured
// runtime installed by ConfigureEdgeCertAPI.
func NewEdgeCertAPIWithDB(db *gorm.DB) *EdgeCertAPI {
	configuredEdgeCertAPI.RLock()
	service := configuredEdgeCertAPI.service
	proofs := configuredEdgeCertAPI.proofs
	configuredEdgeCertAPI.RUnlock()

	if service == nil {
		if db == nil {
			db = database.DB
		}
		service = edgecert.NewService(db, nil, nil, nil)
	}
	return newEdgeCertAPI(service, proofs, service.DB)
}

func newEdgeCertAPI(service edgeCertService, proofs edgeCertProofConsumer, db *gorm.DB) *EdgeCertAPI {
	api := &EdgeCertAPI{
		svc: service, proofs: proofs, db: db, recordAction: audittrail.RecordAction,
	}
	if concrete, ok := service.(*edgecert.Service); ok {
		api.deploymentConfigured = concrete.Deployer != nil
	}
	return api
}

func (a *EdgeCertAPI) List(c *gin.Context) {
	if a == nil || a.svc == nil {
		serviceUnavailable(c, "证书服务尚未就绪")
		return
	}
	list, err := a.svc.List(c.Request.Context())
	if err != nil {
		response.InternalServerError(c, "edge certificate list failed")
		return
	}
	out := make([]edgeCertView, 0, len(list))
	for i := range list {
		out = append(out, toEdgeCertView(list[i]))
	}
	response.Success(c, out)
}

func (a *EdgeCertAPI) Capabilities(c *gin.Context) {
	capabilities := edgeCertCapabilitiesView{}
	if a == nil || a.svc == nil {
		capabilities.disableAll("证书服务尚未就绪")
		response.Success(c, capabilities)
		return
	}
	runtime := a.svc.Capabilities()
	issueReady := runtime.EncryptionConfigured && runtime.AsyncTasks && runtime.DurableHTTP01 && a.db != nil
	capabilities.Issue = capability(issueReady, "签发所需的加密密钥、任务队列或持久化挑战未就绪")
	capabilities.Renew = capability(issueReady && runtime.AutomaticRenewal, "自动续期能力未就绪")
	capabilities.Deploy = capability(issueReady && a.deploymentConfigured && contains(runtime.DeploymentModes, edgecert.DeploymentModeTraefikFile), "Traefik 文件部署未配置")
	capabilities.Probe = capability(runtime.AsyncTasks && a.db != nil, "线上探测任务未就绪")
	capabilities.Export = capability(runtime.EncryptionConfigured && a.proofs != nil && a.db != nil && a.recordAction != nil, "私钥导出认证或审计依赖未就绪")
	response.Success(c, capabilities)
}

func (a *EdgeCertAPI) Create(c *gin.Context) {
	if a == nil || a.svc == nil {
		serviceUnavailable(c, "证书服务尚未就绪")
		return
	}
	var req edgecert.UpsertInput
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, edgeCertificateJSONLimit)
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "请求内容无效")
		return
	}
	value, err := a.svc.UpsertDraft(c.Request.Context(), req)
	if err != nil {
		writeMutationError(c, err, "证书配置无效")
		return
	}
	if value == nil {
		response.InternalServerError(c, "edge certificate create returned nil")
		return
	}
	response.Success(c, toEdgeCertView(*value))
}

func (a *EdgeCertAPI) Issue(c *gin.Context)  { a.queueTask(c, edgecert.TaskKindIssue) }
func (a *EdgeCertAPI) Renew(c *gin.Context)  { a.queueTask(c, edgecert.TaskKindRenew) }
func (a *EdgeCertAPI) Deploy(c *gin.Context) { a.queueTask(c, edgecert.TaskKindDeploy) }
func (a *EdgeCertAPI) Probe(c *gin.Context)  { a.queueTask(c, edgecert.TaskKindProbe) }

func (a *EdgeCertAPI) queueTask(c *gin.Context, kind string) {
	if a == nil || a.svc == nil {
		serviceUnavailable(c, "证书服务尚未就绪")
		return
	}
	id, ok := edgeCertID(c, "id")
	if !ok {
		return
	}
	userID, ok := contextUserID(c)
	if !ok {
		response.Unauthorized(c, "登录上下文无效")
		return
	}
	existingTaskID, err := a.activeTaskID(c.Request.Context(), id)
	if err != nil {
		response.InternalServerError(c, "edge certificate active task lookup failed")
		return
	}
	task, err := a.svc.QueueTask(c.Request.Context(), id, kind, userID)
	if err != nil {
		writeQueueError(c, err)
		return
	}
	if task == nil {
		response.InternalServerError(c, "edge certificate enqueue returned nil")
		return
	}
	c.JSON(http.StatusAccepted, response.Response{
		Code: http.StatusOK, Message: "accepted",
		Data: edgeCertTaskAccepted{Task: toEdgeCertTaskView(*task), Reused: existingTaskID != 0 && existingTaskID == task.ID},
	})
}

func (a *EdgeCertAPI) activeTaskID(ctx context.Context, certificateID uint64) (uint64, error) {
	if a == nil || a.db == nil {
		return 0, nil
	}
	var task edgecert.Task
	err := a.db.WithContext(ctx).
		Select("id").
		Where("certificate_id = ? AND status IN ?", certificateID, []string{edgecert.TaskStatusQueued, edgecert.TaskStatusRunning}).
		Order("id DESC").First(&task).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return 0, nil
	}
	return task.ID, err
}

func (a *EdgeCertAPI) ListTasks(c *gin.Context) {
	if a == nil || a.svc == nil {
		serviceUnavailable(c, "证书服务尚未就绪")
		return
	}
	id, ok := edgeCertID(c, "id")
	if !ok {
		return
	}
	limit := 50
	if raw := strings.TrimSpace(c.Query("limit")); raw != "" {
		parsed, err := strconv.Atoi(raw)
		if err != nil || parsed <= 0 || parsed > 100 {
			response.BadRequest(c, "limit 必须在 1 到 100 之间")
			return
		}
		limit = parsed
	}
	tasks, err := a.svc.ListTasks(c.Request.Context(), id, limit)
	if err != nil {
		response.InternalServerError(c, "edge certificate task list failed")
		return
	}
	list := make([]edgeCertTaskView, 0, len(tasks))
	for i := range tasks {
		list = append(list, toEdgeCertTaskView(tasks[i]))
	}
	response.Success(c, gin.H{"list": list})
}

func (a *EdgeCertAPI) GetTask(c *gin.Context) {
	certificateID, ok := edgeCertID(c, "id")
	if !ok {
		return
	}
	taskID, ok := edgeCertID(c, "taskId")
	if !ok {
		return
	}
	if a == nil || a.db == nil {
		serviceUnavailable(c, "任务存储尚未就绪")
		return
	}
	var task edgecert.Task
	err := a.db.WithContext(c.Request.Context()).Where("id = ? AND certificate_id = ?", taskID, certificateID).First(&task).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		response.NotFound(c, "任务不存在")
		return
	}
	if err != nil {
		response.InternalServerError(c, "edge certificate task lookup failed")
		return
	}
	response.Success(c, toEdgeCertTaskView(task))
}

func (a *EdgeCertAPI) Delete(c *gin.Context) {
	if a == nil || a.svc == nil {
		serviceUnavailable(c, "证书服务尚未就绪")
		return
	}
	id, ok := edgeCertID(c, "id")
	if !ok {
		return
	}
	if err := a.svc.Delete(c.Request.Context(), id); err != nil {
		switch {
		case errors.Is(err, edgecert.ErrCertificateBusy), errors.Is(err, edgecert.ErrCertificateDeployed):
			response.Error(c, http.StatusConflict, "证书正在使用中，无法删除")
		case errors.Is(err, edgecert.ErrProviderReconcileRequired):
			providerReconcileRequired(c)
		case errors.Is(err, gorm.ErrRecordNotFound):
			response.NotFound(c, "证书不存在")
		default:
			response.InternalServerError(c, "edge certificate delete failed")
		}
		return
	}
	response.SuccessWithMessage(c, "deleted", nil)
}

// Certificate downloads public certificate material only. Private key bytes
// are deliberately unavailable from this endpoint and from JSON responses.
func (a *EdgeCertAPI) Certificate(c *gin.Context) {
	if a == nil || a.svc == nil {
		serviceUnavailable(c, "证书服务尚未就绪")
		return
	}
	id, ok := edgeCertID(c, "id")
	if !ok {
		return
	}
	fullchain, domain, err := a.svc.ExportCertificate(c.Request.Context(), id)
	if err != nil {
		writeCertificateReadError(c, err)
		return
	}
	setSensitiveDownloadHeaders(c, domain+".fullchain.pem")
	c.Data(http.StatusOK, "application/x-pem-file", []byte(fullchain))
}

type edgeCertExportRequest struct {
	StepUpProof   string `json:"step_up_proof"`
	Proof         string `json:"proof"`
	ConfirmDomain string `json:"confirm_domain"`
}

// Export consumes a session-bound, single-use proof before decrypting. The
// synchronous audit write must succeed before a byte of the private key leaves
// the process.
func (a *EdgeCertAPI) Export(c *gin.Context) {
	setNoStoreHeaders(c)
	if a == nil || a.svc == nil || a.proofs == nil || a.db == nil || a.recordAction == nil {
		serviceUnavailable(c, "私钥导出能力尚未就绪")
		return
	}
	id, ok := edgeCertID(c, "id")
	if !ok {
		return
	}
	var req edgeCertExportRequest
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, edgeCertificateJSONLimit)
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "请求内容无效")
		return
	}
	proof, ok := exportProof(req)
	if !ok {
		response.BadRequest(c, "step-up proof 无效")
		return
	}
	_, domain, err := a.svc.ExportCertificate(c.Request.Context(), id)
	if err != nil {
		writeCertificateReadError(c, err)
		return
	}
	if req.ConfirmDomain != domain {
		response.BadRequest(c, "确认域名不匹配")
		return
	}
	userID, ok := contextUserID(c)
	if !ok {
		response.Unauthorized(c, "登录上下文无效")
		return
	}
	sessionID, ok := contextSessionID(c)
	if !ok {
		response.Unauthorized(c, "登录会话无效")
		return
	}
	binding := exportproof.Binding{
		UserID: userID, SessionID: sessionID,
		ResourceType: exportproof.ResourceTypeEdgeCertificate,
		ResourceID:   id, Audience: exportproof.AudienceEdgeCertificateExport,
	}
	if err := a.proofs.Consume(c.Request.Context(), proof, binding); err != nil {
		if errors.Is(err, exportproof.ErrStoreUnavailable) {
			serviceUnavailable(c, "step-up proof 服务暂不可用")
		} else {
			response.Forbidden(c, "step-up proof 无效或已过期")
		}
		return
	}
	privateKey, keyDomain, err := a.svc.ExportPrivateKey(c.Request.Context(), id)
	if err != nil {
		writePrivateKeyReadError(c, err)
		return
	}
	defer clear(privateKey)
	if keyDomain != domain {
		response.InternalServerError(c, "edge certificate export domain invariant failed")
		return
	}
	if err := a.recordAction(c.Request.Context(), a.db, audittrail.ActionRecord{
		TenantID: edgeCertificateAuditTenant,
		Action:   "export", TargetType: edgeCertificateAuditTarget, TargetID: strconv.FormatUint(id, 10),
		Summary:  "导出边缘证书私钥",
		Metadata: map[string]any{"domain": domain, "format": "pem"},
	}); err != nil {
		response.InternalServerError(c, "edge certificate export audit failed")
		return
	}
	setSensitiveDownloadHeaders(c, domain+".private-key.pem")
	c.Data(http.StatusOK, "application/x-pem-file", privateKey)
}

// Download is an intentionally dead V1 route. It must never call the service
// because the legacy JSON shape included private_key_pem.
func (a *EdgeCertAPI) Download(c *gin.Context) {
	setNoStoreHeaders(c)
	response.Error(c, http.StatusGone, "旧下载接口已停用，请分别下载证书或完成二次认证后导出私钥")
}

// ACMEChallenge is public by protocol, but reads only an unexpired token from
// the durable database shared by every system replica.
func (a *EdgeCertAPI) ACMEChallenge(c *gin.Context) {
	token := c.Param("token")
	c.Header("Cache-Control", "no-store")
	c.Header("X-Content-Type-Options", "nosniff")
	if a == nil || a.db == nil {
		c.String(http.StatusServiceUnavailable, "temporarily unavailable")
		return
	}
	keyAuthorization, found, err := edgecert.LookupChallenge(c.Request.Context(), a.db, token)
	if err != nil {
		c.String(http.StatusServiceUnavailable, "temporarily unavailable")
		return
	}
	if !found {
		c.String(http.StatusNotFound, "not found")
		return
	}
	c.Data(http.StatusOK, "text/plain; charset=utf-8", []byte(keyAuthorization))
}

// ACMEChallenge preserves the old package-level entry point for callers that
// have not yet switched to the configured API instance.
func ACMEChallenge(c *gin.Context) { NewEdgeCertAPI().ACMEChallenge(c) }

type edgeCertCapabilityView struct {
	Enabled bool   `json:"enabled"`
	Reason  string `json:"reason,omitempty"`
}

type edgeCertCapabilitiesView struct {
	Issue  edgeCertCapabilityView `json:"issue"`
	Renew  edgeCertCapabilityView `json:"renew"`
	Deploy edgeCertCapabilityView `json:"deploy"`
	Probe  edgeCertCapabilityView `json:"probe"`
	Export edgeCertCapabilityView `json:"export"`
}

func (v *edgeCertCapabilitiesView) disableAll(reason string) {
	value := capability(false, reason)
	v.Issue, v.Renew, v.Deploy, v.Probe, v.Export = value, value, value, value, value
}

func capability(enabled bool, reason string) edgeCertCapabilityView {
	if enabled {
		return edgeCertCapabilityView{Enabled: true}
	}
	return edgeCertCapabilityView{Reason: reason}
}

type edgeCertView struct {
	ID          uint64                  `json:"id"`
	Domain      string                  `json:"domain"`
	Email       string                  `json:"email"`
	Provider    string                  `json:"provider"`
	IsStaging   bool                    `json:"is_staging"`
	Certificate edgeCertCertificateView `json:"certificate"`
	Issuance    edgeCertIssuanceView    `json:"issuance"`
	Deployment  edgeCertDeploymentView  `json:"deployment"`
	Renewal     edgeCertRenewalView     `json:"renewal"`
	Serving     edgeCertServingView     `json:"serving"`
	ActiveTask  *edgeCertTaskView       `json:"active_task"`
	CreatedAt   time.Time               `json:"created_at"`
	UpdatedAt   time.Time               `json:"updated_at"`
}

type edgeCertCertificateView struct {
	Status            string     `json:"status"`
	HasCertificate    bool       `json:"has_certificate"`
	NotBefore         *time.Time `json:"not_before,omitempty"`
	NotAfter          *time.Time `json:"not_after,omitempty"`
	FingerprintSHA256 string     `json:"fingerprint_sha256,omitempty"`
}

type edgeCertIssuanceView struct {
	Status    string `json:"status"`
	LastError string `json:"last_error,omitempty"`
}

type edgeCertDeploymentView struct {
	Mode                      string     `json:"mode"`
	Provider                  string     `json:"provider"`
	Status                    string     `json:"status"`
	DeployedFingerprintSHA256 string     `json:"deployed_fingerprint_sha256,omitempty"`
	DeployedAt                *time.Time `json:"deployed_at,omitempty"`
	LastError                 string     `json:"last_error,omitempty"`
}

type edgeCertRenewalView struct {
	Status        string     `json:"status"`
	AutoEnabled   bool       `json:"auto_enabled"`
	RenewAt       *time.Time `json:"renew_at,omitempty"`
	LastRenewalAt *time.Time `json:"last_renewal_at,omitempty"`
	LastError     string     `json:"last_error,omitempty"`
}

type edgeCertServingView struct {
	Status                  string     `json:"status"`
	ManagedCertificateInUse bool       `json:"managed_certificate_in_use"`
	FingerprintSHA256       string     `json:"fingerprint_sha256,omitempty"`
	NotAfter                *time.Time `json:"not_after,omitempty"`
	Issuer                  string     `json:"issuer,omitempty"`
	CheckedAt               *time.Time `json:"checked_at,omitempty"`
	ErrorCode               string     `json:"error_code,omitempty"`
	ErrorMessage            string     `json:"error_message,omitempty"`
}

type edgeCertTaskView struct {
	ID            uint64     `json:"id"`
	CertificateID uint64     `json:"certificate_id"`
	Kind          string     `json:"kind"`
	Status        string     `json:"status"`
	Step          string     `json:"step,omitempty"`
	Environment   string     `json:"environment,omitempty"`
	AttemptCount  int        `json:"attempt_count,omitempty"`
	ErrorCode     string     `json:"error_code,omitempty"`
	ErrorMessage  string     `json:"error_message,omitempty"`
	ErrorHint     string     `json:"error_hint,omitempty"`
	CreatedAt     time.Time  `json:"created_at"`
	UpdatedAt     time.Time  `json:"updated_at,omitempty"`
	StartedAt     *time.Time `json:"started_at,omitempty"`
	FinishedAt    *time.Time `json:"finished_at,omitempty"`
}

type edgeCertTaskAccepted struct {
	Task   edgeCertTaskView `json:"task"`
	Reused bool             `json:"reused,omitempty"`
}

func toEdgeCertView(row edgecert.PublicView) edgeCertView {
	active := publicTaskPointer(row.ActiveTask)
	certificateStatus := firstNonEmpty(row.Certificate.Status, row.Status)
	certificateHasCert := row.Certificate.HasCertificate || row.HasCert
	certificateFingerprint := firstNonEmpty(row.Certificate.FingerprintSHA256, row.CertFingerprintSHA256)
	certificateNotBefore := firstTime(row.Certificate.NotBefore, row.NotBefore)
	certificateNotAfter := firstTime(row.Certificate.NotAfter, row.NotAfter)
	issuanceStatus := firstNonEmpty(row.Issuance.Status, row.Status)
	deploymentMode := firstNonEmpty(row.Deployment.Mode, row.DeploymentMode)
	deploymentProvider := firstNonEmpty(row.Deployment.Provider, row.DeploymentProvider)
	deploymentStatus := firstNonEmpty(row.Deployment.Status, row.DeploymentStatus)
	deployedFingerprint := firstNonEmpty(row.Deployment.DeployedFingerprintSHA256, row.DeployedFingerprintSHA256)
	deployedAt := firstTime(row.Deployment.DeployedAt, row.DeployedAt)
	autoRenew := row.Renewal.AutoEnabled || row.AutoRenewEnabled
	renewAt := firstTime(row.Renewal.RenewAt, row.RenewAt)
	lastRenewalAt := firstTime(row.Renewal.LastRenewalAt, row.LastRenewalAt)
	servingStatus := firstNonEmpty(row.Serving.Status, row.ServingStatus)
	servingFingerprint := firstNonEmpty(row.Serving.FingerprintSHA256, row.ServingFingerprintSHA256)
	servingNotAfter := firstTime(row.Serving.NotAfter, row.ServingNotAfter)
	servingIssuer := firstNonEmpty(row.Serving.Issuer, row.ServingIssuer)
	servingCheckedAt := firstTime(row.Serving.CheckedAt, row.ServingCheckedAt)
	servingErrorCode := firstNonEmpty(row.Serving.ErrorCode, row.ServingErrorCode)
	servingErrorPresent := row.Serving.ErrorMessage != "" || row.ServingErrorMessage != ""
	renewalStatus := firstNonEmpty(row.Renewal.Status, "idle")
	if active != nil {
		switch active.Kind {
		case edgecert.TaskKindIssue:
			issuanceStatus = active.Status
		case edgecert.TaskKindRenew:
			renewalStatus = active.Status
		case edgecert.TaskKindDeploy:
			deploymentStatus = active.Status
		}
	}
	lastError := safeLifecycleError(row.LastError != "" || row.Issuance.LastError != "")
	deploymentError := ""
	if row.DeploymentStatus == edgecert.DeploymentStatusFailed {
		deploymentError = lastError
	}
	return edgeCertView{
		ID: row.ID, Domain: row.Domain, Email: row.Email, Provider: firstNonEmpty(row.Provider, row.Issuance.Provider), IsStaging: row.IsStaging,
		Certificate: edgeCertCertificateView{
			Status: certificateStatus, HasCertificate: certificateHasCert,
			NotBefore: certificateNotBefore, NotAfter: certificateNotAfter, FingerprintSHA256: certificateFingerprint,
		},
		Issuance: edgeCertIssuanceView{Status: issuanceStatus, LastError: lastError},
		Deployment: edgeCertDeploymentView{
			Mode: deploymentMode, Provider: deploymentProvider, Status: deploymentStatus,
			DeployedFingerprintSHA256: deployedFingerprint, DeployedAt: deployedAt,
			LastError: deploymentError,
		},
		Renewal: edgeCertRenewalView{
			Status: renewalStatus, AutoEnabled: autoRenew,
			RenewAt: renewAt, LastRenewalAt: lastRenewalAt,
			LastError: safeLifecycleError(row.Renewal.LastError != ""),
		},
		Serving: edgeCertServingView{
			Status: servingStatus, ManagedCertificateInUse: row.Serving.ManagedCertificateInUse,
			FingerprintSHA256: servingFingerprint,
			NotAfter:          servingNotAfter, Issuer: servingIssuer, CheckedAt: servingCheckedAt,
			ErrorCode:    safeErrorCode(servingErrorCode),
			ErrorMessage: safeServingError(servingStatus, servingErrorCode, servingErrorPresent),
		},
		ActiveTask: active, CreatedAt: row.CreatedAt, UpdatedAt: row.UpdatedAt,
	}
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}

func firstTime(values ...*time.Time) *time.Time {
	for _, value := range values {
		if value != nil {
			return value
		}
	}
	return nil
}

func publicTaskPointer(task *edgecert.Task) *edgeCertTaskView {
	if task == nil {
		return nil
	}
	value := toEdgeCertTaskView(*task)
	return &value
}

func toEdgeCertTaskView(task edgecert.Task) edgeCertTaskView {
	code := safeErrorCode(task.ErrorCode)
	message, hint := safeTaskFailure(code, task.ErrorMessage != "" || task.Status == edgecert.TaskStatusFailed)
	return edgeCertTaskView{
		ID: task.ID, CertificateID: task.CertificateID, Kind: task.Kind, Status: task.Status,
		Step: task.Step, Environment: task.Environment, AttemptCount: task.AttemptCount,
		ErrorCode: code, ErrorMessage: message, ErrorHint: hint,
		CreatedAt: task.CreatedAt, UpdatedAt: task.UpdatedAt, StartedAt: task.StartedAt, FinishedAt: task.FinishedAt,
	}
}

func safeTaskFailure(code string, failed bool) (string, string) {
	if !failed {
		return "", ""
	}
	switch code {
	case "encryption_key_unavailable":
		return "证书加密密钥不可用", "请配置证书加密密钥并重启 system worker"
	case "lease_lost":
		return "任务租约已转移", "任务可由其他 worker 安全接管"
	case "task_timeout":
		return "任务执行超时", "请检查 DNS 与 ACME 连通性后重试"
	case "provider_state_uncertain":
		return "证书机构订单状态无法确认", "请先在证书机构核对该任务的订单，确认后再发起新签发"
	case "tls_probe_failed":
		return "TLS 线上探测失败", "请检查域名解析和网关 TLS 配置"
	default:
		return "任务执行失败", "请使用任务编号查看 system 服务日志后重试"
	}
}

func safeLifecycleError(present bool) string {
	if !present {
		return ""
	}
	return "证书任务失败，请查看任务记录"
}

func safeServingError(status, code string, present bool) string {
	if !present && status != edgecert.ServingStatusMismatch && status != edgecert.ServingStatusUnreachable && status != edgecert.ServingStatusInvalid {
		return ""
	}
	switch status {
	case edgecert.ServingStatusMismatch:
		return "在线证书与当前证书指纹不一致"
	case edgecert.ServingStatusUnreachable:
		return "无法连接域名的 TLS 服务"
	case edgecert.ServingStatusInvalid:
		return "在线 TLS 证书无效"
	default:
		if code == "tls_probe_failed" {
			return "TLS 线上探测失败"
		}
		return "TLS 线上状态检查失败"
	}
}

func safeErrorCode(code string) string {
	code = strings.TrimSpace(code)
	switch code {
	case "":
		return ""
	case "encryption_key_unavailable", "lease_lost", "task_timeout", "provider_state_uncertain", "tls_probe_failed", "fingerprint_mismatch", "task_failed":
		return code
	default:
		return "task_failed"
	}
}

func edgeCertID(c *gin.Context, name string) (uint64, bool) {
	id, err := strconv.ParseUint(c.Param(name), 10, 64)
	if err != nil || id == 0 {
		response.BadRequest(c, "ID 无效")
		return 0, false
	}
	return id, true
}

func contextUserID(c *gin.Context) (uint64, bool) {
	value, exists := c.Get("user_id")
	if !exists {
		return 0, false
	}
	switch typed := value.(type) {
	case uint:
		return uint64(typed), typed > 0
	case uint64:
		return typed, typed > 0
	case int:
		return uint64(typed), typed > 0
	case int64:
		return uint64(typed), typed > 0
	default:
		return 0, false
	}
}

func contextSessionID(c *gin.Context) (string, bool) {
	value, exists := c.Get("session_id")
	if !exists {
		return "", false
	}
	sessionID, ok := value.(string)
	return sessionID, ok && sessionID != "" && sessionID == strings.TrimSpace(sessionID)
}

func exportProof(req edgeCertExportRequest) (string, bool) {
	if req.StepUpProof != "" && req.Proof != "" && req.StepUpProof != req.Proof {
		return "", false
	}
	proof := req.StepUpProof
	if proof == "" {
		proof = req.Proof
	}
	return proof, proof != "" && proof == strings.TrimSpace(proof)
}

func setSensitiveDownloadHeaders(c *gin.Context, filename string) {
	setNoStoreHeaders(c)
	c.Header("Content-Disposition", mime.FormatMediaType("attachment", map[string]string{"filename": filename}))
	c.Header("X-Content-Type-Options", "nosniff")
}

func setNoStoreHeaders(c *gin.Context) {
	c.Header("Cache-Control", "no-store, private, max-age=0")
	c.Header("Pragma", "no-cache")
	c.Header("Expires", "0")
	c.Header("Vary", "Authorization, Cookie")
}

func contains(values []string, wanted string) bool {
	for _, value := range values {
		if value == wanted {
			return true
		}
	}
	return false
}

func writeMutationError(c *gin.Context, err error, fallback string) {
	switch {
	case errors.Is(err, edgecert.ErrCertificateBusy), errors.Is(err, edgecert.ErrCertificateDeployed):
		response.Error(c, http.StatusConflict, "证书正在使用中，无法修改")
	case errors.Is(err, edgecert.ErrProviderReconcileRequired):
		providerReconcileRequired(c)
	case errors.Is(err, gorm.ErrRecordNotFound):
		response.NotFound(c, "证书不存在")
	case errors.Is(err, edgecert.ErrEncryptionRequired):
		serviceUnavailable(c, "证书加密能力尚未就绪")
	case isSafeInputError(err):
		response.BadRequest(c, fallback)
	default:
		response.InternalServerError(c, "edge certificate mutation failed")
	}
}

func writeQueueError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, edgecert.ErrProviderReconcileRequired):
		providerReconcileRequired(c)
	case errors.Is(err, edgecert.ErrEncryptionRequired):
		serviceUnavailable(c, "证书加密能力尚未就绪")
	case errors.Is(err, edgecert.ErrWorkerDisabled):
		serviceUnavailable(c, "证书任务 worker 未启用")
	case errors.Is(err, edgecert.ErrCertificateBusy):
		response.Error(c, http.StatusConflict, "证书已有执行中的任务")
	case errors.Is(err, gorm.ErrRecordNotFound):
		response.NotFound(c, "证书不存在")
	case strings.Contains(err.Error(), "disabled"), strings.Contains(err.Error(), "not eligible"):
		response.Error(c, http.StatusConflict, "当前证书状态不允许执行此操作")
	default:
		response.InternalServerError(c, "edge certificate task enqueue failed")
	}
}

func providerReconcileRequired(c *gin.Context) {
	response.Error(c, http.StatusConflict, "CA 订单状态不确定，需人工核对 CA 订单，请勿重试创建")
}

func writeCertificateReadError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, gorm.ErrRecordNotFound):
		response.NotFound(c, "证书不存在")
	case strings.Contains(err.Error(), "not issued"):
		response.Error(c, http.StatusConflict, "证书尚未签发")
	default:
		response.InternalServerError(c, "edge certificate read failed")
	}
}

func writePrivateKeyReadError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, edgecert.ErrEncryptionRequired):
		serviceUnavailable(c, "证书加密能力尚未就绪")
	case errors.Is(err, gorm.ErrRecordNotFound):
		response.NotFound(c, "证书不存在")
	default:
		response.InternalServerError(c, "edge certificate private key read failed")
	}
}

func isSafeInputError(err error) bool {
	if err == nil {
		return false
	}
	message := err.Error()
	return message == "invalid domain" || message == "domain must be fully qualified" ||
		message == "valid email is required" || message == "unsupported deployment mode" ||
		message == "unsupported external deployment provider" ||
		message == "traefik file mode requires the traefik provider" ||
		message == "staging certificates cannot be installed on the gateway" ||
		message == "automatic renewal requires a production traefik-managed certificate" ||
		strings.Contains(message, "environment cannot change after issuance")
}

func serviceUnavailable(c *gin.Context, message string) {
	response.Error(c, http.StatusServiceUnavailable, message)
}
