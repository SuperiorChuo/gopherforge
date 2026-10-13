package migrate

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/go-admin-kit/services/monitor/internal/config"
	"github.com/jackc/pgx/v5/pgconn"
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

// The public release appends a forward migration instead of rewriting its historical SQL.
func latestMigration(t *testing.T, slug string) string {
	t.Helper()
	paths, err := filepath.Glob("../../migrations/*_" + slug + ".sql")
	if err != nil || len(paths) == 0 {
		t.Fatalf("find migration %s: %v", slug, err)
	}
	content, err := os.ReadFile(paths[len(paths)-1])
	if err != nil {
		t.Fatalf("read migration: %v", err)
	}
	return string(content)
}

func TestBusinessSchemaSplitAllowsFreshDatabase(t *testing.T) {
	migration := latestMigration(t, "business_schema_split")
	if !strings.Contains(migration, "gak_copy_public_table_if_exists") && !strings.Contains(migration, "gak_move_public_table_if_exists") {
		t.Fatal("migration must guard copies of service-owned AutoMigrate tables")
	}
	if !strings.Contains(migration, "-- +goose StatementBegin\nCREATE OR REPLACE FUNCTION") ||
		!strings.Contains(migration, "$$;\n-- +goose StatementEnd") {
		t.Fatal("PL/pgSQL helper must be wrapped in goose statement markers")
	}
	if strings.Contains(migration, "(LIKE public.") {
		t.Fatal("migration contains an unconditional public-table copy that breaks fresh databases")
	}
	if strings.Contains(migration, "gak_move_public_table_if_exists") {
		for _, required := range []string{"SET SCHEMA", "IS NULL THEN", "IS NOT NULL THEN", "RAISE EXCEPTION"} {
			if !strings.Contains(migration, required) {
				t.Fatalf("public migration missing data preservation guard %q", required)
			}
		}
		if strings.Contains(migration, "DROP TABLE") || strings.Contains(migration, "INCLUDING ALL") {
			t.Fatal("public migration must preserve table identity and foreign keys")
		}
	}
}

func TestCRMTagsMigrationAllowsFreshDatabase(t *testing.T) {
	migration := latestMigration(t, "crm_tags")
	if !strings.Contains(migration, "to_regclass('public.crm_customers')") && !strings.Contains(migration, "to_regclass('crm_svc.crm_customers')") {
		t.Fatal("CRM tag migration must skip fresh databases before crm-service AutoMigrate")
	}
	if !strings.Contains(migration, "-- +goose StatementBegin") ||
		!strings.Contains(migration, "-- +goose StatementEnd") {
		t.Fatal("guarded CRM migration must use goose statement markers")
	}
}

func TestServiceIDSequenceRepairCoversSchemaSplitTables(t *testing.T) {
	split := latestMigration(t, "business_schema_split")
	repairText := latestMigration(t, "repair_service_id_sequences")
	callPattern := regexp.MustCompile(`gak_(?:copy|move)_public_table_if_exists\('([^']+)', '([^']+)'(?:, TRUE)?\)`)
	matches := callPattern.FindAllStringSubmatch(split, -1)
	if len(matches) == 0 {
		t.Fatal("schema split contains no verifiable table inventory")
	}
	for _, match := range matches {
		if match[2] == "im_agent_presence" { // This table has no serial id.
			continue
		}
		entry := fmt.Sprintf("('%s', '%s')", match[1], match[2])
		if !strings.Contains(repairText, entry) {
			t.Errorf("sequence repair is missing %s.%s", match[1], match[2])
		}
	}
	for _, required := range []string{
		"CREATE SEQUENCE IF NOT EXISTS", "OWNED BY", "ALTER COLUMN id SET DEFAULT",
		"COALESCE(MAX(id), 0)",
	} {
		if !strings.Contains(repairText, required) {
			t.Errorf("sequence repair is missing contract %q", required)
		}
	}
	// Public upgrades fail closed on down; source development can undo only its own sequences.
	if !strings.Contains(repairText, "Full product upgrade is forward-only") {
		for _, required := range []string{"ALTER COLUMN id DROP DEFAULT", "DROP SEQUENCE IF EXISTS"} {
			if !strings.Contains(repairText, required) {
				t.Errorf("sequence rollback is missing %q", required)
			}
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
	steps := rehearsalMigrationSteps("./migrations", false)
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

func TestForwardOnlyRehearsalSteps(t *testing.T) {
	steps := rehearsalMigrationSteps("./public-migrations", true)
	want := []string{"up", "up", "down", "up"}
	if len(steps) != len(want) {
		t.Fatalf("step count = %d, want %d", len(steps), len(want))
	}
	for i, step := range steps {
		if step.Command != want[i] || step.Dir != "./public-migrations" {
			t.Fatalf("step %d = %+v", i, step)
		}
	}
}

func TestForwardOnlyDowngradeRequiresExactDatabaseRejection(t *testing.T) {
	guard := &pgconn.PgError{Code: "P0001", Message: forwardOnlyDowngradeMessage}
	for _, err := range []error{guard, fmt.Errorf("goose failure: %w", guard)} {
		if got := validateForwardOnlyDowngrade(err); got != nil {
			t.Fatalf("expected downgrade guard rejected: %v", got)
		}
	}
	for _, err := range []error{
		nil, errors.New("connection refused"), errors.New(forwardOnlyDowngradeMessage),
		&pgconn.PgError{Code: "P0001", Message: "unrelated failure"},
		&pgconn.PgError{Code: "23505", Message: forwardOnlyDowngradeMessage},
	} {
		if validateForwardOnlyDowngrade(err) == nil {
			t.Fatalf("unexpected downgrade outcome accepted: %v", err)
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
