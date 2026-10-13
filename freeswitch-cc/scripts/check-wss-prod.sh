#!/usr/bin/env bash
# 生产 WebPhone WSS 就绪自检（只读）——在 FreeSWITCH 宿主机上运行。
#
# 用途：把 deploy-and-debug.md §4.3 的分散手工验证收敛成一条只读命令，证书/NAT 到位后
#      跑一遍即可知道生产 WSS 还差哪一步。脚本不写任何状态、不改任何服务。
#
# 用法：
#   ./check-wss-prod.sh                                    # 端点从 microservices/.env 的 CC_SIP_WSS_URL 读
#   ./check-wss-prod.sh wss://pbx.example.com:7443         # 显式给定期望端点（用于证书 SAN 校验与严格验收）
#
# 可选环境变量：
#   REPO_ROOT    仓库在宿主机上的路径（默认由本脚本所在目录推导）
#   FS_CONTAINER FreeSWITCH 容器名（默认 fscc-freeswitch-host）
#   FRONTEND_PREFIX 前端 Swarm 容器名前缀（默认 gak_frontend）
#
# 退出码：0=内网就绪（外部验证项单独提示）；1=存在 FAIL；2=环境不可判（缺工具/容器）
set -u

REPO_ROOT="${REPO_ROOT:-$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)}"
FS_CONTAINER="${FS_CONTAINER:-fscc-freeswitch-host}"
FRONTEND_PREFIX="${FRONTEND_PREFIX:-gak_frontend}"
ENV_FILE="$REPO_ROOT/microservices/.env"
VERIFIER="${VERIFIER:-$REPO_ROOT/freeswitch-cc/scripts/check-wss.mjs}"
CERT_IN_CONTAINER="/etc/freeswitch/tls/wss.pem"

ok=0; warn=0; fail=0
report() { # 状态 标签 详情
  local st="$1" label="$2" detail="$3"
  case "$st" in
    OK)   ok=$((ok+1));   printf '  [OK]   %-28s %s\n' "$label" "$detail" ;;
    WARN) warn=$((warn+1)); printf '  [WARN] %-28s %s\n' "$label" "$detail" ;;
    FAIL) fail=$((fail+1)); printf '  [FAIL] %-28s %s\n' "$label" "$detail" ;;
  esac
}

have() { command -v "$1" >/dev/null 2>&1; }

echo "== WebPhone 生产 WSS 就绪自检 =="
echo "  主机: $(hostname)  时间: $(date '+%Y-%m-%d %H:%M:%S')"
echo

# 期望端点：优先参数，其次 .env
endpoint="${1:-}"
if [[ -z "$endpoint" && -f "$ENV_FILE" ]]; then
  endpoint="$(grep -E '^CC_SIP_WSS_URL=' "$ENV_FILE" | head -1 | cut -d= -f2- | tr -d ' "' 2>/dev/null || true)"
fi

# 工具与容器前置
if ! have docker; then
  echo "  [FATAL] 缺 docker，无法探测容器。" >&2
  exit 2
fi
if ! docker ps --format '{{.Names}}' | grep -qx "$FS_CONTAINER"; then
  echo "  [FATAL] 未发现容器 $FS_CONTAINER（freeswitch-cc 栈未启动？）。" >&2
  exit 2
fi

echo "-- 1/6 媒体面 --"
sofia_out="$(docker exec "$FS_CONTAINER" fs_cli -x 'sofia status profile internal' 2>/dev/null || true)"
if grep -q 'WSS-BIND-URL' <<<"$sofia_out"; then
  wss_bind="$(grep 'WSS-BIND-URL' <<<"$sofia_out" | awk '{print $2}')"
  if grep -q ':7443' <<<"$wss_bind"; then
    report OK 'WSS 监听 :7443' "$wss_bind"
  else
    report FAIL 'WSS 监听' "WSS-BIND-URL 存在但未绑定 7443：$wss_bind"
  fi
else
  report FAIL 'WSS 监听' 'sofia internal 无 WSS-BIND-URL（未启用 TLS 监听）'
fi

echo "-- 2/6 证书 --"
cert_pem="$(docker exec "$FS_CONTAINER" cat "$CERT_IN_CONTAINER" 2>/dev/null || true)"
if [[ -z "$cert_pem" ]]; then
  report FAIL '证书文件' "容器内 $CERT_IN_CONTAINER 缺失"
else
  report OK '证书文件' "容器内 $CERT_IN_CONTAINER 可读 ($(wc -c <<<"$cert_pem") bytes)"
  if have openssl; then
    subject="$(openssl x509 -noout -subject 2>/dev/null <<<"$cert_pem" | sed 's/^subject=//' || true)"
    issuer="$(openssl x509 -noout -issuer 2>/dev/null <<<"$cert_pem" | sed 's/^issuer=//' || true)"
    dates="$(openssl x509 -noout -dates 2>/dev/null <<<"$cert_pem" || true)"
    not_before="$(grep 'notBefore' <<<"$dates" | cut -d= -f2- | xargs)"
    not_after="$(grep 'notAfter' <<<"$dates" | cut -d= -f2- | xargs)"
    sans="$(openssl x509 -noout -ext subjectAltName 2>/dev/null <<<"$cert_pem" | grep -o 'DNS:[^,]*' | cut -d: -f2- || true)"

    if [[ "$subject" == "$issuer" ]]; then
      report WARN '证书身份' "自签名 $subject（生产须替换为受信证书）"
    else
      report OK '证书身份' "subject=$subject issuer=$issuer"
    fi

    if [[ -n "$not_after" ]]; then
      if [[ "$(uname)" == "Darwin" ]]; then
        report WARN '证书有效期' "至 $not_after（macOS 无法精确比对，请目测）"
      else
        expiry="$(date -d "$not_after" +%s 2>/dev/null || echo 0)"
        now="$(date +%s)"
        if [[ "$expiry" -gt "$now" ]]; then
          days=$(( (expiry - now) / 86400 ))
          report OK '证书有效期' "至 $not_after（剩余 $days 天）"
        else
          report FAIL '证书有效期' "已于 $not_after 过期"
        fi
      fi
    else
      report WARN '证书有效期' '无法解析 notAfter'
    fi

    if [[ -n "$endpoint" && "$endpoint" == wss://* ]]; then
      host="${endpoint#wss://}"; host="${host%%[:/]*}"
      matched=""
      for san in $sans; do
        if [[ "$san" == "$host" ]]; then matched="$san"; break; fi
        if [[ "$san" == "*."* && "$host" == "${san#*.}" ]]; then matched="$san"; break; fi
      done
      if [[ -n "$matched" ]]; then
        report OK '证书 SAN' "覆盖 $host（SAN: $matched）"
      else
        report FAIL '证书 SAN' "不覆盖 $host（SAN: ${sans:-无}）"
      fi
    fi
  else
    report WARN '证书解析' '缺 openssl，跳过身份/有效期/SAN 校验'
  fi
fi

echo "-- 3/6 端点配置 --"
if [[ -n "$endpoint" ]]; then
  report OK 'CC_SIP_WSS_URL' "$endpoint"
else
  report FAIL 'CC_SIP_WSS_URL' '未配置（.env 缺 CC_SIP_WSS_URL，生产 HTTPS 下 WebPhone 不可用）'
fi

echo "-- 4/6 前端运行时注入 --"
fe_container="$(docker ps --format '{{.Names}}' | grep "^${FRONTEND_PREFIX}\." | head -1 || true)"
if [[ -n "$fe_container" ]]; then
  rc="$(docker exec "$fe_container" cat /usr/share/nginx/html/runtime-config.js 2>/dev/null || true)"
  if [[ "$rc" == *'"ccSipWsUrl":"'* ]]; then
    injected="$(grep -o '"ccSipWsUrl":"[^"]*"' <<<"$rc" | cut -d'"' -f4)"
    report OK 'runtime-config.js' "已注入 $injected"
  else
    report FAIL 'runtime-config.js' "$fe_container 的 runtime-config.js 无 ccSipWsUrl（前端未重启或变量未下发）"
  fi
else
  report FAIL '前端容器' "未找到 $FRONTEND_PREFIX 容器"
fi

echo "-- 5/6 严格验收器（复用 check-wss.mjs） --"
if [[ -n "$endpoint" && -f "$VERIFIER" ]]; then
  if have node; then
    if ver_out="$(node "$VERIFIER" "$endpoint" 2>&1)"; then
      report OK '严格 WSS 验收' "$(printf '%s' "$ver_out" | tr '\n' ' ')"
    else
      report FAIL '严格 WSS 验收' "$(printf '%s' "$ver_out" | sed -n 's/^\[cc-wss\] //p' | tr '\n' ' ')"
    fi
  else
    report WARN '严格 WSS 验收' '缺 node，跳过（可在有 node 的机器跑 npm run smoke:cc-wss）'
  fi
elif [[ -n "$endpoint" ]]; then
  report WARN '严格 WSS 验收' "验收器不存在：$VERIFIER"
else
  report WARN '严格 WSS 验收' '无端点，跳过'
fi

echo "-- 6/6 公网可达性 --"
if [[ -n "$endpoint" ]]; then
  report WARN '公网 NAT' "必须从公司网络之外实测 $endpoint 可达（本脚本在内网，不能代替）"
else
  report WARN '公网 NAT' '未配置端点，待证书/域名/NAT 到位后异网实测'
fi

echo
echo "== 汇总：OK=$ok  WARN=$warn  FAIL=$fail =="
if [[ "$fail" -gt 0 ]]; then
  echo "== 结论：存在 FAIL，生产 WSS 未就绪。按 deploy-and-debug.md §4.3 补齐。=="
  exit 1
fi
if [[ "$warn" -gt 0 ]]; then
  echo "== 结论：内网侧就绪，剩余为外部验证项（受信证书/公网 NAT/浏览器 CA）。=="
else
  echo "== 结论：内网侧全部通过。仍需异网实测与浏览器 CA 信任确认。=="
fi
exit 0
