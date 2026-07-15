package migrate

import (
	"context"
	"database/sql"
	"fmt"
	"io"
	"regexp"
	"strings"
	"time"

	"github.com/go-admin-kit/server/internal/config"
)

const DefaultRehearsalDatabase = "go_admin_kit_migration_rehearsal"

type RehearsalOptions struct {
	ConfigPath string
	Dir        string
	Database   string
	Keep       bool
	LogWriter  io.Writer
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
			_, _ = adminDB.ExecContext(dropCtx, `DROP DATABASE IF EXISTS "`+opts.Database+`"`)
		}()
	}

	dbCfg.DBName = opts.Database
	steps := []Options{
		{Dir: opts.Dir, Command: "up"},
		{Dir: opts.Dir, Command: "down-to", Args: []string{"0"}},
		{Dir: opts.Dir, Command: "up"},
	}
	for _, step := range steps {
		logRehearsal(opts, "running %s", commandString(step))
		stepCtx, cancel := context.WithTimeout(ctx, 2*time.Minute)
		err := RunWithConfig(stepCtx, dbCfg, step)
		cancel()
		if err != nil {
			return fmt.Errorf("migration rehearsal %s: %w", commandString(step), err)
		}
	}
	logRehearsal(opts, "migration rehearsal completed")
	return nil
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
	if _, err := db.ExecContext(ctx, `DROP DATABASE IF EXISTS "`+database+`"`); err != nil {
		return fmt.Errorf("drop rehearsal database: %w", err)
	}
	if _, err := db.ExecContext(ctx, `CREATE DATABASE "`+database+`" ENCODING 'UTF8'`); err != nil {
		return fmt.Errorf("create rehearsal database: %w", err)
	}
	return nil
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
