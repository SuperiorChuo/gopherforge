#!/usr/bin/env bash
# 用被扫描模块实际选择的 Go 工具链构建扫描器，不使用 PATH 中的旧二进制。
set -euo pipefail

if [[ $# -lt 1 ]]; then
  printf '用法: bash scripts/govulncheck.sh <模块目录> [govulncheck 参数，默认 ./...]\n' >&2
  exit 2
fi
cd -- "$1"
shift
[[ -f go.mod ]] || { printf '扫描目录缺少 go.mod\n' >&2; exit 2; }

# 显式按模块扫描，避免父级 workspace 排除该模块或改写它的依赖选择。
export GOWORK=off
# 先在目标模块内解析 auto/toolchain，再固定给工具构建及扫描子进程。
toolchain="$(go env GOVERSION)"
tool_dir="$(mktemp -d "${TMPDIR:-/tmp}/govulncheck.XXXXXX")"
trap 'rm -rf "$tool_dir"' EXIT
printf 'govulncheck v1.6.0 / %s\n' "$toolchain" >&2
# 分析器版本固定，漏洞数据库仍在每次扫描时按 govulncheck 默认机制更新。
GOTOOLCHAIN="$toolchain" GOBIN="$tool_dir" go install golang.org/x/vuln/cmd/govulncheck@v1.6.0
if [[ $# -eq 0 ]]; then
  set -- ./...
fi
# 不用 go run：保留扫描器原始退出码（包括发现漏洞时的 3），并在退出后清理。
GOTOOLCHAIN="$toolchain" "$tool_dir/govulncheck" "$@"
