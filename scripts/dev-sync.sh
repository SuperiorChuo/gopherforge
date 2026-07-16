#!/usr/bin/env bash
# 本地 monorepo → 内网开发服务器自动同步（参考 kingYM / go-scaffold）
#
# 用法：
#   ./scripts/dev-sync.sh          # 常驻：每 2 秒增量推送
#   ./scripts/dev-sync.sh once     # 只同步一次
#
# 服务器布局：
#   /www/go-admin-kit/src   ← 本脚本推送的 monorepo
#   /www/go-admin-kit/stack ← 旧版 docker 栈（本脚本不碰）
#
# 热重载：
#   - monolith API：systemd gak-mono-api-dev（air）
#   - micro web：  systemd gak-ms-web-dev（vite HMR）
#   - 全量微服务：见 docs/remote-dev.md（docker compose）

set -euo pipefail

REMOTE="${REMOTE:-root@192.168.220.109}"
REMOTE_DIR="${REMOTE_DIR:-/www/go-admin-kit/src}"
LOCAL_DIR="$(cd "$(dirname "$0")/.." && pwd)"
INTERVAL="${INTERVAL:-2}"
SSH_KEY="${SSH_KEY:-$HOME/.ssh/id_ed25519}"

LOCK_DIR="/tmp/go-admin-kit-devsync.lock"
acquire_daemon_lock() {
  while ! mkdir "$LOCK_DIR" 2>/dev/null; do
    local owner
    owner=$(cat "$LOCK_DIR/pid" 2>/dev/null || true)
    if [[ -n "$owner" ]] && kill -0 "$owner" 2>/dev/null; then
      exit 0
    fi
    rm -rf "$LOCK_DIR"
  done
  echo $$ > "$LOCK_DIR/pid"
  trap 'rm -rf "$LOCK_DIR"' EXIT
}
if [[ "${1:-}" != "once" ]]; then
  acquire_daemon_lock
fi

EXCLUDES=(
  --exclude .git
  --exclude .DS_Store
  --exclude .claude
  --exclude '.codex*'
  --exclude node_modules
  --exclude .pnpm-store
  --exclude dist
  --exclude tmp
  --exclude .cache
  --exclude logs
  --exclude uploads
  --exclude .env
  --exclude .env.local
  --exclude '**/.env'
  --exclude '**/configs/config.yaml'
  --exclude coverage.out
  --exclude tsconfig.tsbuildinfo
  --exclude bin
  --exclude '*.exe'
  --exclude go.work.sum
)

RSYNC_SSH="ssh -i $SSH_KEY -o IdentitiesOnly=yes -o ConnectTimeout=8 -o ServerAliveInterval=10 -o ServerAliveCountMax=2"

sync_once() {
  rsync -az --delete --timeout=30 -e "$RSYNC_SSH" "${EXCLUDES[@]}" \
    "$LOCAL_DIR/" "$REMOTE:$REMOTE_DIR/"
}

echo "[dev-sync] $LOCAL_DIR -> $REMOTE:$REMOTE_DIR"
sync_once
echo "[dev-sync] 初始同步完成 $(date '+%H:%M:%S')"

if [[ "${1:-}" == "once" ]]; then
  exit 0
fi

echo "[dev-sync] 监听模式（每 ${INTERVAL}s），Ctrl+C 退出"
fail_count=0
while true; do
  if [[ $fail_count -ge 5 ]]; then
    sleep 30
  else
    sleep "$INTERVAL"
  fi
  if output=$(rsync -azi --delete --timeout=8 -e "$RSYNC_SSH" "${EXCLUDES[@]}" \
    "$LOCAL_DIR/" "$REMOTE:$REMOTE_DIR/" 2>&1); then
    if [[ $fail_count -ge 5 ]]; then
      echo "[dev-sync] $(date '+%H:%M:%S') 网络恢复"
    fi
    fail_count=0
    if [[ -n "$output" ]]; then
      echo "[dev-sync] $(date '+%H:%M:%S') 已推送:"
      echo "$output" | head -8 | sed 's/^/  /'
    fi
  else
    fail_count=$((fail_count + 1))
    if [[ $fail_count -eq 5 ]]; then
      echo "[dev-sync] $(date '+%H:%M:%S') 连续失败，可能不在内网，改为 30s 重试"
    fi
  fi
done
