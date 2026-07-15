SHELL := /bin/sh

# 根 Makefile：微服务 / 单体 / 呼叫媒体
MICRO := microservices
MONO := monolith

.PHONY: help
help:
	@echo "本仓库 monorepo 产品线（一人维护）："
	@echo "  microservices/   微服务中台"
	@echo "  monolith/        单体后台"
	@echo "  freeswitch-cc/   呼叫媒体（FS，可被中台控制）"
	@echo ""
	@echo "微服务：make compose-up | compose-down | test | smoke-api | ..."
	@echo "单体：  make mono-up | mono-down | mono-test"
	@echo "呼叫：  make fs-up | fs-down"
	@echo "详情：cd <目录> && 查看各 README"

.PHONY: mono-up mono-down mono-test fs-up fs-down
mono-up:
	@$(MAKE) -C $(MONO) compose-up
mono-down:
	@$(MAKE) -C $(MONO) compose-down
mono-test:
	@$(MAKE) -C $(MONO) test

fs-up:
	cd freeswitch-cc && docker compose up -d --build
fs-down:
	cd freeswitch-cc && docker compose down

.PHONY: compose-up compose-down compose-monitoring dev-backend dev-auth dev-frontend \
	build-server test lint audit smoke-api e2e-api db-import migrate-up migrate-status \
	migrate-down migrate-redo migrate-reset migrate-create api-contract status logs

compose-up compose-down compose-monitoring dev-backend dev-auth dev-frontend \
build-server test lint smoke-api db-import migrate-up migrate-status \
migrate-down migrate-create api-contract status logs:
	@$(MAKE) -C $(MICRO) $@

audit:
	cd $(MICRO)/web && npm audit --omit=dev

e2e-api: smoke-api

migrate-redo migrate-reset:
	@$(MAKE) -C $(MICRO)/services/monitor $@
