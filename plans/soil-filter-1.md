# Union County vacant filter

## Goal

Mark a Union County GIS record as likely vacant only when numeric `YEARBLT` or `SQFT` is zero. This unit implements the filter-1 section of [the soil layer plan](soil-layer.md). The later persist-1 unit stores the flag for every layer 215 parcel. This unit does not change parcel downloads or storage.

## Scope

- Edit only `internal/connector/union/vacant_filter.go` and `internal/connector/union/vacant_filter_test.go` for code. This plan file is the only additional file.
- Remove the `FMV_IMPRV` currency-string rule and the `RURAL ACREAGE TRACT` rule. Do not replace them with numeric `FMV_IMPRV = 0` or unconditional `RURAL ACRES`.
- Keep case-insensitive attribute lookup. Do not treat null or a string zero as numeric zero.

## Acceptance

- Numeric `FMV_IMPRV` 0 with `YEARBLT` 2025 and `SQFT` 3391 returns false.
- `RURAL ACRES` with `YEARBLT` 1997 and `SQFT` 22797 returns false. `RURAL ACRES` with `YEARBLT` 0 returns true because of the year.
- `$0` and `Rural acreage tract` alone return false. Numeric zero in either `YEARBLT` or `SQFT` returns true. Empty attributes return false.

## Verification

- Run `go test -race ./internal/connector/union/ -count=1 -run TestIsLikelyVacantGISRecord` and `go test -race ./... -count=1`.
- Compare the trunk and head filter tests. Check the ten live scenarios in the filter-1 section of the soil layer plan, including the live layer 215 counts. Record any count drift without changing the rule.
- Time the filter test at trunk, head, and trunk. Record the first trunk duration. The head run must finish in less than 30 seconds; do not claim a speedup ratio.
- Run the repository's lint and type checks before pushing. Report any unavailable verification rather than treating unit tests alone as the full gate. Open a ready PR and leave the merge to the operator.
