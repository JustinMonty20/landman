#!/usr/bin/env python3
"""Test verification command guards without a database or network requests."""

import os
from pathlib import Path
import re
import subprocess
import tempfile
import unittest


SKILL = Path(__file__).resolve().parent


class MakeGuards(unittest.TestCase):
    def setUp(self):
        self.temp = tempfile.TemporaryDirectory()
        self.addCleanup(self.temp.cleanup)
        self.root = Path(self.temp.name)
        self.root.joinpath(".env").write_text("$(error operator settings were loaded)\n")
        self.root.joinpath("Makefile").write_text(
            "-include .env\n"
            ".PHONY: build db-drop migrate-down\n"
            "build db-drop migrate-down:\n"
            "\t@test -z '$(DATABASE_URL)'\n"
            "\t@printf '%s\\n' '$@' >> proof.txt\n"
        )

    def run_helper(self, *args):
        env = dict(os.environ, DATABASE_URL="operator-settings", MAKEFILES=".env")
        return subprocess.run(
            [str(SKILL / "verify-make.sh"), *args],
            cwd=self.root,
            env=env,
            capture_output=True,
            text=True,
            check=False,
        )

    def test_build_excludes_operator_settings(self):
        result = self.run_helper("build")
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertEqual(self.root.joinpath("proof.txt").read_text(), "build\n")

    def test_database_target_accepts_explicit_verification_container(self):
        result = self.run_helper("db-drop", "DB_CONTAINER=landman-verify-fixture")
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertEqual(self.root.joinpath("proof.txt").read_text(), "db-drop\n")

    def test_rejects_extra_targets_options_and_assignments(self):
        cases = [
            ("build", "db-drop"),
            ("build", "-f", "Makefile"),
            ("db-drop",),
            ("db-drop", "DB_CONTAINER=landman-postgis"),
            ("db-drop", "DB_CONTAINER=landman-verify-"),
            ("db-drop", "DB_CONTAINER=landman-verify-fixture", "migrate-down"),
            ("db-drop", "DB_CONTAINER=landman-verify-fixture", "--eval=extra:;"),
            ("db-drop", "DB_CONTAINER=landman-verify-fixture", "-f", "Makefile"),
            ("db-drop", "DB_CONTAINER=landman-verify-fixture", "MAKEFILES=.env"),
            ("db-drop", "DB_CONTAINER=landman-verify-fixture", "DB_CONTAINER=landman-postgis"),
            ("db-drop", "DB_CONTAINER=landman-postgis", "DB_CONTAINER=landman-verify-fixture"),
            ("db-drop", "DB_CONTAINER=landman-verify-fixture", "DB_CONTAINER=landman-verify-other"),
            ("migrate-down",),
        ]
        for args in cases:
            with self.subTest(args=args):
                result = self.run_helper(*args)
                self.assertEqual(result.returncode, 2, result.stderr)
                self.assertFalse(self.root.joinpath("proof.txt").exists())


class BootstrapStatus(unittest.TestCase):
    def test_stops_after_failure_and_preserves_each_status(self):
        guide = SKILL.joinpath("live-gis.md").read_text()
        blocks = [block for block in re.findall(r"```sh\n(.*?)```", guide, re.S) if "setup_step()" in block]
        self.assertEqual(len(blocks), 1)
        steps = ["db-drop", "db-create", "migrate-up", "check-schema", "seed", "check-loaded", "build"]
        for failed, code in [("", 0), ("migrate-up", 17), ("check-schema", 23), ("seed", 29)]:
            with self.subTest(failed=failed), tempfile.TemporaryDirectory() as temp:
                root = Path(temp)
                bin_dir = root / "bin"
                bin_dir.mkdir()
                evidence = root / "evidence"
                evidence.mkdir()
                seed = root / ".cursor/skills/verify-landman-cli/seed.sql"
                seed.parent.mkdir(parents=True)
                seed.write_text("")
                stub = (
                    "#!/usr/bin/env bash\n"
                    "set -eu\n"
                    "case ${0##*/} in\n"
                    " verify_make) step=$1 ;;\n"
                    " go) step=$3 ;;\n"
                    " docker) step=seed ;;\n"
                    "esac\n"
                    "printf '%s\\n' \"$step\" >>\"$EVENTS\"\n"
                    "if [ \"$step\" = \"$FAIL_STEP\" ]; then exit \"$FAIL_STATUS\"; fi\n"
                )
                for command in ["verify_make", "go", "docker"]:
                    path = bin_dir / command
                    path.write_text(stub)
                    path.chmod(0o755)
                env = dict(
                    os.environ,
                    PATH=str(bin_dir) + os.pathsep + os.environ["PATH"],
                    EVIDENCE=str(evidence),
                    EVENTS=str(root / "events.txt"),
                    FAIL_STEP=failed,
                    FAIL_STATUS=str(code),
                    VERIFY_CONTAINER="landman-verify-fixture",
                    POSTGRES_URL="fixture",
                    GOOSE_DATABASE_URL="fixture",
                )
                result = subprocess.run(
                    ["bash", "-c", blocks[0]], cwd=root, env=env,
                    capture_output=True, text=True, check=False,
                )
                self.assertEqual(result.returncode, code, result.stderr)
                expected = steps[:steps.index(failed) + 1] if failed else steps
                self.assertEqual(root.joinpath("events.txt").read_text().splitlines(), expected)
                self.assertEqual(len(list(evidence.glob("*.exit.txt"))), len(expected))
                for step in expected:
                    status = code if step == failed else 0
                    self.assertEqual(evidence.joinpath(step + ".exit.txt").read_text(), str(status) + "\n")


if __name__ == "__main__":
    unittest.main()
