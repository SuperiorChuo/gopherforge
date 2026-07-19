#!/usr/bin/env bash
# PG 每日全量备份：主库(go-admin-kit-postgres)。
# 部署在 109 的 root crontab（安装：scripts/ops/install-ops-cron.sh）。
# 产物：/www/backups/pgsql/<库>-YYYYmmdd-HHMM.sql.gz，保留 RETAIN_DAYS 天。
set -euo pipefail

BACKUP_DIR=${BACKUP_DIR:-/www/backups/pgsql}
RETAIN_DAYS=${RETAIN_DAYS:-7}
STAMP=$(date +%Y%m%d-%H%M)
mkdir -p "$BACKUP_DIR"

dump() { # dump <容器名> <标签>
  local ctr=$1 tag=$2 out
  out="$BACKUP_DIR/${tag}-${STAMP}.sql.gz"
  if ! docker ps --format '{{.Names}}' | grep -qx "$ctr"; then
    echo "[pg-backup] 跳过 $ctr（容器未运行）" >&2
    return 0
  fi
  # 容器内 env 自带 POSTGRES_USER/POSTGRES_DB；pg_dumpall 连全局对象一起备
  docker exec "$ctr" sh -c 'pg_dumpall -U "$POSTGRES_USER"' | gzip > "$out.tmp"
  # 空产物视为失败，不覆盖历史
  if [ ! -s "$out.tmp" ]; then
    echo "[pg-backup] $ctr 产物为空，失败" >&2
    rm -f "$out.tmp"
    return 1
  fi
  mv "$out.tmp" "$out"
  echo "[pg-backup] $ctr -> $out ($(du -h "$out" | cut -f1))"
}

rc=0
dump go-admin-kit-postgres main || rc=1

# 清理过期备份
find "$BACKUP_DIR" -name '*.sql.gz' -mtime +"$RETAIN_DAYS" -delete

exit $rc
