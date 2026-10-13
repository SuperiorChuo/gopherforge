#!/usr/bin/env python3
"""公开候选 Git 对象卫生检查；不扫未跟踪文件、不输出命中内容、不连接网络。"""

import argparse
import io
import json
import os
from pathlib import Path, PurePosixPath
import re
import shutil
import subprocess
import sys

ROOT = Path(__file__).resolve().parents[1]
PUBLIC_BASELINE = "26118eae783e12a3d9f5f24bcae9908a5eaaedcc"
OID = re.compile(r"(?:[0-9a-f]{40}|[0-9a-f]{64})\Z")
MAX_BLOB = 16 * 1024 * 1024
MAX_TOTAL = 256 * 1024 * 1024
INTERNAL_PREFIXES = (".pi/", ".agents/", ".claude/", ".harness/", "docs/worklog/", "scripts/evolution/")
INTERNAL_NAMES = {"AGENTS.md", "CLAUDE.md", "GEMINI.md", "PROJECT.md", "PROJECTS.md", "SYNC_LOG.md", "WORKFLOW.md"}
INTERNAL_REFERENCE = re.compile(rb"AGENTS\.md|CLAUDE\.md|CODEX_|SYNC_LOG|WORKFLOW\.md|PROJECTS?\.md|scripts/evolution|(?:^|[\s/'\"])\.(?:pi|harness|agents)/")
INTERNAL_LOCATION = re.compile(rb"/www/|root@|/Users/[^\s/]+/|/home/[^\s/]+/(?:Desktop|projects)/")
SECRET = re.compile(rb"sk-[a-zA-Z0-9]{20,}|AKIA[0-9A-Z]{16}|gh[pousr]_[a-zA-Z0-9]{20,}|github_pat_[a-zA-Z0-9_]{20,}|xox[bp]-")
# 保留公开门禁对真实 PEM 行的拒绝；行内不完整的单测占位文本不是私钥。
PRIVATE_KEY = re.compile(rb"(?:^|\n)-----BEGIN (?:RSA |EC |OPENSSH |ENCRYPTED )?PRIVATE KEY-----\r?(?:\n|$)")
# 只豁免规则定义本身的内部术语；密钥检测对这些文件仍然无条件生效。
RULE_FILES = {"scripts/release-hygiene.py", "scripts/tests/test_release_hygiene.py"}


def git(repository, *args, data=None):
    # 构造最小 Git 环境，不转发外部 GIT_DIR/替换对象/配置注入，也不读取凭据。
    executable = shutil.which("git")
    if not executable:
        raise RuntimeError("缺少 Git")
    environment = {"PATH": os.defpath, "GIT_CONFIG_NOSYSTEM": "1", "GIT_CONFIG_GLOBAL": os.devnull,
                   "GIT_NO_LAZY_FETCH": "1", "GIT_NO_REPLACE_OBJECTS": "1",
                   "GIT_GRAFT_FILE": os.devnull}  # legacy grafts 同样会伪造祖先关系，必须禁用。
    try:
        return subprocess.run([executable, "--no-replace-objects", "-C", str(repository), *args],
                              input=data, capture_output=True, check=True, timeout=60, env=environment).stdout
    except (subprocess.SubprocessError, OSError) as error:
        # stderr 可能带文件内容、地址或配置值，错误只报告分类，绝不回显原文。
        raise RuntimeError("Git 对象读取或历史关系检查失败") from error


def resolve_commit(repository, revision):
    if revision != "HEAD" and not OID.fullmatch(revision):
        raise ValueError("revision 必须为完整对象 ID 或 HEAD")
    commit = git(repository, "rev-parse", "--verify", f"{revision}^{{commit}}").decode().strip()
    if not OID.fullmatch(commit):
        raise ValueError("Git 返回非法 commit ID")
    return commit


def path_issue(name):
    # 路径本身也会公开；命中凭据形态时不再读取其内容。
    if SECRET.search(name.encode("utf-8")):
        return "credential-path"
    path = PurePosixPath(name)
    if name.startswith(INTERNAL_PREFIXES) or name in {prefix.rstrip("/") for prefix in INTERNAL_PREFIXES} or path.name in INTERNAL_NAMES or path.name.startswith("CODEX_"):
        return "internal-path"
    if path.name == ".env" or (path.name.startswith(".env.") and path.name != ".env.example"):
        return "environment-file"
    if path.name in {"id_rsa", "id_ed25519", "id_ecdsa", ".netrc", ".npmrc"} or path.suffix.lower() in {".key", ".pem", ".p12", ".pfx", ".p8", ".jks", ".keystore", ".kdbx"}:
        return "credential-file"
    return None


def scan_commit(repository, revision, seen=None):
    """检查指定提交的原始 blob，而不是当前 checkout/index；seen 仅复用本次相同路径＋对象。"""
    commit = resolve_commit(repository, revision)
    # -t 必须保留目录项：Git 可表示空子 tree，其目录名同样会被传输。
    entries = git(repository, "ls-tree", "-rtlz", commit).split(b"\0")
    if entries == [b""] or len(entries) > 20001:
        raise ValueError("空 tree 或 tree 超出 20000 路径上限")
    findings, pending = [], []
    seen = seen if seen is not None else set()
    total = 0
    for entry in entries:
        if not entry:
            continue
        metadata, raw_name = entry.split(b"\t", 1)
        mode, kind, oid, raw_size = metadata.split()
        name = raw_name.decode("utf-8", errors="strict")
        issue = path_issue(name)
        if issue:
            findings.append((issue, name))
            continue  # 敏感路径在读取 blob 之前拒绝，不打开真实环境或私钥文件。
        if mode == b"040000" and kind == b"tree":
            continue  # 目录路径已查，其子条目由同次递归遍历列出，不把目录当 blob。
        if mode not in (b"100644", b"100755") or kind != b"blob":
            findings.append(("not-regular", name))
            continue
        size = int(raw_size)
        total += size
        if size < 0 or size > MAX_BLOB or total > MAX_TOTAL:
            raise ValueError("对象超出 16MiB 单文件 / 256MiB tree 检查上限，拒绝漏扫")
        if not OID.fullmatch(oid.decode("ascii")):
            raise ValueError("非法 blob ID")
        if (name, oid) not in seen:
            pending.append((name, oid, size))

    if not pending:
        return findings
    # 一次批量读取原文；不走 checkout/filter，不跳过二进制文件。大小已由 tree 元数据限定。
    raw = git(repository, "cat-file", "--batch", data=b"".join(oid + b"\n" for _, oid, _ in pending))
    stream = io.BytesIO(raw)
    for name, oid, size in pending:
        if stream.readline() != oid + b" blob " + str(size).encode() + b"\n":
            raise ValueError("blob 响应与 tree 身份不一致")
        content = stream.read(size)
        if len(content) != size or stream.read(1) != b"\n":
            raise ValueError("blob 响应不完整")
        for code, pattern in [("secret-shape", SECRET), ("private-key", PRIVATE_KEY)]:
            if pattern.search(content):
                findings.append((code, name))
        if name not in RULE_FILES:
            if name != ".gitignore" and INTERNAL_REFERENCE.search(content):
                findings.append(("internal-reference", name))
            if INTERNAL_LOCATION.search(content):
                findings.append(("internal-location", name))
        seen.add((name, oid))
    if stream.read(1):
        raise ValueError("blob 响应存在多余数据")
    return findings


def push_revisions(repository, stream, baseline=PUBLIC_BASELINE):
    """冻结公开基线之后每一条提交；清理 tip 不能掩盖中间历史泄漏。"""
    baseline = resolve_commit(repository, baseline)
    revisions = set()
    for index, line in enumerate(stream):
        fields = line.split()
        if index >= 64 or len(line) > 4096 or len(fields) != 4:
            raise ValueError("非法或过大的 pre-push 输入")
        _, local, _, remote = fields
        if not OID.fullmatch(local) or not OID.fullmatch(remote):
            raise ValueError("pre-push 对象 ID 非法")
        if set(local) == {"0"}:
            continue  # 删除引用不传输 blob；是否允许删除由发布权限层决定。
        tip = resolve_commit(repository, local)
        git(repository, "merge-base", "--is-ancestor", baseline, tip)
        introduced = git(repository, "rev-list", "--max-count=501", tip, "^" + baseline).decode().splitlines()
        revisions.update(introduced or [tip])
        if len(revisions) > 500:
            raise ValueError("待审历史超过 500 个提交，拒绝截断")
    # 不允许把不源于公开基线的私有历史 merge 进来，即使 merge tip 本身已清理。
    for commit in revisions:
        git(repository, "merge-base", "--is-ancestor", baseline, commit)
    return sorted(revisions)


def safe_name(name):
    # 路径也可能含 token 或控制字符；输出 JSON 转义，token 形态路径不回显。
    if SECRET.search(name.encode("utf-8")):
        return '"<redacted-path>"'
    return json.dumps(name, ensure_ascii=True)


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--revision", default="HEAD")
    parser.add_argument("--pre-push", action="store_true")
    args = parser.parse_args()
    try:
        revisions = push_revisions(ROOT, sys.stdin) if args.pre_push else [resolve_commit(ROOT, args.revision)]
        seen, findings = set(), set()
        for revision in revisions:
            findings.update(scan_commit(ROOT, revision, seen))
        for code, name in sorted(findings):
            print(f"{code}: {safe_name(name)}")
        if findings:
            print(f"公开内容检查拒绝：{len(findings)} 项；只报告类别和路径，未输出命中内容。")
            return 1
        print(f"公开内容检查通过：{len(revisions)} 个提交；未新增对象不等于发布权限批准。")
        return 0
    except (RuntimeError, ValueError, OSError) as error:
        # Unicode/Git 等异常本身可能包含敏感输入，只返回固定分类，不展开异常对象。
        print(f"公开内容检查无法完成：{type(error).__name__}；缺对象、非法历史或资源超限均拒绝。", file=sys.stderr)
        return 2


if __name__ == "__main__":
    raise SystemExit(main())
