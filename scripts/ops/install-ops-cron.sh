#!/usr/bin/env bash
# 在 109 上安装运维定时任务（幂等：重复执行不会重复添加）。
# 用法：ssh root@109 'bash /www/go-admin-kit/src/scripts/ops/install-ops-cron.sh'
set -euo pipefail

OPS_DIR=/www/go-admin-kit/src/scripts/ops
chmod +x "$OPS_DIR"/*.sh

install_line() { # install_line <标记> <cron 行>
  local mark=$1 line=$2
  ( crontab -l 2>/dev/null | grep -v "$mark" ; echo "$line # $mark" ) | crontab -
}

# 凌晨 3:17 备份（错开宝塔 4 点任务）
install_line GOADMIN_PG_BACKUP  "17 3 * * * bash $OPS_DIR/pg-backup.sh >> /www/backups/pg-backup.log 2>&1"

mkdir -p /www/backups/pgsql
echo "[install-ops-cron] 已安装："
crontab -l | grep GOADMIN
