"""用真实 Git 对象验证公开内容门禁；所有密钥形态均为现场拼接的无效测试样例。"""

import importlib.util
import io
from pathlib import Path
import subprocess
import tempfile
import unittest
from unittest.mock import patch

ROOT = Path(__file__).resolve().parents[2]
SPEC = importlib.util.spec_from_file_location("release_hygiene", ROOT / "scripts/release-hygiene.py")
MODULE = importlib.util.module_from_spec(SPEC)
SPEC.loader.exec_module(MODULE)


class HygieneTests(unittest.TestCase):
    def setUp(self):
        self.tmp = tempfile.TemporaryDirectory(prefix="go-admin-kit-hygiene-")
        self.addCleanup(self.tmp.cleanup)
        self.root = Path(self.tmp.name)
        self.git("init", "-q")
        self.git("config", "user.name", "Release Test")
        self.git("config", "user.email", "release-test@example.invalid")
        self.write("README.md", b"Public product\n")
        self.base = self.commit()

    def git(self, *args):
        return subprocess.run(["git", *args], cwd=self.root, capture_output=True, check=True).stdout.decode().strip()

    def write(self, name, content):
        path = self.root / name
        path.parent.mkdir(parents=True, exist_ok=True)
        path.write_bytes(content)

    def commit(self):
        self.git("add", "--all")
        self.git("commit", "-qm", "test fixture")
        return self.git("rev-parse", "HEAD")

    def test_approved_business_and_schemas_are_public(self):
        self.write("freeswitch-cc/README.md", b"cc-service crm-service im-service mp-service pay-service ticket-service visibility-service")
        self.write("microservices/services/monitor/migrations/000082_business.sql", b"CREATE SCHEMA IF NOT EXISTS ai_svc; CREATE SCHEMA IF NOT EXISTS crm_svc;")
        self.write("microservices/configs/.env.example", b"DB_PASSWORD=replace-me\n")
        self.write("website/index.md", b"Public docs and Demo")
        self.write("README.en.md", b"Full product")
        self.assertEqual(MODULE.scan_commit(self.root, self.commit()), [])

    def test_internal_files_are_rejected_without_content_read(self):
        paths = [".pi/state.json", ".agents/skills/test.md", "docs/worklog/test.md", "scripts/evolution/test.mjs", "PROJECT.md", "SYNC_LOG.md"]
        for name in paths:
            self.write(name, b"otherwise harmless")
        findings = MODULE.scan_commit(self.root, self.commit())
        for name in paths:
            self.assertIn(("internal-path", name), findings)

    def test_real_environment_paths_are_rejected(self):
        for name in [".env", "web/.env.production", "web/.env.local"]:
            self.write(name, b"placeholder only")
        self.assertEqual(sum(code == "environment-file" for code, _ in MODULE.scan_commit(self.root, self.commit())), 3)

    def test_key_paths_are_rejected(self):
        self.write("config/id_ed25519", b"placeholder only")
        self.write("config/tls.key", b"placeholder only")
        self.assertEqual(sum(code == "credential-file" for code, _ in MODULE.scan_commit(self.root, self.commit())), 2)

    def test_internal_references_and_host_paths_are_rejected(self):
        self.write("docs/guide.md", b"Read AGENTS.md and scripts/evolution/check.mjs\n")
        self.write("docs/ops.md", b"/Users/" + b"example/project\n/www/" + b"app\nroot" + b"@example.invalid")
        findings = MODULE.scan_commit(self.root, self.commit())
        self.assertIn(("internal-reference", "docs/guide.md"), findings)
        self.assertIn(("internal-location", "docs/ops.md"), findings)

    def test_credential_detection_has_no_whole_file_allowlist(self):
        # 旧扫描把测试文件、CI 等整文件豁免；这里连自检脚本也不能藏 token。
        names = [".gitignore", ".github/workflows/ci.yml", "scripts/release-hygiene.py", "microservices/services/shared/pkg/mask/mask_test.go"]
        for name in names:
            self.write(name, b"sk-" + b"A" * 32)
        findings = MODULE.scan_commit(self.root, self.commit())
        for name in names:
            self.assertIn(("secret-shape", name), findings)

    def test_credentials_in_filename_are_rejected_and_redacted(self):
        # Git 会传输文件名；只查内容会遗漏藏在路径里的凭据。
        fake = "ghp_" + "A" * 32
        name = "docs/" + fake + ".txt"
        self.write(name, b"otherwise public")
        self.assertTrue(("credential-path", name) in MODULE.scan_commit(self.root, self.commit()))
        self.assertEqual(MODULE.safe_name(name), '"<redacted-path>"')

    def test_hook_is_anchored_to_its_own_repository(self):
        # 外部 Git 环境不应把预检转向另一仓库，甚至执行另一仓库的脚本。
        for name in ["scripts/release-hygiene.py", "scripts/git-hooks/pre-push"]:
            self.write(name, (ROOT / name).read_bytes())
        self.commit()
        environment = {"PATH": "/usr/bin:/bin:/usr/local/bin", "GIT_DIR": str(self.root / "missing-git"),
                       "GIT_WORK_TREE": str(self.root / "not-the-product")}
        result = subprocess.run(["bash", "scripts/git-hooks/pre-push"], cwd=self.root,
                                env=environment, capture_output=True)
        self.assertEqual(result.returncode, 0, result.stderr)

    def test_ignore_rules_may_name_internal_paths_but_not_credentials(self):
        self.write(".gitignore", b"AGENTS.md\n.pi/\nscripts/evolution/\n")
        self.assertEqual(MODULE.scan_commit(self.root, self.commit()), [])

    def test_private_key_header_is_rejected(self):
        # 运行时仍是同一完整标头；源码不落连续密钥标头，兼容提交前密钥形态扫描。
        self.write("docs/example.txt", b"-----" + b"BEGIN RSA" + b" PRIVATE KEY" + b"-----\nnot-a-key\n")
        self.assertIn(("private-key", "docs/example.txt"), MODULE.scan_commit(self.root, self.commit()))

    def test_symlink_cannot_escape_content_scan(self):
        (self.root / "link.txt").symlink_to("README.md")
        self.assertIn(("not-regular", "link.txt"), MODULE.scan_commit(self.root, self.commit()))

    def test_newline_in_filename_is_not_a_second_record(self):
        name = "docs/line\nbreak.txt"
        self.write(name, b"sk-" + b"A" * 32)
        self.assertIn(("secret-shape", name), MODULE.scan_commit(self.root, self.commit()))

    def test_reads_committed_blob_not_clean_worktree_or_index(self):
        self.write("docs/guide.md", b"AGENTS.md")
        bad = self.commit()
        self.write("docs/guide.md", b"clean replacement")
        self.git("add", "docs/guide.md")
        self.assertIn(("internal-reference", "docs/guide.md"), MODULE.scan_commit(self.root, bad))
        self.assertEqual(MODULE.scan_commit(self.root, self.base), [])

    def test_missing_commit_and_empty_tree_fail_closed(self):
        with self.assertRaises((RuntimeError, ValueError)):
            MODULE.scan_commit(self.root, "0" * 40)
        self.git("rm", "README.md")
        with self.assertRaises(ValueError):
            MODULE.scan_commit(self.root, self.commit())

    def test_all_introduced_commits_are_checked_even_if_tip_is_clean(self):
        self.write("docs/guide.md", b"AGENTS.md")
        bad = self.commit()
        self.write("docs/guide.md", b"clean tip")
        tip = self.commit()
        updates = io.StringIO(f"refs/heads/release {tip} refs/heads/release {'0' * 40}\n")
        revisions = MODULE.push_revisions(self.root, updates, baseline=self.base)
        self.assertEqual(set(revisions), {bad, tip})
        self.assertTrue(any(MODULE.scan_commit(self.root, rev) for rev in revisions))
        self.assertEqual(MODULE.scan_commit(self.root, tip), [])

    def test_push_uses_input_sha_not_current_head(self):
        self.write("bad.md", b"AGENTS.md")
        bad = self.commit()
        self.git("checkout", "-q", "--detach", self.base)
        stream = io.StringIO(f"refs/heads/other {bad} refs/heads/other {'0' * 40}\n")
        self.assertEqual(MODULE.push_revisions(self.root, stream, baseline=self.base), [bad])

    def test_push_rejects_unrelated_history_and_malformed_input(self):
        self.git("checkout", "-q", "--orphan", "private")
        # 相同秒内的同 tree/作者/消息会产生相同根提交；必须构造真正不同的历史。
        self.write("private-history.txt", b"unrelated root")
        private = self.commit()
        self.assertNotEqual(private, self.base)
        for value in [f"refs/heads/private {private} refs/heads/private {'0' * 40}\n", "malformed\n"]:
            with self.assertRaises((RuntimeError, ValueError)):
                MODULE.push_revisions(self.root, io.StringIO(value), baseline=self.base)

    def test_merge_cannot_import_clean_but_unrelated_history(self):
        self.git("checkout", "-q", "--orphan", "private")
        self.write("otherwise-public.txt", b"clean but unrelated")
        private = self.commit()
        self.git("checkout", "-q", "--detach", self.base)
        self.git("merge", "--allow-unrelated-histories", "-qm", "fixture merge", private)
        tip = self.git("rev-parse", "HEAD")
        self.assertEqual(MODULE.scan_commit(self.root, tip), [])
        with self.assertRaises(RuntimeError):
            MODULE.push_revisions(self.root, io.StringIO(f"refs/heads/a {tip} refs/heads/a {'0' * 40}\n"), baseline=self.base)

    def test_empty_tree_names_are_checked(self):
        # 空目录也属于将公开的原始 tree，不能仅遍历叶子 blob。
        fake = "ghp_" + "A" * 32
        empty = MODULE.git(self.root, "mktree", data=b"").decode().strip()
        blob = self.git("rev-parse", "HEAD:README.md")
        records = f"100644 blob {blob}\tREADME.md\n040000 tree {empty}\t{fake}\n"
        tree = MODULE.git(self.root, "mktree", data=records.encode()).decode().strip()
        commit = self.git("commit-tree", tree, "-p", self.base, "-m", "empty tree fixture")
        self.assertTrue(("credential-path", fake) in MODULE.scan_commit(self.root, commit))
        self.assertEqual(MODULE.safe_name(fake), '"<redacted-path>"')

    def test_legacy_graft_cannot_hide_introduced_history(self):
        self.write("bad.md", b"AGENTS.md")
        bad = self.commit()
        self.write("bad.md", b"clean tip")
        tip = self.commit()
        # 仅在测试仓构造 legacy 视图；原始 tip 的父对象仍指向 bad。
        self.write(".git/info/grafts", f"{tip} {self.base}\n".encode())
        updates = io.StringIO(f"refs/heads/a {tip} refs/heads/a {'0' * 40}\n")
        revisions = MODULE.push_revisions(self.root, updates, baseline=self.base)
        self.assertIn(bad, revisions)
        self.assertTrue(any(MODULE.scan_commit(self.root, rev) for rev in revisions))

    def test_size_limit_fails_closed_instead_of_skipping_blob(self):
        self.write("large.txt", b"a" * 33)
        tip = self.commit()
        with patch.object(MODULE, "MAX_BLOB", 32):
            with self.assertRaises(ValueError):
                MODULE.scan_commit(self.root, tip)
        with patch.object(MODULE, "MAX_BLOB", 33):
            self.assertEqual(MODULE.scan_commit(self.root, tip), [])

    def test_sensitive_path_is_never_requested_from_cat_file(self):
        self.write(".env", b"non-secret fixture")
        tip = self.commit()
        blob = self.git("rev-parse", "HEAD:.env").encode()
        actual_git, requested = MODULE.git, []

        def observe(repository, *args, **kwargs):
            if args[0] == "cat-file":
                requested.append(kwargs["data"])
            return actual_git(repository, *args, **kwargs)

        with patch.object(MODULE, "git", side_effect=observe):
            self.assertIn(("environment-file", ".env"), MODULE.scan_commit(self.root, tip))
        self.assertNotIn(blob, b"".join(requested))

    def test_cli_hook_and_ci_share_checker_and_do_not_print_secret(self):
        for name in ["scripts/release-hygiene.py", "scripts/git-hooks/pre-push"]:
            self.write(name, (ROOT / name).read_bytes())
        self.commit()
        result = subprocess.run(["bash", "scripts/git-hooks/pre-push"], cwd=self.root, capture_output=True)
        self.assertEqual(result.returncode, 0, result.stderr)
        fake = b"ghp_" + b"A" * 32
        self.write("docs/accident.txt", fake)
        self.commit()
        result = subprocess.run(["bash", "scripts/git-hooks/pre-push"], cwd=self.root, capture_output=True)
        self.assertEqual(result.returncode, 1, result.stderr)
        self.assertNotIn(fake, result.stdout + result.stderr)
        self.assertIn(b"secret-shape", result.stdout)
        ci = (ROOT / ".github/workflows/ci.yml").read_text()
        hygiene = ci.split("  hygiene:", 1)[1].split("  go-mod-cache:", 1)[0]
        self.assertIn("bash scripts/verify-product.sh public-hygiene", hygiene)
        self.assertIn("bash scripts/verify-product.sh public-migrations", hygiene)
        self.assertIn("fetch-depth: 0", hygiene)
        self.assertNotIn("forbidden=", hygiene)


if __name__ == "__main__":
    unittest.main()
