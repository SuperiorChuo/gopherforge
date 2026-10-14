#!/usr/bin/env bash
# 一次性割接：把 部署服务器上在跑的单 compose 栈（project go-admin-kit）拆为
#   infra 栈（project go-admin-kit-infra：PG/Redis/NATS/MinIO，volume 钉旧名零迁移）
# + 应用栈（project go-admin-kit：其余服务，网络改 external）
# 前置：源码同步已把拆分后的 docker-compose.yml / docker-compose.infra.yml 推到部署服务器。
# 用法（部署服务器上）：bash /opt/go-admin-kit/src/scripts/ops/infra-split-cutover.sh
set -euo pipefail

MS=/opt/go-admin-kit/src/microservices
SRC=/opt/go-admin-kit/src
cd "$MS"
TS=$(date +%Y%m%d-%H%M%S)

echo "[cutover] 0/6 预检：文件校验与在跑容器"
test -f docker-compose.infra.yml || { echo "缺 docker-compose.infra.yml，先 源码同步"; exit 1; }
docker compose --profile compose-fallback -f docker-compose.yml config -q
docker compose -p go-admin-kit-infra -f docker-compose.infra.yml config -q
docker ps --format '{{.Names}}' | grep -q '^go-admin-kit-postgres$' \
  || { echo "postgres 容器未在跑，割接前状态异常，中止"; exit 1; }

# 按当前在跑容器自动识别 profile（monitoring/storage）
APP_PROFILES=(--profile compose-fallback)
docker ps --format '{{.Names}}' | grep -q '^go-admin-kit-prometheus$' && APP_PROFILES+=(--profile monitoring)
INFRA_PROFILES=()
docker ps --format '{{.Names}}' | grep -q '^go-admin-kit-minio$' && INFRA_PROFILES+=(--profile storage)
echo "  app: ${APP_PROFILES[*]}  infra: ${INFRA_PROFILES[*]:-无 profile}"

echo "[cutover] 1/6 PG 全量备份 + 基线行数"
mkdir -p /opt/go-admin-kit/backups/pgsql
docker exec go-admin-kit-postgres sh -c 'pg_dumpall -U "$POSTGRES_USER"' \
  | gzip > "/opt/go-admin-kit/backups/pgsql/pre-infra-split-$TS.sql.gz"
ls -lh "/opt/go-admin-kit/backups/pgsql/pre-infra-split-$TS.sql.gz"
USERS_BEFORE=$(docker exec go-admin-kit-postgres sh -c \
  'psql -U "$POSTGRES_USER" -d "$POSTGRES_DB" -tAc "select count(*) from users"')
echo "  users 基线: $USERS_BEFORE 行"

echo "[cutover] 2/6 镜像快照（rollback 还原点）"
bash "$SRC/scripts/ops/rollback.sh" snapshot

echo "[cutover] 3/6 停旧单栈（旧 infra 容器成孤儿一并移除；volume 不动）"
docker compose -p go-admin-kit ${APP_PROFILES[@]+"${APP_PROFILES[@]}"} down --remove-orphans

# 网络必须在 down 之后建：旧网络带 project=go-admin-kit 标签，down 会按标签
# 连它一起回收（compose 文件里的 external 声明挡不住），先建必被删。
echo "[cutover] 4/6 重建共享网络"
docker network inspect go-admin-kit-net >/dev/null 2>&1 \
  || docker network create --subnet 172.28.0.0/16 go-admin-kit-net

echo "[cutover] 5/6 起 infra 栈并等健康"
docker compose -p go-admin-kit-infra -f docker-compose.infra.yml \
  ${INFRA_PROFILES[@]+"${INFRA_PROFILES[@]}"} up -d
WAIT="go-admin-kit-postgres go-admin-kit-redis go-admin-kit-nats"
[ ${#INFRA_PROFILES[@]} -gt 0 ] && WAIT="$WAIT go-admin-kit-minio"
for c in $WAIT; do
  ok=""
  for _ in $(seq 1 60); do
    [ "$(docker inspect -f '{{.State.Health.Status}}' "$c" 2>/dev/null || true)" = healthy ] && { ok=1; break; }
    sleep 2
  done
  [ -n "$ok" ] || { echo "  $c 未健康，中止（volume 未动，可从备份恢复）"; docker logs --tail 50 "$c" || true; exit 1; }
  echo "  $c healthy"
done
USERS_AFTER=$(docker exec go-admin-kit-postgres sh -c \
  'psql -U "$POSTGRES_USER" -d "$POSTGRES_DB" -tAc "select count(*) from users"')
echo "  users 割接后: $USERS_AFTER 行（基线 $USERS_BEFORE）"
[ "$USERS_AFTER" = "$USERS_BEFORE" ] || { echo "  数据核对不一致，中止"; exit 1; }

echo "[cutover] 6/6 起应用栈"
docker compose -p go-admin-kit ${APP_PROFILES[@]+"${APP_PROFILES[@]}"} up -d
docker compose -p go-admin-kit ps

echo "[cutover] 完成。后续验证："
echo "  docker compose -p go-admin-kit ps                       # 应全 healthy"
echo "  curl -s http://127.0.0.1:18100/api/v1/captcha | head -c 120"
