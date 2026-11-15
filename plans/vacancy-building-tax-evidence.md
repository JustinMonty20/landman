# Parcel vacancy and tax evidence extension

## Objective

Extend the completed parcel and soil foundation with a separate, evidence-based likely-vacant tier. Use the official Union County structures layer and the assessed-building-value GIS layer to check independent parcel evidence. Show reported back taxes as a parcel-level total. Use the dated delinquent list to report back taxes. Do not ingest countywide payment history. A separate on-favorite check in [the tax plan](tax-data-ingest.md#on-favorite-parcel-balance-check) records one dated DevNet parcel observation for a buyer who saves a parcel. It does not affect vacancy.

The existing parcel import stores `parcels.likely_vacant` today. Keep it until the later tier implementation migrates its consumers. Import and verify the parcel, structures, and assessed-value layers first. Only after all three have clean, reconciled source generations, derive the tier. Then remove the stored boolean in a separate migration after dependent reads use `vacancy_tier`. Tax imports can finish separately; tax debt does not affect vacancy. Keep all parcels, including improved parcels, in storage. GIS layers cannot prove that land is truly vacant: recent construction, small structures, mapping gaps, and occupancy require a current on-site or official check. Label the result **likely vacant**, not **verified vacant**.

## Source priority and cost gate

Use public, no-fee sources first. Before import, record the source URL, public access terms, update date, fields, coverage, and file or service limits. Prefer official county GIS layers and published files. Use other public sources only when their terms permit this use. A page that responds to a normal request does not establish access terms. The narrow on-favorite DevNet request is an explicit operator decision; record the site's `robots.txt` restriction and fail visibly if access is refused.

For each end-user fact, keep the source, as-of date, and an unavailable or unknown state. Show the best available public evidence even when another useful field is absent. Do not purchase data or ask the county for a paid export as the first step. If a required fact is unavailable from permitted no-fee sources, record the precise missing field, why it matters, and what the public sources cannot show. Then present the county request, cost, and value for a separate decision. A paid source must not silently replace public evidence or erase its dates.

## Verified sources and limits (2026-11-14)

| Source | Location and observed data | Limit |
| --- | --- | --- |
| Parcel GIS | `https://atlas.unioncountync.gov/server/rest/services/OperationalLayers/MapServer/215` (`Parcels`). 118,202 features. Fields include `PID`, `YEARBLT`, `SQFT`, `FMV_IMPRV`, `FMV_LAND`, `FMV_TOTAL`, `STRUCTTYPE`, and `MULSTR`. | Parcel fields can disagree. `MULSTR` is not a verified structure count. Keep null distinct from zero. |
| Building GIS | `https://atlas.unioncountync.gov/server/rest/services/OperationalLayers/MapServer/171` (`Structures`). Public polygon query; 163,500 features at inspection. Fields include `OBJECTID`, `outline_id`, `PIN_Num`, `status`, and polygon geometry. Maximum page size: 2,000. Sample status is `existing`. | Confirm coverage, status values, update date, completeness, and stable ID before a full import. A footprint is not occupancy evidence. |
| Assessed-value GIS | `https://atlas.unioncountync.gov/server/rest/services/Property_Tax_Live/Assessed_Value/MapServer/0` exposes `parcel_number`, `parcel_year`, `assess_category`, `market_building`, `market_land`, and `market_total`. Layer 3 exposes prior- and current-year building-value changes. Spot checks returned `market_building = 283100` for `09336350` and a `709700` `AV` row for `01201017K`. | Spot checks returned duplicate `OBJECTID` rows, competing `AV` rows for `01201017K`, and `parcel_year = 2027` on 2026-11-15. Validate year, category, row identity, coverage, and currency before import. Assessed value is not proof of a current building or tax payment. |
| Billing archive | `Downloads/Billing Data.zip` contains `WKBILA-2026-07-22-1222.csv`, 138,405 rows for 2026; 116,421 `RE` and 21,984 `PP`. | `TOTAL TAXES AND CHARGES` is billed tax, not unpaid tax. No paid amount or payment date column was found. No separate building value in this CSV. |
| Delinquent list | `Downloads/Delinquent Tax List 2017-2025.csv` has 5,942 rows; 325 distinct `RE` parcel keys and 151 `RE` keys with more than one listed year. The `(1)` and `(2)` copies are duplicates. | Reported balance is dated evidence, not a current payoff. Do not combine duplicate files or different snapshots. |
| DevNet parcel view | The user-provided parcel page displays a year-by-year payment-history summary, with total due, total paid, unpaid amount, and date paid. For `09336350`, it shows 2026 unpaid `1359.48` and 2025 paid `1359.48`. | `robots.txt` disallows `/parcel/*`. The separate on-favorite check is limited to one saved parcel; it does not create a countywide scrape. These page values are not a verified bulk payment feed or complete countywide balance. |

A direct structures-layer query returned one `existing` polygon with `PIN_Num = 09336350`. It returned no polygons for `PIN_Num = 01003003`. These are spot checks, not a coverage or match-rate study. A missing match is not proof of no structure.

Payment-source check: the billing CSV header has `Date Due` but no payment, paid-amount, or balance field. The public Union GIS service catalogue and the exposed `Property_Tax_Live/Parcels`, `Property_Tax_Live/Personal_Property`, `PropertyTaxLive_May/Parcels`, and `ParcelCentroids` layer schemas showed no payment, paid-amount, balance, or delinquency fields. This is a check of those public endpoints, not proof that the Tax Collector has no export. The county downloadable GIS page returned HTTP 403 during this check, so its current file catalogue is not verified.

## Work and acceptance checks

### 1. Validate the building source

- [ ] Confirm county source terms, publication/update date, spatial coverage, refresh behavior, geometry reference, and whether layer 171 includes every relevant structure. Record all observed `status` values and their meanings. Do not include obsolete or proposed shapes as confirmed current buildings without a source definition.
- [ ] Measure distinct and missing `PIN_Num` values, duplicate `outline_id` values, and shape validity on a reproducible snapshot. Compare PIN matches with parcel `PID` after conservative key normalization. A structure can match multiple parcel shapes or cross a boundary; retain those conflicts.
- [ ] Compare spatial overlap against `PIN_Num` on samples of vacant-flag, improved, split-PID, null-PID, and boundary parcels. Compute overlap in a suitable projected reference, not Web Mercator square feet. A boundary touch with zero area is not an overlapping building. Keep positive-area slivers for review, not automatic classification.
- [ ] Record counts, dates, URLs, checksums or source-generation IDs, and query receipts. Do not claim complete county coverage until the count and imported features reconcile.

### 2. Validate and import assessed-building-value evidence
- [ ] Confirm source terms, publication and effective dates, assessment categories, coverage, and whether the sampled `parcel_year = 2027` is provisional. Match `parcel_number` to stable stored parcel IDs without losing leading zeros or suffixes.
- [ ] Investigate duplicate query rows and competing `AV` values for one parcel and year. Define a verified, deterministic selection rule or mark the assessment ambiguous. Do not sum duplicates or treat `PY` as the current appraisal.
- [ ] Import assessed values through a stable `jobs.Runner.Run` job ID. Use bounded, count-checked pages and atomic source-generation publication. Retain separate building, land, and total values, category, year, source identity, and retrieval time. A failed or partial import must not replace a clean generation.
- [ ] Compare samples with official parcel views and GIS layer 215. Record conflicts such as `FMV_IMPRV = 0` with positive assessed building value. Zero assessed value does not prove that no structure exists.

### 3. Import structure evidence without changing the soil plan

- [ ] Create a separate scoped implementation design for fetching layer 171 on the existing county rate limiter, with bounded pages, cancellation, retries, count reconciliation, and atomic generation publication. Do not insert or delete parcels in this import.
- [ ] Preserve each building feature's source ID, `PIN_Num`, status, geometry, and source generation. Link to stable `parcels.id` by verified PIN and geometry. Define deterministic handling for split PIDs and for a footprint that crosses two parcels. Keep unmatched records visible. Reimport must not duplicate current evidence, and a failed import must not publish a partial generation.
- [ ] Retain raw source evidence and source dates. Recompute affected parcel results when either parcel geometry or structure evidence changes. Do not change private lead records.

### 4. Derive a vacancy tier

Do not publish a new tier from one partial import. Wait for clean, reconciled parcel, structures, and assessed-value source generations. If any required layer fails or has unknown coverage, keep the prior dated result or show unknown; never promote a parcel because a building import is incomplete.

- [ ] During migration, retain numeric `YEARBLT = 0 OR SQFT = 0` as an input fact, not as a final decision or a second stored vacancy status. Replace reads of `parcels.likely_vacant` before dropping that column. Keep independent parcel `FMV_IMPRV`, `FMV_LAND`, `STRUCTTYPE`, and verified structure facts as evidence; do not assume that `FMV_IMPRV = 0` means no house. The billing file does not supply building value or structure count.
- [ ] Define **strong candidate** only with validated layer coverage, no current overlapping footprint, parcel fields that do not report a structure, and no selected positive assessed building value. Define **review** for incomplete independent evidence. Define **conflict** when footprint, parcel facts, and assessed value disagree. Define **unknown** for missing coverage, ambiguous assessed-value rows, unsafe identity, or missing necessary inputs. A positive footprint can be a shed or garage; do not equate it with an occupied house.
- [ ] Store a parcel-level `vacancy_tier` with constrained values `strong_candidate`, `review`, `conflict`, and `unknown`, keyed by stable `parcels.id` in a derived assessment. Store the rule version, all three source generations, contributing facts, and reason for each result. Do not call this `is_vacant` or claim verified vacancy. Keep tax debt, a leading `0` in an address, and land-use labels out of the vacancy rule. Show conflict and unknown records in the UI instead of forcing them to occupied or vacant.
- [ ] Test no-footprint with verified coverage, unverified coverage, a split parcel, boundary-only touch, a cross-boundary footprint, accessory structure, a positive assessed value without footprint, competing `AV` rows, a future or provisional assessment year, a zero-value improved parcel, and stale source generations. Test refresh after a new building appears or assessed value changes.

### 5. Replace the old vacancy column and its consumers
- [ ] Update the parcel importer to stop writing `parcels.likely_vacant`. Keep the source `YEARBLT` and `SQFT` facts for assessment. Migrate all parcel filters, query views, fixtures, and tests that read the boolean. The planned `tax_owed` view in [the tax plan](tax-data-ingest.md) is one such consumer: keep delinquent rows for every matched real-estate parcel, and define its buyer-facing candidate filter with `vacancy_tier`, not the old boolean. Do not use tax debt to calculate the tier.
- [ ] Backfill `vacancy_tier` for every stored parcel from validated source generations before dropping `parcels.likely_vacant`. Preserve the prior dated result or show `unknown` when required evidence is absent. Use a new Goose migration; do not edit applied migrations or drop the boolean while a reader still uses it.
- [ ] Verify a clean repeat import, an import failure, the backfill, the `tax_owed` read path, and the column removal on a migrated database. No parcel UUID, source geometry, tax record, or private lead record changes because of the transition.

### 6. Show reported back-tax totals

- [ ] Join the single current delinquent-list version on normalized `Property Key` and `Primary Source = RE`. Keep each tax year and `Balance Due`. Sum listed years once per parcel as `reported_back_tax_total` and count distinct years. Show source publication span, retrieval date, and an **unknown/not listed** state; missing data is not a verified zero or a current payoff amount.
- [ ] Keep `TOTAL TAXES AND CHARGES` from the billing archive in a separate `current_year_tax_billed` field. Do not add it to back-tax balance. Reconcile tax-unit scope before making a claim that a billed total is a payable total; parcel `09336350` has CSV total `2737.12` and a DevNet display of `1359.48`, a difference of `1377.64` equal to its CSV City of Monroe line.
- [ ] Do not ingest countywide payment history. Keep the on-favorite DevNet observation separate, with its observation time and access-failure state. The delinquent file reports a balance at publication, not a live amount due. Show its source date and a route to confirm the present amount with the Tax Collector. Do not derive a current balance from the billing file.

### 7. Integrate and verify

- [ ] Show `vacancy_tier`, reasons, footprint and assessed-building-value evidence, source year and dates, back-tax total, year details, current-year tax billed, and unknown/conflict states in parcel discovery and details. Do not call a reported balance a current payoff quote.
- [ ] Run full imports of the parcel, structures, and assessed-value GIS layers and repeat them. Confirm stable parcel IDs, no duplicate footprints or back-tax years, no partial results after failure, count reconciliation, and correct refresh when parcel or building data changes. Confirm that tier calculation starts only after all three GIS generations pass validation. Compare a small manual sample with official parcel views only as human verification, not automated collection.

## Open source decisions

- Does layer 171 cover all buildings, and what do its `status` values mean? The public endpoint and field names are verified; completeness is not.
- What is the safe parcel-link rule when `PIN_Num` and geometric overlap disagree?
- Which assessed-value category and year identify a final current value, and how should duplicate or competing `AV` rows be resolved?
- Is a newer delinquent-list publication available at import time? Use one verified version at a time; do not present an old list as the present balance.

Do not start implementation based on an assumption about any open source decision. Keep this extension separate from [soil-layer.md](soil-layer.md) and coordinate its tax fields with [tax-data-ingest.md](tax-data-ingest.md).
