#!/usr/bin/env python3
"""Reject duplicate numeric Goose SQL migration versions before Goose reads them."""
import argparse
import re
import sys
from pathlib import Path


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("directory", nargs="?", type=Path, default=Path("db/migrations"))
    args = parser.parse_args()
    if not args.directory.is_dir():
        parser.error(f"migration directory does not exist: {args.directory}")

    versions = {}
    errors = []
    for path in sorted(args.directory.glob("*.sql")):
        match = re.fullmatch(r"([0-9]+)_.+\.sql", path.name)
        if not match or int(match.group(1)) == 0:
            errors.append(f"invalid positive SQL migration version: {path}")
            continue
        version = int(match.group(1))
        if version in versions:
            errors.append(f"duplicate migration version {version}: {versions[version]} and {path}")
        else:
            versions[version] = path
    if errors:
        print("\n".join(errors), file=sys.stderr)
        return 1
    print(f"Migration versions unique: {len(versions)} SQL files in {args.directory}")
    return 0


if __name__ == "__main__":
    sys.exit(main())
