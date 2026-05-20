package migrate

import (
	"context"
	"database/sql"
	"flag"
	"fmt"
	"io"
	"strings"

	"github.com/go-admin-kit/server/internal/config"
	_ "github.com/go-sql-driver/mysql"
	"github.com/pressly/goose/v3"
)

const (
	DefaultConfigPath = "./configs/config.yaml"
	DefaultDir        = "./migrations"
)

type Options struct {
	ConfigPath string
	Dir        string
	Command    string
	Args       []string
}

func ParseOptions(args []string) (Options, error) {
	opts := Options{
		ConfigPath: DefaultConfigPath,
		Dir:        DefaultDir,
		Command:    "up",
	}

	fs := flag.NewFlagSet("migrate", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	fs.StringVar(&opts.ConfigPath, "config", opts.ConfigPath, "path to backend config yaml")
	fs.StringVar(&opts.Dir, "dir", opts.Dir, "path to goose migrations directory")
	if err := fs.Parse(args); err != nil {
		return opts, err
	}

	remaining := fs.Args()
	if len(remaining) > 0 {
		opts.Command = strings.ToLower(strings.TrimSpace(remaining[0]))
		opts.Args = remaining[1:]
	}
	if !isSupportedCommand(opts.Command) {
		return opts, fmt.Errorf("unsupported migration command %q; supported: %s", opts.Command, strings.Join(supportedCommands(), ", "))
	}
	return opts, nil
}

func Run(ctx context.Context, opts Options) error {
	if opts.Command == "create" {
		return runCreate(opts)
	}

	if err := config.LoadConfig(opts.ConfigPath); err != nil {
		return fmt.Errorf("load config: %w", err)
	}
	if err := config.Validate(); err != nil {
		return fmt.Errorf("validate config: %w", err)
	}

	return RunWithConfig(ctx, config.Cfg.Database, opts)
}

func runCreate(opts Options) error {
	if len(opts.Args) == 0 {
		return fmt.Errorf("create migration requires a name argument")
	}

	migrationType := "sql"
	if len(opts.Args) > 1 {
		migrationType = opts.Args[1]
	}
	if err := goose.Create(nil, opts.Dir, opts.Args[0], migrationType); err != nil {
		return fmt.Errorf("create migration: %w", err)
	}
	return nil
}

func RunWithConfig(ctx context.Context, dbCfg config.DatabaseConfig, opts Options) error {
	dialect, err := DialectForDriver(dbCfg.Driver)
	if err != nil {
		return err
	}
	if err := goose.SetDialect(dialect); err != nil {
		return fmt.Errorf("set goose dialect: %w", err)
	}

	db, err := sql.Open(dbCfg.Driver, MigrationDSN(dbCfg))
	if err != nil {
		return fmt.Errorf("open database: %w", err)
	}
	defer db.Close()

	if err := db.PingContext(ctx); err != nil {
		return fmt.Errorf("ping database: %w", err)
	}
	if err := goose.RunContext(ctx, opts.Command, db, opts.Dir, opts.Args...); err != nil {
		return fmt.Errorf("run migration %q: %w", opts.Command, err)
	}
	return nil
}

func DialectForDriver(driver string) (string, error) {
	switch strings.ToLower(strings.TrimSpace(driver)) {
	case "mysql":
		return "mysql", nil
	default:
		return "", fmt.Errorf("unsupported database driver %q for migrations", driver)
	}
}

func MigrationDSN(cfg config.DatabaseConfig) string {
	return ensureMultiStatements(cfg.GetDSN())
}

func ensureMultiStatements(dsn string) string {
	if strings.Contains(dsn, "multiStatements=") {
		return dsn
	}
	if strings.Contains(dsn, "?") {
		return dsn + "&multiStatements=true"
	}
	return dsn + "?multiStatements=true"
}

func isSupportedCommand(command string) bool {
	for _, supported := range supportedCommands() {
		if command == supported {
			return true
		}
	}
	return false
}

func supportedCommands() []string {
	return []string{
		"up",
		"up-by-one",
		"up-to",
		"down",
		"down-to",
		"redo",
		"reset",
		"status",
		"version",
		"create",
	}
}
