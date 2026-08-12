package edgecert

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/uuid"
	"golang.org/x/sys/unix"
)

const activationMarkerVersion = 1

var ErrStaleActivation = errors.New("edge certificate activation ownership is stale")

type FileDeployer struct {
	CertDir           string
	DynamicConfigDir  string
	ContainerCertDir  string
	GatewayTLSAddress string
	ProbePort         string
	ProbeTimeout      time.Duration
	DialContext       func(ctx context.Context, network, address string) (net.Conn, error)
	DialTLS           func(ctx context.Context, network, address string, config *tls.Config) (*tls.Conn, error)
}

type DeploymentResult struct {
	FingerprintSHA256 string
	CertificatePath   string
	PrivateKeyPath    string
	DynamicConfigPath string
	RollbackPath      string
	ActivationToken   string
	InstalledAt       time.Time
}

type activationMarker struct {
	Version        int    `json:"version"`
	Token          string `json:"token"`
	HadPrevious    bool   `json:"had_previous"`
	PreviousConfig []byte `json:"previous_config,omitempty"`
}

type ProbeResult struct {
	FingerprintSHA256 string
	NotAfter          time.Time
	Issuer            string
	CheckedAt         time.Time
}

// Deploy 使用指纹命名不可变密钥对，最后原子切换每域名的 dynamic config。
// 任一步骤中断都不会让 Traefik 观察到半套证书。
func (d *FileDeployer) Deploy(ctx context.Context, cert Certificate, privateKey []byte) (DeploymentResult, error) {
	if d == nil || strings.TrimSpace(d.CertDir) == "" || strings.TrimSpace(d.DynamicConfigDir) == "" {
		return DeploymentResult{}, fmt.Errorf("traefik file deployment is not configured")
	}
	domain, err := canonicalDomain(cert.Domain)
	if err != nil {
		return DeploymentResult{}, err
	}
	if cert.IsStaging || cert.DeploymentMode != DeploymentModeTraefikFile {
		return DeploymentResult{}, fmt.Errorf("certificate is not eligible for traefik file deployment")
	}
	if err := ctx.Err(); err != nil {
		return DeploymentResult{}, err
	}
	pair, err := tls.X509KeyPair([]byte(cert.FullchainPEM), privateKey)
	if err != nil {
		return DeploymentResult{}, fmt.Errorf("certificate/key mismatch: %w", err)
	}
	leaf, err := x509.ParseCertificate(pair.Certificate[0])
	if err != nil {
		return DeploymentResult{}, fmt.Errorf("parse leaf certificate: %w", err)
	}
	if err := leaf.VerifyHostname(domain); err != nil {
		return DeploymentResult{}, fmt.Errorf("certificate does not cover domain: %w", err)
	}
	fingerprint := fingerprintDER(leaf.Raw)
	if cert.CertFingerprintSHA256 != "" && cert.CertFingerprintSHA256 != fingerprint {
		return DeploymentResult{}, fmt.Errorf("certificate fingerprint mismatch")
	}
	if err := os.MkdirAll(d.CertDir, 0o700); err != nil {
		return DeploymentResult{}, fmt.Errorf("create certificate directory: %w", err)
	}
	if err := os.MkdirAll(d.DynamicConfigDir, 0o700); err != nil {
		return DeploymentResult{}, fmt.Errorf("create dynamic config directory: %w", err)
	}
	base := safeDomain(domain) + "-" + fingerprint
	crtPath := filepath.Join(d.CertDir, base+".crt")
	keyPath := filepath.Join(d.CertDir, base+".key")
	configPath := filepath.Join(d.DynamicConfigDir, safeDomain(domain)+".yaml")
	rollbackPath := configPath + ".rollback"
	if err := writeAtomic(crtPath, []byte(cert.FullchainPEM), 0o600); err != nil {
		return DeploymentResult{}, fmt.Errorf("install certificate: %w", err)
	}
	if err := writeAtomic(keyPath, privateKey, 0o600); err != nil {
		return DeploymentResult{}, fmt.Errorf("install private key: %w", err)
	}
	containerDir := strings.TrimSuffix(strings.TrimSpace(d.ContainerCertDir), "/")
	if containerDir == "" {
		containerDir = filepath.ToSlash(d.CertDir)
	}
	config := []byte(fmt.Sprintf(
		"# managed by go-admin-kit edge certificate service; fingerprint=%s\n"+
			"tls:\n  certificates:\n    - certFile: %q\n      keyFile: %q\n",
		fingerprint, containerDir+"/"+filepath.Base(crtPath), containerDir+"/"+filepath.Base(keyPath),
	))
	activationToken := uuid.NewString()
	err = withActivationLock(configPath, func() error {
		// A new lease first restores any uncommitted activation left by the
		// previous owner. The lock makes recovery + replacement one ABA-safe unit.
		if err := recoverPendingActivationLocked(configPath, rollbackPath); err != nil {
			return fmt.Errorf("recover previous activation: %w", err)
		}
		previousConfig, previousErr := os.ReadFile(configPath)
		hadPrevious := previousErr == nil
		if previousErr != nil && !os.IsNotExist(previousErr) {
			return fmt.Errorf("read previous dynamic config: %w", previousErr)
		}
		markerBytes, err := json.Marshal(activationMarker{
			Version: activationMarkerVersion, Token: activationToken,
			HadPrevious: hadPrevious, PreviousConfig: previousConfig,
		})
		if err != nil {
			return fmt.Errorf("encode activation rollback marker: %w", err)
		}
		if err := writeAtomic(rollbackPath, markerBytes, 0o600); err != nil {
			return fmt.Errorf("prepare activation rollback: %w", err)
		}
		if err := writeAtomic(configPath, config, 0o600); err != nil {
			recoverErr := recoverPendingActivationLocked(configPath, rollbackPath)
			return errors.Join(fmt.Errorf("activate traefik dynamic config: %w", err), recoverErr)
		}
		return nil
	})
	if err != nil {
		return DeploymentResult{}, err
	}
	return DeploymentResult{
		FingerprintSHA256: fingerprint, CertificatePath: crtPath, PrivateKeyPath: keyPath,
		DynamicConfigPath: configPath, RollbackPath: rollbackPath,
		ActivationToken: activationToken, InstalledAt: time.Now().UTC(),
	}, nil
}

// Rollback restores the exact dynamic config that existed before Deploy. The
// immutable fingerprint files may remain unreferenced and are safe to GC later.
func (d *FileDeployer) Rollback(result DeploymentResult) error {
	if err := validateActivationResult(result); err != nil {
		return fmt.Errorf("deployment rollback token is invalid")
	}
	return withActivationLock(result.DynamicConfigPath, func() error {
		marker, err := ownedActivationMarker(result.RollbackPath, result.ActivationToken)
		if err != nil {
			return err
		}
		return restoreActivationLocked(result.DynamicConfigPath, result.RollbackPath, marker)
	})
}

func (d *FileDeployer) Commit(result DeploymentResult) error {
	if err := validateActivationResult(result); err != nil {
		return fmt.Errorf("deployment commit token is invalid")
	}
	return withActivationLock(result.DynamicConfigPath, func() error {
		if _, err := ownedActivationMarker(result.RollbackPath, result.ActivationToken); err != nil {
			return err
		}
		if err := os.Remove(result.RollbackPath); err != nil {
			return err
		}
		return syncDirectory(filepath.Dir(result.RollbackPath))
	})
}

func validateActivationResult(result DeploymentResult) error {
	if strings.TrimSpace(result.DynamicConfigPath) == "" ||
		result.RollbackPath != result.DynamicConfigPath+".rollback" ||
		uuid.Validate(result.ActivationToken) != nil {
		return fmt.Errorf("invalid activation result")
	}
	return nil
}

func readActivationMarker(rollbackPath string) (activationMarker, error) {
	markerBytes, err := os.ReadFile(rollbackPath)
	if err != nil {
		return activationMarker{}, err
	}
	var marker activationMarker
	if err := json.Unmarshal(markerBytes, &marker); err != nil {
		return activationMarker{}, fmt.Errorf("invalid activation rollback marker: %w", err)
	}
	if marker.Version != activationMarkerVersion || uuid.Validate(marker.Token) != nil ||
		(!marker.HadPrevious && len(marker.PreviousConfig) != 0) {
		return activationMarker{}, fmt.Errorf("invalid activation rollback marker")
	}
	return marker, nil

}

func ownedActivationMarker(rollbackPath, activationToken string) (activationMarker, error) {
	marker, err := readActivationMarker(rollbackPath)
	if os.IsNotExist(err) {
		return activationMarker{}, ErrStaleActivation
	}
	if err != nil {
		return activationMarker{}, err
	}
	if marker.Token != activationToken {
		return activationMarker{}, fmt.Errorf("%w: activation token no longer owns rollback marker", ErrStaleActivation)
	}
	return marker, nil
}

func recoverPendingActivationLocked(configPath, rollbackPath string) error {
	marker, err := readActivationMarker(rollbackPath)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	return restoreActivationLocked(configPath, rollbackPath, marker)
}

func restoreActivationLocked(configPath, rollbackPath string, marker activationMarker) error {
	if marker.HadPrevious {
		if err := writeAtomic(configPath, marker.PreviousConfig, 0o600); err != nil {
			return err
		}
	} else if err := os.Remove(configPath); err != nil && !os.IsNotExist(err) {
		return err
	}
	if err := os.Remove(rollbackPath); err != nil && !os.IsNotExist(err) {
		return err
	}
	return syncDirectory(filepath.Dir(configPath))
}

func withActivationLock(configPath string, fn func() error) error {
	lockPath := configPath + ".lock"
	lockFile, err := os.OpenFile(lockPath, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return err
	}
	defer lockFile.Close()
	if err := lockFile.Chmod(0o600); err != nil {
		return fmt.Errorf("secure certificate activation lock: %w", err)
	}
	if err := unix.Flock(int(lockFile.Fd()), unix.LOCK_EX); err != nil {
		return fmt.Errorf("lock certificate activation: %w", err)
	}
	defer func() { _ = unix.Flock(int(lockFile.Fd()), unix.LOCK_UN) }()
	return fn()

}

func syncDirectory(dir string) error {
	directory, err := os.Open(dir)
	if err != nil {
		return err
	}
	defer directory.Close()
	return directory.Sync()
}

// Probe 连接实际 TLS 入口并按 SNI 校验，installed 与 serving 状态由调用方分别保存。
func (d *FileDeployer) Probe(ctx context.Context, domain string) (ProbeResult, error) {
	domain, err := canonicalDomain(domain)
	if err != nil {
		return ProbeResult{}, err
	}
	port := "443"
	timeout := 10 * time.Second
	if d != nil {
		if strings.TrimSpace(d.ProbePort) != "" {
			port = strings.TrimSpace(d.ProbePort)
		}
		if d.ProbeTimeout > 0 {
			timeout = d.ProbeTimeout
		}
	}
	probeCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	config := &tls.Config{ServerName: domain, MinVersion: tls.VersionTLS12}
	address := net.JoinHostPort(domain, port)
	if d != nil && strings.TrimSpace(d.GatewayTLSAddress) != "" {
		address = strings.TrimSpace(d.GatewayTLSAddress)
	}
	var conn *tls.Conn
	if d != nil && d.DialTLS != nil {
		conn, err = d.DialTLS(probeCtx, "tcp", address, config)
	} else {
		if d != nil && d.DialContext != nil {
			var raw net.Conn
			raw, err = d.DialContext(probeCtx, "tcp", address)
			if err == nil {
				conn = tls.Client(raw, config)
			}
		} else {
			dialer := &tls.Dialer{NetDialer: &net.Dialer{Timeout: timeout}, Config: config}
			var raw net.Conn
			raw, err = dialer.DialContext(probeCtx, "tcp", address)
			if err == nil {
				conn, _ = raw.(*tls.Conn)
			}
		}
	}
	if err != nil {
		return ProbeResult{}, fmt.Errorf("tls probe connect: %w", err)
	}
	if conn == nil {
		return ProbeResult{}, fmt.Errorf("tls probe did not return a TLS connection")
	}
	defer conn.Close()
	if err := conn.HandshakeContext(probeCtx); err != nil {
		return ProbeResult{}, fmt.Errorf("tls probe handshake: %w", err)
	}
	state := conn.ConnectionState()
	if len(state.PeerCertificates) == 0 {
		return ProbeResult{}, fmt.Errorf("tls probe returned no peer certificate")
	}
	leaf := state.PeerCertificates[0]
	return ProbeResult{
		FingerprintSHA256: fingerprintDER(leaf.Raw), NotAfter: leaf.NotAfter.UTC(),
		Issuer: leaf.Issuer.String(), CheckedAt: time.Now().UTC(),
	}, nil
}

// ProbeCertificate keeps external/Caddy observation on the public domain while
// managed mode probes the configured gateway address with the certificate SNI.
func (d *FileDeployer) ProbeCertificate(ctx context.Context, cert Certificate) (ProbeResult, error) {
	if d == nil {
		return (&FileDeployer{}).Probe(ctx, cert.Domain)
	}
	if cert.DeploymentMode == DeploymentModeExternal {
		publicProbe := *d
		publicProbe.GatewayTLSAddress = ""
		return publicProbe.Probe(ctx, cert.Domain)
	}
	return d.Probe(ctx, cert.Domain)
}

func writeAtomic(target string, data []byte, mode os.FileMode) (err error) {
	dir := filepath.Dir(target)
	tmp, err := os.CreateTemp(dir, ".edgecert-*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer func() {
		_ = tmp.Close()
		if err != nil {
			_ = os.Remove(tmpName)
		}
	}()
	if err = tmp.Chmod(mode); err != nil {
		return err
	}
	if _, err = tmp.Write(data); err != nil {
		return err
	}
	if err = tmp.Sync(); err != nil {
		return err
	}
	if err = tmp.Close(); err != nil {
		return err
	}
	if err = os.Rename(tmpName, target); err != nil {
		return err
	}
	directory, err := os.Open(dir)
	if err != nil {
		return err
	}
	defer directory.Close()
	return directory.Sync()
}

func safeDomain(domain string) string {
	return strings.ReplaceAll(domain, ".", "_")
}

func certificateFingerprint(fullchain string) (string, error) {
	block, _ := pem.Decode([]byte(fullchain))
	if block == nil || block.Type != "CERTIFICATE" {
		return "", fmt.Errorf("invalid certificate PEM")
	}
	return fingerprintDER(block.Bytes), nil
}
