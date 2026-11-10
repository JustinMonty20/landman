# Tax data ingest

## Out-of-scope decision

Did not edit `plans/scraper-enrichment.md`. On main it is still the per-parcel scraper plan, and it does not say tax-data-ingest replaced it. The copy at `74bc62a` turned that file into a one-line pointer. Restoring the pointer would delete the plan that main kept. The soil plan still links the full file.

## Out-of-scope decision

Did not add `parcels.likely_vacant` and did not recreate `likely_vacant_parcels`. persist-1 in `plans/soil-layer.md` owns the column. Tax rows only read it.

## Out-of-scope decision

Did not export the parcel lookup from `internal/parcelstore`. `SaveSales` already looks up `(county_id, source_name, source_parcel_id)` and omits `id`. The tax stores repeat that predicate. Editing `parcelstore` is outside the tax file boundaries.

## Operator correction

Tax rows match `parcels` and `parcel_sales`. Balances and bills are one row per parcel per tax year, with `county_id` and `source_name`. A foreclosure is a case plus a join row per PIN. A missing parcel keeps the case and skips the join. The `TBD` case key is the PIN-set hash plus the legal-text hash, and the operator still confirms it.

## Skipped refactor

Did not retitle `plans/soil-layer.md` migration notes. That plan already tells persist-1 and soil-3 to take the next goose prefix. This plan follows the same rule and does not edit the soil file.
