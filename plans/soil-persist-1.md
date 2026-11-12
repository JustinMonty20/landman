# Persist Union County parcel shapes

## Goal
Store every layer 215 feature as a parcel shape, including occupied parcels and features without a PID. Preserve parcel UUIDs across imports. Record the numeric-zero vacancy rule as `likely_vacant` without filtering the county download.

## Boundary
This unit follows `plan/soil-filter-1`. It changes only the importer, Union GIS paging, parcel writer, main, their tests, and the next sequential parcel migration. Soil fetches, soil tables, sales writes, map views, and parcel deletion stay outside this unit.

## Implementation
- Page all layer 215 features with `where=1=1`, `orderByFields=OBJECTID`, and 2000 records per request. Require a numeric count and reject missing or short pages.
- Call the full-record hook before extracting IDs. Write each page before requesting the next. A missing PID uses its feature's unprefixed `OBJECTID` as the natural key and has `likely_vacant=false`.
- Keep mapped parcel writes and three-column temporary shape parts in one transaction. Union parts by natural key after all pages, use `bool_or` for vacancy, and commit only after the final shape update succeeds. An import error rolls back. Conflict updates never replace `parcels.id`.
- Add the `likely_vacant` column with a false default and county-vacancy index. Main uses one rate-limited GIS source, a 2000-row page size, and the existing ten-minute deadline.

## Verification
- Run `go test -race ./internal/service/ ./internal/parcelstore/ ./internal/connector/union/ -count=1` and `go test -race ./... -count=1`.
- Apply migrations with `make migrate-up`. Run the tagged parcelstore integration tests with `DATABASE_URL` against PostGIS. Check two parts across pages, one stable UUID after a second import, the union area, and the vacancy flag.
- Run the 4500-feature HTTP fixture. It must make one count request and three feature requests. Check the query parameters and confirm that null PID uses `OBJECTID`.
- Compare the prior default 500-row source setting with this import's explicit 2000-row setting. Do not claim a speed ratio because the parent has no parcel writer.
