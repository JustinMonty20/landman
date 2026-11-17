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

## Unverified release conditions

A full live import, its memory and time budget, the future stability of county object IDs, county zoning authority and update dates, and road coverage beyond the county boundary remain unverified. The ArcGIS count and ordered pages detect some drift but cannot prove a transactional county snapshot. A terminated lock session cancels context-aware source work; a transport or storage call that ignores context may continue until it returns. FEMA ingestion and the buyer-facing API and UI are separate V1 work.
