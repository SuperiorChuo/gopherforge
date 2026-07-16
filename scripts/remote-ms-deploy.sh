#!/usr/bin/env bash
# 用 monorepo microservices 替换服务器上旧 go-admin-kit Docker 栈
# - 不部署单体
# - 保留 postgres/redis 数据卷（compose down 不加 -v）
# - 端口与旧 stack 对齐（18100 网关等）
#
# 用法（本机）：
#   ./scripts/remote-ms-deploy.sh
set -euo pipefail

REMOTE="${REMOTE:-root@192.168.220.109}"
SSH_KEY="${SSH_KEY:-$HOME/.ssh/id_ed25519}"
SSH=(ssh -i "$SSH_KEY" -o IdentitiesOnly=yes -o BatchMode=yes -o ConnectTimeout=15)
ROOT="$(cd "$(dirname "$0")/.." && pwd)"

echo "[remote-ms] 1/5 同步 monorepo → /www/go-admin-kit/src"
"$ROOT/scripts/dev-sync.sh" once

echo "[remote-ms] 2/5 停用单体热更服务（按你的要求不部署单体）"
"${SSH[@]}" "$REMOTE" 'systemctl disable --now gak-mono-api-dev 2>/dev/null || true'

echo "[remote-ms] 3/5 停止旧 stack 容器（保留 volume）"
"${SSH[@]}" "$REMOTE" bash -s <<'REMOTE'
set -euo pipefail
if [[ -f /www/go-admin-kit/stack/docker-compose.yml ]]; then
  cd /www/go-admin-kit/stack
  docker compose down --remove-orphans || true
fi
# 清掉同名残留容器（若有）
for c in go-admin-kit-backend go-admin-kit-gateway go-admin-kit-frontend \
  go-admin-kit-auth go-admin-kit-identity go-admin-kit-system go-admin-kit-audit \
  go-admin-kit-file go-admin-kit-ai go-admin-kit-im go-admin-kit-monitor \
  go-admin-kit-postgres go-admin-kit-redis go-admin-kit-nats go-admin-kit-minio \
  go-admin-kit-prometheus go-admin-kit-grafana go-admin-kit-otel-collector go-admin-kit-jaeger; do
  docker rm -f "$c" 2>/dev/null || true
done
REMOTE

echo "[remote-ms] 4/5 生成 microservices/.env 并对齐旧端口"
"${SSH[@]}" "$REMOTE" bash -s <<'REMOTE'
set -euo pipefail
SRC=/www/go-admin-kit/src
MS=$SRC/microservices
mkdir -p "$MS"

# 从旧 stack .env 继承密钥与端口
STACK_ENV=/www/go-admin-kit/stack/.env
if [[ -f "$STACK_ENV" ]]; then
  set -a
  # shellcheck disable=SC1091
  source "$STACK_ENV"
  set +a
fi

# minio 9000/9001 被 sku 占用，映射到 19000/19001
cat > "$MS/.env" <<EOF
TZ=${TZ:-Asia/Shanghai}
APP_ENV=${APP_ENV:-development}

# Host ports（与旧 stack 一致，避免改浏览器书签）
GATEWAY_PORT=${GATEWAY_PORT:-18100}
FRONTEND_PORT=${FRONTEND_PORT:-13100}
BACKEND_PORT=${BACKEND_PORT:-18081}
AUTH_SERVICE_PORT=${AUTH_SERVICE_PORT:-18082}
IDENTITY_SERVICE_PORT=${IDENTITY_SERVICE_PORT:-18083}
SYSTEM_SERVICE_PORT=${SYSTEM_SERVICE_PORT:-18084}
AUDIT_SERVICE_PORT=${AUDIT_SERVICE_PORT:-18085}
FILE_SERVICE_PORT=${FILE_SERVICE_PORT:-18086}
AI_SERVICE_PORT=${AI_SERVICE_PORT:-18087}
IM_SERVICE_PORT=${IM_SERVICE_PORT:-18088}
NATS_PORT=${NATS_PORT:-14222}
POSTGRES_PORT=${POSTGRES_PORT:-15434}
REDIS_PORT=${REDIS_PORT:-16380}
MINIO_API_PORT=${MINIO_API_PORT:-19000}
MINIO_CONSOLE_PORT=${MINIO_CONSOLE_PORT:-19001}

POSTGRES_DB=${POSTGRES_DB:-go_admin_kit}
POSTGRES_USER=${POSTGRES_USER:-go_admin_kit}
POSTGRES_PASSWORD=${POSTGRES_PASSWORD:-}
DB_SSLMODE=${DB_SSLMODE:-disable}

REDIS_PASSWORD=${REDIS_PASSWORD:-}
REDIS_DB=${REDIS_DB:-0}

JWT_SECRET=${JWT_SECRET:-}
JWT_REFRESH_TOKEN_ROTATION=${JWT_REFRESH_TOKEN_ROTATION:-true}

# 远程 vite HMR + 静态前端 + 本机
CORS_ALLOW_ORIGINS=http://192.168.220.109:13200,http://192.168.220.109:13100,http://192.168.220.109:18100,http://localhost:13200,http://127.0.0.1:13200
CORS_ALLOW_CREDENTIALS=true

DEFAULT_ADMIN_WARN_DEFAULT_PASSWORD=true
DEFAULT_ADMIN_FORCE_CHANGE_PASSWORD=false
DEFAULT_ADMIN_USERNAME=admin

UPLOAD_STORAGE_TYPE=local
UPLOAD_LOCAL_PATH=./uploads
UPLOAD_PUBLIC_BASE_URL=/uploads
UPLOAD_LOCAL_URL_PREFIX=/uploads

# AI 可选：服务器上若有 key 可写进 stack .env 再部署
AI_PROVIDER=${AI_PROVIDER:-}
AI_BASE_URL=${AI_BASE_URL:-}
AI_API_KEY=${AI_API_KEY:-}
AI_CHAT_MODEL=${AI_CHAT_MODEL:-}
AI_EMBED_MODEL=${AI_EMBED_MODEL:-}
EOF

# 校验必需项
if [[ -z "${POSTGRES_PASSWORD:-}" || -z "${JWT_SECRET:-}" ]]; then
  echo "[remote-ms] 警告：POSTGRES_PASSWORD 或 JWT_SECRET 为空，请检查 $STACK_ENV"
fi

# platform 路径：compose 里 otel 用 ../platform/deploy
test -d "$SRC/platform/deploy" || { echo "缺少 platform/deploy"; exit 1; }

echo "[remote-ms] 5/5 docker compose up --build（保留 volume 名 go-admin-kit_*）"
cd "$MS"
# 使用与旧栈相同 project name，复用 go-admin-kit_go_admin_kit_postgres_data
export COMPOSE_PROJECT_NAME=go-admin-kit
docker compose pull postgres redis nats 2>/dev/null || true
docker compose up -d --build --remove-orphans

echo "--- containers ---"
docker compose ps
echo "--- health ---"
sleep 8
curl -sS -m 8 -o /dev/null -w "gateway=%{http_code}\n" http://127.0.0.1:${GATEWAY_PORT:-18100}/ || true
curl -sS -m 8 -o /dev/null -w "frontend=%{http_code}\n" http://127.0.0.1:${FRONTEND_PORT:-13100}/ || true
# 可选健康
curl -sS -m 5 "http://127.0.0.1:${GATEWAY_PORT:-18100}/api/v1/health/live" 2>/dev/null | head -c 200 || true
echo
REMOTE

echo "[remote-ms] 更新 vite 代理仍指向网关 18100"
"${SSH[@]}" "$REMOTE" 'systemctl restart gak-ms-web-dev 2>/dev/null || true; systemctl is-active gak-ms-web-dev 2>/dev/null || true'

echo
echo "完成。访问："
echo "  网关/API:  http://192.168.220.109:18100"
echo "  静态前端:  http://192.168.220.109:13100"
echo "  Vite HMR:  http://192.168.220.109:13200  （改前端热更新）"
echo "  本机同步:  ./scripts/dev-sync.sh"
echo "  改后端后重建: ./scripts/remote-ms-deploy.sh   # 或 ssh 后 docker compose up -d --build <service>"
