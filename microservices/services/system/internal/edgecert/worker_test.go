package edgecert

import (
	"context"
	"crypto"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"database/sql"
	"encoding/json"
	"encoding/pem"
	"errors"
	"math/big"
	"strings"
	"testing"
	"time"

	"github.com/go-admin-kit/services/shared/pkg/secretbox"
	"golang.org/x/crypto/acme"
)

type fakeIssuer struct {
	result IssueResult
	err    error
	seen   chan IssueRequest
}

type issuerFunc func(context.Context, IssueRequest) (IssueResult, error)

func (f issuerFunc) Issue(ctx context.Context, req IssueRequest) (IssueResult, error) {
	return f(ctx, req)
}

type transientRecoveryIssuer struct {
	calls     int
	orderURIs []string
}

func (i *transientRecoveryIssuer) Issue(_ context.Context, req IssueRequest) (IssueResult, error) {
	i.calls++
	i.orderURIs = append(i.orderURIs, req.OrderURI)
	return IssueResult{}, errors.New("certificate authority order lookup failed")
}

func (f fakeIssuer) Issue(_ context.Context, req IssueRequest) (IssueResult, error) {
	if f.seen != nil {
		f.seen <- req
	}
	result := f.result
	result.PrivateKeyPEM = append([]byte(nil), f.result.PrivateKeyPEM...)
	result.AccountKeyPEM = append([]byte(nil), f.result.AccountKeyPEM...)
	return result, f.err
}

func TestIssuancePersistsOnlyEncryptedKeys(t *testing.T) {
	db := openEdgeCertTestDB(t)
	ring := testKeyring(t)
	chain, key, leaf := issueTestCertificate(t, "admin.example.com", time.Now().Add(90*24*time.Hour))
	_, accountKey, err := loadOrCreateAccountKey(nil)
	if err != nil {
		t.Fatal(err)
	}
	defer clear(accountKey)
	issuer := fakeIssuer{result: IssueResult{
		FullchainPEM: chain, PrivateKeyPEM: key, AccountKeyPEM: accountKey,
		NotBefore: leaf.NotBefore, NotAfter: leaf.NotAfter, FingerprintSHA256: fingerprintDER(leaf.Raw),
	}}
	svc := NewService(db, ring, issuer, nil)
	now := time.Now().UTC()
	cert := Certificate{
		Domain: "admin.example.com", Email: "ops@example.com", Provider: "letsencrypt",
		Status: StatusDraft, DeploymentMode: DeploymentModeExternal, DeploymentProvider: "external",
		DeploymentStatus: DeploymentStatusExternal, ServingStatus: ServingStatusUnchecked,
		CreatedAt: now, UpdatedAt: now,
	}
	if err := db.Create(&cert).Error; err != nil {
		t.Fatal(err)
	}
	task, err := svc.QueueTask(context.Background(), cert.ID, TaskKindIssue, 7)
	if err != nil {
		t.Fatalf("QueueTask() error = %v", err)
	}
	accountEnc, err := ring.Seal(accountKey, secretAAD(cert, "account_key"))
	if err != nil {
		t.Fatal(err)
	}
	providerKeyEnc, err := ring.Seal(key, taskSecretAAD(*task, cert, "provider_cert_key"))
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Model(&cert).Update("account_key_enc", accountEnc).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Model(task).Update("provider_cert_key_enc", providerKeyEnc).Error; err != nil {
		t.Fatal(err)
	}
	worker := NewWorker(svc, "worker-a")
	claimed, err := worker.claim(context.Background())
	if err != nil || claimed == nil || claimed.ID != task.ID {
		t.Fatalf("claim() = %#v/%v", claimed, err)
	}
	if err := worker.runClaim(context.Background(), *claimed); err != nil {
		t.Fatalf("runClaim() error = %v", err)
	}

	var stored Certificate
	if err := db.First(&stored, cert.ID).Error; err != nil {
		t.Fatal(err)
	}
	if stored.Status != StatusIssued || stored.PrivateKeyPEM != "" || stored.AccountKeyPEM != "" {
		t.Fatalf("stored lifecycle/plaintext = %q/%q/%q", stored.Status, stored.PrivateKeyPEM, stored.AccountKeyPEM)
	}
	if stored.PrivateKeyEnc == "" || stored.AccountKeyEnc == "" {
		t.Fatal("encrypted envelopes were not persisted")
	}
	if strings.Contains(stored.PrivateKeyEnc, "BEGIN") || strings.Contains(stored.AccountKeyEnc, "BEGIN") {
		t.Fatal("encrypted columns contain PEM material")
	}
	exported, _, err := svc.ExportPrivateKey(context.Background(), cert.ID)
	if err != nil || string(exported) != string(key) {
		t.Fatalf("ExportPrivateKey() = %q/%v", exported, err)
	}
	if stored.RenewAt == nil || !stored.RenewAt.Equal(leaf.NotAfter.UTC().Add(-30*24*time.Hour)) {
		t.Fatalf("renew_at = %v", stored.RenewAt)
	}
	var legacyPrivate, legacyAccount, providerCertKey sql.NullString
	if err := db.Raw(
		"SELECT c.private_key_pem, c.account_key_pem, t.provider_cert_key_enc FROM edge_tls_certificates c JOIN edge_cert_tasks t ON t.certificate_id = c.id WHERE c.id = ? AND t.id = ?",
		cert.ID, task.ID,
	).Row().Scan(&legacyPrivate, &legacyAccount, &providerCertKey); err != nil {
		t.Fatal(err)
	}
	if legacyPrivate.Valid || legacyAccount.Valid || providerCertKey.Valid {
		t.Fatalf("successful issuance did not NULL plaintext/task secret columns: private=%#v account=%#v task=%#v", legacyPrivate, legacyAccount, providerCertKey)
	}
}

func TestFailedRenewalPreservesExistingUsableCertificate(t *testing.T) {
	db := openEdgeCertTestDB(t)
	ring := testKeyring(t)
	chain, key, leaf := issueTestCertificate(t, "admin.example.com", time.Now().Add(60*24*time.Hour))
	now := time.Now().UTC()
	cert := Certificate{
		Domain: "admin.example.com", Email: "ops@example.com", Provider: "letsencrypt", Status: StatusIssued,
		DeploymentMode: DeploymentModeTraefikFile, DeploymentProvider: "traefik_file",
		DeploymentStatus: DeploymentStatusInstalled, ServingStatus: ServingStatusHealthy,
		AutoRenewEnabled: true, FullchainPEM: chain, CertFingerprintSHA256: fingerprintDER(leaf.Raw),
		NotBefore: ptrTime(leaf.NotBefore), NotAfter: ptrTime(leaf.NotAfter), CreatedAt: now, UpdatedAt: now,
	}
	if err := db.Create(&cert).Error; err != nil {
		t.Fatal(err)
	}
	privateEnc, err := ring.Seal(key, secretAAD(cert, "private_key"))
	if err != nil {
		t.Fatal(err)
	}
	_, accountPEM, err := loadOrCreateAccountKey(nil)
	if err != nil {
		t.Fatal(err)
	}
	defer clear(accountPEM)
	accountEnc, err := ring.Seal(accountPEM, secretAAD(cert, "account_key"))
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Model(&cert).Updates(map[string]any{"private_key_enc": privateEnc, "account_key_enc": accountEnc}).Error; err != nil {
		t.Fatal(err)
	}
	svc := NewService(db, ring, fakeIssuer{err: errors.New("issued certificate hostname mismatch")}, nil)
	task, err := svc.QueueTask(context.Background(), cert.ID, TaskKindRenew, 7)
	if err != nil {
		t.Fatal(err)
	}
	worker := NewWorker(svc, "worker-a")
	claimed, err := worker.claim(context.Background())
	if err != nil || claimed == nil || claimed.ID != task.ID {
		t.Fatalf("claim() = %#v/%v", claimed, err)
	}
	if err := worker.runClaim(context.Background(), *claimed); err == nil || !strings.Contains(err.Error(), "failed") {
		t.Fatalf("runClaim() error = %v, want observable terminal failure", err)
	}

	var stored Certificate
	if err := db.First(&stored, cert.ID).Error; err != nil {
		t.Fatal(err)
	}
	if stored.Status != StatusIssued || stored.FullchainPEM != chain || stored.PrivateKeyEnc != privateEnc || stored.CertFingerprintSHA256 != fingerprintDER(leaf.Raw) {
		t.Fatal("failed renewal replaced or downgraded the existing certificate")
	}
	var finished Task
	if err := db.First(&finished, task.ID).Error; err != nil {
		t.Fatal(err)
	}
	if finished.Status != TaskStatusFailed || finished.ErrorMessage != "edge certificate task failed" || strings.Contains(finished.ErrorMessage, "hostname") {
		t.Fatalf("finished task = %#v", finished)
	}
}

func TestLeaseCompetitionExpiryAndTokenGuard(t *testing.T) {
	db := openEdgeCertTestDB(t)
	svc := NewService(db, testKeyring(t), fakeIssuer{}, nil)
	now := time.Date(2026, 8, 12, 12, 0, 0, 0, time.UTC)
	svc.Now = func() time.Time { return now }
	cert := Certificate{
		Domain: "admin.example.com", Email: "ops@example.com", Provider: "letsencrypt", Status: StatusDraft,
		DeploymentMode: DeploymentModeExternal, DeploymentProvider: "external", DeploymentStatus: DeploymentStatusExternal,
		ServingStatus: ServingStatusUnchecked, CreatedAt: now, UpdatedAt: now,
	}
	if err := db.Create(&cert).Error; err != nil {
		t.Fatal(err)
	}
	if _, err := svc.QueueTask(context.Background(), cert.ID, TaskKindIssue, 1); err != nil {
		t.Fatal(err)
	}
	first := NewWorker(svc, "first")
	first.LeaseDuration = time.Minute
	second := NewWorker(svc, "second")
	second.LeaseDuration = time.Minute
	firstClaim, err := first.claim(context.Background())
	if err != nil || firstClaim == nil {
		t.Fatalf("first claim = %#v/%v", firstClaim, err)
	}
	secondClaim, err := second.claim(context.Background())
	if err != nil || secondClaim != nil {
		t.Fatalf("concurrent second claim = %#v/%v, want none", secondClaim, err)
	}
	now = now.Add(2 * time.Minute)
	secondClaim, err = second.claim(context.Background())
	if err != nil || secondClaim == nil || secondClaim.ID != firstClaim.ID || secondClaim.LeaseOwner == firstClaim.LeaseOwner {
		t.Fatalf("expired takeover = %#v/%v", secondClaim, err)
	}
	if err := first.completeAndQueue(context.Background(), *firstClaim, ""); !errors.Is(err, ErrLeaseLost) {
		t.Fatalf("stale lease completion error = %v, want ErrLeaseLost", err)
	}
}

func TestLegacySecretsMigrateAndRewrapAtomically(t *testing.T) {
	db := openEdgeCertTestDB(t)
	oldRing, err := secretbox.NewKeyring(secretbox.Key{ID: "old", Material: bytesOf(0x22, 32)})
	if err != nil {
		t.Fatal(err)
	}
	cert := Certificate{Domain: "admin.example.com", Email: "ops@example.com", Provider: "letsencrypt", Status: StatusIssued,
		DeploymentMode: DeploymentModeExternal, DeploymentStatus: DeploymentStatusExternal, ServingStatus: ServingStatusUnchecked,
		PrivateKeyPEM: "legacy-private", CreatedAt: time.Now(), UpdatedAt: time.Now()}
	if err := db.Create(&cert).Error; err != nil {
		t.Fatal(err)
	}
	oldAccount, err := oldRing.Seal([]byte("old-account"), secretAAD(cert, "account_key"))
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Model(&cert).Update("account_key_enc", oldAccount).Error; err != nil {
		t.Fatal(err)
	}
	rotated, err := secretbox.NewKeyring(
		secretbox.Key{ID: "new", Material: bytesOf(0x11, 32)},
		secretbox.Key{ID: "old", Material: bytesOf(0x22, 32)},
	)
	if err != nil {
		t.Fatal(err)
	}
	svc := NewService(db, rotated, nil, nil)
	svc.ClearLegacySecrets = true
	if err := svc.MigrateLegacySecrets(context.Background()); err != nil {
		t.Fatal(err)
	}
	var stored Certificate
	if err := db.First(&stored, cert.ID).Error; err != nil {
		t.Fatal(err)
	}
	if stored.PrivateKeyPEM != "" || stored.PrivateKeyEnc == "" || !strings.HasPrefix(stored.AccountKeyEnc, "v1.new.") {
		t.Fatalf("migration result = private plaintext %q, private envelope %q, account envelope %q", stored.PrivateKeyPEM, stored.PrivateKeyEnc, stored.AccountKeyEnc)
	}
	plaintext, _, err := rotated.Open(stored.PrivateKeyEnc, secretAAD(stored, "private_key"))
	if err != nil || string(plaintext) != "legacy-private" {
		t.Fatalf("migrated private key = %q/%v", plaintext, err)
	}
}

func TestLegacyWriterNewPlaintextReplacesStaleEnvelope(t *testing.T) {
	db := openEdgeCertTestDB(t)
	ring := testKeyring(t)
	cert := Certificate{Domain: "admin.example.com", Email: "ops@example.com", Provider: "letsencrypt", Status: StatusIssued,
		DeploymentMode: DeploymentModeExternal, DeploymentStatus: DeploymentStatusExternal, ServingStatus: ServingStatusUnchecked,
		CreatedAt: time.Now(), UpdatedAt: time.Now()}
	if err := db.Create(&cert).Error; err != nil {
		t.Fatal(err)
	}
	stale, err := ring.Seal([]byte("old-private-key"), secretAAD(cert, "private_key"))
	if err != nil {
		t.Fatal(err)
	}
	// Simulate an old process issuing after V2's first backfill: it can only write
	// the legacy column, while the envelope still contains the prior generation.
	if err := db.Model(&cert).Updates(map[string]any{"private_key_enc": stale, "private_key_pem": "new-private-key"}).Error; err != nil {
		t.Fatal(err)
	}
	svc := NewService(db, ring, nil, nil)
	svc.ClearLegacySecrets = true
	if err := svc.MigrateLegacySecrets(context.Background()); err != nil {
		t.Fatal(err)
	}
	var stored Certificate
	if err := db.First(&stored, cert.ID).Error; err != nil {
		t.Fatal(err)
	}
	plaintext, _, err := ring.Open(stored.PrivateKeyEnc, secretAAD(stored, "private_key"))
	if err != nil || string(plaintext) != "new-private-key" || stored.PrivateKeyPEM != "" {
		t.Fatalf("new legacy value not promoted: plaintext=%q legacy=%q err=%v", plaintext, stored.PrivateKeyPEM, err)
	}
}

func TestLegacyMigrationEncryptOnlyThenExplicitCleanup(t *testing.T) {
	db := openEdgeCertTestDB(t)
	ring := testKeyring(t)
	cert := Certificate{Domain: "admin.example.com", Email: "ops@example.com", Provider: "letsencrypt", Status: StatusIssued,
		DeploymentMode: DeploymentModeExternal, DeploymentStatus: DeploymentStatusExternal, ServingStatus: ServingStatusUnchecked,
		PrivateKeyPEM: "legacy-private", CreatedAt: time.Now(), UpdatedAt: time.Now()}
	if err := db.Create(&cert).Error; err != nil {
		t.Fatal(err)
	}
	svc := NewService(db, ring, nil, nil)
	if err := svc.MigrateLegacySecrets(context.Background()); err != nil {
		t.Fatal(err)
	}
	var phaseOne Certificate
	if err := db.First(&phaseOne, cert.ID).Error; err != nil {
		t.Fatal(err)
	}
	if phaseOne.PrivateKeyEnc == "" || phaseOne.PrivateKeyPEM != "legacy-private" {
		t.Fatalf("encrypt-only phase = envelope %q legacy %q", phaseOne.PrivateKeyEnc, phaseOne.PrivateKeyPEM)
	}
	svc.ClearLegacySecrets = true
	if err := svc.MigrateLegacySecrets(context.Background()); err != nil {
		t.Fatal(err)
	}
	var phaseTwo Certificate
	if err := db.First(&phaseTwo, cert.ID).Error; err != nil {
		t.Fatal(err)
	}
	if phaseTwo.PrivateKeyPEM != "" || phaseTwo.PrivateKeyEnc == "" {
		t.Fatalf("cleanup phase = envelope %q legacy %q", phaseTwo.PrivateKeyEnc, phaseTwo.PrivateKeyPEM)
	}
}

func TestTaskProviderKeyRewrapsAndFailsClosedWithoutKeyring(t *testing.T) {
	db := openEdgeCertTestDB(t)
	oldRing, err := secretbox.NewKeyring(secretbox.Key{ID: "old", Material: bytesOf(0x22, 32)})
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	cert := Certificate{
		Domain: "admin.example.com", Email: "ops@example.com", Provider: "letsencrypt", Status: StatusDraft,
		DeploymentMode: DeploymentModeExternal, DeploymentStatus: DeploymentStatusExternal,
		ServingStatus: ServingStatusUnchecked, CreatedAt: now, UpdatedAt: now,
	}
	if err := db.Create(&cert).Error; err != nil {
		t.Fatal(err)
	}
	task := Task{
		CertificateID: cert.ID, Kind: TaskKindIssue, Status: TaskStatusFailed, Step: "failed",
		Environment: EnvironmentProduction, RunAfter: now, CreatedAt: now, UpdatedAt: now,
	}
	if err := db.Create(&task).Error; err != nil {
		t.Fatal(err)
	}
	envelope, err := oldRing.Seal([]byte("durable-certificate-key"), taskSecretAAD(task, cert, "provider_cert_key"))
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Model(&task).Update("provider_cert_key_enc", envelope).Error; err != nil {
		t.Fatal(err)
	}
	if err := NewService(db, nil, nil, nil).ValidateSecurityState(context.Background()); !errors.Is(err, ErrEncryptionRequired) {
		t.Fatalf("ValidateSecurityState() error = %v, want ErrEncryptionRequired", err)
	}
	rotated, err := secretbox.NewKeyring(
		secretbox.Key{ID: "new", Material: bytesOf(0x11, 32)},
		secretbox.Key{ID: "old", Material: bytesOf(0x22, 32)},
	)
	if err != nil {
		t.Fatal(err)
	}
	if err := NewService(db, rotated, nil, nil).MigrateLegacySecrets(context.Background()); err != nil {
		t.Fatal(err)
	}
	var stored Task
	if err := db.First(&stored, task.ID).Error; err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(stored.ProviderCertKeyEnc, "v1.new.") {
		t.Fatalf("task provider key envelope was not rewrapped: %q", stored.ProviderCertKeyEnc)
	}
	plaintext, _, err := rotated.Open(stored.ProviderCertKeyEnc, taskSecretAAD(stored, cert, "provider_cert_key"))
	if err != nil || string(plaintext) != "durable-certificate-key" {
		t.Fatalf("rewrapped provider key = %q/%v", plaintext, err)
	}
	clear(plaintext)
}

func TestManualRenewAllowsExternalCertificate(t *testing.T) {
	db := openEdgeCertTestDB(t)
	ring := testKeyring(t)
	now := time.Now().UTC()
	cert := Certificate{Domain: "admin.example.com", Email: "ops@example.com", Provider: "letsencrypt", Status: StatusIssued,
		DeploymentMode: DeploymentModeExternal, DeploymentProvider: "caddy", DeploymentStatus: DeploymentStatusExternal,
		ServingStatus: ServingStatusHealthy, FullchainPEM: "issued-chain", NotAfter: ptrTime(now.Add(24 * time.Hour)), CreatedAt: now, UpdatedAt: now}
	if err := db.Create(&cert).Error; err != nil {
		t.Fatal(err)
	}
	envelope, err := ring.Seal([]byte("private"), secretAAD(cert, "private_key"))
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Model(&cert).Update("private_key_enc", envelope).Error; err != nil {
		t.Fatal(err)
	}
	svc := NewService(db, ring, nil, nil)
	if _, err := svc.QueueTask(context.Background(), cert.ID, TaskKindRenew, 7); err != nil {
		t.Fatalf("manual external renewal rejected: %v", err)
	}
}

func TestTransientFailureRequeuesWithBoundedBackoff(t *testing.T) {
	db := openEdgeCertTestDB(t)
	now := time.Date(2026, 8, 12, 12, 0, 0, 0, time.UTC)
	svc := NewService(db, testKeyring(t), fakeIssuer{err: errors.New("acme temporary outage")}, nil)
	svc.Now = func() time.Time { return now }
	cert := Certificate{Domain: "admin.example.com", Email: "ops@example.com", Provider: "letsencrypt", Status: StatusDraft,
		DeploymentMode: DeploymentModeExternal, DeploymentStatus: DeploymentStatusExternal, ServingStatus: ServingStatusUnchecked,
		CreatedAt: now, UpdatedAt: now}
	if err := db.Create(&cert).Error; err != nil {
		t.Fatal(err)
	}
	queued, err := svc.QueueTask(context.Background(), cert.ID, TaskKindIssue, 1)
	if err != nil {
		t.Fatal(err)
	}
	worker := NewWorker(svc, "worker-a")
	claimed, err := worker.claim(context.Background())
	if err != nil || claimed == nil {
		t.Fatalf("claim = %#v/%v", claimed, err)
	}
	if err := worker.runClaim(context.Background(), *claimed); err == nil || !strings.Contains(err.Error(), "retry scheduled") {
		t.Fatalf("runClaim() error = %v, want observable retry", err)
	}
	var retried Task
	if err := db.First(&retried, queued.ID).Error; err != nil {
		t.Fatal(err)
	}
	if retried.Status != TaskStatusQueued || retried.AttemptCount != 1 || !retried.RunAfter.Equal(now.Add(time.Minute)) {
		t.Fatalf("retry state = %#v", retried)
	}
	if retried.ErrorMessage != "certificate authority request failed" {
		t.Fatalf("unsafe retry message = %q", retried.ErrorMessage)
	}
}

func TestIssuanceKeysAreDurableBeforeIssuerCall(t *testing.T) {
	db := openEdgeCertTestDB(t)
	ring := testKeyring(t)
	now := time.Now().UTC()
	cert := Certificate{
		Domain: "admin.example.com", Email: "ops@example.com", Provider: "letsencrypt", Status: StatusDraft,
		DeploymentMode: DeploymentModeExternal, DeploymentProvider: "external",
		DeploymentStatus: DeploymentStatusExternal, ServingStatus: ServingStatusUnchecked,
		CreatedAt: now, UpdatedAt: now,
	}
	if err := db.Create(&cert).Error; err != nil {
		t.Fatal(err)
	}
	called := false
	svc := NewService(db, ring, issuerFunc(func(_ context.Context, req IssueRequest) (IssueResult, error) {
		called = true
		var storedCert Certificate
		var storedTask Task
		if err := db.First(&storedCert, cert.ID).Error; err != nil {
			t.Fatal(err)
		}
		if err := db.Where("certificate_id = ?", cert.ID).First(&storedTask).Error; err != nil {
			t.Fatal(err)
		}
		if storedCert.AccountKeyEnc == "" || storedTask.ProviderCertKeyEnc == "" {
			t.Fatalf("issuer observed non-durable keys: cert=%#v task=%#v", storedCert, storedTask)
		}
		account, _, err := ring.Open(storedCert.AccountKeyEnc, secretAAD(storedCert, "account_key"))
		if err != nil {
			t.Fatal(err)
		}
		defer clear(account)
		certKey, _, err := ring.Open(storedTask.ProviderCertKeyEnc, taskSecretAAD(storedTask, storedCert, "provider_cert_key"))
		if err != nil {
			t.Fatal(err)
		}
		defer clear(certKey)
		if string(account) != string(req.AccountKeyPEM) || string(certKey) != string(req.CertKeyPEM) {
			t.Fatal("persisted issuance keys differ from issuer request")
		}
		return IssueResult{}, errors.New("test issuer stop")
	}), nil)
	queued, err := svc.QueueTask(context.Background(), cert.ID, TaskKindIssue, 1)
	if err != nil {
		t.Fatal(err)
	}
	worker := NewWorker(svc, "worker-a")
	claimed, err := worker.claim(context.Background())
	if err != nil || claimed == nil || claimed.ID != queued.ID {
		t.Fatalf("claim = %#v/%v", claimed, err)
	}
	if _, err := worker.processIssuance(context.Background(), *claimed); err == nil {
		t.Fatal("processIssuance() unexpectedly succeeded")
	}
	if !called {
		t.Fatal("issuer was not called")
	}
}

func TestACMEIssuerRejectsMissingWorkerPersistedKeysBeforeClientCreation(t *testing.T) {
	clientCreations := 0
	issuer := ACMEIssuer{
		ProductionDirectory: "https://acme.test/directory",
		clientFactory: func(crypto.Signer, string) acmeClient {
			clientCreations++
			return &recoveryTestACMEClient{}
		},
	}
	_, err := issuer.Issue(context.Background(), IssueRequest{
		CertificateID: 1,
		Domain:        "admin.example.com",
		Email:         "ops@example.com",
		Challenges:    recoveryTestChallengeStore{},
		PersistProgress: func(context.Context, IssueProgress) error {
			return nil
		},
	})
	if err == nil || !strings.Contains(err.Error(), "worker-persisted key material") {
		t.Fatalf("Issue() error = %v", err)
	}
	if clientCreations != 0 {
		t.Fatalf("missing durable keys created provider client %d times", clientCreations)
	}
}

func TestOrderCreatingWithoutURIRequiresManualReconcile(t *testing.T) {
	db := openEdgeCertTestDB(t)
	now := time.Now().UTC()
	cert := Certificate{
		Domain: "admin.example.com", Email: "ops@example.com", Provider: "letsencrypt", Status: StatusDraft,
		DeploymentMode: DeploymentModeExternal, DeploymentProvider: "external",
		DeploymentStatus: DeploymentStatusExternal, ServingStatus: ServingStatusUnchecked,
		CreatedAt: now, UpdatedAt: now,
	}
	if err := db.Create(&cert).Error; err != nil {
		t.Fatal(err)
	}
	issuerCalls := 0
	svc := NewService(db, testKeyring(t), issuerFunc(func(context.Context, IssueRequest) (IssueResult, error) {
		issuerCalls++
		return IssueResult{}, nil
	}), nil)
	queued, err := svc.QueueTask(context.Background(), cert.ID, TaskKindIssue, 1)
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Model(queued).Update("step", "order_creating").Error; err != nil {
		t.Fatal(err)
	}
	worker := NewWorker(svc, "worker-a")
	claimed, err := worker.claim(context.Background())
	if err != nil || claimed == nil {
		t.Fatalf("claim = %#v/%v", claimed, err)
	}
	runErr := worker.runClaim(context.Background(), *claimed)
	if runErr == nil || !errors.Is(runErr, ErrProviderStateUncertain) {
		t.Fatalf("runClaim() error = %v", runErr)
	}
	if issuerCalls != 0 {
		t.Fatalf("uncertain provider state invoked issuer %d times", issuerCalls)
	}
	var finished Task
	if err := db.First(&finished, queued.ID).Error; err != nil {
		t.Fatal(err)
	}
	if finished.Status != TaskStatusFailed || finished.AttemptCount != 1 || finished.ErrorCode != "provider_state_uncertain" {
		t.Fatalf("uncertain task state = %#v", finished)
	}
	if finished.ErrorMessage != "certificate authority order state requires manual reconciliation" || !strings.Contains(finished.ErrorHint, "manually reconcile") {
		t.Fatalf("unsafe/unstable uncertainty details = %q/%q", finished.ErrorMessage, finished.ErrorHint)
	}
	for _, retryKind := range []string{TaskKindIssue, TaskKindRenew} {
		if retryKind == TaskKindRenew {
			chain, privateKey, leaf := issueTestCertificate(t, cert.Domain, now.Add(24*time.Hour))
			privateEnc, sealErr := svc.Keyring.Seal(privateKey, secretAAD(cert, "private_key"))
			if sealErr != nil {
				t.Fatal(sealErr)
			}
			if updateErr := db.Model(&cert).Updates(map[string]any{
				"fullchain_pem": chain, "private_key_enc": privateEnc,
				"not_before": leaf.NotBefore.UTC(), "not_after": leaf.NotAfter.UTC(), "status": StatusIssued,
			}).Error; updateErr != nil {
				t.Fatal(updateErr)
			}
		}
		if _, retryErr := svc.QueueTask(context.Background(), cert.ID, retryKind, 2); !errors.Is(retryErr, ErrProviderReconcileRequired) {
			t.Fatalf("QueueTask(%s) error = %v, want ErrProviderReconcileRequired", retryKind, retryErr)
		}
	}
	var count int64
	if err := db.Model(&Task{}).Where("certificate_id = ?", cert.ID).Count(&count).Error; err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("manual-reconcile retries created %d tasks, want 1", count)
	}
}

func TestCanceledRecoveredOrderRequeuesAndReclaimsSameState(t *testing.T) {
	db := openEdgeCertTestDB(t)
	ring := testKeyring(t)
	now := time.Now().UTC()
	cert := Certificate{
		Domain: "admin.example.com", Email: "ops@example.com", Provider: "letsencrypt", Status: StatusDraft,
		DeploymentMode: DeploymentModeExternal, DeploymentProvider: "external",
		DeploymentStatus: DeploymentStatusExternal, ServingStatus: ServingStatusUnchecked,
		CreatedAt: now, UpdatedAt: now,
	}
	if err := db.Create(&cert).Error; err != nil {
		t.Fatal(err)
	}
	queued, err := NewService(db, ring, fakeIssuer{}, nil).QueueTask(context.Background(), cert.ID, TaskKindIssue, 1)
	if err != nil {
		t.Fatal(err)
	}
	_, accountPEM, err := loadOrCreateAccountKey(nil)
	if err != nil {
		t.Fatal(err)
	}
	defer clear(accountPEM)
	_, certKeyPEM, err := loadOrCreateCertificateKey(nil)
	if err != nil {
		t.Fatal(err)
	}
	defer clear(certKeyPEM)
	accountEnc, err := ring.Seal(accountPEM, secretAAD(cert, "account_key"))
	if err != nil {
		t.Fatal(err)
	}
	providerKeyEnc, err := ring.Seal(certKeyPEM, taskSecretAAD(*queued, cert, "provider_cert_key"))
	if err != nil {
		t.Fatal(err)
	}
	certificateDER := recoveryTestCertificateDER(t, certKeyPEM, cert.Domain)
	client := &recoveryTestACMEClient{
		order: &acme.Order{
			URI: recoveryTestOrderURI, Status: acme.StatusValid, CertURL: recoveryTestCertURL,
		},
		getOrderErrors:    []error{context.Canceled},
		certificateBundle: [][]byte{certificateDER},
	}
	svc := NewService(db, ring, ACMEIssuer{
		ProductionDirectory: recoveryTestDirectory,
		clientFactory: func(crypto.Signer, string) acmeClient {
			return client
		},
	}, nil)
	if err := db.Model(&cert).Update("account_key_enc", accountEnc).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Model(queued).Updates(map[string]any{
		"step": "finalizing", "provider_order_uri": recoveryTestOrderURI, "provider_cert_key_enc": providerKeyEnc,
	}).Error; err != nil {
		t.Fatal(err)
	}
	worker := NewWorker(svc, "worker-a")
	claimed, err := worker.claim(context.Background())
	if err != nil || claimed == nil {
		t.Fatalf("claim = %#v/%v", claimed, err)
	}
	if err := worker.runClaim(context.Background(), *claimed); !errors.Is(err, context.Canceled) {
		t.Fatalf("runClaim() error = %v", err)
	}
	var released Task
	if err := db.First(&released, queued.ID).Error; err != nil {
		t.Fatal(err)
	}
	if released.Status != TaskStatusQueued || released.Step != "finalizing" || released.ProviderOrderURI != recoveryTestOrderURI || released.ProviderCertKeyEnc == "" || released.FinishedAt != nil {
		t.Fatalf("canceled recovery state = %#v", released)
	}
	releasedKey, _, err := ring.Open(released.ProviderCertKeyEnc, taskSecretAAD(released, cert, "provider_cert_key"))
	if err != nil || string(releasedKey) != string(certKeyPEM) {
		t.Fatalf("released provider key = %q/%v", releasedKey, err)
	}
	clear(releasedKey)
	reclaimed, err := worker.claim(context.Background())
	if err != nil || reclaimed == nil || reclaimed.ID != queued.ID || reclaimed.Step != "finalizing" || reclaimed.ProviderOrderURI != recoveryTestOrderURI {
		t.Fatalf("reclaim = %#v/%v", reclaimed, err)
	}
	if err := worker.runClaim(context.Background(), *reclaimed); err != nil {
		t.Fatalf("recovered runClaim() error = %v", err)
	}
	if client.authorizeOrderCalls != 0 || client.getOrderCalls != 2 || client.fetchCertCalls != 1 {
		t.Fatalf("recovered provider calls = authorize:%d get:%d fetch:%d", client.authorizeOrderCalls, client.getOrderCalls, client.fetchCertCalls)
	}
	for call, uri := range client.getOrderURIs {
		if uri != recoveryTestOrderURI {
			t.Fatalf("GetOrder call %d URI = %q", call+1, uri)
		}
	}
}

func TestQueueTaskRevivesFailedProviderRecoveryInsteadOfCreatingOrder(t *testing.T) {
	db := openEdgeCertTestDB(t)
	ring := testKeyring(t)
	now := time.Now().UTC()
	cert := Certificate{
		Domain: "admin.example.com", Email: "ops@example.com", Provider: "letsencrypt", Status: StatusDraft,
		DeploymentMode: DeploymentModeExternal, DeploymentProvider: "external",
		DeploymentStatus: DeploymentStatusExternal, ServingStatus: ServingStatusUnchecked,
		CreatedAt: now, UpdatedAt: now,
	}
	if err := db.Create(&cert).Error; err != nil {
		t.Fatal(err)
	}
	failed := Task{
		CertificateID: cert.ID, Kind: TaskKindIssue, Status: TaskStatusFailed, Step: "finalizing",
		Environment: EnvironmentProduction, RequestedBy: 1, AttemptCount: 3, RunAfter: now,
		ProviderOrderURI: recoveryTestOrderURI, ErrorCode: "task_failed", ErrorMessage: "certificate authority request failed",
		FinishedAt: ptrTime(now), CreatedAt: now, UpdatedAt: now,
	}
	if err := db.Create(&failed).Error; err != nil {
		t.Fatal(err)
	}
	_, certKeyPEM, err := loadOrCreateCertificateKey(nil)
	if err != nil {
		t.Fatal(err)
	}
	defer clear(certKeyPEM)
	providerKeyEnc, err := ring.Seal(certKeyPEM, taskSecretAAD(failed, cert, "provider_cert_key"))
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Model(&failed).Update("provider_cert_key_enc", providerKeyEnc).Error; err != nil {
		t.Fatal(err)
	}

	revived, err := NewService(db, ring, nil, nil).QueueTask(context.Background(), cert.ID, TaskKindIssue, 9)
	if err != nil {
		t.Fatal(err)
	}
	if revived.ID != failed.ID || revived.Status != TaskStatusQueued || revived.Step != "finalizing" || revived.ProviderOrderURI != recoveryTestOrderURI || revived.ProviderCertKeyEnc != providerKeyEnc {
		t.Fatalf("revived task = %#v", revived)
	}
	if revived.AttemptCount != 0 || revived.RequestedBy != 9 || revived.FinishedAt != nil || revived.ErrorCode != "" || revived.ErrorMessage != "" {
		t.Fatalf("revived retry metadata = %#v", revived)
	}
	var count int64
	if err := db.Model(&Task{}).Where("certificate_id = ?", cert.ID).Count(&count).Error; err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("QueueTask created %d provider tasks, want 1", count)
	}
}

func TestIssueRequestRevivesFailedRenewalProviderRecovery(t *testing.T) {
	db := openEdgeCertTestDB(t)
	ring := testKeyring(t)
	now := time.Now().UTC()
	chain, privateKey, leaf := issueTestCertificate(t, "admin.example.com", now.Add(60*24*time.Hour))
	cert := Certificate{
		Domain: "admin.example.com", Email: "ops@example.com", Provider: "letsencrypt", Status: StatusIssued,
		DeploymentMode: DeploymentModeExternal, DeploymentProvider: "external",
		DeploymentStatus: DeploymentStatusExternal, ServingStatus: ServingStatusHealthy,
		FullchainPEM: chain, NotBefore: ptrTime(leaf.NotBefore), NotAfter: ptrTime(leaf.NotAfter),
		CreatedAt: now, UpdatedAt: now,
	}
	if err := db.Create(&cert).Error; err != nil {
		t.Fatal(err)
	}
	privateEnc, err := ring.Seal(privateKey, secretAAD(cert, "private_key"))
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Model(&cert).Update("private_key_enc", privateEnc).Error; err != nil {
		t.Fatal(err)
	}
	failed := Task{
		CertificateID: cert.ID, Kind: TaskKindRenew, Status: TaskStatusFailed, Step: "finalizing",
		Environment: EnvironmentProduction, AttemptCount: 3, RunAfter: now,
		ProviderOrderURI: recoveryTestOrderURI, FinishedAt: ptrTime(now), CreatedAt: now, UpdatedAt: now,
	}
	if err := db.Create(&failed).Error; err != nil {
		t.Fatal(err)
	}
	_, certKeyPEM, err := loadOrCreateCertificateKey(nil)
	if err != nil {
		t.Fatal(err)
	}
	defer clear(certKeyPEM)
	providerKeyEnc, err := ring.Seal(certKeyPEM, taskSecretAAD(failed, cert, "provider_cert_key"))
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Model(&failed).Update("provider_cert_key_enc", providerKeyEnc).Error; err != nil {
		t.Fatal(err)
	}
	revived, err := NewService(db, ring, nil, nil).QueueTask(context.Background(), cert.ID, TaskKindIssue, 9)
	if err != nil {
		t.Fatal(err)
	}
	if revived.ID != failed.ID || revived.Kind != TaskKindIssue || revived.ProviderOrderURI != recoveryTestOrderURI || revived.ProviderCertKeyEnc == "" {
		t.Fatalf("issue request abandoned failed renewal recovery: %#v", revived)
	}
}

func TestRenewRequestRevivesFailedIssueProviderRecovery(t *testing.T) {
	db := openEdgeCertTestDB(t)
	ring := testKeyring(t)
	now := time.Now().UTC()
	chain, privateKey, leaf := issueTestCertificate(t, "admin.example.com", now.Add(60*24*time.Hour))
	cert := Certificate{
		Domain: "admin.example.com", Email: "ops@example.com", Provider: "letsencrypt", Status: StatusIssued,
		DeploymentMode: DeploymentModeExternal, DeploymentProvider: "external",
		DeploymentStatus: DeploymentStatusExternal, ServingStatus: ServingStatusHealthy,
		FullchainPEM: chain, NotBefore: ptrTime(leaf.NotBefore), NotAfter: ptrTime(leaf.NotAfter),
		CreatedAt: now, UpdatedAt: now,
	}
	if err := db.Create(&cert).Error; err != nil {
		t.Fatal(err)
	}
	privateEnc, err := ring.Seal(privateKey, secretAAD(cert, "private_key"))
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Model(&cert).Update("private_key_enc", privateEnc).Error; err != nil {
		t.Fatal(err)
	}
	failed := Task{
		CertificateID: cert.ID, Kind: TaskKindIssue, Status: TaskStatusFailed, Step: "order_created",
		Environment: EnvironmentProduction, AttemptCount: 3, RunAfter: now,
		ProviderOrderURI: recoveryTestOrderURI, FinishedAt: ptrTime(now), CreatedAt: now, UpdatedAt: now,
	}
	if err := db.Create(&failed).Error; err != nil {
		t.Fatal(err)
	}
	_, certKeyPEM, err := loadOrCreateCertificateKey(nil)
	if err != nil {
		t.Fatal(err)
	}
	defer clear(certKeyPEM)
	providerKeyEnc, err := ring.Seal(certKeyPEM, taskSecretAAD(failed, cert, "provider_cert_key"))
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Model(&failed).Update("provider_cert_key_enc", providerKeyEnc).Error; err != nil {
		t.Fatal(err)
	}
	revived, err := NewService(db, ring, nil, nil).QueueTask(context.Background(), cert.ID, TaskKindRenew, 9)
	if err != nil {
		t.Fatal(err)
	}
	if revived.ID != failed.ID || revived.Kind != TaskKindRenew || revived.ProviderOrderURI != recoveryTestOrderURI || revived.ProviderCertKeyEnc == "" {
		t.Fatalf("renew request abandoned failed issue recovery: %#v", revived)
	}
	var count int64
	if err := db.Model(&Task{}).Where("certificate_id = ?", cert.ID).Count(&count).Error; err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("renew request created %d tasks, want reused recovery task", count)
	}
}

func TestProviderRecoveryBlocksDeleteAndEnvironmentChange(t *testing.T) {
	for _, state := range []struct {
		name      string
		step      string
		orderURI  string
		errorCode string
		withKey   bool
	}{
		{name: "known order", step: "finalizing", orderURI: recoveryTestOrderURI, withKey: true},
		{name: "known order missing key", step: "finalizing", orderURI: recoveryTestOrderURI},
		{name: "uncertain order", step: "order_creating", errorCode: "provider_state_uncertain", withKey: true},
	} {
		t.Run(state.name, func(t *testing.T) {
			db := openEdgeCertTestDB(t)
			ring := testKeyring(t)
			now := time.Now().UTC()
			cert := Certificate{
				Domain: "admin.example.com", Email: "ops@example.com", Provider: "letsencrypt", Status: StatusFailed,
				IsStaging: true, DeploymentMode: DeploymentModeExternal, DeploymentProvider: "external",
				DeploymentStatus: DeploymentStatusExternal, ServingStatus: ServingStatusUnchecked,
				CreatedAt: now, UpdatedAt: now,
			}
			if err := db.Create(&cert).Error; err != nil {
				t.Fatal(err)
			}
			task := Task{
				CertificateID: cert.ID, Kind: TaskKindIssue, Status: TaskStatusFailed, Step: state.step,
				Environment: EnvironmentStaging, RunAfter: now, ProviderOrderURI: state.orderURI,
				ErrorCode: state.errorCode, FinishedAt: ptrTime(now), CreatedAt: now, UpdatedAt: now,
			}
			if err := db.Create(&task).Error; err != nil {
				t.Fatal(err)
			}
			_, certKeyPEM, err := loadOrCreateCertificateKey(nil)
			if err != nil {
				t.Fatal(err)
			}
			defer clear(certKeyPEM)
			providerKeyEnc, err := ring.Seal(certKeyPEM, taskSecretAAD(task, cert, "provider_cert_key"))
			if err != nil {
				t.Fatal(err)
			}
			if state.withKey {
				if err := db.Model(&task).Update("provider_cert_key_enc", providerKeyEnc).Error; err != nil {
					t.Fatal(err)
				}
			}
			svc := NewService(db, ring, nil, nil)
			if err := svc.Delete(context.Background(), cert.ID); !errors.Is(err, ErrProviderReconcileRequired) {
				t.Fatalf("Delete() error = %v, want ErrProviderReconcileRequired", err)
			}
			if _, err := svc.UpsertDraft(context.Background(), UpsertInput{
				Domain: cert.Domain, Email: cert.Email, IsStaging: false,
				DeploymentMode: DeploymentModeExternal, DeploymentProvider: "external",
			}); !errors.Is(err, ErrProviderReconcileRequired) {
				t.Fatalf("UpsertDraft() environment change error = %v, want ErrProviderReconcileRequired", err)
			}
			if _, err := svc.UpsertDraft(context.Background(), UpsertInput{
				Domain: cert.Domain, Email: "other@example.com", IsStaging: cert.IsStaging,
				DeploymentMode: DeploymentModeExternal, DeploymentProvider: "external",
			}); !errors.Is(err, ErrProviderReconcileRequired) {
				t.Fatalf("UpsertDraft() account contact change error = %v, want ErrProviderReconcileRequired", err)
			}
			var certCount, taskCount int64
			if err := db.Model(&Certificate{}).Where("id = ?", cert.ID).Count(&certCount).Error; err != nil {
				t.Fatal(err)
			}
			if err := db.Model(&Task{}).Where("id = ?", task.ID).Count(&taskCount).Error; err != nil {
				t.Fatal(err)
			}
			if certCount != 1 || taskCount != 1 {
				t.Fatalf("recovery state was deleted: certificate=%d task=%d", certCount, taskCount)
			}
		})
	}
}

func TestProviderTransientExhaustionRevivesSameTaskAndOrder(t *testing.T) {
	db := openEdgeCertTestDB(t)
	ring := testKeyring(t)
	now := time.Date(2026, 8, 12, 12, 0, 0, 0, time.UTC)
	cert := Certificate{
		Domain: "admin.example.com", Email: "ops@example.com", Provider: "letsencrypt", Status: StatusDraft,
		DeploymentMode: DeploymentModeExternal, DeploymentProvider: "external",
		DeploymentStatus: DeploymentStatusExternal, ServingStatus: ServingStatusUnchecked,
		CreatedAt: now, UpdatedAt: now,
	}
	if err := db.Create(&cert).Error; err != nil {
		t.Fatal(err)
	}
	issuer := new(transientRecoveryIssuer)
	svc := NewService(db, ring, issuer, nil)
	svc.Now = func() time.Time { return now }
	queued, err := svc.QueueTask(context.Background(), cert.ID, TaskKindIssue, 1)
	if err != nil {
		t.Fatal(err)
	}
	_, accountPEM, err := loadOrCreateAccountKey(nil)
	if err != nil {
		t.Fatal(err)
	}
	defer clear(accountPEM)
	_, certKeyPEM, err := loadOrCreateCertificateKey(nil)
	if err != nil {
		t.Fatal(err)
	}
	defer clear(certKeyPEM)
	accountEnc, err := ring.Seal(accountPEM, secretAAD(cert, "account_key"))
	if err != nil {
		t.Fatal(err)
	}
	providerKeyEnc, err := ring.Seal(certKeyPEM, taskSecretAAD(*queued, cert, "provider_cert_key"))
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Model(&cert).Update("account_key_enc", accountEnc).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Model(queued).Updates(map[string]any{
		"step": "order_created", "provider_order_uri": recoveryTestOrderURI, "provider_cert_key_enc": providerKeyEnc,
	}).Error; err != nil {
		t.Fatal(err)
	}
	worker := NewWorker(svc, "worker-a")
	for attempt := 1; attempt <= 3; attempt++ {
		claimed, err := worker.claim(context.Background())
		if err != nil || claimed == nil || claimed.ID != queued.ID {
			t.Fatalf("claim attempt %d = %#v/%v", attempt, claimed, err)
		}
		runErr := worker.runClaim(context.Background(), *claimed)
		if runErr == nil {
			t.Fatalf("runClaim attempt %d unexpectedly succeeded", attempt)
		}
		if attempt < 3 {
			if !strings.Contains(runErr.Error(), "retry scheduled") {
				t.Fatalf("runClaim attempt %d = %v", attempt, runErr)
			}
			now = now.Add(time.Duration(1<<uint(attempt-1)) * time.Minute)
		}
	}
	var failed Task
	if err := db.First(&failed, queued.ID).Error; err != nil {
		t.Fatal(err)
	}
	if failed.Status != TaskStatusFailed || failed.ProviderOrderURI != recoveryTestOrderURI || failed.ProviderCertKeyEnc == "" {
		t.Fatalf("exhausted recovery task = %#v", failed)
	}
	revived, err := svc.QueueTask(context.Background(), cert.ID, TaskKindIssue, 9)
	if err != nil {
		t.Fatal(err)
	}
	if revived.ID != queued.ID || revived.Status != TaskStatusQueued || revived.ProviderOrderURI != recoveryTestOrderURI || revived.ProviderCertKeyEnc == "" {
		t.Fatalf("revived recovery task = %#v", revived)
	}
	for call, uri := range issuer.orderURIs {
		if uri != recoveryTestOrderURI {
			t.Fatalf("issuer call %d order URI = %q", call+1, uri)
		}
	}
	if issuer.calls != 3 {
		t.Fatalf("issuer calls = %d", issuer.calls)
	}
	var count int64
	if err := db.Model(&Task{}).Where("certificate_id = ?", cert.ID).Count(&count).Error; err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("provider transient retry created %d tasks", count)
	}
}

func TestInvalidProviderOrderClearsRecoveryAndAllowsOneNewOrder(t *testing.T) {
	db := openEdgeCertTestDB(t)
	ring := testKeyring(t)
	now := time.Now().UTC()
	cert := Certificate{
		Domain: "admin.example.com", Email: "ops@example.com", Provider: "letsencrypt", Status: StatusDraft,
		DeploymentMode: DeploymentModeExternal, DeploymentProvider: "external",
		DeploymentStatus: DeploymentStatusExternal, ServingStatus: ServingStatusUnchecked,
		CreatedAt: now, UpdatedAt: now,
	}
	if err := db.Create(&cert).Error; err != nil {
		t.Fatal(err)
	}
	queued, err := NewService(db, ring, fakeIssuer{}, nil).QueueTask(context.Background(), cert.ID, TaskKindIssue, 1)
	if err != nil {
		t.Fatal(err)
	}
	_, accountPEM, err := loadOrCreateAccountKey(nil)
	if err != nil {
		t.Fatal(err)
	}
	defer clear(accountPEM)
	_, certKeyPEM, err := loadOrCreateCertificateKey(nil)
	if err != nil {
		t.Fatal(err)
	}
	defer clear(certKeyPEM)
	accountEnc, err := ring.Seal(accountPEM, secretAAD(cert, "account_key"))
	if err != nil {
		t.Fatal(err)
	}
	providerKeyEnc, err := ring.Seal(certKeyPEM, taskSecretAAD(*queued, cert, "provider_cert_key"))
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Model(&cert).Update("account_key_enc", accountEnc).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Model(queued).Updates(map[string]any{
		"step": "order_created", "provider_order_uri": recoveryTestOrderURI, "provider_cert_key_enc": providerKeyEnc,
	}).Error; err != nil {
		t.Fatal(err)
	}
	client := &recoveryTestACMEClient{
		order: &acme.Order{URI: recoveryTestOrderURI, Status: acme.StatusInvalid},
	}
	svc := NewService(db, ring, ACMEIssuer{
		ProductionDirectory: recoveryTestDirectory,
		clientFactory: func(crypto.Signer, string) acmeClient {
			return client
		},
	}, nil)
	worker := NewWorker(svc, "worker-a")
	claimed, err := worker.claim(context.Background())
	if err != nil || claimed == nil {
		t.Fatalf("claim = %#v/%v", claimed, err)
	}
	if err := worker.runClaim(context.Background(), *claimed); !errors.Is(err, ErrProviderOrderTerminal) {
		t.Fatalf("invalid order runClaim() error = %v", err)
	}
	var terminal Task
	if err := db.First(&terminal, queued.ID).Error; err != nil {
		t.Fatal(err)
	}
	if terminal.Status != TaskStatusFailed || terminal.ErrorCode != "provider_order_terminal" || terminal.ProviderOrderURI != "" || terminal.ProviderCertKeyEnc != "" {
		t.Fatalf("terminal provider state = %#v", terminal)
	}
	if client.authorizeOrderCalls != 0 {
		t.Fatalf("recovered invalid order called AuthorizeOrder %d times", client.authorizeOrderCalls)
	}

	retry, err := svc.QueueTask(context.Background(), cert.ID, TaskKindIssue, 2)
	if err != nil {
		t.Fatal(err)
	}
	if retry.ID == queued.ID {
		t.Fatalf("terminal order revived old task %#v", retry)
	}
	client.authorizeOrder = &acme.Order{
		URI: "https://acme.test/orders/43", Status: acme.StatusInvalid,
	}
	claimed, err = worker.claim(context.Background())
	if err != nil || claimed == nil || claimed.ID != retry.ID {
		t.Fatalf("new order claim = %#v/%v", claimed, err)
	}
	if err := worker.runClaim(context.Background(), *claimed); !errors.Is(err, ErrProviderOrderTerminal) {
		t.Fatalf("new terminal order runClaim() error = %v", err)
	}
	if client.authorizeOrderCalls != 1 {
		t.Fatalf("new issuance AuthorizeOrder calls = %d, want 1", client.authorizeOrderCalls)
	}
	var count int64
	if err := db.Model(&Task{}).Where("certificate_id = ?", cert.ID).Count(&count).Error; err != nil {
		t.Fatal(err)
	}
	if count != 2 {
		t.Fatalf("terminal order retry task count = %d, want 2", count)
	}
}

func TestTaskProviderRecoveryStateIsNeverSerialized(t *testing.T) {
	task := Task{
		ID: 42, CertificateID: 7, Kind: TaskKindIssue, Status: TaskStatusRunning,
		ProviderOrderURI:   "https://ca.internal.example/acme/order/private-token",
		ProviderCertKeyEnc: "v1.current.secret-envelope",
	}
	encoded, err := json.Marshal(task)
	if err != nil {
		t.Fatal(err)
	}
	for _, secret := range []string{task.ProviderOrderURI, task.ProviderCertKeyEnc, "provider_order_uri", "provider_cert_key_enc"} {
		if strings.Contains(string(encoded), secret) {
			t.Fatalf("serialized task leaked provider recovery state: %s", encoded)
		}
	}
}

func TestProviderFailureRedactsUnderlyingURI(t *testing.T) {
	const privateURI = "https://ca.internal.example/acme/order/private-token"
	err := providerFailure("order lookup", errors.New("POST "+privateURI+": bearer-token=also-private"))
	if strings.Contains(err.Error(), privateURI) || strings.Contains(err.Error(), "bearer-token") {
		t.Fatalf("provider error leaked underlying details: %q", err)
	}
	message := safeTaskMessage(err)
	if message != "certificate authority request failed" || strings.Contains(message, privateURI) {
		t.Fatalf("safeTaskMessage() = %q", message)
	}
}

func TestUpsertConfigurationDoesNotOverwriteCertificateMaterial(t *testing.T) {
	db := openEdgeCertTestDB(t)
	ring := testKeyring(t)
	now := time.Now().UTC()
	cert := Certificate{Domain: "admin.example.com", Email: "old@example.com", Provider: "letsencrypt", Status: StatusIssued,
		DeploymentMode: DeploymentModeExternal, DeploymentStatus: DeploymentStatusExternal, ServingStatus: ServingStatusUnchecked,
		FullchainPEM: "old-chain-snapshot", PrivateKeyEnc: "old-envelope-snapshot", CreatedAt: now, UpdatedAt: now}
	if err := db.Create(&cert).Error; err != nil {
		t.Fatal(err)
	}
	// Simulate the worker committing a renewal after another request took an old
	// snapshot. Upsert must touch only the configuration whitelist.
	if err := db.Model(&Certificate{}).Where("id = ?", cert.ID).Updates(map[string]any{
		"fullchain_pem": "new-chain", "private_key_enc": "new-envelope", "cert_fingerprint_sha256": strings.Repeat("a", 64),
	}).Error; err != nil {
		t.Fatal(err)
	}
	svc := NewService(db, ring, nil, nil)
	enabled := false
	if _, err := svc.UpsertDraft(context.Background(), UpsertInput{
		Domain: cert.Domain, Email: "new@example.com", DeploymentMode: DeploymentModeExternal, AutoRenewEnabled: &enabled,
	}); err != nil {
		t.Fatal(err)
	}
	var stored Certificate
	if err := db.First(&stored, cert.ID).Error; err != nil {
		t.Fatal(err)
	}
	if stored.FullchainPEM != "new-chain" || stored.PrivateKeyEnc != "new-envelope" || stored.CertFingerprintSHA256 != strings.Repeat("a", 64) {
		t.Fatalf("configuration update overwrote renewal material: %#v", stored)
	}
}

func TestCrashRecoveryResumesOnlyCommittedBoundaries(t *testing.T) {
	db := openEdgeCertTestDB(t)
	svc := NewService(db, testKeyring(t), nil, nil)
	now := time.Now().UTC()
	cert := Certificate{Domain: "admin.example.com", Email: "ops@example.com", Provider: "letsencrypt", Status: StatusIssued,
		DeploymentMode: DeploymentModeExternal, DeploymentStatus: DeploymentStatusExternal, ServingStatus: ServingStatusUnchecked,
		FullchainPEM: "old-chain", PrivateKeyEnc: "old-envelope", CreatedAt: now, UpdatedAt: now}
	if err := db.Create(&cert).Error; err != nil {
		t.Fatal(err)
	}
	worker := NewWorker(svc, "worker-a")
	for _, tc := range []struct {
		name string
		kind string
		step string
	}{
		{name: "pre-persist renewal retries issuance", kind: TaskKindRenew, step: "persist"},
		{name: "pre-commit deploy retries activation", kind: TaskKindDeploy, step: "probing"},
		{name: "standalone probe reruns", kind: TaskKindProbe, step: "probing"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			followup, err := worker.processTask(context.Background(), Task{CertificateID: cert.ID, Kind: tc.kind, Step: tc.step})
			if err == nil || followup != "" {
				t.Fatalf("processTask(%s/%s) = %q/%v, expected operation to rerun and fail its unavailable dependency", tc.kind, tc.step, followup, err)
			}
		})
	}
	if followup, err := worker.processTask(context.Background(), Task{CertificateID: cert.ID, Kind: TaskKindRenew, Step: "issued"}); err != nil || followup != TaskKindProbe {
		t.Fatalf("committed issued boundary = %q/%v", followup, err)
	}
}

func testKeyring(t *testing.T) *secretbox.Keyring {
	t.Helper()
	ring, err := secretbox.NewKeyring(secretbox.Key{ID: "current", Material: bytesOf(0x11, 32)})
	if err != nil {
		t.Fatal(err)
	}
	return ring
}

func bytesOf(fill byte, count int) []byte {
	value := make([]byte, count)
	for i := range value {
		value[i] = fill
	}
	return value
}

func ptrTime(value time.Time) *time.Time {
	value = value.UTC()
	return &value
}

func issueTestCertificate(t *testing.T, domain string, notAfter time.Time) (string, []byte, *x509.Certificate) {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().Add(-time.Hour).UTC()
	template := &x509.Certificate{
		SerialNumber: newSerial(t), Subject: pkix.Name{CommonName: domain}, DNSNames: []string{domain},
		NotBefore: now, NotAfter: notAfter.UTC(), KeyUsage: x509.KeyUsageDigitalSignature,
		ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth}, BasicConstraintsValid: true,
	}
	der, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	leaf, err := x509.ParseCertificate(der)
	if err != nil {
		t.Fatal(err)
	}
	keyDER, err := x509.MarshalECPrivateKey(key)
	if err != nil {
		t.Fatal(err)
	}
	return string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})),
		pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER}), leaf
}

func newSerial(t *testing.T) *big.Int {
	t.Helper()
	serial, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 120))
	if err != nil {
		t.Fatal(err)
	}
	return serial
}
