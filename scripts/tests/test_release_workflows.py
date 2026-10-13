"""验证真实工作流调用链；只替代 Docker config，Go workspace 解析仍运行真工具。"""

from contextlib import redirect_stdout
import io
import json
import os
from pathlib import Path
import re
import subprocess
import tempfile
import textwrap
import unittest
from unittest.mock import patch

ROOT = Path(__file__).resolve().parents[2]


class WorkflowTests(unittest.TestCase):
    def test_release_matrix_runs_real_workspace_parser(self):
        source = (ROOT / ".github/workflows/release.yml").read_text()
        block = source.split("python3 - <<'PY' >> \"$GITHUB_OUTPUT\"", 1)[1].split("\n          PY", 1)[0]
        actual_run = subprocess.run

        def run(command, **kwargs):
            if command[:2] == ["docker", "compose"]:
                self.assertEqual(command[2:], ["--env-file", ".env.example", "config", "--format", "json"])
                # 与 Compose JSON 同形；排除 migrate，两个真实应用镜像不得丢失。
                services = {
                    name: {"image": image, "build": {"context": str(ROOT / "microservices")}}
                    for name, image in [
                        ("monitor-service", "go-admin-kit-monitor-service:latest"),
                        ("ai-service", "go-admin-kit-ai-service:latest"),
                        ("migrate", "go-admin-kit-monitor-service:latest"),
                    ]
                }
                return subprocess.CompletedProcess(command, 0, json.dumps({"services": services}))
            return actual_run(command, **kwargs)

        output = io.StringIO()
        previous = Path.cwd()
        try:
            os.chdir(ROOT)
            with patch("subprocess.run", side_effect=run), redirect_stdout(output):
                exec(compile(textwrap.dedent(block), "release-matrix", "exec"), {})
        finally:
            os.chdir(previous)
        entries = dict(line.split("=", 1) for line in output.getvalue().splitlines())
        self.assertEqual(json.loads(entries["modules"]), ["services", "tools/dbdoc"])
        images = json.loads(entries["services"])["include"]
        self.assertEqual({image["service"] for image in images}, {"monitor-service", "ai-service"})

    def test_security_matrix_runs_real_workspace_parser(self):
        source = (ROOT / ".github/workflows/security-scan.yml").read_text()
        block = source.split("id: modules\n        run: |\n", 1)[1].split("\n  govulncheck:", 1)[0]
        with tempfile.TemporaryDirectory(prefix="security-matrix-") as tmp:
            output = Path(tmp) / "output"
            subprocess.run(["bash", "-c", textwrap.dedent(block)], cwd=ROOT,
                           env={**os.environ, "GITHUB_OUTPUT": str(output)}, check=True)
            name, value = output.read_text().strip().split("=", 1)
        self.assertEqual(name, "modules")
        self.assertEqual(json.loads(value), ["services", "tools/dbdoc"])

    def test_fixed_cache_paths_exist(self):
        for name in ["ci", "release", "security-scan"]:
            source = (ROOT / f".github/workflows/{name}.yml").read_text()
            for path in re.findall(r"cache-dependency-path: ([^\n]+)", source):
                if "${{" in path:
                    path = path.replace("${{ matrix.module }}", "services")
                self.assertTrue((ROOT / path).is_file(), f"{name}: 不存在的依赖缓存路径 {path}")

    def test_dependabot_tracks_real_product_modules(self):
        source = (ROOT / ".github/dependabot.yml").read_text()
        block = source.split("package-ecosystem: gomod", 1)[1].split("schedule:", 1)[0]
        paths = set(re.findall(r"- /([^\n]+)", block))
        expected = {"microservices/services", "microservices/tools/dbdoc", "freeswitch-cc/control-api", "clients/hermes-manager"}
        self.assertEqual(paths, expected)
        for path in paths:
            self.assertTrue((ROOT / path / "go.mod").is_file(), path)

    def test_product_entrypoint_standalone_and_rejects_unknown_targets(self):
        # 没有任何本地治理目录的干净目录也能展示用法和拒绝错拼目标。
        source = (ROOT / "scripts/verify-product.sh").read_bytes()
        with tempfile.TemporaryDirectory(prefix="product-verify-") as tmp:
            script = Path(tmp) / "scripts/verify-product.sh"
            script.parent.mkdir()
            script.write_bytes(source)
            (script.parent / "release-workspace.py").write_bytes((ROOT / "scripts/release-workspace.py").read_bytes())
            result = subprocess.run(["bash", str(script), "--help"], capture_output=True, text=True)
            self.assertEqual(result.returncode, 0, result.stderr)
            result = subprocess.run(["bash", str(script), "typo"], capture_output=True, text=True)
            self.assertEqual(result.returncode, 2)
            self.assertNotIn("全绿", result.stdout)
            # 缺 workspace 必须在任何 Go build 之前退出，不得空循环后宣称成功。
            result = subprocess.run(["bash", str(script), "all-go"], capture_output=True, text=True)
            self.assertNotEqual(result.returncode, 0)
            self.assertNotIn("全绿", result.stdout)


if __name__ == "__main__":
    unittest.main()
