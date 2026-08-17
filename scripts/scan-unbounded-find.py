#!/usr/bin/env python3
"""Scan production Go files for unbounded GORM Find calls."""

from __future__ import annotations

import argparse
import re
import subprocess
import sys
from pathlib import Path

FIND_RE = re.compile(r"\.Find\s*\(")
GUARD_RE = re.compile(r"\.(Limit|First|Take|Count)\s*\(")
NOLINT_RE = re.compile(r"nolint:unbounded-find")
SKIP_DIR_PARTS = {
    "api/gen",
    "node_modules",
    "vendor",
    ".git",
}


def should_skip(path: Path) -> bool:
    posix = path.as_posix()
    if path.name.endswith("_test.go"):
        return True
    return any(part in posix for part in SKIP_DIR_PARTS)


def scan_text(rel: str, text: str) -> list[str]:
    lines = text.splitlines()
    hits: list[str] = []
    for i, line in enumerate(lines):
        if not FIND_RE.search(line):
            continue
        start = max(0, i - 8)
        window = "\n".join(lines[start : i + 1])
        if NOLINT_RE.search(window):
            continue
        if GUARD_RE.search(window):
            continue
        hits.append(f"{rel}:{i + 1}")
    return hits


def tracked_go_files(root: Path) -> list[str]:
    proc = subprocess.run(
        ["git", "-C", str(root), "ls-files", "*.go"],
        stdout=subprocess.PIPE,
        stderr=subprocess.DEVNULL,
        text=True,
    )
    if proc.returncode != 0:
        return []
    return [rel for rel in proc.stdout.splitlines() if not should_skip(Path(rel))]


def scan_tree(root: Path) -> list[str]:
    hits: list[str] = []
    for rel in tracked_go_files(root):
        path = root / rel
        if not path.is_file():
            continue
        hits.extend(scan_text(rel, path.read_text(encoding="utf-8", errors="ignore")))
    return sorted(hits)


def git_show(root: Path, ref: str, rel: str) -> str | None:
    proc = subprocess.run(
        ["git", "-C", str(root), "show", f"{ref}:{rel}"],
        stdout=subprocess.PIPE,
        stderr=subprocess.DEVNULL,
        text=True,
    )
    if proc.returncode != 0:
        return None
    return proc.stdout


def git_ls_tree(root: Path, ref: str) -> list[str]:
    proc = subprocess.run(
        ["git", "-C", str(root), "ls-tree", "-r", "--name-only", ref],
        stdout=subprocess.PIPE,
        stderr=subprocess.DEVNULL,
        text=True,
        check=True,
    )
    return [line for line in proc.stdout.splitlines() if line.endswith(".go")]


def scan_ref(root: Path, ref: str) -> list[str]:
    hits: list[str] = []
    for rel in git_ls_tree(root, ref):
        path = Path(rel)
        if should_skip(path):
            continue
        text = git_show(root, ref, rel)
        if text is None:
            continue
        hits.extend(scan_text(rel, text))
    return sorted(hits)


def staged_text(root: Path, rel: str) -> str | None:
    proc = subprocess.run(
        ["git", "-C", str(root), "show", f":{rel}"],
        stdout=subprocess.PIPE,
        stderr=subprocess.DEVNULL,
        text=True,
    )
    if proc.returncode != 0:
        return None
    return proc.stdout


def scan_staged(root: Path) -> list[str]:
    proc = subprocess.run(
        ["git", "-C", str(root), "diff", "--cached", "--name-only", "--diff-filter=ACM"],
        stdout=subprocess.PIPE,
        text=True,
        check=True,
    )
    hits: list[str] = []
    for rel in proc.stdout.splitlines():
        path = Path(rel)
        if not rel.endswith(".go") or should_skip(path):
            continue
        text = staged_text(root, rel)
        if text is None:
            continue
        hits.extend(scan_text(rel, text))
    return sorted(hits)


def main() -> int:
    parser = argparse.ArgumentParser()
    parser.add_argument("--root", default=".")
    parser.add_argument("--against")
    parser.add_argument("--staged", action="store_true")
    args = parser.parse_args()

    root = Path(args.root).resolve()
    if args.staged:
        current = scan_staged(root)
        baseline = scan_ref(root, args.against or "HEAD")
        added = sorted(set(current) - set(baseline))
        if added:
            print(f"新增无界 Find {len(added)} 处（相对 {args.against or 'HEAD'}）：")
            print("\n".join(added))
            print("给查询加 Limit/First/Take/Count，或在邻近 8 行写 //nolint:unbounded-find")
            return 1
        print(f"无新增无界 Find（暂存 {len(current)} / 基线 {len(baseline)}）")
        return 0

    if args.against:
        current = scan_tree(root)
        baseline = scan_ref(root, args.against)
        added = sorted(set(current) - set(baseline))
        if added:
            print(f"新增无界 Find {len(added)} 处（相对 {args.against}）：")
            print("\n".join(added))
            print("给查询加 Limit/First/Take/Count，或在邻近 8 行写 //nolint:unbounded-find")
            return 1
        print(f"无新增无界 Find（工作区 {len(current)} / {args.against} {len(baseline)}）")
        return 0

    hits = scan_tree(root)
    print(f"无界 Find 候选 {len(hits)} 处（存量只列不改）")
    if hits:
        print("\n".join(hits))
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
