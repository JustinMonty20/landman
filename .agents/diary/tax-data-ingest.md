# Tax data ingest

## Out-of-scope decision

Did not edit `plans/scraper-enrichment.md`. On main it is still the per-parcel scraper plan, and it does not say tax-data-ingest replaced it. The copy at `74bc62a` turned that file into a one-line pointer. Restoring the pointer would delete the plan that main kept. The soil plan still links the full file.

## Out-of-scope decision

Did not add `parcels.likely_vacant` and did not recreate `likely_vacant_parcels`. persist-1 in `plans/soil-layer.md` owns the column. Tax rows only read it.

## Out-of-scope decision

Did not export the parcel lookup from `internal/parcelstore`. `SaveSales` already looks up `(county_id, source_name, source_parcel_id)` and omits `id`. The tax stores repeat that predicate. Editing `parcelstore` is outside the tax file boundaries.

## Operator correction

Tax rows match `parcels` and `parcel_sales`. Balances and bills are one row per parcel per tax year, with `county_id` and `source_name`. A foreclosure is a case plus a join row per PIN. A missing parcel keeps the case and skips the join. The `TBD` case key is the PIN-set hash plus the legal-text hash, and the operator still confirms it.

## Operator correction

OCR stays out of this program. A picture-only pipeline page is skipped and counted. The operator will deal with OCR when a file actually needs it.

## Operator correction

A renamed required column fails the file. The error names the missing column and the headers seen. Each CSV parser has an empty alias list. Adding an alias is a later code edit.

## Operator correction

The billing zip cap stays 256 MiB until tax-4 measures the file. Lane 7 and lane 9 numbers are written into the plan and the PR before that PR merges. A length over 128 MiB raises the cap in that same PR.

## Operator correction

The operator confirmed dropping the delinquent and billing write-time `likely_vacant` gate. Every matched real-estate row is stored. `tax_owed` still keeps only likely-vacant parcels with a balance.

## Operator correction

Upcoming foreclosure is not gated on `likely_vacant`. The auction-page list is a discovered file on the same periodic run. The live body on 2026-11-10 is a PDF text layer, stored as `tax_foreclosure_pipeline` rows joined on `parcels.id`. A boolean on `parcels` was rejected.


## Skipped refactor

Did not retitle `plans/soil-layer.md` migration notes. That plan already tells persist-1 and soil-3 to take the next goose prefix. This plan follows the same rule and does not edit the soil file.
