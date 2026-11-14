# Full-county performance receipt

**Source.** All runs used the approved disposable `soil5_arena_cold_20261113_234401` PostGIS database. The normal executable retained its 600-second total limit. The source job processed 118,202 county records. The screening job processed 11,250 soil polygons. The `job_attempts` query below provides the individual job times. The plain logs `plans/soil5-arena-{cold,changed,repeat,locked-retry,bound-final}.log` record the county import progress. The request and heap trace is `plans/soil-5-full-trace.log.gz`. These files contain test-only runtime records, not application code.

**Completed runs.** Select `job_id`, `started_at`, `round(extract(epoch from (finished_at-started_at))::numeric,1)`, `outcome`, and `processed_count` from `job_attempts`. Filter to job IDs beginning with `union.`. Order by `started_at`. The disposable database returned these successful source/screening pairs, in time order:

| Case | Source seconds | Screening seconds | Sum seconds |
| --- | ---: | ---: | ---: |
| First import | 282.7 | 65.9 | 348.6 |
| Added parcel | 236.4 | 65.8 | 302.2 |
| Unchanged refresh | 227.7 | 15.5 | 243.2 |
| Added parcel with SHARE lock | 227.0 | 65.9 | 292.9 |
| Locked unchanged refresh with test-only trace | 226.8 | 16.0 | 242.8 |
| Final set-ID-bound executable, unchanged refresh | 236.5 | 16.4 | 252.9 |

One source job stopped after 24.3 seconds before screening when the county HTTP service did not return response headers. The next source and screening jobs passed. The final executable returned success under its 600-second limit. Its binary SHA-256 was `9bedbe00bb0b914aee2b5d654f63771721dbb5b29614b17e21ea26bf23a92939`. Its job-attempt records show source and screening success. The published run `f8bdad95-b0a1-46a2-8b15-60f1038b782b` uses soil set `d2edd3d1-e110-4fd7-804a-02f0d22133a7`. It has 117,985 parcel assessments, 196,834 map-unit area rows, and zero parcel-area reconciliation failures. The program did not write to the application database. `plans/soil-5-phased-explain.txt` records the separate rollback-only phased SQL plan and its zero-difference area check. Do not add the isolated SQL time to the complete-run time; the complete-run screening time already includes its database work.

**Requests and memory.** Decompress `plans/soil-5-full-trace.log.gz` and count lines matching `soil5_http source=gis bytes=N` or `soil5_http source=usda bytes=N`. The trace records 68 county responses with 362,291,002 body bytes, and 9 USDA responses with 231,048 body bytes. Read the second number in each Go `gctrace` `before->observed->after MB` group. The largest observed value is 95 MB. This is a rounded, garbage-collector-observed sample of allocated Go heap, not an exact peak. The compressed trace SHA-256 is `379d645664bc2e8e75a1f797a211fc16d3a12b53e7c0b2003e7b8e77d22d3e77`; its uncompressed SHA-256 is `4aae9570f27aab1da356d328665f0766dd93f6eb6df30355c0b17b74cf86251d`.

**Scope.** The first five runs precede the soil-set-ID concurrency fix. The final row uses the fixed executable and the normal deadline. It is an unchanged-data refresh. Earlier changed-data full-county runs and the rollback-only SQL plan cover the changed-data path. The test-only HTTP/heap trace also precedes the set-ID fix. Do not describe its sampled Go heap as an exact peak.
