#!/usr/bin/env python3
"""Check reviewer tools and the validity of external verification receipts."""

import argparse
import hashlib
import json
import shutil
import subprocess
import sys
from pathlib import Path


def git(*args):
    return subprocess.run(["git", *args], check=True, capture_output=True, text=True).stdout.strip()


def profile_tools(path):
    lines = Path(path).read_text().splitlines()
    if not lines or lines[0] != "---":
        raise ValueError("agent profile has no frontmatter")
    for line in lines[1:]:
        if line == "---":
            break
        if line.startswith("tools:"):
            return {tool.strip() for tool in line.removeprefix("tools:").split(",")}
    raise ValueError("agent profile has no single-line tools list")


def digest(path):
    file = Path(path)
    value = hashlib.sha256()
    with file.open("rb") as body:
        for chunk in iter(lambda: body.read(1 << 20), b""):
            value.update(chunk)
    return {"path": str(file), "size": file.stat().st_size, "sha256": value.hexdigest()}


def clean_tree():
    if git("status", "--porcelain", "--untracked-files=all"):
        raise ValueError("working tree is dirty; commit changes before recording or checking evidence")


def fingerprints(base):
    diff = subprocess.run(
        ["git", "diff", "--binary", "--full-index", f"{base}...HEAD"],
        check=True, capture_output=True,
    ).stdout
    result = subprocess.run(
        ["git", "patch-id", "--stable"], input=diff, check=True, capture_output=True,
    ).stdout.decode().split()
    if not result:
        raise ValueError("no committed patch exists against the selected base")
    return result[0], hashlib.sha256(diff).hexdigest()


def inputs(values):
    result = {}
    for value in values:
        name, sep, path = value.partition("=")
        if not sep or not name or not path or name in result:
            raise ValueError("each input needs one unique name=path")
        result[name] = digest(path)
    return result


def record(args):
    clean_tree()
    if not args.receipt or not args.input:
        raise ValueError("record needs at least one receipt and one named input")
    base = git("rev-parse", "--verify", args.base)
    patch, full_diff = fingerprints(base)
    data = {
        "version": 2,
        "head": git("rev-parse", "HEAD"),
        "base": base,
        "patch_id": patch,
        "diff_sha256": full_diff,
        "receipts": [digest(path) for path in args.receipt],
        "inputs": inputs(args.input),
    }
    destination = Path(args.output)
    if destination.exists():
        raise ValueError("output already exists; keep each run's evidence immutable")
    destination.write_text(json.dumps(data, indent=2, sort_keys=True) + "\n")
    print(f"RECORDED {data['head']} {destination}")


def check(args):
    clean_tree()
    data = json.loads(Path(args.manifest).read_text())
    if data.get("version") != 2:
        raise ValueError("unsupported receipt manifest version")
    current = git("rev-parse", "HEAD")
    base = git("rev-parse", "--verify", args.base or data["base"])
    changed = []
    for item in data["receipts"] + list(data["inputs"].values()):
        if digest(item["path"]) != item:
            changed.append(item["path"])
    if changed:
        print("STALE: input or receipt bytes changed: " + ", ".join(changed))
        return 1
    if current == data["head"]:
        print("EXACT: head, input, and receipt bytes match")
        return 0
    patch, full_diff = fingerprints(base)
    if patch == data["patch_id"] and full_diff == data["diff_sha256"]:
        print("UNCHANGED PATCH: full diff, input, and receipt bytes match; the head changed")
        return 0
    print("STALE: the committed patch changed; use the plan's focused verifier rule")
    return 1


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    commands = parser.add_subparsers(dest="command", required=True)
    profile = commands.add_parser("profile", help="check agent profile before review launch")
    profile.add_argument("path")
    self_check = commands.add_parser("self-check", help="run inside the review child before Git or test work")
    self_check.add_argument("--require", action="append", default=[], help="extra command needed by this lane")
    save = commands.add_parser("record", help="save external verification evidence")
    save.add_argument("--base", required=True)
    save.add_argument("--output", required=True)
    save.add_argument("--receipt", action="append", default=[])
    save.add_argument("--input", action="append", default=[])
    verify = commands.add_parser("check", help="check whether saved evidence is still valid")
    verify.add_argument("manifest")
    verify.add_argument("--base", help="new base after a rebase")
    args = parser.parse_args()
    try:
        if args.command == "profile":
            missing = {"read", "bash"} - profile_tools(args.path)
            if missing:
                raise ValueError("review profile lacks: " + ", ".join(sorted(missing)))
            print("PROFILE PASS: read and bash tools declared; child must run self-check")
        elif args.command == "self-check":
            missing = [name for name in ["git", *args.require] if not shutil.which(name)]
            if missing:
                raise ValueError("commands unavailable in this child: " + ", ".join(missing))
            git("rev-parse", "--show-toplevel")
            print("SELF-CHECK PASS: Git works; required commands are available")
        elif args.command == "record":
            record(args)
        else:
            return check(args)
    except (OSError, ValueError, KeyError, subprocess.CalledProcessError) as error:
        print(f"BLOCKED: {error}", file=sys.stderr)
        return 2
    return 0


if __name__ == "__main__":
    sys.exit(main())
