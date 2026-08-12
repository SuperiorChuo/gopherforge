package envsecret

import (
	"os"
	"path/filepath"
	"testing"
)

func TestGetPrefersSecretFileOverEnv(t *testing.T) {
	dir := t.TempDir()
	oldDir := SecretsDir
	SecretsDir = dir
	t.Cleanup(func() { SecretsDir = oldDir })

	t.Setenv("JWT_SECRET", "from-env-value-should-lose")
	if err := os.WriteFile(filepath.Join(dir, "jwt_secret"), []byte("  from-file-secret  \n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if got := Get("JWT_SECRET", "fallback"); got != "from-file-secret" {
		t.Fatalf("Get = %q, want from-file-secret", got)
	}
}

func TestGetDashedAndPrefixedNames(t *testing.T) {
	dir := t.TempDir()
	oldDir := SecretsDir
	SecretsDir = dir
	t.Cleanup(func() { SecretsDir = oldDir })

	if err := os.WriteFile(filepath.Join(dir, "go-admin-kit-db-password"), []byte("pg-secret"), 0o600); err != nil {
		t.Fatal(err)
	}
	if got := Get("DB_PASSWORD", ""); got != "pg-secret" {
		t.Fatalf("Get = %q, want pg-secret", got)
	}
}

func TestGetFallsBackToEnvThenDefault(t *testing.T) {
	dir := t.TempDir()
	oldDir := SecretsDir
	SecretsDir = dir
	t.Cleanup(func() { SecretsDir = oldDir })

	t.Setenv("REDIS_PASSWORD", "env-redis")
	if got := Get("REDIS_PASSWORD", "fb"); got != "env-redis" {
		t.Fatalf("env fallback = %q", got)
	}
	t.Setenv("REDIS_PASSWORD", "")
	if got := Get("REDIS_PASSWORD", "fb"); got != "fb" {
		t.Fatalf("default = %q", got)
	}
}

func TestGetRequired(t *testing.T) {
	dir := t.TempDir()
	oldDir := SecretsDir
	SecretsDir = dir
	t.Cleanup(func() { SecretsDir = oldDir })
	t.Setenv("MISSING_KEY_XYZ", "")
	if _, ok := GetRequired("MISSING_KEY_XYZ"); ok {
		t.Fatal("want missing")
	}
	t.Setenv("MISSING_KEY_XYZ", "v")
	if v, ok := GetRequired("MISSING_KEY_XYZ"); !ok || v != "v" {
		t.Fatalf("got %q ok=%v", v, ok)
	}
}
