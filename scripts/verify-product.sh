#!/usr/bin/env bash
# 产品源码验证入口：仅依赖仓库产品文件与标准 Go/Node/Python 工具链。
# all 不启动容器；compose 只解析示例配置，部署/数据库演练另行执行。
set -euo pipefail
ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
TARGET="${1:-all}"

run_go() {
  local path="$1"
  echo "== ${path}: build / vet / test -race =="
  (cd "$ROOT/$path" && go build ./... && go vet ./... && go test -race ./...)
}

release_tools() {
  bash -n "$ROOT/scripts/verify-product.sh"
  python3 -B -m unittest discover -s "$ROOT/scripts/tests" -p 'test_release_*.py' -v
  # The snapshot exporter is source-only; the public snapshot ships generated SQL.
  if [[ -f "$ROOT/scripts/release-product-migrations.py" ]]; then
    python3 -B -m unittest discover -s "$ROOT/scripts/tests" -p 'test_product_migration_export.py' -v
  fi
}

all_go() {
  local modules module
  # 先取得清单再循环：进程替换中的解析失败不能被 while 吞掉。
  modules="$(python3 -B "$ROOT/scripts/release-workspace.py" --workspace "$ROOT/microservices" --lines)"
  while IFS= read -r module; do
    (export GOWORK="$ROOT/microservices/go.work"; run_go "microservices/$module")
  done <<< "$modules"
  # 附属模块不在 workspace 中，显式关闭 workspace 防止误选父目录模块。
  for module in freeswitch-cc/control-api clients/hermes-manager; do
    (export GOWORK=off; run_go "$module")
  done
}

frontend() {
  (cd "$ROOT/microservices/web" && npm run lint:i18n && npm run lint && npm test && npm run build)
}

contract() {
  (
    cd "$ROOT/microservices"
    npm run api:contract
    # 对暂存版本核对，兼容尚未提交的开发批次；生成物漂移必须失败。
    git diff --exit-code -- services/monitor/docs/openapi.json web/src/api/generated/schema.d.ts
    npm run test:contract
    npm run test:codegen:contract
    npm run test:smoke:unit
    npm run test:frontend-runtime-config
    node --test tests/monitoring-config.test.mjs
    npm run test:cc-wss
    # 缺工具不能伪装成契约通过；breaking 以当前已提交 proto 为基线。
    cd services/api/proto
    buf lint
    buf breaking --against "$ROOT/.git#ref=HEAD,subdir=microservices/services/api/proto"
  )
}

case "$TARGET" in
  release-tools) release_tools ;;
  # 私有开发历史与公开历史不同；此目标只在公开候选执行，不混入源码 all。
  public-migrations) python3 -B "$ROOT/scripts/release-migrations.py" ;;
  public-hygiene) python3 -B "$ROOT/scripts/release-hygiene.py" ;;
  all-go) all_go ;;
  frontend) frontend ;;
  contract) contract ;;
  compose) (cd "$ROOT/microservices" && npm run test:compose:config) ;;
  all) release_tools; all_go; frontend; contract ;;
  -h|--help)
    echo '用法: bash scripts/verify-product.sh [release-tools|public-migrations|public-hygiene|all-go|frontend|contract|compose|all]'
    echo 'all 不启动 Docker，也不替代远程部署与带数据升级验收。'
    exit 0
    ;;
  *) echo "未知产品验证目标: $TARGET" >&2; exit 2 ;;
esac
printf '✓ product %s 全绿\n' "$TARGET"
