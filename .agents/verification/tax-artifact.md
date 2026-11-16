# Tax artifact verification

**Scope.** This record covers the artifact store and import evidence in `plans/tax-data-ingest.md`. An artifact is an exact source file. A backend is a file-storage method. S3 is cloud object storage. One run selects the local or S3 backend.

**Store contract.** `go test -race ./internal/taxartifact -count=1` passed for the local backend. The same tests passed with the S3 test settings and a temporary `adobe/s3mock` service. The tests read exact bytes, keep two file versions, reuse an identical version, reject a corrupt object, and stop on cancellation or source failure. The S3 test used an isolated test bucket. Default tests do not contact S3.

**Large files.** Each backend accepted a 32 MiB (mebibytes) synthetic file in one test run. Each test set an 8 MiB limit on new Go allocations during `Put`. Both tests passed. This limit is not a peak-heap measurement or a speed claim.

**Database.** A new, disposable PostGIS (spatial database) container received migrations 1 through 18. Goose (the migration tool) then removed and applied migration 18 again. The `taxstore` test passed against that database. It checked the link from a typed tax row to its import, rejected an unverified artifact, kept an invalid import inactive, and kept the prior active import after a transaction rollback. `go test -tags postgres -race ./... -count=1 -timeout 15m` passed with `DATABASE_URL` set to that disposable database.

**Repository.** `go vet ./...`, `go build ./...`, and `env -u DATABASE_URL make test` passed. Go used cached results for unchanged packages in `make test`. The uncached `env -u DATABASE_URL go test -race ./... -count=1` run passed.

**Limits.** No test contacted a county service, an operator database, or a deployed S3 bucket. Tax adapters do not call the new store yet. Bucket access, encryption, retention, and release configuration remain open in the plan.
