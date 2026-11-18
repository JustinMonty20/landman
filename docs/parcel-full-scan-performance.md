# Union parcel full-scan measurement

## Saved-capture comparison

The saved, private capture has 61 checksum-verified responses, 118,202 features, and 318,486,463 response bytes. Its `COMPLETE` digest is `80557c10588218c5872a7d5eb355b1875d59a5ec8722e91f7f6d97fcbb02cc8e`. Neither run requested the county or USDA. Both used fresh, migrated PostGIS 18/3.6 databases with `pg_stat_statements.track=all`, a local HTTP replay, and the unchanged ten-minute per-attempt deadline.

| Run | Empty first import | Identical second import | Feature, parcel, sale counts |
| --- | ---: | ---: | --- |
| PR #77 head `9c4b738` | 365.579 s | 222.066 s | 118,202; 117,983; 238,016 |
| Batched publication candidate | 330.478 s | 194.155 s | 118,202; 117,983; 238,016 |

Both sides completed 61 local HTTP responses per attempt without errors. The candidate preserved zero invalid parcel SRIDs, zero orphan sales, zero quarantined records, and zero differences in the complete parcel and sale snapshots after the identical import. The first-run difference of 35.101 s is **one paired observation**, not a demonstrated median speedup. The baseline already met the deadline in this controlled environment. The earlier 9m57s versus 6m figures compared a first import against a different, unchanged import and do not provide a speedup estimate.

The phase boundaries below come from five-second SQL sampling and the per-attempt test clock. They are approximate, not separately timed function calls.

| First-import phase | Baseline | Candidate | Measurement boundary |
| --- | ---: | ---: | --- |
| Local count and 60 page requests, durable byte capture, checksum and manifest verification | about 64 s | about 62 s | Test start to registered capture |
| Page decode, normalization and quarantine checks, part staging | about 66 s | about 61 s | Registered capture to first publication SQL |
| Complete-PID geometry, parcel and sale publication, history and transaction commit | about 236 s | about 207 s | First publication SQL to committed observation |

The HTTP transfer, `fsync`, and checksum subphases are not individually timed. Neither are Go decode and normalization. A separate instrumented profile is needed to isolate those costs. On both sides, the test's 61 responses contain the same 318,486,463 bytes, and the server validates their digests before delivery. The full import also verifies the separately saved application snapshot before publication.

Across the two measured attempts, `pg_stat_statements` reports raw-key lookup, active-attempt check, and quarantine UPDATE **235,966 calls each** on the baseline and **1,180 calls each** on the candidate. All three use the same 200-PID publication batch; the UPDATE matched zero quarantines on this capture. The nested `job_attempts` foreign-key check ran 709,952 times on each side, with 2.974 s baseline and 2.573 s candidate cumulative executor time. Omitted-sale DELETE ran 117,983 times on each side, with 0.663 s and 0.585 s cumulative executor time, and deleted zero rows in the saved capture. The synthetic omitted-sale plan used `parcel_sales_parcel_id_idx`, examined three sales, kept one by sequence, and deleted two in 0.078 ms before rollback. The existing 58 `ANALYZE parcels, parcel_sales` calls took 47.104 s and 43.600 s. Those timings are query executor time summed across both passes, not wall-clock phase time. On each side, `pg_stat_user_tables` recorded 117,983 parcel inserts and 117,983 parcel updates from first-import geometry publication, 238,016 sale inserts, and zero sale updates or deletes after both attempts. Thus the second attempt made no parcel or sale content writes.

## Verification scope and limits

The capture comparison tests empty migration state, complete source count, stable parcel and sale IDs, exact serving values and EWKB, geometry equivalence, unchanged reimport without content writes, and successful attempts. The 118,000-feature synthetic gate passed after a deliberately failed first import at PID 105000, full rollback, and successful retry. It verified 117,999 parcels, 353,995 sales, zero orphan sales, omitted-sale deletion, quarantine resolution, and an identical third import with zero serving-row differences. A focused 201-PID regression covers quarantine resolution across the publication-batch boundary. Selected checks include the backend Go CI equivalents and PostGIS tests. Browser UI lanes are not selected because this change does not touch browser behavior.

These are one unprofiled first-import pair and one unchanged-import pair on one host. Host load changed during the two runs, and warmed caches may contribute. The result proves that both measured full attempts met 600 seconds in this setup; it does not establish a population speedup or a worst-case production bound. Keep raw run, SQL, and load logs outside Git. No source capture, owner value, private evidence path, or database credential belongs in this document.
