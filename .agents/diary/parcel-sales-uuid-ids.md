# Parcel sales UUID ids

**Out-of-scope decision.** Did not edit `plans/soil-layer.md`, `plans/scraper-enrichment.md`, or `docs/ARCHITECTURE.md`. Those files disagree with the keying rule. The plan records the lines and leaves the rewrites for a later change.

**Out-of-scope decision.** Did not edit `000001_init_schema.sql`. goose v3.24.3 keeps version 3 after that file is changed, so a new migration is the one that moves an existing database.

**Out-of-scope decision.** Did not brand `ParcelIdentity.CountyID` as `county.ID`. The store reads the string the translator already writes. Changing the field would touch the Union translator while this program is only the sale id and the upsert.

**Out-of-scope decision.** Did not wire `SaveParcel` or `SaveSales` from `main.go`. The soil plan's persist step already owns that call. uuid-2 adds the functions and the id clauses.

**Skipped.** Did not add a `pstack/` tree, a `control-cli` skill, or the missing `deslop` skill. Those are recorded as risks. Inventing them is a different change.

**Scope tangent.** Soil polygons still need their own UUID and an overlap join. That reshape belongs in a follow-up to `plans/soil-layer.md`, not in this plan's code.

**Unrelated.** `plans/spatialist-removal-and-critical-fixes.md` still says `TaxOwnerID1` is `*int64`. `internal/models/gis_parcel.go` already stores those fields as `*string`. Leave that stale sentence alone.

**Out-of-scope decision.** The first plan draft listed the integer type declarations and missed the fixture literals. The follow-up adds those lines, `main.go` lines 29 to 40, `plans/soil-layer.md` line 641, and `plans/db-schema-type-audit.md` line 9. It does not change uuid-1 or uuid-2.

**Out-of-scope decision.** uuid-1 is the new goose file only. uuid-2, the Go writers, and `internal/parcelstore` stay on a later PR. No Go file was edited. `000001`, `000002`, and `000003` stay as they are. The directory listing at the start had no `000004`, so the file is `000004_parcel_sales_uuid_id.sql`.

**Skipped.** This VM has no Docker and no `psql`, so `make migrate-up`, live lanes 2 through 10, the head column check in lane 1, and the upsert perf probe were not run. Trunk and head `go test -race ./... -count=1` both exited 0. Plan checkboxes stay open because live and perf evidence does not exist yet.

**Skipped.** Did not add `pstack/`, a `/loop` audit, a swarm, Bugbot triage, or `/deslop` during implementation. The repo has none of that. Inventing it would be a different change.
