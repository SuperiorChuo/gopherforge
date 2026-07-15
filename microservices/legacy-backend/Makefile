PROJECT_NAME := go-admin-kit
GO := go

DB_HOST ?= 127.0.0.1
DB_PORT ?= 5432
DB_USER ?= postgres
DB_PASSWORD ?= 123456
DB_NAME ?= go_admin_kit
MIGRATION_REHEARSAL_DB ?= go_admin_kit_migration_rehearsal
POSTGRES_CONTAINER ?= go-admin-kit-postgres
SQL_FILE ?= ./docs/go_admin_kit.sql
CONFIG_FILE ?= ./configs/config.yaml
MIGRATION_DIR ?= ./migrations
MIGRATION_NAME ?= change_name
OBJECT_STORAGE_SMOKE_CONTAINER ?= scaffold-minio
OBJECT_STORAGE_SMOKE_CREATE_BUCKET ?= 1
OBJECT_STORAGE_SMOKE_PROVIDER ?= s3
OBJECT_STORAGE_SMOKE_ENDPOINT ?= 127.0.0.1:9000
OBJECT_STORAGE_SMOKE_BUCKET ?= go-admin-kit
OBJECT_STORAGE_SMOKE_REGION ?= us-east-1
OBJECT_STORAGE_SMOKE_ACCESS_KEY ?= minioadmin
OBJECT_STORAGE_SMOKE_SECRET_KEY ?= minioadmin
OBJECT_STORAGE_SMOKE_USE_SSL ?= false
OBJECT_STORAGE_SMOKE_PREFIX ?= black8-smoke

ifeq ($(strip $(POSTGRES_CONTAINER)),)
PSQL_CLI := PGPASSWORD='$(DB_PASSWORD)' psql -h$(DB_HOST) -p$(DB_PORT) -U$(DB_USER)
PSQL_IMPORT := $(PSQL_CLI)
else
PSQL_CLI := docker exec -e PGPASSWORD='$(DB_PASSWORD)' $(POSTGRES_CONTAINER) psql -h127.0.0.1 -p$(DB_PORT) -U$(DB_USER)
PSQL_IMPORT := docker exec -i -e PGPASSWORD='$(DB_PASSWORD)' $(POSTGRES_CONTAINER) psql -h127.0.0.1 -p$(DB_PORT) -U$(DB_USER)
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
	@$(PSQL_CLI) -d postgres -tc "SELECT 1 FROM pg_database WHERE datname = '$(DB_NAME)'" | grep -q 1 || $(PSQL_CLI) -d postgres -c "CREATE DATABASE \"$(DB_NAME)\" ENCODING 'UTF8';"

.PHONY: db-import
db-import: db-create
	@echo "Importing $(SQL_FILE) into $(DB_NAME)..."
	@$(PSQL_IMPORT) -d $(DB_NAME) < $(SQL_FILE)
	@echo "Database import completed."

.PHONY: db-reset
db-reset:
	@echo "Resetting local database $(DB_NAME)..."
	@$(PSQL_CLI) -d postgres -c "DROP DATABASE IF EXISTS \"$(DB_NAME)\";"
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

.PHONY: migrate-rehearse
migrate-rehearse:
	@$(GO) run ./cmd/migration-rehearsal -config $(CONFIG_FILE) -dir $(MIGRATION_DIR) -database $(MIGRATION_REHEARSAL_DB)

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

.PHONY: smoke-storage-minio
smoke-storage-minio:
ifeq ($(OBJECT_STORAGE_SMOKE_CREATE_BUCKET),1)
	@echo "Ensuring MinIO bucket $(OBJECT_STORAGE_SMOKE_BUCKET) exists in $(OBJECT_STORAGE_SMOKE_CONTAINER)..."
	@docker exec $(OBJECT_STORAGE_SMOKE_CONTAINER) sh -c 'mc alias set smoke http://127.0.0.1:9000 "$$MINIO_ROOT_USER" "$$MINIO_ROOT_PASSWORD" >/dev/null && mc mb -p smoke/$(OBJECT_STORAGE_SMOKE_BUCKET) >/dev/null'
endif
	@echo "Running object storage smoke against $(OBJECT_STORAGE_SMOKE_PROVIDER) endpoint $(OBJECT_STORAGE_SMOKE_ENDPOINT)..."
	@BLACK8_OBJECT_STORAGE_SMOKE=1 \
	BLACK8_OBJECT_STORAGE_PROVIDER='$(OBJECT_STORAGE_SMOKE_PROVIDER)' \
	BLACK8_OBJECT_STORAGE_ENDPOINT='$(OBJECT_STORAGE_SMOKE_ENDPOINT)' \
	BLACK8_OBJECT_STORAGE_BUCKET='$(OBJECT_STORAGE_SMOKE_BUCKET)' \
	BLACK8_OBJECT_STORAGE_REGION='$(OBJECT_STORAGE_SMOKE_REGION)' \
	BLACK8_OBJECT_STORAGE_ACCESS_KEY='$(OBJECT_STORAGE_SMOKE_ACCESS_KEY)' \
	BLACK8_OBJECT_STORAGE_SECRET_KEY='$(OBJECT_STORAGE_SMOKE_SECRET_KEY)' \
	BLACK8_OBJECT_STORAGE_USE_SSL='$(OBJECT_STORAGE_SMOKE_USE_SSL)' \
	BLACK8_OBJECT_STORAGE_OBJECT_PREFIX='$(OBJECT_STORAGE_SMOKE_PREFIX)' \
	$(GO) test ./internal/pkg/upload -run TestObjectStorageSmokeOpenReadsRealEndpoint -count=1 -v

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
