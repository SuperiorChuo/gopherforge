SHELL := /bin/sh

.PHONY: help
help:
	@echo "单体项目 (monolith/) 可用目标:"
	@echo "  compose-up / compose-down"
	@echo "  dev-backend / dev-frontend"
	@echo "  test / migrate-up"

.PHONY: compose-up
compose-up:
	docker compose up -d --build

.PHONY: compose-down
compose-down:
	docker compose down

.PHONY: dev-backend
dev-backend:
	cd server && CGO_ENABLED=0 go run ./cmd/main.go

.PHONY: dev-frontend
dev-frontend:
	cd web && npm run dev -- --host 127.0.0.1 --port 5174

.PHONY: test
test:
	cd server && go test ./...
	cd web && npm run build

.PHONY: migrate-up
migrate-up:
	cd server && $(MAKE) migrate-up
