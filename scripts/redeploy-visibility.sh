#!/usr/bin/env bash
# 热更新 visibility 相关 Swarm 服务到内网开发服务器（约定流程）。
# 用法：./scripts/redeploy-visibility.sh [service...]
#   默认重建 visibility-service frontend；也可只传其一。
# 流程：同步源码 + 远端 swarm-redeploy 滚动更新 + 健康检查。
set -euo pipefail

REMOTE="${REMOTE:-deploy-user@deploy-host.example}"
SSH_KEY="${SSH_KEY:-$HOME/.ssh/id_ed25519}"
REMOTE_SRC="${REMOTE_SRC:-/opt/go-admin-kit/src}"
SSH=(ssh -i "$SSH_KEY" -o IdentitiesOnly=yes -o BatchMode=yes -o ConnectTimeout=15)
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
SERVICES=("$@")
[ ${#SERVICES[@]} -gt 0 ] || SERVICES=(visibility-service frontend)

echo "[redeploy-vis] 1/3 同步源码"
# 公开模板：先将源码同步到部署服务器（rsync / CI 制品均可）

echo "[redeploy-vis] 2/3 Swarm 滚动更新：${SERVICES[*]}"
"${SSH[@]}" "$REMOTE" bash "$REMOTE_SRC/scripts/swarm-redeploy.sh" "${SERVICES[@]}"

echo "[redeploy-vis] 3/3 等待健康检查"
for i in $(seq 1 30); do
  if curl -sf --max-time 3 "http://deploy-host.example:18100/api/v1/visibility/health/ready" >/dev/null 2>&1; then
    echo "[redeploy-vis] ready ✔"
    exit 0
  fi
  sleep 2
done
echo "[redeploy-vis] 健康检查超时" >&2
exit 1
