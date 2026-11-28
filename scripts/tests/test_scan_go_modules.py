"""安全扫描模块发现的真实 Git 正负控；不下载工具链或访问漏洞服务。"""

import json
import os
from pathlib import Path
import shutil
import subprocess
import sys
import tempfile
import unittest

ROOT = Path(__file__).resolve().parents[2]
SCRIPT = ROOT / "scripts/scan-go-modules.py"


class ScanModulesTests(unittest.TestCase):
    def setUp(self):
        self.tmp = tempfile.TemporaryDirectory(prefix="scan modules ")
        self.addCleanup(self.tmp.cleanup)
        self.root = Path(self.tmp.name) / "repo"
        self.root.mkdir()
        self.git("init", "-q")

    def git(self, *args):
        return subprocess.run(["git", "-C", str(self.root), *args],
                              check=True, capture_output=True, text=True)

    def module(self, name, tracked=True):
        manifest = self.root / name / "go.mod"
        manifest.parent.mkdir(parents=True, exist_ok=True)
        manifest.write_text("module example.com/fixture\ngo 1.27.0\n")
        if tracked:
            self.git("add", "--", str(manifest))
        return manifest

    def discover(self, *args, root=None, env=None):
        return subprocess.run([sys.executable, "-B", str(SCRIPT), "--root", str(root or self.root), *args],
                              cwd=self.tmp.name, env=env, capture_output=True, text=True, timeout=10)

    def assert_failure(self, result, message):
        self.assertNotEqual(result.returncode, 0, result.stdout)
        self.assertEqual(result.stdout, "", "失败不能输出部分清单或成功摘要")
        self.assertIn(message, result.stderr)

    def test_all_tracked_modules_ignore_workspace_and_untracked_modules(self):
        names = ["microservices/services", "microservices/tools/dbdoc",
                 "control/api", "clients/manager"]
        for name in names:
            self.module(name)
        # 即使 go.work 只列出 services（甚至无法解析），安全扫描也必须覆盖全部模块。
        (self.root / "microservices/go.work").write_text("use ./services\ninvalid workspace syntax\n")
        self.module("untracked/scratch", tracked=False)
        result = self.discover()
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertEqual(json.loads(result.stdout), sorted(names))
        result = self.discover("--lines")
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertEqual(result.stdout.splitlines(), sorted(names))

    def test_root_and_space_paths_are_supported(self):
        self.module(".")
        self.module("tools/with space")
        result = self.discover()
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertEqual(json.loads(result.stdout), [".", "tools/with space"])

    def test_empty_or_untracked_only_repository_fails(self):
        self.assert_failure(self.discover(), "未找到任何已跟踪")
        self.module("untracked", tracked=False)
        self.assert_failure(self.discover(), "未找到任何已跟踪")

    def test_non_repository_and_subdirectory_fail(self):
        self.assert_failure(self.discover(root=self.tmp.name), "Git 枚举失败")
        self.module("nested")
        self.assert_failure(self.discover(root=self.root / "nested"), "必须指定仓库根目录")

    def test_missing_tracked_file_fails_without_partial_output(self):
        self.module("a-valid")
        self.module("z-missing").unlink()
        self.assert_failure(self.discover(), "z-missing/go.mod")

    def test_non_file_manifest_fails(self):
        manifest = self.module("module")
        manifest.unlink()
        manifest.mkdir()
        self.assert_failure(self.discover(), "普通 go.mod")

    def test_symlink_manifest_is_rejected_even_inside_repository(self):
        target = self.module("target")
        manifest = self.module("link")
        manifest.unlink()
        manifest.symlink_to(target)
        self.assert_failure(self.discover(), "符号链接")

    def test_external_file_and_directory_symlinks_fail(self):
        external = Path(self.tmp.name) / "outside"
        external.mkdir()
        (external / "go.mod").write_text("module example.com/outside\n")
        manifest = self.module("module")
        manifest.unlink()
        manifest.symlink_to(external / "go.mod")
        self.assert_failure(self.discover(), "越出仓库目录")
        manifest.unlink()
        manifest.parent.rmdir()
        manifest.parent.symlink_to(external, target_is_directory=True)
        self.assert_failure(self.discover(), "越出仓库目录")

    def test_control_character_in_path_fails(self):
        self.module("line\nbreak")
        self.assert_failure(self.discover(), "无效的已跟踪模块路径")

    def test_git_enumeration_error_cannot_become_empty_success(self):
        self.module("valid")
        binary = Path(self.tmp.name) / "bin"
        binary.mkdir()
        fake = binary / "git"
        # 根目录检查可通过，但真正的模块枚举失败。
        fake.write_text('#!/bin/sh\nif [ "$3" = rev-parse ]; then printf "%s\\n" "$2"; '
                        'else printf "index unreadable\\n" >&2; exit 7; fi\n')
        fake.chmod(0o755)
        self.assert_failure(self.discover(env={**os.environ, "PATH": str(binary)}), "index unreadable")

    def product_entrypoint(self):
        scripts = self.root / "scripts"
        scripts.mkdir()
        for name in ["scan-go-modules.py", "verify-product.sh"]:
            shutil.copyfile(ROOT / "scripts" / name, scripts / name)
        # 用扫描器边界替身记录调用；保留真实 Git 发现和真实产品入口。
        (scripts / "govulncheck.sh").write_text(
            '#!/bin/bash\nprintf "%s\\n" "$1" >> "$SCAN_LOG"\nexit "${SCAN_EXIT:-0}"\n')
        return scripts / "verify-product.sh"

    def run_product(self, script, **env):
        return subprocess.run(["bash", str(script), "security-go"], cwd=self.tmp.name,
                              env={**os.environ, "SCAN_LOG": str(self.root / "scan.log"), **env},
                              capture_output=True, text=True, timeout=10)

    def test_local_security_entrypoint_scans_all_modules_and_propagates_failures(self):
        for name in ["server", "tools/with space"]:
            self.module(name)
        script = self.product_entrypoint()
        result = self.run_product(script)
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertEqual((self.root / "scan.log").read_text().splitlines(),
                         [str(self.root / "server"), str(self.root / "tools/with space")])
        for code in [3, 7]:
            with self.subTest(code=code):
                result = self.run_product(script, SCAN_EXIT=str(code))
                self.assertEqual(result.returncode, code, result.stderr)
                self.assertNotIn("全绿", result.stdout)

    def test_local_discovery_failure_stops_before_scanner(self):
        script = self.product_entrypoint()
        result = self.run_product(script)
        self.assertNotEqual(result.returncode, 0)
        self.assertNotIn("全绿", result.stdout)
        self.assertFalse((self.root / "scan.log").exists())


if __name__ == "__main__":
    unittest.main()
