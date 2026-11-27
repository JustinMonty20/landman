#!/usr/bin/env bash
# Use the repository targets without loading operator settings.
set -euo pipefail
target=${1:-}
case "$target" in
  build|db-create|db-drop|migrate-up|migrate-version|migrate-status) ;;
  *) echo 'Select build, db-create, db-drop, migrate-up, migrate-version, or migrate-status' >&2; exit 2 ;;
esac
shift
container=false
for argument in "$@"; do
  if [ "$target" = build ]; then
    echo 'Build accepts no additional arguments' >&2
    exit 2
  fi
  case "$argument" in
    DB_CONTAINER=landman-verify-?*)
      if [ "$container" = true ]; then
        echo 'Set DB_CONTAINER only once' >&2
        exit 2
      fi
      container=true
      ;;
    POSTGRES_URL=*|DATABASE_URL=*|GOOSE_VERSION=*|GOOSE_CMD=*) ;;
    *) echo 'Use one target and documented verification assignments only' >&2; exit 2 ;;
  esac
done
if [ "$target" != build ] && [ "$container" != true ]; then
  echo 'Require an explicit DB_CONTAINER=landman-verify-<name>' >&2
  exit 2
fi
[ -f Makefile ] || { echo 'Run from the repository root' >&2; exit 2; }
makefile=$(mktemp)
trap 'rm -f "$makefile"' EXIT
awk '$0 != "-include .env" { print }' Makefile >"$makefile"
env -i PATH="$PATH" HOME="$HOME" make -f "$makefile" "$target" "$@"
