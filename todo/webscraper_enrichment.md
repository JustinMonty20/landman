# Plan

Add a new web-scraping enricher that fits the existing batch enrichment flow and DI/testing conventions already used in `internal/service`. The implementation will isolate external effects (HTTP/time/parsing) behind injected seams, return typed flattened output, and include comprehensive stdlib-only unit tests before wiring into the Union connector surface.

## Scope
- In: New Union web-scraping source + enricher path, typed transform output, DI-friendly constructors/interfaces, unit tests, and integration into Union exports/runtime wiring.
- Out: Database persistence changes, translator/merger behavior changes, non-Union county connectors, and UI/API contract redesign.

## Action items
[ ] Define the target contract for the scraper output (typed structs only) and the required retained fields, including `og_` string fields for parsed values where applicable.
[ ] Add a new connector package (for example `internal/connector/union/<websource>/`) implementing `connector.SingleParcelFetcher` with constructor injection for HTTP doer, clock (`func() time.Time`), and parser/transform function.
[ ] Implement request building, scraping fetch logic, status/error handling, and deterministic `connector.RawRecord` construction without introducing new global dependencies.
[ ] Implement a typed flatten/transform layer for scraped content (no `map[string]interface{}` for retained output fields) and keep parsing logic isolated for testability.
[ ] Add an enricher service wrapper that reads batch raw records, applies the transform, and aggregates partial failures consistently.
[ ] Wire the new source/enricher into Union’s public surface (`internal/connector/union/exports.go`) and runtime composition (`main.go`, and `register.go` only if this source should auto-register).
[ ] Add stdlib-only unit tests using `httptest` and handwritten fakes to cover constructor validation, scrape success/error paths, parser edge cases, and enricher aggregation behavior.
[ ] Validate end-to-end by running `go test ./...` and `make test`, then document source configuration defaults and operational constraints (rate limits, selectors, failure modes).

## Open questions
- Which exact website and URL pattern should be scraped (and is it per-parcel lookup, search flow, or paginated listing)?
- What specific fields must be extracted and retained in the typed output?
- Does the target site require auth/session/cookies/anti-bot handling beyond basic HTTP requests?
