# Controlled parcel ingestion verification

## Verification scope

The default ingestion-gap tests use local `httptest` ArcGIS responses and disposable PostGIS databases. They make no county or USDA requests. A separately tagged, explicitly approved live capture gate records the official parcel feed once and replays those bytes twice. Neither path establishes a complete moving live snapshot or unattended cloud operation. Cloud runtime, supervision, and monitoring remain undecided. The executable remains a local development path.

The writer stages normalized parts, an indexed rejected-PID set, and a primary-keyed `OBJECTID` identity table before publication. Identity observations include rejected records. A contradictory PID assignment aborts the entire attempt. Go retains one source page or one bounded publication batch, not the whole county. Repair buffers at most its 10,000-feature source budget. PostgreSQL owns the whole-attempt transaction and temporary staging tables.

```mermaid
flowchart LR
    S[Current source pages] --> I[Indexed OID identity check]
    I --> N[Shared normalizer]
    N -->|Invalid individual record| Q[Independent quarantine write]
    Q --> R[Rejected PID set]
    N -->|Valid part| T[Normalized temp stage]
    R --> A[Accepted PID anti-join]
    T --> A
    A --> W[Existing parcel and sales writer]
    W --> U[Union geometry and resolve observed evidence]
    U --> C[One atomic commit]
```

## Repeatable checks

Use only an explicitly selected disposable database, migrated through migration 16. Do not use an operator database or `make db-drop`. The Makefile includes `.env`; use the direct commands below when that file must not be loaded.

```sh
# No database or live services.
env -u DATABASE_URL go test -race ./... -count=1
env -u DATABASE_URL go vet ./...
env -u DATABASE_URL go build ./...

# Explicit disposable PostGIS URL, never the operator database.
DATABASE_URL="$DISPOSABLE_DATABASE_URL" go test -tags postgres -race ./... -count=1 -timeout 15m

# Separate county-sized synthetic gate. Each import has a ten-minute deadline.
DATABASE_URL="$DISPOSABLE_DATABASE_URL" go test -tags 'postgres ingestionfixture' . \
    -run '^TestCountySizedParcelRepeatability$' -count=1 -timeout 35m -v
```

The opt-in gate generates 118,000 features page-by-page in 2,000-feature responses. It deliberately fails late during the first parcel publication, after the writer has exercised foreign-key checks, omitted-sale deletes, and repeated `ANALYZE parcels, parcel_sales`. It requires zero partial parcel or sale rows and surviving quarantine evidence. It then retries successfully and runs the identical full fixture again.

The final comparison covers every parcel and sale value except the expected mutable `updated_at`, including UUIDs, `created_at`, source extras, EWKB, topology, SRID, multipart union, fallback vacancy, last-part field precedence, and deleted sale slots. The fixture has 117,999 effective parcel keys because its last feature is another part of the first PID. These rectangles and synthetic sale slots are not a captured county snapshot.

The smaller public-path fixtures cover both cross-page rejection orders, existing and absent rejected PIDs, valid unrelated publication, quarantine dedupe and recurrence, current multipart recovery, missing or changed source identity, storage failures, failed final shape publication, request-budget rollback, repair selection limits, and full-versus-repair exclusion. Additional runner regressions require a one-record repair to leave 100 of 101 same-PID records pending while publishing all 101 current parts. They also cover multiple reasons per `OBJECTID`, changed `last_seen_at` tokens, numeric values that round to a column maximum or overflow, and contradictory `OBJECTID` identities within and across pages in both valid and invalid arrival orders. Source-only fixtures cover request, feature, and deadline limits, incomplete pages, duplicate parts, and escaped PID predicates. Jobs tests distinguish clean full-import freshness from clean repair success. Executable composition tests preserve source success when derived refresh fails.

## Remaining release gates

The existing ring translator is unchanged. Self-touching source rings that PostGIS accepts remain a separate monitoring concern under the [topology plan](../plans/parcel-geometry-topology.md). No geometry repair or broader topology rejection policy is added.

An unchanged fixture proves repeatability, not snapshot isolation at the county. Offset pagination can miss remote changes despite matching counts, and imports do not delete absent parcels. The approved parcel capture below checks current county geometry and two frozen full imports under the existing deadline. It does not cover USDA, moving-feed snapshot completeness, cloud deployment, supervision, monitoring, or the wider V1 workflow.

## Local fixture result

The offline race suite, `go vet ./...`, `go build ./...`, and the complete Postgres race suite passed. The Makefile test and package build also passed with its `.env` include removed from a temporary copy. No operator environment file was loaded.

The opt-in 118,000-feature gate passed twice on the disposable PostGIS database. Each scan made one count request and 59 page requests. Both clean full scans in each gate finished within the existing per-attempt deadline. The comparison found no parcel or sale differences across 117,999 parcel UUIDs and 353,995 sale UUIDs. The deliberate failed first publication left no partial rows and retained its quarantine evidence. This is controlled synthetic evidence, not a live import or a performance comparison.

The correctness follow-up passed the offline race suite, vet, build, and relevant PostGIS race tests for the runner, jobs, parcel writer, and Union source. Its 118,000-feature gate also passed with the observed-identity check enabled. The one-record repair left 100 of 101 same-PID entries pending while publishing all 101 parts. A separate same-OBJECTID case resolved only one of two reasons, and changed last-seen tokens stayed pending. The unchanged rerun again matched all 117,999 parcel IDs and 353,995 sale IDs. This follow-up did not rerun the unchanged PostGIS soil suite.

Parcel runner fixtures share database-level job exclusion even when their tables use separate schemas. Run the root PostGIS suite and county-sized gate serially against one disposable database.

Migration 16 defaults prior attempts to `full_scan`. Its rollback refuses existing repair history rather than silently making repairs eligible as full-import freshness evidence. The fixture checks that refusal and preserved history.

## Approved live parcel capture

`parcel_capture_postgres_test.go` requires both `postgres` and `ingestioncapture` build tags. Without `UNION_PARCEL_CAPTURE_MODE`, the live test skips. Capture mode also requires `UNION_PARCEL_ALLOW_LIVE=1`. Both capture and replay require `UNION_PARCEL_DISPOSABLE_DB=1` and an explicit `DATABASE_URL` for an empty migrated disposable database.

The recorder sends only the normal `GISSource` count and page queries to its code-defined official layer 215 URL. It follows no redirects and accepts no external endpoint from the environment. Capture runs through `jobs.Runner.Run` as `union.parcels.capture`. Imports use the existing `union.gis.parcels` job. Both source paths retain 2,000-feature pages and the 1 request/sec limiter. Each import has its own unchanged ten-minute deadline.

`UNION_PARCEL_CAPTURE_DIR` must be relative. Its default is `artifacts/verify/ingestion-gaps-live/capture`. Capture refuses an existing directory. Raw count and page responses are written exclusively into a `0700` directory as `0600` files. An ordered manifest records queries, status codes, byte counts, timestamps, and SHA-256 checksums. A checksum-bearing `COMPLETE` file is written only after the full fetch succeeds. Replay verifies the marker and every response, then serves the same checked bytes locally for both imports. Payloads and owner values are not logged. Treat the whole fixture and its database as private data. Checksums detect changes but are not signed attestations.

### Reproduce in a new disposable container

Run from the repository root. Do not source `.env`, reuse an operator database, or run `make db-drop`. These commands create a new container on a random loopback port. The fixture remains local and ignored by Git.

```sh
set -eu
umask 077
root=artifacts/verify/ingestion-gaps-live
mkdir -p "$root"
chmod 700 "$root"
suffix=$(openssl rand -hex 6)
user=fixture_$suffix
db=parcels_live_$suffix
password=$(openssl rand -hex 24)
printf 'POSTGRES_USER=%s\nPOSTGRES_DB=%s\nPOSTGRES_PASSWORD=%s\n' \
    "$user" "$db" "$password" > "$root/container.env"
container=$(docker run -d --env-file "$root/container.env" \
    -p 127.0.0.1::5432 postgis/postgis:18-3.6-alpine \
    -c shared_preload_libraries=pg_stat_statements \
    -c pg_stat_statements.track=all)
until docker exec "$container" pg_isready -h 127.0.0.1 -U "$user" -d "$db"; do
    sleep 1
done
port=$(docker port "$container" 5432/tcp | cut -d: -f2)
export DATABASE_URL="postgres://$user:$password@127.0.0.1:$port/$db?sslmode=disable"
for migration in db/migrations/*.sql; do
    sed -n '/^-- +goose Up$/,/^-- +goose Down$/p' "$migration" | sed '$d' |
        docker exec -i "$container" psql -X -h 127.0.0.1 \
            -v ON_ERROR_STOP=1 -U "$user" -d "$db" || exit 1
done
docker exec "$container" psql -X -h 127.0.0.1 -v ON_ERROR_STOP=1 \
    -U "$user" -d "$db" -c 'CREATE EXTENSION pg_stat_statements;' \
    -c 'SHOW pg_stat_statements.track; SELECT pg_stat_statements_reset();'

UNION_PARCEL_DISPOSABLE_DB=1 UNION_PARCEL_CAPTURE_MODE=capture \
UNION_PARCEL_ALLOW_LIVE=1 UNION_PARCEL_CAPTURE_DIR="$root/capture" \
    go test -tags 'postgres ingestioncapture' . \
    -run '^TestLiveParcelCaptureRepeatability$' -count=1 -timeout 35m -v \
    > "$root/live-capture-replay.log" 2>&1
```

The migration loop applies all 16 SQL Up sections. It does not create Goose tracking history. Wait for TCP readiness as shown, not socket readiness during image bootstrap. The test checks empty parcel, sale, attempt, quarantine, and soil-polygon tables before any source request. Run only against a new task database.

For another offline verification, create another empty migrated disposable database and point `DATABASE_URL` at it. Retain the completed capture path and run the following command. Each verification imports that capture twice into the same database.

```sh
env -u UNION_PARCEL_ALLOW_LIVE UNION_PARCEL_DISPOSABLE_DB=1 \
    UNION_PARCEL_CAPTURE_MODE=replay \
    UNION_PARCEL_CAPTURE_DIR=artifacts/verify/ingestion-gaps-live/capture \
    go test -tags 'postgres ingestioncapture' . \
    -run '^TestLiveParcelCaptureRepeatability$' -count=1 -timeout 25m -v

# Local checksum, incomplete-capture, and overwrite checks. No DB or live feed.
env -u DATABASE_URL -u UNION_PARCEL_CAPTURE_MODE \
    go test -tags 'postgres ingestioncapture' -race . \
    -run '^TestParcelCaptureIntegrity$' -count=1 -v
```

### Measured result

The predeclared claim was clean completion under each ten-minute import deadline and exact equality after an identical full rerun. This was not a speedup experiment. Pre-run load was 0.83, 1.41, and 1.00 on 20 CPUs. Single-run durations have no comparison or variability claim.

The approved run used a new `postgis/postgis:18-3.6-alpine` container, PostgreSQL 18.6, PostGIS 3.6.4, and all 16 migrations. `pg_stat_statements` 1.12 was enabled with `track=all`. An initial setup query was interrupted by the image's normal bootstrap shutdown. Final TCP startup, empty tables, migration 16, and the recreated statistics extension were verified before capture. This was a setup error, not an ingestion failure.

| Check | Completed work | Duration | Errors |
| --- | --- | --- | --- |
| One live capture | 61 requests, 118,202 features, 318,486,463 bytes | 60.303s | 0 |
| Frozen import 1, empty database | 61 local responses, 118,202 processed parts | 153.220s | 0 |
| Frozen import 2, populated database | Same 61 local responses and 118,202 parts | 170.629s | 0 |

The complete test passed in 397.835s. Each import stored 117,983 parcels and 238,016 sales. The capture contains exactly 117,983 distinct parcel keys, 118,202 distinct `OBJECTID` values, and 1,961 fallback-ID features. All 61 upstream responses succeeded. Both import attempts are clean full scans with no quarantine. There were no application-level live retries or second county scans.

The final SQL full joins found zero parcel or sale differences. They include UUIDs, `created_at`, every mapped value, source extras, EWKB, `ST_Equals`, and SRID. Only the expected mutable `updated_at` is excluded. There are zero duplicate natural keys, null shapes, or incorrect SRIDs. The geometry contains 1,418,132 vertices, with at most 1,762 vertices and 28 geometry parts per parcel. This is real captured county geometry, not synthetic rectangles.

Across the two imports, query statistics recorded 236,404 omitted-sale-delete calls, zero deleted rows, and 238,744 nested parcel foreign-key key-share checks. The zero delete count is a property of this unchanged capture. Synthetic coverage still checks deletion of omitted slots and failed-first-import retry. These query counters prove those paths ran; they do not establish a performance improvement. `ANALYZE parcels, parcel_sales` accounted for 118 calls and 91.624s aggregate database execution time in this instrumented run.

To retain the relevant query measurements before other tests change the counters, run this query inside the disposable database.

```sql
SELECT calls, rows, total_exec_time, mean_exec_time,
       shared_blks_hit, shared_blks_read, query
FROM pg_stat_statements
WHERE query LIKE '%FOR KEY SHARE%'
   OR query LIKE 'DELETE FROM parcel_sales%'
ORDER BY total_exec_time DESC;
```

Final checks passed `go test ./...`, `go test -race ./...`, `go vet ./...`, and `go build ./...` with `DATABASE_URL` and live opt-ins unset. The tagged capture integrity race check, serial root parcel PostGIS race tests, and jobs and parcel-writer PostGIS race tests passed. No soil source import or USDA request ran.

The existing synthetic failed-first-import gate also passed again in 199.186s. Its three scans completed 60 requests each. The deliberate first publication failed with one quarantine and zero published rows after 53.836s. Retry and identical rerun succeeded with zero quarantine after 61.476s and 75.686s. They preserved 117,999 parcel IDs and 353,995 sale IDs. These remain synthetic deadline and retry checks, not live failure-recovery or performance-comparison evidence.

Private local evidence is under `artifacts/verify/ingestion-gaps-live`. `live-capture-replay.log` records work counts and equality. `capture-validation.log` records checksum and source-cardinality verification. `live-measurements.log` and `live-pg-stat-statements.csv` retain geometry and query measurements. `capture/manifest.json` and `capture/COMPLETE` identify the frozen fixture. `default.log`, `race.log`, `vet.log`, `build.log`, `capture-integrity.log`, `postgres-parcels.log`, `postgres-writer-jobs.log`, and `synthetic.log` retain final test output. No raw payload is committed.

### Limits carried forward

The live capture is one sequence of offset queries, not a transactional remote snapshot. Matching counts cannot rule out concurrent county changes. Import durations include local rate-limited replay and real geometry publication, not the live network calls performed during capture. They cannot establish end-to-end moving-feed performance or a speed difference between empty and populated databases.

`ST_IsValid` reports 28 stored geometries as invalid. The existing translator and PostGIS publication accept them without quarantine, and both imports preserve the same EWKB and `ST_Equals` results. No geometry was repaired, omitted, or reclassified to pass this gate. Broader topology handling remains under the existing [topology plan](../plans/parcel-geometry-topology.md), with the observation recorded in the [live verification diary](../.agents/diary/ingestion-gaps-live.md).

The raw quarantine payload cap remains 64 KiB in Go and PostgreSQL. This capture produced no quarantines, so it did not test an oversized invalid geometry payload. Failed-first-import rollback and retry remain synthetic evidence, not an injected failure of this live replay. This run does not close absent-parcel deletion, USDA, deployment, supervision, monitoring, tax, UI, or the wider V1 release gates.
