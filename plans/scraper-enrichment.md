# Parcel scraper

Source: [Go architecture audit, revised](https://cursor.com/canvas/store-11be7bad-acc0-4616-92d5-8d428c775323/99b2a528-396b-4acf-9126-69a9bf84bab3).

Some parcel facts have to come from a site other than the county GIS service. This plan is that scraper. It replaces [todo/webscraper_enrichment.md](../todo/webscraper_enrichment.md), which pointed at `connector.SingleParcelFetcher` and `SpatialistEnricher`. Those types are removed by [plans/spatialist-removal-and-critical-fixes.md](spatialist-removal-and-critical-fixes.md).

The site, the URL pattern, the field list, and any auth or cookie requirement are still unknown. The old todo listed them as open questions, and this plan does not answer them. The package layout, the interface, the place in the GIS run, rate limiting, storage, and error behavior are decided below so the unknown site can land without bringing the Spatialist ports back.

`internal/service` does not import `union` today. Keep it that way. The scraper is county-specific until a second county uses the same site.

## Where it lives

Put the HTTP client, the config struct, the parser, and the typed record in `internal/connector/union/scrape`. That is a Union source, next to the Union GIS code, the same way Spatialist was a child package of `union` before its removal. A second county with a different site gets `internal/connector/<county>/scrape`. It does not add files to package `union`, and it does not implement a port in `internal/connector`.

`internal/connector/union/exports.go` is not in the tree. The old todo said to wire the scraper through that file. Do not create it for this feature. `main` calls the Union constructor directly today. `main.go` lines 20 to 23.

The typed record stays in package `scrape`. Retained fields are named on that struct once the field list is known. Do not keep the retained output as `map[string]interface{}`. `connector.RawRecord.RawData` is already that map for GIS input (`internal/connector/datasource.go` lines 8 to 15). The scraper output is a different, typed value.

## The interface

Define one small interface in package `union`, which is the package that calls the scraper after GIS ids exist. The HTTP client in package `scrape` is not required to name the interface. Its method set matches.

```go
type parcelScraper interface {
    Scrape(ctx context.Context, parcelID string) (scrape.Record, error)
}
```

Export a function on package `union`, for example `EnrichParcelIDs`, that accepts this interface. Tests in package `union` pass a fake. That fake is the second implementation. The interface stays unexported. `main` passes `*scrape.Client` into the exported function.

Do not add `SingleParcelFetcher` back to `internal/connector`. Do not add `BatchEnricher` back to `internal/service`. The audit's reason still holds: one global per-parcel port, with its only implementation deleted, is a dead port. `spatialist.RawBatchEnricher` lived beside `SpatialistEnricher` (`spatialist_enricher.go` lines 11 to 14). Copy that placement. Do not copy `SpatialistEnricher`.

If the unknown site turns out to be a paged bulk feed rather than a per-parcel lookup, stop. Implement `connector.BatchFetchSource` (`internal/connector/batch.go` lines 9 to 14) on that client and let `ParcelIDImportService` stream it. Delete `parcelScraper` in that same change. A paged feed does not need a per-parcel interface.

## How it runs next to the GIS importer

`ParcelIDImportService` already streams parcel ids from a `BatchFetchSource` and calls `ParcelIDBatchHook`. `internal/service/parcel_id_importer.go` lines 10 to 12 and 63 to 71. Union GIS implements that port. `main` passes `union.IsLikelyVacantGISRecord` as the filter. `main.go` lines 55 to 59.

After the Spatialist removal, the hook only prints ids. The scraper runs inside that hook, on the ids the filter kept, by calling `union.EnrichParcelIDs`. The GIS fetch stays the source of ids. The scraper does not page the GIS service and does not replace `GISSource`.

`NewParcelIDImportService`'s `batchSize` is ignored when the source implements `FetchBatches`. `parcel_id_importer.go` lines 63 to 72 versus 83 to 100. Union's page size is `GISSourceConfig.BatchSize`, default 50. `gis.go` lines 25 to 30 and 61 to 63. The hook therefore sees GIS pages, not the integer passed to the importer. Scrape those ids. Do not add a second chunking layer.

Call the scraper one id at a time. Check `ctx.Err()` before each id. A worker pool is a later change, and only after this sequential path is tested. The deleted `BatchEnricher` is the example of a pool that drops cancellation. `internal/service/source_fetcher.go` lines 78 to 81.

## Rate limiting

Build the scraper's HTTP client with `httpclient.NewHTTPClient` and `httpclient.NewRateLimitedClient`. `internal/connector/shared/httpclient/http_client.go` lines 36 to 53 and 63 to 71. `Do` waits on the limiter with `ctx` and returns that error. Lines 24 to 28.

Give the scraper its own `RateLimitedClient`. `NewGISSource` builds a separate one for GIS. `gis.go` lines 69 to 70. The two sources do not share a limiter. `main` passes the scraper's requests-per-second in the scrape config. The scrape package does not call `os.Getenv`. No Go file calls `os.Getenv` today.

`NewRateLimitedClient` substitutes 1 when the rate is 0, and it passes a negative rate through. `http_client.go` lines 63 to 67. `NewGISSource` clamps `<= 0` to 1 before the call. `gis.go` lines 65 to 67. The scrape constructor does the same clamp, in the scrape package, before `NewRateLimitedClient`.

The rate value itself waits on the open question of what the site allows. Default it to 1 request per second in the config constructor, matching the GIS default, and override it from `main` when the limit is known.

## Where scraped fields are stored

County GIS leftovers stay in `parcels.source_extras`. That column is `JSONB NOT NULL DEFAULT '{}'`. `db/migrations/000001_init_schema.sql` line 68. After Step 5 of the removal plan, the Go value is a `json.RawMessage` filled by the county translator, not a struct field per county.

Scraped fields do not go in that column, and they do not go on `UnionParcelExtras`. They are a different source. The parcels unique key is `(county_id, source_name, source_parcel_id)` (migration line 73), which would allow a second `parcels` row. Do not use a second `parcels` row for the scraper. `shape` is `GEOMETRY(Geometry, 3857)` (line 62). A scrape has no geometry in this design, and a second parcel row would still invite someone to hide the scrape JSON in that row's `source_extras`.

Add a new Goose migration. Do not edit `000001` for this table. The table is:

- `county_id TEXT NOT NULL`
- `source_name TEXT NOT NULL`, the scraper's own name, distinct from `"union_county_gis"` (`gis.go` line 79)
- `source_parcel_id TEXT NOT NULL`
- `fetched_at TIMESTAMPTZ NOT NULL`
- `document JSONB NOT NULL`, the JSON form of `scrape.Record`
- unique `(county_id, source_name, source_parcel_id)`

No `shape` column. The storage SRID rule does not apply to this table.

`parcel_sales` references `parcels(id)` and has no county column (migration lines 94 to 97). Do not hang scraped documents off `parcel_sales`.

Promoting a scraped value into a shared `parcels` column, such as `year_built`, is a merger change. `ParcelMerger.Merge` returns `parcels[0]` whenever more than one parcel is present. `internal/connector/union/merger.go` lines 23 to 31. The comment above that function describes an authority order the code does not implement. Do not write scraped values onto the GIS row to get around the stub. Leave promotion for a later change that replaces the "first parcel wins" test in `merger_test.go` with literal field expectations.

There is no repository in this module yet. The migration can land before the writer. The `httptest` work does not need a database.

## Errors and cancellation

Copy the status check Spatialist had, not the hook that swallowed it.

- Status outside 200 to 299 is an error that includes the status code. `spatialist.go` lines 97 to 98 did this. `queryTotalParcels` does not. `gis.go` lines 173 to 185. The scraper follows the Spatialist check.
- Invalid JSON is an error wrapped with `%w`.
- `EnrichParcelIDs` returns the first scrape error to the hook. The hook returns that error. `Run` returns it (`parcel_id_importer.go` lines 65 to 71). Do not log the error and return nil. That is `main.go` lines 74 to 81, and Step 2 of the removal plan deletes it.
- Before each parcel id, if `ctx.Err()` is set, return `ctx.Err()` and do not start the request. Pass that same `ctx` into `http.NewRequestWithContext` and into `RateLimitedClient.Do`.
- A cancelled batch is a single error. Do not also append a "missing record" error per id. `SpatialistEnricher` did that at `spatialist_enricher.go` lines 74 to 87.
- One failed parcel fails the hook call. The import stops. A later change can record a per-parcel failure and continue, and that change has to say so in the hook's contract. This plan does not.

## Tests and fixtures

Tests sit in `internal/connector/union/scrape`, next to the client. Use `httptest`. Cover constructor rejection of an empty URL, HTTP 500, a 200 body that is not the expected document, a successful parse into `scrape.Record`, and a cancelled `ctx` that does not issue the request.

Put fixtures in `internal/connector/union/scrape/testdata/`. `docs/examples/union-gis-parcel-09336350.json` is a GIS example, and no test loads it. Do not point the scraper tests at it.

Package `union` tests use a handwritten fake `parcelScraper` for `EnrichParcelIDs`: success, a returned error, and a cancelled context. They do not start an HTTP server.

`go test ./...` stays hermetic. No Docker and no database. The migration is checked by reading the SQL file until a Go package opens Postgres.

## Prerequisites

Do these before the scraper package exists. The steps and the pass/fail checks are in [plans/spatialist-removal-and-critical-fixes.md](spatialist-removal-and-critical-fixes.md).

**Prerequisite. Spatialist removal (that plan, Steps 1 and 2).** `SingleParcelFetcher`, `BatchEnricher`, and the Spatialist package are gone. `rg -n "spatialist|SingleParcelFetcher|BatchEnricher" --glob '*.go'` prints nothing. `go test -race ./...` and `make build` exit 0. Starting the scraper before this recreates the port the old todo asked for.

**Prerequisite. Union extras leave `internal/models` (that plan, Step 5).** `ParcelSourceExtras` becomes `json.RawMessage`. `rg -n "UnionParcelExtras|UnionGISParcel" internal/models` prints nothing. The scraper then has nowhere convenient to hang a `Scrape` pointer on the shared model. County GIS leftovers and scraped documents stay in different stores.

**Prerequisite before any insert into `parcels.shape`. Storage SRID rule (that plan, Step 6).** Storage SRID stays 3857. Writers transform into it. `source_srid` is the native code. The comment migration exists, and `translator_test.go` expects the literal `3857`. This is not a blocker for `go test` of package `scrape`, because the scrape table has no geometry. It is a blocker for persisting `GISParcel.Geometry`.

The GIS HTTP status check and the sale/owner type alignment are High findings in the removal plan (Steps 3 and 4). They are not blockers for the scrape package. Land them before a GIS writer exists. The scraper does not read `TaxOwnerID1` or `ParcelSale`.

## Land alongside: explicit county list in `main`

This is not a blocker for the scrape package tests. Land it in the same change that wires `EnrichParcelIDs` into `main`. A one-element slice does not, by itself, make a second county cheaper. The extras move does. The slice is how the scraper and the GIS source stay visible in one function.

`main` currently calls `union.NewGISSource` directly. `main.go` lines 20 to 23. There is no `func init()` under `internal/`. `docs/ARCHITECTURE.md` still says to add `register.go` with `init()` and a blank import. Lines 441 to 444 and the sample at lines 476 to 492.

1. Add a function in `main` that returns a slice of county runs. Each element carries the county id, the `connector.DataSource`, and, once this feature is wired, the scrape client. Union is the only element.
2. `main` loops over that slice for the GIS run and the scrape hook. The Union constructor is inside the function, not in `init()`.
3. Replace the "When Adding a New County" steps and the `init()` sample in `docs/ARCHITECTURE.md` (lines 436 to 444 and 473 to 493) with the slice. A new county adds a constructor call in that function. It does not add `register.go`.
4. Keep config structs in the county or scrape package. `main` passes them in. `DefaultGISSourceConfig` already returns a struct by value. `gis.go` lines 24 to 30.

Check:

- `rg -n "func init\\(" --glob '*.go' internal` prints nothing.
- `rg -n "func init\\(" docs/ARCHITECTURE.md` prints nothing.
- `main.go` contains the slice function and names `union.NewGISSource` inside it.
- `go test -race ./...` exits 0 with no database running.

Skip the slice only while the scraper is still an unwired package under test. Add it when `main` starts calling `EnrichParcelIDs`.

## Open questions

These block the parser and the real URL. They do not block the interface, the `httptest` seams, or the migration shape.

- Which site, and is the fetch a per-parcel lookup, a search, or a paginated listing? A paginated listing uses `BatchFetchSource` instead of `parcelScraper`, as specified above.
- Which fields are retained on `scrape.Record`?
- Does the site need auth, cookies, or anything beyond the shared HTTP client?

## Steps

### Step 1. Scrape package and the Union interface

Prerequisite: removal plan Steps 1, 2, and 5 are green.

1. Add `internal/connector/union/scrape` with a config struct (base URL, requests per second), `New` that rejects an empty URL and clamps a non-positive rate to 1, and `Scrape(ctx, parcelID)`.
2. Build the client from `httpclient.NewHTTPClient` and `NewRateLimitedClient`. Use `http.NewRequestWithContext`. Return an error on status outside 200 to 299.
3. Parse into `scrape.Record`. Until the field list is known, `Record` can hold the smallest set the first fixture needs, and the open question stays open. Do not store the retained fields as a map.
4. Add `parcelScraper` and `EnrichParcelIDs` in package `union`. Sequential calls. Return `ctx.Err()` before the next id. Return the scrape error unchanged to the caller.
5. Tests and `testdata/` as specified above. Do not wire `main` yet.

Check:

- `go test -race ./internal/connector/union/...` exits 0.
- `rg -n "SingleParcelFetcher|BatchEnricher" --glob '*.go'` prints nothing.
- `rg -n "type parcelScraper" --glob '*.go'` matches a file in `internal/connector/union` and does not match `internal/connector/single_fetcher.go`.
- Package `scrape` does not import `internal/service`. Package `service` does not import `internal/connector/union`.

### Step 2. Wire the hook from the county list

Prerequisite: Step 1. Land the county-list section in this same change.

1. Construct the scrape client in the county slice in `main`, next to `union.NewGISSource`.
2. From the GIS batch hook, call `union.EnrichParcelIDs` with the hook's `ctx` and the parcel ids. Return its error.
3. Print a count only after a nil error.

Check:

- `go test -race ./...` exits 0.
- Cancelling the hook context is covered by the `EnrichParcelIDs` test from Step 1.
- `rg -n "func init\\(" --glob '*.go' internal docs/ARCHITECTURE.md` prints nothing.

### Step 3. Migration for the scrape table

Prerequisite: Step 1. Can land in parallel with Step 2. The writer can wait.

1. Add a new file under `db/migrations` with the columns and unique key in "Where scraped fields are stored". Up and down. No geometry column. No edit to `parcels.source_extras`.
2. Do not add a Go writer in this step unless Step 2 is already done and the field list is known. A writer that inserts the JSON of `scrape.Record` belongs with the known fields.

Check:

- `rg -n "source_extras" db/migrations` still matches the `parcels` column in `000001_init_schema.sql` and does not match the new scrape table as its document column. The new table's JSON column is `document`.
- `rg -n "GEOMETRY" db/migrations` does not match the new file.
- `go test -race ./...` exits 0 with no database running.

### Step 4. Do not merge scraped fields into the GIS row yet

This step is a hold, not a code change. `ParcelMerger` returns the first parcel. `merger.go` lines 30 to 31. Scraped JSON stays in the scrape table.

Revisit only when a named scraped field must occupy a shared `parcels` column. That change replaces the first-wins behavior and its test, and it still does not write the scrape document into `source_extras`.

Check, for as long as this hold stands:

- `rg -n "source_extras" internal/connector/union/scrape` prints nothing.

## Done

Steps 1 through 3 are green, Step 4's hold is still true, and the three prerequisites in the removal plan are green. `go test -race ./...`, `go build ./...`, and `make build` exit 0. Default `go test` does not require Postgres.
