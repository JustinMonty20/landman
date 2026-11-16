#!/usr/bin/env python3
"""Exercise review preflight and receipt reuse in a disposable Git repository."""

import hashlib
import json
import subprocess
import sys
import tempfile
import unittest
from pathlib import Path

SCRIPT = Path(__file__).with_name("verify-review.py")


class ReviewVerificationTest(unittest.TestCase):
    def setUp(self):
        self.temp = tempfile.TemporaryDirectory()
        self.addCleanup(self.temp.cleanup)
        self.root = Path(self.temp.name)
        self.repo = self.root / "repo"
        self.repo.mkdir()
        self.git("init", "-q", "-b", "main")
        self.git("config", "user.name", "Test")
        self.git("config", "user.email", "test@example.invalid")
        (self.repo / "base.txt").write_text("base\n")
        self.git("add", ".")
        self.git("commit", "-qm", "base")
        self.git("checkout", "-qb", "change")
        (self.repo / "change.txt").write_text("change\n")
        self.git("add", ".")
        self.git("commit", "-qm", "change")
        self.receipt = self.root / "lane.txt"
        self.receipt.write_text("PASS\n")
        self.source = self.root / "source.zip"
        self.source.write_bytes(b"private test input")
        self.manifest = self.root / "evidence.json"

    def git(self, *args):
        return subprocess.run(["git", *args], cwd=self.repo, capture_output=True, check=True, text=True).stdout.strip()

    def command(self, *args):
        return subprocess.run([sys.executable, SCRIPT, *map(str, args)], cwd=self.repo, capture_output=True, text=True)

    def save(self):
        result = self.command(
            "record", "--base", "main", "--output", self.manifest,
            "--receipt", self.receipt, "--input", f"billing={self.source}",
        )
        self.assertEqual(result.returncode, 0, result.stderr)

    def test_profile_requires_reader_and_shell(self):
        profile = self.root / "reviewer.md"
        profile.write_text("---\nname: reviewer\ntools: read, grep, find\n---\n")
        result = self.command("profile", profile)
        self.assertEqual(result.returncode, 2)
        self.assertIn("lacks: bash", result.stderr)
        profile.write_text("---\nname: worker\ntools: read, bash\n---\n")
        self.assertIn("PROFILE PASS", self.command("profile", profile).stdout)
        self.assertIn("SELF-CHECK PASS", self.command("self-check", "--require", "git").stdout)
        missing = self.command("self-check", "--require", "command-that-does-not-exist")
        self.assertEqual(missing.returncode, 2)
        self.assertIn("commands unavailable", missing.stderr)

    def test_exact_and_same_patch_on_new_base(self):
        self.save()
        self.assertIn("EXACT", self.command("check", self.manifest).stdout)
        data = json.loads(self.manifest.read_text())
        self.assertEqual(data["inputs"]["billing"]["sha256"], hashlib.sha256(self.source.read_bytes()).hexdigest())
        self.assertNotIn(self.source.read_bytes().decode(), self.manifest.read_text())
        self.git("checkout", "-q", "main")
        (self.repo / "other.txt").write_text("unrelated\n")
        self.git("add", ".")
        self.git("commit", "-qm", "new base")
        self.git("checkout", "-q", "change")
        self.git("rebase", "-q", "main")
        result = self.command("check", self.manifest, "--base", "main")
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertIn("UNCHANGED PATCH", result.stdout)
        self.assertEqual(self.command("check", self.manifest).returncode, 1)

    def test_changed_input_receipt_or_patch_is_stale(self):
        self.save()
        self.receipt.write_text("FAIL\n")
        self.assertIn("STALE", self.command("check", self.manifest).stdout)
        self.receipt.write_text("PASS\n")
        self.source.write_bytes(b"new private test input")
        self.assertIn("STALE", self.command("check", self.manifest).stdout)
        self.source.write_bytes(b"private test input")
        (self.repo / "change.txt").write_text("different change\n")
        self.git("add", ".")
        self.git("commit", "-qm", "new behavior")
        result = self.command("check", self.manifest)
        self.assertEqual(result.returncode, 1)
        self.assertIn("committed patch changed", result.stdout)

    def test_whitespace_change_cannot_reuse_patch(self):
        (self.repo / "change.txt").write_text('println("a b")\n')
        self.git("add", ".")
        self.git("commit", "--amend", "-qm", "change")
        self.save()
        (self.repo / "change.txt").write_text('println("ab")\n')
        self.git("add", ".")
        self.git("commit", "--amend", "-qm", "change")
        result = self.command("check", self.manifest)
        self.assertEqual(result.returncode, 1, result.stdout + result.stderr)
        self.assertIn("STALE", result.stdout)

    def test_dirty_tree_cannot_record_or_reuse(self):
        source = self.repo / "change.txt"
        source.write_text("untested change\n")
        record = self.command(
            "record", "--base", "main", "--output", self.manifest,
            "--receipt", self.receipt, "--input", f"billing={self.source}",
        )
        self.assertEqual(record.returncode, 2, record.stdout + record.stderr)
        self.git("restore", "change.txt")
        self.save()
        source.write_text("untested change\n")
        self.assertEqual(self.command("check", self.manifest).returncode, 2)
        self.git("add", "change.txt")
        self.assertEqual(self.command("check", self.manifest).returncode, 2)
        self.git("restore", "--staged", "change.txt")
        self.git("restore", "change.txt")
        (self.repo / "untracked.go").write_text("package untracked\n")
        self.assertEqual(self.command("check", self.manifest).returncode, 2)

    def test_missing_input_or_existing_manifest_fails_closed(self):
        result = self.command("record", "--base", "main", "--output", self.manifest, "--receipt", self.receipt)
        self.assertEqual(result.returncode, 2)
        self.save()
        self.assertEqual(self.command("record", "--base", "main", "--output", self.manifest, "--receipt", self.receipt, "--input", f"billing={self.source}").returncode, 2)
        self.receipt.unlink()
        self.assertEqual(self.command("check", self.manifest).returncode, 2)


if __name__ == "__main__":
    unittest.main()
