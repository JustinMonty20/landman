---
name: verify-landman-cli
description: Verify CLI ingestion with local fixtures and PostGIS data. Use for parcels, soil, retained capture or replay, FEMA, vacancy, storage, quarantine, repair, scheduler, or qualification changes.
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
DATABASE_URL="$VERIFY_DATABASE_URL" go test -race -tags postgres ./cmd/landman \
  -run '^(TestParcelQuarantineWholePIDAcrossPages|TestParcelQuarantineSurvivesFailureAndStorageFailureRollsBack|TestParcelReprocessCurrentMultipartAndRecurrence|TestParcelNumericStorageBoundaries|TestParcelContradictoryObjectIdentityFailsAtomically|TestParcelRepeatedObjectSamePIDPreservesLastPart|TestParcelEvidenceObservationAndOfflineReplay)$' \
  -count=1 -v
DATABASE_URL="$VERIFY_DATABASE_URL" go test -race -tags postgres ./internal/parcelstore \
  -run '^(TestSaveParcelKeepsID|TestShapeWriterPersistsNumericZeroVacancy|TestShapeWriterRejectsMissingShapeAndRollsBack|TestShapeWriterReplacesSales|TestShapeWriterSaleFailureRollsBack)$' \
  -count=1 -v
```

Require the named tests to execute. An unset database URL can cause skips and is not a pass. Save verbose logs and exit codes. Exit the setup shell to remove its container.

## Parcel cases and expected results

| Case | Required result |
| --- | --- |
| Missing PID | Stable fallback identity; no favorable vacancy assumption |
| Zero and missing numeric fields | Preserve zero; keep missing values distinct |
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

## Other ingestion paths

Select the matching row when its source or shared ingestion behavior changes. Use a new container from [database setup](../verify-landman/database.md) for each row. All commands use small local fixtures. Run from the repository root and require the database tests to execute.

| Path | Database name | Command |
| --- | --- | --- |
| Parcel publication, owner labels, and replay | `landman_parceljob_test_verify` | `DATABASE_URL="$VERIFY_DATABASE_URL" go test -race -tags postgres ./internal/connector/union/parceljob -count=1 -v` |
| Soil capture, screening, and replay | `soil_test_verify` | `SOIL_TEST_DATABASE_URL="$VERIFY_DATABASE_URL" go test -race -tags postgres ./internal/connector/union/soiljob -count=1 -v` |
| Retained FEMA publication and replay | `landman_api_test_fema` | `FEMA_API_TEST_DATABASE_URL="$VERIFY_DATABASE_URL" go test -race -tags postgres ./internal/connector/fema -count=1 -v` |
| Vacancy fixtures | `landman_api_test_verify` | `API_TEST_DATABASE_URL="$VERIFY_DATABASE_URL" go test -race -tags postgres ./internal/vacancy -count=1 -v` |
| Structure and assessed-value capture/replay | `landman_api_test_verify` | `API_TEST_DATABASE_URL="$VERIFY_DATABASE_URL" go test -race -tags postgres ./internal/connector/union/vacancyjob -count=1 -v` |

FEMA is the Federal Emergency Management Agency. Its database fixture checks publication retention and replay. It does not parse a saved official archive or download a release. `TestSavedUnionArchive` needs an explicitly selected `FEMA_TEST_ARCHIVE` path; report its default skip. Structure and assessed-value fixtures use local HTTP services, not county requests. These checks do not prove full-county completeness or runtime limits. Tax capture and replay use `verify-landman-tax`. Full stored-soil assessment and performance checks remain separate selected checks; do not infer them from capture fixtures.

## Isolated Make targets

For the extended bootstrap, migration, and scheduler recipes, define this function in Bash at the repository root:

```sh
verify_make() { .cursor/skills/verify-landman-cli/verify-make.sh "$@"; }
```

The executable helper removes the `.env` include from a temporary Makefile and clears inherited settings. It accepts build and the documented database targets. Database targets require an explicit `DB_CONTAINER=landman-verify-<name>`. Confirm that this run owns the container before any reset. Pass each database URL on the command line. The helper does not prove container ownership.

Use `verify_make build`, not `make run`. The helper leaves the product Makefile unchanged. Do not use it to run arbitrary Make targets.

## Optional checks

- [Feature map](features/README.md): local bootstrap, migrations, scheduler, and qualification checks, plus the optional GIS journey. Bootstrap and scheduler waiting do not need a live-source request.
- [Live GIS journey](live-gis.md): explicit approval only. It fetches county soil and parcels and calls USDA. The one-page timing probe is a diagnostic, not countywide capture proof.
- [Qualification checks](features/qualification.md): extended local spatial cases and a separately approved live-source path.
- County-sized synthetic performance and retained live-capture replay: use `docs/ingestion-gaps-verification.md` only when explicitly selected.
- Manual tax files and the combined bare command: use `verify-landman-tax` only when that opt-in drive is requested.

## Completion

Report the selected edge cases, commands, executed test names, skips, exits, and elapsed time. A default pass proves only the selected local cases. It does not prove live-source access, county-wide performance, or complete V1 behavior. Preserve the optional release gates; do not run them as a prerequisite for this default check.
