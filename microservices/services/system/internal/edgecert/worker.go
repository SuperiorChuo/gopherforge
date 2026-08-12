package edgecert

import (
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const (
	defaultPollInterval  = 5 * time.Second
	defaultLeaseDuration = 2 * time.Minute
	defaultScanInterval  = time.Hour
)

type Worker struct {
	Service         *Service
	ID              string
	PollInterval    time.Duration
	LeaseDuration   time.Duration
	RenewalInterval time.Duration
	MaxConcurrency  int
	OnError         func(error)
	ActivationWait  time.Duration
	ActivationPoll  time.Duration
}

func NewWorker(service *Service, workerID string) *Worker {
	return &Worker{Service: service, ID: strings.TrimSpace(workerID), MaxConcurrency: 2}
}

func (w *Worker) pollInterval() time.Duration {
	if w.PollInterval > 0 {
		return w.PollInterval
	}
	return defaultPollInterval
}

func (w *Worker) leaseDuration() time.Duration {
	if w.LeaseDuration > 0 {
		return w.LeaseDuration
	}
	return defaultLeaseDuration
}

func (w *Worker) renewalInterval() time.Duration {
	if w.RenewalInterval > 0 {
		return w.RenewalInterval
	}
	return defaultScanInterval
}

func (w *Worker) report(err error) {
	if err != nil && w.OnError != nil {
		w.OnError(err)
	}
}

// Run 持续抢占持久化任务。调用前先验证/迁移密钥状态，避免在缺 key 时静默运行。
func (w *Worker) Run(ctx context.Context) error {
	if w == nil || w.Service == nil || w.Service.DB == nil {
		return fmt.Errorf("edge certificate worker is not configured")
	}
	if w.ID == "" {
		w.ID = "system-" + uuid.NewString()
	}
	ctx = workerAuditContext(ctx)
	if err := w.Service.ValidateSecurityState(ctx); err != nil {
		return err
	}
	if err := w.Service.MigrateLegacySecrets(ctx); err != nil {
		return err
	}
	max := w.MaxConcurrency
	if max <= 0 {
		max = 2
	}
	semaphore := make(chan struct{}, max)
	var workers sync.WaitGroup
	poll := time.NewTicker(w.pollInterval())
	defer poll.Stop()
	renew := time.NewTicker(w.renewalInterval())
	defer renew.Stop()

	// Startup passes make queued work and already-due renewals prompt without a thundering loop.
	w.report(w.Service.ScheduleRenewals(ctx))
	w.tryStart(ctx, semaphore, &workers)
	for {
		select {
		case <-ctx.Done():
			workers.Wait()
			return nil
		case <-poll.C:
			w.tryStart(ctx, semaphore, &workers)
		case <-renew.C:
			w.report(w.Service.ScheduleRenewals(ctx))
			w.report(CleanupExpiredChallenges(ctx, w.Service.DB, w.Service.now()))
		}
	}
}

func (w *Worker) tryStart(ctx context.Context, semaphore chan struct{}, workers *sync.WaitGroup) {
	select {
	case semaphore <- struct{}{}:
	default:
		return
	}
	task, err := w.claim(ctx)
	if err != nil {
		<-semaphore
		w.report(err)
		return
	}
	if task == nil {
		<-semaphore
		return
	}
	workers.Add(1)
	go func() {
		defer workers.Done()
		defer func() { <-semaphore }()
		w.report(w.runClaim(ctx, *task))
	}()
}

func (w *Worker) claim(ctx context.Context) (*Task, error) {
	now := w.Service.now()
	leaseUntil := now.Add(w.leaseDuration())
	workerID := w.ID
	if len(workerID) > 20 {
		workerID = workerID[:20]
	}
	leaseToken := workerID + ":" + uuid.NewString()
	var claimed Task
	err := w.Service.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var task Task
		err := tx.Clauses(clause.Locking{Strength: "UPDATE", Options: "SKIP LOCKED"}).
			Where("(status = ? AND run_after <= ?) OR (status = ? AND lease_until < ?)",
				TaskStatusQueued, now, TaskStatusRunning, now).
			Order("run_after ASC, id ASC").First(&task).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil
		}
		if err != nil {
			return err
		}
		updates := map[string]any{
			"status": TaskStatusRunning, "lease_owner": leaseToken,
			"lease_until": leaseUntil, "attempt_count": gorm.Expr("attempt_count + 1"), "updated_at": now,
		}
		if task.Status == TaskStatusQueued && (task.Step == "" || task.Step == "queued") {
			updates["step"] = "claimed"
		}
		if task.StartedAt == nil {
			updates["started_at"] = now
		}
		result := tx.Model(&Task{}).Where("id = ?", task.ID).Updates(updates)
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return ErrLeaseLost
		}
		if err := tx.First(&claimed, task.ID).Error; err != nil {
			return err
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	if claimed.ID == 0 {
		return nil, nil
	}
	return &claimed, nil
}

var (
	ErrLeaseLost              = errors.New("edge certificate task lease lost")
	ErrProviderStateUncertain = errors.New("edge certificate provider state is uncertain")
	ErrProviderOrderTerminal  = errors.New("edge certificate provider order is terminal")
)

func (w *Worker) runClaim(parent context.Context, task Task) error {
	timeout := 5 * time.Minute
	switch task.Kind {
	case TaskKindDeploy:
		timeout = time.Minute
	case TaskKindProbe:
		timeout = 30 * time.Second
	}
	ctx, cancel := context.WithTimeout(parent, timeout)
	defer cancel()
	heartbeatDone := make(chan struct{})
	go w.heartbeat(ctx, cancel, task, heartbeatDone)
	followup, err := w.processTask(ctx, task)
	close(heartbeatDone)
	if err != nil {
		if errors.Is(err, context.Canceled) {
			if releaseErr := w.releaseCanceledTask(context.WithoutCancel(parent), task); releaseErr != nil && !errors.Is(releaseErr, ErrLeaseLost) {
				return errors.Join(err, releaseErr)
			}
			return err
		}
		if retry, retryErr := w.retryTransient(context.WithoutCancel(parent), task, err); retryErr != nil {
			return errors.Join(err, retryErr)
		} else if retry {
			return fmt.Errorf("edge certificate task %d retry scheduled: %w", task.ID, err)
		}
		if markErr := w.markFailed(context.WithoutCancel(parent), task, err); markErr != nil {
			return errors.Join(err, markErr)
		}
		return fmt.Errorf("edge certificate task %d failed: %w", task.ID, err)
	}
	return w.completeAndQueue(context.WithoutCancel(parent), task, followup)
}

func (w *Worker) releaseCanceledTask(ctx context.Context, task Task) error {
	now := w.Service.now()
	result := w.Service.DB.WithContext(ctx).Model(&Task{}).Where(
		"id = ? AND status = ? AND lease_owner = ?", task.ID, TaskStatusRunning, task.LeaseOwner,
	).Updates(map[string]any{
		"status": TaskStatusQueued, "lease_owner": "", "lease_until": nil,
		"run_after": now, "error_code": "", "error_message": "", "error_hint": "",
		"finished_at": nil, "updated_at": now,
	})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected != 1 {
		return ErrLeaseLost
	}
	return nil
}

func (w *Worker) completeAndQueue(ctx context.Context, task Task, followup string) error {
	now := w.Service.now()
	return w.Service.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		result := tx.Model(&Task{}).Where(
			"id = ? AND status = ? AND lease_owner = ?", task.ID, TaskStatusRunning, task.LeaseOwner,
		).Updates(map[string]any{
			"status": TaskStatusSucceeded, "step": "completed", "lease_owner": "", "lease_until": nil,
			"error_code": "", "error_message": "", "error_hint": "", "finished_at": now, "updated_at": now,
		})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return ErrLeaseLost
		}
		if followup == "" {
			return nil
		}
		if !validTaskKind(followup) {
			return fmt.Errorf("unsupported follow-up task kind")
		}
		var cert Certificate
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&cert, task.CertificateID).Error; err != nil {
			return err
		}
		next := Task{
			CertificateID: cert.ID, Kind: followup, Status: TaskStatusQueued, Step: "queued",
			Environment: environmentFor(cert), RequestedBy: task.RequestedBy,
			RunAfter: now, CreatedAt: now, UpdatedAt: now,
		}
		return tx.Create(&next).Error
	})
}

func (w *Worker) retryTransient(ctx context.Context, task Task, taskErr error) (bool, error) {
	if task.AttemptCount >= 3 || !isTransientTaskError(task.Kind, taskErr) {
		return false, nil
	}
	now := w.Service.now()
	backoff := time.Duration(1<<max(task.AttemptCount-1, 0)) * time.Minute
	code, hint := classifyTaskError(taskErr)
	result := w.Service.DB.WithContext(ctx).Model(&Task{}).Where(
		"id = ? AND status = ? AND lease_owner = ?", task.ID, TaskStatusRunning, task.LeaseOwner,
	).Updates(map[string]any{
		"status": TaskStatusQueued, "lease_owner": "", "lease_until": nil,
		"run_after": now.Add(backoff), "error_code": code, "error_message": safeTaskMessage(taskErr),
		"error_hint": hint, "updated_at": now,
	})
	if result.Error != nil {
		return false, result.Error
	}
	if result.RowsAffected != 1 {
		return false, ErrLeaseLost
	}
	return true, nil
}

func isTransientTaskError(kind string, err error) bool {
	if errors.Is(err, ErrEncryptionRequired) || errors.Is(err, ErrLeaseLost) || errors.Is(err, ErrProviderStateUncertain) || errors.Is(err, ErrProviderOrderTerminal) || errors.Is(err, context.Canceled) {
		return false
	}
	message := strings.ToLower(err.Error())
	for _, marker := range []string{"invalid domain", "hostname mismatch", "fingerprint mismatch", "incomplete", "not eligible", "not configured", "unsupported", "decrypt", "encrypt", "legacy"} {
		if strings.Contains(message, marker) {
			return false
		}
	}
	return kind == TaskKindIssue || kind == TaskKindRenew || kind == TaskKindDeploy || kind == TaskKindProbe
}

func (w *Worker) heartbeat(ctx context.Context, cancel context.CancelFunc, task Task, done <-chan struct{}) {
	interval := w.leaseDuration() / 3
	if interval < time.Second {
		interval = time.Second
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-done:
			return
		case <-ctx.Done():
			return
		case <-ticker.C:
			now := w.Service.now()
			result := w.Service.DB.WithContext(ctx).Model(&Task{}).
				Where("id = ? AND status = ? AND lease_owner = ?", task.ID, TaskStatusRunning, task.LeaseOwner).
				Updates(map[string]any{"lease_until": now.Add(w.leaseDuration()), "updated_at": now})
			if result.Error != nil || result.RowsAffected != 1 {
				cancel()
				return
			}
		}
	}
}

func (w *Worker) processTask(ctx context.Context, task Task) (string, error) {
	if (task.Kind == TaskKindIssue || task.Kind == TaskKindRenew) && task.Step == "order_creating" && strings.TrimSpace(task.ProviderOrderURI) == "" {
		return "", ErrProviderStateUncertain
	}
	// A successful side effect and its durable step are committed together. A
	// reclaimed expired lease resumes after that boundary instead of reissuing.
	switch task.Step {
	case "issued":
		cert, err := w.Service.getCertificate(ctx, task.CertificateID)
		if err != nil {
			return "", err
		}
		if cert.DeploymentMode == DeploymentModeTraefikFile && !cert.IsStaging {
			return TaskKindDeploy, nil
		}
		return TaskKindProbe, nil
	case "deployed":
		if task.Kind == TaskKindDeploy {
			cert, err := w.Service.getCertificate(ctx, task.CertificateID)
			if err != nil {
				return "", err
			}
			if cert.DeploymentStatus != DeploymentStatusInstalled || cert.DeployedFingerprintSHA256 == "" {
				return "", fmt.Errorf("deployment activation was not committed")
			}
		}
		return TaskKindProbe, nil
	case "probed":
		return "", nil
	}
	switch task.Kind {
	case TaskKindIssue, TaskKindRenew:
		return w.processIssuance(ctx, task)
	case TaskKindDeploy:
		if err := w.processDeployment(ctx, task); err != nil {
			return "", err
		}
		return TaskKindProbe, nil
	case TaskKindProbe:
		return "", w.processProbe(ctx, task)
	default:
		return "", fmt.Errorf("unsupported task kind %q", task.Kind)
	}
}

func (w *Worker) processIssuance(ctx context.Context, task Task) (string, error) {
	cert, err := w.Service.getCertificate(ctx, task.CertificateID)
	if err != nil {
		return "", err
	}
	if w.Service.Keyring == nil {
		return "", ErrEncryptionRequired
	}
	if cert.PrivateKeyPEM != "" || cert.AccountKeyPEM != "" {
		if err := w.Service.rewrapCertificateSecrets(ctx, cert.ID); err != nil {
			return "", fmt.Errorf("secure legacy certificate secrets: %w", err)
		}
		cert, err = w.Service.getCertificate(ctx, cert.ID)
		if err != nil {
			return "", err
		}
	}
	var accountPEM []byte
	if cert.AccountKeyEnc != "" {
		accountPEM, _, err = w.Service.Keyring.Open(cert.AccountKeyEnc, secretAAD(*cert, "account_key"))
		if err != nil {
			return "", fmt.Errorf("decrypt account key: %w", err)
		}
	} else if cert.AccountKeyPEM != "" {
		return "", fmt.Errorf("legacy account key was not migrated")
	}
	openedAccountPEM := accountPEM
	_, accountPEM, err = loadOrCreateAccountKey(openedAccountPEM)
	clear(openedAccountPEM)
	if err != nil {
		return "", err
	}
	defer clear(accountPEM)

	var certKeyPEM []byte
	if task.ProviderCertKeyEnc != "" {
		certKeyPEM, _, err = w.Service.Keyring.Open(task.ProviderCertKeyEnc, taskSecretAAD(task, *cert, "provider_cert_key"))
		if err != nil {
			return "", fmt.Errorf("decrypt provider certificate key: %w", err)
		}
	}
	openedCertKeyPEM := certKeyPEM
	_, certKeyPEM, err = loadOrCreateCertificateKey(openedCertKeyPEM)
	clear(openedCertKeyPEM)
	if err != nil {
		return "", err
	}
	defer clear(certKeyPEM)

	initialStep := "keys_persisted"
	if task.Step == "order_created" || task.Step == "finalizing" {
		initialStep = ""
	}
	if err := w.persistIssueProgress(ctx, task, *cert, IssueProgress{
		AccountKeyPEM: accountPEM,
		CertKeyPEM:    certKeyPEM,
		Step:          initialStep,
	}); err != nil {
		return "", err
	}
	persistProgress := func(progressCtx context.Context, progress IssueProgress) error {
		return w.persistIssueProgress(progressCtx, task, *cert, progress)
	}
	result, err := w.Service.issuer().Issue(ctx, IssueRequest{
		CertificateID: cert.ID, Domain: cert.Domain, Email: cert.Email, IsStaging: cert.IsStaging,
		AccountKeyPEM: accountPEM, CertKeyPEM: certKeyPEM, OrderURI: task.ProviderOrderURI,
		Challenges: DBChallengeStore{DB: w.Service.DB, Now: w.Service.Now}, PersistProgress: persistProgress,
	})
	if err != nil {
		if errors.Is(err, ErrProviderOrderTerminal) {
			if clearErr := w.clearProviderRecovery(ctx, task); clearErr != nil {
				return "", clearErr
			}
		}
		return "", err
	}
	defer clear(result.PrivateKeyPEM)
	defer clear(result.AccountKeyPEM)
	if !bytes.Equal(bytes.TrimSpace(result.PrivateKeyPEM), bytes.TrimSpace(certKeyPEM)) {
		return "", fmt.Errorf("issuer returned a certificate key that differs from durable provider state")
	}
	if !bytes.Equal(bytes.TrimSpace(result.AccountKeyPEM), bytes.TrimSpace(accountPEM)) {
		return "", fmt.Errorf("issuer returned an account key that differs from durable provider state")
	}
	leaf, err := validateIssueResult(*cert, result)
	if err != nil {
		return "", err
	}
	result.NotBefore = leaf.NotBefore.UTC()
	result.NotAfter = leaf.NotAfter.UTC()
	result.FingerprintSHA256 = fingerprintDER(leaf.Raw)
	privateEnc, err := w.Service.Keyring.Seal(result.PrivateKeyPEM, secretAAD(*cert, "private_key"))
	if err != nil {
		return "", fmt.Errorf("encrypt private key: %w", err)
	}
	accountEnc, err := w.Service.Keyring.Seal(result.AccountKeyPEM, secretAAD(*cert, "account_key"))
	if err != nil {
		return "", fmt.Errorf("encrypt account key: %w", err)
	}
	now := w.Service.now()
	renewAt := result.NotAfter.Add(-w.Service.renewBefore()).UTC()
	err = w.withLease(ctx, task, func(tx *gorm.DB) error {
		updates := map[string]any{
			"fullchain_pem": result.FullchainPEM, "private_key_enc": privateEnc, "account_key_enc": accountEnc,
			"private_key_pem": nil, "account_key_pem": nil, "cert_fingerprint_sha256": result.FingerprintSHA256,
			"not_before": result.NotBefore.UTC(), "not_after": result.NotAfter.UTC(), "renew_at": renewAt,
			"status": StatusIssued, "last_error": "", "updated_at": now,
		}
		if task.Kind == TaskKindRenew {
			updates["last_renewal_at"] = now
		}
		if cert.DeploymentMode == DeploymentModeTraefikFile && !cert.IsStaging {
			updates["deployment_status"] = DeploymentStatusQueued
		}
		if err := tx.Model(&Certificate{}).Where("id = ?", cert.ID).Updates(updates).Error; err != nil {
			return err
		}
		return tx.Model(&Task{}).Where("id = ?", task.ID).Updates(map[string]any{
			"step": "issued", "provider_cert_key_enc": nil, "updated_at": now,
		}).Error
	})
	if err != nil {
		return "", err
	}
	if cert.DeploymentMode == DeploymentModeTraefikFile && !cert.IsStaging {
		return TaskKindDeploy, nil
	}
	return TaskKindProbe, nil
}

func (w *Worker) clearProviderRecovery(ctx context.Context, task Task) error {
	return w.withLease(ctx, task, func(tx *gorm.DB) error {
		return tx.Model(&Task{}).Where("id = ?", task.ID).Updates(map[string]any{
			"provider_order_uri": nil, "provider_cert_key_enc": nil, "updated_at": w.Service.now(),
		}).Error
	})
}

func taskSecretAAD(task Task, cert Certificate, field string) string {
	return fmt.Sprintf("edge_cert_tasks:%d:%s:%s", task.ID, strings.ToLower(strings.TrimSpace(cert.Domain)), field)
}

func (w *Worker) persistIssueProgress(ctx context.Context, task Task, cert Certificate, progress IssueProgress) error {
	if len(bytes.TrimSpace(progress.AccountKeyPEM)) == 0 || len(bytes.TrimSpace(progress.CertKeyPEM)) == 0 {
		return fmt.Errorf("issuance progress is missing durable key material")
	}
	if progress.Step != "" && progress.Step != "keys_persisted" && progress.Step != "order_creating" && progress.Step != "order_created" && progress.Step != "finalizing" {
		return fmt.Errorf("invalid issuance progress step")
	}
	accountEnc, err := w.Service.Keyring.Seal(progress.AccountKeyPEM, secretAAD(cert, "account_key"))
	if err != nil {
		return fmt.Errorf("encrypt account key: %w", err)
	}
	certKeyEnc, err := w.Service.Keyring.Seal(progress.CertKeyPEM, taskSecretAAD(task, cert, "provider_cert_key"))
	if err != nil {
		return fmt.Errorf("encrypt provider certificate key: %w", err)
	}
	now := w.Service.now()
	return w.withLease(ctx, task, func(tx *gorm.DB) error {
		var current Task
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&current, task.ID).Error; err != nil {
			return err
		}
		existingURI := strings.TrimSpace(current.ProviderOrderURI)
		incomingURI := strings.TrimSpace(progress.OrderURI)
		if existingURI != "" && incomingURI != "" && existingURI != incomingURI {
			return ErrProviderStateUncertain
		}
		effectiveURI := existingURI
		if effectiveURI == "" {
			effectiveURI = incomingURI
		}
		switch progress.Step {
		case "order_creating":
			if effectiveURI != "" {
				return ErrProviderStateUncertain
			}
		case "order_created", "finalizing":
			if effectiveURI == "" {
				return ErrProviderStateUncertain
			}
		}
		if err := tx.Model(&Certificate{}).Where("id = ?", cert.ID).Updates(map[string]any{
			"account_key_enc": accountEnc, "account_key_pem": nil, "updated_at": now,
		}).Error; err != nil {
			return err
		}
		updates := map[string]any{"provider_cert_key_enc": certKeyEnc, "updated_at": now}
		if incomingURI != "" {
			updates["provider_order_uri"] = incomingURI
		}
		if progress.Step != "" {
			updates["step"] = progress.Step
		}
		return tx.Model(&Task{}).Where("id = ?", task.ID).Updates(updates).Error
	})
}

func validateIssueResult(cert Certificate, result IssueResult) (*x509.Certificate, error) {
	if strings.TrimSpace(result.FullchainPEM) == "" || len(result.PrivateKeyPEM) == 0 || len(result.AccountKeyPEM) == 0 {
		return nil, fmt.Errorf("issuer returned incomplete secret material")
	}
	pair, err := tlsKeyPair(result.FullchainPEM, result.PrivateKeyPEM)
	if err != nil {
		return nil, err
	}
	if err := pair.VerifyHostname(cert.Domain); err != nil {
		return nil, fmt.Errorf("issued certificate hostname mismatch: %w", err)
	}
	if !pair.NotAfter.After(pair.NotBefore) || !pair.NotAfter.After(time.Now().UTC()) {
		return nil, fmt.Errorf("issuer returned invalid certificate lifetime")
	}
	fingerprint := fingerprintDER(pair.Raw)
	if result.FingerprintSHA256 != "" && result.FingerprintSHA256 != fingerprint {
		return nil, fmt.Errorf("issuer returned inconsistent fingerprint")
	}
	return pair, nil
}

func tlsKeyPair(fullchain string, privateKey []byte) (*x509.Certificate, error) {
	pair, err := tls.X509KeyPair([]byte(fullchain), privateKey)
	if err != nil {
		return nil, fmt.Errorf("issued certificate/key mismatch: %w", err)
	}
	if len(pair.Certificate) == 0 {
		return nil, fmt.Errorf("issued certificate chain is empty")
	}
	leaf, err := x509.ParseCertificate(pair.Certificate[0])
	if err != nil {
		return nil, fmt.Errorf("parse issued leaf: %w", err)
	}
	return leaf, nil
}

func (w *Worker) processDeployment(ctx context.Context, task Task) error {
	if err := w.updateStep(ctx, task, "installing"); err != nil {
		return err
	}
	cert, err := w.Service.getCertificate(ctx, task.CertificateID)
	if err != nil {
		return err
	}
	if cert.IsStaging || cert.DeploymentMode != DeploymentModeTraefikFile {
		return fmt.Errorf("certificate is not eligible for gateway deployment")
	}
	if w.Service.Deployer == nil {
		return fmt.Errorf("traefik file deployment is not configured")
	}
	if err := w.withLease(ctx, task, func(tx *gorm.DB) error {
		return tx.Model(&Certificate{}).Where("id = ?", cert.ID).Updates(map[string]any{
			"deployment_status": DeploymentStatusRunning, "updated_at": w.Service.now(),
		}).Error
	}); err != nil {
		return err
	}
	if err := w.ensureLease(ctx, task); err != nil {
		return err
	}
	privateKey, _, err := w.Service.ExportPrivateKey(ctx, cert.ID)
	if err != nil {
		return err
	}
	defer clear(privateKey)
	result, err := w.Service.Deployer.Deploy(ctx, *cert, privateKey)
	if err != nil {
		_ = w.withLease(context.WithoutCancel(ctx), task, func(tx *gorm.DB) error {
			return tx.Model(&Certificate{}).Where("id = ?", cert.ID).Updates(map[string]any{
				"deployment_status": DeploymentStatusFailed, "updated_at": w.Service.now(),
			}).Error
		})
		return err
	}
	if err := w.updateStep(ctx, task, "probing"); err != nil {
		_ = w.Service.Deployer.Rollback(result)
		return err
	}
	probe, probeErr := w.waitForFingerprint(ctx, cert, result.FingerprintSHA256)
	if probeErr != nil || probe.FingerprintSHA256 != result.FingerprintSHA256 {
		rollbackErr := w.Service.Deployer.Rollback(result)
		if rollbackErr != nil {
			return fmt.Errorf("gateway activation verification failed and rollback failed")
		}
		if probeErr != nil {
			return fmt.Errorf("gateway activation verification failed")
		}
		return fmt.Errorf("gateway activation fingerprint mismatch")
	}
	if err := w.Service.Deployer.Commit(result); err != nil {
		return fmt.Errorf("commit gateway activation: %w", err)
	}
	return w.withLease(ctx, task, func(tx *gorm.DB) error {
		if err := tx.Model(&Certificate{}).Where("id = ?", cert.ID).Updates(map[string]any{
			"deployment_status":           DeploymentStatusInstalled,
			"deployed_fingerprint_sha256": result.FingerprintSHA256,
			"deployed_at":                 result.InstalledAt, "updated_at": w.Service.now(),
		}).Error; err != nil {
			return err
		}
		return tx.Model(&Task{}).Where("id = ?", task.ID).Update("step", "deployed").Error
	})
}

func (w *Worker) waitForFingerprint(ctx context.Context, cert *Certificate, expected string) (ProbeResult, error) {
	wait := w.ActivationWait
	if wait <= 0 {
		wait = 10 * time.Second
	}
	interval := w.ActivationPoll
	if interval <= 0 {
		interval = 500 * time.Millisecond
	}
	probeCtx, cancel := context.WithTimeout(ctx, wait)
	defer cancel()
	var last ProbeResult
	var lastErr error
	for {
		last, lastErr = w.Service.Deployer.ProbeCertificate(probeCtx, *cert)
		if lastErr == nil && last.FingerprintSHA256 == expected {
			return last, nil
		}
		timer := time.NewTimer(interval)
		select {
		case <-probeCtx.Done():
			timer.Stop()
			if lastErr != nil {
				return last, lastErr
			}
			return last, fmt.Errorf("gateway activation fingerprint mismatch")
		case <-timer.C:
		}
	}
}

func (w *Worker) processProbe(ctx context.Context, task Task) error {
	if err := w.updateStep(ctx, task, "probing"); err != nil {
		return err
	}
	cert, err := w.Service.getCertificate(ctx, task.CertificateID)
	if err != nil {
		return err
	}
	deployer := w.Service.Deployer
	if deployer == nil {
		deployer = &FileDeployer{}
	}
	probe, err := deployer.ProbeCertificate(ctx, *cert)
	if err != nil {
		_ = w.withLease(context.WithoutCancel(ctx), task, func(tx *gorm.DB) error {
			return tx.Model(&Certificate{}).Where("id = ?", cert.ID).Updates(map[string]any{
				"serving_status": probeErrorStatus(err), "serving_checked_at": w.Service.now(),
				"serving_error_code": "tls_probe_failed", "serving_error_message": safeTaskMessage(err),
				"updated_at": w.Service.now(),
			}).Error
		})
		return err
	}
	status := ServingStatusHealthy
	code, message := "", ""
	if cert.DeploymentMode == DeploymentModeTraefikFile && probe.FingerprintSHA256 != cert.CertFingerprintSHA256 {
		status, code, message = ServingStatusMismatch, "fingerprint_mismatch", "the managed gateway is serving a different certificate"
	} else {
		status, code, message = ServingStatusHealthy, "", ""
	}
	return w.withLease(ctx, task, func(tx *gorm.DB) error {
		if err := tx.Model(&Certificate{}).Where("id = ?", cert.ID).Updates(map[string]any{
			"serving_status": status, "serving_fingerprint_sha256": probe.FingerprintSHA256,
			"serving_not_after": probe.NotAfter, "serving_issuer": probe.Issuer,
			"serving_checked_at": probe.CheckedAt, "serving_error_code": code,
			"serving_error_message": message, "updated_at": w.Service.now(),
		}).Error; err != nil {
			return err
		}
		return tx.Model(&Task{}).Where("id = ?", task.ID).Update("step", "probed").Error
	})
}

func probeErrorStatus(err error) string {
	var unknownAuthority x509.UnknownAuthorityError
	var hostname x509.HostnameError
	var invalid x509.CertificateInvalidError
	if errors.As(err, &unknownAuthority) || errors.As(err, &hostname) || errors.As(err, &invalid) {
		return ServingStatusInvalid
	}
	return ServingStatusUnreachable
}

func (w *Worker) ensureLease(ctx context.Context, task Task) error {
	var count int64
	err := w.Service.DB.WithContext(ctx).Model(&Task{}).Where(
		"id = ? AND status = ? AND lease_owner = ?", task.ID, TaskStatusRunning, task.LeaseOwner,
	).Count(&count).Error
	if err != nil {
		return err
	}
	if count != 1 {
		return ErrLeaseLost
	}
	return nil
}

func (w *Worker) updateStep(ctx context.Context, task Task, step string) error {
	if len(step) == 0 || len(step) > 32 {
		return fmt.Errorf("invalid task step")
	}
	result := w.Service.DB.WithContext(ctx).Model(&Task{}).Where(
		"id = ? AND status = ? AND lease_owner = ?", task.ID, TaskStatusRunning, task.LeaseOwner,
	).Updates(map[string]any{"step": step, "updated_at": w.Service.now()})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected != 1 {
		return ErrLeaseLost
	}
	return nil
}

func (w *Worker) withLease(ctx context.Context, task Task, fn func(*gorm.DB) error) error {
	return w.Service.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var locked Task
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&locked, task.ID).Error; err != nil {
			return err
		}
		if locked.Status != TaskStatusRunning || locked.LeaseOwner != task.LeaseOwner {
			return ErrLeaseLost
		}
		return fn(tx)
	})
}

func (w *Worker) markFailed(ctx context.Context, task Task, taskErr error) error {
	now := w.Service.now()
	code, hint := classifyTaskError(taskErr)
	message := safeTaskMessage(taskErr)
	return w.Service.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		result := tx.Model(&Task{}).Where(
			"id = ? AND status = ? AND lease_owner = ?", task.ID, TaskStatusRunning, task.LeaseOwner,
		).Updates(map[string]any{
			"status": TaskStatusFailed, "step": "failed", "lease_owner": "", "lease_until": nil,
			"error_code": code, "error_message": message, "error_hint": hint, "finished_at": now, "updated_at": now,
		})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return ErrLeaseLost
		}
		var cert Certificate
		if err := tx.First(&cert, task.CertificateID).Error; err != nil {
			return err
		}
		if task.Kind == TaskKindDeploy {
			return tx.Model(&Certificate{}).Where("id = ?", cert.ID).Updates(map[string]any{
				"deployment_status": DeploymentStatusFailed, "updated_at": now,
			}).Error
		}
		if task.Kind == TaskKindProbe {
			return nil
		}
		// Never replace or downgrade an existing usable certificate after a failed renewal/issue.
		if cert.FullchainPEM == "" || cert.PrivateKeyEnc == "" {
			return tx.Model(&Certificate{}).Where("id = ?", cert.ID).Updates(map[string]any{
				"status": StatusFailed, "last_error": message, "updated_at": now,
			}).Error
		}
		return tx.Model(&Certificate{}).Where("id = ?", cert.ID).Updates(map[string]any{
			"last_error": message, "updated_at": now,
		}).Error
	})
}

func classifyTaskError(err error) (code, hint string) {
	switch {
	case errors.Is(err, ErrEncryptionRequired):
		return "encryption_key_unavailable", "configure the edge certificate encryption key and restart the worker"
	case errors.Is(err, ErrProviderStateUncertain):
		return "provider_state_uncertain", "manually reconcile the ACME provider order before starting another issuance"
	case errors.Is(err, ErrProviderOrderTerminal):
		return "provider_order_terminal", "fix domain validation or provider policy errors, then start a new issuance"
	case errors.Is(err, ErrLeaseLost):
		return "lease_lost", "the task was safely handed to another worker"
	case errors.Is(err, context.DeadlineExceeded):
		return "task_timeout", "check DNS and ACME reachability before retrying"
	default:
		return "task_failed", "inspect system service logs using the task id, then retry"
	}
}

func safeTaskMessage(err error) string {
	if err == nil {
		return ""
	}
	switch {
	case errors.Is(err, ErrEncryptionRequired):
		return "edge certificate encryption is unavailable"
	case errors.Is(err, ErrProviderStateUncertain):
		return "certificate authority order state requires manual reconciliation"
	case errors.Is(err, ErrProviderOrderTerminal):
		return "certificate authority order is no longer usable"
	case errors.Is(err, ErrLeaseLost):
		return "task ownership changed before completion"
	case errors.Is(err, context.DeadlineExceeded), errors.Is(err, context.Canceled):
		return "edge certificate operation timed out"
	}
	message := strings.ToLower(err.Error())
	switch {
	case strings.Contains(message, "challenge") || strings.Contains(message, "authorization"):
		return "ACME domain validation failed"
	case strings.Contains(message, "register") || strings.Contains(message, "order") || strings.Contains(message, "acme"):
		return "certificate authority request failed"
	case strings.Contains(message, "probe") || strings.Contains(message, "handshake") || strings.Contains(message, "connect"):
		return "TLS serving verification failed"
	case strings.Contains(message, "deploy") || strings.Contains(message, "activation") || strings.Contains(message, "traefik"):
		return "gateway certificate deployment failed"
	case strings.Contains(message, "decrypt") || strings.Contains(message, "encrypt") || strings.Contains(message, "secret"):
		return "certificate secret operation failed"
	default:
		return "edge certificate task failed"
	}
}

// ScheduleRenewals 幂等入队生产、file 部署且到期的自动续期证书。
func (s *Service) ScheduleRenewals(ctx context.Context) error {
	ctx = workerAuditContext(ctx)
	now := s.now()
	var ids []uint64
	err := s.DB.WithContext(ctx).Model(&Certificate{}).
		Where("is_staging = FALSE AND deployment_mode = ? AND auto_renew_enabled = TRUE AND status = ? AND renew_at IS NOT NULL AND renew_at <= ?",
			DeploymentModeTraefikFile, StatusIssued, now).
		Order("renew_at ASC").Limit(100).Pluck("id", &ids).Error
	if err != nil {
		return err
	}
	for _, id := range ids {
		if _, err := s.QueueTask(ctx, id, TaskKindRenew, 0); err != nil && !errors.Is(err, ErrCertificateBusy) {
			return err
		}
	}
	return nil
}
