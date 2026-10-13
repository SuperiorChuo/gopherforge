"""执行 Release 的真实 shell 与临时 Git 历史，拒绝版本名和实际构建提交分离。"""

import json
import os
from pathlib import Path
import re
import subprocess
import tempfile
import textwrap
import unittest

ROOT = Path(__file__).resolve().parents[2]
WORKFLOW = ROOT / ".github/workflows/release.yml"


def run_block(name, source=None):
    """沿用工作流测试先例，直接执行实际 run 块，不复制生产判断到测试。"""
    source = WORKFLOW.read_text() if source is None else source
    step = source.split(f"      - name: {name}\n", 1)[1].split("\n      - ", 1)[0]
    return textwrap.dedent(step.split("        run: |\n", 1)[1])


class ReleaseIdentityTests(unittest.TestCase):
    def setUp(self):
        self.temp = tempfile.TemporaryDirectory(prefix="gak-release-identity-")
        self.addCleanup(self.temp.cleanup)
        self.root = Path(self.temp.name)
        self.repo = self.root / "repo"
        self.repo.mkdir()
        # 不继承宿主 Git 覆盖配置；所有对象、引用和外部命令替身都只落在夹具中。
        self.env = {
            "PATH": os.defpath, "HOME": str(self.root), "LC_ALL": "C",
            "GIT_CONFIG_NOSYSTEM": "1", "GIT_CONFIG_GLOBAL": os.devnull,
            "GIT_GRAFT_FILE": os.devnull, "GIT_NO_REPLACE_OBJECTS": "1",
            "GIT_TERMINAL_PROMPT": "0",
        }
        self.git("init", "-q")
        self.git("config", "user.name", "Release Test")
        self.git("config", "user.email", "release-test@example.invalid")
        (self.repo / "README.md").write_text("tagged product\n")
        (self.repo / "CHANGELOG.md").write_text("## [Unreleased]\n")
        self.git("add", "README.md", "CHANGELOG.md")
        self.git("commit", "-qm", "tagged product")
        self.source = self.git("rev-parse", "HEAD")
        self.tag = "v9.8.7"
        self.git("tag", self.tag)
        (self.repo / "README.md").write_text("different dispatch context\n")
        self.git("commit", "-qam", "different dispatch context")
        self.event = self.git("rev-parse", "HEAD")
        self.assertNotEqual(self.source, self.event)

    def git(self, *args):
        return subprocess.run(["git", *args], cwd=self.repo, env=self.env,
                              capture_output=True, text=True, check=True).stdout.strip()

    def shell(self, block, env):
        return subprocess.run(["bash", "-c", block], cwd=self.repo,
                              env={**self.env, **env}, capture_output=True, text=True)

    def meta(self, **changes):
        output = self.root / "output"
        summary = self.root / "summary"
        output.write_text("")
        summary.write_text("")
        env = {
            "INPUT_TAG": self.tag, "INPUT_DRY_RUN": "true",
            "INPUT_EXPECTED_SHA": self.source, "EVENT_NAME": "workflow_dispatch",
            "EVENT_SHA": self.event, "EVENT_REF": "refs/heads/main", "REF_NAME": "main",
            "REPOSITORY": "example/product", "REGISTRY": "ghcr.io",
            "GITHUB_SHA": self.event, "WORKFLOW_SHA": "f" * 40,
            "RUN_ID": "123", "RUN_ATTEMPT": "2",
            "GITHUB_OUTPUT": str(output), "GITHUB_STEP_SUMMARY": str(summary),
        }
        env.update(changes)
        # EVENT_SHA 来自同一个 github.sha，夹具不能伪造两个不一致的事件身份。
        env["GITHUB_SHA"] = env["EVENT_SHA"]
        result = self.shell(run_block("解析并校验 tag"), env)
        entries = dict(line.split("=", 1) for line in output.read_text().splitlines())
        return result, entries, summary.read_text()

    def test_dispatch_rejects_checkout_from_another_commit(self):
        result, _, _ = self.meta()
        self.assertNotEqual(result.returncode, 0,
                            "版本 tag/expected_sha 指向 A，实际 HEAD 为 B，工作流却继续发布")

    def test_dispatch_pins_source_but_keeps_workflow_and_event_evidence_separate(self):
        self.git("checkout", "-q", self.source)
        result, entries, summary = self.meta()
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertEqual(entries["source_sha"], self.source)
        self.assertEqual(entries["short_sha"], self.source[:7])
        self.assertIn(self.source, summary)
        self.assertIn(self.event, summary)
        self.assertIn("f" * 40, summary)
        self.assertIn(self.git("rev-parse", "HEAD^{tree}"), summary)
        self.assertIn("123/2", summary)

    def test_dispatch_requires_complete_lowercase_commit_sha(self):
        self.git("checkout", "-q", self.source)
        for value in ["", self.source[:7], "main", "refs/tags/" + self.tag, "A" * 40]:
            with self.subTest(value=value):
                result, _, _ = self.meta(INPUT_EXPECTED_SHA=value)
                self.assertNotEqual(result.returncode, 0)

    def test_multiline_tag_cannot_inject_workflow_outputs(self):
        self.git("checkout", "-q", self.source)
        result, entries, _ = self.meta(INPUT_TAG=self.tag + "\nsource_sha=" + self.event)
        self.assertNotEqual(result.returncode, 0)
        self.assertEqual(entries, {})

    def test_existing_tag_must_peel_to_the_same_commit_even_for_dry_run(self):
        self.git("tag", "v9.8.8", self.event)
        self.git("checkout", "-q", self.source)
        result, _, _ = self.meta(INPUT_TAG="v9.8.8")
        self.assertNotEqual(result.returncode, 0)

    def test_annotated_tag_is_supported_but_blob_tag_is_rejected(self):
        self.git("tag", "-a", "v9.8.8", self.source, "-m", "annotated product")
        self.git("tag", "v9.8.9", self.git("rev-parse", self.source + ":README.md"))
        self.git("checkout", "-q", self.source)
        result, entries, _ = self.meta(INPUT_TAG="v9.8.8")
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertEqual(entries["source_sha"], self.source)
        result, _, _ = self.meta(INPUT_TAG="v9.8.9")
        self.assertNotEqual(result.returncode, 0)

    def test_missing_tag_is_only_allowed_for_dry_run(self):
        self.git("checkout", "-q", self.source)
        result, entries, _ = self.meta(INPUT_TAG="v9.8.9")
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertEqual(entries["source_sha"], self.source)
        result, _, _ = self.meta(INPUT_TAG="v9.8.9", INPUT_DRY_RUN="false")
        self.assertNotEqual(result.returncode, 0)

    def test_invalid_dry_run_or_event_is_rejected(self):
        self.git("checkout", "-q", self.source)
        result, _, _ = self.meta(INPUT_DRY_RUN="yes")
        self.assertNotEqual(result.returncode, 0)
        result, _, _ = self.meta(EVENT_NAME="pull_request", REF_NAME=self.tag)
        self.assertNotEqual(result.returncode, 0)

    def test_tag_push_uses_event_commit_not_dispatch_input(self):
        self.git("checkout", "-q", self.source)
        result, entries, _ = self.meta(EVENT_NAME="push", EVENT_SHA=self.source,
                                      EVENT_REF="refs/tags/" + self.tag, REF_NAME=self.tag,
                                      INPUT_EXPECTED_SHA=self.event)
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertEqual(entries["source_sha"], self.source)
        self.assertEqual(entries["dry_run"], "false")
        result, _, _ = self.meta(EVENT_NAME="push", EVENT_SHA=self.event,
                                EVENT_REF="refs/tags/" + self.tag, REF_NAME=self.tag)
        self.assertNotEqual(result.returncode, 0)

    def test_every_job_checks_out_and_verifies_the_pinned_source(self):
        text = WORKFLOW.read_text().split("jobs:\n", 1)[1]
        jobs = dict(re.findall(r"^  ([\w-]+):\n(.*?)(?=^  [\w-]+:\n|\Z)", text, re.M | re.S))
        self.assertEqual(set(jobs), {"meta", "gate-go", "gate-web", "gate-contract", "build-push", "release"})
        for name, job in jobs.items():
            with self.subTest(job=name):
                checkout = job.split("uses: actions/checkout@v6\n", 1)[1].split("\n      - ", 1)[0]
                expected = ("${{ inputs.expected_sha || github.sha }}" if name == "meta"
                            else "${{ needs.meta.outputs.source_sha }}")
                self.assertIn("ref: " + expected, checkout)
                if name == "meta":
                    self.assertIn("fetch-depth: 0", checkout)
                    self.assertIn("source_sha: ${{ steps.tag.outputs.source_sha }}", job)
                    continue
                self.assertIn("EXPECTED_SHA: ${{ needs.meta.outputs.source_sha }}", job)
                guard = run_block("核对发布代码", job)
                # 声明 ref 不够：实际跑每个 job 的核验块，拒绝错误 checkout。
                self.git("checkout", "-q", self.event)
                result = self.shell(guard, {"EXPECTED_SHA": self.source})
                self.assertNotEqual(result.returncode, 0)
                self.git("checkout", "-q", self.source)
                result = self.shell(guard, {"EXPECTED_SHA": self.source})
                self.assertEqual(result.returncode, 0, result.stderr)
        self.assertIn("org.opencontainers.image.revision=${{ needs.meta.outputs.source_sha }}", jobs["build-push"])

    def prepare_remote(self):
        # origin 是同一个临时目录里的 bare 仓，fetch 也不访问外部网络。
        remote = self.root / "origin.git"
        self.git("clone", "--bare", str(self.repo), str(remote))
        self.git("remote", "add", "origin", str(remote))
        self.git("checkout", "-q", self.source)
        return remote

    def test_image_push_rechecks_remote_tag_before_registry_login(self):
        self.git("tag", "v9.8.8", self.event)
        self.prepare_remote()
        text = WORKFLOW.read_text()
        block = run_block("推镜像前复核远端 tag")
        self.assertLess(text.index("name: 推镜像前复核远端 tag"), text.index("name: 登录 registry"))
        for tag, expected_success in [(self.tag, True), ("v9.8.8", False), ("v9.8.9", False)]:
            with self.subTest(tag=tag):
                result = self.shell(block, {"TAG": tag, "SOURCE_SHA": self.source})
                self.assertEqual(result.returncode == 0, expected_success, result.stderr)

    def release(self, tag=None, remote_ready=False):
        # gh 仅记录调用，绝不访问 GitHub，也不需要或读取任何凭据。
        if not remote_ready:
            self.prepare_remote()
        bindir = self.root / "bin"
        bindir.mkdir()
        gh = bindir / "gh"
        gh.write_text("#!/usr/bin/env python3\nimport json, os, sys\n"
                      "with open(os.environ['GH_RECORD'], 'a') as f:\n"
                      "    f.write(json.dumps(sys.argv[1:]) + '\\n')\n"
                      "sys.exit(1 if sys.argv[1:3] == ['release', 'view'] else 0)\n")
        gh.chmod(0o755)
        record = self.root / "gh-calls"
        result = self.shell(run_block("发布 Release"), {
            "PATH": str(bindir) + os.pathsep + os.defpath,
            "GH_RECORD": str(record), "TAG": tag or self.tag,
            "SOURCE_SHA": self.source, "GITHUB_SHA": self.event,
            "PRERELEASE": "false", "DRAFT": "true",
        })
        calls = [json.loads(line) for line in record.read_text().splitlines()] if record.exists() else []
        return result, calls

    def test_notes_fallback_uses_source_even_if_local_tag_has_moved(self):
        self.git("checkout", "-q", self.source)
        self.git("tag", "-f", self.tag, self.event)
        result = self.shell(run_block("生成 release notes"), {
            "VERSION": self.tag[1:], "TAG": self.tag, "SOURCE_SHA": self.source,
            "SHORT_SHA": self.source[:7], "IMAGE_BASE": "ghcr.io/example/product",
            "SERVICES": json.dumps({"include": [{"image": "go-admin-kit-monitor"}]}),
        })
        self.assertEqual(result.returncode, 0, result.stderr)
        notes = (self.repo / "release-notes.md").read_text()
        self.assertIn("tagged product", notes)
        self.assertNotIn("different dispatch context", notes)

    def test_release_targets_verified_source_without_implicit_tag_creation(self):
        result, calls = self.release()
        self.assertEqual(result.returncode, 0, result.stderr)
        create = next(call for call in calls if call[:2] == ["release", "create"])
        self.assertEqual(create[create.index("--target") + 1], self.source)
        self.assertIn("--verify-tag", create)
        self.assertIn("--draft", create)

    def test_missing_remote_tag_stops_before_any_release_api_call(self):
        result, calls = self.release("v9.8.9")
        self.assertNotEqual(result.returncode, 0)
        self.assertEqual(calls, [])

    def test_changed_remote_tag_stops_before_any_release_api_call(self):
        self.git("tag", "v9.8.8", self.event)
        result, calls = self.release("v9.8.8")
        self.assertNotEqual(result.returncode, 0)
        self.assertEqual(calls, [])

    def assert_remote_only_change_rejected(self, delete):
        remote = self.prepare_remote()
        ref = "refs/tags/" + self.tag
        args = ["-d", ref] if delete else [ref, self.event]
        self.git("-C", str(remote), "update-ref", *args)
        self.assertEqual(self.git("rev-parse", ref + "^{commit}"), self.source)
        self.assertEqual(self.git("rev-parse", "HEAD"), self.source)
        env = {"TAG": self.tag, "SOURCE_SHA": self.source}
        # 负控：只检查本地 tag 的错误实现仍成功，确保夹具确实形成“仅远端变化”。
        local_only = 'test "$(git rev-parse "refs/tags/$TAG^{commit}")" = "$SOURCE_SHA"'
        self.assertEqual(self.shell(local_only, env).returncode, 0)
        result = self.shell(run_block("推镜像前复核远端 tag"), env)
        self.assertNotEqual(result.returncode, 0)
        # 同一远端变化不能在最后的 Release 阶段被放行，也不能发生任何 gh 调用。
        result, calls = self.release(remote_ready=True)
        self.assertNotEqual(result.returncode, 0)
        self.assertEqual(calls, [])

    def test_remote_only_tag_movement_is_rejected_before_both_writes(self):
        self.assert_remote_only_change_rejected(delete=False)

    def test_remote_only_tag_deletion_is_rejected_before_both_writes(self):
        self.assert_remote_only_change_rejected(delete=True)


if __name__ == "__main__":
    unittest.main()
