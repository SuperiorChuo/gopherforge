package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/go-admin-kit/services/shared/pkg/envsecret"
)

// TestApplyEnvPrefersSwarmSecret 验证 JWT/DB/REDIS 敏感项优先读 /run/secrets 风格文件。
func TestApplyEnvPrefersSwarmSecret(t *testing.T) {
	dir := t.TempDir()
	old := envsecret.SecretsDir
	envsecret.SecretsDir = dir
	t.Cleanup(func() { envsecret.SecretsDir = old })

	// env 有弱值；secret 文件应胜出
	t.Setenv("APP_ENV", "development")
	t.Setenv("JWT_SECRET", "from-env-should-lose-but-long-enough-xxx")
	t.Setenv("DB_PASSWORD", "from-env-db")
	t.Setenv("REDIS_PASSWORD", "from-env-redis")

	if err := os.WriteFile(filepath.Join(dir, "jwt_secret"), []byte("from-file-jwt-secret-value-32chars!!\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "db_password"), []byte("from-file-db\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "redis_password"), []byte("from-file-redis\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	cfg := Defaults()
	applyEnv(&cfg)
	if cfg.JWT.Secret != "from-file-jwt-secret-value-32chars!!" {
		t.Fatalf("JWT.Secret = %q, want file value", cfg.JWT.Secret)
	}
	if cfg.Database.Password != "from-file-db" {
		t.Fatalf("Database.Password = %q, want from-file-db", cfg.Database.Password)
	}
	if cfg.Redis.Password != "from-file-redis" {
		t.Fatalf("Redis.Password = %q, want from-file-redis", cfg.Redis.Password)
	}
}
