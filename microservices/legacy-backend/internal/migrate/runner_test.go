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
	for _, driver := range []string{"postgres", "pgx"} {
		dialect, err := DialectForDriver(driver)
		if err != nil {
			t.Fatalf("DialectForDriver(%q) returned error: %v", driver, err)
		}
		if dialect != "postgres" {
			t.Fatalf("dialect = %q, want postgres", dialect)
		}
	}

	if _, err := DialectForDriver("mysql"); err == nil {
		t.Fatal("DialectForDriver accepted unsupported driver")
	}
}

func TestSQLDriverName(t *testing.T) {
	for _, driver := range []string{"postgres", "pgx"} {
		if got := SQLDriverName(driver); got != "pgx" {
			t.Fatalf("SQLDriverName(%q) = %q, want pgx", driver, got)
		}
	}
}

func TestMigrationDSNAppendsConnectTimeout(t *testing.T) {
	cfg := config.DatabaseConfig{
		User:     "postgres",
		Password: "secret",
		Host:     "127.0.0.1",
		Port:     5432,
		DBName:   "go_admin_kit",
		SSLMode:  "disable",
	}

	dsn := MigrationDSN(cfg)
	if !strings.Contains(dsn, "connect_timeout=10") {
		t.Fatalf("dsn = %q, want connect_timeout=10", dsn)
	}
	for _, want := range []string{"host=127.0.0.1", "port=5432", "user=postgres", "dbname=go_admin_kit", "sslmode=disable", "password=secret"} {
		if !strings.Contains(dsn, want) {
			t.Fatalf("dsn = %q, want %s", dsn, want)
		}
	}
}

func TestEnsureDSNParamDoesNotDuplicateExistingKey(t *testing.T) {
	input := "host=127.0.0.1 port=5432 user=postgres dbname=go_admin_kit sslmode=disable connect_timeout=3"
	dsn := ensureDSNParam(input, "connect_timeout", "10")

	if strings.Count(dsn, "connect_timeout=") != 1 || !strings.Contains(dsn, "connect_timeout=3") {
		t.Fatalf("dsn = %q, want existing connect_timeout preserved", dsn)
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
	if !strings.Contains(up, "ON CONFLICT DO NOTHING") {
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

	createRe := regexp.MustCompile(`CREATE TABLE IF NOT EXISTS ([A-Za-z0-9_]+)`)
	dropRe := regexp.MustCompile(`DROP TABLE IF EXISTS ([A-Za-z0-9_]+)`)

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
		"alter table permissions",
		"add column description",
		"drop column description",
	} {
		if !strings.Contains(sql, want) {
			t.Fatalf("incremental migration missing %q", want)
		}
	}
}

func TestPasswordPolicyMigrationExists(t *testing.T) {
	addMigration, err := os.ReadFile("../../migrations/000003_add_password_policy.sql")
	if err != nil {
		t.Fatalf("read incremental password policy migration: %v", err)
	}
	sql := strings.ToLower(string(addMigration))
	for _, want := range []string{
		"alter table users",
		"add column password_changed_at",
		"create table if not exists password_history",
		"drop table if exists password_history",
		"drop column password_changed_at",
	} {
		if !strings.Contains(sql, want) {
			t.Fatalf("password policy migration missing %q", want)
		}
	}
}

func TestTOTP2FAMigrationExists(t *testing.T) {
	addMigration, err := os.ReadFile("../../migrations/000004_add_totp_2fa.sql")
	if err != nil {
		t.Fatalf("read incremental totp migration: %v", err)
	}
	sql := strings.ToLower(string(addMigration))
	for _, want := range []string{
		"alter table users",
		"add column totp_secret",
		"add column totp_enabled",
		"create table if not exists totp_recovery_codes",
		"foreign key (user_id) references users (id) on delete cascade",
		"drop table if exists totp_recovery_codes",
		"drop column totp_enabled",
		"drop column totp_secret",
	} {
		if !strings.Contains(sql, want) {
			t.Fatalf("totp migration missing %q", want)
		}
	}
}

func TestSystemSettingsRouteMigrationExists(t *testing.T) {
	addMigration, err := os.ReadFile("../../migrations/000005_add_system_settings_route.sql")
	if err != nil {
		t.Fatalf("read incremental system settings migration: %v", err)
	}
	sql := strings.ToLower(string(addMigration))
	for _, want := range []string{
		"system:setting:list",
		"system:setting:update",
		"system:setting:delete",
		"/api/v1/system-settings",
		"/system/setting",
		"system/setting/index",
	} {
		if !strings.Contains(sql, want) {
			t.Fatalf("system settings migration missing %q", want)
		}
	}
}

func TestSensitivePermissionTighteningMigrationExists(t *testing.T) {
	addMigration, err := os.ReadFile("../../migrations/000006_tighten_sensitive_permissions.sql")
	if err != nil {
		t.Fatalf("read incremental sensitive permission migration: %v", err)
	}
	sql := strings.ToLower(string(addMigration))
	for _, want := range []string{
		"system:log:operation:clear",
		"rp.role_id = 2",
		"system:setting:update",
		"system:setting:delete",
	} {
		if !strings.Contains(sql, want) {
			t.Fatalf("sensitive permission migration missing %q", want)
		}
	}
}

func TestRenameWMTablesMigrationExists(t *testing.T) {
	addMigration, err := os.ReadFile("../../migrations/000007_rename_wm_tables.sql")
	if err != nil {
		t.Fatalf("read incremental table rename migration: %v", err)
	}
	sql := strings.ToLower(string(addMigration))
	for _, want := range []string{
		"alter table wm_audit_log rename to audit_logs",
		"alter table wm_console_route rename to console_routes",
		"alter table wm_console_session rename to console_sessions",
		"alter table wm_system_setting rename to system_settings",
		"alter table system_settings rename to wm_system_setting",
		"alter table console_sessions rename to wm_console_session",
		"alter table console_routes rename to wm_console_route",
		"alter table audit_logs rename to wm_audit_log",
		"alter index idx_wm_audit_log_created_at rename to idx_audit_logs_created_at",
		"alter index idx_wm_console_route_path rename to idx_console_routes_path",
		"alter index idx_wm_console_session_username rename to idx_console_sessions_username",
		"alter index idx_wm_system_setting_updated_at rename to idx_system_settings_updated_at",
		"alter index idx_system_settings_updated_at rename to idx_wm_system_setting_updated_at",
		"alter index idx_console_sessions_username rename to idx_wm_console_session_username",
		"alter index idx_console_routes_path rename to idx_wm_console_route_path",
		"alter index idx_audit_logs_created_at rename to idx_wm_audit_log_created_at",
	} {
		if !strings.Contains(sql, want) {
			t.Fatalf("table rename migration missing %q", want)
		}
	}
}

func TestOAuthBindingUserProviderUniqueMigrationExists(t *testing.T) {
	addMigration, err := os.ReadFile("../../migrations/000008_add_oauth_binding_user_provider_unique.sql")
	if err != nil {
		t.Fatalf("read oauth binding unique migration: %v", err)
	}
	sql := strings.ToLower(string(addMigration))
	for _, want := range []string{
		"delete older duplicate rows before adding the user/provider unique key",
		"delete from oauth_bindings ob",
		"create unique index if not exists uk_oauth_bindings_user_provider on oauth_bindings (user_id, provider)",
		"drop index if exists uk_oauth_bindings_user_provider",
	} {
		if !strings.Contains(sql, want) {
			t.Fatalf("oauth binding unique migration missing %q", want)
		}
	}
}
