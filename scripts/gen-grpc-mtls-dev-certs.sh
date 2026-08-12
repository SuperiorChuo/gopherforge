#!/usr/bin/env bash
# 生成本地/内网开发用 gRPC mTLS 证书（自签 CA + 服务端证书）。
# 不用于生产公网；生产请用内部 CA / cert-manager。
#
# 用法：
#   bash scripts/gen-grpc-mtls-dev-certs.sh [输出目录]
# 默认输出：./.local/grpc-mtls/
#
# 生成后导出：
#   export TLS_CA_PATH=.../ca.crt
#   export TLS_CERT_PATH=.../server.crt
#   export TLS_KEY_PATH=.../server.key
#   export TLS_SERVER_NAME=localhost   # 或证书 CN
#   # 生产强制：export GRPC_TLS_REQUIRED=1
set -euo pipefail

OUT="${1:-.local/grpc-mtls}"
mkdir -p "$OUT"
cd "$OUT"
OUT_ABS=$(pwd)

if ! command -v openssl >/dev/null 2>&1; then
  echo "需要 openssl" >&2
  exit 1
fi

# CA
openssl req -x509 -newkey rsa:2048 -nodes -keyout ca.key -out ca.crt -days 3650 \
  -subj "/CN=go-admin-kit-dev-ca" 2>/dev/null

# Server key + CSR
openssl req -newkey rsa:2048 -nodes -keyout server.key -out server.csr \
  -subj "/CN=localhost" 2>/dev/null

# Sign with SAN for localhost + common service DNS names (dev)
cat > server.ext <<'EOF'
subjectAltName = DNS:localhost,DNS:identity-service,DNS:go-admin-kit-identity,DNS:*.local,IP:127.0.0.1
extendedKeyUsage = serverAuth,clientAuth
EOF
openssl x509 -req -in server.csr -CA ca.crt -CAkey ca.key -CAcreateserial \
  -out server.crt -days 825 -extfile server.ext 2>/dev/null

rm -f server.csr server.ext ca.srl
chmod 600 server.key ca.key

echo "证书已生成：$OUT_ABS"
echo
echo "export TLS_CA_PATH=$OUT_ABS/ca.crt"
echo "export TLS_CERT_PATH=$OUT_ABS/server.crt"
echo "export TLS_KEY_PATH=$OUT_ABS/server.key"
echo "export TLS_SERVER_NAME=localhost"
echo "# 可选强制：export GRPC_TLS_REQUIRED=1"
