# Parcel API reference

The Go API serves stored Union County evidence under `/api/v1`. Send a valid same-origin application session cookie with every GET request. Responses set `Cache-Control: no-store`. Missing or revoked sessions return 401. Database authentication failures return 503. There is no write API.

| Route | Response |
| --- | --- |
| `/api/v1/parcels/lookup?q=prefix` | Up to 20 parcel-number or address-prefix suggestions, independent of discovery. |
| `/api/v1/parcels?likely_vacant=true` | Parcel summaries, matching count, optional `next_cursor`, and source stamp. Without discovery filters, `items` is empty and `count` is zero. |
| `/api/v1/parcels/map?bbox=west,south,east,north&zoom=12&likely_vacant=true` | Neutral context and filtered match counts. Zoom below 13 returns 16 by 16 grid cells. Closer zoom returns up to 200 simplified geometries, or 413 instead of incomplete features. |
| `/api/v1/parcels/{id}` | Parcel summary, source identity, sale evidence, and location point. |
| `/api/v1/parcels/{id}/tax` | Active-file bill amounts, reported delinquent balances by year, pipeline entries, and foreclosure cases. Money is a decimal string. An empty array means no active evidence, not zero due or a payoff quote. |
| `/api/v1/parcels/{id}/soil` | Published soil assessment and its freshness, or `available: false` if no assessment exists. |
| `/api/v1/sources` | Parcel update time, active tax file URLs, hashes, retrieval dates, and latest job outcomes. Artifact paths stay private. |

Discovery accepts `min_acres`, `max_acres`, `likely_vacant`, and `delinquent`. Acreage uses `parcels.mapped_acres`. The vacancy flag is provisional evidence. List requests also accept `bbox`, `page_size` from 1 to 100, and `cursor`. Map requests require ordered WGS84 `bbox` and integer `zoom` from 0 to 22. Unknown or duplicate parameters return 400. Bounds apply to full geometry before display simplification.

Each list or map response reads one database snapshot. A cursor binds to its filters and source stamp. A different stamp returns 400. The stamp is a refresh hint, not a retained snapshot between requests; separately fetched soil evidence has its own revision and freshness fields. The service caps serialized responses at 1 MiB and request work at eight seconds.
