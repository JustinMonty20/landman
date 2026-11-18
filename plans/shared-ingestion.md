# Shared ingestion for parcels, tax, and soil

## Scope

Use `ingest.Engine` for verified capture registration and transactional publication. Use the same artifact store contract for local disk and S3. Keep source records typed and keep source-specific publication rules in adapters. `jobs.Runner` owns attempt records, exclusion, and outcomes. Do not change zoning or floodplain ingestion.

This is a forward-only cutover. Replay accepts captures from the shared registry. Do not convert legacy captures, add a fallback, or delete saved input files. Keep existing business data and applied migrations.

## Publication units

- **Parcels.** Publish complete parcels in one scan or repair transaction. Check replay eligibility per parcel. Preserve exact source references and quarantine selection rules. Request soil and qualification work only for geometry changes or repairs.
- **Tax.** Process delinquent, billing, foreclosure, and pipeline files in that order. Publish each complete file in one transaction. Record unchanged observations without rewriting business rows. Permit later links to parcels that did not exist during the first import.
- **County soil.** Retain count and geometry responses, requests, and CRS declarations. Publish complete sets atomically. Reuse intersections when the spatial inputs are unchanged.
- **USDA soil.** Retain interpretation and map-unit evidence. Bind screening to the applicable soil and parcel inputs. Do not fetch source data or reset the observation time on replay.

## Verification

Push the complete implementation before formal verification. Run the checks at the exact PR commit. Use fresh disposable PostGIS databases and saved source data. Do not load `.env` or contact live county or USDA services.

- Run the full offline and PostGIS suites. Check a populated database upgrade and a failed first import followed by retry. Check foreign keys and sale deletion when an input omits a sale.
- Verify that equivalent numeric representations and polygon ring orders produce identical hashes.
- Measure business writes and spatial work. Verify that unchanged input causes neither. Verify that an ownership-only change does not run soil intersections. Use positive controls for both measurements.
- Replay an older capture. Verify that it preserves current ownership and does not add source ownership-change history.
- Verify that parcel imports stay within the existing ten-minute deadline. Record the first import and repeat separately.
- Obtain independent GPT-6.2 Astra review at high thinking. Follow `docs/review-verification.md` and record the results in the PR.
