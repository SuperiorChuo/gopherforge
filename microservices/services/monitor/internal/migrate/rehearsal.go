package migrate

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io"
	"regexp"
	"strings"
	"time"

	"github.com/go-admin-kit/services/monitor/internal/config"
	"github.com/jackc/pgx/v5/pgconn"
)

const DefaultRehearsalDatabase = "go_admin_kit_migration_rehearsal"

type RehearsalOptions struct {
	ConfigPath  string
	Dir         string
	Database    string
	Keep        bool
	ForwardOnly bool
	LogWriter   io.Writer
}

func RunRehearsal(ctx context.Context, opts RehearsalOptions) error {
	opts = normalizeRehearsalOptions(opts)
	logRehearsal(opts, "loading config %s", opts.ConfigPath)
	if err := validateRehearsalDatabaseName(opts.Database); err != nil {
		return err
	}
	if err := config.LoadConfig(opts.ConfigPath); err != nil {
		return fmt.Errorf("load config: %w", err)
	}
	if strings.EqualFold(strings.TrimSpace(config.Cfg.App.Env), "production") {
		return fmt.Errorf("migration rehearsal refuses to run with APP_ENV=production")
	}
	if err := config.Validate(); err != nil {
		return fmt.Errorf("validate config: %w", err)
	}

	dbCfg := config.Cfg.Database
	logRehearsal(opts, "connecting to database server %s:%d", dbCfg.Host, dbCfg.Port)
	adminDB, err := sql.Open(SQLDriverName(dbCfg.Driver), migrationServerDSN(dbCfg))
	if err != nil {
		return fmt.Errorf("open database server: %w", err)
	}
	defer adminDB.Close()
	pingCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	if err := adminDB.PingContext(pingCtx); err != nil {
		return fmt.Errorf("ping database server: %w", err)
	}

	logRehearsal(opts, "resetting rehearsal database %s", opts.Database)
	if err := resetRehearsalDatabase(ctx, adminDB, opts.Database); err != nil {
		return err
	}
	if !opts.Keep {
		defer func() {
			dropCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()
			_, _ = adminDB.ExecContext(dropCtx, forceDropRehearsalDatabaseStatement(opts.Database))
		}()
	}

	dbCfg.DBName = opts.Database
	for _, step := range rehearsalMigrationSteps(opts.Dir, opts.ForwardOnly) {
		logRehearsal(opts, "running %s", commandString(step))
		stepCtx, cancel := context.WithTimeout(ctx, 2*time.Minute)
		if opts.ForwardOnly && step.Command == "down" {
			err := rehearseForwardOnlyDowngrade(stepCtx, dbCfg, step)
			cancel()
			if err != nil {
				return err
			}
			logRehearsal(opts, "downgrade refused as expected; migration ledger unchanged")
			continue
		}
		err := RunWithConfig(stepCtx, dbCfg, step)
		cancel()
		if err != nil {
			return fmt.Errorf("migration rehearsal %s: %w", commandString(step), err)
		}
	}
	logRehearsal(opts, "migration rehearsal completed")
	return nil
}

const forwardOnlyDowngradeMessage = "Full product upgrade is forward-only; restore the pre-upgrade backup"

func validateForwardOnlyDowngrade(err error) error {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "P0001" && pgErr.Message == forwardOnlyDowngradeMessage {
		return nil
	}
	if err == nil {
		return fmt.Errorf("forward-only rehearsal: downgrade unexpectedly succeeded")
	}
	return fmt.Errorf("forward-only rehearsal: unexpected downgrade failure: %w", err)
}

func rehearseForwardOnlyDowngrade(ctx context.Context, cfg config.DatabaseConfig, step Options) error {
	db, err := sql.Open(SQLDriverName(cfg.Driver), MigrationDSN(cfg))
	if err != nil {
		return err
	}
	defer db.Close()
	const query = `SELECT COALESCE(string_agg(format('%s:%s:%s', id, version_id, is_applied), ',' ORDER BY id), '') FROM goose_db_version`
	var before, after string
	if err := db.QueryRowContext(ctx, query).Scan(&before); err != nil {
		return fmt.Errorf("read pre-downgrade migration ledger: %w", err)
	}
	if err := validateForwardOnlyDowngrade(RunWithConfig(ctx, cfg, step)); err != nil {
		return err
	}
	if err := db.QueryRowContext(ctx, query).Scan(&after); err != nil {
		return fmt.Errorf("read post-downgrade migration ledger: %w", err)
	}
	if before != after {
		return fmt.Errorf("forward-only rehearsal: rejected downgrade changed migration ledger")
	}
	return nil
}

func rehearsalMigrationSteps(dir string, forwardOnly bool) []Options {
	if forwardOnly {
		return []Options{
			{Dir: dir, Command: "up"},
			{Dir: dir, Command: "up"},
			{Dir: dir, Command: "down"},
			{Dir: dir, Command: "up"},
		}
	}
	// Development migrations retain the existing tip-only rollback requirement.
	return []Options{
		{Dir: dir, Command: "up"},
		{Dir: dir, Command: "down"},
		{Dir: dir, Command: "up"},
	}
}

func normalizeRehearsalOptions(opts RehearsalOptions) RehearsalOptions {
	if strings.TrimSpace(opts.ConfigPath) == "" {
		opts.ConfigPath = DefaultConfigPath
	}
	if strings.TrimSpace(opts.Dir) == "" {
		opts.Dir = DefaultDir
	}
	if strings.TrimSpace(opts.Database) == "" {
		opts.Database = DefaultRehearsalDatabase
	}
	opts.Database = strings.TrimSpace(opts.Database)
	return opts
}

func resetRehearsalDatabase(ctx context.Context, db *sql.DB, database string) error {
	if _, err := db.ExecContext(ctx, forceDropRehearsalDatabaseStatement(database)); err != nil {
		return fmt.Errorf("drop rehearsal database: %w", err)
	}
	if _, err := db.ExecContext(ctx, `CREATE DATABASE "`+database+`" ENCODING 'UTF8'`); err != nil {
		return fmt.Errorf("create rehearsal database: %w", err)
	}
	return nil
}

func forceDropRehearsalDatabaseStatement(database string) string {
	// PostgreSQL exporters may discover the short-lived database and retain an
	// idle connection. FORCE is safe here because the validated name can only
	// identify the dedicated scratch database, never an application database.
	return `DROP DATABASE IF EXISTS "` + database + `" WITH (FORCE)`
}

func migrationServerDSN(cfg config.DatabaseConfig) string {
	// Connect to the maintenance database; PostgreSQL has no "no database" mode.
	cfg.DBName = "postgres"
	return cfg.GetDSN()
}

var safeRehearsalDatabaseName = regexp.MustCompile(`^[A-Za-z0-9_]+$`)

func validateRehearsalDatabaseName(database string) error {
	database = strings.TrimSpace(database)
	if database == "" {
		return fmt.Errorf("rehearsal database name is required")
	}
	if !safeRehearsalDatabaseName.MatchString(database) {
		return fmt.Errorf("rehearsal database name %q may only contain letters, digits and underscores", database)
	}
	switch strings.ToLower(database) {
	case "postgres", "template0", "template1", "information_schema":
		return fmt.Errorf("rehearsal database name %q is reserved", database)
	}
	return nil
}

func commandString(opts Options) string {
	if len(opts.Args) == 0 {
		return opts.Command
	}
	return opts.Command + " " + strings.Join(opts.Args, " ")
}

func logRehearsal(opts RehearsalOptions, format string, args ...any) {
	if opts.LogWriter == nil {
		return
	}
	_, _ = fmt.Fprintf(opts.LogWriter, "[migration-rehearsal] "+format+"\n", args...)
}
