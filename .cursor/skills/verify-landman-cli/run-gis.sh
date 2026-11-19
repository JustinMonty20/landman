#!/usr/bin/env bash
set -eu

: "${HOST_DATABASE_URL:?Set the isolated host database URL}"
: "${EVIDENCE:?Set the evidence directory}"
binary=${VERIFY_LANDMAN_BINARY:-./landman}
umask 077
artifact_root="$EVIDENCE/private"
mkdir -p "$artifact_root"
: >"$EVIDENCE/import.stdout.txt"
: >"$EVIDENCE/import.stderr.txt"

for source in soil parcels; do
  if DATABASE_URL="$HOST_DATABASE_URL" PARCEL_EVIDENCE_DIR="$artifact_root" "$binary" run "union.gis.$source" \
    >"$EVIDENCE/$source.stdout.txt" 2>"$EVIDENCE/$source.stderr.txt"; then
    status=0
  else
    status=$?
  fi
  printf '%s\n' "$status" >"$EVIDENCE/$source.exit.txt"
  cat "$EVIDENCE/$source.stdout.txt" >>"$EVIDENCE/import.stdout.txt"
  cat "$EVIDENCE/$source.stderr.txt" >>"$EVIDENCE/import.stderr.txt"
  printf '%s\n' "$status" >"$EVIDENCE/import.exit.txt"
  if [ "$status" -ne 0 ]; then
    exit "$status"
  fi
done
