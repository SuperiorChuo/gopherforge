#!/usr/bin/env bash
# 部署快照与回滚：
#   snapshot —— 部署前给当前运行镜像打 prev 标签（rollback 的还原点）
#   rollback <服务名...> —— Swarm 服务回滚镜像并滚动重启；Compose 服务回退后重启
# 用法（部署服务器上）：
#   cd /opt/go-admin-kit/src/microservices && ops/rollback.sh snapshot
#   cd /opt/go-admin-kit/src/microservices && ops/rollback.sh rollback cc-service
set -euo pipefail

ACTION=${1:-}
shift || true

case "$ACTION" in
snapshot)
  # 给当前所有 go-admin-kit/fscc 镜像打 :prev（覆盖上一个 prev）
  docker images --format '{{.Repository}}:{{.Tag}}' \
    | grep -E '^(go-admin-kit|microservices|freeswitch-cc|src)[-_].*:latest$' \
    | while read -r img; do
        docker tag "$img" "${img%:latest}:prev"
        echo "[rollback] snapshot ${img%:latest}:prev"
      done
  ;;
rollback)
  [ $# -ge 1 ] || { echo "用法: rollback.sh rollback <服务名...>" >&2; exit 1; }
  for svc in "$@"; do
    swarm_svc="gak_${svc}"
    if docker service inspect "$swarm_svc" >/dev/null 2>&1; then
      img=$(docker service inspect --format '{{.Spec.TaskTemplate.ContainerSpec.Image}}' "$swarm_svc")
      base="${img%%@*}"
      case "$base" in
        *:*) prev="${base%:*}:prev" ;;
        *) prev="$base:prev" ;;
      esac
      docker image inspect "$prev" >/dev/null 2>&1 || { echo "[rollback] $prev 不存在（没打过快照）" >&2; exit 1; }
      docker service update --image "$prev" --force --detach=false "$swarm_svc"
      echo "[rollback] $swarm_svc 已回滚到上一版镜像"
      continue
    fi

    # Compose fallback 镜像命名 <project>-<service>:latest；把 prev 复位为 latest 再 up。
    img=$(docker compose --profile compose-fallback config --images "$svc" 2>/dev/null | head -1)
    if [ -z "$img" ]; then echo "[rollback] 找不到服务 $svc 的 Swarm 或 Compose 实例" >&2; exit 1; fi
    prev="${img%:*}:prev"
    docker image inspect "$prev" >/dev/null 2>&1 || { echo "[rollback] $prev 不存在（没打过快照）" >&2; exit 1; }
    docker tag "$prev" "$img"
    docker compose --profile compose-fallback up -d --no-build "$svc"
    echo "[rollback] $svc 已按 Compose fallback 回滚到上一版镜像"
  done
  ;;
*)
  echo "用法: rollback.sh snapshot | rollback <服务名...>" >&2
  exit 1
  ;;
esac
