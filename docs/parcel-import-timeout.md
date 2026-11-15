# Parcel import timeout

## Cause

The sales import added a parcel lookup for every source feature. `ShapeWriter.SavePart` had already received that parcel ID from `SaveParcel`.

On the unchanged executable, 118,202 lookups used 177.69 seconds of PostgreSQL execution time. A separate query-plan test used the same parcel schema. PostgreSQL retained a plan chosen for a small table. After 60,000 inserts, that plan discarded 60,035 rows to find one parcel. A new plan used the full natural-key index.

The test reproduces a poor cached plan. It does not capture the exact plan inside the executable's active connection.

## Change

`SavePart` now passes the stored parcel ID to a private sales writer. The public `SaveSales` function still validates and looks up a parcel by its natural key.

Sale slots, generated IDs, dates, omitted-slot deletion, and transaction boundaries remain unchanged. No source rate limit or timeout changes.

The verification helper no longer treats batch log gaps as county request time. Each gap includes parcel translation and database writes. A slow timed HTTP probe remains evidence of a slow county page.

## Parcel publication

A parcel can have geometry parts on different pages. The import stages those parts and publishes their combined shape in one transaction. Cancellation preserves the previously published snapshot. A failed first import can leave zero parcels.

Page commits would change that guarantee. They also would not create resume support. Persistent staging and restart support require a separate design.

## Verification

The local evidence is in `artifacts/verify/parcel-timeout/`. That directory is not committed.

The unchanged executable completed one seeded run. Its three jobs used 553.57 seconds in total. The process monitor recorded an upper bound of 562.02 seconds. The monitor sampled process completion every ten seconds.

The fixed executable completed one empty-schema run in 309.47 seconds. It processed 118,202 source features and stored 117,983 parcels, 238,016 sale slots, and 11,250 soil polygons. All three jobs succeeded. The published result had 117,983 parcel assessments and zero acreage reconciliation failures.

Stored parcel values, geometry bytes, and sale values matched the unchanged run. Database counters showed zero repeated natural-key parcel lookups during the fixed import.

These are individual runs with different initial data. They establish completion under the unchanged 600-second deadline. They are not a controlled speedup measurement.

The simple-shape benchmark was rejected. Its geometry update dominated the measurement, and it did not reproduce the slow lookup. No speedup claim uses that benchmark.

The tests cover cancellation, sale-slot replacement, ignored caller IDs, and multipart geometry. The verification helper tests fail before the batch-gap correction and pass after it.
