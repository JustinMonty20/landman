# Spatialist removal and critical fixes

Source: [Go architecture audit, revised](https://cursor.com/canvas/store-11be7bad-acc0-4616-92d5-8d428c775323/99b2a528-396b-4acf-9126-69a9bf84bab3).

This plan is the Spatialist removal plus the findings that audit rated High. Medium and Low findings stay out. The scraper that replaces this enricher is specified in [plans/scraper-enrichment.md](scraper-enrichment.md). Do not add the scrape package until Steps 1, 2, and 5 pass. Step 6 passes before any insert into `parcels.shape`. Step 4 passes before a writer maps `GISParcel` with `db` tags. Steps 3 and 4 do not block the scrape package.

No step here revives `connector.SingleParcelFetcher` or `service.BatchEnricher`.

## What the running path uses Spatialist for

`main` builds a Spatialist client, wraps it in `service.BatchEnricher` at concurrency 5, wraps that in `SpatialistEnricher` with `FlattenSpatialistRecord` and `KeepWhenYearBuiltIsNil`, and calls `EnrichBatch` from the GIS batch hook. `main.go` lines 25 to 51 and 73 to 80. The hook logs an enricher error and returns nil. Lines 74 to 81. `sortedParcelIDs` sorts `*spatialist.SpatialistFlatEnvelope` keys. Lines 90 to 105.

The GIS importer does not need that path. It already filters with `union.IsLikelyVacantGISRecord` and emits parcel ids. `main.go` lines 55 to 59. `internal/service/parcel_id_importer.go` lines 63 to 71.

## Delete list

Delete the package `internal/connector/union/spatialist`. Production callers are `main.go` and `main_test.go`.

- `internal/connector/union/spatialist/spatialist.go`
- `internal/connector/union/spatialist/spatialist_test.go`
- `internal/connector/union/spatialist/spatialist_enricher.go`
- `internal/connector/union/spatialist/spatialist_enricher_test.go`
- `internal/connector/union/spatialist/spatialist_filter.go`
- `internal/connector/union/spatialist/spatialist_filter_test.go`
- `internal/connector/union/spatialist/spatialist_flatten.go`
- `internal/connector/union/spatialist/spatialist_flatten_test.go`

Delete these with the package. A search of `*.go` shows no other production user.

- `internal/connector/single_fetcher.go`. `SingleParcelFetcher` is implemented only by `UnionCountySpatialist`. `spatialist.go` `FetchByParcelID`, and the status check at lines 97 to 98.
- `internal/service/source_fetcher.go`. `BatchEnricher`, `SourceConfig`, and `Logger` fan out `SingleParcelFetcher`. Lines 14 to 31.
- `internal/service/source_fetcher_test.go`.
- `main_test.go`. Both tests call `sortedParcelIDs` with a Spatialist envelope. Lines 7 to 28. After the helper is gone the file has no other test.

`todo/webscraper_enrichment.md` used to tell the next change to implement `SingleParcelFetcher` and to mirror `SpatialistEnricher`. It is now a pointer to [plans/scraper-enrichment.md](scraper-enrichment.md) and to this file. Do not follow the old action list.

## What stays

- `internal/connector/union/gis.go`, `translator.go`, `merger.go`, `vacant_filter.go`, and their tests.
- `internal/connector/shared/httpclient`. GIS calls `NewHTTPClient` and `NewRateLimitedClient` at `gis.go` lines 69 to 70.
- `internal/service/parcel_id_importer.go` and `parcel_id_importer_test.go`.
- `internal/models`, `db/migrations`, Compose, and the Makefile goose targets.

`internal/service` is then one type, `ParcelIDImportService`. Keep the package. The next county uses the same importer.

## Retired findings

Do not fix these. The code leaves with Spatialist.

- The `main` hook logs a Spatialist error and returns nil. `main.go` lines 73 to 81.
- `BatchEnricher` workers return on `ctx.Err()` without recording an error, so `EnrichBatch` can return a nil error after cancellation. `internal/service/source_fetcher.go` lines 78 to 81 and 125 to 129.
- `SpatialistEnricher` appends the batch error and then appends a missing-record error for each parcel the batch already failed. `spatialist_enricher.go` lines 74 to 87.
- Tests that only cover Spatialist flatten, filter, and enrich behavior.

The GIS status bug is not in this list. It is Step 3.

## Step 1. Point `make build` at the real main package

High. `Makefile` line 25 builds `./cmd/main.go`. There is no `cmd/` tree. `go build ./...` succeeds. The `run` target at line 36 already builds `./main.go`. One binary at the module root is the layout until a second process exists.

1. Change the `build` recipe so it compiles `.` or `./main.go` and still writes `$(BINARY_NAME)`.
2. Add `/landman` to `.gitignore`. `.gitignore` ignores `.env` at line 25 and does not name the binary `make run` writes.

Check:

- `make build` exits 0.
- `go build ./...` exits 0.
- The build log names `./main.go` or the module root, and it does not name `./cmd/main.go`.

## Step 2. Remove Spatialist and the ports that exist only for it

High. Deleting the package and leaving `SingleParcelFetcher` or `BatchEnricher` in place creates dead ports.

1. Delete the files in the delete list.
2. Edit `main.go`. Drop the `spatialist` import. Drop the `connector` import if it remains only for the `SingleParcelFetcher` map. Drop the `sort` import used by `sortedParcelIDs`. Delete `sortedParcelIDs`.
3. Keep the timeout, `union.NewGISSource`, `union.IsLikelyVacantGISRecord`, and `service.NewParcelIDImportService`. `main.go` lines 16 to 23 and 54 to 59.
4. Leave the batch hook as a print of the parcel ids. It returns nil only when that print is the whole body. Any later error from work inside the hook must be returned. `ParcelIDImportService.Run` already returns the hook error. `parcel_id_importer.go` lines 65 to 71.
5. In `docs/ARCHITECTURE.md`, remove the `spatialist/` entries at lines 92 to 95 so the target tree stops naming this package. Leave the rest of that document for Step 6 of [plans/scraper-enrichment.md](scraper-enrichment.md), which replaces the `init()` registration sample.

Check:

- `rg -n "spatialist|SingleParcelFetcher|BatchEnricher|sortedParcelIDs" --glob '*.go'` prints nothing.
- `rg -n "spatialist/" docs/ARCHITECTURE.md` prints nothing.
- `go test -race ./...` exits 0.
- `go build ./...` exits 0.
- `make build` exits 0.

`internal/connector` and `internal/models` have no test files today. A green `go test ./...` does not cover them.

## Step 3. Fail the GIS fetch on a non-success HTTP status

High. `queryTotalParcels` reads `count` and returns `result.Int()` with no status check. `internal/connector/union/gis.go` lines 173 to 185. `fetchBatch` does the same for `features`. Lines 199 to 211. A body without a numeric `count` becomes 0. `FetchBatches` then performs no data requests and returns nil. After Step 2 this is the program's success signal.

Spatialist's client rejected statuses outside 200 to 299. `spatialist.go` lines 97 to 98. That check disappears in Step 2. GIS needs its own.

1. In `queryTotalParcels` and `fetchBatch`, if `StatusCode` is outside 200 to 299, return an error that includes the status. Use `%w` when wrapping another error.
2. On a 200 response, treat a missing `count` as an error. A present `count` of 0 stays an empty layer. `gjson`'s `Exists` distinguishes those. Do not use `Int()` alone.
3. Add an `httptest` case for HTTP 500 and a case for a 200 body shaped like an ArcGIS error object with no `count`. Both must fail the fetch.
4. While editing `validateURL` (`gis.go` lines 274 to 283), switch the format verb that wraps the parse error to `%w`. The neighboring GIS errors already use `%w` at lines 113 and 145. Update `TestValidateURL` if it matches the old substring.
5. Run `gofmt` on the edited Union files. `gofmt -l` already lists `internal/connector/union/gis_test.go` on this tree.

Check:

- `go test -race ./internal/connector/union` exits 0.
- The new 500 test fails the package if the status check is removed.
- `gofmt -l .` prints nothing.
- `go test -race ./...` exits 0.

## Step 4. Make the Go sale and owner fields match the table

High. This is the first writer you will want once more than one county is stored. Do it before any repository maps `GISParcel` with `db` tags.

`TaxOwnerID1` and `TaxOwnerID2` are `*int64` with db tags `tax_owner_id_1` and `tax_owner_id_2`. `internal/models/gis_parcel.go` lines 44 to 45. The columns are `TEXT`. `db/migrations/000001_init_schema.sql` lines 20 to 21. The translator fills them with `intPtr`. `internal/connector/union/translator.go` lines 72 to 73. The test expects integers 101 and 202. `translator_test.go` lines 166 to 167.

`ParcelSale` still has `SaleYear`, `SaleMonth`, and `SaleDay`. `gis_parcel.go` lines 95 to 97. `buildSales` fills them from `s1_SALE_YEAR`, `s1_SALE_MTH`, and `s1_SALE_DAY` (and the `s2` / `s3` prefixes). `translator.go` lines 383 to 385. `parcel_sales` has `sale_date` and `og_sale_date` only. Migration lines 94 to 113. [plans/remove-sale-date-parts.md](remove-sale-date-parts.md) already removed the SQL columns. Do not put those columns back.

1. Change `TaxOwnerID1` and `TaxOwnerID2` to `*string`. Fill them with the existing string helper in the translator. Update the translator test to expect the string form of the fixture values.
2. Remove `SaleYear`, `SaleMonth`, and `SaleDay` from `ParcelSale`. Stop setting them in `buildSales`. Update the sale-present check at `translator.go` line 402 so it no longer reads those fields. Update `translator_test.go` assertions such as the `SaleYear` check at line 235.
3. Leave `sale_date` and `og_sale_date` as they are.

Check:

- `rg -n "sale_year|sale_month|sale_day" internal db` prints nothing.
- `TaxOwnerID1` in `gis_parcel.go` is a string type, and `tax_owner_id_1` in `db/migrations/000001_init_schema.sql` is `TEXT`.
- `go test -race ./...` exits 0.

## Step 5. Move Union extras out of `internal/models`

High. Prerequisite for [plans/scraper-enrichment.md](scraper-enrichment.md). The scraper must not grow another pointer on this struct.

`ParcelSourceExtras` has one field, `Union *UnionParcelExtras`. `internal/models/gis_parcel.go` lines 115 to 120. The comment at lines 116 to 117 says to add another pointer per county. The translator fills `SourceExtras.Union`. `translator.go` lines 124 to 125. The test walks `parcel.SourceExtras.Union` field by field. `translator_test.go` lines 203 to 222.

`UnionGISParcel` lives in `internal/models/union_gis_parcel.go` and no other Go file names the type. Its comment says geometry is requested as GeoJSON with `outSR=4326` (lines 9 to 14). The live GIS request does not set `outSR`. The translator uses `unionGISSourceSRID = 3857`. `translator.go` line 15.

1. Move `UnionParcelExtras` into package `union`.
2. Change the shared extras field to `json.RawMessage` (db tag `source_extras`). The Union translator marshals `{"union": ...}` from the county struct. `internal/models` does not name Union. A later county writes its own JSON key from its own package.
3. Point the translator tests at the county struct or at unmarshalled JSON. Keep literal field expectations. `OBJECTID` is still 1 in the existing fixture (`translator_test.go` line 206).
4. Delete `internal/models/union_gis_parcel.go` unless a live decoder starts using `UnionGISParcel` in the same change. Nothing does today.

Skip the JSON move only if every reader of `GISParcel` must see typed Union fields without importing package `union`. Then keep the pointer and add a sibling pointer for one experimental county. With several counties planned, do the JSON move.

Check:

- `rg -n "UnionParcelExtras|UnionGISParcel" internal/models` prints nothing.
- `rg -n "type ParcelSourceExtras" -A 8 internal/models/gis_parcel.go` shows `json.RawMessage` and no county name.
- `go test -race ./...` exits 0.

## Step 6. Write the storage SRID rule on the geometry column

High. Prerequisite before any insert into `parcels.shape`. The scraper table in the other plan has no geometry, so this step does not block the scraper's `httptest` package. It does block persisting `GISParcel.Geometry`.

`shape` is `GEOMETRY(Geometry, 3857)` and `srid` is `INTEGER NOT NULL DEFAULT 3857`. `db/migrations/000001_init_schema.sql` lines 62 to 63. The check at line 91 requires `shape IS NULL OR ST_SRID(shape) = srid`. The typmod already forces 3857, so a second county cannot insert a native State Plane or 4326 polygon until the column changes or the translator transforms the rings.

The Union translator assigns `unionGISSourceSRID` (3857) to `Geometry.SRID`, `SourceSRID`, and the polygon. `translator.go` lines 15, 55, and 119 to 120, and `SetSRID` at line 333. `TestGISTranslator_TranslateGeometry` compares the result to that same constant. `translator_test.go` lines 281 to 294. A wrong constant that both sides share still passes.

No Go file inserts rows. `go list` imports for this module contain no `pgx` and no `database/sql`. The migration is still the contract. The column default means an insert that omits `srid` stores 3857 (line 63).

Decision for this tree: keep storage SRID 3857. Writers transform into 3857. `source_srid` holds the native code. `unionGISSourceSRID` stays in package `union`. County packages must not import it.

1. Add a Goose migration whose up script comments `parcels.shape` and `parcels.srid`. The comment states that 3857 is the storage SRID, that writers transform into it, and that `source_srid` is the native code. Do not edit `000001` for this comment. [plans/remove-sale-date-parts.md](remove-sale-date-parts.md) already edited that file for a different change.
2. Change `translator_test.go` so the geometry assertions expect the literal `3857`, not `unionGISSourceSRID`.
3. Leave the typmod at 3857.

Change the typmod in a later migration only when a county's native SRID is not 3857 and you refuse to transform. Until that migration exists, the check below matches the 3857 decision.

Check:

- `rg -n "GEOMETRY\\(Geometry, 3857\\)" db/migrations` still matches `000001_init_schema.sql`.
- `rg -n "storage SRID" db/migrations` matches the new comment migration.
- `rg -n "unionGISSourceSRID" internal/connector/union/translator_test.go` prints nothing.
- `go test -race ./internal/connector/union` exits 0.

## Done

All six checks above pass, and `go test -race ./...`, `go build ./...`, and `make build` exit 0 on the resulting tree.

`go mod tidy -diff` currently fails because `go.mod` still requires unused `github.com/jackc/pgx/v5` and its related modules. That is a Medium finding in the audit. This plan does not adopt it. Do not treat a tidy failure on `pgx` as a failure of these steps.
