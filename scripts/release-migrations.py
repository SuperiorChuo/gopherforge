#!/usr/bin/env python3
"""冻结公开 SQL 的编号与原文；仅检查历史身份，不替代真实数据库升级演练。"""

import argparse
import hashlib
import os
from pathlib import Path
import re
import subprocess
import sys

ROOT = Path(__file__).resolve().parents[1]
MIGRATION_PATH = "microservices/services/monitor/migrations"
# 固定公共提交而非可移动 tag：包含 v0.7.0 的 51 份 SQL，以及随后公开的 000081。
PUBLIC_BASELINE = "26118eae783e12a3d9f5f24bcae9908a5eaaedcc"
SQL_NAME = re.compile(r"([0-9]{6})_[a-z0-9_]+\.sql\Z")


def load_baseline(repository: Path, commit: str = PUBLIC_BASELINE) -> dict:
    # 不继承外部 GIT_DIR/index/replace 设置，避免把其他工作区误认作身份真源。
    env = {key: value for key, value in os.environ.items() if not key.startswith("GIT_")}
    env.update(GIT_NO_LAZY_FETCH="1", GIT_CONFIG_NOSYSTEM="1", GIT_CONFIG_GLOBAL=os.devnull)

    def git(*args):
        try:
            return subprocess.run(
                ["git", "--no-replace-objects", "-C", str(repository), *args],
                env=env, capture_output=True, check=True, timeout=30,
            ).stdout
        except (OSError, subprocess.SubprocessError) as error:
            # 不打印 Git 原始输出或 SQL；浅克隆缺基线不能静默通过，也不自动联网补取。
            raise ValueError("基线 Git 对象不可读取；请在获准入口取得完整公开历史") from error

    paths = git("ls-tree", "-rz", "--name-only", commit, "--", MIGRATION_PATH).decode("utf-8").split("\0")
    expected = {}
    for path in paths:
        file = Path(path)
        if not file.name.lower().endswith(".sql"):
            continue
        if str(file.parent) != MIGRATION_PATH or not SQL_NAME.fullmatch(file.name):
            raise ValueError("公开基线的迁移路径不符合约定")
        expected[file.name] = hashlib.sha256(git("cat-file", "blob", f"{commit}:{path}")).hexdigest()
    if not expected:
        raise ValueError("公开迁移基线为空，拒绝空集通过")
    return expected


def check_history(directory: Path, expected: dict) -> list:
    if not expected:
        raise ValueError("公开迁移基线为空，拒绝空集通过")
    if not directory.is_dir():
        raise ValueError("迁移目录不存在")
    highest = max(int(SQL_NAME.fullmatch(name).group(1)) for name in expected)
    issues, seen, versions = [], set(), {}
    for path in sorted(directory.iterdir()):
        # 与 *.sql 的发现范围对齐；Path('.sql').suffix 为空，不能因此漏过非法名称。
        if not path.name.lower().endswith(".sql"):
            continue
        name = path.name
        seen.add(name)
        match = SQL_NAME.fullmatch(name)
        if not match:
            issues.append(f"invalid-name: {name!r}")
            continue
        version = int(match.group(1))
        if version in versions:
            issues.append(f"duplicate-version: {name!r} / {versions[version]!r}")
        versions[version] = name
        # Goose 按编号识别历史，旧缺号不是可重用槽位；同号改名也必须拒绝。
        if name not in expected and version <= highest:
            issues.append(f"historical-insert: {name!r}")
        if path.is_symlink() or not path.is_file():
            issues.append(f"not-regular: {name!r}")
            continue
        if name in expected:
            digest = hashlib.sha256()
            with path.open("rb") as stream:
                for chunk in iter(lambda: stream.read(65536), b""):
                    digest.update(chunk)
            if digest.hexdigest() != expected[name]:
                issues.append(f"changed: {name!r}")
    issues.extend(f"missing: {name!r}" for name in sorted(set(expected) - seen))
    return issues


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--directory", type=Path, default=ROOT / MIGRATION_PATH,
                        help="待核对的公开候选迁移目录（只读）")
    args = parser.parse_args()
    try:
        expected = load_baseline(ROOT)
        issues = check_history(args.directory, expected)
    except (OSError, ValueError) as error:
        print(f"公开迁移身份检查失败: {error}", file=sys.stderr)
        return 2
    if issues:
        print(f"公开迁移身份拒绝: {len(issues)} 项", file=sys.stderr)
        for issue in issues:
            print(issue, file=sys.stderr)
        return 1
    print(f"公开迁移身份通过: {len(expected)} 份历史 SQL 原文不变，baseline={PUBLIC_BASELINE}")
    print("仅为静态身份检查；fresh、带数据升级、序列与外键仍需独立演练。")
    return 0


if __name__ == "__main__":
    sys.exit(main())
