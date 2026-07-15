package main

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/go-admin-kit/server/internal/migrate"
)

func main() {
	opts, err := migrate.ParseOptions(os.Args[1:])
	if err != nil {
		fmt.Fprintf(os.Stderr, "parse migration options failed: %v\n", err)
		os.Exit(2)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()

	if err := migrate.Run(ctx, opts); err != nil {
		fmt.Fprintf(os.Stderr, "migration failed: %v\n", err)
		os.Exit(1)
	}
}
