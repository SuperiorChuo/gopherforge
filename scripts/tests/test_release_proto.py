"""Proto CI 使用真实 Git 基线；安装 Buf 后同时验证兼容/破坏性变更。"""

import os
from pathlib import Path
import re
import shutil
import subprocess
import tempfile
import unittest

ROOT = Path(__file__).resolve().parents[2]
PROTO = "microservices/services/api/proto"


class ProtoWorkflowTests(unittest.TestCase):
    def test_workflow_binds_event_base_and_runs_real_regressions(self):
        source = (ROOT / ".github/workflows/ci.yml").read_text()
        block = re.search(r"(?ms)^  proto-breaking:.*?(?=^  [\w-]+:|\Z)", source).group()
        self.assertIn("PROTO_BASE_SHA: ${{ github.event.pull_request.base.sha || github.event.before }}", block)
        self.assertIn('bash scripts/check-proto-breaking.sh "$PROTO_BASE_SHA"', block)
        self.assertIn("python3 -B -m unittest discover -s scripts/tests -p 'test_release_proto.py' -v", block)
        self.assertNotIn("working-directory:", block)
        self.assertNotIn("#branch=main", block)


@unittest.skipUnless(shutil.which("buf"), "requires buf; proto CI installs it and runs this suite")
class ProtoGitTests(unittest.TestCase):
    def setUp(self):
        self.temp = tempfile.TemporaryDirectory(prefix="proto-regression-")
        self.addCleanup(self.temp.cleanup)
        self.repo = Path(self.temp.name) / "repo with spaces"
        self.repo.mkdir()
        self.git("init", "--initial-branch=main")
        self.git("config", "user.name", "Test")
        self.git("config", "user.email", "test@example.com")
        self.git("config", "core.hooksPath", "/dev/null")
        (self.repo / "scripts").mkdir()
        (self.repo / "scripts/check-proto-breaking.sh").write_bytes((ROOT / "scripts/check-proto-breaking.sh").read_bytes())
        path = self.repo / PROTO
        (path / "example/v1").mkdir(parents=True)
        (path / "buf.yaml").write_text("version: v2\nbreaking:\n  use:\n    - FILE\n")
        self.schema = path / "example/v1/example.proto"
        self.schema.write_text('syntax = "proto3";\npackage example.v1;\nmessage Item { string name = 1; }\n')
        self.commit("baseline")
        self.base = self.git("rev-parse", "HEAD").strip()

    def git(self, *args):
        return subprocess.check_output(["git", "-C", str(self.repo), *args], text=True, stderr=subprocess.DEVNULL)

    def commit(self, message):
        self.git("add", "--all")
        self.git("commit", "-m", message)

    def check(self, base=None, root=None):
        root = root or self.repo
        result = subprocess.run(["bash", str(root / "scripts/check-proto-breaking.sh"), base or self.base],
                                cwd=root / PROTO, capture_output=True, text=True, timeout=30,
                                env={**os.environ, "BUF_TOKEN": "", "TMPDIR": self.temp.name})
        self.assertEqual(list(Path(self.temp.name).glob("proto-baseline.*")), [], "baseline export leaked")
        return result

    def test_compatible_change_uses_root_from_nested_cwd(self):
        self.schema.write_text('syntax = "proto3";\npackage example.v1;\nmessage Item { string name = 1; string note = 2; }\n')
        self.commit("compatible")
        result = self.check()
        self.assertEqual(result.returncode, 0, result.stdout + result.stderr)

    def test_deleted_field_fails_against_previous_commit_not_current_main(self):
        self.schema.write_text('syntax = "proto3";\npackage example.v1;\nmessage Item {}\n')
        self.commit("breaking")
        # 自比会错误放过该变更；事件基线必须仍是前一提交。
        control = self.check(self.git("rev-parse", "HEAD").strip())
        self.assertEqual(control.returncode, 0, control.stdout + control.stderr)
        result = self.check()
        self.assertNotEqual(result.returncode, 0)
        self.assertIn('Previously present field "1" with name "name"', result.stdout + result.stderr)

    def test_linked_worktree_git_file_is_supported(self):
        worktree = Path(self.temp.name) / "linked worktree"
        self.git("worktree", "add", "--detach", str(worktree), "HEAD")
        self.assertTrue((worktree / ".git").is_file())
        result = self.check(root=worktree)
        self.assertEqual(result.returncode, 0, result.stdout + result.stderr)

    def test_missing_zero_and_non_commit_bases_fail_closed(self):
        blob = self.git("rev-parse", "HEAD:" + PROTO + "/buf.yaml").strip()
        for base in ["0" * 40, "f" * 40, "main", "--help", blob]:
            with self.subTest(base=base):
                result = self.check(base)
                self.assertNotEqual(result.returncode, 0)
                self.assertIn("invalid proto base commit", result.stderr)


if __name__ == "__main__":
    unittest.main()
