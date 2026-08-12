package edgecert

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/go-admin-kit/services/shared/pkg/audittrail"
	"github.com/go-admin-kit/services/shared/pkg/secretbox"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

var (
	ErrEncryptionRequired        = errors.New("edge certificate encryption key is required")
	ErrWorkerDisabled            = errors.New("edge certificate worker is disabled")
	ErrCertificateBusy           = errors.New("edge certificate has an active task")
	ErrCertificateDeployed       = errors.New("deployed edge certificate must be removed from the gateway before deletion")
	ErrProviderReconcileRequired = errors.New("edge certificate provider order requires manual reconciliation")
)

type Service struct {
	DB                 *gorm.DB
	Keyring            *secretbox.Keyring
	Issuer             Issuer
	Deployer           *FileDeployer
	Now                func() time.Time
	RenewBefore        time.Duration
	WorkerEnabled      bool
	ClearLegacySecrets bool
}

func NewService(db *gorm.DB, keyring *secretbox.Keyring, issuer Issuer, deployer *FileDeployer) *Service {
	if issuer == nil {
		issuer = ACMEIssuer{}
	}
	return &Service{DB: db, Keyring: keyring, Issuer: issuer, Deployer: deployer, WorkerEnabled: true}
}

func (s *Service) now() time.Time {
	if s.Now != nil {
		return s.Now().UTC()
	}
	return time.Now().UTC()
}

func (s *Service) issuer() Issuer {
	if s.Issuer != nil {
		return s.Issuer
	}
	return ACMEIssuer{}
}

func (s *Service) renewBefore() time.Duration {
	if s.RenewBefore > 0 {
		return s.RenewBefore
	}
	return 30 * 24 * time.Hour
}

func (s *Service) List(ctx context.Context) ([]PublicView, error) {
	if s.DB == nil {
		return nil, fmt.Errorf("database unavailable")
	}
	var rows []Certificate
	if err := s.DB.WithContext(ctx).Order("id DESC").Find(&rows).Error; err != nil {
		return nil, err
	}
	now := s.now()
	latest, err := s.taskStates(ctx, 0)
	if err != nil {
		return nil, err
	}
	out := make([]PublicView, 0, len(rows))
	for i := range rows {
		view := rows[i].toPublicAt(now)
		applyTaskState(&view, latest[rows[i].ID])
		out = append(out, view)
	}
	return out, nil
}

func (s *Service) Get(ctx context.Context, id uint64) (*PublicView, error) {
	row, err := s.getCertificate(ctx, id)
	if err != nil {
		return nil, err
	}
	latest, err := s.taskStates(ctx, id)
	if err != nil {
		return nil, err
	}
	view := row.toPublicAt(s.now())
	applyTaskState(&view, latest[id])
	return &view, nil
}

type certificateTaskState struct {
	active   *Task
	issuance *Task
	renewal  *Task
}

func (s *Service) taskStates(ctx context.Context, certificateID uint64) (map[uint64]certificateTaskState, error) {
	query := s.DB.WithContext(ctx)
	if certificateID != 0 {
		query = query.Where("certificate_id = ?", certificateID)
	}
	var tasks []Task
	if err := query.Order("id DESC").Find(&tasks).Error; err != nil {
		return nil, err
	}
	out := make(map[uint64]certificateTaskState, len(tasks))
	for i := range tasks {
		task := tasks[i]
		state := out[task.CertificateID]
		if state.active == nil && (task.Status == TaskStatusQueued || task.Status == TaskStatusRunning) {
			copy := task
			state.active = &copy
		}
		if state.issuance == nil && (task.Kind == TaskKindIssue || task.Kind == TaskKindRenew) {
			copy := task
			state.issuance = &copy
		}
		if state.renewal == nil && task.Kind == TaskKindRenew {
			copy := task
			state.renewal = &copy
		}
		out[task.CertificateID] = state
	}
	return out, nil
}

func applyTaskState(view *PublicView, state certificateTaskState) {
	if view == nil {
		return
	}
	if state.active != nil {
		copy := *state.active
		view.ActiveTask = &copy
	}
	if state.issuance != nil {
		view.Issuance.Status = state.issuance.Status
		if state.issuance.Status == TaskStatusFailed {
			view.Issuance.LastError = state.issuance.ErrorMessage
		}
	}
	if state.renewal != nil {
		switch state.renewal.Status {
		case TaskStatusQueued, TaskStatusRunning, TaskStatusFailed:
			view.Renewal.Status = state.renewal.Status
		}
		if state.renewal.Status == TaskStatusFailed {
			view.Renewal.LastError = state.renewal.ErrorMessage
		}
	}
}

func (s *Service) getCertificate(ctx context.Context, id uint64) (*Certificate, error) {
	if s.DB == nil {
		return nil, fmt.Errorf("database unavailable")
	}
	var row Certificate
	if err := s.DB.WithContext(ctx).First(&row, id).Error; err != nil {
		return nil, err
	}
	return &row, nil
}

type UpsertInput struct {
	Domain             string `json:"domain"`
	Email              string `json:"email"`
	IsStaging          bool   `json:"is_staging"`
	DeploymentMode     string `json:"deployment_mode"`
	DeploymentProvider string `json:"deployment_provider"`
	AutoRenewEnabled   *bool  `json:"auto_renew_enabled,omitempty"`
}

func (s *Service) UpsertDraft(ctx context.Context, in UpsertInput) (*PublicView, error) {
	domain, err := canonicalDomain(in.Domain)
	if err != nil {
		return nil, err
	}
	email := strings.TrimSpace(in.Email)
	if email == "" || !strings.Contains(email, "@") {
		return nil, fmt.Errorf("valid email is required")
	}
	mode := strings.TrimSpace(in.DeploymentMode)
	if mode == "" {
		mode = DeploymentModeExternal
	}
	if mode != DeploymentModeExternal && mode != DeploymentModeTraefikFile {
		return nil, fmt.Errorf("unsupported deployment mode")
	}
	if in.IsStaging && mode == DeploymentModeTraefikFile {
		return nil, fmt.Errorf("staging certificates cannot be installed on the gateway")
	}
	if in.AutoRenewEnabled != nil && *in.AutoRenewEnabled && (in.IsStaging || mode != DeploymentModeTraefikFile) {
		return nil, fmt.Errorf("automatic renewal requires a production traefik-managed certificate")
	}
	provider, err := normalizeDeploymentProvider(mode, in.DeploymentProvider)
	if err != nil {
		return nil, err
	}
	now := s.now()
	var row Certificate
	err = s.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("domain = ?", domain).First(&row).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			autoRenew := !in.IsStaging && mode == DeploymentModeTraefikFile
			if in.AutoRenewEnabled != nil {
				autoRenew = *in.AutoRenewEnabled
			}
			deploymentStatus := DeploymentStatusExternal
			if mode == DeploymentModeTraefikFile {
				deploymentStatus = DeploymentStatusNotDeployed
			}
			row = Certificate{
				Domain: domain, Email: email, Status: StatusDraft, Provider: "letsencrypt",
				IsStaging: in.IsStaging, DeploymentMode: mode, DeploymentProvider: provider,
				AutoRenewEnabled: autoRenew, DeploymentStatus: deploymentStatus,
				ServingStatus: ServingStatusUnchecked, CreatedAt: now, UpdatedAt: now,
			}
			return tx.Omit("private_key_pem", "account_key_pem").Create(&row).Error
		}
		if err != nil {
			return err
		}
		var active int64
		if err := tx.Model(&Task{}).Where("certificate_id = ? AND status IN ?", row.ID, []string{TaskStatusQueued, TaskStatusRunning}).Count(&active).Error; err != nil {
			return err
		}
		if active != 0 {
			return ErrCertificateBusy
		}
		var recoveryCount int64
		if err := tx.Model(&Task{}).Where(
			"certificate_id = ? AND kind IN ? AND status <> ? AND (COALESCE(provider_order_uri, '') <> '' OR COALESCE(provider_cert_key_enc, '') <> '' OR error_code = ?)",
			row.ID, []string{TaskKindIssue, TaskKindRenew}, TaskStatusSucceeded, "provider_state_uncertain",
		).Count(&recoveryCount).Error; err != nil {
			return err
		}
		// The persisted account/order state is bound to the original ACME
		// environment and account contact. Deployment presentation may still be
		// edited, but provider identity must remain stable until reconciliation.
		if recoveryCount != 0 && (row.IsStaging != in.IsStaging || row.Email != email) {
			return ErrProviderReconcileRequired
		}
		if row.IsStaging != in.IsStaging && row.FullchainPEM != "" {
			return fmt.Errorf("environment cannot change after issuance; create a new certificate record")
		}
		if row.DeploymentMode != mode && (row.DeploymentStatus == DeploymentStatusInstalled || row.DeployedFingerprintSHA256 != "") {
			return ErrCertificateDeployed
		}
		autoRenew := row.AutoRenewEnabled
		if in.AutoRenewEnabled != nil {
			autoRenew = *in.AutoRenewEnabled
		}
		if in.IsStaging || mode == DeploymentModeExternal {
			autoRenew = false
		}
		deploymentStatus := row.DeploymentStatus
		if mode == DeploymentModeExternal && deploymentStatus != DeploymentStatusInstalled {
			deploymentStatus = DeploymentStatusExternal
		} else if mode == DeploymentModeTraefikFile && deploymentStatus == DeploymentStatusExternal {
			deploymentStatus = DeploymentStatusNotDeployed
		}
		updates := map[string]any{
			"email": email, "is_staging": in.IsStaging, "deployment_mode": mode,
			"deployment_provider": provider, "deployment_status": deploymentStatus,
			"auto_renew_enabled": autoRenew, "updated_at": now,
		}
		if row.Status == StatusFailed {
			updates["status"], updates["last_error"] = StatusDraft, ""
		}
		if err := tx.Model(&Certificate{}).Where("id = ?", row.ID).Updates(updates).Error; err != nil {
			return err
		}
		return tx.First(&row, row.ID).Error
	})
	if err != nil {
		return nil, err
	}
	view := row.toPublicAt(now)
	return &view, nil
}

func normalizeDeploymentProvider(mode, provider string) (string, error) {
	provider = strings.TrimSpace(strings.ToLower(provider))
	if mode == DeploymentModeTraefikFile {
		if provider != "" && provider != "traefik" && provider != "traefik_file" {
			return "", fmt.Errorf("traefik file mode requires the traefik provider")
		}
		return "traefik", nil
	}
	if provider == "" {
		provider = "external"
	}
	switch provider {
	case "external", "caddy", "cdn", "other":
		return provider, nil
	default:
		return "", fmt.Errorf("unsupported external deployment provider")
	}
}

// QueueTask 只持久化任务，不在 HTTP 请求里执行网络或文件操作。
func (s *Service) QueueTask(ctx context.Context, certificateID uint64, kind string, requestedBy uint64) (*Task, error) {
	if !s.WorkerEnabled {
		return nil, ErrWorkerDisabled
	}
	if !validTaskKind(kind) {
		return nil, fmt.Errorf("unsupported task kind")
	}
	if (kind == TaskKindIssue || kind == TaskKindRenew || kind == TaskKindDeploy) && s.Keyring == nil {
		return nil, ErrEncryptionRequired
	}
	var task Task
	err := s.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var cert Certificate
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&cert, certificateID).Error; err != nil {
			return err
		}
		if kind == TaskKindRenew && (cert.FullchainPEM == "" || cert.PrivateKeyEnc == "" || cert.NotAfter == nil || !cert.NotAfter.After(s.now())) {
			return fmt.Errorf("an unexpired issued certificate is required for renewal")
		}
		if kind == TaskKindDeploy && (cert.IsStaging || cert.DeploymentMode != DeploymentModeTraefikFile) {
			return fmt.Errorf("certificate is not eligible for gateway deployment")
		}
		existing, err := findActiveTask(tx, certificateID)
		if err == nil {
			if existing.Kind != kind {
				return ErrCertificateBusy
			}
			task = *existing
			return nil
		}
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		if kind == TaskKindIssue || kind == TaskKindRenew {
			var uncertain Task
			uncertainErr := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where(
				"certificate_id = ? AND kind IN ? AND status = ? AND ((step = ? AND COALESCE(provider_order_uri, '') = '') OR error_code = ?)",
				certificateID, []string{TaskKindIssue, TaskKindRenew}, TaskStatusFailed, "order_creating", "provider_state_uncertain",
			).Order("id DESC").First(&uncertain).Error
			if uncertainErr == nil {
				return ErrProviderReconcileRequired
			}
			if !errors.Is(uncertainErr, gorm.ErrRecordNotFound) {
				return uncertainErr
			}
			var recovery Task
			recoveryQuery := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where(
				"certificate_id = ? AND kind IN ? AND status = ? AND provider_order_uri <> '' AND provider_cert_key_enc <> ''",
				certificateID, []string{TaskKindIssue, TaskKindRenew}, TaskStatusFailed,
			)
			recoveryErr := recoveryQuery.Order("id DESC").First(&recovery).Error
			if recoveryErr == nil {
				now := s.now()
				if err := tx.Model(&Task{}).Where("id = ?", recovery.ID).Updates(map[string]any{
					"kind": kind, "status": TaskStatusQueued, "requested_by": requestedBy,
					"attempt_count": 0, "run_after": now, "lease_owner": "", "lease_until": nil,
					"error_code": "", "error_message": "", "error_hint": "", "finished_at": nil,
					"updated_at": now,
				}).Error; err != nil {
					return err
				}
				if err := tx.First(&task, recovery.ID).Error; err != nil {
					return err
				}
				return nil
			}
			if !errors.Is(recoveryErr, gorm.ErrRecordNotFound) {
				return recoveryErr
			}
		}
		task = Task{
			CertificateID: certificateID, Kind: kind, Status: TaskStatusQueued, Step: "queued",
			Environment: environmentFor(cert), RequestedBy: requestedBy,
			RunAfter: s.now(), CreatedAt: s.now(), UpdatedAt: s.now(),
		}
		created := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&task)
		if created.Error != nil {
			return created.Error
		}
		if created.RowsAffected == 0 {
			winner, winnerErr := findActiveTask(tx, certificateID)
			if winnerErr != nil {
				return winnerErr
			}
			if winner.Kind != kind {
				return ErrCertificateBusy
			}
			task = *winner
			return nil
		}
		if kind == TaskKindIssue && cert.FullchainPEM == "" {
			return tx.Model(&Certificate{}).Where("id = ?", certificateID).Updates(map[string]any{
				"status": StatusPending, "last_error": "", "updated_at": s.now(),
			}).Error
		}
		if kind == TaskKindDeploy {
			return tx.Model(&Certificate{}).Where("id = ?", certificateID).Updates(map[string]any{
				"deployment_status": DeploymentStatusQueued, "updated_at": s.now(),
			}).Error
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return &task, nil
}

func findActiveTask(db *gorm.DB, certificateID uint64) (*Task, error) {
	var task Task
	err := db.Where("certificate_id = ? AND status IN ?", certificateID, []string{TaskStatusQueued, TaskStatusRunning}).Order("id DESC").First(&task).Error
	return &task, err
}

func validTaskKind(kind string) bool {
	switch kind {
	case TaskKindIssue, TaskKindRenew, TaskKindDeploy, TaskKindProbe:
		return true
	default:
		return false
	}
}

func environmentFor(cert Certificate) string {
	if cert.IsStaging {
		return EnvironmentStaging
	}
	return EnvironmentProduction
}

func (s *Service) ListTasks(ctx context.Context, certificateID uint64, limit int) ([]Task, error) {
	if limit <= 0 || limit > 100 {
		limit = 50
	}
	query := s.DB.WithContext(ctx).Order("id DESC").Limit(limit)
	if certificateID != 0 {
		query = query.Where("certificate_id = ?", certificateID)
	}
	var tasks []Task
	if err := query.Find(&tasks).Error; err != nil {
		return nil, err
	}
	return tasks, nil
}

func (s *Service) Capabilities() Capabilities {
	return Capabilities{
		EncryptionConfigured: s.Keyring != nil, AsyncTasks: s.WorkerEnabled, DurableHTTP01: true,
		DeploymentModes: []string{DeploymentModeExternal, DeploymentModeTraefikFile}, AutomaticRenewal: s.WorkerEnabled,
	}
}

func (s *Service) ExportCertificate(ctx context.Context, id uint64) (fullchain, domain string, err error) {
	row, err := s.getCertificate(ctx, id)
	if err != nil {
		return "", "", err
	}
	if strings.TrimSpace(row.FullchainPEM) == "" {
		return "", "", fmt.Errorf("certificate not issued")
	}
	return row.FullchainPEM, row.Domain, nil
}

// ExportPrivateKey 的调用方必须完成独立导出授权；该方法绝不回退读取旧明文列。
func (s *Service) ExportPrivateKey(ctx context.Context, id uint64) (key []byte, domain string, err error) {
	row, err := s.getCertificate(ctx, id)
	if err != nil {
		return nil, "", err
	}
	if s.Keyring == nil {
		return nil, "", ErrEncryptionRequired
	}
	if row.PrivateKeyPEM != "" || row.AccountKeyPEM != "" {
		if err := s.rewrapCertificateSecrets(ctx, id); err != nil {
			return nil, "", fmt.Errorf("secure legacy certificate secrets: %w", err)
		}
		row, err = s.getCertificate(ctx, id)
		if err != nil {
			return nil, "", err
		}
	}
	if row.PrivateKeyEnc == "" {
		return nil, "", fmt.Errorf("encrypted private key unavailable")
	}
	plaintext, _, err := s.Keyring.Open(row.PrivateKeyEnc, secretAAD(*row, "private_key"))
	if err != nil {
		return nil, "", fmt.Errorf("decrypt private key: %w", err)
	}
	return plaintext, row.Domain, nil
}

// Issue 是旧 API 的异步兼容门面，返回入队后的最新公开状态。
func (s *Service) Issue(ctx context.Context, id uint64) (*PublicView, error) {
	if _, err := s.QueueTask(ctx, id, TaskKindIssue, 0); err != nil {
		return nil, err
	}
	return s.Get(ctx, id)
}

func (s *Service) Delete(ctx context.Context, id uint64) error {
	return s.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var cert Certificate
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&cert, id).Error; err != nil {
			return err
		}
		if cert.DeploymentStatus == DeploymentStatusInstalled || cert.DeployedFingerprintSHA256 != "" {
			return ErrCertificateDeployed
		}
		var count int64
		if err := tx.Model(&Task{}).Where("certificate_id = ? AND status IN ?", id, []string{TaskStatusQueued, TaskStatusRunning}).Count(&count).Error; err != nil {
			return err
		}
		if count != 0 {
			return ErrCertificateBusy
		}
		var recoveryCount int64
		if err := tx.Model(&Task{}).Where(
			"certificate_id = ? AND kind IN ? AND status <> ? AND (COALESCE(provider_order_uri, '') <> '' OR COALESCE(provider_cert_key_enc, '') <> '' OR error_code = ?)",
			id, []string{TaskKindIssue, TaskKindRenew}, TaskStatusSucceeded, "provider_state_uncertain",
		).Count(&recoveryCount).Error; err != nil {
			return err
		}
		if recoveryCount != 0 {
			return ErrProviderReconcileRequired
		}
		if err := tx.Where("certificate_id = ?", id).Delete(&Challenge{}).Error; err != nil {
			return err
		}
		return tx.Delete(&cert).Error
	})
}

func secretAAD(cert Certificate, field string) string {
	return fmt.Sprintf("edge_tls_certificates:%d:%s:%s", cert.ID, strings.ToLower(strings.TrimSpace(cert.Domain)), field)
}

// MigrateLegacySecrets 加密旧明文并重包旧 key。每行在同一事务中 seal/clear，失败即不清明文。
func (s *Service) MigrateLegacySecrets(ctx context.Context) error {
	ctx = workerAuditContext(ctx)
	var count, taskSecretCount int64
	if err := s.DB.WithContext(ctx).Model(&Certificate{}).Where(
		"private_key_pem <> '' OR account_key_pem <> '' OR private_key_enc <> '' OR account_key_enc <> ''",
	).Count(&count).Error; err != nil {
		return err
	}
	if err := s.DB.WithContext(ctx).Model(&Task{}).Where("provider_cert_key_enc <> ''").Count(&taskSecretCount).Error; err != nil {
		return err
	}
	if count == 0 && taskSecretCount == 0 {
		return nil
	}
	if s.Keyring == nil {
		return ErrEncryptionRequired
	}
	var ids []uint64
	if err := s.DB.WithContext(ctx).Model(&Certificate{}).Where(
		"private_key_pem <> '' OR account_key_pem <> '' OR private_key_enc <> '' OR account_key_enc <> ''",
	).Pluck("id", &ids).Error; err != nil {
		return err
	}
	for _, id := range ids {
		if err := s.rewrapCertificateSecrets(ctx, id); err != nil {
			return fmt.Errorf("secure edge certificate %d: %w", id, err)
		}
	}
	var taskIDs []uint64
	if err := s.DB.WithContext(ctx).Model(&Task{}).Where("provider_cert_key_enc <> ''").Pluck("id", &taskIDs).Error; err != nil {
		return err
	}
	for _, id := range taskIDs {
		if err := s.rewrapTaskProviderKey(ctx, id); err != nil {
			return fmt.Errorf("secure edge certificate task %d: %w", id, err)
		}
	}
	return nil
}

func (s *Service) rewrapTaskProviderKey(ctx context.Context, id uint64) error {
	return s.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var task Task
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&task, id).Error; err != nil {
			return err
		}
		if task.ProviderCertKeyEnc == "" {
			return nil
		}
		var cert Certificate
		if err := tx.First(&cert, task.CertificateID).Error; err != nil {
			return err
		}
		rewrapped, changed, err := s.Keyring.Rewrap(task.ProviderCertKeyEnc, taskSecretAAD(task, cert, "provider_cert_key"))
		if err != nil {
			return err
		}
		if !changed {
			return nil
		}
		return tx.Model(&Task{}).Where("id = ?", task.ID).Updates(map[string]any{
			"provider_cert_key_enc": rewrapped, "updated_at": s.now(),
		}).Error
	})
}

func (s *Service) rewrapCertificateSecrets(ctx context.Context, id uint64) error {
	return s.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var cert Certificate
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&cert, id).Error; err != nil {
			return err
		}
		updates := map[string]any{}
		fields := []struct {
			legacy          string
			envelope        string
			name            string
			legacyColumn    string
			encryptedColumn string
		}{
			{cert.PrivateKeyPEM, cert.PrivateKeyEnc, "private_key", "private_key_pem", "private_key_enc"},
			{cert.AccountKeyPEM, cert.AccountKeyEnc, "account_key", "account_key_pem", "account_key_enc"},
		}
		for _, field := range fields {
			envelope := field.envelope
			if field.legacy != "" {
				if envelope != "" {
					existing, _, err := s.Keyring.Open(envelope, secretAAD(cert, field.name))
					if err != nil {
						return err
					}
					matches := bytes.Equal(existing, []byte(field.legacy))
					clear(existing)
					if !matches {
						envelope = ""
					}
				}
			}
			if envelope == "" && field.legacy != "" {
				sealed, err := s.Keyring.Seal([]byte(field.legacy), secretAAD(cert, field.name))
				if err != nil {
					return err
				}
				envelope = sealed
				updates[field.encryptedColumn] = sealed
			} else if envelope != "" {
				rewrapped, changed, err := s.Keyring.Rewrap(envelope, secretAAD(cert, field.name))
				if err != nil {
					return err
				}
				if changed {
					updates[field.encryptedColumn] = rewrapped
				}
			}
			if s.ClearLegacySecrets && field.legacy != "" && envelope != "" {
				updates[field.legacyColumn] = nil
			}
		}
		if len(updates) == 0 {
			return nil
		}
		updates["updated_at"] = s.now()
		return tx.Model(&Certificate{}).Where("id = ?", id).Updates(updates).Error
	})
}

// ValidateSecurityState 在启动 Worker 前使用；有敏感数据或可执行任务却没有 key 时拒绝启动。
func (s *Service) ValidateSecurityState(ctx context.Context) error {
	if s.Keyring != nil {
		return nil
	}
	var secretCount, taskCount int64
	if err := s.DB.WithContext(ctx).Model(&Certificate{}).Where(
		"private_key_pem <> '' OR account_key_pem <> '' OR private_key_enc <> '' OR account_key_enc <> ''",
	).Count(&secretCount).Error; err != nil {
		return err
	}
	if err := s.DB.WithContext(ctx).Model(&Task{}).Where(
		"status IN ? OR provider_cert_key_enc <> ''", []string{TaskStatusQueued, TaskStatusRunning},
	).Count(&taskCount).Error; err != nil {
		return err
	}
	if secretCount != 0 || taskCount != 0 {
		return ErrEncryptionRequired
	}
	return nil
}

func workerAuditContext(ctx context.Context) context.Context {
	return audittrail.WithTenantID(audittrail.WithActor(ctx, "system", "edge-cert-worker"), 1)
}
