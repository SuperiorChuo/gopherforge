#!/usr/bin/env bash
# 在 192.168.220.109 上一次性安装 go-admin-kit 远程热更环境
# 用法：./scripts/remote-bootstrap.sh
set -euo pipefail

REMOTE="${REMOTE:-root@192.168.220.109}"
SSH_KEY="${SSH_KEY:-$HOME/.ssh/id_ed25519}"
SSH=(ssh -i "$SSH_KEY" -o IdentitiesOnly=yes -o BatchMode=yes -o ConnectTimeout=10)
SCP=(scp -i "$SSH_KEY" -o IdentitiesOnly=yes)
ROOT="$(cd "$(dirname "$0")/.." && pwd)"

echo "[bootstrap] 首次同步 monorepo → /www/go-admin-kit/src"
"$ROOT/scripts/dev-sync.sh" once

echo "[bootstrap] 上传 systemd 单元"
"${SCP[@]}" \
  "$ROOT/deploy/remote/gak-mono-api-dev.service" \
  "$ROOT/deploy/remote/gak-ms-web-dev.service" \
  "$REMOTE:/etc/systemd/system/"

echo "[bootstrap] 远端：依赖、配置、启服务"
"${SSH[@]}" "$REMOTE" bash -s <<'REMOTE'
set -euo pipefail
mkdir -p /www/go-admin-kit/src

# Go / air 已在 kingym 环境装好；再确认
export PATH=/usr/local/go/bin:/root/go/bin:$PATH
export GOPROXY=https://goproxy.cn,direct
command -v go >/dev/null
command -v air >/dev/null || go install github.com/air-verse/air@latest

# 单体 .env.remote（不覆盖已有）
ENVF=/www/go-admin-kit/src/monolith/server/configs/.env.remote
if [[ ! -f "$ENVF" ]]; then
  cat > "$ENVF" <<'EOF'
APP_ENV=development
APP_PORT=18201
# 复用本机 docker postgres/redis（旧 stack 默认端口）
DB_HOST=127.0.0.1
DB_PORT=15434
DB_USER=go_admin_kit
DB_PASSWORD=change-me
DB_NAME=go_admin_kit
DB_SSLMODE=disable
REDIS_HOST=127.0.0.1
REDIS_PORT=16380
REDIS_PASSWORD=
REDIS_DB=1
JWT_SECRET=go-admin-kit-dev-remote-secret-change-me-32b
CORS_ALLOW_ORIGINS=http://192.168.220.109:13200,http://192.168.220.109:13201,http://localhost:13200,http://127.0.0.1:13200
EOF
  # 从 stack .env 抄真实库账号（不入库、不打印）
  if [[ -f /www/go-admin-kit/stack/.env ]]; then
    set -a
    # shellcheck disable=SC1091
    source /www/go-admin-kit/stack/.env 2>/dev/null || true
    set +a
    [[ -n "${POSTGRES_USER:-}" ]] && sed -i "s/^DB_USER=.*/DB_USER=${POSTGRES_USER}/" "$ENVF" || true
    [[ -n "${POSTGRES_PASSWORD:-}" ]] && sed -i "s|^DB_PASSWORD=.*|DB_PASSWORD=${POSTGRES_PASSWORD}|" "$ENVF" || true
    [[ -n "${POSTGRES_DB:-}" ]] && sed -i "s/^DB_NAME=.*/DB_NAME=${POSTGRES_DB}/" "$ENVF" || true
    [[ -n "${POSTGRES_PORT:-}" ]] && sed -i "s/^DB_PORT=.*/DB_PORT=${POSTGRES_PORT}/" "$ENVF" || true
    [[ -n "${REDIS_PASSWORD:-}" ]] && sed -i "s|^REDIS_PASSWORD=.*|REDIS_PASSWORD=${REDIS_PASSWORD}|" "$ENVF" || true
    [[ -n "${REDIS_PORT:-}" ]] && sed -i "s/^REDIS_PORT=.*/REDIS_PORT=${REDIS_PORT}/" "$ENVF" || true
    [[ -n "${JWT_SECRET:-}" ]] && sed -i "s|^JWT_SECRET=.*|JWT_SECRET=${JWT_SECRET}|" "$ENVF" || true
  fi
  echo "[bootstrap] 已写 $ENVF （密钥仅存服务器，勿提交 git）"
fi

# 单体 configs/config.yaml：从 example 生成（若无）
CFG=/www/go-admin-kit/src/monolith/server/configs/config.yaml
if [[ ! -f "$CFG" && -f /www/go-admin-kit/src/monolith/server/configs/config.example.yaml ]]; then
  cp /www/go-admin-kit/src/monolith/server/configs/config.example.yaml "$CFG"
  # 粗略改端口
  sed -i 's/8081/18201/g' "$CFG" || true
fi

# 前端依赖
export PATH=/usr/local/node22/bin:/usr/local/node/bin:$PATH
cd /www/go-admin-kit/src/microservices/web
if command -v pnpm >/dev/null; then
  pnpm install --prefer-offline || pnpm install
else
  npm install -g pnpm
  pnpm install
fi

# 预热 go mod
cd /www/go-admin-kit/src/monolith/server
go mod download || true

systemctl daemon-reload
systemctl enable --now gak-mono-api-dev.service
systemctl enable --now gak-ms-web-dev.service
systemctl restart gak-mono-api-dev.service
systemctl restart gak-ms-web-dev.service
sleep 2
systemctl --no-pager --full status gak-mono-api-dev.service | head -20
systemctl --no-pager --full status gak-ms-web-dev.service | head -20
ss -lntp | grep -E ':18201|:13200' || true
echo "[bootstrap] 完成"
REMOTE

echo
echo "本机常驻同步:  ./scripts/dev-sync.sh"
echo "前端: http://192.168.220.109:13200"
echo "单体 API: http://192.168.220.109:18201"
