# Parcel sales UUID ids

**Out-of-scope decision.** Did not edit `plans/soil-layer.md`, `plans/scraper-enrichment.md`, or `docs/ARCHITECTURE.md`. Those files disagree with the keying rule. The plan records the lines and leaves the rewrites for a later change.

**Out-of-scope decision.** Did not edit `000001_init_schema.sql`. goose v3.24.3 keeps version 3 after that file is changed, so a new migration is the one that moves an existing database.

**Out-of-scope decision.** Did not brand `ParcelIdentity.CountyID` as `county.ID`. The store reads the string the translator already writes. Changing the field would touch the Union translator while this program is only the sale id and the upsert.

**Out-of-scope decision.** Did not wire `SaveParcel` or `SaveSales` from `main.go`. The soil plan's persist step already owns that call. uuid-2 adds the functions and the id clauses.

**Skipped.** Did not add a `pstack/` tree, a `control-cli` skill, or the missing `deslop` skill. Those are recorded as risks. Inventing them is a different change.

**Scope tangent.** Soil polygons still need their own UUID and an overlap join. That reshape belongs in a follow-up to `plans/soil-layer.md`, not in this plan's code.

**Unrelated.** `plans/spatialist-removal-and-critical-fixes.md` still says `TaxOwnerID1` is `*int64`. `internal/models/gis_parcel.go` already stores those fields as `*string`. Leave that stale sentence alone.
