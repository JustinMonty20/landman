---
name: verify-landman
description: Verify landman by driving ./landman and goose against a dedicated verify PostGIS database. The binary reads DATABASE_URL, upserts Union County parcels, joins soil from MapServer layer 197, and runs soil screening. Use after changes to main.go, the Union GIS client, the vacant-land filter, parcel or soil storage, Makefile database targets, docker-compose, or db/migrations. Never point this skill at the operator database.
---

# Verify landman

Landman is a Go CLI plus PostGIS. There is no web UI, no TUI, and no HTTP server of our own. The binary has no flags or subcommands. `./landman` reads `DATABASE_URL`, fetches Union County parcels and soil, writes those rows, runs soil screening, and prints one summary line.

This skill is for the next agent. Drive the real binary and a verify database this run created. Do not treat `go test` as a substitute. Do not use the operator database.

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
  --health-cmd 'pg_isready -U landman -d postgres' \
  --health-interval 5s \
  --health-timeout 5s \
  --health-retries 12 \
  postgis/postgis:18-3.6-alpine
```

Ready for migrations means `docker inspect` reports the container running and healthy. The healthcheck is `pg_isready` against the `postgres` database. That is not a migrated `landman` database.

Reset, migrate, and seed on every pass. `make db-drop` drops the database named `landman` on `DB_CONTAINER`. The explicit `DB_CONTAINER` is what keeps that drop on the verify container.

```sh
make db-drop DB_CONTAINER="$VERIFY_CONTAINER" POSTGRES_URL="$POSTGRES_URL"
make db-create DB_CONTAINER="$VERIFY_CONTAINER" POSTGRES_URL="$POSTGRES_URL"
make migrate-up DB_CONTAINER="$VERIFY_CONTAINER" DATABASE_URL="$GOOSE_DATABASE_URL"
docker exec -i "$VERIFY_CONTAINER" \
  psql "$GOOSE_DATABASE_URL" -v ON_ERROR_STOP=1 \
  < .cursor/skills/verify-landman/seed.sql
```

The seed inserts one `parcels` row with `source_name` `verify-landman` and `source_parcel_id` `verify-sentinel`. Leave `landman-postgis` alone. Do not run `docker compose` for this skill. Compose publishes port `5432` and mounts the volume `landman-postgis-data`.

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
make migrate-version DB_CONTAINER="$VERIFY_CONTAINER" DATABASE_URL="$GOOSE_DATABASE_URL"
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

Worth driving when `./landman` is executable, `go version` can build the module in `go.mod`, and both layer metadata requests return HTTP 200. Layer 215 is parcels. Layer 197 is soil. Exit 0 also calls the USDA tabular endpoint `https://sdmdataaccess.sc.egov.usda.gov/Tabular/post.rest`. There is no separate metadata probe for that host. A failed USDA call fails the drive.

## Drive

Read `features/README.md` and the matching feature file before a drive. Use that file's commands.

Capture stdout, stderr, and the exit code for every command. `RUN_ID` identifies the pass. Evidence paths are under `artifacts/verify/$RUN_ID/`.

```sh
RUN_ID=$(date +%Y%m%dT%H%M%S)-$$
EVIDENCE=artifacts/verify/$RUN_ID
mkdir -p "$EVIDENCE"
DATABASE_URL="$HOST_DATABASE_URL" ./landman \
  >"$EVIDENCE/import.stdout.txt" \
  2>"$EVIDENCE/import.stderr.txt"
echo $? >"$EVIDENCE/import.exit.txt"
```

The process deadline is 10 minutes. It covers the soil fetch, the parcel import, soil storage (including the optional USDA name lookup), and required soil screening. Parcel pages use `BatchSize` 2000 from `main.go`. The GIS client sends about one request per second. Soil pages are also 2000. Soil fetch logs nothing. The first parcel log line appears after the soil fetch returns. The import summary is printed after parcels commit and before soil storage and screening finish. That line alone is not success.

Stable handles:

- CLI stdout: `Imported parcels, %d likely-vacant feature IDs`
- CLI stderr: `[union_county_gis] Total parcels to fetch: <n>`
- CLI stderr: `[union_county_gis] Batch size: 2000, Total batches: <m>`
- CLI stderr: `[union_county_gis] Batch <i>/<total> complete:`
- CLI stderr: `[union_county_gis] Fetch complete:`
- CLI stderr fatal substrings: `create GIS source:`, `fetch soil polygons:`, `create parcel writer:`, `import parcels:`, `commit parcels:`, `store soil geometry:`, `required soil screening:`, `source import did not complete cleanly:`, `required soil screening did not complete cleanly:`, `invalid SOIL_SCREENING_MAX_AGE`
- Makefile targets, always with the verify `DB_CONTAINER` and URL overrides: `make build`, `make db-drop`, `make db-create`, `make migrate-up`, `make migrate-version`, `make migrate-status`
- SQL sentinel: `source_name = 'verify-landman'` and `source_parcel_id = 'verify-sentinel'`

`log.Fatal` writes a timestamp before the error text and exits 1.

## Evidence

Store proof under `artifacts/verify/$RUN_ID/` in the repo root. Cleanup must not delete this directory. Do not commit it. `artifacts/verify/` is gitignored.

Proof standards:

- Exercise `./landman` with `HOST_DATABASE_URL`, and exercise goose through the Makefile against `GOOSE_DATABASE_URL`. Do not call `GISSource.FetchBatches`, `ParcelIDImportService.Run`, `SaveSales`, or `IsLikelyVacantGISRecord` from a scratch program and call that a user-path proof.
- Capture the command, stdout, stderr, and exit code. For the database, capture a later SQL read.
- A successful import exits 0. Stdout contains the import summary. Stderr contains the batch-size line and `Fetch complete:`. The same `DATABASE_URL` then shows county parcel rows, soil rows, and `job_attempts` outcomes `success` for `union.gis.parcels` and `union.soil.screening`. The sentinel row is still present.
- The printed vacant-ID count is not the number of `likely_vacant` true rows. An empty `PID` is stored with `likely_vacant` false. `job_attempts.processed_count` on the parcel job counts saved features, which can exceed the number of `parcels` rows.
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

This skill ships no helper scripts. The seed file is `.cursor/skills/verify-landman/seed.sql`. Use `make`, `docker`, `curl`, and `psql` via `docker exec` as shown above.
