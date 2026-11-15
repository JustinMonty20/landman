# Parcel import timeout

## Cause

The sales import added a parcel lookup for every source feature. `ShapeWriter.SavePart` had already received that parcel ID from `SaveParcel`. The parcel natural key contains the county, source name, and source parcel ID.

On the unchanged executable, 118,202 lookups used 177.69 seconds of PostgreSQL execution time. A separate query-plan test used the same parcel schema. PostgreSQL retained a plan chosen for a small table. After 60,000 inserts, that plan discarded 60,035 rows to find one parcel. A new plan used the full natural-key index.

The test reproduces a poor cached plan. It does not capture the exact plan inside the executable's active connection.

Removing that lookup was not sufficient. A later retry still reached the 600-second deadline. Foreign-key checks and omitted-sale deletes consumed most database time. A foreign key requires the referenced database row to exist. A failed import can leave zero published rows while table pages remain. That retry state must also pass.

## Change

`SavePart` now passes the stored parcel ID to a private sales writer. The public `SaveSales` function still validates and looks up a parcel by its natural key.

After every 2,000 saved parts, the writer runs `ANALYZE parcels, parcel_sales` inside its existing transaction. This updates the estimates used for cached query plans. No page is committed. The import role must own both tables or have PostgreSQL permission to analyze them.

Sale slots, generated IDs, dates, omitted-slot deletion, and transaction boundaries remain unchanged. No source rate limit or timeout changes.

The verification helper no longer treats batch log gaps as county request time. Each gap includes parcel translation and database writes. A slow timed HTTP probe remains evidence of a slow county page.

## Parcel publication

A parcel can have geometry parts on different pages. The import stages those parts and publishes their combined shape in one transaction. Cancellation preserves the previously published snapshot. A failed first import can leave zero parcels.

Page commits would change that guarantee. They also would not create resume support. Persistent staging and restart support require a separate design.

## Verification

The local evidence is in `artifacts/verify/parcel-timeout/`. That directory is not committed.

The unchanged executable completed one seeded run. Its three jobs used 553.57 seconds in total. The process monitor recorded an upper bound of 562.02 seconds. The monitor sampled process completion every ten seconds.

The ID-only change completed one empty-schema run in 309.47 seconds. It processed 118,202 source features and stored 117,983 parcels, 238,016 sale slots, and 11,250 soil polygons. All three jobs succeeded. The published result had 117,983 parcel assessments and zero acreage reconciliation failures.

Stored parcel values, geometry bytes, and sale values matched the unchanged run. Database counters showed zero repeated natural-key parcel lookups during the fixed import.

These are individual runs with different initial data. They are not a controlled speedup measurement. They did not prove the retry case. The ID-only change later failed that case at 600.04 seconds.

The simple-shape benchmark was rejected. Its geometry update dominated the measurement, and it did not reproduce the slow lookup. No speedup claim uses that benchmark.

The final statistics change completed the failed-import retry on current `main` in 438.02 seconds. A new database migrated through version 15 completed in 438.12 seconds. All three jobs succeeded in both runs. Stored parcel values, geometry bytes, and sale values matched the unchanged run. Each published result covered all 117,983 parcels and had zero acreage reconciliation failures.

The tests cover cancellation after a statistics update, sale-slot replacement, ignored caller IDs, and multipart geometry across the statistics boundary. The new statistics test uses isolated empty tables. It fails without the statistics update. It also checks that no parcel is visible before the full scan commits. The verification helper tests fail before the batch-gap correction and pass after it. `make test`, the full database-tagged race tests, `go vet ./...`, and `go build ./...` pass on the final code.
