#!/usr/bin/env bash
# Swarm 服务热更新（部署服务器上执行）：准备依赖 → 构建镜像 → 迁移 → stack 声明同步 → 滚动更新。
# 更新顺序以 docker-stack.yml 为准：常规服务 start-first；密钥迁移所属的
# system-service stop-first，避免 V1/V2 进程在重叠窗口互相覆写证书密钥状态。
# gateway 仍由 Compose 常驻，PG/Redis/NATS/MinIO 由独立 infra Compose 栈管理。
# 用法：
#   bash scripts/swarm-redeploy.sh frontend            # 前端热更新（最常用）
#   bash scripts/swarm-redeploy.sh auth-service        # 滚动单个后端
#   bash scripts/swarm-redeploy.sh                     # 全量（构建+迁移+滚动全部）
#   bash scripts/swarm-redeploy.sh --sync-all frontend # 只重建 frontend，但把 .env
#                                                      # 改动同步给栈内所有服务
#   bash scripts/swarm-redeploy.sh --prebuilt auth-service # 快路径：产物取 fast-deploy.sh
#                                                      # 预编译同步来的 .bin/，跳过容器内编译
#
# 局部 vs 全量同步（改过 .env 时要留意）：
#   stack deploy 会按传入的声明同步服务 spec。默认只渲染本次指定的服务，
#   所以部署 A 不会顺带滚动 B、C——这是有意的：以前渲染整个栈，改一次 .env
#   再随便部署任何一个服务，都会把新 env 推给全部 17 个服务并触发全量滚动。
#   反过来，改了 .env 想让所有服务都拿到新值时，必须显式加 --sync-all
#   （不带服务名的全量部署本身就等于 --sync-all）。
#   未加 --sync-all 而 .env 比上次部署新时，脚本会告警提示。
set -euo pipefail

MS="${MS:-/opt/go-admin-kit/src/microservices}"
STACK="${STACK:-gak}"
APP_PROJECT="${APP_PROJECT:-go-admin-kit}"
INFRA_PROJECT="${INFRA_PROJECT:-go-admin-kit-infra}"
NETWORK="${NETWORK:-go-admin-kit-net}"
NETWORK_SUBNET="${NETWORK_SUBNET:-172.28.0.0/16}"
GATEWAY_PORT="${GATEWAY_PORT:-18100}"
FRONTEND_PORT="${FRONTEND_PORT:-13100}"
ALL=(monitor-service auth-service identity-service system-service audit-service \
  file-service ai-service im-service cc-service crm-service mp-service \
  visibility-service ticket-service notify-service bpm-service pay-service frontend)
cd "$MS"

# docker compose 会自动读取 .env 做 stack 插值，但本脚本的前置守卫运行在
# shell 中，不能看到其中的 key id / secret 名。这里只白名单读取非敏感 selector，
# 绝不 source 整份 .env（其中包含密码和密钥），外部显式环境变量仍优先。
EDGE_CERT_SELECTOR_KEYS="EDGE_CERT_HOST_DIR EDGE_CERT_CURRENT_KEY_ID EDGE_CERT_ENCRYPTION_SECRET_NAME EDGE_CERT_PREVIOUS_KEY_ID EDGE_CERT_PREVIOUS_ENCRYPTION_SECRET_NAME WEBHOOK_KEY_ID WEBHOOK_ENCRYPTION_SECRET_NAME"
load_edge_cert_selectors() {
  [ -f .env ] || return 0
  local key value
  while IFS='=' read -r key value; do
    case " $EDGE_CERT_SELECTOR_KEYS " in
      *" $key "*) ;;
      *) continue ;;
    esac
    # 调用方显式 export 的值优先于部署目录 .env。
    declare -p "$key" >/dev/null 2>&1 && continue
    value="${value%$'\r'}"
    case "$value" in
      \"*\") value="${value#\"}"; value="${value%\"}" ;;
      \'*\') value="${value#\'}"; value="${value%\'}" ;;
    esac
    printf -v "$key" '%s' "$value"
    export "$key"
  done < .env
}
load_edge_cert_selectors
EDGE_CERT_HOST_DIR="${EDGE_CERT_HOST_DIR:-/opt/go-admin-kit/certs}"
EDGE_CERT_ENCRYPTION_SECRET_NAME="${EDGE_CERT_ENCRYPTION_SECRET_NAME:-go-admin-kit-edge-cert-encryption-key}"
EDGE_CERT_PREVIOUS_ENCRYPTION_SECRET_NAME="${EDGE_CERT_PREVIOUS_ENCRYPTION_SECRET_NAME:-}"
WEBHOOK_KEY_ID="${WEBHOOK_KEY_ID:-current}"
WEBHOOK_ENCRYPTION_SECRET_NAME="${WEBHOOK_ENCRYPTION_SECRET_NAME:-go-admin-kit-webhook-encryption-key}"
export EDGE_CERT_HOST_DIR EDGE_CERT_ENCRYPTION_SECRET_NAME EDGE_CERT_PREVIOUS_ENCRYPTION_SECRET_NAME WEBHOOK_KEY_ID WEBHOOK_ENCRYPTION_SECRET_NAME

SYNC_ALL="${SYNC_ALL:-0}"
PREBUILT="${PREBUILT:-0}"
ARGS=()
for arg in "$@"; do
  case "$arg" in
    --sync-all) SYNC_ALL=1 ;;
    --prebuilt) PREBUILT=1 ;;
    -*)
      echo "[swarm] 未知选项：$arg（仅支持 --sync-all / --prebuilt）" >&2
      exit 2
      ;;
    *) ARGS+=("$arg") ;;
  esac
done

SVCS=("${ARGS[@]}")
# 不指定服务即全量部署，语义上本就包含"同步所有 spec"
[ ${#SVCS[@]} -gt 0 ] || { SVCS=("${ALL[@]}"); SYNC_ALL=1; }

contains_service() {
  local wanted="$1"
  local service
  for service in "${ALL[@]}"; do
    [ "$service" = "$wanted" ] && return 0
  done
  return 1
}

for service in "${SVCS[@]}"; do
  contains_service "$service" || {
    echo "[swarm] 不支持的服务：$service" >&2
    echo "[swarm] 可用服务：${ALL[*]}" >&2
    exit 2
  }
done

# 源码指纹（构建信息生成）：本次构建用的是哪份源码；dirty>0 = 脏树构建（先部署验证后 commit 的常规流程）
if [ -f "$MS/../BUILD_INFO.json" ]; then
  echo "[swarm] 源码指纹 $(cat "$MS/../BUILD_INFO.json")"
else
  echo "[swarm] 提示：无 BUILD_INFO.json（构建信息未生成指纹，部署照常）"
fi

ensure_network() {
  if ! docker network inspect "$NETWORK" >/dev/null 2>&1; then
    echo "[swarm] 创建 attachable overlay 网络：$NETWORK"
    docker network create --driver overlay --attachable --subnet "$NETWORK_SUBNET" "$NETWORK" >/dev/null
  fi

  local driver scope attachable
  driver=$(docker network inspect "$NETWORK" --format '{{.Driver}}')
  scope=$(docker network inspect "$NETWORK" --format '{{.Scope}}')
  attachable=$(docker network inspect "$NETWORK" --format '{{.Attachable}}')
  if [ "$driver" != overlay ] || [ "$scope" != swarm ] || [ "$attachable" != true ]; then
    echo "[swarm] 网络 $NETWORK 必须是 attachable overlay（当前 driver=$driver scope=$scope attachable=$attachable）" >&2
    exit 1
  fi
}

ensure_edge_cert_storage() {
  case "$EDGE_CERT_HOST_DIR" in
    /opt/go-admin-kit/*) ;;
    *)
      echo "[swarm] EDGE_CERT_HOST_DIR 必须位于 /opt/go-admin-kit/ 下：$EDGE_CERT_HOST_DIR" >&2
      exit 1
      ;;
  esac
  echo "[swarm] 准备边缘证书持久目录：$EDGE_CERT_HOST_DIR"
  install -d -m 0700 -o 10001 -g 10001 \
    "$EDGE_CERT_HOST_DIR" \
    "$EDGE_CERT_HOST_DIR/edge" \
    "$EDGE_CERT_HOST_DIR/traefik-dynamic"
}

require_webhook_secret() {
  local service
  for service in "${DEPLOY_SVCS[@]}"; do
    [ "$service" = audit-service ] || continue
    docker secret inspect "$WEBHOOK_ENCRYPTION_SECRET_NAME" >/dev/null 2>&1 || {
      echo "[swarm] 缺少 Swarm secret：$WEBHOOK_ENCRYPTION_SECRET_NAME" >&2
      echo "[swarm] 生成 base64(raw 32 bytes) 后执行：printf '%s' \"\$KEY\" | docker secret create $WEBHOOK_ENCRYPTION_SECRET_NAME -" >&2
      exit 1
    }
  done
}

require_edge_cert_secret() {
  local service
  for service in "${DEPLOY_SVCS[@]}"; do
    [ "$service" = system-service ] || continue
    [ -n "${EDGE_CERT_CURRENT_KEY_ID:-}" ] || {
      echo "[swarm] 部署 system-service 必须设置 EDGE_CERT_CURRENT_KEY_ID" >&2
      exit 1
    }
    docker secret inspect "$EDGE_CERT_ENCRYPTION_SECRET_NAME" >/dev/null 2>&1 || {
      echo "[swarm] 缺少 Swarm secret：$EDGE_CERT_ENCRYPTION_SECRET_NAME" >&2
      echo "[swarm] 生成 base64(raw 32 bytes) 后执行：printf '%s' \"\$KEY\" | docker secret create $EDGE_CERT_ENCRYPTION_SECRET_NAME -" >&2
      exit 1
    }
    if [ -n "${EDGE_CERT_PREVIOUS_KEY_ID:-}" ]; then
      [ -n "$EDGE_CERT_PREVIOUS_ENCRYPTION_SECRET_NAME" ] || {
        echo "[swarm] EDGE_CERT_PREVIOUS_KEY_ID 已设置，但缺少 EDGE_CERT_PREVIOUS_ENCRYPTION_SECRET_NAME" >&2
        exit 1
      }
      docker secret inspect "$EDGE_CERT_PREVIOUS_ENCRYPTION_SECRET_NAME" >/dev/null 2>&1 || {
        echo "[swarm] 缺少上一把密钥 Swarm secret：$EDGE_CERT_PREVIOUS_ENCRYPTION_SECRET_NAME" >&2
        exit 1
      }
    fi
  done
}

wait_for_container() {
  local container="$1"
  local status=""
  for _ in $(seq 1 90); do
    status=$(docker inspect -f '{{.State.Health.Status}}' "$container" 2>/dev/null || true)
    [ "$status" = healthy ] && return 0
    sleep 2
  done
  echo "[swarm] 容器未通过健康检查：$container（status=${status:-unknown}）" >&2
  docker logs --tail 80 "$container" >&2 || true
  return 1
}

ensure_infra() {
  echo "[swarm] 1/5 确保 infra 栈存在（PostgreSQL、Redis、NATS、MinIO）"
  docker compose -p "$INFRA_PROJECT" -f docker-compose.infra.yml --profile storage up -d
  for container in go-admin-kit-postgres go-admin-kit-redis go-admin-kit-nats go-admin-kit-minio; do
    wait_for_container "$container"
  done
}

ensure_gateway() {
  echo "[swarm] 1/5 确保 Compose gateway 存在（业务服务不走 Compose）"
  docker compose -p "$APP_PROJECT" --profile compose-fallback up -d --build gateway
  wait_for_container go-admin-kit-gateway
}

ensure_network
ensure_edge_cert_storage
ensure_infra
ensure_gateway

NEED_MIGRATE=0
for service in "${SVCS[@]}"; do
  [ "$service" != frontend ] && NEED_MIGRATE=1
done

BUILD_SVCS=("${SVCS[@]}")
[ "$NEED_MIGRATE" = 1 ] && BUILD_SVCS+=(migrate)

# --prebuilt 快路径：镜像里的产物直接取 fast-deploy.sh 交叉编译并同步来的 .bin/，
# 跳过容器内编译。守卫：产物必须存在且新鲜（30 分钟内），防止把陈旧二进制滚上线。
BUILD_ARGS=()
if [ "$PREBUILT" = 1 ]; then
  check_fresh() {
    local path="$1" what="$2"
    [ -e "$path" ] || {
      echo "[swarm] --prebuilt 缺产物：${path}（${what}；先跑 scripts/fast-deploy.sh，或去掉 --prebuilt 走容器内编译）" >&2
      exit 1
    }
    local age=$(( $(date +%s) - $(stat -c %Y "$path") ))
    [ "$age" -le 1800 ] || {
      echo "[swarm] --prebuilt 产物陈旧：$path 编译于 $((age/60)) 分钟前（守卫上限 30 分钟）。重跑 scripts/fast-deploy.sh。" >&2
      exit 1
    }
  }
  for service in "${BUILD_SVCS[@]}"; do
    case "$service" in
      frontend) check_fresh "web/.bin/html/index.html" "本机 npm run build 产物" ;;
      migrate)  check_fresh "services/monitor/.bin/migrate" "monitor 迁移二进制" ;;
      *)        check_fresh "services/${service%-service}/.bin/main" "$service 二进制" ;;
    esac
  done
  BUILD_ARGS+=(--build-arg MAIN_SRC=prebuilt)
  echo "[swarm] 2/5 构建镜像（--prebuilt 快路径：预编译产物直接进瘦镜像）"
else
  echo "[swarm] 2/5 构建镜像（复用 Compose build 定义，显式启用 profile）"
fi
docker compose -p "$APP_PROJECT" --profile compose-fallback build ${BUILD_ARGS[@]+"${BUILD_ARGS[@]}"} "${BUILD_SVCS[@]}"

echo "[swarm] 3/5 数据库迁移（goose 幂等；仅纯前端更新时跳过）"
if [ "$NEED_MIGRATE" = 1 ]; then
  docker compose -p "$APP_PROJECT" --profile compose-fallback run --rm --no-deps migrate
else
  echo "  本次仅更新 frontend，跳过 migrate"
fi

warn_env_drift() {
  # 局部部署不会把 .env 改动传播给未指定的服务。若 .env 比栈里最早一次更新还新，
  # 说明有服务仍在用旧 env，提示用 --sync-all（只告警，不阻断）。
  [ "$SYNC_ALL" = 1 ] && return 0
  [ -f .env ] || return 0

  local env_mtime oldest="" updated svc_epoch
  env_mtime=$(stat -c %Y .env 2>/dev/null || stat -f %m .env 2>/dev/null || echo 0)
  [ "$env_mtime" -gt 0 ] || return 0

  local service
  for service in "${ALL[@]}"; do
    updated=$(docker service inspect --format '{{.UpdatedAt}}' \
      "${STACK}_${service}" 2>/dev/null || true)
    [ -n "$updated" ] || continue
    svc_epoch=$(date -d "$updated" +%s 2>/dev/null || echo 0)
    [ "$svc_epoch" -gt 0 ] || continue
    if [ -z "$oldest" ] || [ "$svc_epoch" -lt "$oldest" ]; then
      oldest="$svc_epoch"
    fi
  done

  if [ -n "$oldest" ] && [ "$env_mtime" -gt "$oldest" ]; then
    echo "[swarm] 注意：.env 比栈内最早一次服务更新还新，但本次是局部部署。"
    echo "[swarm]       未指定的服务仍会用旧 env；要全栈同步请加 --sync-all。"
  fi
}

echo "[swarm] 4/5 同步 stack 声明（幂等；.env 插值经 Compose config 展开）"
BEFORE=$(docker stack services "$STACK" --format '{{.Name}}' 2>/dev/null || true)
warn_env_drift

# 只渲染本次要动的服务：stack deploy 不带 --prune 不会删除声明里缺席的服务，
# 所以局部声明只同步这些服务的 spec，不会顺带滚动其它服务。
if [ "$SYNC_ALL" = 1 ]; then
  DEPLOY_SVCS=("${ALL[@]}")
  echo "  同步范围：全部 ${#DEPLOY_SVCS[@]} 个服务（--sync-all）"
else
  DEPLOY_SVCS=("${SVCS[@]}")
  echo "  同步范围：${DEPLOY_SVCS[*]}"
fi
require_edge_cert_secret
require_webhook_secret

# compose config 会把两类标量归一成 stack deploy 不接受的形态，必须还原：
#   published: "13100" → 13100     stack deploy 要求整数
#   cpus: '2.0'        → cpus: "2.0"  stack deploy 要求字符串
# 任一不还原，stack 同步都会直接报错退出
STACK_CONFIG_FILES=(-f docker-stack.yml)
if [ -n "${EDGE_CERT_PREVIOUS_KEY_ID:-}" ]; then
  STACK_CONFIG_FILES+=(-f docker-stack.edge-cert-rotation.yml)
fi
docker compose "${STACK_CONFIG_FILES[@]}" config "${DEPLOY_SVCS[@]}" \
  | sed -e '/^name:/d' \
        -e 's/published: "\([0-9][0-9]*\)"/published: \1/' \
        -e 's/\(cpus: \)\([0-9][0-9.]*\)$/\1"\2"/' \
  | docker stack deploy --resolve-image never --detach=true -c - "$STACK"

echo "[swarm] 4/5 按 stack 声明策略更新请求服务（新建服务由 stack deploy 启动）"
for service in "${SVCS[@]}"; do
  if echo "$BEFORE" | grep -qx "${STACK}_${service}"; then
    docker service update --force --detach=false "${STACK}_${service}"
  else
    echo "  ${STACK}_${service} 为新建服务，stack deploy 已启动，跳过 --force"
  fi
done

wait_for_service() {
  local service="$1"
  local full_name="${STACK}_${service}"
  local replicas=""
  local desired=""
  for _ in $(seq 1 90); do
    replicas=$(docker service ls --format '{{.Name}} {{.Replicas}}' \
      | awk -v name="$full_name" '$1 == name {print $2}')
    desired=$(docker service inspect --format '{{.Spec.Mode.Replicated.Replicas}}' \
      "$full_name" 2>/dev/null || true)
    if [ -n "$desired" ] && [ "$replicas" = "$desired/$desired" ]; then
      return 0
    fi
    sleep 2
  done
  echo "[swarm] 服务副本未收敛：$full_name（${replicas:-unknown}/${desired:-unknown}）" >&2
  docker service ps --no-trunc "$full_name" >&2 || true
  return 1
}

echo "[swarm] 5/5 等待全部业务服务副本就绪"
for service in "${ALL[@]}"; do
  wait_for_service "$service"
done

docker stack services "$STACK"
curl -fsS --max-time 8 "http://127.0.0.1:${GATEWAY_PORT}/api/v1/health/ready" >/dev/null
curl -fsS --max-time 8 "http://127.0.0.1:${FRONTEND_PORT}/" >/dev/null

# 前端上线指纹：与上方源码指纹比对 sha/generated_at 即知新构建是否真上线（替代 assets 哈希考古）
for service in "${SVCS[@]}"; do
  if [ "$service" = frontend ]; then
    echo "[swarm] 前端上线指纹 $(curl -fsS --max-time 5 "http://127.0.0.1:${FRONTEND_PORT}/version.json" 2>/dev/null || echo '（无 version.json：旧镜像或未经 构建信息生成指纹）')"
  fi
done

# 部署留痕：每服务一行 JSONL（后端没有 /version.json，这里是"某服务上次从哪个 SHA 部署"的唯一记录）
# 查询：grep '"<服务名>"' /opt/go-admin-kit/deploy-log.jsonl | tail -1
DEPLOY_LOG="${DEPLOY_LOG:-/opt/go-admin-kit/deploy-log.jsonl}"
BI="$(cat "$MS/../BUILD_INFO.json" 2>/dev/null || echo '{}')"
for service in "${SVCS[@]}"; do
  printf '{"ts":"%s","service":"%s","build":%s}\n' "$(date '+%Y-%m-%dT%H:%M:%S%z')" "$service" "$BI" >> "$DEPLOY_LOG" || true
done
echo "[swarm] 部署已留痕 → ${DEPLOY_LOG}"
echo "[swarm] gateway=${GATEWAY_PORT} frontend=${FRONTEND_PORT} ready"
