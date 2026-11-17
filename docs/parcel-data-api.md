# Parcel evidence API

All `/api/v1/` routes require the existing allowed-account browser session. Responses are read-only. The API neither fetches new evidence nor recomputes soil assessments.

`GET /api/v1/parcels` accepts `min_acres`, `max_acres`, `likely_vacant`, `delinquent`, `bbox`, `page_size` (1–100), and a returned `cursor`. It returns `items`, matching `count`, `next_cursor`, and `source_stamp`. Without a discovery filter, the count and items are empty. Cursors are tied to their filter and source stamp. A later request can reject a cursor after a source refresh, but a stamp is not a cross-request database snapshot.

`GET /api/v1/parcels/map` requires WGS84 `bbox=west,south,east,north` within Web Mercator latitude limits and `zoom` (0–22). It accepts discovery filters but not list page parameters. At zoom below 13, cells summarize the **neutral** viewport and filtered matches. At higher zoom, up to 200 context features have simplified geometry and a match flag. An over-limit map returns an error, not a partial map. `GET /api/v1/parcels/lookup?q=prefix` independently returns up to 20 parcel-number or address suggestions.

`GET /api/v1/parcels/{id}` returns source identity, parcel facts, location, and sales. `/tax` returns billed amounts separately from reported delinquent balances, a distinct delinquent year count, pipeline evidence, and foreclosure cases. Tax evidence carries the active source URL, SHA-256, and retrieval time. A null total means no balance evidence, not a confirmed zero or payoff quote. `/soil` reads the existing published assessment and freshness flags; a missing assessment is explicitly unavailable. Raw refresh errors are private. `GET /api/v1/sources` returns active tax file identities, published soil set and run, freshness deadline and failed-refresh timestamp, latest job outcomes, and last clean successes. Artifact storage keys and raw job errors are not included.

Geometry is mapped survey evidence, not site verification. `likely_vacant` is provisional. Query responses have a one-megabyte encoded limit. Requests run with an eight-second database deadline. No endpoint provides authoritative global publication snapshots.
