#!/usr/bin/env bash
# 安装项目 git hooks：把 scripts/git-hooks/* 指向 core.hooksPath。
# 一次执行，克隆后每人各自跑一遍（hooks 不随 git clone 自动生效）。
set -euo pipefail
repo_root=$(git rev-parse --show-toplevel)
cd "$repo_root"

hooks_dir="scripts/git-hooks"
chmod +x "$hooks_dir"/* 2>/dev/null || true

# 用 core.hooksPath 指向版本库内目录：随仓库更新，无需拷贝到 .git/hooks。
git config core.hooksPath "$hooks_dir"
echo "✅ 已设置 core.hooksPath=$hooks_dir"
echo "   pre-commit 敏感信息扫描已启用。绕过单次检查：git commit --no-verify"
