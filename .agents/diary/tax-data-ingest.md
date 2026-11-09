# Tax data ingest

## Out-of-scope decision

The operator dropped the devnetwedge site and the records request. The plan is a fetch of the delinquent CSV and the billing zip. A pointer stays at `plans/scraper-enrichment.md` because the soil plan links that path and this PR does not edit the soil plan.

`county.ID` stays in county-1. This plan does not declare it.

## Out-of-scope decision

The soil plan still tells soil-3 to create `db/migrations/000002_soil.sql`. `000002_parcels_storage_srid.sql` is already on main. The tax migration takes the next free integer.
