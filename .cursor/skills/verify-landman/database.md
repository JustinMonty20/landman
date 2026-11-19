# Isolated verification database

Use this setup for API, tax, or qualification checks. Use the CLI seed setup only for the live GIS journey. Run from the repository root in Bash. Do not source `.env` or use Make for these checks.

## Create and migrate

Select one database name before this block:

- API sessions: `export VERIFY_DB_NAME=landman_auth_test_verify`
- Parcel API: `export VERIFY_DB_NAME=landman_api_test_verify`
- Tax or qualification: `export VERIFY_DB_NAME=landman_tax_integration`

Run each lane in a separate shell and container. The container has a random loopback port and no retained volume. Docker removal deletes only this test database. The password below is for disposable local data only.

```sh
set -euo pipefail
case "${VERIFY_DB_NAME:-}" in
  landman_auth_test_verify|landman_api_test_verify|landman_tax_integration) ;;
  *) echo 'Select a verification database name'; exit 1 ;;
esac
VERIFY_CONTAINER=$(docker run -d --rm \
  -e POSTGRES_USER=verify -e POSTGRES_PASSWORD=verify \
  -e POSTGRES_DB="$VERIFY_DB_NAME" \
  -p 127.0.0.1::5432 postgis/postgis:18-3.6-alpine)
cleanup_verify() { docker stop "$VERIFY_CONTAINER" >/dev/null; }
trap cleanup_verify EXIT
ready=false
for attempt in {1..60}; do
  if docker exec "$VERIFY_CONTAINER" pg_isready -h 127.0.0.1 \
    -U verify -d "$VERIFY_DB_NAME" >/dev/null 2>&1; then
    ready=true
    break
  fi
  sleep 1
done
[ "$ready" = true ]
VERIFY_PORT=$(docker port "$VERIFY_CONTAINER" 5432/tcp)
VERIFY_PORT=${VERIFY_PORT##*:}
[ "$VERIFY_PORT" != 5432 ]
export VERIFY_DATABASE_URL="postgres://verify:verify@127.0.0.1:$VERIFY_PORT/$VERIFY_DB_NAME?sslmode=disable"
VERIFY_GOOSE_URL="postgres://verify:verify@localhost:5432/$VERIFY_DB_NAME?sslmode=disable"
docker run --rm --network "container:$VERIFY_CONTAINER" \
  -v "$PWD/db/migrations:/migrations:ro" \
  -e GOOSE_DRIVER=postgres -e GOOSE_DBSTRING="$VERIFY_GOOSE_URL" \
  golang:1.26-alpine sh -ec \
  'go install github.com/pressly/goose/v3/cmd/goose@v3.30.0; goose -dir /migrations up; goose -dir /migrations version'
docker exec "$VERIFY_CONTAINER" psql -X -U verify -d "$VERIFY_DB_NAME" \
  -v ON_ERROR_STOP=1 -c 'SELECT current_database(), PostGIS_Version();'
```

Stop on any failed setup command. Save the migration output and database identity with the lane evidence. Do not print credentials from any other environment.

## Cleanup

Exit the setup shell after the checks. Its trap stops only the container it created. Keep evidence files. Do not use `docker compose down`, an operator container, or an operator database.
