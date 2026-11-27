"""目录门禁的真实进程回归：违规与工具失败都不得报通过。"""

import os
from pathlib import Path
import shutil
import subprocess
import tempfile
import unittest

ROOT = Path(__file__).resolve().parents[2]
MICRO = (ROOT / "microservices").is_dir()
BASH = shutil.which("bash")
TOOLS = ("dirname", "find", "sort") + (("rg",) if MICRO else ())


class LayoutFixture(unittest.TestCase):
    def setUp(self):
        self.tmp = tempfile.TemporaryDirectory(prefix="layout check ")
        self.addCleanup(self.tmp.cleanup)
        self.root = Path(self.tmp.name)
        self.bin = self.root / "bin"
        self.bin.mkdir()
        for tool in TOOLS:
            executable = shutil.which(tool)
            self.assertIsNotNone(executable, f"回归测试需要 {tool}")
            (self.bin / tool).symlink_to(executable)
        self.script = self.root / "scripts/check-code-layout.sh"
        self.script.parent.mkdir()
        shutil.copyfile(ROOT / "scripts/check-code-layout.sh", self.script)
        self.web = self.root / ("microservices/web/src" if MICRO else "web/src")
        self.api = self.web / "api"
        self.api.mkdir(parents=True)
        (self.api / "index.ts").write_text("export {};\n")
        if MICRO:
            self.services = self.root / "microservices/services"
            self.go_root = self.services / "cc/internal/api"
            for directory in [self.go_root / "cc", self.services / "cc/internal/store/cc",
                              self.services / "ai/internal/service/hermes"]:
                directory.mkdir(parents=True)
            (self.go_root / "cc/valid.go").write_text("package cc\n")
        else:
            self.go_root = self.root / "server/internal/pkg"
            (self.go_root / "cache").mkdir(parents=True)
            (self.web / "components/common").mkdir(parents=True)
            (self.go_root / "cache/valid.go").write_text("package cache\n")

    def run_guard(self):
        return subprocess.run([BASH, str(self.script)], cwd=self.root,
                              env={**os.environ, "PATH": str(self.bin)},
                              text=True, capture_output=True, timeout=10)

    def stub(self, name, body):
        path = self.bin / name
        if path.exists():
            path.unlink()
        path.write_text("#!/bin/sh\n" + body + "\n")
        path.chmod(0o755)

    def assert_blocked(self, result, code):
        self.assertEqual(result.returncode, code, result.stdout + result.stderr)
        self.assertNotIn("代码目录守卫通过", result.stdout)


class LayoutTests(LayoutFixture):
    def test_valid_tree_and_index_entrypoint_pass(self):
        result = self.run_guard()
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertIn("代码目录守卫通过", result.stdout)

    def test_flat_go_file_is_rejected(self):
        (self.go_root / "invalid.go").write_text("package invalid\n")
        result = self.run_guard()
        self.assert_blocked(result, 1)
        self.assertIn("invalid.go", result.stderr)

    def test_flat_web_api_is_rejected(self):
        (self.api / "invalid.tsx").write_text("export {};\n")
        result = self.run_guard()
        self.assert_blocked(result, 1)
        self.assertIn("invalid.tsx", result.stderr)

    def test_missing_required_tools_are_errors(self):
        for tool in TOOLS:
            with self.subTest(tool=tool):
                path = self.bin / tool
                target = path.readlink()
                path.unlink()
                try:
                    result = self.run_guard()
                    self.assert_blocked(result, 2)
                    self.assertIn(tool, result.stderr)
                finally:
                    path.symlink_to(target)

    def test_enumeration_errors_are_not_empty_results(self):
        for tool in ("dirname", "find", "sort"):
            with self.subTest(tool=tool):
                path = self.bin / tool
                target = path.readlink()
                self.stub(tool, "printf 'injected enumeration failure\\n' >&2\nexit 17")
                try:
                    result = self.run_guard()
                    self.assert_blocked(result, 2)
                    self.assertIn("injected enumeration failure", result.stderr)
                finally:
                    path.unlink()
                    path.symlink_to(target)

    def test_frontend_enumeration_errors_are_not_ignored(self):
        if MICRO:
            shutil.rmtree(self.services)
        self.stub("find", "printf 'injected frontend failure\\n' >&2\nexit 17")
        result = self.run_guard()
        self.assert_blocked(result, 2)
        self.assertIn("injected frontend failure", result.stderr)


if MICRO:
    class ImportTests(LayoutFixture):
        def test_old_domain_import_is_rejected(self):
            (self.go_root / "cc/valid.go").write_text(
                'package cc\nimport "github.com/go-admin-kit/services/cc/internal/api"\n')
            self.assert_blocked(self.run_guard(), 1)

        def test_old_ai_import_is_rejected(self):
            (self.services / "ai/internal/service/hermes/valid.go").write_text(
                'package hermes\nimport "github.com/go-admin-kit/services/ai/internal/service"\n')
            self.assert_blocked(self.run_guard(), 1)

        def test_search_failures_are_not_no_matches(self):
            for code in (2, 127):
                with self.subTest(code=code):
                    self.stub("rg", f"printf 'injected search failure\\n' >&2\nexit {code}")
                    result = self.run_guard()
                    self.assert_blocked(result, 2)
                    self.assertIn("injected search failure", result.stderr)

        def test_missing_domain_directory_is_rejected(self):
            shutil.rmtree(self.go_root / "cc")
            self.assert_blocked(self.run_guard(), 1)
else:
    class ComponentTests(LayoutFixture):
        def test_flat_component_is_rejected(self):
            (self.web / "components/invalid.tsx").write_text("export {};\n")
            self.assert_blocked(self.run_guard(), 1)


if __name__ == "__main__":
    unittest.main()
