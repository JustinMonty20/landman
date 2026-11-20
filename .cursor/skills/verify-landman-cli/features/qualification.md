# Qualification evidence

Qualification jobs publish zoning and road generations and derive stored parcel assessments. Scheduler waiting alone does not verify these jobs. Use local fixtures for failure cases and spatial checks. Keep full-county live checks separate.

## Sub-features

- `qualification-publication` checks complete-group publication and prior-head retention after failure.
- `qualification-spatial` checks stored topology, gaps, conflicts, holes, and projected measurement limits.
- `qualification-refresh` checks source and parcel changes, stale evidence, independent group failures, and attempt ordering.
- `qualification-locks` checks county-host exclusion, cooldown, cancellation, and lost sessions.

## How to get to it (user POV)

- `./landman run union.gis.zoning` fetches the zoning source group and reconciles stored qualification evidence.
- `./landman run union.gis.roads` fetches roads and reconciles stored qualification evidence.
- `./landman run union.qualification.assess` reconciles stored data without a county fetch.
- The scheduler uses separate zoning, road, and qualification calendars. It reconciles stored qualification evidence once at startup before calendar dispatch.

## Driving it with make and the landman binary

Preconditions:

- Use a fresh isolated database from `verify-landman/database.md` with `VERIFY_DB_NAME=landman_tax_integration`. Do not load the GIS seed.
- Run packages serially; fixtures change shared qualification heads and remove fixture rows.

- Run local source/parser tests:

```sh
go test -race ./internal/connector/union -count=1
```

- Run the full qualification database suite and CLI composition checks:

```sh
DATABASE_URL="$VERIFY_DATABASE_URL" go test -race -tags postgres \
  ./internal/qualification -count=1 -v
DATABASE_URL="$VERIFY_DATABASE_URL" go test -race -tags postgres \
  ./cmd/landman -count=1 -v
```

Require the captured full stored replay, group publication/retirement, failed repair, stored-topology, long-edge distance, failure-order, and host-lock tests to execute. Save verbose output. Read `docs/union-qualification-verification.md` for the approximation limits; do not claim exact curved-edge measurements.

- Check the real no-source path in a new setup database, not the fixture database:

```sh
mkdir -p artifacts/verify
go build -o artifacts/verify/qualification-cli ./cmd/landman
DATABASE_URL="$VERIFY_DATABASE_URL" artifacts/verify/qualification-cli run union.qualification.assess
docker exec "$VERIFY_CONTAINER" psql -X -U verify -d "$VERIFY_DB_NAME" \
  -v ON_ERROR_STOP=1 -c "SELECT job_id,outcome FROM job_attempts ORDER BY started_at;" \
  -c 'SELECT count(*) FROM qualification_heads; SELECT count(*) FROM qualification_assessments;'
```

Require a successful assessment attempt and zero heads and assessments on this empty database. The command does not read a parcel when no zoning or roads head exists. The unavailable-source result is a parcel read with no head, so this empty command does not prove that result.

- Only with explicit live approval, use another fresh setup database populated with verified parcel inputs. Build the CLI, run zoning once, roads once, and assessment once. Capture each exit and query:

```sql
SELECT source_group,id,completed_at FROM qualification_generations ORDER BY started_at;
SELECT source_group,generation_id FROM qualification_heads ORDER BY source_group;
SELECT generation_id,layer_id,feature_count,source_srid FROM qualification_layer_receipts ORDER BY generation_id,layer_id;
SELECT source_group,failed_at FROM qualification_refresh_failures ORDER BY source_group;
SELECT source_group,count(*) FROM qualification_assessments GROUP BY source_group;
```

Compare receipts with the client's scoped layer manifest. Require complete groups and clean attempts. Confirm both groups produce assessments for the stored parcels. Inspect evidence through the existing qualification read interface in database tests. Do not infer complete coverage or legal access from row counts. Failed-refresh retention and failure-order proof come from local injected-failure tests, not deliberate damage to a live feed.

## Gotchas

- `./landman run union.qualification.assess` waits up to 60 minutes, including the assessment lock. Scheduled and automatic assessments keep the 10-minute limit.
- Live county-scale runtime and memory remain separate release gates. Fixture success cannot close them.
- Road and zoning source data can be unavailable independently. Do not turn absent evidence into a favorable result.
- The CLI GIS classifier checks soil and parcel publication, not qualification topology or freshness.
- Exit the setup shell to remove only its container. Keep evidence under ignored `artifacts/verify/`.
