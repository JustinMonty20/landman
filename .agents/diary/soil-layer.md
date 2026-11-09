# Soil layer plan

## Out-of-scope decision

Ingest parallelism stays inside persist-1, soil-2, and soil-4. No new PR. filter-1 only supplies the vacant rule the server where copies. The county HTTP client stays at 1 request per second. The speedups are the vacant where, page size 2000, a page write overlapped with the next fetch, and a 4 worker join.

PostGIS chunk timing was skipped. `psql` and `docker` are not installed here, so 4 workers and 1000 ids per chunk are a conservative default. soil-4 fails a slow fixture at 30 seconds and a county join at 480 seconds.

## Unrelated bug

The 10 minute cancel in `main.go` still fires if the import pages all 118,202 parcels at batch size 50. The revised budget is 21 HTTP calls, about half a minute, so the plan keeps the cancel and does not lengthen it.

## Skipped refactor

`GISSourceConfig.BaseURL` is the full layer 215 query URL. A MapServer root plus layer ids would fit a second layer better. Left it. The parcel tests lock the full URL, and a third layer is the point where that split earns a change.

## Unrelated code smell

`internal/connector/shared/httpclient/http_client.go` still says the rate limiter should move into the shared package. The file is already there.

`Makefile` `build` compiles `./cmd/main.go`. `run` compiles `./main.go`.

## Out-of-scope decision

The join links soil polygons only to parcels persist-1 stored, which is the kept vacant set. A later paint of every soil polygon reads `soil_polygons` and does not need that join. Storing all 118,202 parcels so every touched parcel has a link is a different program.

Map painting, vector tiles, low-zoom simplification, and a map UI stay out. The plan only records a GiST index on `soil_polygons.shape`. `parcels.shape` already has `parcels_shape_gist_idx` in `000001_init_schema.sql`.

## Unrelated bug

`main` cancels after 10 minutes. Paging every parcel at batch size 50 would still miss that cancel. The parallelism revision above keeps the deadline and changes the import off that path. The earlier note that a live persist-1 run must hit the timeout described the 50 row path only.

## Scope tangent

Four kept PIDs have no positive-area soil overlap. They are `04162008`, `05180003B`, `06240001B`, and `04189007`. The join stores no row for them. The gap itself was not investigated.

The ArcGIS service list also names `OperationalLayersTest123`. This plan still reads `OperationalLayers` only.
