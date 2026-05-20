package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"github.com/gin-gonic/gin"
	"github.com/go-admin-kit/server/internal/api"
	"github.com/go-admin-kit/server/internal/openapi"
)

func main() {
	output := flag.String("output", "./docs/openapi.json", "OpenAPI JSON output path")
	title := flag.String("title", "Go Admin Kit API", "OpenAPI document title")
	version := flag.String("version", "dev", "OpenAPI document version")
	server := flag.String("server", "http://localhost:8081", "OpenAPI server URL")
	flag.Parse()

	gin.SetMode(gin.ReleaseMode)
	router := gin.New()
	api.SetupRoutes(router)

	spec := openapi.BuildSpec(router.Routes(), openapi.Options{
		Title:   *title,
		Version: *version,
		Server:  *server,
	})

	payload, err := json.MarshalIndent(spec, "", "  ")
	if err != nil {
		fmt.Fprintf(os.Stderr, "marshal openapi spec failed: %v\n", err)
		os.Exit(1)
	}

	if err := os.MkdirAll(filepath.Dir(*output), 0o755); err != nil {
		fmt.Fprintf(os.Stderr, "create output directory failed: %v\n", err)
		os.Exit(1)
	}
	if err := os.WriteFile(*output, append(payload, '\n'), 0o644); err != nil {
		fmt.Fprintf(os.Stderr, "write openapi spec failed: %v\n", err)
		os.Exit(1)
	}
}
