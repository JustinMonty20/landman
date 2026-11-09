# Soil layer plan

## Skipped refactor

`GISSourceConfig.BaseURL` is the full layer 215 query URL. A MapServer root plus layer ids would fit a second layer better. Left it. The parcel tests lock the full URL, and a third layer is the point where that split earns a change.

## Unrelated code smell

`internal/connector/shared/httpclient/http_client.go` still says the rate limiter should move into the shared package. The file is already there.

`Makefile` `build` compiles `./cmd/main.go`. `run` compiles `./main.go`.

## Out-of-scope decision

The join links soil polygons only to parcels persist-1 stored, which is the kept vacant set. A later paint of every soil polygon reads `soil_polygons` and does not need that join. Storing all 118,202 parcels so every touched parcel has a link is a different program.

Map painting, vector tiles, low-zoom simplification, and a map UI stay out. The plan only records a GiST index on `soil_polygons.shape`. `parcels.shape` already has `parcels_shape_gist_idx` in `000001_init_schema.sql`.

## Unrelated bug

`main` cancels after 10 minutes. Paging 118,202 parcels at batch size 50 and one request per second is about 2,364 requests before transfer time. A live county run of persist-1 will hit that timeout. This plan does not change `main`'s timeout.

## Scope tangent

Four kept PIDs have no positive-area soil overlap. They are `04162008`, `05180003B`, `06240001B`, and `04189007`. The join stores no row for them. The gap itself was not investigated.

The ArcGIS service list also names `OperationalLayersTest123`. This plan still reads `OperationalLayers` only.
