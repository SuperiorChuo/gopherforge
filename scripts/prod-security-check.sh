#!/usr/bin/env bash
# 生产硬化自检：检查关键密钥/ACL/mTLS 配置是否就绪。
# 不打印密钥内容，只输出 ok / fail / skip。
#
# 用法：
#   bash scripts/prod-security-check.sh              # 本机当前 env + /run/secrets
#   bash scripts/prod-security-check.sh --strict     # APP_ENV 必须为 production，且强制 ACL/mTLS 检查
#   REQUIRE_CONSUL_ACL=1 REQUIRE_GRPC_MTLS=1 bash scripts/prod-security-check.sh
#
# 退出码：0 全部通过；1 有 fail；2 用法错误
set -euo pipefail

STRICT=0
for arg in "$@"; do
  case "$arg" in
    --strict) STRICT=1 ;;
    -h|--help)
      sed -n '2,12p' "$0"
      exit 0
      ;;
    *)
      echo "unknown arg: $arg" >&2
      exit 2
      ;;
  esac
done

SECRETS_DIR="${SECRETS_DIR:-/run/secrets}"
FAILS=0
WARNS=0

# 读敏感值：文件优先（与 shared/pkg/envsecret 命名一致），不 echo 值。
read_secret() {
  local key="$1"
  local lower dashed prefixed
  lower=$(printf '%s' "$key" | tr '[:upper:]' '[:lower:]')
  dashed=${lower//_/-}
  prefixed="go-admin-kit-${dashed}"
  local p
  for p in "${SECRETS_DIR}/${lower}" "${SECRETS_DIR}/${dashed}" "${SECRETS_DIR}/${prefixed}"; do
    if [[ -f "$p" ]]; then
      # shellcheck disable=SC2002
      local v
      v=$(tr -d '\r\n' <"$p" | sed 's/^[[:space:]]*//;s/[[:space:]]*$//')
      if [[ -n "$v" ]]; then
        printf '%s' "$v"
        return 0
      fi
    fi
  done
  if [[ -n "${!key-}" ]]; then
    printf '%s' "${!key}"
    return 0
  fi
  return 1
}

is_weak() {
  local v="$1"
  case "$v" in
    ""|123456|password|admin|minioadmin|change-me|changeme|secret|redis|postgres)
      return 0 ;;
  esac
  local lower
  lower=$(printf '%s' "$v" | tr '[:upper:]' '[:lower:]')
  case "$lower" in
    *placeholder*|*change-me*|*changeme*|*example*|*local-dev*)
      return 0 ;;
  esac
  return 1
}

ok() { printf '  [ok]   %s\n' "$1"; }
fail() { printf '  [fail] %s\n' "$1"; FAILS=$((FAILS + 1)); }
warn() { printf '  [warn] %s\n' "$1"; WARNS=$((WARNS + 1)); }
skip() { printf '  [skip] %s\n' "$1"; }

echo "== 生产硬化检查 =="
echo "secrets_dir=${SECRETS_DIR}"

APP_ENV_VAL="${APP_ENV:-}"
if [[ -z "$APP_ENV_VAL" ]]; then
  if [[ "$STRICT" -eq 1 ]]; then
    fail "APP_ENV 未设置（strict 要求 production）"
  else
    warn "APP_ENV 未设置（开发环境可忽略）"
  fi
elif [[ "$APP_ENV_VAL" == "production" || "$APP_ENV_VAL" == "prod" ]]; then
  ok "APP_ENV=${APP_ENV_VAL}"
else
  if [[ "$STRICT" -eq 1 ]]; then
    fail "APP_ENV=${APP_ENV_VAL}（strict 要求 production）"
  else
    warn "APP_ENV=${APP_ENV_VAL}（非 production，弱密钥检查仍会报告）"
  fi
fi

# JWT
if JWT_VAL=$(read_secret JWT_SECRET 2>/dev/null); then
  if is_weak "$JWT_VAL"; then
    fail "JWT_SECRET 为空、默认或占位符"
  elif [[ ${#JWT_VAL} -lt 32 ]]; then
    fail "JWT_SECRET 长度 ${#JWT_VAL} < 32"
  else
    ok "JWT_SECRET 已配置且长度≥32"
  fi
else
  fail "JWT_SECRET 未配置（env 或 ${SECRETS_DIR}）"
fi

# DB
if DB_VAL=$(read_secret DB_PASSWORD 2>/dev/null); then
  if is_weak "$DB_VAL"; then
    fail "DB_PASSWORD 为空、默认或弱口令"
  else
    ok "DB_PASSWORD 已配置且非弱口令"
  fi
else
  # 兼容 POSTGRES_PASSWORD
  if DB_VAL=$(read_secret POSTGRES_PASSWORD 2>/dev/null); then
    if is_weak "$DB_VAL"; then
      fail "POSTGRES_PASSWORD 为空、默认或弱口令"
    else
      ok "POSTGRES_PASSWORD 已配置且非弱口令"
    fi
  else
    fail "DB_PASSWORD / POSTGRES_PASSWORD 未配置"
  fi
fi

# Redis
if REDIS_VAL=$(read_secret REDIS_PASSWORD 2>/dev/null); then
  if is_weak "$REDIS_VAL"; then
    fail "REDIS_PASSWORD 为空、默认或弱口令"
  else
    ok "REDIS_PASSWORD 已配置且非弱口令"
  fi
else
  fail "REDIS_PASSWORD 未配置"
fi

# Consul ACL
REQUIRE_CONSUL_ACL="${REQUIRE_CONSUL_ACL:-0}"
if [[ "$STRICT" -eq 1 ]]; then
  REQUIRE_CONSUL_ACL=1
fi
if TOKEN_VAL=$(read_secret CONSUL_HTTP_TOKEN 2>/dev/null); then
  if [[ -n "$TOKEN_VAL" ]]; then
    ok "CONSUL_HTTP_TOKEN 已配置"
  else
    fail "CONSUL_HTTP_TOKEN 为空"
  fi
else
  if [[ "$REQUIRE_CONSUL_ACL" == "1" ]]; then
    fail "CONSUL_HTTP_TOKEN 未配置（Consul ACL 已要求）"
  else
    skip "CONSUL_HTTP_TOKEN 未配置（开发默认可跳过；生产设 REQUIRE_CONSUL_ACL=1）"
  fi
fi

# gRPC mTLS
REQUIRE_GRPC_MTLS="${REQUIRE_GRPC_MTLS:-0}"
if [[ "$STRICT" -eq 1 ]]; then
  REQUIRE_GRPC_MTLS=1
fi
TLS_CA="${TLS_CA_PATH:-}"
TLS_CERT="${TLS_CERT_PATH:-}"
TLS_KEY="${TLS_KEY_PATH:-}"
if [[ -n "$TLS_CA" && -f "$TLS_CA" ]]; then
  ok "TLS_CA_PATH 可读"
  if [[ -n "$TLS_CERT" && -f "$TLS_CERT" && -n "$TLS_KEY" && -f "$TLS_KEY" ]]; then
    ok "TLS_CERT_PATH / TLS_KEY_PATH 可读（服务端 mTLS 齐备）"
  else
    warn "仅 CA 就绪（客户端可拨 TLS；服务端证书未齐）"
  fi
else
  if [[ "$REQUIRE_GRPC_MTLS" == "1" ]]; then
    fail "TLS_CA_PATH 未设置或不可读（gRPC mTLS 已要求）"
  else
    skip "TLS_CA_PATH 未设置（开发明文；生产设 REQUIRE_GRPC_MTLS=1 或 TLS_*）"
  fi
fi

# 强制改密
if [[ "${DEFAULT_ADMIN_FORCE_CHANGE_PASSWORD:-}" == "true" || "${DEFAULT_ADMIN_FORCE_CHANGE_PASSWORD:-}" == "1" ]]; then
  ok "DEFAULT_ADMIN_FORCE_CHANGE_PASSWORD 已开启"
else
  if [[ "$STRICT" -eq 1 || "${APP_ENV_VAL}" == "production" || "${APP_ENV_VAL}" == "prod" ]]; then
    warn "DEFAULT_ADMIN_FORCE_CHANGE_PASSWORD 未开启（生产建议 true）"
  else
    skip "DEFAULT_ADMIN_FORCE_CHANGE_PASSWORD 未开启"
  fi
fi

echo
if [[ "$FAILS" -gt 0 ]]; then
  echo "结果: FAIL (${FAILS} fail, ${WARNS} warn)"
  exit 1
fi
echo "结果: PASS (${WARNS} warn)"
exit 0
