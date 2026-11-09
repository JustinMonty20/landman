# Spatialist removal and critical fixes

## Out-of-scope decision

Did not drop unused `github.com/jackc/pgx/v5` from `go.mod`. The plan classifies that tidy failure as a Medium finding and says not to treat it as a failure of these steps.

## Out-of-scope decision

Left the Spatialist mention in the ingestion-pipeline prose of `docs/ARCHITECTURE.md`. Step 2 only removes the `spatialist/` tree entries and leaves the rest of that document for the scraper plan.

## Out-of-scope decision

Did not revise `PLAN.md`. It still describes `SourceExtras.Union` and `models.UnionGISParcel`. That file is a completed historical plan, and this plan does not list it.

## Out-of-scope decision

Did not copy `s1_SALE_YEAR`, `s1_SALE_MTH`, or `s1_SALE_DAY` onto `UnionParcelExtras`. Step 4 removes those parts from `ParcelSale` and leaves `sale_date` and `og_sale_date`. The SQL columns are already gone.

## Out-of-scope decision

Did not treat a 200 feature response that lacks a `features` array as an error. Step 3 requires the status check on `fetchBatch` and the missing-`count` check on `queryTotalParcels`.

## Out-of-scope decision

Chose the JSON move for county extras. Several counties are planned, and no reader of `GISParcel` needs typed Union fields without importing package `union`. `ParcelSourceExtras` is an alias of `json.RawMessage` so the payload stays raw JSON. A defined type would marshal as base64. The field on `GISParcel` is `json.RawMessage`.
