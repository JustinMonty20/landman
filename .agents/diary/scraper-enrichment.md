# Scraper enrichment plan

## Out-of-scope decision

The soil plan still tells soil-3 to create `db/migrations/000002_soil.sql`. `000002_parcels_storage_srid.sql` is already on main. This plan takes the next free integer and does not edit the soil plan.

## Unrelated code smell

`internal/connector/shared/httpclient/http_client.go` still says the limiter should move into the shared package. The file is already there. The soil diary recorded the same comment.

`connector.BatchHook` takes no `context.Context`. The GIS fetch checks `ctx` in `FetchBatches` instead. Left it. The scraper uses `ParcelIDBatchHook`, which already takes a context.
