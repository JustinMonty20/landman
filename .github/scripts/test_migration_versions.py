"""Exercise the migration version gate through its command-line interface."""
import subprocess
import sys
import tempfile
import unittest
from pathlib import Path

SCRIPT = Path(__file__).with_name("check-migration-versions.py")
ROOT = SCRIPT.parents[2]


class MigrationVersionsTest(unittest.TestCase):
    def check(self, directory=None):
        command = [sys.executable, str(SCRIPT)]
        if directory is not None:
            command.append(str(directory))
        return subprocess.run(command, cwd=ROOT, capture_output=True, text=True)

    def test_current_migrations(self):
        result = self.check()
        self.assertEqual(result.returncode, 0, result.stderr)

    def test_duplicate_versions_name_both_files(self):
        with tempfile.TemporaryDirectory() as directory:
            for number in (20, 21):
                first = f"0000{number}_parcel_api.sql"
                second = f"0000{number}_qualification.sql"
                Path(directory, first).touch()
                Path(directory, second).touch()
            result = self.check(directory)
            self.assertNotEqual(result.returncode, 0)
            for number in (20, 21):
                self.assertIn(f"duplicate migration version {number}", result.stderr)
                self.assertIn(f"0000{number}_parcel_api.sql", result.stderr)
                self.assertIn(f"0000{number}_qualification.sql", result.stderr)

    def test_leading_zero_equivalence(self):
        with tempfile.TemporaryDirectory() as directory:
            Path(directory, "000020_first.sql").touch()
            Path(directory, "20_second.sql").touch()
            result = self.check(directory)
            self.assertNotEqual(result.returncode, 0)
            self.assertIn("000020_first.sql", result.stderr)
            self.assertIn("20_second.sql", result.stderr)

    def test_noncontiguous_positive_versions(self):
        with tempfile.TemporaryDirectory() as directory:
            Path(directory, "000001_first.sql").touch()
            Path(directory, "000023_last.sql").touch()
            result = self.check(directory)
            self.assertEqual(result.returncode, 0, result.stderr)

    def test_zero_version_is_rejected(self):
        with tempfile.TemporaryDirectory() as directory:
            Path(directory, "000000_invalid.sql").touch()
            result = self.check(directory)
            self.assertNotEqual(result.returncode, 0)
            self.assertIn("000000_invalid.sql", result.stderr)


if __name__ == "__main__":
    unittest.main()
