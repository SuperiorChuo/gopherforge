#!/usr/bin/env bash
# 检查业务域代码是否落在稳定的服务/技术层/域目录中。
# 该脚本只做路径守卫，不读取运行配置，也不启动 Docker。
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
SERVICES_DIR="$ROOT_DIR/microservices/services"
WEB_DIR="$ROOT_DIR/microservices/web/src"
errors=0

report() {
  printf '布局错误：%s\n' "$*" >&2
  errors=$((errors + 1))
}

check_no_root_go() {
  local dir="$1"
  if [[ ! -d "$dir" ]]; then
    return
  fi
  while IFS= read -r file; do
    report "Go 文件不应继续平铺在 ${dir#"$ROOT_DIR"}/：${file#"$ROOT_DIR"/}"
  done < <(find "$dir" -maxdepth 1 -type f -name '*.go' -print | sort)
}

# 这些业务服务的 API/Store 已按服务域下沉；保留完整 package，避免跨 package
# 改造未导出符号。服务在下游脚手架不存在时跳过，不把业务服务引入下游。
for service in ai cc crm im mp notify pay ticket visibility; do
  service_dir="$SERVICES_DIR/$service"
  [[ -d "$service_dir" ]] || continue
  case "$service" in
    ai)
      check_no_root_go "$service_dir/internal/service"
      [[ -d "$service_dir/internal/service/hermes" ]] || report "$service_dir/internal/service/hermes/ 缺失"
      ;;
    *)
      check_no_root_go "$service_dir/internal/api"
      check_no_root_go "$service_dir/internal/store"
      [[ -d "$service_dir/internal/api/$service" ]] || report "$service_dir/internal/api/$service/ 缺失"
      [[ -d "$service_dir/internal/store/$service" ]] || report "$service_dir/internal/store/$service/ 缺失"
      ;;
  esac
done

# 旧 import 路径一旦重新出现，说明新代码绕过了域目录守卫。
if [[ -d "$SERVICES_DIR" ]] && rg -n 'github\.com/go-admin-kit/services/(cc|crm|im|mp|notify|pay|ticket|visibility)/internal/(api|store)"' "$SERVICES_DIR" --glob '*.go'; then
  report '发现已下沉业务包的旧 Go import 路径'
fi
if [[ -d "$SERVICES_DIR/ai" ]] && rg -n 'github\.com/go-admin-kit/services/ai/internal/service"' "$SERVICES_DIR/ai" --glob '*.go'; then
  report '发现 AI service 根包的旧 Go import 路径'
fi

# 前端域 API 不得回到 src/api 根目录；index.ts 是公共聚合入口例外。
if [[ -d "$WEB_DIR/api" ]]; then
  while IFS= read -r file; do
    base="${file##*/}"
    [[ "$base" == 'index.ts' ]] && continue
    report "前端业务 API 不应平铺在 ${WEB_DIR#"$ROOT_DIR"}/api/：${file#"$ROOT_DIR"/}"
  done < <(find "$WEB_DIR/api" -maxdepth 1 -type f \( -name '*.ts' -o -name '*.tsx' \) -print | sort)
fi

if (( errors > 0 )); then
  printf '代码目录守卫失败：%d 项\n' "$errors" >&2
  exit 1
fi
printf '代码目录守卫通过\n'
