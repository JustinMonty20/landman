# Tax data ingest

## Out-of-scope decision

Did not edit `plans/scraper-enrichment.md`. On main it is still the per-parcel scraper plan, and it does not say tax-data-ingest replaced it. The copy at `74bc62a` turned that file into a one-line pointer. Restoring the pointer would delete the plan that main kept. The soil plan still links the full file.

## Out-of-scope decision

Did not add `parcels.likely_vacant` and did not recreate `likely_vacant_parcels`. persist-1 in `plans/soil-layer.md` owns the column. Tax rows only read it.

## Out-of-scope decision

Did not export the parcel lookup from `internal/parcelstore`. `SaveSales` already looks up `(county_id, source_name, source_parcel_id)` and omits `id`. The tax stores repeat that predicate. Editing `parcelstore` is outside the tax file boundaries.

## Operator correction

Tax rows match `parcels` and `parcel_sales`. Balances and bills are one row per parcel per tax year, with `county_id` and `source_name`. A foreclosure is a case plus a join row per PIN. A missing parcel keeps the case and skips the join. The `TBD` case key is the PIN-set hash plus the legal-text hash.

## Operator correction

OCR stays out of this program. A picture-only pipeline page is skipped and counted. The operator will deal with OCR when a file actually needs it.

## Operator correction

A renamed required column fails the file. The error names the missing column and the headers seen. Each CSV parser has an empty alias list. Adding an alias is a later code edit.

## Operator correction

The billing zip cap stays 256 MiB until tax-4 measures the file. Lane 7 and lane 9 numbers are written into the plan and the PR before that PR merges. A length over 128 MiB raises the cap in that same PR.

## Operator correction

The operator confirmed the null case-number key `(county_id, source_name, parcel_set_sha256, legal_sha256)`. A later file that assigns a real case number leaves the old `TBD` row.

## Out-of-scope decision

A later pass that matches a `TBD` row to a numbered case by legal description and parcel set, then removes the stale row, stays out of this plan.

## Operator correction

The operator confirmed dropping the delinquent and billing write-time `likely_vacant` gate. Every matched real-estate row is stored. `tax_owed` still keeps only likely-vacant parcels with a balance.

## Operator correction

Upcoming foreclosure is not gated on `likely_vacant`. The auction-page list is a discovered file on the same periodic run. The live body on 2026-11-10 is a PDF text layer, stored as `tax_foreclosure_pipeline` rows joined on `parcels.id`. A boolean on `parcels` was rejected.


## Skipped refactor

Did not retitle `plans/soil-layer.md` migration notes. That plan already tells persist-1 and soil-3 to take the next goose prefix. This plan follows the same rule and does not edit the soil file.

## Out-of-scope decision

The operator selected one scheduled `union.tax` runner job. Keep the four ordered file reads and each file's freshness check in that job. Do not add four schedules or bypass the runner.

## Scope tangent

A shallow file scan missed `internal/connector/shared/httpclient/`. The full Go test run confirmed that the package exists and passes. No HTTP limiter change belongs in the tax source work.

## Out-of-scope decision

The operator approved isolated local verification lanes instead of separate cloud VMs. Keep the ten-lane behavior checks and use disposable databases. Do not claim that local lanes ran in the cloud.

## Scope tangent

The tax-2 verification provider returned HTTP 402 after one behavior lane and the static gate passed. The operator approved `openai-codex/gpt-6.1-sol` with high thinking for the remaining lanes and audits. Keep the two completed receipts and do not call stopped lanes project failures.

[Out-of-scope decision] Keep tax-3 through tax-7 adapters out of the artifact-store change. The operator chose to retain file bytes before any adapter parses or publishes them, so this change only adds the shared store, import evidence, and tests.

[Out-of-scope decision] Use only local disk for current single-instance tax runs. Keep the S3 backend inactive until the operator selects cloud infrastructure; local disk does not give shared durability.

[Out-of-scope decision] The operator approved a verified artifact field in shared `taxsource.Meta` for tax-3. Use this value instead of a separate method on each Union source; the `Read` signature and tax table schemas stay unchanged.

[Out-of-scope decision] The one approved county CSV request returned Akamai HTTP 403. Do not retry or bypass the denial; use the operator's saved CSV only for offline parsing and timing, and leave the live-fetch gate blocked.

[Out-of-scope decision] One plain Go HTTP request each to the other three known county tax document URLs also returned 403. The operator chose manual local input for all four source files now; do not add automatic county fetches to the tax run.

[Scope tangent] The operator wants a later bucket-upload action to start ingestion after cloud infrastructure exists. Do not add a cloud event adapter or assume a provider and runtime before the operator selects them.

[Out-of-scope decision] The earlier scheduled `union.tax` choice above is superseded for the first single-instance release. Four operator-supplied disk files now enter one manual `jobs.Runner.Run` call; keep parcel and soil calendars independent. Do not treat the four Akamai 403 replies as permission to bypass a denial.

[Out-of-scope decision] Stage the two files initially available in private, Git-ignored `artifacts/tax-input/`, not in tracked full-file fixtures. At that time the two foreclosure files were not in Downloads; do not substitute old cached documents.

[Out-of-scope decision] Later on 2026-11-16 the operator downloaded both foreclosure files. The sales source is a 10-page PDF (`b8406ff8dcdd927bbb681c21b5a97be8ca1c957ac665969e7d054614cd537a68`); the pipeline source is a 27-page PDF (`b0f8926a4d9d893dc022c0225649d9752f2c905ecb8b0ffcf4813d845e5b801b`). Both now reside in the private ignored input directory; no full county document belongs in Git.

[Skipped refactor] Billing needs the same parcel-key normalization as delinquent. Reuse `normalizeDelinquentKey` in tax-4 rather than rename tax-3 code outside this unit; review the name when tax-5 and tax-7 share the rule.
