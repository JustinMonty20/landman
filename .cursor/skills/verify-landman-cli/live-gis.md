# Optional live GIS verification

Run this journey only after explicit approval for live county GIS and USDA requests. The default parcel checks are in `SKILL.md`. A failure here does not replace or invalidate the separate local fixture results.

Landman has a Go CLI, PostGIS, and a separate API executable. This skill verifies CLI parcel, soil, scheduler, and qualification behavior. It does not verify manual tax ingestion, API/authentication, or browser UI behavior. Use `verify-landman` to select all required skills.

With no arguments, `landman` requires `UNION_TAX_ARTIFACT_DIR`, four tax-file paths, `UNION_FEMA_ARTIFACT_DIR`, `UNION_FEMA_CATALOG_PATH`, and `UNION_FEMA_ARCHIVE_PATH`. It then runs parcels, soil, zoning, roads, FEMA, structures, assessed value, screening, qualification, and the four tax files. Each of those steps has its own ten-minute limit. The tax skill owns that combined drive and its operator files. This CLI skill does not use that command. It drives `./landman run union.gis.soil` and then `./landman run union.gis.parcels`. A clean soil run screens the stored soil set and reconciles stored qualification data. A clean parcel run reconciles qualification, and it screens only when stored screening is due. Stdout stays empty. Stderr lines start with a timestamp. Parcel progress is `Parcel capture`, `Parcel capture page`, and `Parcel capture complete`, then publication lines. Soil, screening, ingest, and qualification print their own lines. `success_with_quarantine` stops downstream work. `./landman schedule` requires cron values for parcels, soil, zoning, roads, and qualification. It reconciles stored qualification once, then waits for the next due time. `./landman reprocess union.gis.parcels --limit N` repairs pending parcel records. `N` is an integer from 1 through 100, and the check occurs before the database opens. A clean repair screens stored soil when that screen is due and does not fetch county soil. An unknown command, including `import`, prints usage and exits 1 before it opens the database.

Drive the real binary and verify database changes. Do not treat `go test` as a substitute. Do not use the operator database.

## Verification sync
The 2026-11-14 seed still loads after goose version 41. On 2026-11-20 at 04:35:48Z, `./landman run union.gis.soil` succeeded and logged soil capture plus screening on stderr. `./landman run union.gis.parcels` then logged `Parcel capture page` lines, committed `full_scan` `success` with processed 118204, and soil screening recorded `interrupted` at the command's 10-minute deadline. Probe pages were 437 ms and 1069 ms. `classify` printed `fail`, with `seededSales` met and `fullImport` missed. An empty `reprocess --limit 1` then finished screening in 4m17s. Evidence is `artifacts/verify/20261120T043546Z-50629`.

## Launch

Launch creates the verify database, migrates it, loads the seed, and builds the binary. Each `./landman` drive is a new process against that database.

### Names

- Container `landman-verify-postgis`. This name is reserved for verification.
- Host port `127.0.0.1:55432`. Goose does not use this port.
- Image `postgis/postgis:18-3.6-alpine`, the same tag as `docker-compose.yml`.
- Volume `landman-verify-data`.
- Database name inside the container is `landman`, because `db/bootstrap/create_database.sql` always creates that name.

`POSTGRES_URL` and the goose `DATABASE_URL` use `localhost:5432`. Goose joins the verify container network, so that host is Postgres inside the container. `./landman` runs on the host, so its `DATABASE_URL` uses `127.0.0.1:55432`. Do not export a single `DATABASE_URL` for both.

Do not copy or source `.env`. Make loads `.env` when the file exists, and that file is the operator database. Pass `DB_CONTAINER`, `POSTGRES_URL`, and `DATABASE_URL` on the `make` command line. Command-line assignments override `.env`.

```sh
export VERIFY_CONTAINER=landman-verify-postgis
export VERIFY_VOLUME=landman-verify-data
export POSTGRES_URL='postgres://landman:landman@localhost:5432/postgres?sslmode=disable'
export GOOSE_DATABASE_URL='postgres://landman:landman@localhost:5432/landman?sslmode=disable'
export HOST_DATABASE_URL='postgres://landman:landman@127.0.0.1:55432/landman?sslmode=disable'
```

### Verify database

If `landman-verify-postgis` already exists, inspect it before removing it. Remove it only when the image is `postgis/postgis:18-3.6-alpine` and the published binding is `127.0.0.1:55432`. Any other existing container with that name is a stop.

```sh
if docker inspect "$VERIFY_CONTAINER" >/dev/null 2>&1; then
  image=$(docker inspect -f '{{.Config.Image}}' "$VERIFY_CONTAINER")
  binding=$(docker inspect -f '{{json .HostConfig.PortBindings}}' "$VERIFY_CONTAINER")
  case "$image" in
    postgis/postgis:18-3.6-alpine) ;;
    *) echo "refusing to remove $VERIFY_CONTAINER (image $image)"; exit 1 ;;
  esac
  case "$binding" in
    *'"HostIp":"127.0.0.1"'*'"HostPort":"55432"'*) ;;
    *) echo "refusing to remove $VERIFY_CONTAINER (binding $binding)"; exit 1 ;;
  esac
  docker rm -f "$VERIFY_CONTAINER"
fi
docker volume rm "$VERIFY_VOLUME" >/dev/null 2>&1 || true
docker run -d \
  --name "$VERIFY_CONTAINER" \
  -e POSTGRES_USER=landman \
  -e POSTGRES_PASSWORD=landman \
  -e POSTGRES_DB=postgres \
  -p 127.0.0.1:55432:5432 \
  -v "$VERIFY_VOLUME":/var/lib/postgresql \
  --health-cmd 'pg_isready -h 127.0.0.1 -U landman -d postgres' \
  --health-interval 5s \
  --health-timeout 5s \
  --health-retries 12 \
  postgis/postgis:18-3.6-alpine
```

Ready for migrations means `docker inspect` reports the container running and healthy. The healthcheck uses `pg_isready -h 127.0.0.1` against the `postgres` database. TCP readiness excludes the temporary bootstrap server. That is not a migrated `landman` database.

Reset, migrate, and seed on every pass. `make db-drop` drops the database named `landman` on `DB_CONTAINER`. The explicit `DB_CONTAINER` is what keeps that drop on the verify container.

```sh
make db-drop DB_CONTAINER="$VERIFY_CONTAINER" POSTGRES_URL="$POSTGRES_URL"
make db-create DB_CONTAINER="$VERIFY_CONTAINER" POSTGRES_URL="$POSTGRES_URL"
make migrate-up DB_CONTAINER="$VERIFY_CONTAINER" DATABASE_URL="$GOOSE_DATABASE_URL" GOOSE_VERSION=v3.30.0
go run ./.cursor/skills/verify-landman-cli/cmd/verify-landman check-schema \
  --container "$VERIFY_CONTAINER" --goose-url "$GOOSE_DATABASE_URL" \
  --seed .cursor/skills/verify-landman-cli/seed.sql
docker exec -i "$VERIFY_CONTAINER" \
  psql "$GOOSE_DATABASE_URL" -v ON_ERROR_STOP=1 \
  < .cursor/skills/verify-landman-cli/seed.sql
go run ./.cursor/skills/verify-landman-cli/cmd/verify-landman check-loaded \
  --container "$VERIFY_CONTAINER" --goose-url "$GOOSE_DATABASE_URL" \
  --seed .cursor/skills/verify-landman-cli/seed.sql
```

Run `check-schema` after `migrate-up` and before the seed load. Stdout is `schema: ok`, or one `schema: <table>.<column> missing` line for each inserted column the database does not have, or one `schema: <table>.<column> not null` line when a `NOT NULL` column has no default and the seed does not insert it. Exit 0 lets you load the seed. Exit 1 means you stop and do not load the seed.

The seed is a captured Union County sample, not a synthetic row. The parcel ids are the ones whose centroids fall in EPSG:3857 500 m cells `(-17961, 8354)` and `(-17960, 8354)`. `capture-seed` refreshes those parcels, their kept sale slots, and the soil that intersects them. The header of `seed.sql` records the capture time and the two query URLs. Sale rows are the kept slots for those parcel ids. `SavePart` upserts each parcel and its kept slots on the import transaction and deletes omitted sequences. `SaveSales` alone does not delete.

Run `check-loaded` after the seed load. It prints the census parsed from `seed.sql`, then `loaded: ok` when the live counts and the two sample parcels match that file. Exit 1 names the table or the `source_parcel_id`.

Leave `landman-postgis` alone. Do not run `docker compose` for this skill. Compose publishes port `5432` and mounts the volume `landman-postgis-data`.

### CLI

```sh
make build
```

Ready means `make build` exits 0 and `./landman` exists in the repo root. Git ignores that binary (`/landman` in `.gitignore`). There is no listen port and no prompt.

`make run` rebuilds and then executes with Make's environment, including `.env` when that file exists. Do not use `make run` for a verify drive.

## Doctor

Run doctor before the first drive, after any failed drive, and again after a fresh container. A doctor failure caused by a wrong URL, container name, or expected line in this skill is drift in this skill. Fix the skill and retry the step that failed.

### Database doctor

```sh
docker inspect -f '{{.Name}} {{.Config.Image}} {{.State.Status}} {{if .State.Health}}{{.State.Health.Status}}{{end}}' "$VERIFY_CONTAINER"
docker inspect -f '{{json .HostConfig.PortBindings}}' "$VERIFY_CONTAINER"
make migrate-version DB_CONTAINER="$VERIFY_CONTAINER" DATABASE_URL="$GOOSE_DATABASE_URL" GOOSE_VERSION=v3.30.0
```

Worth driving when all of these are true.

- The name is `/landman-verify-postgis`.
- The image is `postgis/postgis:18-3.6-alpine`.
- The container is running and healthy.
- The host binding is port `55432` on `127.0.0.1`.
- `make migrate-version` exits 0. Stderr contains `goose: version ` followed by the integer of the highest filename prefix in `db/migrations/`. A log timestamp precedes that line. Stdout is the Make recipe.

If a container named `landman-postgis` exists, leave it running. Do not inspect it as a substitute for the verify container.

### CLI doctor

```sh
test -x ./landman
go version
curl -sS -o /tmp/landman-verify-layer-215.json -w "%{http_code}" \
  "https://atlas.unioncountync.gov/server/rest/services/OperationalLayers/MapServer/215?f=json"
curl -sS -o /tmp/landman-verify-layer-197.json -w "%{http_code}" \
  "https://atlas.unioncountync.gov/server/rest/services/OperationalLayers/MapServer/197?f=json"
```

Worth driving when `./landman` is executable, `go version` can build the module in `go.mod`, and both layer metadata requests return HTTP 200. Layer 215 is parcels. Layer 197 is soil. Those curls check that the hosts answer. Page timings come from `probe`. The doctor and `probe` do not call USDA. `./landman run union.gis.soil` calls `https://sdmdataaccess.sc.egov.usda.gov/Tabular/post.rest` while screening. Stderr then contains `Fetch USDA names` and `Fetch USDA interpretations`. An interpretation fetch error fails screening. A map-unit name error during soil store does not fail that store. There is no separate metadata probe for that host.

## Drive

Read `features/README.md` and the matching feature file before a drive. Use that file's commands.

Capture stdout, stderr, and the exit code for every command. `RUN_ID` identifies the pass. Evidence paths are under `artifacts/verify/$RUN_ID/`.

```sh
RUN_ID=$(date +%Y%m%dT%H%M%S)-$$
EVIDENCE=artifacts/verify/$RUN_ID
mkdir -p "$EVIDENCE"
go run ./.cursor/skills/verify-landman-cli/cmd/verify-landman probe \
  --evidence "$EVIDENCE"
if HOST_DATABASE_URL="$HOST_DATABASE_URL" EVIDENCE="$EVIDENCE" \
  bash .cursor/skills/verify-landman-cli/run-gis.sh; then
  drive_status=0
else
  drive_status=$?
fi
# The runner writes each source exit and the combined exit before it returns.
go run ./.cursor/skills/verify-landman-cli/cmd/verify-landman classify \
  --container "$VERIFY_CONTAINER" --goose-url "$GOOSE_DATABASE_URL" \
  --seed .cursor/skills/verify-landman-cli/seed.sql --evidence "$EVIDENCE"
```

Run `probe` before the GIS commands. It times each layer's `?f=json` metadata request, then one 2000-row data page, and writes `page-timings.json`. Parcel pages use `orderByFields=OBJECTID`. Soil pages use `orderByFields=OBJECTID_1`. Exit 0 when both metadata calls returned HTTP 200. A slow data page still exits 0. Classify treats metadata as fast when the call is HTTP 200 within 5 seconds. A data page is slow at 15 seconds or a timeout. The parcel command writes `Parcel capture page` lines on stderr. It does not write `Batch` or `Fetch complete` lines. Quiet stderr is not evidence that the county is slow. Only the timed HTTP probe supplies that evidence.

Run `classify` after both GIS commands. Stdout is one verdict line. The command also writes `$EVIDENCE/drive-outcome.json`. The fields are `verdict`, `seededSales`, `fullImport`, and `sales`. `sales` is null when the parcel transaction did not commit. The parcel transaction committed when `job_attempts` has a `full_scan` row for `union.gis.parcels` whose outcome is `success` or `success_with_quarantine`. The helper does not look for an import summary line. It counts `soil_parcel_assessments` joined to the current Union `soil_screening_heads` row, and that count must equal the county parcel count. The helper exits 2 for `inconclusive: county-source-slow`. `go run` turns that into shell exit 1 and prints `exit status 2` on stderr. Read the verdict line and `drive-outcome.json`.

Each GIS command has a 10-minute deadline shared by the source attempt and its derived screening and qualification. Parcel pages are 2000. The GIS client sends about one request per second. Soil pages are also 2000. `Parcel capture complete` is not command success. The process still publishes parcels and may screen and assess them. A source `Finished: outcome=success` line can be followed by a derived failure and exit 1. This journey does not run `replay`, `union.fema`, `union.gis.structures`, or `union.gis.assessed_value`.

Stable handles:

- CLI stdout for each GIS command is empty
- CLI stderr timestamp prefix on every log line, including `log.Fatal`
- CLI stderr parcel capture: `[union_county_gis] Parcel capture: <n> records across <m> pages`
- CLI stderr parcel capture: `[union_county_gis] Parcel capture page <i>/<m>: <got>/<n> records`
- CLI stderr parcel capture: `[union_county_gis] Parcel capture complete: <n> records`
- CLI stderr publication: `[union.gis.parcels] capture=<id>: Checked <n> parcel groups in publication transaction` and `Parcel publication complete: processed=<n> quarantined=<n>`
- CLI stderr job boundary: `[<job ID>] Started: kind=<kind>`, `Running: attempt=<id>`, `Finished: outcome=<outcome> processed=<n> quarantined=<n> elapsed=<duration> attempt=<id>`
- CLI stderr soil: `[union.gis.soil] Soil Polygons (layer 197):`
- CLI stderr screening: `[union.soil.screening]` USDA name and interpretation lines
- CLI stderr qualification: `[union.qualification.assess] Wait for assessment lock`
- Do not require `Total parcels to fetch`, `Batch size`, `Batch <i>/<total> complete`, or `Fetch complete:`. Those lines are not on this command.
- CLI stderr for an unknown command is a timestamp, then `usage: landman [schedule | run <job ID> | replay <union.gis.parcels|union.tax|union.gis.soil|union.soil.screening|union.fema|union.gis.structures|union.gis.assessed_value> --snapshot UUID | reprocess union.gis.parcels --limit N]`
- CLI stderr when `--limit` is not an integer from 1 through 100: `reprocess limit must be between 1 and 100`
- CLI stderr fatal substrings: `create GIS source:`, `capture parcels:`, `capture county geometry for Soil Polygons (layer 197):`, `capture=<id>:`, `union repair:`, `required union.gis.parcels:`, `required union.gis.soil:`, `required union.soil.screening:`, `required union.gis.parcels did not complete cleanly:`, `required union.gis.soil did not complete cleanly:`, `required union.soil.screening did not complete cleanly:`, `success_with_quarantine`, `invalid SOIL_SCREENING_MAX_AGE`, `connect for derived refresh exclusion:`, `wait for derived refresh exclusion:`
- Makefile targets, always with the verify `DB_CONTAINER` and URL overrides: `make build`, `make db-drop`, `make db-create`, `make migrate-up`, `make migrate-version`, `make migrate-status`
- SQL seed identities: `source_parcel_id` `07102010` and `07102051`, both `source_name` `union_county_gis`. Vacancy and situs are the `census parcel` lines from `check-loaded`.
- `check-schema` stdout `schema: ok`
- `check-loaded` stdout begins `census parcels=` and ends with `loaded: ok`
- `classify` stdout `pass`, `fail`, or `inconclusive: county-source-slow`

`log.Fatal` writes a timestamp before the error text and exits 1.

## Evidence

Store proof under `artifacts/verify/$RUN_ID/` in the repo root. Cleanup must not delete this directory. Do not commit it. `artifacts/verify/` is gitignored.

Proof standards:

- Exercise `./landman` with `HOST_DATABASE_URL`, and exercise goose through the Makefile against `GOOSE_DATABASE_URL`. Do not call the GIS fetch, the import service, or the vacant filter from a scratch program and call that a user-path proof.
- Capture the command, stdout, stderr, and exit code. For the database, capture `drive-outcome.json` from `classify`.
- A successful import exits 0. Stdout is empty. Parcel stderr contains `Parcel capture complete:` and `Finished: outcome=success` for `union.gis.parcels`. Soil and screening stderr contain their own `Finished: outcome=success` lines. `classify` then prints `pass`. `drive-outcome.json` records `seededSales` and `fullImport` as `met`. The parcel attempt is `attempt_kind` `full_scan` and outcome `success`. `SavePart` upserts each parcel and its kept slots on the import transaction and deletes omitted sequences. `SaveSales` alone does not delete. Do not query a county-wide `parcel_sales` total. A fast probe page can still miss the ten-minute budget during parcel publication or derived screening. That result is `fail`, with `seededSales` `met` when the seed slots match, not `inconclusive: county-source-slow`.
- An empty `PID` is stored with `likely_vacant` false. `job_attempts.processed_count` on the parcel job counts saved features, which can exceed the number of `parcels` rows. A quarantined record is not one of those saved features. Outcome `success_with_quarantine` exits non-zero and does not start that source command's derived work.
- The GIS and USDA endpoints are production systems. There is no dry-run flag. Do not run two `./landman` drives at the same time. A second run records `skipped_overlap` and exits non-zero.
- A green `make test` or `go test` is not evidence that the CLI or migrations work.

## Cleanup

Run cleanup after the last drive of the pass, including any re-drive. Check that `artifacts/verify/$RUN_ID/` still exists after cleanup.

- If this run started `./landman` and it is still running, kill that PID only (`kill "$PID"`). Do not `pkill landman` or kill by process name.
- `make clean` may remove the `./landman` binary this run built.
- Remove only the verify container and its volume.

```sh
docker rm -f "$VERIFY_CONTAINER"
docker volume rm "$VERIFY_VOLUME"
```

- Do not `docker compose down`, do not stop `landman-postgis`, and do not `make db-drop` without `DB_CONTAINER=landman-verify-postgis`.
- Do not delete `artifacts/verify/`.

## Helpers

The helper is one program.

```sh
go run ./.cursor/skills/verify-landman-cli/cmd/verify-landman
```

`check-schema`, `check-loaded`, `probe`, and `classify` are the steps in the launch and the drive above. The seed file is `.cursor/skills/verify-landman-cli/seed.sql`.

`capture-seed` refreshes that file from the county. It writes `seed.sql.capture-tmp` beside the seed and renames the temp file over `seed.sql` only after the temp file parses back to the same parcel ids. A request that hits 30 seconds, or a missed 3 minute budget, prints `capture: left seed.sql in place (county slow)` and the helper exits 2. The current seed stays. `go run` turns that exit into shell exit 1 and prints `exit status 2` on stderr. Do not run `capture-seed` on a normal drive.

```sh
go run ./.cursor/skills/verify-landman-cli/cmd/verify-landman capture-seed \
  --container "$VERIFY_CONTAINER" --postgres-url "$POSTGRES_URL" \
  --seed .cursor/skills/verify-landman-cli/seed.sql
```
