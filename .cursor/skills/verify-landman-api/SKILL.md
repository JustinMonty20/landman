---
name: verify-landman-api
description: Verify the Landman Go API, sessions, and private parcel records. Use for cmd/api, internal/auth, internal/parcelapi, internal/parcelcrm, API migrations, routes, or sessions.
---
# Verify Landman API

Use the small seeded authentication and parcel API fixtures by default. They make no live Google or county requests. Run commands from the repository root. Read `docs/application-login.md` and `docs/parcel-api.md`. Never load `.env` or use Make. Each database lane requires its own new container from [database setup](../verify-landman/database.md).

## Authentication database lane

Select `VERIFY_DB_NAME=landman_auth_test_verify` and complete database setup. Then run:

```sh
export AUTH_TEST_DATABASE_URL="$VERIFY_DATABASE_URL"
go test -race -tags postgres ./internal/auth -count=1 -v
```

Require tests to run, not skip. The database name must start with `landman_auth_test_`. Tests use a local signed-token provider. They check identity, session rotation, hashing, expiry, revocation, allowed accounts, callback replay, rejected claims, origin checks, migration rollback, saved detail fields, and private owner information. Exit the lane shell to remove its container.

## Private favorites and lead-state database lane

Use a new setup shell with `VERIFY_DB_NAME=landman_auth_test_verify`. Run this package separately from the authentication package:

```sh
CRM_TEST_DATABASE_URL="$VERIFY_DATABASE_URL" \
  go test -race -tags postgres ./internal/parcelcrm \
  -run '^(TestFocusedStateRead|TestBoardIsolationAndRevision|TestReplayAfterRemovalRequiresRevision|TestConcurrentDeleteAndMoveHistory|TestBoundedPagesAndOrigin)$' \
  -count=1 -v
```

Require the database tests to execute. Check account isolation, board and card pages, state creation and changes, default states, exclusions, favorite removal, revision conflicts, retries, and exact-origin checks. These tests exercise stored private records. They do not prove browser behavior or reminder delivery. `TestBoardFixtureMeasurement` uses 1,000 favorites and 50 states; run it only as an explicitly selected local performance check. Do not make it a default pass requirement. Exit this shell to remove its container.

## Parcel API database lane

Use a new setup shell with `VERIFY_DB_NAME=landman_api_test_verify`. Then run:

```sh
export API_TEST_DATABASE_URL="$VERIFY_DATABASE_URL"
go test -race -tags postgres ./internal/parcelapi -count=1 -v
```

Require `TestHTTPPostGIS` and the tax and source database tests to run. An unset URL causes skips and is not a pass. Check list/map agreement, parcel lookup, cursor and source stamps, bounds, response limits, tax active-file selection, missing evidence, soil provenance, qualification layers, jurisdiction filters, and stored flood evidence. Exit this shell after the fixture tests. Keep it only when the optional process check is explicitly selected.

## Optional real API process

Run only when explicitly selected. This is not a prerequisite for a default fixture pass.

Run only when port 8080 is unused. Do not stop another process to free it. This check contacts Google's public provider metadata endpoint, but does not sign in or exchange tokens. Record a metadata/network failure separately from a code failure. Dummy client settings cannot verify a real Google callback.

In the parcel API setup shell:

```sh
command -v ss >/dev/null
if [ -n "$(ss -H -ltn 'sport = :8080')" ]; then
  echo 'API process check blocked: port 8080 is in use'
  exit 1
fi
mkdir -p artifacts/verify/api
GOFLAGS= go build -o artifacts/verify/api/server ./cmd/api
env -i PATH="$PATH" HOME="$HOME" \
  DATABASE_URL="$VERIFY_DATABASE_URL" \
  GOOGLE_CLIENT_ID=verify-client GOOGLE_CLIENT_SECRET=verify-secret \
  GOOGLE_REDIRECT_URI=http://127.0.0.1:5173/auth/google/callback \
  AUTH_ALLOWED_EMAILS=buyer@example.test \
  artifacts/verify/api/server >artifacts/verify/api/server.stdout.txt \
  2>artifacts/verify/api/server.stderr.txt &
api_pid=$!
trap 'kill -TERM "$api_pid" 2>/dev/null || true; wait "$api_pid" || true; cleanup_verify' EXIT
api_ready=false
for attempt in {1..30}; do
  kill -0 "$api_pid" 2>/dev/null || break
  code=$(curl -s -o artifacts/verify/api/denied.json -w '%{http_code}' \
    -H 'Host: 127.0.0.1:5173' http://127.0.0.1:8080/api/v1/parcels || true)
  if [ "$code" = 401 ]; then api_ready=true; break; fi
  sleep 1
done
[ "$api_ready" = true ]
kill -TERM "$api_pid"
wait "$api_pid"
trap cleanup_verify EXIT
```

Require HTTP 401 without a session and a clean process exit. Database fixtures verify allowed-session data requests. A real Google consent/callback requires separate operator approval.

## Offline checks and completion

```sh
env -u DATABASE_URL -u AUTH_TEST_DATABASE_URL -u API_TEST_DATABASE_URL \
  -u CRM_TEST_DATABASE_URL \
  go test -race ./internal/auth ./internal/parcelapi ./internal/parcelcrm -count=1
go vet ./cmd/api ./internal/auth ./internal/parcelapi ./internal/parcelcrm
go build ./cmd/api ./internal/auth ./internal/parcelapi ./internal/parcelcrm
```

Record commands, exits, test execution or skips, and process evidence under ignored `artifacts/verify/`. Use `verify-landman-ui` for proxy or browser changes. Never substitute browser fixture success for API database evidence. Keep secrets and session tokens out of logs.
