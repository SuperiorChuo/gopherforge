#!/usr/bin/env python3
"""Scan production Go files for unbounded GORM Find calls."""

from __future__ import annotations

import argparse
import re
import subprocess
import sys
from collections import Counter
from functools import lru_cache
from pathlib import Path
from typing import Callable

FIND_RE = re.compile(r"\.\s*Find\s*\(")
GUARD_RE = re.compile(r"\.\s*(Limit|First|Take|Count)\s*\(")
GOQUERY_IMPORT_RE = re.compile(r'^\s*(?:\w+\s+)?"github\.com/PuerkitoBio/goquery"', re.MULTILINE)
DOM_FIND_RE = re.compile(r'\.\s*Find\s*\(\s*(?:"(?:\\.|[^"\\])*"|`[^`]*`)\s*\)')
FUNCTION_RE = re.compile(r"^func\s+")
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
    has_goquery = GOQUERY_IMPORT_RE.search(text) is not None
    for i, line in enumerate(lines):
        candidate = line
        if i and lines[i - 1].rstrip().endswith("."):
            candidate = "." + line.lstrip()
        calls = list(FIND_RE.finditer(candidate))
        if not calls:
            continue
        # A single literal CSS argument is not a writable GORM destination.
        # Do not exempt directories, files, variables, or mixed DOM/DB lines.
        if has_goquery and all(DOM_FIND_RE.match(candidate, call.start()) for call in calls):
            continue
        start = max(0, i - 8)
        window = "\n".join(lines[start : i + 1])
        if NOLINT_RE.search(window):
            continue
        if GUARD_RE.search(window):
            continue
        hits.append(f"{rel}:{i + 1}")
    return hits


def added_findings(
    current: list[str], baseline: list[str],
    current_text: Callable[[str], str | None], baseline_text: Callable[[str], str | None],
) -> list[str]:
    """Compare callsites, not line numbers; retain multiplicity and old warnings.

    This is still a textual candidate scanner, not a Go type checker. Identity is
    path + enclosing function declaration + call line. A moved/renamed function,
    changed call or additional identical call must not inherit a blanket waiver.
    """
    def keys(hits: list[str], loader: Callable[[str], str | None]):
        read = lru_cache(maxsize=None)(loader)
        for hit in hits:
            rel, number = hit.rsplit(":", 1)
            text = read(rel)
            if text is None:
                raise ValueError(f"cannot read finding: {hit}")
            lines = text.splitlines()
            index = int(number) - 1
            function = "<file>"
            for line in reversed(lines[:index + 1]):
                if FUNCTION_RE.match(line):
                    function = line.strip()
                    break
            yield hit, (rel, function, lines[index].strip())

    remaining = Counter(key for _, key in keys(baseline, baseline_text))
    added: list[str] = []
    for hit, key in keys(current, current_text):
        if remaining[key]:
            remaining[key] -= 1
        else:
            added.append(hit)
    return sorted(added)


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
        added = added_findings(current, baseline,
                               lambda rel: staged_text(root, rel),
                               lambda rel: git_show(root, args.against or "HEAD", rel))
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
        added = added_findings(current, baseline,
                               lambda rel: (root / rel).read_text(encoding="utf-8"),
                               lambda rel: git_show(root, args.against, rel))
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
