package migrate

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"strings"
	"testing"

	"github.com/go-admin-kit/server/internal/config"
)

func TestParseOptionsDefaultsToUp(t *testing.T) {
	opts, err := ParseOptions(nil)
	if err != nil {
		t.Fatalf("ParseOptions returned error: %v", err)
	}

	if opts.ConfigPath != "./configs/config.yaml" {
		t.Fatalf("ConfigPath = %q, want default config", opts.ConfigPath)
	}
	if opts.Dir != "./migrations" {
		t.Fatalf("Dir = %q, want default migrations dir", opts.Dir)
	}
	if opts.Command != "up" {
		t.Fatalf("Command = %q, want up", opts.Command)
	}
	if len(opts.Args) != 0 {
		t.Fatalf("Args = %v, want empty", opts.Args)
	}
}

func TestParseOptionsAcceptsFlagsAndCommandArgs(t *testing.T) {
	opts, err := ParseOptions([]string{
		"-config", "configs/test.yaml",
		"-dir", "db/migrations",
		"down-to", "202605200001",
	})
	if err != nil {
		t.Fatalf("ParseOptions returned error: %v", err)
	}

	if opts.ConfigPath != "configs/test.yaml" {
		t.Fatalf("ConfigPath = %q", opts.ConfigPath)
	}
	if opts.Dir != "db/migrations" {
		t.Fatalf("Dir = %q", opts.Dir)
	}
	if opts.Command != "down-to" {
		t.Fatalf("Command = %q", opts.Command)
	}
	if !reflect.DeepEqual(opts.Args, []string{"202605200001"}) {
		t.Fatalf("Args = %v", opts.Args)
	}
}

func TestParseOptionsRejectsUnknownCommand(t *testing.T) {
	_, err := ParseOptions([]string{"drop-everything"})
	if err == nil {
		t.Fatal("ParseOptions returned nil error for unknown command")
	}
	if !strings.Contains(err.Error(), "unsupported migration command") {
		t.Fatalf("error = %q, want unsupported command", err.Error())
	}
}

func TestParseOptionsAcceptsCreateCommand(t *testing.T) {
	opts, err := ParseOptions([]string{"create", "add_widgets", "sql"})
	if err != nil {
		t.Fatalf("ParseOptions returned error: %v", err)
	}

	if opts.Command != "create" {
		t.Fatalf("Command = %q, want create", opts.Command)
	}
	if !reflect.DeepEqual(opts.Args, []string{"add_widgets", "sql"}) {
		t.Fatalf("Args = %v", opts.Args)
	}
}

func TestDialectForDriver(t *testing.T) {
	dialect, err := DialectForDriver("mysql")
	if err != nil {
		t.Fatalf("DialectForDriver returned error: %v", err)
	}
	if dialect != "mysql" {
		t.Fatalf("dialect = %q, want mysql", dialect)
	}

	if _, err := DialectForDriver("postgres"); err == nil {
		t.Fatal("DialectForDriver accepted unsupported driver")
	}
}

func TestMigrationDSNEnablesMultiStatements(t *testing.T) {
	cfg := config.DatabaseConfig{
		User:     "root",
		Password: "secret",
		Host:     "127.0.0.1",
		Port:     3306,
		DBName:   "go_admin_kit",
		Charset:  "utf8mb4",
	}

	dsn := MigrationDSN(cfg)
	if !strings.Contains(dsn, "multiStatements=true") {
		t.Fatalf("dsn = %q, want multiStatements=true", dsn)
	}
}

func TestMigrationDSNDoesNotDuplicateMultiStatements(t *testing.T) {
	input := "root:secret@tcp(127.0.0.1:3306)/go_admin_kit?charset=utf8mb4&multiStatements=true"
	dsn := ensureMultiStatements(input)

	if strings.Count(dsn, "multiStatements=true") != 1 {
		t.Fatalf("dsn = %q, want one multiStatements=true", dsn)
	}
}

func TestRunCreateWritesSQLMigrationWithoutDatabaseConfig(t *testing.T) {
	dir := t.TempDir()
	err := Run(context.Background(), Options{
		ConfigPath: filepath.Join(dir, "missing.yaml"),
		Dir:        dir,
		Command:    "create",
		Args:       []string{"add_widgets", "sql"},
	})
	if err != nil {
		t.Fatalf("Run create returned error: %v", err)
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("read temp migration dir: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("created files = %d, want 1", len(entries))
	}

	name := entries[0].Name()
	if !strings.Contains(name, "add_widgets") || !strings.HasSuffix(name, ".sql") {
		t.Fatalf("created migration name = %q", name)
	}

	content, err := os.ReadFile(filepath.Join(dir, name))
	if err != nil {
		t.Fatalf("read created migration: %v", err)
	}
	if !strings.Contains(string(content), "-- +goose Up") || !strings.Contains(string(content), "-- +goose Down") {
		t.Fatalf("created migration missing goose sections:\n%s", string(content))
	}
}

func TestBaselineMigrationUpIsNonDestructive(t *testing.T) {
	content, err := os.ReadFile("../../migrations/000001_init_go_admin_kit.sql")
	if err != nil {
		t.Fatalf("read baseline migration: %v", err)
	}

	parts := strings.Split(string(content), "-- +goose Down")
	if len(parts) != 2 {
		t.Fatalf("baseline migration should have one Down section, got %d sections", len(parts))
	}

	up := strings.ToUpper(parts[0])
	if strings.Contains(up, "DROP TABLE") {
		t.Fatal("baseline migration Up section must not drop tables")
	}
	if !strings.Contains(up, "CREATE TABLE IF NOT EXISTS") {
		t.Fatal("baseline migration Up section should create tables idempotently")
	}
	if !strings.Contains(up, "INSERT IGNORE INTO") {
		t.Fatal("baseline migration Up section should seed rows idempotently")
	}
}

func TestBaselineMigrationDownDropsEveryCreatedTable(t *testing.T) {
	content, err := os.ReadFile("../../migrations/000001_init_go_admin_kit.sql")
	if err != nil {
		t.Fatalf("read baseline migration: %v", err)
	}

	parts := strings.Split(string(content), "-- +goose Down")
	if len(parts) != 2 {
		t.Fatalf("baseline migration should have one Down section, got %d sections", len(parts))
	}

	createRe := regexp.MustCompile("CREATE TABLE IF NOT EXISTS `([^`]+)`")
	dropRe := regexp.MustCompile("DROP TABLE IF EXISTS `([^`]+)`")

	created := createRe.FindAllStringSubmatch(parts[0], -1)
	droppedMatches := dropRe.FindAllStringSubmatch(parts[1], -1)
	dropped := make(map[string]struct{}, len(droppedMatches))
	for _, match := range droppedMatches {
		dropped[match[1]] = struct{}{}
	}

	for _, match := range created {
		table := match[1]
		if _, ok := dropped[table]; !ok {
			t.Fatalf("Down section does not drop table %q", table)
		}
	}
}

func TestPermissionDescriptionMigrationExists(t *testing.T) {
	addMigration, err := os.ReadFile("../../migrations/000002_add_permission_description.sql")
	if err != nil {
		t.Fatalf("read incremental permission description migration: %v", err)
	}
	sql := strings.ToLower(string(addMigration))
	for _, want := range []string{
		"alter table `permissions`",
		"add column `description`",
		"drop column `description`",
	} {
		if !strings.Contains(sql, want) {
			t.Fatalf("incremental migration missing %q", want)
		}
	}
}
