# Flood parcel API reference

The flood filter finds Union County parcels that intersect a selected FEMA map category. It uses the full stored parcel and flood polygons. The flood overlay returns simplified display geometry. Do not use the display geometry to calculate parcel matches.

## Access

Send an application session cookie to the configured application origin. The origin includes the scheme, host, and port. These routes require an approved account and a valid session. See [application sign-in](application-login.md). There is no separate API token or service account flow. All routes below use `GET` and return `Cache-Control: no-store`.

## Category values

Send one `flood_category` value per request. Use the value, not the display label.

| Value | FEMA map category |
| --- | --- |
| `floodway` | Regulatory floodway. |
| `sfha` | Special flood hazard area outside the `floodway` category. |
| `moderate` | Zone X, 0.2 percent annual chance. |
| `minimal` | Zone X, minimal flood hazard. |
| `unclassified` | A mapped feature that does not meet the other category rules. |

`sfha` and `floodway` are separate categories in this interface. To include both, request both categories separately and remove duplicate parcel IDs. `unclassified` is not the same as missing coverage. An area without mapped data does not establish an absence of flood risk.

## Parcel routes

| Route | Example query | Result |
| --- | --- | --- |
| `/api/v1/parcels` | `flood_category=moderate&page_size=25` | A page of parcel summaries in `items`, the full match `count`, `source_stamp`, and `next_cursor` if more results exist. Omit `bbox` for the county count. |
| `/api/v1/parcels/map` | `flood_category=moderate&bbox=-80.65,34.90,-80.40,35.10&zoom=12` | Grid cells at zoom below 13. Parcel features at zoom 13 or higher. `context_count` counts the response's parcel context. `match_count` counts matches in the specified view. The result includes `source_stamp`. |

Both routes accept `flood_contact` with these values:

| Value | Match rule |
| --- | --- |
| `area` | Default. Match any overlap with positive area. Full containment and partial overlap both count. |
| `intersects` | Match positive area, contact at a shared edge, or contact at a shared point. |

`flood_contact` requires `flood_category`. Flood filters combine with other parcel filters with logical AND. Each parcel appears once, even if more than one flood polygon matches. A parcel without usable geometry does not match. A zero count means no parcel met the selected test. It does not establish the absence of a flood hazard.

Use the same filters and bounds to compare a list with a map. Keep the same filter values when you send `next_cursor` as `cursor`. If a cursor becomes invalid after a source refresh, start again without the cursor. Separately requested pages and maps do not share a retained database snapshot.

## Flood display route

`GET /api/v1/flood/map?overview=true&flood_category=moderate` returns a GeoJSON `FeatureCollection` for the selected category. Each feature carries `properties.category` and the original FEMA `code`, `subtype`, and `sfha` values. The response includes the active `release` and an `overview` flag. Omit `flood_category` to request all categories. Use `bbox` and `zoom` instead of `overview=true` to request detail. This route does not accept `flood_contact`.

## Response limits and errors

The API limits serialized responses to 1 MiB. It also limits feature counts. A `413` response means the result exceeds a limit. For a detailed map, send smaller bounds or request the county flood overview. The API does not substitute a partial feature set.

| Status | Meaning |
| --- | --- |
| `400` | An invalid or repeated query parameter, or an invalid or stale cursor. |
| `401` | A missing, expired, or invalid application session. |
| `413` | The result exceeds a display or response limit. |
| `503` | The API cannot read the required data. Do not treat this response as a zero match count. |

See the [parcel API reference](parcel-api.md) for other parcel fields, filters, and bounds rules.
