"""发布矩阵必须和 Go 的真实 workspace 语义一致，且不能逃逸产品目录。"""

import importlib.util
import json
import os
from pathlib import Path
import subprocess
import sys
import tempfile
import unittest

ROOT = Path(__file__).resolve().parents[2]
SCRIPT = ROOT / "scripts/release-workspace.py"
spec = importlib.util.spec_from_file_location("release_workspace", SCRIPT)
workspace = importlib.util.module_from_spec(spec)
spec.loader.exec_module(workspace)


class WorkspaceTests(unittest.TestCase):
    def setUp(self):
        self.tmp = tempfile.TemporaryDirectory(prefix="release-workspace-")
        self.addCleanup(self.tmp.cleanup)
        self.root = Path(self.tmp.name)
        self.work = self.root / "microservices"
        self.work.mkdir()

    def module(self, name, module_path=None):
        path = self.work / name
        path.mkdir(parents=True)
        module_path = module_path or f"example.com/{path.name}"
        (path / "go.mod").write_text(f"module {module_path}\n\ngo 1.20\n")
        return path

    def definition(self, body):
        (self.work / "go.work").write_text("go 1.20\n" + body + "\n")

    def test_single_line_use(self):
        # 统一模块后的真实布局，旧正则会得到 [] 并让整个 Release 提前失败。
        self.module("services")
        self.definition("use ./services")
        self.assertEqual(workspace.workspace_modules(self.work), ["services"])

    def test_block_use_and_comments(self):
        self.module("services")
        self.module("tools/dbdoc")
        self.definition('use (\n ./tools/dbdoc // 工具\n "./services"\n)')
        self.assertEqual(workspace.workspace_modules(self.work), ["services", "tools/dbdoc"])

    def test_missing_work_fails(self):
        with self.assertRaises((ValueError, OSError)):
            workspace.workspace_modules(self.work)

    def test_empty_use_fails(self):
        self.definition("use ()")
        with self.assertRaises(ValueError):
            workspace.workspace_modules(self.work)

    def test_missing_module_fails(self):
        self.definition("use ./missing")
        with self.assertRaises(ValueError):
            workspace.workspace_modules(self.work)

    def test_outside_path_fails(self):
        self.module("../outside")
        self.definition("use ../outside")
        with self.assertRaises(ValueError):
            workspace.workspace_modules(self.work)

    def test_symlink_escape_fails(self):
        outside = self.module("../outside")
        (self.work / "services").symlink_to(outside, target_is_directory=True)
        self.definition("use ./services")
        with self.assertRaises(ValueError):
            workspace.workspace_modules(self.work)

    def test_duplicate_module_fails(self):
        self.module("services")
        self.definition("use (\n ./services\n ./services\n)")
        with self.assertRaises(ValueError):
            workspace.workspace_modules(self.work)

    def test_different_directories_with_same_module_identity_fail(self):
        self.module("services", "example.com/duplicate")
        self.module("tools/dbdoc", "example.com/duplicate")
        self.definition("use (\n ./services\n ./tools/dbdoc\n)")
        # 不只验证目录唯一性：Go 会拒绝不同目录声明同一个 module path。
        actual = subprocess.run(
            ["go", "list", "-m"], cwd=self.work,
            env={**os.environ, "GOWORK": str(self.work / "go.work")},
            capture_output=True, text=True, timeout=60,
        )
        self.assertNotEqual(actual.returncode, 0)
        self.assertIn("appears multiple times", actual.stderr)
        with self.assertRaisesRegex(ValueError, "重复"):
            workspace.workspace_modules(self.work)

    def test_invalid_module_file_fails(self):
        path = self.module("services")
        (path / "go.mod").write_text("module example.com/test\nnot-a-directive\n")
        self.definition("use ./services")
        with self.assertRaises(ValueError):
            workspace.workspace_modules(self.work)

    def test_missing_module_identity_fails(self):
        path = self.module("services")
        (path / "go.mod").write_text("go 1.20\n")
        self.definition("use ./services")
        with self.assertRaises(ValueError):
            workspace.workspace_modules(self.work)

    def test_invalid_syntax_fails(self):
        self.module("services")
        self.definition("use (\n ./services\n)\nnot-a-directive")
        with self.assertRaises(ValueError):
            workspace.workspace_modules(self.work)

    def test_cli_json_and_lines(self):
        self.module("services")
        self.definition("use ./services")
        command = [sys.executable, "-B", str(SCRIPT), "--workspace", str(self.work)]
        result = subprocess.run(command, capture_output=True, text=True, check=True)
        self.assertEqual(json.loads(result.stdout), ["services"])
        result = subprocess.run(command + ["--lines"], capture_output=True, text=True, check=True)
        self.assertEqual(result.stdout, "services\n")

    def test_real_product_workspace(self):
        modules = workspace.workspace_modules(ROOT / "microservices")
        self.assertEqual(modules, ["services", "tools/dbdoc"])
        # The documented `cd tools/dbdoc && go run .` must work with normal
        # workspace selection, not only with verify-product's GOWORK=off.
        result = subprocess.run(
            ["go", "list", "."], cwd=ROOT / "microservices/tools/dbdoc",
            env={key: value for key, value in os.environ.items() if key != "GOWORK"},
            capture_output=True, text=True, timeout=60, check=True,
        )
        self.assertEqual(result.stdout.strip(), "github.com/go-admin-kit/tools/dbdoc")


if __name__ == "__main__":
    unittest.main()
