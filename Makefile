SHELL := /bin/sh

# 根 Makefile：默认转发微服务；单体用 make mono-*
MICRO := microservices
MONO := monolith

.PHONY: help
help:
	@echo "本仓库含两个独立产品线（互不调用业务）："
	@echo "  microservices/  微服务版"
	@echo "  monolith/       单体版"
	@echo ""
	@echo "微服务：make compose-up | compose-down | test | smoke-api | ..."
	@echo "单体：  make mono-up | mono-down | mono-test"
	@echo "详情：cd microservices && make help  /  cd monolith && make help"

.PHONY: mono-up mono-down mono-test
mono-up:
	@$(MAKE) -C $(MONO) compose-up
mono-down:
	@$(MAKE) -C $(MONO) compose-down
mono-test:
	@$(MAKE) -C $(MONO) test

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
