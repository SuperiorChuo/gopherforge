#!/usr/bin/env bash
# SLO 轻量探针：对网关健康/就绪路径采样延迟，输出 p50/p95 与是否达标。
# 不依赖 k6；用 curl 即可。适合日常与 chaos 后回归。
#
# 用法：
#   bash scripts/slo-probe.sh
#   BASE_URL=http://192.168.220.109:18100 SAMPLES=30 bash scripts/slo-probe.sh
#
# 环境变量：
#   BASE_URL   默认 http://192.168.220.109:18100
#   SAMPLES    采样次数，默认 20
#   PATHS      空格分隔路径，默认 "/api/v1/health/live /api/v1/health/ready"
#   P95_MS     p95 上限毫秒，默认 500
#   MAX_FAIL   允许失败次数，默认 1
#
# 退出码：0 达标；1 未达标；2 用法/依赖错误
set -euo pipefail

BASE_URL="${BASE_URL:-http://192.168.220.109:18100}"
SAMPLES="${SAMPLES:-20}"
PATHS="${PATHS:-/api/v1/health/live /api/v1/health/ready}"
P95_MS="${P95_MS:-500}"
MAX_FAIL="${MAX_FAIL:-1}"

if ! command -v curl >/dev/null 2>&1; then
  echo "需要 curl" >&2
  exit 2
fi

echo "== SLO probe =="
echo "base=$BASE_URL samples=$SAMPLES p95_limit=${P95_MS}ms max_fail=$MAX_FAIL"
echo "paths=$PATHS"
echo

overall_fail=0

for path in $PATHS; do
  times=()
  fails=0
  for i in $(seq 1 "$SAMPLES"); do
    # %{time_total} 秒 → 毫秒
    out=$(curl -sS -o /dev/null -w '%{http_code} %{time_total}' \
      --connect-timeout 3 --max-time 10 "${BASE_URL}${path}" 2>/dev/null || echo "000 9.999")
    code=${out%% *}
    sec=${out##* }
    ms=$(awk -v s="$sec" 'BEGIN{printf "%d", s*1000+0.5}')
    if [[ "$code" != "200" && "$code" != "204" ]]; then
      fails=$((fails + 1))
      echo "  $path #$i HTTP $code ${ms}ms"
    else
      times+=("$ms")
    fi
  done

  n=${#times[@]}
  if [[ "$n" -eq 0 ]]; then
    echo "[FAIL] $path: 全部失败 fails=$fails"
    overall_fail=1
    continue
  fi

  # sort times
  IFS=$'\n' sorted=($(printf '%s\n' "${times[@]}" | sort -n))
  unset IFS
  p50_idx=$(( (n - 1) * 50 / 100 ))
  p95_idx=$(( (n - 1) * 95 / 100 ))
  p50=${sorted[$p50_idx]}
  p95=${sorted[$p95_idx]}

  status=PASS
  if [[ "$fails" -gt "$MAX_FAIL" ]]; then
    status=FAIL
    overall_fail=1
  fi
  if [[ "$p95" -gt "$P95_MS" ]]; then
    status=FAIL
    overall_fail=1
  fi

  echo "[$status] $path n_ok=$n fails=$fails p50=${p50}ms p95=${p95}ms (limit p95<=${P95_MS}ms fail<=${MAX_FAIL})"
done

echo
if [[ "$overall_fail" -ne 0 ]]; then
  echo "结果: FAIL"
  exit 1
fi
echo "结果: PASS"
exit 0
