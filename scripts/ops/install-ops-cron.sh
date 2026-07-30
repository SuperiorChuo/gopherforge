#!/usr/bin/env bash
# 在部署服务器上安装运维定时任务（幂等：重复执行不会重复添加）。
# 用法：ssh <部署用户>@<服务器> 'bash <仓库路径>/scripts/ops/install-ops-cron.sh'
# 可用环境变量覆盖：OPS_DIR（脚本目录）、BACKUP_ROOT（备份与日志根目录）
set -euo pipefail

OPS_DIR=${OPS_DIR:-$(cd "$(dirname "$0")" && pwd)}
BACKUP_ROOT=${BACKUP_ROOT:-/var/backups/go-admin-kit}
chmod +x "$OPS_DIR"/*.sh

install_line() { # install_line <标记> <cron 行>
  local mark=$1 line=$2
  ( crontab -l 2>/dev/null | grep -v "$mark" ; echo "$line # $mark" ) | crontab -
}

# 凌晨 3:17 备份（避开整点，错开服务器上其它定时任务）
install_line GOADMIN_PG_BACKUP  "17 3 * * * bash $OPS_DIR/pg-backup.sh >> $BACKUP_ROOT/pg-backup.log 2>&1"

mkdir -p "$BACKUP_ROOT/pgsql"
echo "[install-ops-cron] 已安装："
crontab -l | grep GOADMIN
