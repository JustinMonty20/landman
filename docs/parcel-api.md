# Parcel API reference

The Go API serves stored Union County evidence under `/api/v1`. Send a valid same-origin application session cookie with every GET request. Responses set `Cache-Control: no-store`. Missing or revoked sessions return 401. Database authentication failures return 503. There is no write API. Apply migrations through `000021_parcel_api_stamp.sql` before use. The application does not apply migrations at startup.

| Route | Response |
| --- | --- |
| `/api/v1/parcels/lookup?q=prefix` | Up to 20 parcel-number or address-prefix suggestions, independent of discovery. |
| `/api/v1/parcels?likely_vacant=true` | Parcel summaries, matching count, optional `next_cursor`, and source stamp. Without discovery filters, `items` is empty and `count` is zero. |
| `/api/v1/parcels/map?bbox=west,south,east,north&zoom=12&likely_vacant=true` | Neutral context and filtered match counts. Zoom below 13 returns 16 by 16 grid cells. Closer zoom returns up to 200 simplified geometries, or 413 instead of incomplete features. |
| `/api/v1/parcels/{id}` | Parcel summary, source identity, sale evidence, and location point. |
| `/api/v1/parcels/{id}/tax` | Active-file bill amounts, reported delinquent balances by year, pipeline entries, and foreclosure cases. Money is a decimal string. `reported_delinquent_total` is null without active balance evidence, not zero due or a payoff quote. |
| `/api/v1/parcels/{id}/soil` | Published soil assessment and its freshness, or `available: false` if no assessment exists. |
| `/api/v1/sources` | Parcel update time, active tax file identities, soil generations, latest job outcomes, and last clean imports. Artifact paths and raw errors stay private. |

Discovery accepts `min_acres`, `max_acres`, `likely_vacant`, and `delinquent`. Acreage uses `parcels.mapped_acres`. The vacancy flag is provisional evidence. `delinquent=false` means no positive active balance row, not proof of no debt. List requests also accept `bbox`, `page_size` from 1 to 100, and `cursor`. Map requests require ordered WGS84 `bbox` within Web Mercator latitude limits and integer `zoom` from 0 to 22. Unknown or duplicate parameters return 400. Bounds apply to full geometry before display simplification.

Each list or map response reads one database snapshot. A cursor binds to its filters and source stamp. A different stamp returns 400. The stamp is a refresh hint, not a retained snapshot between requests; separately fetched soil evidence has its own revision and freshness fields. The service caps serialized responses at 1 MiB and request work at eight seconds.

The `published_soil.soil_set_id` belongs to the returned screening run. The separate `current_soil_set_id` identifies the current geometry source. `geometry_set_changed` reports a difference. A failed refresh does not replace the run's source identity. A parcel repair does not replace the last clean full-import date.

Tax `fetched_at` values retain the stored file-observation time. The tax job reads operator-supplied files. These timestamps do not establish that the application downloaded a county file or that its balances remain current.

See [the measurement procedure](parcel-api-benchmark.md) and [the measured results](parcel-api-performance.md).
