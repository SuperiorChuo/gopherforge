"""公开迁移身份的真实文件负例；不把静态检查冒充数据库升级演练。"""

import hashlib
import importlib.util
from pathlib import Path
import tempfile
import unittest

ROOT = Path(__file__).resolve().parents[2]
SPEC = importlib.util.spec_from_file_location("release_migrations", ROOT / "scripts/release-migrations.py")
MIGRATIONS = importlib.util.module_from_spec(SPEC)
SPEC.loader.exec_module(MIGRATIONS)


class MigrationHistoryTests(unittest.TestCase):
    def setUp(self):
        self.temp = tempfile.TemporaryDirectory(prefix="gak-release-migrations-")
        self.addCleanup(self.temp.cleanup)
        self.directory = Path(self.temp.name)
        # 特意留下 000002 缺号，证明不能在最高公开版本以下补填。
        self.files = {"000001_init.sql": b"-- +goose Up\nSELECT 1;\n",
                      "000003_next.sql": b"-- +goose Up\nSELECT 3;\n"}
        self.expected = {}
        for name, content in self.files.items():
            (self.directory / name).write_bytes(content)
            self.expected[name] = hashlib.sha256(content).hexdigest()

    def check(self):
        return MIGRATIONS.check_history(self.directory, self.expected)

    def assertRejected(self, category):
        issues = self.check()
        self.assertTrue(any(issue.startswith(category + ":") for issue in issues), issues)

    def test_identical_history_and_new_forward_version_pass(self):
        (self.directory / "000004_business.sql").write_text("-- +goose Up\nSELECT 4;\n")
        self.assertEqual(self.check(), [])

    def test_changed_sql_is_rejected_even_when_only_comment_changes(self):
        path = self.directory / "000001_init.sql"
        path.write_bytes(path.read_bytes() + b"-- changed\n")
        self.assertRejected("changed")

    def test_deleted_history_is_rejected(self):
        (self.directory / "000001_init.sql").unlink()
        self.assertRejected("missing")

    def test_rename_with_identical_content_is_rejected(self):
        (self.directory / "000001_init.sql").rename(self.directory / "000001_renamed.sql")
        self.assertRejected("missing")
        self.assertRejected("historical-insert")

    def test_reused_historical_version_is_rejected(self):
        (self.directory / "000003_business.sql").write_text("SELECT 9;\n")
        self.assertRejected("duplicate-version")

    def test_duplicate_new_version_is_rejected(self):
        for name in ["000004_first.sql", "000004_second.sql"]:
            (self.directory / name).write_text("SELECT 4;\n")
        self.assertRejected("duplicate-version")

    def test_filling_old_gap_is_rejected(self):
        (self.directory / "000002_gap.sql").write_text("SELECT 2;\n")
        self.assertRejected("historical-insert")

    def test_renumbering_history_is_rejected(self):
        (self.directory / "000001_init.sql").rename(self.directory / "000004_init.sql")
        self.assertRejected("missing")

    def test_symlink_is_rejected_even_if_bytes_match(self):
        path = self.directory / "000001_init.sql"
        path.rename(self.directory / "original.txt")
        path.symlink_to(self.directory / "original.txt")
        self.assertRejected("not-regular")

    def test_sql_named_directory_is_rejected(self):
        (self.directory / "000004_directory.sql").mkdir()
        self.assertRejected("not-regular")

    def test_invalid_sql_name_is_rejected(self):
        (self.directory / "4_bad.sql").write_text("SELECT 4;\n")
        self.assertRejected("invalid-name")

    def test_bare_sql_filename_is_rejected(self):
        # Path('.sql').suffix 为空，但 Goose 的 *.sql 发现路径仍会遇到它。
        (self.directory / ".sql").write_text("SELECT 4;\n")
        self.assertRejected("invalid-name")

    def test_bare_sql_symlink_is_rejected(self):
        (self.directory / ".sql").symlink_to(self.directory / "000001_init.sql")
        self.assertRejected("invalid-name")

    def test_missing_candidate_directory_fails(self):
        with self.assertRaisesRegex(ValueError, "迁移目录不存在"):
            MIGRATIONS.check_history(self.directory / "absent", self.expected)

    def test_empty_baseline_fails_instead_of_accepting_everything(self):
        with self.assertRaisesRegex(ValueError, "基线为空"):
            MIGRATIONS.check_history(self.directory, {})

    def test_missing_git_object_fails_without_fetching(self):
        with self.assertRaisesRegex(ValueError, "基线 Git 对象不可读取"):
            MIGRATIONS.load_baseline(ROOT, "0" * 40)


if __name__ == "__main__":
    unittest.main()
