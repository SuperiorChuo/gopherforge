package edgecert

import (
	"os"
	"strings"
	"testing"
)

func TestEdgeCertLifecycleMigrationPersistsAndGuardsProviderRecoveryState(t *testing.T) {
	content, err := os.ReadFile("../../../monitor/migrations/000075_edge_tls_certificate_lifecycle.sql")
	if err != nil {
		t.Fatal(err)
	}
	sql := strings.ToLower(string(content))
	for _, contract := range []string{
		"provider_order_uri text",
		"provider_cert_key_enc text",
		"add column if not exists provider_order_uri text",
		"add column if not exists provider_cert_key_enc text",
		"coalesce(provider_order_uri, '') <> ''",
		"coalesce(provider_cert_key_enc, '') <> ''",
	} {
		if !strings.Contains(sql, contract) {
			t.Fatalf("edge certificate migration is missing %q", contract)
		}
	}
}
