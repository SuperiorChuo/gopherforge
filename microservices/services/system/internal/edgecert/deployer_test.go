package edgecert

import (
	"context"
	"crypto/tls"
	"errors"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestFileDeployerInstallsImmutablePairThenDynamicConfig(t *testing.T) {
	root := t.TempDir()
	certDir := filepath.Join(root, "certificates")
	dynamicDir := filepath.Join(root, "dynamic")
	chain, key, leaf := issueTestCertificate(t, "admin.example.com", time.Now().Add(90*24*time.Hour))
	cert := Certificate{
		Domain: "admin.example.com", IsStaging: false, DeploymentMode: DeploymentModeTraefikFile,
		FullchainPEM: chain, CertFingerprintSHA256: fingerprintDER(leaf.Raw),
	}
	deployer := &FileDeployer{CertDir: certDir, DynamicConfigDir: dynamicDir, ContainerCertDir: "/edge-certs"}
	result, err := deployer.Deploy(context.Background(), cert, key)
	if err != nil {
		t.Fatalf("Deploy() error = %v", err)
	}
	for _, path := range []string{result.CertificatePath, result.PrivateKeyPath, result.DynamicConfigPath} {
		info, statErr := os.Stat(path)
		if statErr != nil {
			t.Fatalf("stat %s: %v", path, statErr)
		}
		if info.Mode().Perm() != 0o600 {
			t.Fatalf("mode %s = %o, want 600", path, info.Mode().Perm())
		}
	}
	config, err := os.ReadFile(result.DynamicConfigPath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(config), result.FingerprintSHA256) ||
		!strings.Contains(string(config), "/edge-certs/"+filepath.Base(result.CertificatePath)) ||
		!strings.Contains(string(config), "/edge-certs/"+filepath.Base(result.PrivateKeyPath)) {
		t.Fatalf("dynamic config does not bind the immutable pair: %s", config)
	}
	marker, err := readActivationMarker(result.RollbackPath)
	if err != nil || marker.Version != activationMarkerVersion || marker.Token != result.ActivationToken {
		t.Fatalf("versioned activation marker = %#v/%v", marker, err)
	}
	if err := deployer.Commit(result); err != nil {
		t.Fatalf("Commit() error = %v", err)
	}
	if _, err := os.Stat(result.RollbackPath); !os.IsNotExist(err) {
		t.Fatalf("rollback marker remains after commit: %v", err)
	}
	entries, err := filepath.Glob(filepath.Join(root, "**", ".edgecert-*"))
	if err != nil || len(entries) != 0 {
		t.Fatalf("temporary files left behind: %v/%v", entries, err)
	}
}

func TestFileDeployerRejectsMismatchBeforeActivation(t *testing.T) {
	root := t.TempDir()
	chain, _, leaf := issueTestCertificate(t, "admin.example.com", time.Now().Add(90*24*time.Hour))
	_, wrongKey, _ := issueTestCertificate(t, "admin.example.com", time.Now().Add(90*24*time.Hour))
	deployer := &FileDeployer{CertDir: filepath.Join(root, "certs"), DynamicConfigDir: filepath.Join(root, "dynamic")}
	_, err := deployer.Deploy(context.Background(), Certificate{
		Domain: "admin.example.com", DeploymentMode: DeploymentModeTraefikFile,
		FullchainPEM: chain, CertFingerprintSHA256: fingerprintDER(leaf.Raw),
	}, wrongKey)
	if err == nil {
		t.Fatal("Deploy() accepted a mismatched private key")
	}
	if _, statErr := os.Stat(filepath.Join(root, "dynamic", "admin_example_com.yaml")); !os.IsNotExist(statErr) {
		t.Fatalf("dynamic config activated after mismatch: %v", statErr)
	}
}

func TestFileDeployerRollbackRestoresPreviousDynamicConfig(t *testing.T) {
	root := t.TempDir()
	dynamicDir := filepath.Join(root, "dynamic")
	if err := os.MkdirAll(dynamicDir, 0o700); err != nil {
		t.Fatal(err)
	}
	configPath := filepath.Join(dynamicDir, "admin_example_com.yaml")
	previous := []byte("tls:\n  certificates: [] # previous\n")
	if err := os.WriteFile(configPath, previous, 0o600); err != nil {
		t.Fatal(err)
	}
	chain, key, leaf := issueTestCertificate(t, "admin.example.com", time.Now().Add(90*24*time.Hour))
	deployer := &FileDeployer{CertDir: filepath.Join(root, "certs"), DynamicConfigDir: dynamicDir}
	result, err := deployer.Deploy(context.Background(), Certificate{
		Domain: "admin.example.com", DeploymentMode: DeploymentModeTraefikFile,
		FullchainPEM: chain, CertFingerprintSHA256: fingerprintDER(leaf.Raw),
	}, key)
	if err != nil {
		t.Fatal(err)
	}
	if err := deployer.Rollback(result); err != nil {
		t.Fatal(err)
	}
	restored, err := os.ReadFile(configPath)
	if err != nil || string(restored) != string(previous) {
		t.Fatalf("restored config = %q/%v", restored, err)
	}
}

func TestFileDeployerRecoversInterruptedActivationBeforeNextDeploy(t *testing.T) {
	root := t.TempDir()
	dynamicDir := filepath.Join(root, "dynamic")
	certDir := filepath.Join(root, "certs")
	if err := os.MkdirAll(dynamicDir, 0o700); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dynamicDir, "admin_example_com.yaml")
	previous := []byte("tls:\n  certificates: [] # stable\n")
	if err := os.WriteFile(path, previous, 0o600); err != nil {
		t.Fatal(err)
	}
	deployer := &FileDeployer{CertDir: certDir, DynamicConfigDir: dynamicDir}
	chain1, key1, leaf1 := issueTestCertificate(t, "admin.example.com", time.Now().Add(60*24*time.Hour))
	first, err := deployer.Deploy(context.Background(), Certificate{Domain: "admin.example.com", DeploymentMode: DeploymentModeTraefikFile, FullchainPEM: chain1, CertFingerprintSHA256: fingerprintDER(leaf1.Raw)}, key1)
	if err != nil {
		t.Fatal(err)
	}
	// Simulate process death before probe/commit: rollback marker remains.
	if _, err := os.Stat(first.RollbackPath); err != nil {
		t.Fatal(err)
	}
	chain2, key2, leaf2 := issueTestCertificate(t, "admin.example.com", time.Now().Add(90*24*time.Hour))
	second, err := deployer.Deploy(context.Background(), Certificate{Domain: "admin.example.com", DeploymentMode: DeploymentModeTraefikFile, FullchainPEM: chain2, CertFingerprintSHA256: fingerprintDER(leaf2.Raw)}, key2)
	if err != nil {
		t.Fatal(err)
	}
	// The new rollback point must be the last committed stable config, not the
	// unverified first activation.
	if err := deployer.Rollback(second); err != nil {
		t.Fatal(err)
	}
	restored, err := os.ReadFile(path)
	if err != nil || string(restored) != string(previous) {
		t.Fatalf("interrupted activation recovery = %q/%v", restored, err)
	}
}

func TestStaleActivationCannotRollbackOrCommitSuccessor(t *testing.T) {
	root := t.TempDir()
	dynamicDir := filepath.Join(root, "dynamic")
	if err := os.MkdirAll(dynamicDir, 0o700); err != nil {
		t.Fatal(err)
	}
	configPath := filepath.Join(dynamicDir, "admin_example_com.yaml")
	stable := []byte("tls:\n  certificates: [] # stable\n")
	if err := os.WriteFile(configPath, stable, 0o600); err != nil {
		t.Fatal(err)
	}
	deployer := &FileDeployer{CertDir: filepath.Join(root, "certs"), DynamicConfigDir: dynamicDir}
	chainA, keyA, leafA := issueTestCertificate(t, "admin.example.com", time.Now().Add(60*24*time.Hour))
	activationA, err := deployer.Deploy(context.Background(), Certificate{
		Domain: "admin.example.com", DeploymentMode: DeploymentModeTraefikFile,
		FullchainPEM: chainA, CertFingerprintSHA256: fingerprintDER(leafA.Raw),
	}, keyA)
	if err != nil {
		t.Fatal(err)
	}
	chainB, keyB, leafB := issueTestCertificate(t, "admin.example.com", time.Now().Add(90*24*time.Hour))
	activationB, err := deployer.Deploy(context.Background(), Certificate{
		Domain: "admin.example.com", DeploymentMode: DeploymentModeTraefikFile,
		FullchainPEM: chainB, CertFingerprintSHA256: fingerprintDER(leafB.Raw),
	}, keyB)
	if err != nil {
		t.Fatal(err)
	}
	if activationA.ActivationToken == activationB.ActivationToken {
		t.Fatal("successive deployments reused an activation token")
	}
	wantConfig, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatal(err)
	}
	wantMarker, err := os.ReadFile(activationB.RollbackPath)
	if err != nil {
		t.Fatal(err)
	}

	if err := deployer.Rollback(activationA); !errors.Is(err, ErrStaleActivation) {
		t.Fatalf("stale Rollback() error = %v, want ErrStaleActivation", err)
	}
	if err := deployer.Commit(activationA); !errors.Is(err, ErrStaleActivation) {
		t.Fatalf("stale Commit() error = %v, want ErrStaleActivation", err)
	}
	gotConfig, err := os.ReadFile(configPath)
	if err != nil || string(gotConfig) != string(wantConfig) {
		t.Fatalf("stale owner changed successor config: %q/%v", gotConfig, err)
	}
	gotMarker, err := os.ReadFile(activationB.RollbackPath)
	if err != nil || string(gotMarker) != string(wantMarker) {
		t.Fatalf("stale owner changed successor marker: %q/%v", gotMarker, err)
	}
	if err := deployer.Rollback(activationB); err != nil {
		t.Fatal(err)
	}
	restored, err := os.ReadFile(configPath)
	if err != nil || string(restored) != string(stable) {
		t.Fatalf("successor rollback = %q/%v", restored, err)
	}
}

func TestActivationLockSerializesStaleRollbackAndSuccessorCommit(t *testing.T) {
	root := t.TempDir()
	dynamicDir := filepath.Join(root, "dynamic")
	deployer := &FileDeployer{CertDir: filepath.Join(root, "certs"), DynamicConfigDir: dynamicDir}
	chainA, keyA, leafA := issueTestCertificate(t, "admin.example.com", time.Now().Add(60*24*time.Hour))
	activationA, err := deployer.Deploy(context.Background(), Certificate{
		Domain: "admin.example.com", DeploymentMode: DeploymentModeTraefikFile,
		FullchainPEM: chainA, CertFingerprintSHA256: fingerprintDER(leafA.Raw),
	}, keyA)
	if err != nil {
		t.Fatal(err)
	}
	chainB, keyB, leafB := issueTestCertificate(t, "admin.example.com", time.Now().Add(90*24*time.Hour))
	activationB, err := deployer.Deploy(context.Background(), Certificate{
		Domain: "admin.example.com", DeploymentMode: DeploymentModeTraefikFile,
		FullchainPEM: chainB, CertFingerprintSHA256: fingerprintDER(leafB.Raw),
	}, keyB)
	if err != nil {
		t.Fatal(err)
	}
	wantConfig, err := os.ReadFile(activationB.DynamicConfigPath)
	if err != nil {
		t.Fatal(err)
	}
	start := make(chan struct{})
	staleDone := make(chan error, 1)
	commitDone := make(chan error, 1)
	go func() {
		<-start
		staleDone <- deployer.Rollback(activationA)
	}()
	go func() {
		<-start
		commitDone <- deployer.Commit(activationB)
	}()
	close(start)
	if err := <-staleDone; !errors.Is(err, ErrStaleActivation) {
		t.Fatalf("concurrent stale rollback error = %v", err)
	}
	if err := <-commitDone; err != nil {
		t.Fatalf("successor Commit() error = %v", err)
	}
	gotConfig, err := os.ReadFile(activationB.DynamicConfigPath)
	if err != nil || string(gotConfig) != string(wantConfig) {
		t.Fatalf("concurrent stale rollback changed committed config: %q/%v", gotConfig, err)
	}
}

func TestProbeReportsServingFingerprint(t *testing.T) {
	chain, key, leaf := issueTestCertificate(t, "admin.example.com", time.Now().Add(90*24*time.Hour))
	serverPair, err := tls.X509KeyPair([]byte(chain), key)
	if err != nil {
		t.Fatal(err)
	}
	deployer := &FileDeployer{DialTLS: func(ctx context.Context, _, _ string, config *tls.Config) (*tls.Conn, error) {
		serverRaw, clientRaw := net.Pipe()
		server := tls.Server(serverRaw, &tls.Config{Certificates: []tls.Certificate{serverPair}, MinVersion: tls.VersionTLS12})
		go func() {
			_ = server.HandshakeContext(ctx)
			_ = server.Close()
		}()
		clientConfig := config.Clone()
		clientConfig.InsecureSkipVerify = true // local self-signed test only
		return tls.Client(clientRaw, clientConfig), nil
	}}
	result, err := deployer.Probe(context.Background(), "admin.example.com")
	if err != nil {
		t.Fatalf("Probe() error = %v", err)
	}
	if result.FingerprintSHA256 != fingerprintDER(leaf.Raw) || !result.NotAfter.Equal(leaf.NotAfter.UTC()) {
		t.Fatalf("Probe() = %#v", result)
	}
}

func TestProbeCertificateAddressSelection(t *testing.T) {
	var addresses []string
	deployer := &FileDeployer{
		GatewayTLSAddress: "go-admin-kit-gateway:443",
		DialContext: func(_ context.Context, _, address string) (net.Conn, error) {
			addresses = append(addresses, address)
			return nil, errors.New("capture only")
		},
	}
	_, _ = deployer.ProbeCertificate(context.Background(), Certificate{Domain: "admin.example.com", DeploymentMode: DeploymentModeExternal})
	_, _ = deployer.ProbeCertificate(context.Background(), Certificate{Domain: "admin.example.com", DeploymentMode: DeploymentModeTraefikFile})
	if len(addresses) != 2 || addresses[0] != "admin.example.com:443" || addresses[1] != "go-admin-kit-gateway:443" {
		t.Fatalf("probe addresses = %#v", addresses)
	}
}
