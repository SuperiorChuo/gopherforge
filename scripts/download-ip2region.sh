#!/usr/bin/env bash
# 下载 ip2region 离线 IP 归属地数据文件（xdb，约 11MB，大文件不进 git）。
#
# 用法:
#   ./scripts/download-ip2region.sh              # 下到 microservices/data/ip2region.xdb
#   ./scripts/download-ip2region.sh /tmp/ip.xdb  # 指定输出路径
#
# 服务侧读取路径: 环境变量 IP2REGION_XDB，未设置时默认 ./data/ip2region.xdb
# （相对服务工作目录）。容器部署时把本文件挂载进容器并用 IP2REGION_XDB 指过去。
# 文件缺失时服务优雅降级（登录日志回退在线查询、其余归属地留空），不影响启动。
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
OUT="${1:-$ROOT/microservices/data/ip2region.xdb}"

# 官方仓库 master 分支 data/ 目录维护最新 xdb（IPv4 版 ip2region_v4.xdb）；
# 国内网络不通时依次尝试镜像。如需 IPv6 版换 ip2region_v6.xdb，加载端自动识别。
URLS=(
  "https://raw.githubusercontent.com/lionsoul2014/ip2region/master/data/ip2region_v4.xdb"
  "https://ghproxy.net/https://raw.githubusercontent.com/lionsoul2014/ip2region/master/data/ip2region_v4.xdb"
  "https://gitee.com/lionsoul/ip2region/raw/master/data/ip2region_v4.xdb"
)

mkdir -p "$(dirname "$OUT")"
TMP="$(mktemp)"
trap 'rm -f "$TMP"' EXIT

for url in "${URLS[@]}"; do
  echo "==> 尝试下载: $url"
  if curl -fSL --connect-timeout 10 --retry 2 -o "$TMP" "$url"; then
    SIZE=$(wc -c < "$TMP" | tr -d ' ')
    # 粗校验：正常 xdb 应有数 MB，明显偏小说明拿到的是错误页
    if [ "$SIZE" -gt 1000000 ]; then
      mv "$TMP" "$OUT"
      trap - EXIT
      echo "==> 完成: $OUT ($(du -h "$OUT" | cut -f1))"
      exit 0
    fi
    echo "==> 文件过小 (${SIZE}B)，疑似下载失败，换下一个源"
  fi
done

echo "!! 所有下载源均失败，请手动从 https://github.com/lionsoul2014/ip2region/tree/master/data 获取 ip2region.xdb" >&2
exit 1
