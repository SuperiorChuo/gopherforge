#!/usr/bin/env bash
# 工程验证脚本的正负控及真实目录检查；不依赖本地治理配置。
set -euo pipefail
ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT"
for script in check-code-layout govulncheck verify-checks; do
  bash -n "scripts/$script.sh"
done
python3 -B -m unittest -v scripts/tests/test_code_layout.py scripts/tests/test_govulncheck.py
bash scripts/check-code-layout.sh
