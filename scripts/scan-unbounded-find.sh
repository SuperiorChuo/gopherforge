#!/usr/bin/env bash
# 扫描生产 Go 代码里「.Find(& 且邻近窗口无 Limit/First/Take/Count」的查询。
# 存量只列不改；对照基线的新增候选才阻断。
#
# 用法：
#   bash scripts/scan-unbounded-find.sh                 # 列工作区候选，退出 0
#   bash scripts/scan-unbounded-find.sh --against HEAD  # 工作区相对 HEAD 新增则退出 1
#   bash scripts/scan-unbounded-find.sh --staged        # 暂存区相对 HEAD 新增则退出 1
#   bash scripts/scan-unbounded-find.sh --root <dir>    # 扫指定树（下游仓）
#
# 豁免：Find 行或向上 8 行含 //nolint:unbounded-find
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
MODE="list"
AGAINST=""
SCAN_ROOT="$ROOT"

while [ $# -gt 0 ]; do
  case "$1" in
    --against)
      MODE="against"
      AGAINST="${2:?--against 需要一个 git ref}"
      shift 2
      ;;
    --staged)
      MODE="staged"
      AGAINST="HEAD"
      shift
      ;;
    --root)
      SCAN_ROOT="$(cd "${2:?--root 需要目录}" && pwd)"
      shift 2
      ;;
    -h|--help)
      sed -n '2,16p' "$0"
      exit 0
      ;;
    *)
      echo "未知参数: $1" >&2
      exit 2
      ;;
  esac
done

SCANNER="$(dirname "$0")/scan-unbounded-find.py"
if [ ! -f "$SCANNER" ]; then
  echo "缺少 $SCANNER" >&2
  exit 2
fi

if [ "$MODE" = "list" ]; then
  python3 "$SCANNER" --root "$SCAN_ROOT"
  exit 0
fi

if [ "$MODE" = "staged" ]; then
  python3 "$SCANNER" --root "$SCAN_ROOT" --staged --against "$AGAINST"
  exit $?
fi

python3 "$SCANNER" --root "$SCAN_ROOT" --against "$AGAINST"
