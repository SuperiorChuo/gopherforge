import importlib.util
import subprocess
import tempfile
import unittest
from pathlib import Path

ROOT = Path(__file__).resolve().parents[2]
SPEC = importlib.util.spec_from_file_location("find_scanner", ROOT / "scripts/scan-unbounded-find.py")
scanner = importlib.util.module_from_spec(SPEC)
SPEC.loader.exec_module(scanner)


class FindScannerTest(unittest.TestCase):
    def test_multiline_limit(self):
        text = 'func list() {\n db.Order("tenant_id ASC").\n  Limit(maxOverviewAccounts).Find(&rows)\n}'
        self.assertEqual(scanner.scan_text("store.go", text), [])
        self.assertEqual(len(scanner.scan_text("store.go", text.replace("Limit(maxOverviewAccounts).", ""))), 1)

    def test_only_literal_goquery_calls_are_exempt(self):
        prefix = 'package mixed\nimport "github.com/PuerkitoBio/goquery"\nfunc list() {\n'
        for line in ['doc.Find("a[href]").Text()', 'doc.Find(`div .item`)']:
            self.assertEqual(scanner.scan_text("mixed.go", prefix + line), [])
        for line in ['db.Find(&rows)', 'db.Find(destination)', 'doc.Find("a"); db.Find(&rows)']:
            self.assertEqual(len(scanner.scan_text("mixed.go", prefix + line)), 1)
        self.assertEqual(len(scanner.scan_text("other.go", 'db.Find("not-a-proven-DOM-call")')), 1)

    def added(self, before, after, before_path="store.go", after_path="store.go"):
        return scanner.added_findings(scanner.scan_text(after_path, after),
                                     scanner.scan_text(before_path, before),
                                     lambda _: after, lambda _: before)

    def test_line_moves_do_not_clear_existing_warning(self):
        before = 'func list() {\n q.Find(&rows)\n}\n'
        after = '\n// unrelated insertion\n' + before
        self.assertEqual(self.added(before, after), [])
        self.assertEqual(len(scanner.scan_text("store.go", after)), 1)

    def test_identity_is_not_a_function_or_directory_waiver(self):
        before = 'func list() {\n q.Find(&rows)\n}\n'
        for after in [before.replace("list()", "other()"), before.replace("&rows", "&secrets"),
                      before.replace(" q.Find(&rows)", " q.Find(&rows)\n q.Find(&rows)")]:
            self.assertEqual(len(self.added(before, after)), 1)
        self.assertEqual(len(self.added(before, before, after_path="other.go")), 1)
        self.assertEqual(len(self.added(before.replace("q.Find", "q.Limit(10).Find"), before)), 1)

    def test_staged_comparison_reads_index_not_worktree(self):
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp)
            def git(*args):
                return subprocess.run(["git", "-C", tmp, *args], check=True, capture_output=True, text=True)
            git("init", "-q")
            git("config", "user.name", "Fixture")
            git("config", "user.email", "fixture@example.invalid")
            path = root / "store.go"
            before = 'package fixture\nfunc list() {\n q.Find(&rows)\n}\n'
            path.write_text(before)
            git("add", "store.go")
            git("commit", "-qm", "fixture baseline")
            path.write_text("// insertion\n" + before)
            git("add", "store.go")
            path.write_text(path.read_text() + 'func another() {\n q.Find(&other)\n}\n')
            current = scanner.scan_staged(root)
            base = scanner.scan_ref(root, "HEAD")
            self.assertEqual(scanner.added_findings(current, base,
                lambda rel: scanner.staged_text(root, rel), lambda rel: scanner.git_show(root, "HEAD", rel)), [])
            self.assertEqual(len(scanner.added_findings(scanner.scan_tree(root), base,
                lambda rel: (root / rel).read_text(), lambda rel: scanner.git_show(root, "HEAD", rel))), 1)

    def test_standalone_media_reader_matches_shared_reader(self):
        shared = ROOT / "microservices/services/shared/pkg/fullread/fullread.go"
        media = ROOT / "freeswitch-cc/control-api/internal/fullread/fullread.go"
        self.assertEqual(shared.read_bytes(), media.read_bytes())


if __name__ == "__main__":
    unittest.main()
