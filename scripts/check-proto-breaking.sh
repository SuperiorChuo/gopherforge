#!/usr/bin/env bash
# 从仓库根读取明确的事件基线，避免在 proto 子目录找 .git 或误与新 HEAD 自比。
set -euo pipefail
ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
BASE="${1:-}"
if [[ ! "$BASE" =~ ^[0-9a-f]{40}$ ]] || [[ "$BASE" == 0000000000000000000000000000000000000000 ]] ||
  ! git -C "$ROOT" cat-file -e "${BASE}^{commit}" 2>/dev/null; then
  echo 'invalid proto base commit: supply an existing full commit SHA' >&2
  exit 2
fi
# 只导出基线 proto，不克隆整个产品历史；普通仓库和 linked worktree 均由 Git 解析。
PROTO='microservices/services/api/proto'
BASE_DIR="$(mktemp -d "${TMPDIR:-/tmp}/proto-baseline.XXXXXX")"
trap 'rm -rf -- "$BASE_DIR"' EXIT
git -C "$ROOT" archive "$BASE:$PROTO" | tar -x -C "$BASE_DIR"
buf breaking "$ROOT/$PROTO" --against "$BASE_DIR"
