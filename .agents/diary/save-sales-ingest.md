# Save sales ingest

**Out-of-scope decision.** `SaveSales` still leaves out a sequence the caller did not send. `TestSaveSalesKeepsOmittedRow` locks that in for a one-slot update. `SavePart` deletes the missing sequences after the upsert, because the import sends the whole s1/s2/s3 set and an emptied slot has to disappear. A second public replace function would only forward those two calls.

**Out-of-scope decision.** Did not edit `.cursor/skills/verify-landman/`. That skill still describes a stream-only `./landman` that prints vacant IDs and does not open `DATABASE_URL`. Nothing in the skill seeds `parcel_sales`. Rewriting it to the current importer would replace the feature file. The import proof for this change is the throwaway PostGIS run, not that skill.

**Out-of-scope decision.** Did not change the 10-minute deadline, `RateLimit` 1, ArcGIS page size, `orderByFields`, or the count-mismatch failure. Did not edit `plans/`. No migration. `parcel_sales` already upserts on `(parcel_id, sequence)`.
