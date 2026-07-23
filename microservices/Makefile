SHELL := /bin/sh
MICRO_ROOT := $(dir $(abspath $(lastword $(MAKEFILE_LIST))))

.PHONY: help
help:
	@echo "微服务项目 (microservices/) 可用目标:"
	@echo "  compose-up        启动完整微服务栈（自动先起 infra 数据栈）"
	@echo "  compose-down      停止应用栈（不碰数据栈）"
	@echo "  infra-up          单独启动数据栈（PG/Redis/NATS）"
	@echo "  infra-down        停止数据栈"
	@echo "  compose-monitoring 启动含监控 profile 的栈"
	@echo "  dev-backend       本地启动 services/monitor（监控兜底）"
	@echo "  dev-auth          本地启动 auth-service"
	@echo "  dev-frontend      本地启动 React 前端"
	@echo "  test              后端与脚本测试"
	@echo "  smoke-api         API 冒烟（需已启动网关）"
	@echo "  migrate-up        应用数据库迁移"
	@echo "  api-contract      生成 OpenAPI 与类型"

# 有状态服务独立栈（PG/Redis/NATS/MinIO 见 docker-compose.infra.yml），
# 应用栈重建不触碰数据；两栈经外部网络互通，操作 infra 栈必须显式 -p
.PHONY: infra-up
infra-up:
	docker network inspect go-admin-kit-net >/dev/null 2>&1 || \
		docker network create --subnet 172.28.0.0/16 go-admin-kit-net
	docker compose -p go-admin-kit-infra -f docker-compose.infra.yml up -d

.PHONY: infra-down
infra-down:
	docker compose -p go-admin-kit-infra -f docker-compose.infra.yml down

.PHONY: compose-up
compose-up: infra-up
	docker compose up -d --build

.PHONY: compose-down
compose-down:
	docker compose down

.PHONY: compose-monitoring
compose-monitoring: infra-up
	docker compose --profile monitoring up -d --build

.PHONY: dev-backend
dev-backend:
	cd services/monitor && CGO_ENABLED=0 go run ./cmd/main.go

.PHONY: dev-auth
dev-auth:
	cd services/auth && CGO_ENABLED=0 go run ./cmd

.PHONY: build-server
build-server:
	cd services/monitor && $(MAKE) build-server

.PHONY: dev-frontend
dev-frontend:
	cd web && npm run dev -- --host 127.0.0.1 --port 5174

.PHONY: test
test:
	npm run test:smoke:unit
	cd services/monitor && go test ./...
	cd services/auth && go test ./...
	cd web && npm run build

.PHONY: lint
lint:
	cd services/monitor && go vet ./...
	cd services/auth && go vet ./...
	cd web && npm run lint

.PHONY: smoke-api
smoke-api:
	npm run smoke:api

.PHONY: db-import
db-import:
	cd services/monitor && $(MAKE) db-import

.PHONY: migrate-up
migrate-up:
	cd services/monitor && $(MAKE) migrate-up

.PHONY: migrate-status
migrate-status:
	cd services/monitor && $(MAKE) migrate-status

.PHONY: migrate-down
migrate-down:
	cd services/monitor && $(MAKE) migrate-down

.PHONY: migrate-create
migrate-create:
	cd services/monitor && $(MAKE) migrate-create MIGRATION_NAME=$(or $(NAME),change_name)

.PHONY: api-contract
api-contract:
	npm run api:contract

.PHONY: status
status:
	@node scripts/dev-status.mjs

.PHONY: logs
logs:
	@node scripts/dev-logs.mjs
