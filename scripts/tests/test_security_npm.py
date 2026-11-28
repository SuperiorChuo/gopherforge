"""验证 Security Scan 的 npm 审计目录、真实锁文件和阻断行为；不访问网络。"""

import json
import os
from pathlib import Path
import re
import shutil
import subprocess
import tempfile
import unittest

ROOT = Path(__file__).resolve().parents[2]


class NpmAuditTests(unittest.TestCase):
    def setUp(self):
        source = (ROOT / ".github/workflows/security-scan.yml").read_text()
        self.job = source.split("  npm-audit:\n", 1)[1]
        matrix = re.search(r"^        directory: (\[[^\n]+\])$", self.job, re.M)
        self.directories = json.loads(matrix.group(1)) if matrix else ["microservices/web"]
        self.workdir = re.search(r"^        working-directory: ([^\n]+)$", self.job, re.M).group(1)
        self.cache = re.search(r"^          cache-dependency-path: ([^\n]+)$", self.job, re.M).group(1)

    def test_all_frontend_lockfiles_have_separate_audit_jobs_and_caches(self):
        expected = ["microservices/web"]
        if (ROOT / "website/package-lock.json").is_file():
            expected.append("website")
        self.assertEqual(self.directories, expected)
        if len(expected) > 1:
            self.assertIn("fail-fast: false", self.job)
            self.assertEqual(self.workdir, "${{ matrix.directory }}")
            self.assertEqual(self.cache, "${{ matrix.directory }}/package-lock.json")
        for directory in self.directories:
            with self.subTest(directory=directory):
                workdir = self.workdir.replace("${{ matrix.directory }}", directory)
                cache = self.cache.replace("${{ matrix.directory }}", directory)
                self.assertEqual(workdir, directory)
                self.assertEqual(cache, directory + "/package-lock.json")
                self.assertTrue((ROOT / directory / "package.json").is_file())
                self.assertTrue((ROOT / cache).is_file())

    def test_each_audit_includes_dev_dependencies_and_propagates_failure(self):
        block = self.job.split("- name: 高危及以上阻断\n", 1)[1].split("\n      - name:", 1)[0]
        command = re.search(r"^        run: ([^\n]+)$", block, re.M).group(1)
        self.assertNotIn("continue-on-error:", self.job)
        self.assertNotIn("--omit", command)
        self.assertNotIn("--production", command)
        with tempfile.TemporaryDirectory(prefix="npm-audit-contract-") as tmp:
            binary = Path(tmp) / "npm"
            binary.write_text(f"#!{shutil.which('python3')}\n" +
                              'import json, os, sys\n'
                              'print(json.dumps({"cwd": os.getcwd(), "args": sys.argv[1:]}))\n'
                              'sys.exit(int(os.environ["AUDIT_EXIT"]))\n')
            binary.chmod(0o755)
            for directory in self.directories:
                for code in [0, 1]:
                    with self.subTest(directory=directory, exit_code=code):
                        workdir = ROOT / self.workdir.replace("${{ matrix.directory }}", directory)
                        result = subprocess.run(["bash", "-e", "-o", "pipefail", "-c", command], cwd=workdir,
                                                env={**os.environ, "PATH": tmp + os.pathsep + os.environ["PATH"],
                                                     "AUDIT_EXIT": str(code)},
                                                capture_output=True, text=True, timeout=10)
                        self.assertEqual(result.returncode, code, result.stderr)
                        call = json.loads(result.stdout)
                        self.assertEqual(Path(call["cwd"]).resolve(), (ROOT / directory).resolve())
                        self.assertEqual(call["args"], ["audit", "--audit-level=high"])


if __name__ == "__main__":
    unittest.main()
