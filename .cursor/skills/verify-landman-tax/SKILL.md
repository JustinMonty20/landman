---
name: verify-landman-tax
description: Verify manual Union tax ingestion, file retention, atomic publication, and the combined CLI path. Use for union.tax, taxjob, taxartifact, taxsource, taxstore, or tax migrations.
---
# Verify Landman Tax

Use the small four-file golden fixture by default. Operator files are not required for a default fixture pass. Run from the repository root. Read the current manual-file boundary in `plans/tax-data-ingest.md`. Use [database setup](../verify-landman/database.md) with `VERIFY_DB_NAME=landman_tax_integration`. Never source `.env` or use Make. The guarded tax database requires loopback and a port other than 5432.

## Offline and database fixtures

Require `pdftotext` from Poppler on PATH. These commands use checked-in inputs and local test services, not county HTTP:

```sh
go test -race ./internal/connector/union -run '(Delinquent|Billing|Foreclosure|Pipeline)' -count=1
go test -race ./internal/connector/union/taxjob ./internal/taxartifact ./internal/taxsource -count=1
DATABASE_URL="$VERIFY_DATABASE_URL" go test -race -tags postgres ./internal/taxstore -count=1 -v
DATABASE_URL="$VERIFY_DATABASE_URL" TAX_GOLDEN_DB=1 \
  go test -race -tags postgres ./cmd/landman \
  -run '^TestGoldenUnionTaxCI$' -count=1 -v
```

Require the golden test to execute, not skip. It exercises the configured tax job, all four feed tables, retained files, identical retry, and the existing performance gate. Store tests check failed publication and active-import retention. These fixtures do not prove that operator files are available.

## Retained capture and replay fixtures

Use a new database setup shell with `VERIFY_DB_NAME=taxworker_verify`. Run:

```sh
TAX_SHARED_TEST_DSN="$VERIFY_DATABASE_URL" TAX_SHARED_TEST_DISPOSABLE=1 \
  go test -race -tags postgres ./internal/connector/union/taxjob \
  -run '^TestSharedTaxRunRetainsAndReplaysFourFiles$' -count=1 -v
```

Require this test to execute. It checks all four retained captures, replay attempts, unchanged and older content, failed publication, and later parcel links. This lane uses local fixture files. It does not require operator paths. The separate schema-upgrade fixture needs its own old-schema database and is not this replay check.

## Optional operator-file drive

Require explicit operator approval and all four input paths. Do not request source URLs. Known county URLs are provenance only. The application does not download these files. Keep files unchanged while the job reads them.

Run this block in a new database setup shell. It creates a private retained-artifact directory. Record its retention decision before the drive. Do not delete retained files as database cleanup.

```sh
: "${UNION_TAX_DELINQUENT_PATH:?Require the operator delinquent file}"
: "${UNION_TAX_BILLING_PATH:?Require the operator billing file}"
: "${UNION_TAX_FORECLOSURE_PATH:?Require the operator foreclosure file}"
: "${UNION_TAX_PIPELINE_PATH:?Require the operator pipeline file}"
umask 077
EVIDENCE="artifacts/verify/tax/$(date +%Y%m%dT%H%M%S)-$$"
mkdir -p "$EVIDENCE/private"
export EVIDENCE
GOFLAGS= go build -o "$EVIDENCE/landman" ./cmd/landman
run_tax_cli() {
  env -i PATH="$PATH" HOME="$HOME" DATABASE_URL="$VERIFY_DATABASE_URL" \
    PARCEL_EVIDENCE_DIR="$PWD/$EVIDENCE/private" \
    UNION_TAX_ARTIFACT_DIR="$PWD/$EVIDENCE/private" \
    UNION_TAX_DELINQUENT_PATH="$UNION_TAX_DELINQUENT_PATH" \
    UNION_TAX_BILLING_PATH="$UNION_TAX_BILLING_PATH" \
    UNION_TAX_FORECLOSURE_PATH="$UNION_TAX_FORECLOSURE_PATH" \
    UNION_TAX_PIPELINE_PATH="$UNION_TAX_PIPELINE_PATH" \
    "$EVIDENCE/landman" "$@"
}
if run_tax_cli run union.tax >"$EVIDENCE/tax.stdout.txt" 2>"$EVIDENCE/tax.stderr.txt"; then
  status=0
else
  status=$?
fi
printf '%s\n' "$status" >"$EVIDENCE/tax.exit.txt"
[ "$status" = 0 ]
docker exec "$VERIFY_CONTAINER" psql -X -U verify -d "$VERIFY_DB_NAME" \
  -v ON_ERROR_STOP=1 -c "SELECT job_id,outcome,processed_count FROM job_attempts ORDER BY started_at;" \
  -c "SELECT source_name,encode(file_sha256,'hex') AS sha256,byte_size,storage_mode,validation_result,active,artifact_key FROM tax_file_imports ORDER BY source_name,created_at;" \
  -c "SELECT 'delinquent' AS feed,count(*) FROM tax_delinquent_balances UNION ALL SELECT 'bills',count(*) FROM tax_bills UNION ALL SELECT 'foreclosure',count(*) FROM tax_foreclosure_cases UNION ALL SELECT 'pipeline',count(*) FROM tax_foreclosure_pipeline;" \
  >"$EVIDENCE/publication.txt"
```

Require one successful `union.tax` attempt and four valid active local imports. Compare stored hashes and byte counts with `sha256sum` and `wc -c` on the supplied files. Check each artifact key under the private directory against the same hash. Record counts even when zero parcels match. Missing matches are not zero debt.

For an approved real-file failure-retention check, run this after the successful drive. It creates a separate empty input and does not modify an operator file.

```sh
active_tax_rows() {
  docker exec "$VERIFY_CONTAINER" psql -X -U verify -d "$VERIFY_DB_NAME" \
    -v ON_ERROR_STOP=1 -At \
    -c "SELECT source_name,encode(file_sha256,'hex') FROM tax_file_imports WHERE active ORDER BY source_name;"
}
active_tax_rows >"$EVIDENCE/active-before.txt"
: >"$EVIDENCE/invalid.csv"
if (UNION_TAX_DELINQUENT_PATH="$PWD/$EVIDENCE/invalid.csv"; run_tax_cli run union.tax) \
  >"$EVIDENCE/rejected.stdout.txt" 2>"$EVIDENCE/rejected.stderr.txt"; then
  status=0
else
  status=$?
fi
printf '%s\n' "$status" >"$EVIDENCE/rejected.exit.txt"
[ "$status" -ne 0 ]
active_tax_rows >"$EVIDENCE/active-after.txt"
cmp "$EVIDENCE/active-before.txt" "$EVIDENCE/active-after.txt"
docker exec "$VERIFY_CONTAINER" psql -X -U verify -d "$VERIFY_DB_NAME" \
  -v ON_ERROR_STOP=1 -c "SELECT job_id,outcome FROM job_attempts ORDER BY started_at DESC LIMIT 1;" \
  >"$EVIDENCE/rejected-attempt.txt"
rm "$EVIDENCE/invalid.csv"
```

Require the latest attempt to be `union.tax` with outcome `failed`. Automated store tests supply rollback proof when this file check is not approved; report the distinction.

## Optional combined bare-command drive

This is a separate opt-in check, not the tax-only drive. Require all four tax paths, a FEMA catalog path, and a FEMA archive path. FEMA is the Federal Emergency Management Agency. Require approval for county parcel, soil, zoning, road, structure, and assessed-value requests, plus USDA requests. Use a new empty migrated database. Do not also run the separate GIS journey in this pass.

Before a live recommendation, compare current source counts with each client's row, receipt, byte, and time limits. Record any known rejecting gate. Local fixture success does not prove that a full source fits those limits.

Repeat the operator-file block through the `run_tax_cli` definition in this new shell. The tax-only function deliberately excludes FEMA settings. Define a separate combined function:

```sh
: "${UNION_FEMA_CATALOG_PATH:?Require the FEMA catalog file}"
: "${UNION_FEMA_ARCHIVE_PATH:?Require the FEMA archive file}"
run_all_cli() {
  env -i PATH="$PATH" HOME="$HOME" DATABASE_URL="$VERIFY_DATABASE_URL" \
    PARCEL_EVIDENCE_DIR="$PWD/$EVIDENCE/private" \
    UNION_TAX_ARTIFACT_DIR="$PWD/$EVIDENCE/private" \
    UNION_TAX_DELINQUENT_PATH="$UNION_TAX_DELINQUENT_PATH" \
    UNION_TAX_BILLING_PATH="$UNION_TAX_BILLING_PATH" \
    UNION_TAX_FORECLOSURE_PATH="$UNION_TAX_FORECLOSURE_PATH" \
    UNION_TAX_PIPELINE_PATH="$UNION_TAX_PIPELINE_PATH" \
    UNION_FEMA_ARTIFACT_DIR="$PWD/$EVIDENCE/private/fema" \
    UNION_FEMA_CATALOG_PATH="$UNION_FEMA_CATALOG_PATH" \
    UNION_FEMA_ARCHIVE_PATH="$UNION_FEMA_ARCHIVE_PATH" \
    "$EVIDENCE/landman"
}
if run_all_cli >"$EVIDENCE/all.stdout.txt" 2>"$EVIDENCE/all.stderr.txt"; then
  status=0
else
  status=$?
fi
printf '%s\n' "$status" >"$EVIDENCE/all.exit.txt"
docker exec "$VERIFY_CONTAINER" psql -X -U verify -d "$VERIFY_DB_NAME" \
  -v ON_ERROR_STOP=1 -c 'SELECT job_id,started_at,finished_at,outcome FROM job_attempts ORDER BY started_at;' \
  >"$EVIDENCE/all-attempts.txt"
```

Require exit 0, empty stdout, and clean attempts in this order: `union.gis.parcels`, `union.gis.soil`, `union.gis.zoning`, `union.gis.roads`, `union.fema`, `union.gis.structures`, `union.gis.assessed_value`, `union.soil.screening`, and `union.tax`. Source attempts use full-scan kind. Screening is a separate derived attempt. Qualification assessment is not in this combined sequence; run its stored-input command separately when selected.

The combined command bounds each step separately. Most steps have ten minutes; assessed-value capture has three hours. There is no single ten-minute outer deadline for the whole combined command. A failed or quarantined required job stops later jobs. Use `go test ./cmd/landman -count=1 -v` for controlled ordering and deadline tests. Do not damage a live source to force failure. A combined drive without FEMA inputs cannot verify this path.

## Completion and cleanup

Save commands, exits, hashes, counts, and publication evidence under ignored `artifacts/verify/`. Do not commit file paths or raw inputs. Exit the setup shell to remove its database. Preserve retained artifacts for the selected retention period. Run `verify-landman-api` when tax response output can change. Report tax-only and combined drives separately. If files or approval are absent, report the optional live drive as not run. They do not block a default fixture pass.
