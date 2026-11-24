---
name: verify-landman-cli
description: Verify parcel edge cases with small local fixtures and seeded PostGIS data. Use for parcel ingestion, storage, geometry, sales, quarantine, repair, scheduler, or qualification changes.
---
# Verify Landman CLI

Use small local fixtures by default. No routine check in this skill calls county GIS or USDA. A fixture is a fixed test input. The existing tests create a few parcel rows, serve local ArcGIS responses, and remove their rows after each test. Do not fetch a county to test one parcel edge case.

## Default parcel checks

Run from the repository root. First check translation and source identities without a database:

```sh
go test -race ./internal/connector/union \
  -run '^(TestGISTranslator_Translate|TestGISTranslator_TranslateGeometry|TestGISTranslatorSaleSlots|TestGISTranslatorAssociatesHoleWithContainingShell|TestGISSource_NullPIDUsesObjectID)$' \
  -count=1 -v
```

Use [database setup](../verify-landman/database.md) in a new shell with `VERIFY_DB_NAME=landman_tax_integration`. Then run these bounded fixture groups serially:

```sh
# A misspelled -run regex must fail before the verification lane starts.
for name in TestSaveParcelKeepsID TestShapeWriterPreservesNumericZeroBuildingFacts TestShapeWriterRejectsMissingShapeAndRollsBack TestShapeWriterReplacesSales TestShapeWriterSaleFailureRollsBack; do
  go test -tags postgres ./internal/parcelstore -list "^${name}$" | grep -Fx "$name" || exit 1
done
DATABASE_URL="$VERIFY_DATABASE_URL" go test -race -tags postgres ./cmd/landman \
  -run '^(TestParcelQuarantineWholePIDAcrossPages|TestParcelQuarantineSurvivesFailureAndStorageFailureRollsBack|TestParcelReprocessCurrentMultipartAndRecurrence|TestParcelNumericStorageBoundaries|TestParcelContradictoryObjectIdentityFailsAtomically|TestParcelRepeatedObjectSamePIDPreservesLastPart|TestParcelEvidenceObservationAndOfflineReplay)$' \
  -count=1 -v
DATABASE_URL="$VERIFY_DATABASE_URL" go test -race -tags postgres ./internal/parcelstore \
  -run '^(TestSaveParcelKeepsID|TestShapeWriterPreservesNumericZeroBuildingFacts|TestShapeWriterRejectsMissingShapeAndRollsBack|TestShapeWriterReplacesSales|TestShapeWriterSaleFailureRollsBack)$' \
  -count=1 -v
```

Require the named tests to execute. Check each required name appears as a `--- PASS:` line in the verbose log; fail the lane when any name is missing or skipped. An unset database URL can cause skips and is not a pass. Save verbose logs and exit codes. Exit the setup shell to remove its container.

## Parcel cases and expected results

| Case | Required result |
| --- | --- |
| Missing PID | Stable fallback identity; no favorable vacancy assumption |
| Zero and missing numeric fields | Preserve zero and NULL independently; the seed and check-loaded commands compare reported `year_built` without asserting vacancy from either value |
| Numeric column boundary or overflow | Accept valid bounds; quarantine invalid values without losing unrelated parcels |
| Multipart geometry and holes | Preserve all valid parts and assign holes to the containing shell |
| Invalid part on another page | Quarantine the whole parcel identity; preserve prior published data |
| Repeated or contradictory object identity | Preserve valid last-part semantics; reject contradictory identity atomically |
| Identical observations and offline replay | Retain observation evidence without false field changes; preserve newer values and make no source request during replay |
| Repair and recurrence | Use current multipart data; resolve selected evidence and retain recurrent evidence |
| Sale replacement or failed write | Delete omitted slots on complete replacement; retain stable IDs; roll back failed publication |

## Scheduler and qualification changes

Use the same isolated setup, with local fixtures only:

```sh
go test -race ./cmd/landman -count=1 -v
DATABASE_URL="$VERIFY_DATABASE_URL" go test -race -tags postgres ./internal/qualification \
  -run '^(TestQualificationSpatialAndFreshness|TestQualificationGroupPublicationAndRetirement|TestQualificationHoleAndUnavailableStates)$' \
  -count=1 -v
```

The command-package unit tests check dispatch and failure ordering. The selected qualification tests check a few stored shapes, source retirement, missing evidence, and independent refresh failures. For a qualification algorithm change, also run the full local qualification suite in [qualification checks](features/qualification.md). This uses local data, not live county requests.

## Optional checks

- [Live GIS journey](live-gis.md): explicit approval only. It fetches county soil and parcels and calls USDA.
- [Qualification checks](features/qualification.md): extended local spatial cases and a separately approved live-source path.
- County-sized synthetic performance and retained live-capture replay: use `docs/ingestion-gaps-verification.md` only when explicitly selected.
- Manual tax files and the combined bare command: use `verify-landman-tax` only when that opt-in drive is requested.

## Vacancy cutover

`capture-seed` writes parcel facts without the removed `likely_vacant` column. `check-loaded` compares retained sample `year_built` and situs fields against the seeded database; it does not certify a vacancy tier. For classifier publication and cursor freshness use the disposable PostGIS vacancy tests and the current live capture writer with local HTTP fixtures. The checked-in seed covers parcel, soil, sales, and ownership verification, not county coverage or source admission.

## Completion

Report the selected edge cases, commands, executed test names, skips, exits, and elapsed time. A default pass proves only the selected local cases. It does not prove live-source access, county-wide performance, or complete V1 behavior. Preserve the optional release gates; do not run them as a prerequisite for this default check.
