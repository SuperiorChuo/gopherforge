package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCodegenWriteCannotEnableOutsideDevelopment(t *testing.T) {
	cfg := Defaults()
	cfg.App.Env = "production"
	cfg.JWT.Secret = strings.Repeat("s", 40)
	cfg.Codegen.WriteEnabled = true
	cfg.Codegen.RepoRoot = newCodegenRepoRoot(t)
	if err := validate(cfg); err == nil || !strings.Contains(err.Error(), "development") {
		t.Fatalf("error = %v", err)
	}
}

func TestCodegenWriteRequiresValidRepositoryRoot(t *testing.T) {
	for name, root := range map[string]string{
		"空路径":  "",
		"普通目录": t.TempDir(),
	} {
		t.Run(name, func(t *testing.T) {
			cfg := Defaults()
			cfg.Codegen.WriteEnabled = true
			cfg.Codegen.RepoRoot = root
			if err := validate(cfg); err == nil || !strings.Contains(err.Error(), "CODEGEN_REPO_ROOT") {
				t.Fatalf("error = %v", err)
			}
		})
	}
}

func TestCodegenConfigLoadsDisabledByDefaultAndExplicitDevelopmentRoot(t *testing.T) {
	defaults := Defaults()
	if defaults.Codegen.WriteEnabled || defaults.Codegen.RepoRoot != "" {
		t.Fatalf("defaults = %#v", defaults.Codegen)
	}

	root := newCodegenRepoRoot(t)
	t.Setenv("APP_ENV", "development")
	t.Setenv("CODEGEN_WRITE_ENABLED", "true")
	t.Setenv("CODEGEN_REPO_ROOT", root)
	previous := Cfg
	t.Cleanup(func() { Cfg = previous })
	if err := Load(); err != nil {
		t.Fatalf("Load: %v", err)
	}
	if !Cfg.Codegen.WriteEnabled || Cfg.Codegen.RepoRoot != root {
		t.Fatalf("codegen config = %#v", Cfg.Codegen)
	}
}

func newCodegenRepoRoot(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	for _, path := range []string{
		".git",
		"microservices/services/system",
		"microservices/web",
	} {
		if err := os.MkdirAll(filepath.Join(root, path), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	return root
}
