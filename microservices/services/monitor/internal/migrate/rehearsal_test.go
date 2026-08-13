package migrate

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/go-admin-kit/services/monitor/internal/config"
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

func TestPLpgSQLBlocksUseGooseStatementMarkers(t *testing.T) {
	paths, err := filepath.Glob("../../migrations/*.sql")
	if err != nil {
		t.Fatalf("glob migrations: %v", err)
	}
	for _, path := range paths {
		content, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read %s: %v", path, err)
		}
		insideStatement := false
		for i, line := range strings.Split(string(content), "\n") {
			line = strings.TrimSpace(line)
			switch line {
			case "-- +goose StatementBegin":
				if insideStatement {
					t.Fatalf("%s:%d nested StatementBegin", path, i+1)
				}
				insideStatement = true
			case "-- +goose StatementEnd":
				if !insideStatement {
					t.Fatalf("%s:%d StatementEnd without StatementBegin", path, i+1)
				}
				insideStatement = false
			default:
				isPLpgSQL := strings.HasPrefix(line, "DO $$") ||
					strings.HasPrefix(line, "CREATE FUNCTION ") ||
					strings.HasPrefix(line, "CREATE OR REPLACE FUNCTION ")
				if isPLpgSQL && !insideStatement {
					t.Fatalf("%s:%d PL/pgSQL block is not wrapped in goose statement markers", path, i+1)
				}
			}
		}
		if insideStatement {
			t.Fatalf("%s has an unclosed StatementBegin", path)
		}
	}
}

func TestRehearsalMigrationStepsRollBackLatestOnly(t *testing.T) {
	steps := rehearsalMigrationSteps("./migrations")
	if len(steps) != 3 {
		t.Fatalf("step count = %d, want 3", len(steps))
	}
	want := []string{"up", "down", "up"}
	for i, step := range steps {
		if got := commandString(step); got != want[i] {
			t.Fatalf("step %d = %q, want %q", i, got, want[i])
		}
	}
}

func TestForceDropRehearsalDatabaseStatement(t *testing.T) {
	got := forceDropRehearsalDatabaseStatement("go_admin_kit_migration_rehearsal")
	want := `DROP DATABASE IF EXISTS "go_admin_kit_migration_rehearsal" WITH (FORCE)`
	if got != want {
		t.Fatalf("force drop SQL = %q, want %q", got, want)
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
