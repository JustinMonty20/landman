# Optional live GIS verification

Run the source journey only after explicit approval for county GIS and USDA requests. USDA is the United States Department of Agriculture. Default local fixture checks are in `SKILL.md`. Live failure does not invalidate a separate fixture result.

This journey runs soil, then parcels. It does not run the combined bare command, tax, FEMA, or vacancy jobs. Soil success starts stored screening. Parcel success starts screening when stored screening is due. Qualification assessment is a separate stored-input job. Do not infer its success from a GIS command.

## Launch

Run in Bash at the repository root. Use one database for this journey and a fresh CLI process for each drive. Do not source `.env`, use Compose, or use the operator database.

Define `verify_make` from `SKILL.md`. It runs repository Make targets without operator settings. Use these dedicated names. Stop if the container, volume, or host port is already in use; do not remove an earlier run's data.

```sh
export VERIFY_CONTAINER=landman-verify-postgis
export VERIFY_VOLUME=landman-verify-data
export POSTGRES_URL='postgres://landman:landman@localhost:5432/postgres?sslmode=disable'
export GOOSE_DATABASE_URL='postgres://landman:landman@localhost:5432/landman?sslmode=disable'
export HOST_DATABASE_URL='postgres://landman:landman@127.0.0.1:55432/landman?sslmode=disable'
RUN_ID=$(date +%Y%m%dT%H%M%S)-$$
EVIDENCE=artifacts/verify/$RUN_ID
mkdir -p "$EVIDENCE"
if docker container inspect "$VERIFY_CONTAINER" >/dev/null 2>&1 ||
   docker volume inspect "$VERIFY_VOLUME" >/dev/null 2>&1 ||
   [ -n "$(ss -H -ltn 'sport = :55432')" ]; then
  echo 'Verification container, volume, or port is already in use' >&2
  exit 1
fi
docker run -d --name "$VERIFY_CONTAINER" \
  -e POSTGRES_USER=landman -e POSTGRES_PASSWORD=landman -e POSTGRES_DB=postgres \
  -p 127.0.0.1:55432:5432 -v "$VERIFY_VOLUME":/var/lib/postgresql \
  --health-cmd 'pg_isready -h 127.0.0.1 -U landman -d postgres' \
  --health-interval 5s --health-timeout 5s --health-retries 12 \
  postgis/postgis:18-3.6-alpine
```

Record the returned container ID. Clean this container and volume on failure as well as success. Keep evidence. Wait for Docker to report running and healthy before migrations. TCP readiness against `postgres` is not proof of a migrated `landman` database.

Follow [bootstrap](features/local-database-bootstrap.md) and [migration status](features/migration-status.md). Capture each command's stdout, stderr, and exit before another command can replace the status. Stop on any failed setup command:

```sh
verify_make db-drop DB_CONTAINER="$VERIFY_CONTAINER" POSTGRES_URL="$POSTGRES_URL"
verify_make db-create DB_CONTAINER="$VERIFY_CONTAINER" POSTGRES_URL="$POSTGRES_URL"
verify_make migrate-up DB_CONTAINER="$VERIFY_CONTAINER" DATABASE_URL="$GOOSE_DATABASE_URL" GOOSE_VERSION=v3.30.0
go run ./.cursor/skills/verify-landman-cli/cmd/verify-landman check-schema \
  --container "$VERIFY_CONTAINER" --goose-url "$GOOSE_DATABASE_URL" \
  --seed .cursor/skills/verify-landman-cli/seed.sql
docker exec -i "$VERIFY_CONTAINER" psql "$GOOSE_DATABASE_URL" \
  -v ON_ERROR_STOP=1 < .cursor/skills/verify-landman-cli/seed.sql
go run ./.cursor/skills/verify-landman-cli/cmd/verify-landman check-loaded \
  --container "$VERIFY_CONTAINER" --goose-url "$GOOSE_DATABASE_URL" \
  --seed .cursor/skills/verify-landman-cli/seed.sql
verify_make build
```

Require `schema: ok` before seed load, then the census and `loaded: ok`. The seed is a retained county sample, not a full import. `check-loaded` checks selected table counts and two parcel identities, not the whole schema. `capture-seed` replaces the sample from the county and is not a normal verification action.

## Doctor

A doctor is a readiness check. Run it before the first drive, on each fresh database, and after any failed drive. If a skill error causes failure, correct it and retry once. Do not continue from unknown state.

```sh
docker inspect -f '{{.Id}} {{.Config.Image}} {{.State.Status}} {{if .State.Health}}{{.State.Health.Status}}{{end}}' "$VERIFY_CONTAINER"
docker inspect -f '{{json .HostConfig.PortBindings}}' "$VERIFY_CONTAINER"
verify_make migrate-version DB_CONTAINER="$VERIFY_CONTAINER" DATABASE_URL="$GOOSE_DATABASE_URL" GOOSE_VERSION=v3.30.0
test -x ./landman
go version
```

Require this run's container ID, image `postgis/postgis:18-3.6-alpine`, running/healthy state, and only `127.0.0.1:55432` as the published database binding. The applied version must equal the highest migration prefix in this checkout. Goose uses container port 5432; host CLI commands use port 55432.

Only after live approval, check layer 215 (parcels) and layer 197 (soil) through the established client URLs. `probe` below records their metadata and one ordered 2000-row page each. It is a timing diagnostic, not a multi-page completeness check. Compare the latest observed source counts with the application's row, receipt, byte, and time limits before recommending the import. Record any gate that rejects the run. A prior source count is not a current completeness receipt. USDA has no separate metadata check here; a failed USDA call is an external drive failure, not a pass.

## Drive

Read [the feature map](features/README.md) and [source import](features/import-parcels-and-soil.md). Do not run two GIS drives at once. Each source process has one ten-minute outer deadline that includes its derived work. Do not apply that claim to the combined bare command or every scheduled job.

```sh
go run ./.cursor/skills/verify-landman-cli/cmd/verify-landman probe --evidence "$EVIDENCE"
if HOST_DATABASE_URL="$HOST_DATABASE_URL" EVIDENCE="$EVIDENCE" \
  bash .cursor/skills/verify-landman-cli/run-gis.sh; then
  drive_status=0
else
  drive_status=$?
fi
go run ./.cursor/skills/verify-landman-cli/cmd/verify-landman classify \
  --container "$VERIFY_CONTAINER" --goose-url "$GOOSE_DATABASE_URL" \
  --seed .cursor/skills/verify-landman-cli/seed.sql --evidence "$EVIDENCE"
```

Run `classify` after a failed runner too. It can make live requests to compare seeded sales. Preserve per-source and combined stdout, stderr, and exit files, `page-timings.json`, and `drive-outcome.json`. Require the combined exit to equal `drive_status`. The runner stops after the first failed source.

A pass requires clean full-scan publication, stored screening, a screening head, one soil assessment per county parcel, and the seeded sale comparison. Source counts and receipts must prove full capture separately from the single-page probe. Do not query a countywide sale total. Omitted sale slots must disappear on complete replacement. A source with quarantine is not a clean pass and does not start its derived work.

GIS stdout is empty. Stderr includes job, attempt, and stage logs; quiet output is not a success condition. `Fetch complete:` ends fetching, not publication or screening. Unknown commands, including `import`, fail before database access; match `usage: landman` and `run <job ID>`, not a historical full usage string. Repair limits outside 1 through 100 fail before database access.

The classifier prints `pass`, `fail`, or `inconclusive: county-source-slow`. It writes the same verdict with `seededSales`, `fullImport`, and `sales` in `drive-outcome.json`. Its exit 2 becomes shell exit 1 under `go run`. Read the JSON and verdict, not only that shell status. An HTTP 502 from USDA is not proof of county slowness.

## Cleanup and evidence

Run cleanup after the last drive, including any correction check. Confirm the named evidence files still exist. Stop only a CLI PID this run started. Remove only this run's recorded container and volume:

```sh
docker rm -f "$VERIFY_CONTAINER"
docker volume rm "$VERIFY_VOLUME"
test -d "$EVIDENCE"
```

Do not remove evidence or stop `landman-postgis`. Test results do not replace real CLI and database evidence for this optional journey. Report an unapproved source path as not run, with the exact approval or input prerequisite. Preserve full-county release and performance gates.
