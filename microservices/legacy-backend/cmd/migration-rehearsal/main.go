package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/go-admin-kit/server/internal/migrate"
)

func main() {
	var opts migrate.RehearsalOptions
	flag.StringVar(&opts.ConfigPath, "config", migrate.DefaultConfigPath, "path to backend config yaml")
	flag.StringVar(&opts.Dir, "dir", migrate.DefaultDir, "path to goose migrations directory")
	flag.StringVar(&opts.Database, "database", migrate.DefaultRehearsalDatabase, "temporary database used for migration rehearsal")
	flag.BoolVar(&opts.Keep, "keep", false, "keep the temporary rehearsal database after the run")
	flag.Parse()
	opts.LogWriter = os.Stdout

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Minute)
	defer cancel()

	if err := migrate.RunRehearsal(ctx, opts); err != nil {
		fmt.Fprintf(os.Stderr, "migration rehearsal failed: %v\n", err)
		os.Exit(1)
	}
}
