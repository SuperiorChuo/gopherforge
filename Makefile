SHELL := /bin/sh

# 根 Makefile：微服务脚手架
MICRO := microservices

.PHONY: help
help:
	@echo "Go Admin Kit 微服务脚手架（一人维护）："
	@echo "  microservices/   微服务中台脚手架"
	@echo ""
	@echo "微服务：make compose-up | compose-down | test | smoke-api | ..."
	@echo "详情：cd microservices && 查看 README"

.PHONY: compose-up compose-down compose-monitoring infra-up infra-down dev-backend dev-auth dev-frontend \
	build-server test lint audit smoke-api e2e-api db-import migrate-up migrate-status \
	migrate-down migrate-redo migrate-reset migrate-create api-contract status logs

compose-up compose-down compose-monitoring infra-up infra-down dev-backend dev-auth dev-frontend \
build-server test lint smoke-api db-import migrate-up migrate-status \
migrate-down migrate-create api-contract status logs:
	@$(MAKE) -C $(MICRO) $@

audit:
	cd $(MICRO)/web && npm audit --omit=dev

e2e-api: smoke-api

migrate-redo migrate-reset:
	@$(MAKE) -C $(MICRO)/services/monitor $@

# 本机远程开发工作流（remote-sync / remote-ms-deploy / remote-logs 等）
# 放 Makefile.local，不入库；存在时自动加载。
-include Makefile.local
