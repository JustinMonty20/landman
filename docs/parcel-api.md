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

## Union County map overlays

- `GET /api/v1/qualification/map?layer=zoning&overview=true` returns larger zoning areas and simplified jurisdiction boundaries from the published zoning generation.
- `GET /api/v1/qualification/map?layer=roads&overview=true` returns simplified centerlines for roads with a source class of Primary, Highway, or Expressway from the published roads generation.
- `GET /api/v1/flood/map?overview=true` returns simplified polygons grouped by zone code, subtype, and special flood hazard designation from the published FEMA release.

The zoning map also returns named point features for municipalities and extraterritorial jurisdictions (ETJs). The roads map returns up to 30 named point features for main roads. A label point is for display, not measurement.

`GET /api/v1/qualification/zoning?lon=-80.595&lat=34.995` returns the active zoning generation and the distinct stored districts at the selected point. It uses full stored polygons, not map shapes. Each district has a jurisdiction and the stored code, description, case reference, and source record date. Fields the source did not supply are empty strings. More than one district can be present. The result does not establish permitted use.

Overview requests do not accept `bbox` or `zoom`. The response sets `overview: true`. The shapes are for map display only. Blank areas do not establish that zoning or flood data is absent or that a road does not exist. For more detail, omit `overview` and send `bbox` and `zoom`. The existing feature and response limits apply. The UI uses the county overview if a detail request exceeds a display limit.
