#!/usr/bin/env bash
# 本地一键热更新部署到内网开发服务器（deploy-host.example）。
# 把「同步源码 → 远程构建 → 滚动更新 → 验证」串成一条命令。
#
# 用法（本机项目根目录）：
#   ./scripts/deploy.sh                      # 全量：重建 16 后端 + frontend（滚动零停机）
#   ./scripts/deploy.sh frontend             # 只热更前端（最常用）
#   ./scripts/deploy.sh auth-service im-service   # 只滚指定后端（可多个）
#   ./scripts/deploy.sh gateway              # 重建网关（compose 管，不在 swarm 里）
#   ./scripts/deploy.sh --no-verify auth-service  # 跳过部署后冒烟
#
# 服务名可省略 -service 后缀：`auth` 等价于 `auth-service`。
# 应用服务走 swarm（docker-stack.yml，滚动零停机）；gateway/prometheus/grafana
# 走 compose（基础设施，极少改）。两类可混传，脚本自动分流。
#
# 环境变量覆盖：REMOTE / SSH_KEY / REMOTE_SRC / GATEWAY_URL
set -euo pipefail

REMOTE="${REMOTE:-deploy-user@deploy-host.example}"
SSH_KEY="${SSH_KEY:-$HOME/.ssh/id_ed25519}"
REMOTE_SRC="${REMOTE_SRC:-/opt/go-admin-kit/src}"
GATEWAY_URL="${GATEWAY_URL:-http://127.0.0.1:18100}"
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
SSH=(ssh -i "$SSH_KEY" -o IdentitiesOnly=yes -o BatchMode=yes -o ConnectTimeout=15)

# compose 管的基础设施服务（不在 swarm 应用栈里）
COMPOSE_SVCS=" gateway prometheus grafana "

VERIFY=1
ARGS=()
for a in "$@"; do
  case "$a" in
    --no-verify) VERIFY=0 ;;
    -h|--help) sed -n '2,17p' "$0"; exit 0 ;;
    *) ARGS+=("$a") ;;
  esac
done

# 规范化服务名：补 -service 后缀（frontend / gateway / prometheus / grafana 除外）
normalize() {
  local s="$1"
  case "$s" in
    frontend|gateway|prometheus|grafana) echo "$s" ;;
    *-service) echo "$s" ;;
    *) echo "${s}-service" ;;
  esac
}

SWARM_SVCS=()
COMPOSE_TARGETS=()
for raw in "${ARGS[@]:-}"; do
  [ -z "$raw" ] && continue
  svc="$(normalize "$raw")"
  if [[ "$COMPOSE_SVCS" == *" $svc "* ]]; then
    COMPOSE_TARGETS+=("$svc")
  else
    SWARM_SVCS+=("$svc")
  fi
done

echo "[deploy] 1/3 同步源码 → $REMOTE:$REMOTE_SRC"
# 公开模板：先将源码同步到部署服务器（rsync / CI 制品均可）

# ---- 应用服务：走 swarm-redeploy（无参数=全量）----
if [ ${#ARGS[@]} -eq 0 ]; then
  echo "[deploy] 2/3 全量重建应用栈（swarm 滚动零停机，冷编译约 5-10 分钟）"
  "${SSH[@]}" "$REMOTE" "bash $REMOTE_SRC/scripts/swarm-redeploy.sh"
elif [ ${#SWARM_SVCS[@]} -gt 0 ]; then
  echo "[deploy] 2/3 滚动更新 swarm 服务：${SWARM_SVCS[*]}"
  "${SSH[@]}" "$REMOTE" "bash $REMOTE_SRC/scripts/swarm-redeploy.sh ${SWARM_SVCS[*]}"
else
  echo "[deploy] 2/3 无 swarm 服务需更新，跳过"
fi

# ---- 基础设施：走 compose（gateway 等）----
if [ ${#COMPOSE_TARGETS[@]} -gt 0 ]; then
  echo "[deploy] 2b/3 compose 重建基础设施：${COMPOSE_TARGETS[*]}"
  "${SSH[@]}" "$REMOTE" \
    "install -d -m 0700 -o 10001 -g 10001 /opt/go-admin-kit/certs /opt/go-admin-kit/certs/edge /opt/go-admin-kit/certs/traefik-dynamic && cd $REMOTE_SRC/microservices && EDGE_CERT_HOST_DIR=/opt/go-admin-kit/certs COMPOSE_PROJECT_NAME=go-admin-kit docker compose --profile compose-fallback up -d --build ${COMPOSE_TARGETS[*]}"
fi

# ---- 验证 ----
if [ "$VERIFY" = 1 ]; then
  echo "[deploy] 3/3 验证"
  "${SSH[@]}" "$REMOTE" bash -s <<REMOTE_EOF
set -e
# 收敛核对：swarm 服务副本是否全部就绪
NOT_READY=\$(docker stack services gak --format '{{.Name}} {{.Replicas}}' \
  | awk '{split(\$2,a,"/"); if(a[1]!=a[2]) print \$0}')
if [ -n "\$NOT_READY" ]; then
  echo "  ⚠ 未就绪的服务："; echo "\$NOT_READY"
else
  echo "  ✓ swarm 全部服务副本就绪"
fi
# 网关入口
code=\$(curl -s -m 8 -o /dev/null -w '%{http_code}' $GATEWAY_URL/api/v1/captcha || echo 000)
echo "  网关 captcha = \$code \$([ "\$code" = 200 ] && echo ✓ || echo '⚠ 检查 gateway')"
REMOTE_EOF
else
  echo "[deploy] 3/3 跳过验证（--no-verify）"
fi

echo "[deploy] 完成。"
echo "  访问： http://deploy-host.example:18100  （网关/前端）"
echo "  冒烟： cd microservices && API_BASE_URL=http://deploy-host.example:18100/api/v1 npm run smoke:api"
