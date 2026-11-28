#!/usr/bin/env python3
"""输出安全扫描使用的全部已跟踪 Go 模块（仓库根相对路径）。"""

import argparse
import json
from pathlib import Path, PurePosixPath
import subprocess


def git_output(root, *args):
    result = subprocess.run(
        ["git", "-C", str(root), *args],
        capture_output=True, check=False, timeout=60,
    )
    if result.returncode:
        raise ValueError("Git 枚举失败：" + result.stderr.decode("utf-8", errors="replace").strip())
    return result.stdout.decode("utf-8")


def scan_modules(repository):
    root = Path(repository).resolve(strict=True)
    top = Path(git_output(root, "rev-parse", "--show-toplevel").rstrip("\n")).resolve(strict=True)
    if root != top:
        raise ValueError("必须指定仓库根目录，不能只扫描子目录")
    # NUL 分隔兼容空格路径；不读取 go.work，也不包含未跟踪的临时模块。
    output = git_output(root, "ls-files", "--cached", "--full-name", "-z", "--", ":(top,glob)**/go.mod")
    if not output:
        raise ValueError("未找到任何已跟踪的 go.mod")
    if not output.endswith("\0"):
        raise ValueError("Git 模块清单缺少 NUL 分隔符")

    modules = set()
    for name in output[:-1].split("\0"):
        relative = PurePosixPath(name)
        if (relative.is_absolute() or ".." in relative.parts or relative.name != "go.mod"
                or any(ord(char) < 32 or ord(char) == 127 for char in name)):
            raise ValueError("无效的已跟踪模块路径：" + repr(name))
        manifest = root / name
        resolved = manifest.resolve(strict=True)
        if not resolved.is_relative_to(root):
            raise ValueError("模块文件越出仓库目录：" + name)
        if manifest.is_symlink() or resolved != manifest or not manifest.is_file():
            raise ValueError("模块必须使用无符号链接的普通 go.mod 文件：" + name)
        module = relative.parent.as_posix()
        if module in modules:
            raise ValueError("Git 清单中重复的模块（请检查未解决的合并冲突）：" + module)
        modules.add(module)
    return sorted(modules)


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--root", default=Path(__file__).resolve().parents[1], type=Path)
    parser.add_argument("--lines", action="store_true", help="每行输出一个模块，供本地扫描入口使用")
    args = parser.parse_args()
    modules = scan_modules(args.root)
    print("\n".join(modules) if args.lines else json.dumps(modules))


if __name__ == "__main__":
    try:
        main()
    except (ValueError, OSError, RuntimeError, subprocess.SubprocessError) as error:
        raise SystemExit("Go 安全扫描模块发现失败：" + str(error))
