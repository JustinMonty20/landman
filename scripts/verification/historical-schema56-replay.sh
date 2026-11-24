#!/usr/bin/env bash
set -euo pipefail
# Local-only, disposable PostGIS. Preserve both database and captures after run.
root="$(git rev-parse --show-toplevel)"
: "${HISTORICAL_PG_CONTAINER:?set a dedicated disposable PostGIS container}"
: "${HISTORICAL_PG_PORT:?set dedicated container host port}"
cd "$root"
name="landman_parceljob_test_historical_$(date +%s)"
out="$root/artifacts/verify/historical-$name"
mkdir -p "$out/src" "$out/captures"
chmod 700 "$out/captures"
git archive 5b3550d | tar -x -C "$out/src"
cp scripts/verification/historical_seed_postgres_test.go.txt "$out/src/internal/connector/union/parceljob/historical_seed_postgres_test.go"
if docker exec "$HISTORICAL_PG_CONTAINER" psql -U review -d postgres -Atqc "SELECT 1 FROM pg_database WHERE datname='$name'" | grep -q 1; then
 echo 'refusing existing database' >&2; exit 1
fi
docker exec "$HISTORICAL_PG_CONTAINER" createdb -U review "$name"
export DATABASE_URL="postgres://review:disposable-review-only@127.0.0.1:${HISTORICAL_PG_PORT}/${name}?sslmode=disable"
export HISTORICAL_ROOT="$out/captures"
"$(go env GOPATH)/bin/goose" -dir "$out/src/db/migrations" postgres "$DATABASE_URL" up-to 56 > "$out/migrate56.log" 2>&1
(cd "$out/src" && go test -tags postgres ./internal/connector/union/parceljob -run '^TestHistoricalSchema56Seed$' -count=1 -v) > "$out/seed.log" 2>&1
"$(go env GOPATH)/bin/goose" -dir db/migrations postgres "$DATABASE_URL" up > "$out/migrate-head.log" 2>&1
go test -tags postgres ./internal/connector/union/parceljob -run '^TestHistoricalSchema56CaptureAfterUpgrade$' -count=1 -v > "$out/replay.log" 2>&1
printf 'database=%s\nartifact=%s\n' "$name" "$out"
tail -8 "$out/seed.log"
tail -8 "$out/replay.log"
