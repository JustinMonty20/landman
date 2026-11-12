# Page Union soil polygons

## Source and scope

This unit implements the soil-2 section of [soil-layer.md](soil-layer.md). Query Union County layer 197 using the existing GIS source's rate-limited HTTP client. Count first, then fetch geometry and the five soil attributes in ordered 2,000-feature pages. Translate each page using `ParseSoilFeatures` and fail on HTTP, ArcGIS, malformed count, cancellation, or count mismatch. Keep layer 215 parcel behavior unchanged.

## Acceptance checks

- The default soil URL targets `/MapServer/197/query`; invalid configured URLs fail construction.
- An HTTP fixture shows count then two ordered pages at offsets 0 and 2000, even when `exceededTransferLimit` is true. Requested fields omit `HydrcRatng`.
- Status 500, ArcGIS errors, absent/non-numeric counts, short pages, and cancelled contexts return errors. Parcel package tests remain green.

## Exclusions

No soil persistence, parcel overlap, map UI, new HTTP client, or change to parcel paging. Full soil release still requires the later units and their live/performance review gates in the source plan.

## Verification

Run `gofmt` on changed Go files and `go test -race ./internal/connector/union/ -count=1`, then `go test -race ./... -count=1`.
