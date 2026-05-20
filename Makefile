SHELL := /bin/sh

.PHONY: help
help:
	@echo "Available targets:"
	@echo "  dev-backend       Start backend locally"
	@echo "  dev-frontend      Start frontend with real API"
	@echo "  build-server      Build the Go backend server"
	@echo "  compose-up        Start full stack with Docker Compose"
	@echo "  compose-down      Stop Docker Compose stack"
	@echo "  test              Run backend tests and frontend unit/type checks"
	@echo "  lint              Run backend vet and frontend lint/style/type checks"
	@echo "  audit             Run frontend production dependency audit"
	@echo "  smoke-api         Run API smoke tests against a running backend"
	@echo "  db-import         Import Go Admin Kit SQL into local MySQL"
	@echo "  migrate-up        Apply database migrations"
	@echo "  migrate-status    Show database migration status"
	@echo "  migrate-create    Create a new SQL migration, pass NAME=add_table"
	@echo "  api-contract      Generate OpenAPI JSON and frontend API types"
	@echo "  status            Show local service status"
	@echo "  logs              Tail backend/frontend logs"

.PHONY: dev-backend
dev-backend:
	cd server && CGO_ENABLED=0 go run ./cmd/main.go

.PHONY: build-server
build-server:
	cd server && $(MAKE) build-server

.PHONY: dev-frontend
dev-frontend:
	cd tdesign-vue-go && npm run dev:linux -- --host 127.0.0.1 --port 3000

.PHONY: compose-up
compose-up:
	docker compose up -d --build

.PHONY: compose-down
compose-down:
	docker compose down

.PHONY: compose-monitoring
compose-monitoring:
	docker compose --profile monitoring up -d --build

.PHONY: test
test:
	npm run test:smoke:unit
	cd server && go test ./...
	cd tdesign-vue-go && npm run test
	cd tdesign-vue-go && npm run build:type

.PHONY: lint
lint:
	cd server && go vet ./...
	cd tdesign-vue-go && npm run build:type
	cd tdesign-vue-go && npm run lint
	cd tdesign-vue-go && npm run stylelint

.PHONY: audit
audit:
	cd tdesign-vue-go && npm audit --omit=dev

.PHONY: smoke-api
smoke-api:
	npm run smoke:api

.PHONY: e2e-api
e2e-api: smoke-api

.PHONY: db-import
db-import:
	cd server && $(MAKE) db-import

.PHONY: migrate-up
migrate-up:
	cd server && $(MAKE) migrate-up

.PHONY: migrate-status
migrate-status:
	cd server && $(MAKE) migrate-status

.PHONY: migrate-down
migrate-down:
	cd server && $(MAKE) migrate-down

.PHONY: migrate-redo
migrate-redo:
	cd server && $(MAKE) migrate-redo

.PHONY: migrate-reset
migrate-reset:
	cd server && $(MAKE) migrate-reset

.PHONY: migrate-create
migrate-create:
	cd server && $(MAKE) migrate-create MIGRATION_NAME=$(or $(NAME),change_name)

.PHONY: api-contract
api-contract:
	npm run api:contract

.PHONY: status
status:
	@docker ps --filter name='go-admin-kit-' --format 'table {{.Names}}\t{{.Status}}\t{{.Ports}}' || true
	@lsof -nP -iTCP:3000 -sTCP:LISTEN || true
	@lsof -nP -iTCP:3001 -sTCP:LISTEN || true
	@lsof -nP -iTCP:8081 -sTCP:LISTEN || true

.PHONY: logs
logs:
	@tail -n 80 /tmp/go-admin-kit-backend.log 2>/dev/null || true
	@tail -n 40 /tmp/go-admin-kit-frontend-real.log 2>/dev/null || true
