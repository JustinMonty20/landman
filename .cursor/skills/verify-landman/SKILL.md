---
name: verify-landman
description: Verify landman by driving the real CLI and local PostGIS the way an operator would — streaming vacant Union County parcel IDs from the live GIS service and applying goose migrations. Use when checking user-facing landman behavior after changes to main.go, the Union GIS client, the vacant-land filter, the parcel-ID importer, Makefile database targets, docker-compose, or db/migrations.
---

# Verify landman

Landman is a Go CLI plus a local PostGIS database. There is no web UI, no TUI, and no HTTP server of our own. The binary has no flags or subcommands. A user builds and runs it; it queries Union County's public ArcGIS layer and prints likely-vacant parcel IDs. Separately, an operator starts PostGIS with Docker Compose and applies goose migrations through the Makefile.

This skill is for the next agent. Drive the real binary and the real database. Do not treat `go test` as a substitute for those paths.

## Launch

### CLI (no long-lived server)

Launch means build the binary once, then start each drive in its own process.

```sh
make build
```

Ready: `make build` exits 0 and `./landman` exists in the repo root. There is no listen port and no prompt. Git ignores that binary (`/landman` in `.gitignore`).

Each drive starts a new process:

```sh
./landman
```

`make run` is the documented operator path and does the same thing after rebuilding. Prefer `./landman` after a single `make build` so a rebuild does not hide a stale-binary problem.

The process talks to the live Union County ArcGIS query endpoint:

`https://atlas.unioncountync.gov/server/rest/services/OperationalLayers/MapServer/215/query`

It sends a count query (`where=1=1`, `returnCountOnly=true`), then fetches every parcel in batches of 500 at about one request per second, including geometry. Each data page also sends `orderByFields=OBJECTID`. The count query does not. It prints only IDs that pass the likely-vacant filter (zero `YEARBLT` or `SQFT`, zero `FMV_IMPRV` currency, or a `LAND_TYPE` containing `RURAL ACREAGE TRACT`). The process deadline is 10 minutes. It does not read `DATABASE_URL` and does not write to Postgres.

Started: stdout contains `Streaming parcel IDs from Union County GIS...`. That line is printed before any HTTP batch finishes.

Usable completion: the process exits 0 and stdout contains `Total likely-vacant parcel IDs streamed: <n>`. A non-zero exit or `Failed to stream parcel IDs:` on stderr is a failed drive. When the 10-minute deadline fires first, that fatal line is followed by `context deadline exceeded` or `rate: Wait(n=1) would exceed context deadline`, and the success total line is absent.

### Local PostGIS

This path needs Docker. Copy `.env.example` to `.env` only when `.env` is missing. That copy is verification scaffolding so Compose names the container `landman-postgis`, which is the Makefile default. Do not commit `.env`.

```sh
test -f .env || cp .env.example .env
docker compose up -d postgis --wait
make db-create
make migrate-up
```

`--wait` waits for the image healthcheck (`pg_isready` against the `postgres` maintenance database). That is not yet a usable landman database. Ready means all three of: the container is healthy, `make db-create` has run (idempotent bootstrap of the `landman` database), and `make migrate-up` has applied every file in `db/migrations/`. Confirm with `make migrate-version`. Stderr contains `goose: version <n>`, where `<n>` is the integer value of the highest filename prefix in `db/migrations/` (prefix `000004` prints as `4`).

The Compose file publishes `5432:5432` and uses volume `landman-postgis-data`. `POSTGRES_PORT` in `.env` does not change the published port. Two instances cannot run side by side with the repo as written.

If a `landman-postgis` container already exists and this run did not start it, do not start a second one, do not `make db-drop`, and do not `make migrate-down`. Those mutate the operator's shared database. `make db-create` and `make migrate-up` are additive and may run against an existing instance after doctor passes.

### Teardown

See Cleanup. Do not tear down until evidence is written.

## Doctor

Run doctor before driving whenever anything looks off.

### CLI doctor

```sh
test -x ./landman
go version
curl -sS -o /tmp/landman-verify-gis-layer.json -w "%{http_code}" \
  "https://atlas.unioncountync.gov/server/rest/services/OperationalLayers/MapServer/215?f=json"
```

Worth driving when: `./landman` is executable, `go version` reports a toolchain that can build the module (see `go.mod`), and the layer metadata request returns HTTP 200 with JSON for MapServer layer 215. If curl fails or is not 200, the stream path is not worth driving from this machine.

### Database doctor

```sh
docker inspect -f '{{.Name}} {{.State.Status}} {{if .State.Health}}{{.State.Health.Status}}{{end}}' landman-postgis
make migrate-version
```

Worth driving when: the container named `landman-postgis` is running and healthy, `.env` sets `DB_CONTAINER=landman-postgis` plus `POSTGRES_URL` and `DATABASE_URL`, and `make migrate-version` can talk to Postgres. If the container is missing, the Launch PostGIS steps have not been completed. If the name is `postgis` instead of `landman-postgis`, `.env` was not loaded when Compose started (Compose defaults `container_name` to `postgis`) and the Makefile will not find it.

## Drive

There is no browser harness and no `control-*` helper. Drive the CLI as a process. Drive the database with `make` and `docker exec … psql`.

Capture stdout, stderr, and the exit code for every command:

```sh
RUN_ID=$(date +%Y%m%dT%H%M%S)-$$
EVIDENCE=artifacts/verify/$RUN_ID
mkdir -p "$EVIDENCE"
./landman >"$EVIDENCE/stream.stdout.txt" 2>"$EVIDENCE/stream.stderr.txt"
echo $? >"$EVIDENCE/stream.exit.txt"
```

Stable handles (do not use coordinates or line numbers as selectors):

- CLI stdout: `Streaming parcel IDs from Union County GIS...`
- CLI stdout: `Batch <n>: <count> parcel IDs` and optional `(sample: [<id> ...])`
- CLI stdout: `Total likely-vacant parcel IDs streamed: <n>`
- CLI stderr: `[union_county_gis] Total parcels to fetch: <n>`
- CLI stderr: `[union_county_gis] Batch <i>/<total> complete:`
- CLI fatal: `Failed to create GIS source:` or `Failed to stream parcel IDs:`
- Makefile targets: `make build`, `make run`, `make db-create`, `make migrate-up`, `make migrate-down`, `make migrate-version`, `make migrate-status`, `make db-drop`
- Database name `landman`, container name `landman-postgis`

Read `features/README.md` and the matching feature file before a drive. Use that file's commands and proof, not an internal test or a package API.

## Evidence

Store proof under `artifacts/verify/$RUN_ID/` in the repo root. Cleanup must not delete this directory. Do not commit it.

Proof standards:

- Exercise the real user path: `./landman` or `make run` for streaming; `docker compose` plus Makefile targets for the database. Do not call `GISSource.FetchBatches`, `ParcelIDImportService.Run`, or `IsLikelyVacantGISRecord` from a scratch program and call that a user-path proof.
- Capture the action and the resulting state: the command, stdout, stderr, and exit code, plus (for the database) a follow-up read such as `make migrate-version` or `\d parcels`.
- The CLI's side effect is printed IDs, not database rows. Do not expect `parcels` to gain rows after `./landman`.
- The GIS HTTP API is a production county system. There is no in-repo mock or dry-run flag. A stream drive hits that endpoint for real. Do not run two stream drives at the same time against it.
- A green `make test` or `go test` is not evidence that the CLI or migrations work.

## Cleanup

- If this run started `./landman` and it is still running, kill that PID only (`kill "$PID"`). Do not `pkill landman` or kill by process name.
- `make clean` may remove the `./landman` binary this run built.
- If this run started PostGIS with `docker compose up`, stop that project with `docker compose stop postgis`. Do not `docker compose down -v` (that deletes `landman-postgis-data`). If the container was already running when the run began, leave it running.
- Never run `make db-drop` or `make migrate-down` unless this run created the database from scratch and no operator data is in it.
- Do not delete `artifacts/verify/`.

## Helpers

This skill ships no helper scripts. Use `make`, `docker`, `curl`, and `psql` via `docker exec` as shown above.
