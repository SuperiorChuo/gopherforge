SHELL := /bin/sh

# 根 Makefile 仅转发到微服务项目。单体见 monolith/（阶段二）。
MICRO := microservices

.PHONY: help
help:
	@echo "本仓库含两个独立产品线（互不调用业务）："
	@echo "  microservices/  微服务版（当前可运行）"
	@echo "  monolith/       单体版（规划中）"
	@echo ""
	@echo "根目录快捷命令（均进入 microservices/）："
	@echo "  make compose-up | compose-down | test | smoke-api | migrate-up | api-contract"
	@echo "  更多：cd microservices && make help"

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
	@$(MAKE) -C $(MICRO)/legacy-backend $@
