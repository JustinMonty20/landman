#!/usr/bin/env bash
# Use the repository targets without loading operator settings.
set -euo pipefail
case "${1:-}" in
  build) ;;
  db-create|db-drop|migrate-up|migrate-version|migrate-status)
    container=false
    for argument in "$@"; do
      case "$argument" in DB_CONTAINER=landman-verify-*) container=true ;; esac
    done
    if [ "$container" != true ]; then
      echo 'Require an explicit DB_CONTAINER=landman-verify-<name>' >&2
      exit 2
    fi
    ;;
  *) echo 'Select build, db-create, db-drop, migrate-up, migrate-version, or migrate-status' >&2; exit 2 ;;
esac
[ -f Makefile ] || { echo 'Run from the repository root' >&2; exit 2; }
makefile=$(mktemp)
trap 'rm -f "$makefile"' EXIT
awk '$0 != "-include .env" { print }' Makefile >"$makefile"
env -i PATH="$PATH" HOME="$HOME" make -f "$makefile" "$@"
