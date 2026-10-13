#!/usr/bin/env python3
"""输出发布使用的 Go workspace 模块清单。"""

import argparse
import json
import os
from pathlib import Path
import subprocess


def workspace_modules(workspace):
    root = Path(workspace).resolve(strict=True)
    workfile = root / "go.work"
    if not workfile.is_file():
        raise ValueError("workspace 缺少 go.work")

    # 让 Go 自己解析单行、块式 use、引号和注释；无效语法必须阻止发版。
    result = subprocess.run(
        ["go", "work", "edit", "-json"], cwd=root,
        env={**os.environ, "GOWORK": str(workfile)},
        check=False, capture_output=True, text=True, timeout=60,
    )
    if result.returncode != 0:
        raise ValueError("go work edit -json 失败：" + result.stderr.strip())
    definition = json.loads(result.stdout)
    modules, identities = set(), set()
    for item in definition.get("Use") or []:
        path = (root / item["DiskPath"]).resolve()
        # 矩阵只允许产品 workspace 内的真实模块，拒绝 ../ 与 symlink 逃逸。
        try:
            relative = path.relative_to(root).as_posix()
        except ValueError as error:
            raise ValueError("workspace 模块越出产品目录") from error
        if any(ord(char) < 32 or ord(char) == 127 for char in relative):
            raise ValueError("模块路径不得包含控制字符")
        if not (path / "go.mod").is_file() or (path / "go.mod").is_symlink():
            raise ValueError("模块缺少普通文件 go.mod：" + relative)
        if relative in modules:
            raise ValueError("workspace 重复声明模块：" + relative)
        # 目录不同不代表模块不同；必须解析 go.mod 的真实 module 身份。
        parsed = subprocess.run(
            ["go", "mod", "edit", "-json"], cwd=path,
            env={**os.environ, "GOWORK": "off"},
            check=False, capture_output=True, text=True, timeout=60,
        )
        if parsed.returncode != 0:
            raise ValueError("go.mod 解析失败：" + relative + "：" + parsed.stderr.strip())
        identity = (json.loads(parsed.stdout).get("Module") or {}).get("Path")
        if not identity:
            raise ValueError("go.mod 缺少 module 声明：" + relative)
        if identity in identities:
            raise ValueError("workspace 中不同目录重复声明 module：" + identity)
        identities.add(identity)
        modules.add(relative)
    if not modules:
        raise ValueError("未从 go.work 解析出任何模块")
    return sorted(modules)


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--workspace", default="microservices")
    parser.add_argument("--lines", action="store_true")
    args = parser.parse_args()
    modules = workspace_modules(args.workspace)
    print("\n".join(modules) if args.lines else json.dumps(modules))


if __name__ == "__main__":
    try:
        main()
    except (ValueError, OSError, subprocess.SubprocessError) as error:
        raise SystemExit(str(error))
