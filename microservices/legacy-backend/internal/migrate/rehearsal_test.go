package migrate

import (
	"strings"
	"testing"

	"github.com/go-admin-kit/server/internal/config"
)

func TestNormalizeRehearsalOptionsDefaults(t *testing.T) {
	opts := normalizeRehearsalOptions(RehearsalOptions{})

	if opts.ConfigPath != DefaultConfigPath {
		t.Fatalf("ConfigPath = %q, want %q", opts.ConfigPath, DefaultConfigPath)
	}
	if opts.Dir != DefaultDir {
		t.Fatalf("Dir = %q, want %q", opts.Dir, DefaultDir)
	}
	if opts.Database != DefaultRehearsalDatabase {
		t.Fatalf("Database = %q, want %q", opts.Database, DefaultRehearsalDatabase)
	}
}

func TestValidateRehearsalDatabaseName(t *testing.T) {
	valid := []string{
		"go_admin_kit_migration_rehearsal",
		"go_admin_kit_migration_rehearsal_20260522",
	}
	for _, name := range valid {
		if err := validateRehearsalDatabaseName(name); err != nil {
			t.Fatalf("validateRehearsalDatabaseName(%q) error = %v", name, err)
		}
	}

	invalid := []string{
		"",
		"go-admin-kit",
		"go_admin;DROP DATABASE production",
		"../go_admin_kit",
		"postgres",
		"template0",
		"template1",
		"information_schema",
	}
	for _, name := range invalid {
		if err := validateRehearsalDatabaseName(name); err == nil {
			t.Fatalf("validateRehearsalDatabaseName(%q) error = nil, want error", name)
		}
	}
}

func TestMigrationServerDSNUsesMaintenanceDatabase(t *testing.T) {
	dsn := migrationServerDSN(config.DatabaseConfig{
		Driver:   "postgres",
		Host:     "127.0.0.1",
		Port:     5432,
		User:     "postgres",
		Password: "123456",
		DBName:   "go_admin_kit",
		SSLMode:  "disable",
	})

	if strings.Contains(dsn, "dbname=go_admin_kit") {
		t.Fatalf("server DSN = %q, must omit configured database name", dsn)
	}
	if !strings.Contains(dsn, "dbname=postgres") {
		t.Fatalf("server DSN = %q, want maintenance database postgres", dsn)
	}
}
