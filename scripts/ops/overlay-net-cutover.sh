#!/usr/bin/env bash
# 网络割接（集群化第二步）：go-admin-kit-net 由单机 bridge 重建为 overlay attachable，
# 同名同子网（172.28.0.0/16，TRUSTED_PROXIES 白名单不变）。
# 双栈 compose 文件里网络均为 external，本割接零文件改动；
# 换完后未来节点 docker swarm join 即可跨机互通。
# 前置：docker swarm init 已执行（脚本自检）。
# 用法（部署服务器上）：bash /opt/go-admin-kit/src/scripts/ops/overlay-net-cutover.sh
# 回退：停两栈 → docker network rm go-admin-kit-net →
#       docker network create --subnet 172.28.0.0/16 go-admin-kit-net → 两栈 up
set -euo pipefail

MS=/opt/go-admin-kit/src/microservices
cd "$MS"
TS=$(date +%Y%m%d-%H%M%S)

echo "[overlay] 0/5 预检"
docker info --format '{{.Swarm.LocalNodeState}}' | grep -qx active \
  || { echo "swarm 未激活：先 docker swarm init --advertise-addr <本机IP>"; exit 1; }
CUR_DRIVER=$(docker network inspect go-admin-kit-net -f '{{.Driver}}' 2>/dev/null || echo none)
[ "$CUR_DRIVER" = overlay ] && { echo "已是 overlay，无需割接"; exit 0; }
docker ps --format '{{.Names}}' | grep -q '^go-admin-kit-postgres$' \
  || { echo "postgres 未在跑，状态异常，中止"; exit 1; }

APP_PROFILES=(--profile compose-fallback)
docker ps --format '{{.Names}}' | grep -q '^go-admin-kit-prometheus$' && APP_PROFILES+=(--profile monitoring)
INFRA_PROFILES=()
docker ps --format '{{.Names}}' | grep -q '^go-admin-kit-minio$' && INFRA_PROFILES+=(--profile storage)
echo "  app: ${APP_PROFILES[*]}  infra: ${INFRA_PROFILES[*]:-无 profile}"

echo "[overlay] 1/5 PG 备份"
mkdir -p /opt/go-admin-kit/backups/pgsql
docker exec go-admin-kit-postgres sh -c 'pg_dumpall -U "$POSTGRES_USER"' \
  | gzip > "/opt/go-admin-kit/backups/pgsql/pre-overlay-$TS.sql.gz"
ls -lh "/opt/go-admin-kit/backups/pgsql/pre-overlay-$TS.sql.gz"

echo "[overlay] 2/5 停两栈（volume 不动）"
docker compose -p go-admin-kit ${APP_PROFILES[@]+"${APP_PROFILES[@]}"} down
docker compose -p go-admin-kit-infra -f docker-compose.infra.yml \
  ${INFRA_PROFILES[@]+"${INFRA_PROFILES[@]}"} down

echo "[overlay] 3/5 重建网络为 overlay attachable（同名同子网）"
docker network rm go-admin-kit-net 2>/dev/null || true
docker network create -d overlay --attachable --subnet 172.28.0.0/16 go-admin-kit-net

echo "[overlay] 4/5 起 infra 栈等健康，再起应用栈"
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
  [ -n "$ok" ] || { echo "  $c 未健康，中止（volume 未动，可按脚本头部说明回退）"; docker logs --tail 30 "$c" || true; exit 1; }
  echo "  $c healthy"
done
docker compose -p go-admin-kit ${APP_PROFILES[@]+"${APP_PROFILES[@]}"} up -d

echo "[overlay] 5/5 验证"
docker network inspect go-admin-kit-net \
  -f 'driver={{.Driver}} attachable={{.Attachable}} subnet={{(index .IPAM.Config 0).Subnet}}'
sleep 20
docker compose -p go-admin-kit ps --format '{{.Name}}\t{{.Status}}'
curl -s -m 8 http://127.0.0.1:18100/api/v1/captcha | head -c 80; echo
echo "[overlay] 完成"
