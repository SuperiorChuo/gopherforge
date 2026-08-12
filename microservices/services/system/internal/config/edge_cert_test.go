package config

import (
	"encoding/base64"
	"strings"
	"testing"
)

func TestValidateEdgeCertConfigRejectsPartialPreviousKey(t *testing.T) {
	cfg := Defaults()
	cfg.EdgeCert.PreviousKeyID = "old"

	err := validate(cfg)
	if err == nil || !strings.Contains(err.Error(), "EDGE_CERT_PREVIOUS") {
		t.Fatalf("validate() error = %v, want partial previous key rejection", err)
	}
}

func TestEdgeCertKeyMaterialsDecodeBase64(t *testing.T) {
	raw := []byte("0123456789abcdef0123456789abcdef")
	cfg := Defaults().EdgeCert
	cfg.CurrentKeyID = "v2"
	cfg.CurrentKeyBase64 = base64.StdEncoding.EncodeToString(raw)

	id, got, _, _, err := cfg.KeyMaterials()
	if err != nil {
		t.Fatalf("KeyMaterials() error = %v", err)
	}
	if id != "v2" || string(got) != string(raw) {
		t.Fatalf("KeyMaterials() = %q/%q", id, got)
	}
}

func TestEdgeCertKeyMaterialsRejectWrongLength(t *testing.T) {
	cfg := Defaults()
	cfg.EdgeCert.CurrentKeyID = "v1"
	cfg.EdgeCert.CurrentKeyBase64 = base64.StdEncoding.EncodeToString([]byte("too-short"))

	err := validate(cfg)
	if err == nil || !strings.Contains(err.Error(), "exactly 32 bytes") {
		t.Fatalf("validate() error = %v, want key length rejection", err)
	}
}

func TestValidateEdgeCertConfigRejectsDynamicDirWithoutStorageRoot(t *testing.T) {
	cfg := Defaults()
	cfg.EdgeCert.TraefikDynamicDir = "/etc/traefik/dynamic"

	err := validate(cfg)
	if err == nil || !strings.Contains(err.Error(), "EDGE_CERT_STORAGE_ROOT") {
		t.Fatalf("validate() error = %v, want storage root rejection", err)
	}
}

func TestValidateProductionEdgeCertKeyPair(t *testing.T) {
	cfg := prodConfig()
	cfg.EdgeCert.CurrentKeyID = "v1"

	err := validate(cfg)
	if err == nil || !strings.Contains(err.Error(), "EDGE_CERT_ENCRYPTION_KEY") {
		t.Fatalf("validate() error = %v, want missing current key rejection", err)
	}
}

func TestEdgeCertDefaultsAreSafeExternalMode(t *testing.T) {
	cfg := Defaults().EdgeCert
	if cfg.StorageRoot != "" || cfg.TraefikDynamicDir != "" {
		t.Fatalf("edge certificate deploy defaults must be disabled: %+v", cfg)
	}
	if !cfg.WorkerEnabled {
		t.Fatal("edge certificate worker should process explicitly queued external probe tasks")
	}
	if cfg.ClearLegacySecrets {
		t.Fatal("legacy cleanup must require an explicit second-stage rollout")
	}
}
