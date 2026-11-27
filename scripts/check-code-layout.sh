#!/usr/bin/env bash
# 检查业务域代码是否落在稳定的服务/技术层/域目录中。
# 该脚本只做路径守卫，不读取运行配置，也不启动 Docker。
set -euo pipefail

for tool in dirname find sort rg; do
  command -v "$tool" >/dev/null 2>&1 || { printf '目录守卫缺少必需命令：%s\n' "$tool" >&2; exit 2; }
done
SCRIPT_DIR="$(dirname "${BASH_SOURCE[0]}")" || exit 2
ROOT_DIR="$(cd "$SCRIPT_DIR/.." && pwd)" || exit 2
SERVICES_DIR="$ROOT_DIR/microservices/services"
WEB_DIR="$ROOT_DIR/microservices/web/src"
errors=0

# 命令替换由调用方检查状态，避免进程替换吞掉扫描/排序错误。
scan_files() {
  if ! find "$@" | sort; then
    printf '目录守卫扫描失败：%s\n' "$1" >&2
    return 2
  fi
}

check_imports() {
  local message="$1" status
  shift
  if rg -n "$@" --glob '*.go'; then
    report "$message"
  else
    status=$?
    if [[ "$status" -ne 1 ]]; then
      printf '目录守卫 rg 执行失败（退出码 %s）\n' "$status" >&2
      exit 2
    fi
  fi
}

report() {
  printf '布局错误：%s\n' "$*" >&2
  errors=$((errors + 1))
}

check_no_root_go() {
  local dir="$1" files file
  if [[ ! -d "$dir" ]]; then
    return
  fi
  files="$(scan_files "$dir" -maxdepth 1 -type f -name '*.go' -print)" || exit 2
  while IFS= read -r file; do
    [[ -n "$file" ]] || continue
    report "Go 文件不应继续平铺在 ${dir#"$ROOT_DIR"}/：${file#"$ROOT_DIR"/}"
  done <<< "$files"
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
if [[ -d "$SERVICES_DIR" ]]; then
  check_imports '发现已下沉业务包的旧 Go import 路径' 'github\.com/go-admin-kit/services/(cc|crm|im|mp|notify|pay|ticket|visibility)/internal/(api|store)"' "$SERVICES_DIR"
fi
if [[ -d "$SERVICES_DIR/ai" ]]; then
  check_imports '发现 AI service 根包的旧 Go import 路径' 'github\.com/go-admin-kit/services/ai/internal/service"' "$SERVICES_DIR/ai"
fi

# 前端域 API 不得回到 src/api 根目录；index.ts 是公共聚合入口例外。
if [[ -d "$WEB_DIR/api" ]]; then
  files="$(scan_files "$WEB_DIR/api" -maxdepth 1 -type f \( -name '*.ts' -o -name '*.tsx' \) -print)" || exit 2
  while IFS= read -r file; do
    [[ -n "$file" ]] || continue
    base="${file##*/}"
    [[ "$base" == 'index.ts' ]] && continue
    report "前端业务 API 不应平铺在 ${WEB_DIR#"$ROOT_DIR"}/api/：${file#"$ROOT_DIR"/}"
  done <<< "$files"
fi

if (( errors > 0 )); then
  printf '代码目录守卫失败：%d 项\n' "$errors" >&2
  exit 1
fi
printf '代码目录守卫通过\n'
