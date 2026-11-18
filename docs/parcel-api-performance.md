# Parcel API performance results

## Measurement conditions

The comparison used two disposable copies of the same ingested Union County data. The original databases did not change.

- PostgreSQL 18.6 and PostGIS 3.6 ran on a machine with 20 logical CPUs.
- The dataset had 117,985 parcels, 11,250 soil polygons, and 117,985 current parcel assessments. Historical assessments totaled 825,891 rows.
- The tax job imported four existing offline files through the jobs runner. The copies had 116,021 bills, 917 delinquent year records, nine foreclosure parcel links, and 49 pipeline records.
- The baseline used the first working API and migration 19. The improved API used migrations 20 and 21.
- Five comparisons alternated the baseline and improved versions. Both versions used identical measurement code. Each route had five serial requests and five rounds of four concurrent requests per run.
- Timing included the TCP connection, session lookup, API work, and complete response-body read. Google login was outside timing.
- Independent SQL checked list identities, page order, distinct pages, map intersections and match counts, active tax evidence, and published soil identities. Repeated responses also had to match the checked first response.
- All ten runs passed. Timed requests returned zero errors. A separate dense-map test required HTTP 413 and did not count as a successful timed request.

These are repeated local requests without a cache reset. First-observed timings are not cold-cache timings. They do not establish production latency or a latency acceptance limit.

## Request results

Each value below is the median of five run-level p95 values with four concurrent requests. A p95 is the 95th percentile. Each run uses nearest rank, `ceil(0.95*n)-1`, in its sorted sample array. The ranges are the lowest and highest run-level p95 values. They are not pooled percentiles.

| Request | Baseline median and range, ms | Improved median and range, ms |
| --- | --- | --- |
| Source status | 240.580 [237.269, 285.156] | 23.142 [19.639, 31.909] |
| Matching prefix lookup | 2.103 [1.364, 2.635] | 3.342 [1.865, 3.871] |
| Missing prefix lookup | 213.817 [211.669, 218.934] | 1.808 [1.375, 2.167] |
| No active discovery filter | 120.328 [102.493, 132.316] | 11.310 [10.055, 12.545] |
| First vacancy page | 116.454 [106.080, 145.189] | 49.586 [36.648, 53.388] |
| Vacancy, acreage, no positive listed balance | 479.038 [448.711, 973.965] | 54.252 [42.626, 56.270] |
| Vacancy, acreage, positive listed balance | 532.103 [498.788, 602.479] | 117.731 [116.966, 130.470] |
| Regional map grid | 253.458 [248.075, 324.167] | 93.392 [86.408, 109.342] |
| Moved regional grid | 220.912 [216.040, 284.292] | 67.676 [63.374, 85.187] |
| Close map shapes | 137.322 [130.733, 164.200] | 14.776 [10.958, 18.051] |
| Parcel facts | 110.312 [104.245, 149.114] | 10.586 [8.648, 12.700] |
| Tax details | 120.304 [115.474, 135.704] | 17.422 [12.024, 18.073] |
| Soil assessment | 22.569 [20.858, 25.642] | 24.167 [21.792, 26.874] |
| Later vacancy page | 132.157 [119.004, 143.468] | 46.721 [39.905, 49.301] |
| County-wide map grid | 2106.985 [2065.590, 2127.412] | 793.464 [790.048, 798.663] |

No speed improvement was established for matching lookup or soil reads. The lookup now checks prefix existence before a sorted lookup. This adds work for a match but avoids an expensive scan for a missing prefix. The soil calculation and read module did not change.

## Query evidence

The baseline selective count took 280.903 ms in a diagnostic plan. Just-in-time compilation accounted for 245.412 ms. The API now disables that compilation inside its read-only transactions. This does not change the ingestion pool or global PostgreSQL settings.

The source stamp previously read the parcel heap to combine the maximum update time and row count. The API now reads the newest timestamp through a dedicated index and counts through the discovery index. Migrations also add prefix indexes and reverse foreclosure linkage. Prefix keys retain at most 80 characters. This prevents the new indexes from rejecting a long source address.

The map now calculates context and match counts in one spatial aggregate. Close maps no longer issue and discard a geometry query before reading match IDs. Original geometry still determines intersections. Display simplification does not determine evidence or filter results.

Prepared-plan probes compared custom and generic plans. Generic selective and broad filter probes still used sequential scans. The API lookup explicitly selects custom planning for its parameter-dependent prefix query. The recorded HTTP runs include the real pooled query behavior. Do not describe the diagnostic generic plans as the measured HTTP plans.

The county-wide grid remains the slowest measured route. Its diagnostic query took 380.935 ms and read all 117,985 parcels. The plan reported 40,810 shared read blocks and 16,874 shared hit blocks. It performed spatial selection, location transforms, and grouping without JIT or a temporary-file spill. These counters do not separate physical storage time from CPU time.

## Evidence and limits

The five-pair comparison used improved commit `73a07e2` and frozen baseline `0474cb9`. Later fixes separate the assessment's soil set from the current geometry set and bound the lookup index keys. Focused PostGIS regression tests and a later complete HTTP run cover those fixes. The later run passed with zero errors. Its county-wide map p95 was 791.681 ms with four concurrent requests. The list and map query implementations did not change after the paired comparison.

Raw logs, machine snapshots, query plans, and verifier receipts are local files under `artifacts/verify/parcel-data-api/`. They are excluded from Git. See [the measurement procedure](parcel-api-benchmark.md) to repeat the checks.

The original application database has not received these new migrations. The UI still uses its fictional dataset. This work does not establish browser drawing speed, production load limits, or completed V1 behavior. Agree interactive latency limits before UI acceptance. The measurements do not prove that the queries have optimal performance.
