#!/usr/bin/env bash
# 发版预检：打 tag 前的机械化门禁（历史上 README 版本口径连漏两轮、rc 残留漏过多处）。
# 用法：bash scripts/release-preflight.sh v0.4.0
# 全绿才允许 git tag；配合 release.yml 的 CHANGELOG 段落门禁使用。
set -euo pipefail
cd "$(git rev-parse --show-toplevel)"

VER="${1:?用法: release-preflight.sh vX.Y.Z}"
VER_NUM="${VER#v}"
hit=0
fail() { hit=1; echo "✗ $*"; }
ok() { echo "✓ $*"; }

# 1. CHANGELOG 必须已有本版本段落（release.yml 同款门禁，提前到本地）
if grep -q "^## \[$VER_NUM\]" CHANGELOG.md; then ok "CHANGELOG 有 [$VER_NUM] 段落"; else fail "CHANGELOG 缺 [$VER_NUM] 段落（先定稿：把 [Unreleased] 转为版本段并开新空段）"; fi

# 2. [Unreleased] 段落应已清空（防止定稿漏项）
unrel=$(awk '/^## \[Unreleased\]/{f=1;next} /^## \[/{f=0} f&&/^- |^### /' CHANGELOG.md | grep -v '^### *$' || true)
if [ -z "$unrel" ]; then ok "[Unreleased] 已清空"; else fail "[Unreleased] 还有未定稿条目：$(echo "$unrel" | head -2)"; fi

# 3. 门面与文档站无 rc 残留、无过时版本号（README 徽章是动态的不查）
stale=$(grep -rnE "v[0-9]+\.[0-9]+\.[0-9]+-rc\.[0-9]+" README.md README.en.md LOCAL_SETUP.md website/index.md website/en/index.md website/reference/deployment.md website/en/reference/deployment.md docs/ 2>/dev/null | head -5 || true)
if [ -z "$stale" ]; then ok "无 rc 版本残留"; else fail "rc 残留：$stale"; fi

# 4. IMAGE_TAG 示例应指向本版本（部署文档手动口径）
for f in website/reference/deployment.md website/en/reference/deployment.md docs/deployment.md; do
  [ -f "$f" ] || continue
  if grep -q "IMAGE_TAG=" "$f" && ! grep -q "IMAGE_TAG=$VER" "$f"; then
    fail "$f 的 IMAGE_TAG 示例不是 ${VER}（当前：$(grep -o 'IMAGE_TAG=v[0-9.]*' "$f" | head -1)）"
  fi
done
[ "$hit" = 0 ] && ok "IMAGE_TAG 示例口径一致"

# 5. 卫生门禁（与 pre-push 同一套）
if bash scripts/git-hooks/pre-push >/dev/null 2>&1; then ok "卫生扫描干净"; else fail "卫生扫描有命中（单独跑 scripts/git-hooks/pre-push 看详情）"; fi

# 6. 工作区干净（发版点必须是已提交状态）
if [ -z "$(git status --porcelain)" ]; then ok "工作区干净"; else fail "工作区有未提交改动"; fi

echo
if [ "$hit" = 0 ]; then
  echo "[preflight] 全绿，可打 tag：git tag $VER && git push origin $VER"
else
  echo "[preflight] 未通过，先处理上面 ✗ 项"
fi
exit $hit
