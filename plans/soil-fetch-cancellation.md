# Soil fetch cancellation after final page

## Scope
Fix `FetchSoilPolygons` to return `context.Canceled` and no polygons or map units when its context is canceled while the final page response completes, before returning success. Keep count validation, ordered 2,000-feature paging, and count mismatch behavior unchanged.

## Acceptance checks
- A regression test cancels as the final page response closes and verifies `errors.Is(err, context.Canceled)` and nil results.
- Existing soil paging and error tests remain green.

## Verification
Run `gofmt` on changed Go files, focused and full `go test -race` with `-count=1`, and `git diff --check`.

## Exclusions
No changes to parcel fetching, HTTP client behavior, soil translation, or other soil work.
