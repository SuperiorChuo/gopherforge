#!/bin/sh
set -eu

output=${GAK_RUNTIME_CONFIG_PATH:-/usr/share/nginx/html/runtime-config.js}
url=${CC_SIP_WSS_URL:-}

if [ -z "$url" ]; then
  printf '%s\n' 'window.__GAK_RUNTIME_CONFIG__ = Object.freeze({});' > "$output"
  exit 0
fi

case "$url" in
  wss://*) ;;
  *)
    echo "[runtime-config] CC_SIP_WSS_URL 必须使用 wss://" >&2
    exit 1
    ;;
esac

# 生产证书必须绑定 DNS 名称，因此这里只接受常规 DNS URL 字符；同时排除
# 引号、反斜杠和控制字符，保证生成的 JavaScript 不可被配置值注入。
if ! printf '%s\n' "$url" | grep -Eq '^wss://[A-Za-z0-9._~:/?&=@%+-]+$'; then
  echo "[runtime-config] CC_SIP_WSS_URL 含不支持的字符" >&2
  exit 1
fi

printf 'window.__GAK_RUNTIME_CONFIG__ = Object.freeze({"ccSipWsUrl":"%s"});\n' "$url" > "$output"
