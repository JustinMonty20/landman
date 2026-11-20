# Parcel publication after a failed import

## Cause and change

A parcel import reads the current evidence row for each parcel key. The query locks that row before the import can change it. In the retry test without the statistics fix, these lookups used 291 seconds of PostgreSQL execution time across the failed import and the retry. The retry reached the 600-second deadline.

A separate test prepared the lookup on an empty copy of the evidence table. After 2,000 inserts, PostgreSQL still used the saved sequential scan and checked all 2,000 rows for a missing key. `ANALYZE` on the key columns made PostgreSQL use the primary-key index. The importer now runs that command after the first 2,000 changed parcels. The patch also inserts the part and its evidence together and removes two unnecessary shape writes per changed parcel. The transaction, timeout, and request rate are unchanged.

## Results

The tests used the same 61 saved county responses on separate PostGIS 18/3.6 test databases. They did not contact the county. Each test import used the same ten-minute limit. The test served 118,202 features in 60 pages with a limit of one request per second.

| Build and test | First successful import | Unchanged repeat |
| --- | ---: | ---: |
| Original writer | 482.241 s | 304.967 s |
| Updated writer, empty database | 414.311 s | 288.077 s |
| Updated writer, after a late failure | 431.242 s | 281.018 s |

The late failure occurred before the transaction could write the request for downstream parcel work. The failed transaction did not leave any parcel, sale, owner label, or published evidence rows. The saved capture remained available. The successful retry published 117,983 parcels and 238,016 sales with zero quarantined features. The repeat preserved parcel and sale IDs and contents, including exact geometry bytes. Independent databases generate different UUIDs. Compare data between databases by parcel key and sale sequence. Both updated databases matched the original database's digests for parcel data, geometry, sales, current source evidence, and evidence history. All three had zero orphan sales.

The first import took about 60 seconds to capture the input in both builds. Verification and staging took about 86 seconds in the original writer and 75 seconds in the updated writer. Grouped publication through commit took about 336 seconds in the original writer and 279 seconds in the updated writer. These phase estimates use log timestamps and SQL samples collected about every two seconds. The first sample of the query that creates the parcel groups marks the transition from staging to publication.

On the unchanged repeat, capture took about 60 seconds in both builds. Verification and staging went from 88 to 75 seconds. Publication went from 157 to 153 seconds.

| SQL operation | Original: calls / seconds | Updated: calls / seconds |
| --- | ---: | ---: |
| Separate staged evidence update | 236,404 / 19.937 | 0 / 0 |
| Extra temporary shape insert | 117,983 / 0.702 | 0 / 0 |
| Duplicate parcel shape update | 117,983 / 11.642 | 0 / 0 |
| Current evidence lookup | 235,966 / 3.533 | 235,966 / 3.454 |
| Omitted-sale delete | 117,983 / 0.897 | 117,983 / 0.801 |
| Foreign-key check on `job_attempts` | 827,935 / 4.821 | 827,935 / 4.250 |

The table gives cumulative PostgreSQL execution times across the clean import and its repeat. It does not give elapsed times. Across the injected failure, retry, and repeat with the fix, 353,949 evidence lookups took 4.636 seconds. The two key statistics updates used 0.006 seconds. These measurements do not establish live performance.

## Test commands

Set `DATABASE_URL` to an empty disposable database with the repository migrations and `pg_stat_statements`. Set `PARCEL_EVIDENCE_DIR` to a private writable directory. Store the saved input in `artifacts/verify/ingestion-gaps-live/capture` or set `UNION_PARCEL_CAPTURE_DIR` to its relative path. Use a separate database for each test case.

```sh
UNION_PARCEL_CAPTURE_MODE=replay UNION_PARCEL_DISPOSABLE_DB=1 \
  go test -tags 'postgres ingestioncapture' ./cmd/landman \
    -run '^TestLiveParcelCaptureRepeatability$' -timeout 40m -count=1 -v
```

Set `UNION_PARCEL_FAIL_FIRST_PUBLICATION=1` for the failure and retry case. The 40-minute Go test limit permits multiple attempts. Each source attempt still has a ten-minute limit. For the original writer, use a Go overlay that replaces `internal/parcelstore/evidence.go`, `internal/parcelstore/parcels.go`, and `internal/parcelstore/staging.go` with the files from `4558c86`. Keep the test and importer logging the same.
