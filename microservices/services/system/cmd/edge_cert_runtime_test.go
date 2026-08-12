package main

import (
	"encoding/base64"
	"testing"

	"github.com/go-admin-kit/services/system/internal/config"
)

func TestBuildEdgeCertificateRuntimeFailsClosedAndWiresCapabilities(t *testing.T) {
	cfg := config.Defaults().EdgeCert
	cfg.CurrentKeyID = "v2"
	cfg.CurrentKeyBase64 = base64.StdEncoding.EncodeToString([]byte("0123456789abcdef0123456789abcdef"))
	cfg.StorageRoot = "/var/lib/go-admin-kit/edgecert/edge"
	cfg.TraefikDynamicDir = "/var/lib/go-admin-kit/edgecert/traefik-dynamic"
	cfg.WorkerEnabled = false
	cfg.ClearLegacySecrets = true

	service, err := buildEdgeCertificateRuntime(cfg, nil)
	if err != nil {
		t.Fatalf("buildEdgeCertificateRuntime() error = %v", err)
	}
	if service.Keyring == nil || service.Deployer == nil {
		t.Fatal("configured keyring/deployer were not wired")
	}
	if service.Capabilities().AsyncTasks {
		t.Fatal("disabled worker must make async capabilities fail closed")
	}
	if service.Deployer.GatewayTLSAddress != cfg.GatewayTLSAddress {
		t.Fatalf("gateway probe address = %q, want %q", service.Deployer.GatewayTLSAddress, cfg.GatewayTLSAddress)
	}
	if !service.ClearLegacySecrets {
		t.Fatal("explicit legacy cleanup flag was not wired")
	}
}

func TestBuildEdgeCertificateRuntimeKeepsReadOnlyExternalMode(t *testing.T) {
	service, err := buildEdgeCertificateRuntime(config.Defaults().EdgeCert, nil)
	if err != nil {
		t.Fatalf("buildEdgeCertificateRuntime() error = %v", err)
	}
	if service.Keyring != nil || service.Deployer != nil {
		t.Fatal("empty configuration must not invent a keyring or deployment target")
	}
}
