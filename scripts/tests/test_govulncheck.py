"""验证工具链选择、隔离安装、参数和失败码传播；不访问漏洞服务。"""

import json
import os
from pathlib import Path
import shutil
import subprocess
import tempfile
import unittest

ROOT = Path(__file__).resolve().parents[2]


class GovulncheckTests(unittest.TestCase):
    def setUp(self):
        self.tmp = tempfile.TemporaryDirectory(prefix="vuln check ")
        self.addCleanup(self.tmp.cleanup)
        self.root = Path(self.tmp.name)
        self.module = self.root / "module"
        self.module.mkdir()
        (self.module / "go.mod").write_text("module example.com/fixture\ngo 1.27.0\n")
        self.bin = self.root / "bin"
        self.bin.mkdir()
        self.log = self.root / "calls.jsonl"
        go = self.bin / "go"
        go.write_text(f"#!{shutil.which('python3')}\n" + '''import json, os, pathlib, sys
args = sys.argv[1:]
with open(os.environ["CALL_LOG"], "a") as f:
    f.write(json.dumps({"args": args, "cwd": os.getcwd(),
                       "toolchain": os.environ.get("GOTOOLCHAIN"),
                       "gowork": os.environ.get("GOWORK"),
                       "gobin": os.environ.get("GOBIN")}) + "\\n")
if args == ["env", "GOVERSION"]:
    print("go1.27.0")
    sys.exit(int(os.environ.get("ENV_EXIT", "0")))
if args != ["install", "golang.org/x/vuln/cmd/govulncheck@v1.6.0"]:
    sys.exit(90)
if os.environ.get("GOTOOLCHAIN") != "go1.27.0":
    sys.exit(91)
code = int(os.environ.get("INSTALL_EXIT", "0"))
if code:
    sys.exit(code)
scanner = pathlib.Path(os.environ["GOBIN"]) / "govulncheck"
scanner.write_text("#!/bin/sh\\nprintf '%s\\\\n' \\\"$GOTOOLCHAIN\\\" \\\"$@\\\"\\nexit \\\"${SCAN_EXIT:-0}\\\"\\n")
scanner.chmod(0o755)
''')
        go.chmod(0o755)
        old = self.bin / "govulncheck"
        old.write_text("#!/bin/sh\nexit 92\n")
        old.chmod(0o755)
        self.env = {**os.environ, "PATH": str(self.bin) + os.pathsep + os.environ["PATH"],
                    "CALL_LOG": str(self.log), "GOBIN": str(self.bin),
                    "GOTOOLCHAIN": "auto", "GOWORK": str(self.root / "unrelated.work"),
                    "TMPDIR": str(self.root)}

    def run_scan(self, args=None, **env):
        command = ["bash", str(ROOT / "scripts/govulncheck.sh")]
        command += [str(self.module)] if args is None else args
        return subprocess.run(command, env={**self.env, **env},
                              text=True, capture_output=True, timeout=10)

    def calls(self):
        return [json.loads(line) for line in self.log.read_text().splitlines()]

    def assert_cleaned(self):
        self.assertEqual(list(self.root.glob("govulncheck.*")), [])
        self.assertTrue((self.bin / "govulncheck").exists())

    def test_module_toolchain_and_isolated_scanner(self):
        result = self.run_scan()
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertEqual(result.stdout.splitlines(), ["go1.27.0", "./..."])
        env_call, install = self.calls()
        self.assertEqual(Path(env_call["cwd"]).resolve(), self.module.resolve())
        self.assertEqual(env_call["gowork"], "off")
        self.assertEqual(install["gowork"], "off")
        self.assertEqual(install["toolchain"], "go1.27.0")
        self.assertNotEqual(install["gobin"], str(self.bin))
        self.assert_cleaned()

    def test_scanner_flags_are_forwarded(self):
        result = self.run_scan([str(self.module), "-format", "json", "./some package/..."])
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertEqual(result.stdout.splitlines(), ["go1.27.0", "-format", "json", "./some package/..."])
        self.assert_cleaned()

    def test_scanner_exit_codes_are_preserved(self):
        for code in (1, 2, 3):
            with self.subTest(code=code):
                result = self.run_scan(SCAN_EXIT=str(code))
                self.assertEqual(result.returncode, code, result.stderr)
                self.assert_cleaned()

    def test_toolchain_failure_stops_before_install(self):
        result = self.run_scan(ENV_EXIT="7")
        self.assertEqual(result.returncode, 7, result.stderr)
        self.assertEqual(len(self.calls()), 1)
        self.assert_cleaned()

    def test_install_failure_stops_before_scan(self):
        result = self.run_scan(INSTALL_EXIT="9")
        self.assertEqual(result.returncode, 9, result.stderr)
        self.assertEqual(result.stdout, "")
        self.assert_cleaned()

    def test_missing_module_and_missing_argument_fail(self):
        for args in ([], [str(self.root)]):
            with self.subTest(args=args):
                self.assertEqual(self.run_scan(args).returncode, 2)
                self.assertFalse(self.log.exists())


if __name__ == "__main__":
    unittest.main()
