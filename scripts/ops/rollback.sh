#!/usr/bin/env bash
# 部署快照与回滚：
#   snapshot —— 部署前给当前运行镜像打 prev 标签（rollback 的还原点）
#   rollback <服务名...> —— 把服务镜像回滚到 prev 标签并重启（不 build）
# 用法（109 上）：
#   cd /www/go-admin-kit/src/microservices && ops/rollback.sh snapshot
#   cd /www/go-admin-kit/src/microservices && ops/rollback.sh rollback system-service
set -euo pipefail

ACTION=${1:-}
shift || true

case "$ACTION" in
snapshot)
  # 给当前所有 go-admin-kit 镜像打 :prev（覆盖上一个 prev）
  docker images --format '{{.Repository}}:{{.Tag}}' \
    | grep -E '^(go-admin-kit|microservices|src)[-_].*:latest$' \
    | while read -r img; do
        docker tag "$img" "${img%:latest}:prev"
        echo "[rollback] snapshot ${img%:latest}:prev"
      done
  ;;
rollback)
  [ $# -ge 1 ] || { echo "用法: rollback.sh rollback <compose服务名...>" >&2; exit 1; }
  for svc in "$@"; do
    # compose 镜像命名 <project>-<service>:latest；把 prev 复位为 latest 再 up
    img=$(docker compose config --images "$svc" 2>/dev/null | head -1)
    if [ -z "$img" ]; then echo "[rollback] 找不到服务 $svc 的镜像" >&2; exit 1; fi
    prev="${img%:*}:prev"
    docker image inspect "$prev" >/dev/null 2>&1 || { echo "[rollback] $prev 不存在（没打过快照）" >&2; exit 1; }
    docker tag "$prev" "$img"
    docker compose up -d --no-build "$svc"
    echo "[rollback] $svc 已回滚到上一版镜像"
  done
  ;;
*)
  echo "用法: rollback.sh snapshot | rollback <服务名...>" >&2
  exit 1
  ;;
esac
