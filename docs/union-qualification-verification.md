# Union qualification ingestion verification

## Local checks

The checked-in county metadata and two-feature samples cover all 15 scoped layers. The Go client retrieved the samples with HTTP 200; earlier Python probes returned HTTP 403 and do not describe application behavior. Fixtures contain no owner, parcel, contact, or mailing attribute keys. No county-scale live import was run.

The following checks ran on the task worktree after the host-lock, road-boundary, and independent-reconciliation fixes:

- `go test -race ./... -count=1` passed the offline repository suite.
- `go run honnef.co/go/tools/cmd/staticcheck@v0.8.2 ./...`, `go vet ./...`, and `go build ./...` passed.
- `python3 -m unittest discover -s .github/scripts -p 'test_verify_review.py'` passed all six tests.
- `go test -p 1 -race -tags postgres ./cmd/landman ./internal/parcelstore ./internal/taxstore ./internal/qualification -count=1` passed on the task-owned tax-compatible PostGIS database. The taxstore tests reject the separate qualification database by design.
- The qualification database passed focused PostGIS tests for lock-session loss, road boundary freshness, failed-group independence, publication, holes, and stale assessments. A fresh disposable database passed Goose migration up, down of migration 20, and up to migration 20. The fresh database was removed after the check.

Test logs are in ignored `artifacts/verify/union-qualification/`. The task-owned PostGIS container and both disposable task database connections remain available to the operator. Source fixtures check selected real attributes and geometry, not county-wide completeness or current legal authority.

## Corrected independent-review findings

The original `final-receipt.json` returned `EXACT` before these corrections. New evidence uses separate `corrected-*.log` files. The migrated task-owned PostGIS replay stores every captured zoning, jurisdiction, boundary, and road layer twice. Layer 190 OID 229169 and layer 195 OID 116697 retain invalid raw EWKB alongside valid repaired geometry; the measured absolute area changes are 5.364418e-7 and 2.235174e-8 m² in EPSG:3857. Both repairs meet the documented 0.01 m² absolute **and** 1e-8 relative limits. A bow-tie with a larger area delta fails insert. Neither validity constraints nor complete-group publication were weakened.

The corrected PostGIS tests assert zero district area for asymmetric-vertex boundary contact, positive area for a genuine narrow overlap, same-name ETJ/municipal conflict, zero road distance on stored touching shapes, far-outside-county warning, old evidence's 99-day fetch age after head replacement, and failed first manifest insert recording. A published roads head without parcel assessment reports typed `pending_assessment`; absent source remains `source_unavailable`. Cross-process lock tests cover failed, cancelled, and killed lock sessions, with a new holder waiting one second before its callback. Feature tests check each required mapped attribute for missing, wrong type, and explicit null plus malformed OIDs. The scheduler dispatches qualification without invoking soil screening. These tests use local fixtures and the operator-approved task-owned databases only.

The corrected verification commands and receipts are `corrected-offline-race.log` (`go test -race ./... -count=1`), `corrected-postgres-ci.log` (`go test -p 1 -race -tags postgres ./cmd/landman ./internal/parcelstore ./internal/taxstore ./internal/qualification -count=1`), `corrected-staticcheck.log`, `corrected-vet.log`, `corrected-build.log`, `corrected-migration.log`, `corrected-import.log`, `corrected-spatial.log`, `corrected-lock.log`, and `corrected-schema.log`. The migration SQL was reapplied to both task-owned databases for this undeployed migration. The fixture inputs are `internal/connector/union/testdata/qualification/*-metadata.json` and `*-sample.json`. No live county network request or county-scale import was made.

## Unverified release conditions

A full live import, its memory and time budget, the future stability of county object IDs, county zoning authority and update dates, and road coverage beyond the county boundary remain unverified. The ArcGIS count and ordered pages detect some drift but cannot prove a transactional county snapshot. A terminated lock session cancels context-aware source work; a transport or storage call that ignores context may continue until it returns. FEMA ingestion and the buyer-facing API and UI are separate V1 work.
