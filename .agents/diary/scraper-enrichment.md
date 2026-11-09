# Scraper enrichment plan

## Out-of-scope decision

The operator dropped the devnetwedge crawl and the records request. The plan file is now `plans/tax-data-ingest.md`. The old path stays as a pointer because `plans/soil-layer.md` links it and this PR does not edit that file.

`CountyID` is `county.ID` from county-1. Another agent owns that plan and `plans/soil-layer.md`.

## Out-of-scope decision

`robots.txt` on `unionnc.devnetwedge.com` disallows `/parcel/*` and `/taxbill` for every user agent. The tax crawl, year loop, worker pool, and `parcel_tax_years` table stay undesigned. The fixture client does not dial that host.

`CountyID` moves to `internal/county` in county-1. Another agent owns that plan and `plans/soil-layer.md`. This plan only changes its dependency and its import.

## Out-of-scope decision

The soil plan still tells soil-3 to create `db/migrations/000002_soil.sql`. `000002_parcels_storage_srid.sql` is already on main. This plan takes the next free integer and does not edit the soil plan.

## Unrelated code smell

`internal/connector/shared/httpclient/http_client.go` still says the limiter should move into the shared package. The file is already there. The soil diary recorded the same comment.

`connector.BatchHook` takes no `context.Context`. The GIS fetch checks `ctx` in `FetchBatches` instead. Left it. The scraper uses `ParcelIDBatchHook`, which already takes a context.
