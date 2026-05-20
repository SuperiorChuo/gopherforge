PROJECT_NAME := go-admin-kit
GO := go

DB_HOST ?= 127.0.0.1
DB_PORT ?= 3306
DB_USER ?= root
DB_PASSWORD ?= 123456
DB_NAME ?= go_admin_kit
MYSQL_CONTAINER ?= go-admin-kit-mysql
SQL_FILE ?= ./docs/go_admin_kit.sql
CONFIG_FILE ?= ./configs/config.yaml
MIGRATION_DIR ?= ./migrations
MIGRATION_NAME ?= change_name

ifeq ($(strip $(MYSQL_CONTAINER)),)
MYSQL_CLI := MYSQL_PWD='$(DB_PASSWORD)' mysql --default-character-set=utf8mb4 -h$(DB_HOST) -P$(DB_PORT) -u$(DB_USER)
MYSQL_IMPORT := $(MYSQL_CLI)
else
MYSQL_CLI := docker exec -e MYSQL_PWD='$(DB_PASSWORD)' $(MYSQL_CONTAINER) mysql --default-character-set=utf8mb4 -h127.0.0.1 -P$(DB_PORT) -u$(DB_USER)
MYSQL_IMPORT := docker exec -i -e MYSQL_PWD='$(DB_PASSWORD)' $(MYSQL_CONTAINER) mysql --default-character-set=utf8mb4 -h127.0.0.1 -P$(DB_PORT) -u$(DB_USER)
endif

BUILD_DIR := ./build
MAIN_FILE := ./cmd/main.go
TARGET := $(BUILD_DIR)/$(PROJECT_NAME)

VERSION := $(shell git describe --tags --always 2>/dev/null || echo "dev")
COMMIT := $(shell git rev-parse --short HEAD 2>/dev/null || echo "unknown")
BUILD_TIME := $(shell date +%Y-%m-%d_%H:%M:%S)
GOFLAGS := -ldflags "-X main.Version=$(VERSION) -X main.Commit=$(COMMIT) -X main.BuildTime=$(BUILD_TIME)"

.PHONY: deps
deps:
	@echo "Installing dependencies..."
	@$(GO) mod tidy

.PHONY: db-create
db-create:
	@echo "Creating local database $(DB_NAME)..."
	@$(MYSQL_CLI) -e "CREATE DATABASE IF NOT EXISTS \`$(DB_NAME)\` DEFAULT CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci;"

.PHONY: db-import
db-import: db-create
	@echo "Importing $(SQL_FILE) into $(DB_NAME)..."
	@$(MYSQL_IMPORT) $(DB_NAME) < $(SQL_FILE)
	@echo "Database import completed."

.PHONY: db-reset
db-reset:
	@echo "Resetting local database $(DB_NAME)..."
	@$(MYSQL_CLI) -e "DROP DATABASE IF EXISTS \`$(DB_NAME)\`;"
	@$(MAKE) db-import

.PHONY: migrate-up
migrate-up:
	@$(GO) run ./cmd/migrate -config $(CONFIG_FILE) -dir $(MIGRATION_DIR) up

.PHONY: migrate-status
migrate-status:
	@$(GO) run ./cmd/migrate -config $(CONFIG_FILE) -dir $(MIGRATION_DIR) status

.PHONY: migrate-down
migrate-down:
	@$(GO) run ./cmd/migrate -config $(CONFIG_FILE) -dir $(MIGRATION_DIR) down

.PHONY: migrate-redo
migrate-redo:
	@$(GO) run ./cmd/migrate -config $(CONFIG_FILE) -dir $(MIGRATION_DIR) redo

.PHONY: migrate-reset
migrate-reset:
	@$(GO) run ./cmd/migrate -config $(CONFIG_FILE) -dir $(MIGRATION_DIR) reset

.PHONY: migrate-create
migrate-create:
	@$(GO) run ./cmd/migrate -dir $(MIGRATION_DIR) create $(MIGRATION_NAME) sql

.PHONY: build build-server
build: build-server

build-server:
	@echo "Building $(PROJECT_NAME)..."
	@mkdir -p $(BUILD_DIR)
	@CGO_ENABLED=0 $(GO) build $(GOFLAGS) -o $(TARGET) $(MAIN_FILE)
	@echo "Build completed: $(TARGET)"

.PHONY: run
run:
	@echo "Running $(PROJECT_NAME)..."
	@CGO_ENABLED=0 $(GO) run $(MAIN_FILE)

.PHONY: test
test:
	@echo "Running tests..."
	@$(GO) test ./... -v

.PHONY: clean
clean:
	@echo "Cleaning build artifacts..."
	@rm -rf $(BUILD_DIR)

.PHONY: swagger
swagger:
	@$(MAKE) openapi

.PHONY: openapi
openapi:
	@echo "Generating OpenAPI contract..."
	@$(GO) run ./cmd/openapi -output ./docs/openapi.json

.PHONY: fmt
fmt:
	@echo "Formatting code..."
	@$(GO) fmt ./...

.PHONY: vet
vet:
	@echo "Vetting code..."
	@$(GO) vet ./...

.PHONY: build-run
build-run:
	@$(MAKE) build-server
	@$(MAKE) run
