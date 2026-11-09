# Soil layer plan

You want soil for every parcel the vacant filter keeps, for every county, in one shared model. Union is only the first adapter. It reads MapServer layer 197. This file is the plan. It changes no Go code, no migration, and no config.

Shared types live in `internal/soil`. Shared tables are `soil_polygons`, `parcel_soils`, and `soil_map_units`. The county id type lives in `internal/county`, from `plans/county-id-package.md`. Soil stores that id as `TEXT` with `CHECK (county_id IN ('union'))`, the same text type as `parcels.county_id`. Appendix A holds the measurements. Appendix B holds the rejected designs.

## How to read this

One box is one unit of work. Every box names the evidence that checks it. A nested box is a sub-step of the box above it. Check a box only when its evidence exists, a file, a log line, a screenshot, a test run, or a SHA. The body is a how-to. The appendices explain and record.

The program runs `pstack/skills/poteto-mode/playbooks/autopilot-stack.md`. The operator lands filter-1, soil-1, persist-1, soil-2, soil-3, and soil-4. soil-1 waits until county-1 from `plans/county-id-package.md` is on trunk. This program does not land county-1. This plan stops at merge-ready. Nobody merges from the checklist.

Tests alone are not sufficient verification. A PR is verified only when its unit, live, and perf boxes are all checked.

## Program checklist

### Arm the program

- [ ] State the protocol and this plan to the operator, then stop. Start execution only on the operator's explicit go.
- [ ] Read these from trunk at program start. Re-read them at every tick.
  - [ ] `git show origin/main:pstack/skills/poteto-mode/playbooks/autopilot-stack.md`
  - [ ] `git show origin/main:pstack/skills/swarm/SKILL.md`
  - [ ] `git show origin/main:pstack/skills/poteto-mode/playbooks/opening-a-pr.md`
  - [ ] `git show origin/main:plans/soil-layer.md`
  - [ ] `git show origin/main:plans/county-id-package.md`
  - [ ] `git show origin/main:plans/scraper-enrichment.md`
  - [ ] `git show origin/main:plans/spatialist-removal-and-critical-fixes.md`
- [ ] On the operator's go, arm the audit tick as `/loop 1h` with the tick prompt below. Never leave the cadence to memory.
- [ ] Use this tick prompt, verbatim. "Re-read the execution playbook from trunk. Audit the operation against it and fix drift in this tick. Probe every active lane and judge progress by side effects only. Stand down a stuck lane and dispatch its replacement now. Then post a short status message to the operator in chat only when the audit found a tracked change that no earlier status message reported, such as a PR opened, a code-ready head, a round launched or closed, a verdict, a merge, a stuck agent and the action taken, a blocker added or cleared, or a decision only the operator can make. Name every such change and nothing else. Do not repeat a table, the merged list, or an unchanged blocker. If the audit found none, end the turn with no reply text. Either way, log this tick's row in your decision trail. The row names the items reported, or none."
- [ ] On the operator's hold or stand-down, send every owner a zero-writes order at once.

### Spawn owners

- [ ] Spawn one owner per PR with the full lifecycle the execution playbook names.
- [ ] Follow this dependency graph. Start dependent work only after its parent merges, or base it on the parent branch when the execution playbook stacks.
  - [ ] filter-1 branches from trunk. It does not wait on county-1.
  - [ ] soil-1 starts only after county-1 from `plans/county-id-package.md` is on trunk. county-1 is not an owner in this program.
  - [ ] persist-1 after filter-1.
  - [ ] soil-2 and soil-3 both start after soil-1. They do not edit the same files.
  - [ ] soil-4 after persist-1, soil-2, and soil-3.
- [ ] Hold the file boundaries. filter-1 touches only `internal/connector/union/vacant_filter.go` and `internal/connector/union/vacant_filter_test.go`. persist-1 touches only `internal/service/parcel_id_importer.go`, `internal/service/parcel_id_importer_test.go`, `internal/parcelstore/parcels.go`, `internal/parcelstore/parcels_test.go`, and `main.go`. soil-1 touches only `internal/soil/` and `internal/connector/union/soil_translate.go`, `internal/connector/union/soil_translate_test.go`, and `internal/connector/union/testdata/soil/`. soil-2 touches only `internal/connector/union/gis.go`, `internal/connector/union/gis_test.go`, and `internal/connector/union/soil_fetch.go`. soil-3 touches only the one new `db/migrations/<prefix>_soil.sql` file named in its Files box. soil-4 touches only `internal/soil/join.go`, `internal/soil/names.go`, their tests, and `main.go`.
- [ ] Hold the review gate. No PR in this program changes an interaction. None wait for screenshots in chat before the operator decides to land them.

### PR mechanics, for every PR

- [ ] Resolve the forge once. Default to `gh`. If `command -v origin` succeeds and Origin can resolve the repository, use `origin pr` for every PR operation. Record any fallback to `gh`. Never require `gt`.
- [ ] Open the PR ready, never draft, per **Opening a PR**. Use the run's built-in PR tool when it has one, else `origin pr create --status open --base <base-branch>` or `gh pr create --base <base-branch>` according to the resolved forge. A stack child targets its parent branch.
- [ ] Run the repo's lint and typecheck once before the PR-facing push. Push with hooks on.
- [ ] Run `/deslop` before each commit and `/no-comments` before review.
- [ ] Triage every Bugbot and security-reviewer comment per `pstack/skills/poteto-mode/references/bugbot-triage.md`.
- [ ] Rebase onto current trunk before the code-ready report and babysit. Keep that merge base in fix rounds. Rebase again only at merge prep, on a `git merge-tree` conflict with trunk, on a CI failure that comes from a change on trunk, or when a known concurrent change to the same files lands. persist-1 rebases onto filter-1. soil-2 and soil-3 rebase onto soil-1. soil-4 rebases onto persist-1, soil-2, and soil-3 after those parents are stacked.

### Verdict and merge, for every PR

- [ ] A full swarm runs once, at the code-ready head. Another full swarm runs only for new behavior, a redesign, or a large new diff. A rebase, a merge-conflict resolution, or a minor fix gets a single verifier at the new head, and a wording-only change gets none. Run each full swarm per `pstack/skills/swarm/SKILL.md`. One gates lane. The ten live lanes from the PR's **Verify, live** block. The perf lane from its **Verify, perf** block. Two or more audit lanes, each with its own focus, that read the diff and the receipts and distrust the PR body. The root audits the receipts in the merge-ready report before the verdict.
- [ ] Judge every finding by the execution playbook's blocking rule. The verdict is clean when none blocks. A failure on trunk behavior in a lane marked `Regression lane against trunk.` is recorded against trunk and does not block the PR. A new head gets a fresh verdict, except for results that stay valid under the patch-id rule in `playbooks/shipping.md`.
- [ ] Stop when the PR is verified and green at its current head, then hand it to the operator. Do not merge. A restack that leaves `git patch-id` unchanged keeps the verdict. After the handoff, nobody pushes without the operator's go.

### Boot recipe, for every live lane

Each live lane runs on its own cloud VM at the PR head. This repo has no browser UI for soil. Drive the Go test binary and, where a lane says so, `curl` and `psql`. There is no `control-cli` skill installed in this environment. Appendix C records that gap. The commands below are the drive.

- [ ] `git fetch origin <head-branch> && git checkout <head SHA>`.
- [ ] `go test -race ./internal/connector/union/ ./internal/soil/ ./internal/parcelstore/ -count=1` for filter-1, soil-1, and soil-2. For persist-1, soil-3, and soil-4, start the repo's Postgres and PostGIS the way `make migrate-up` does, then apply migrations. Wait until `psql` accepts a query.
- [ ] Deliver input only through those commands. Read-only diagnostics are the test output, the `curl` body, and the `psql` result rows.
- [ ] Save every screenshot to `/tmp/swarm-<pr-id>/worker-<n>/<slug>.png` and return the paths with the report. A lane with no display saves a text render of the command output at that path.

## Drop the dead vacant clauses (filter-1)

**Depends on.** None.

**Files.**

- [ ] Edit `internal/connector/union/vacant_filter.go`.
- [ ] Edit `internal/connector/union/vacant_filter_test.go`.

**Build.**

- [ ] Keep a parcel when `YEARBLT` or `SQFT` is numeric 0. That is the live kept set of 25,885 parcels in Appendix A.
- [ ] Delete the `FMV_IMPRV` currency-string clause and the `RURAL ACREAGE TRACT` clause. Neither matches a live row. Do not add numeric `FMV_IMPRV = 0`. Do not add unconditional `LAND_TYPE = 'RURAL ACRES'`. Appendix A records why both would mark houses vacant.
- [ ] Add table rows for numeric `FMV_IMPRV` 0 with `YEARBLT` 2025, for `RURAL ACRES` with `YEARBLT` 1997 and `SQFT` greater than 0, and for `RURAL ACRES` with `YEARBLT` 0. The first two are false. The third is true because the year is 0.

**You see.**

- [ ] `go test -race ./internal/connector/union/ -count=1 -run TestIsLikelyVacantGISRecord` prints `PASS`, and the 2025 house case is false.

**Verify, unit.** Tests alone are not sufficient verification. A PR is verified only when its unit, live, and perf boxes are all checked.

- [ ] `vacant_filter_test.go` gains the three rows above and drops the expectation that `"$0"` or `"Rural acreage tract"` alone is vacant. Run `go test -race ./internal/connector/union/ -count=1 -run TestIsLikelyVacantGISRecord`.

**Verify, live.** Tests alone are not sufficient verification. A PR is verified only when its unit, live, and perf boxes are all checked. Ten lanes on `grok-4.7-xhigh-fast` at the PR head, per the boot recipe.

- [ ] Lane 1. Regression lane against trunk. Run `go test -race ./internal/connector/union/ -count=1 -run TestIsLikelyVacantGISRecord` at trunk and at head. Trunk still treats `"$0"` as vacant. Record that, and gate the head cases in Appendix A. Save `/tmp/swarm-filter-1/worker-1/regression.png`. Pass when trunk's old test passes and head's new cases pass.
- [ ] Lane 2. Numeric `FMV_IMPRV` 0, `YEARBLT` 2025, `SQFT` 3391. Save `/tmp/swarm-filter-1/worker-2/fmv-house.png`. Pass when the function returns false.
- [ ] Lane 3. `LAND_TYPE` `RURAL ACRES`, `YEARBLT` 1997, `SQFT` 22797. Save `/tmp/swarm-filter-1/worker-3/rural-house.png`. Pass when the function returns false.
- [ ] Lane 4. `LAND_TYPE` `RURAL ACRES`, `YEARBLT` 0, `SQFT` 0. Save `/tmp/swarm-filter-1/worker-4/rural-zero.png`. Pass when the function returns true.
- [ ] Lane 5. `FMV_IMPRV` string `$0` and no other vacant field. Save `/tmp/swarm-filter-1/worker-5/currency.png`. Pass when the function returns false.
- [ ] Lane 6. `LAND_TYPE` `Rural acreage tract` and no zero year or sqft. Save `/tmp/swarm-filter-1/worker-6/old-phrase.png`. Pass when the function returns false.
- [ ] Lane 7. `YEARBLT` 0 as float64. Save `/tmp/swarm-filter-1/worker-7/float-year.png`. Pass when the function returns true.
- [ ] Lane 8. Empty attributes. Save `/tmp/swarm-filter-1/worker-8/empty.png`. Pass when the function returns false.
- [ ] Lane 9. Re-query layer 215 for `YEARBLT = 0 OR SQFT = 0`, `FMV_IMPRV = 0`, and `LAND_TYPE = 'RURAL ACRES'`. Save `/tmp/swarm-filter-1/worker-9/live-counts.png`. Pass when the three counts are still 25885, 116242, and 30861, or the lane records the new counts and does not change the rule in this PR.
- [ ] Lane 10. Run the package tests twice. Save `/tmp/swarm-filter-1/worker-10/twice.png`. Pass when both runs pass and the function has no map of seen parcels.

**Verify, perf.** Tests alone are not sufficient verification. A PR is verified only when its unit, live, and perf boxes are all checked.

- [ ] Metric. Wall time of the vacant filter test at trunk and at head. Trunk already has the test. The end state is the same live kept count of parcels with numeric year or sqft 0.
- [ ] Probe. `go test -race ./internal/connector/union/ -count=1 -run TestIsLikelyVacantGISRecord` at trunk, then head, then trunk.
- [ ] Baseline. Record the trunk duration first.
- [ ] Rule. Fail if the head test takes 30 seconds or longer. Do not report a ratio against trunk. The live kept count is not a test timer.

**Review gate.** None. filter-1 is not review-gated.

**Merge.**

- [ ] Root's clean verdict at the exact head SHA.
- [ ] Bugbot triage done.
- [ ] Rebased onto current trunk after the verdict, with `git patch-id` unchanged.
- [ ] The owner does not merge. The operator lands filter-1 before persist-1.

## Persist kept parcel shapes (persist-1)

**Depends on.** filter-1.

**Files.**

- [ ] Edit `internal/service/parcel_id_importer.go`.
- [ ] Edit `internal/service/parcel_id_importer_test.go`.
- [ ] Create `internal/parcelstore/parcels.go`.
- [ ] Create `internal/parcelstore/parcels_test.go`.
- [ ] Edit `main.go`.

**Build.**

- [ ] Add an optional kept-record hook on `ParcelIDImportService`. Call it with the filtered `[]connector.RawRecord`, geometry included, before `extractParcelIDs` drops `RawData`. `internal/service` does not import `union` or `parcelstore`.
- [ ] Add `parcelstore.Save` that upserts a `models.GISParcel` into `parcels` on `(county_id, source_name, source_parcel_id)`. Write `shape` at SRID 3857. On conflict, update the mapped columns, including `shape`.
- [ ] Return an error when a kept record has no rings. Do not insert a null `shape`.
- [ ] `main` translates each kept record and calls `Save`. Return that error from the hook. Do not log it and return nil. Do not write `parcel_soils`, `soil_polygons`, or `soil_map_units` from this PR.
- [ ] The constructor takes a connection string argument. The package does not read the environment. There is no other plan file for this writer. This section is that prerequisite.

**You see.**

- [ ] A kept fixture parcel with rings is one `parcels` row whose `ST_SRID(shape)` is 3857, and a second save updates that row instead of inserting another.

**Verify, unit.** Tests alone are not sufficient verification. A PR is verified only when its unit, live, and perf boxes are all checked.

- [ ] `parcel_id_importer_test.go` gains a case that the kept-record hook receives geometry and the id hook still receives only strings. Run `go test -race ./internal/service/ ./internal/parcelstore/ -count=1`.

**Verify, live.** Tests alone are not sufficient verification. A PR is verified only when its unit, live, and perf boxes are all checked. Ten lanes on `grok-4.7-xhigh-fast` at the PR head, per the boot recipe.

- [ ] Lane 1. Regression lane against trunk. Run `go test -race ./internal/service/ -count=1` at trunk and at head. Trunk has no kept-record hook. Record that, and gate the new hook plus the old id-hook tests. Save `/tmp/swarm-persist-1/worker-1/regression.png`. Pass when trunk's importer tests pass and head's new hook test passes.
- [ ] Lane 2. Save one kept parcel whose rings match the translator fixture. Save `/tmp/swarm-persist-1/worker-2/insert-shape.png`. Pass when `ST_SRID(shape)` is 3857 and `source_parcel_id` is the PID.
- [ ] Lane 3. Save the same PID again with a different `mapped_acres`. Save `/tmp/swarm-persist-1/worker-3/upsert.png`. Pass when the table still has one row and `mapped_acres` matches the second save.
- [ ] Lane 4. Save a kept record with no rings. Save `/tmp/swarm-persist-1/worker-4/no-rings.png`. Pass when `Save` returns an error and the table gains no row.
- [ ] Lane 5. Pass a record the filter rejects. Save `/tmp/swarm-persist-1/worker-5/not-kept.png`. Pass when the kept-record hook is not called for it and no row is inserted.
- [ ] Lane 6. Insert SRID 4326 through a raw SQL attempt in the test, not through `Save`. Save `/tmp/swarm-persist-1/worker-6/bad-srid.png`. Pass when the existing `parcels` check rejects it.
- [ ] Lane 7. After a successful save, select `source_extras`. Save `/tmp/swarm-persist-1/worker-7/extras.png`. Pass when the value is `{}` and `parcel_soils` does not exist or has no row from this PR.
- [ ] Lane 8. Cancel the context before `Save`. Save `/tmp/swarm-persist-1/worker-8/cancel.png`. Pass when no row is committed.
- [ ] Lane 9. Return a database error from the test double. Save `/tmp/swarm-persist-1/worker-9/save-error.png`. Pass when the hook returns that error and the test fails if the error is nil.
- [ ] Lane 10. Run two kept records, one with rings and one without. Save `/tmp/swarm-persist-1/worker-10/partial.png`. Pass when the error stops the batch and the successful row is not committed if they share one transaction.

**Verify, perf.** Tests alone are not sufficient verification. A PR is verified only when its unit, live, and perf boxes are all checked.

- [ ] Metric. Time to upsert the one-parcel fixture, and the row count after a second upsert. Trunk has no writer. The end state is one `parcels` row with a shape.
- [ ] Probe. The fixture upsert timed at head after recording that trunk has no `parcelstore` package.
- [ ] Baseline. Record trunk as no writer and no elapsed time.
- [ ] Rule. Fail if the one-parcel upsert takes 5 seconds or longer, or if the second upsert leaves a row count other than 1. Do not report a ratio against trunk.

**Review gate.** None. persist-1 is not review-gated.

**Merge.**

- [ ] Root's clean verdict at the exact head SHA.
- [ ] Bugbot triage done.
- [ ] Rebased onto filter-1 after the verdict, with `git patch-id` unchanged.
- [ ] The owner does not merge. The operator lands persist-1 after filter-1 and before soil-4.

## Add the shared soil types (soil-1)

**Depends on.** county-1.

**Files.**

- [ ] Create `internal/soil/polygon.go`.
- [ ] Create `internal/soil/overlap.go`.
- [ ] Create `internal/connector/union/soil_translate.go`.
- [ ] Create `internal/connector/union/soil_translate_test.go`.
- [ ] Create `internal/connector/union/testdata/soil/layer197-feature.json` from a live layer 197 feature response. Do not point the test at `docs/examples/`.

**Build.**

- [ ] Import `github.com/JustinMonty20/landman/internal/county`. `Polygon.County` is `county.ID`. Use `county.Union` for Union County. Do not declare `CountyID`, `CountyUnion`, or `Valid` in package `soil`. Do not add a second county constant in this PR.
- [ ] Add `Polygon` and `Overlap` in package `soil` with the fields in Appendix A. `MapUnitKey` is the national `MUKEY`. `ObjectID` is the text form of the county layer's feature id. `Overlap` has no minimum share.
- [ ] Add `ParseSoilFeature` in package `union`. It returns `soil.Polygon` with `County` set to `county.Union` and `SourceName` `union_county_gis`. Map `OBJECTID_1` to `ObjectID`, `MUKEY`, `MUSYM`, `AREASYMBOL`, and `SPATIALVER`. Ignore `HydrcRatng`. Reject a missing `MUKEY`, a missing `MUSYM`, and a geometry with no rings.
- [ ] Set the polygon SRID to 3857, the same constant `unionGISSourceSRID` in `translator.go` line 15. Parse rings the way `geometryShape` does at `translator.go` lines 311 to 335.
- [ ] Do not put these types in `internal/models`. Do not import `internal/service`. Package `soil` does not import `union`.

**You see.**

- [ ] `go test -race ./internal/soil/ ./internal/connector/union/ -count=1 -run TestParseSoilFeature` prints `PASS` and names the ChA fixture.

**Verify, unit.** Tests alone are not sufficient verification. A PR is verified only when its unit, live, and perf boxes are all checked.

- [ ] `soil_translate_test.go` gains a table for the captured ChA feature, a missing `MUKEY`, an empty `features` list, and a geometry with no rings. Do not add `internal/soil/county_test.go`. County id tests stay in county-1. Run `go test -race ./internal/soil/ ./internal/connector/union/ -count=1 -run TestParseSoilFeature`.

**Verify, live.** Tests alone are not sufficient verification. A PR is verified only when its unit, live, and perf boxes are all checked. Ten lanes on `grok-4.7-xhigh-fast` at the PR head, per the boot recipe.

- [ ] Lane 1. Regression lane against trunk. Run `go test -race ./internal/connector/union/ -count=1` at trunk and at head. Trunk has no `ParseSoilFeature`. Record that, and gate the new parser tests plus a green `go test -race ./internal/connector/union/ -count=1` at head. Save `/tmp/swarm-soil-1/worker-1/regression.png`. Pass when trunk's existing union tests pass and head's new parser tests pass.
- [ ] Lane 2. Parse `testdata/soil/layer197-feature.json`. Save `/tmp/swarm-soil-1/worker-2/parse-cha.png`. Pass when `MapUnitSymbol` is `ChA`, `MapUnitKey` is `115341`, `AreaSymbol` is `NC179`, `SpatialVersion` is 5, `County` is `union`, and `HydrcRatng` is not a field on the struct.
- [ ] Lane 3. Feed a feature with no `MUKEY`. Save `/tmp/swarm-soil-1/worker-3/missing-mukey.png`. Pass when `ParseSoilFeature` returns an error and no polygon.
- [ ] Lane 4. Read the fixture rings. Save `/tmp/swarm-soil-1/worker-4/rings.png`. Pass when the stored polygon SRID is 3857 and the first coordinate equals the fixture.
- [ ] Lane 5. Feed an ArcGIS `error` object instead of a feature. Save `/tmp/swarm-soil-1/worker-5/arcgis-error.png`. Pass when the parser returns an error.
- [ ] Lane 6. Feed `{"features":[]}`. Save `/tmp/swarm-soil-1/worker-6/empty-features.png`. Pass when the list result is empty and the error is nil.
- [ ] Lane 7. Feed `OBJECTID` as JSON null and `OBJECTID_1` as 1. Save `/tmp/swarm-soil-1/worker-7/null-objectid.png`. Pass when `ObjectID` is the text `1`.
- [ ] Lane 8. Feed an extra attribute the struct does not name. Save `/tmp/swarm-soil-1/worker-8/extra-field.png`. Pass when the known fields still match the fixture and the extra attribute is not stored.
- [ ] Lane 9. Feed a geometry with an empty `rings` array. Save `/tmp/swarm-soil-1/worker-9/no-rings.png`. Pass when the parser returns an error.
- [ ] Lane 10. Call `ParseSoilFeature` twice on the same fixture bytes. Save `/tmp/swarm-soil-1/worker-10/parse-twice.png`. Pass when the two structs are equal.

**Verify, perf.** Tests alone are not sufficient verification. A PR is verified only when its unit, live, and perf boxes are all checked.

- [ ] Metric. Wall time of `go test -race ./internal/soil/ ./internal/connector/union/ -count=1` at trunk and at head, plus a count of dials to `atlas.unioncountync.gov`. Trunk has no soil parser, so also record that the new work is the parser test and the end state is a parsed `soil.Polygon`.
- [ ] Probe. Run the test command at trunk, then at head, then at trunk again. Wrap the head run so a dial to `atlas.unioncountync.gov` fails the process.
- [ ] Baseline. Record the trunk duration first. Record trunk soil-parser dials as 0 because the parser does not exist.
- [ ] Rule. Fail the head if it dials `atlas.unioncountync.gov` once, or if the head package test takes 60 seconds or longer. Do not report a ratio against trunk. Trunk does not parse soil.

**Review gate.** None. soil-1 is not review-gated.

**Merge.**

- [ ] Root's clean verdict at the exact head SHA.
- [ ] Bugbot triage done.
- [ ] Rebased onto current trunk after the verdict, with `git patch-id` unchanged.
- [ ] The owner does not merge. The operator lands soil-1 beside filter-1. Neither waits on the other.

## Page the soil layer on the GIS rate limiter (soil-2)

**Depends on.** soil-1.

**Files.**

- [ ] Edit `internal/connector/union/gis.go`.
- [ ] Edit `internal/connector/union/gis_test.go`.
- [ ] Create `internal/connector/union/soil_fetch.go`.
- [ ] Create `internal/connector/union/testdata/soil/layer197-page.json` from a live paged response that includes `exceededTransferLimit`.

**Build.**

- [ ] Add `SoilQueryURL` to `GISSourceConfig`, set in `DefaultGISSourceConfig` to `https://atlas.unioncountync.gov/server/rest/services/OperationalLayers/MapServer/197/query`. Leave `BaseURL` as the layer 215 query URL. `TestDefaultGISSourceConfig` already locks that parcel URL at `gis_test.go` lines 128 to 131.
- [ ] Add `FetchSoilPolygons(ctx) ([]soil.Polygon, error)` on `GISSource`. Each polygon uses `county.Union`. Build the request with the existing `s.httpClient`. Do not call `NewRateLimitedClient` again.
- [ ] Page with `resultOffset`, `resultRecordCount` 2000, and `orderByFields=OBJECTID_1`. `ArcGISParams` cannot send `orderByFields`. `gis.go` lines 40 to 49. Add that field, or build the soil query in `soil_fetch.go`. Do not page without the order. The measured pages sent `orderByFields=OBJECTID_1`. Do not use `GISSource.batchSize`. That value is 50 and would turn this pull into hundreds of requests.
- [ ] Stop when a page returns fewer than 2000 features. If the pages do not add up to the count query, return an error. A body with no numeric `count` is an error. `queryTotalParcels` reads `count` with no existence check, so a bad body becomes 0 and `FetchBatches` returns nil. `gis.go` lines 184 to 185 and 111 to 158. Do not copy that.
- [ ] Treat HTTP status outside 200 to 299 as an error that includes the status. Treat an ArcGIS `error` object as an error. `queryTotalParcels` does not check status, at `gis.go` lines 173 to 185. Do not copy that. Send `User-Agent` `ParcelDataCollection/0.0.1`, the same header as `fetchBatch` at `gis.go` line 199.
- [ ] Before each page, if `ctx.Err()` is set, return `ctx.Err()` and do not start the request. Pass that `ctx` into `http.NewRequestWithContext` and into `RateLimitedClient.Do`.
- [ ] Out fields are `OBJECTID_1,AREASYMBOL,SPATIALVER,MUSYM,MUKEY`. `returnGeometry` is true. `where` is `1=1`. `f` is `json`. Do not store `HydrcRatng` on `soil.Polygon`.
- [ ] Keep this commit after a separate commit that only adds `SoilQueryURL` and its test, if that URL field lands without the fetch loop. Do not redesign `BaseURL` into a MapServer root in this PR.

**You see.**

- [ ] The httptest log shows two soil page URLs, the second with `resultOffset=2000`, and then the function returns. A 500 from the fixture server fails the test with the status in the error string.

**Verify, unit.** Tests alone are not sufficient verification. A PR is verified only when its unit, live, and perf boxes are all checked.

- [ ] `gis_test.go` gains a default `SoilQueryURL` assertion and an httptest that pages two responses, one with `exceededTransferLimit` true. Run `go test -race ./internal/connector/union/ -count=1 -run 'TestFetchSoilPolygons|TestDefaultGISSourceConfig'`.

**Verify, live.** Tests alone are not sufficient verification. A PR is verified only when its unit, live, and perf boxes are all checked. Ten lanes on `grok-4.7-xhigh-fast` at the PR head, per the boot recipe.

- [ ] Lane 1. Regression lane against trunk. Run `go test -race ./internal/connector/union/ -count=1` at trunk and at head. Trunk has no `FetchSoilPolygons`. Record that, and gate the new paging tests plus the existing GIS tests still passing. Save `/tmp/swarm-soil-2/worker-1/regression.png`. Pass when trunk's union tests pass and head's paging tests pass.
- [ ] Lane 2. Serve two pages from httptest, the first with 2000 features and `exceededTransferLimit` true, the second with 1250. Save `/tmp/swarm-soil-2/worker-2/two-pages.png`. Pass when both URLs contain `orderByFields=OBJECTID_1`, the offsets are 0 and 2000, and the client returns 3250 polygons.
- [ ] Lane 3. Ask the fixture to ignore a client `resultRecordCount` above 2000 and still return 2000. Save `/tmp/swarm-soil-2/worker-3/cap.png`. Pass when the client never sends `resultRecordCount` greater than 2000.
- [ ] Lane 4. Return HTTP 500 on the first page. Save `/tmp/swarm-soil-2/worker-4/http-500.png`. Pass when the error text contains 500 and the function does not return a polygon slice with a nil error.
- [ ] Lane 5. Return HTTP 200 and an ArcGIS `error` object. Save `/tmp/swarm-soil-2/worker-5/arcgis-error.png`. Pass when the function returns an error.
- [ ] Lane 6. Cancel the context after the first page is read and before the second `Do`. Save `/tmp/swarm-soil-2/worker-6/cancel.png`. Pass when the second request is absent from the httptest log and the error is `context.Canceled`.
- [ ] Lane 7. Count `RateLimitedClient.Do` calls on the source's existing client. Save `/tmp/swarm-soil-2/worker-7/one-limiter.png`. Pass when soil paging and a parcel page both call that same client, and `FetchSoilPolygons` does not construct another limiter.
- [ ] Lane 8. Count query returns 11250 and the pages return 11249 features. Also serve a count body with no `count` field. Save `/tmp/swarm-soil-2/worker-8/short-count.png`. Pass when both cases return an error.
- [ ] Lane 9. Read `DefaultGISSourceConfig`. Save `/tmp/swarm-soil-2/worker-9/urls.png`. Pass when `BaseURL` still ends in `/MapServer/215/query` and `SoilQueryURL` ends in `/MapServer/197/query`.
- [ ] Lane 10. Call `FetchSoilPolygons` twice against a fixture whose second response has a different `MUKEY` on object 1. Save `/tmp/swarm-soil-2/worker-10/second-fetch.png`. Pass when the second return value replaces the first and does not append to it.

**Verify, perf.** Tests alone are not sufficient verification. A PR is verified only when its unit, live, and perf boxes are all checked.

- [ ] Metric. Requests issued to pull a layer whose count is 11250 at page size 2000. Trunk issues no soil requests. The end state is one full in-memory copy of the polygons.
- [ ] Probe. The httptest counter in `TestFetchSoilPolygons`, run at head. Trunk has no test to run. Record trunk soil requests as 0 before the head run. Separately, and not inside `go test`, `curl` the live count URL in Appendix A and confirm the cap with `resultRecordCount=5000`.
- [ ] Baseline. Record trunk soil requests as 0 first.
- [ ] Rule. Fail if the fixture does not issue exactly 6 feature-page requests for a count of 11250 and a page size of 2000. A separate count query may add a 7th HTTP call. Fail if any feature page asks for more than 2000 rows. Fail if the live re-measure shows `maxRecordCount` above 2000 and the client still assumes 2000 without reading it. Do not report a ratio against trunk.

**Review gate.** None. soil-2 is not review-gated.

**Merge.**

- [ ] Root's clean verdict at the exact head SHA.
- [ ] Bugbot triage done.
- [ ] Rebased onto soil-1 after the verdict, with `git patch-id` unchanged.
- [ ] The owner does not merge. The operator lands soil-2 after soil-1. soil-2 and soil-3 can land in either order.

## Add the shared soil tables (soil-3)

**Depends on.** soil-1.

**Files.**

- [ ] Create the soil migration by hand. `make migrate-create` runs `goose create` with no sequential flag (`Makefile` lines 82 to 88), so the name it emits is a timestamp. Do not use `000002`. `db/migrations/000002_parcels_storage_srid.sql` is already on main. At implementation time, list `db/migrations/*.sql`, take the highest six-digit prefix, add 1, and zero-pad to six digits. county-1 may already have taken the next prefix for `parcels_county_check`. Name the file `db/migrations/<prefix>_soil.sql`. On main at `1bdf7b8cd51f8031f5ff108be623f6a354ef70b1` the highest prefix is `000002`. If county-1 then landed `000003_parcels_county_check.sql` and nothing higher exists, soil-3 uses `000004`.
- [ ] Do not edit `db/migrations/000001_init_schema.sql`. Do not create a PostgreSQL enum.

**Build.**

- [ ] Add `soil_map_units`, `soil_polygons`, and `parcel_soils` with the columns in the sketch below. Goose `Up` and `Down`. `Down` drops `parcel_soils`, then `soil_polygons`, then `soil_map_units`.
- [ ] `parcel_soils.parcel_id` references `parcels(id)` with `ON DELETE CASCADE`, the same link `parcel_sales` uses at `000001_init_schema.sql` lines 94 to 97.
- [ ] Also add a foreign key from `(county_id, source_name, source_parcel_id)` to `parcels (county_id, source_name, source_parcel_id)`, which is `parcels_source_identity_unique` at line 73. The only insert is the `INSERT ... SELECT` in soil-4, so both keys come from one `parcels` row.
- [ ] `county_id` is `TEXT NOT NULL`, the same type as `parcels.county_id` at line 8. The `CHECK` list is the `String` values of `county.All`, in that order. After county-1, that list is `('union')` until the next county. A PostgreSQL enum would not match that text column, so the composite foreign key would not type-check.
- [ ] `soil_map_units` is keyed only by `map_unit_key`. It has no county column. `MUKEY` is national.
- [ ] `soil_polygons` primary key is `(county_id, source_name, object_id)`. `shape` is `GEOMETRY(Geometry, 3857)`, matching `parcels.shape` at line 62, with a SRID check and a GiST index.
- [ ] Do not add a JSON document column. Do not write these rows into `parcels.source_extras`. Do not name any table with a `union_` prefix.

```sql
CREATE TABLE soil_map_units (
    map_unit_key TEXT PRIMARY KEY,
    map_unit_symbol TEXT,
    map_unit_name TEXT,
    area_symbol TEXT,
    area_name TEXT,
    fetched_at TIMESTAMPTZ,
    CONSTRAINT soil_map_units_key_nonempty CHECK (map_unit_key <> '')
);

CREATE TABLE soil_polygons (
    county_id TEXT NOT NULL,
    source_name TEXT NOT NULL,
    object_id TEXT NOT NULL,
    map_unit_key TEXT NOT NULL REFERENCES soil_map_units (map_unit_key),
    map_unit_symbol TEXT,
    area_symbol TEXT,
    spatial_version DOUBLE PRECISION,
    source_area NUMERIC(20, 6),
    source_length NUMERIC(20, 6),
    shape GEOMETRY(Geometry, 3857) NOT NULL,
    fetched_at TIMESTAMPTZ NOT NULL,
    PRIMARY KEY (county_id, source_name, object_id),
    CONSTRAINT soil_polygons_county_check CHECK (county_id IN ('union')),
    CONSTRAINT soil_polygons_srid_check CHECK (ST_SRID(shape) = 3857),
    CONSTRAINT soil_polygons_area_check CHECK (
        (source_area IS NULL OR source_area >= 0)
        AND (source_length IS NULL OR source_length >= 0)
    )
);

CREATE TABLE parcel_soils (
    id BIGINT GENERATED BY DEFAULT AS IDENTITY PRIMARY KEY,
    parcel_id UUID NOT NULL REFERENCES parcels(id) ON DELETE CASCADE,
    county_id TEXT NOT NULL,
    source_name TEXT NOT NULL,
    source_parcel_id TEXT NOT NULL,
    map_unit_key TEXT NOT NULL REFERENCES soil_map_units (map_unit_key),
    intersection_area_sq_m NUMERIC(20, 6) NOT NULL,
    area_share NUMERIC(12, 8) NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT parcel_soils_parcel_mukey_unique UNIQUE (parcel_id, map_unit_key),
    CONSTRAINT parcel_soils_source_mukey_unique UNIQUE (county_id, source_name, source_parcel_id, map_unit_key),
    CONSTRAINT parcel_soils_source_fk FOREIGN KEY (county_id, source_name, source_parcel_id)
        REFERENCES parcels (county_id, source_name, source_parcel_id) ON DELETE CASCADE,
    CONSTRAINT parcel_soils_county_check CHECK (county_id IN ('union')),
    CONSTRAINT parcel_soils_area_check CHECK (intersection_area_sq_m > 0 AND area_share >= 0)
);

CREATE INDEX soil_polygons_shape_gist_idx ON soil_polygons USING GIST (shape);
CREATE INDEX soil_polygons_map_unit_key_idx ON soil_polygons (map_unit_key);
CREATE INDEX parcel_soils_source_parcel_idx ON parcel_soils (county_id, source_name, source_parcel_id);
```

**You see.**

- [ ] `make migrate-up` on a database that already has `000001` prints success, and `\d parcel_soils` shows the unique key `(parcel_id, map_unit_key)`. `soil_map_units` has no `county_id` column.

**Verify, unit.** Tests alone are not sufficient verification. A PR is verified only when its unit, live, and perf boxes are all checked.

- [ ] No new Go test. `go test -race ./...` still passes with no database running. Run `rg -n "source_extras|union_soil|CREATE TYPE"` on the soil migration file and require no matches. Run `rg -n "soil_map_units"` on that same file and require a match. Run `rg -n "CHECK \\(county_id IN"` on that file and require the quoted list to equal `county.All` as strings, in order.

**Verify, live.** Tests alone are not sufficient verification. A PR is verified only when its unit, live, and perf boxes are all checked. Ten lanes on `grok-4.7-xhigh-fast` at the PR head, per the boot recipe.

- [ ] Lane 1. Regression lane against trunk. Run `make migrate-up` at trunk and at head. Trunk has no soil tables. county-1 may already have added `parcels_county_id_check`. Record that, and gate the three new tables at head plus a successful migrate of the trunk migrations. Save `/tmp/swarm-soil-3/worker-1/regression.png`. Pass when trunk `\dt` has no `soil_polygons` and head `\dt` lists `soil_map_units`, `soil_polygons`, and `parcel_soils`.
- [ ] Lane 2. Insert one square parcel and one square soil polygon that covers half of it, then run the `INSERT ... SELECT` from Appendix A. Save `/tmp/swarm-soil-3/worker-2/half-overlap.png`. Pass when one `parcel_soils` row exists and `area_share` is between 0.48 and 0.52.
- [ ] Lane 3. Insert two soil polygons with the same `map_unit_key` that both meet the parcel. Also insert a second polygon whose overlap is a thin sliver. Save `/tmp/swarm-soil-3/worker-3/same-mukey.png`. Pass when the same map unit is one row, the sliver is its own row if its key differs, and no row is dropped for a small positive share.
- [ ] Lane 4. Insert a soil polygon that does not meet the parcel. Save `/tmp/swarm-soil-3/worker-4/no-hit.png`. Pass when the join writes no row for that map unit.
- [ ] Lane 5. Insert a second parcel whose `source_parcel_id` is not in the kept list. Save `/tmp/swarm-soil-3/worker-5/not-kept.png`. Pass when the join writes no row for that parcel.
- [ ] Lane 6. Run the replace twice with a changed overlap. Save `/tmp/swarm-soil-3/worker-6/replace.png`. Pass when the parcel still has one row per map unit and the area matches the second geometry.
- [ ] Lane 7. `DELETE FROM parcels WHERE id = ...` for the fixture parcel. Save `/tmp/swarm-soil-3/worker-7/cascade.png`. Pass when `parcel_soils` has no row left for that parcel.
- [ ] Lane 8. Insert a soil polygon with SRID 4326, and insert `county_id` `wake` into `soil_polygons`. Save `/tmp/swarm-soil-3/worker-8/bad-srid.png`. Pass when the SRID check rejects the first insert and the county check rejects the second.
- [ ] Lane 9. Insert a `parcel_soils` row with `area_share` -0.1, and another with a very small positive share. Save `/tmp/swarm-soil-3/worker-9/negative-share.png`. Pass when the negative share is rejected and the small positive share is stored.
- [ ] Lane 10. After a successful soil insert, select `parcels.source_extras` for the fixture parcel. Save `/tmp/swarm-soil-3/worker-10/extras-unchanged.png`. Pass when the value is still `{}`.

**Verify, perf.** Tests alone are not sufficient verification. A PR is verified only when its unit, live, and perf boxes are all checked.

- [ ] Metric. Wall time of the half-overlap join on the fixture. Trunk has no join. The end state is one `parcel_soils` row whose `area_share` is about one half. There is no county-scale time until parcel shapes exist, so do not invent one.
- [ ] Probe. `EXPLAIN ANALYZE` of the Appendix A join on the fixture, at head, after `make migrate-up`. Trunk has no statement to run. Record that before the head timing.
- [ ] Baseline. Record trunk as no join and no elapsed time.
- [ ] Rule. Fail if the fixture join takes 5 seconds or longer, or if `area_share` is outside 0.48 to 0.52. Do not report a ratio against trunk.

**Review gate.** None. soil-3 is not review-gated.

**Merge.**

- [ ] Root's clean verdict at the exact head SHA.
- [ ] Bugbot triage done.
- [ ] Rebased onto soil-1 after the verdict, with `git patch-id` unchanged.
- [ ] The owner does not merge. The operator lands soil-3 after soil-1. Order against soil-2 does not matter.

## Join kept parcels in the shared soil package (soil-4)

**Depends on.** persist-1, soil-2, and soil-3.

**Files.**

- [ ] Create `internal/soil/join.go`.
- [ ] Create `internal/soil/join_test.go`.
- [ ] Create `internal/soil/names.go`.
- [ ] Create `internal/soil/names_test.go`.
- [ ] Edit `main.go`.
- [ ] Do not edit `internal/connector/union/scrape/`. Do not insert into `parcels` from this package. persist-1 owns that write.

**Build.**

- [ ] Add the join statement in Appendix A inside package `soil`. Group by `parcels.id` and `map_unit_key`. Sum `ST_Area(ST_Intersection(parcels.shape, soil_polygons.shape))`. Divide by `ST_Area(parcels.shape)`. Drop groups whose summed area is 0. Keep every group whose summed area is greater than 0, including a sliver. Skip parcels whose `shape` is null or whose area is 0, and return an error if a kept id has no shape. Do not insert the missing parcel.
- [ ] Call `FetchSoilPolygons` once per import, on the `GISSource` that `main` already builds, before `ParcelIDImportService.Run`. Then stream parcels. persist-1 writes shapes. After `Run` returns nil, run the join for the deduped kept ids inside one transaction. Replace this county's polygon rows, then replace `parcel_soils` for those ids. A cancelled context commits nothing.
- [ ] Before the polygon insert, ensure each distinct `MUKEY` has a `soil_map_units` row. If `map_unit_name` is null, POST one query to Soil Data Access for the missing keys and fill `map_unit_name`, `map_unit_symbol`, `area_symbol`, and `area_name`. A name-service error still inserts the stub row with a null name so the geometry join can commit. The next import retries names that are still null. Do not call Soil Data Access once per parcel.
- [ ] If `Run` returns an error, do not start the join. If the soil fetch returns an error, return it from `main`'s run path. Do not log it and continue. `main.go` lines 73 to 81 log a spatialist error and return nil from the hook. Do not copy that. Wait to edit `main.go` until the removal plan's Step 2 has deleted that swallow. If Step 2 is not on trunk, soil-4 stops before the `main.go` edit and reports the blocker.
- [ ] A second successful run replaces `parcel_soils` for the kept ids and deletes kept-id rows whose map unit no longer overlaps. It does not delete soil rows for parcels that were not in this run's kept set.
- [ ] Do not call the scraper. Do not write `source_extras`. Do not add a field to `parcelScraper`. Package `soil` does not import `union`.

**You see.**

- [ ] A httptest import with one kept PID and one rejected PID writes a soil overlap only for the kept PID, and an HTTP 500 from the soil page makes the run return a non-nil error.

**Verify, unit.** Tests alone are not sufficient verification. A PR is verified only when its unit, live, and perf boxes are all checked.

- [ ] `join_test.go` gains a test that the join SQL text names `ST_Intersection`, `soil_polygons`, and the kept-id parameter. `names_test.go` gains an httptest that one POST fills two `MUKEY`s. Run `go test -race ./internal/soil/ -count=1 -run 'TestSoilJoin|TestSoilNames'`. The area math is not claimed by this test. Lane 2 measures it.

**Verify, live.** Tests alone are not sufficient verification. A PR is verified only when its unit, live, and perf boxes are all checked. Ten lanes on `grok-4.7-xhigh-fast` at the PR head, per the boot recipe.

- [ ] Lane 1. Regression lane against trunk. Run `go test -race ./... -count=1` at trunk and at head. Trunk does not load layer 197. Record that, and gate a head run that loads the fixture soil layer and still passes the existing union tests. Save `/tmp/swarm-soil-4/worker-1/regression.png`. Pass when trunk tests pass and head tests pass.
- [ ] Lane 2. Half-overlap fixture in PostGIS through the soil-4 function. Save `/tmp/swarm-soil-4/worker-2/half-overlap.png`. Pass when `area_share` is between 0.48 and 0.52.
- [ ] Lane 3. One batch contains PID `01060017` marked vacant and a second PID with `YEARBLT` 1998. Save `/tmp/swarm-soil-4/worker-3/kept-only.png`. Pass when the join parameter list contains only `01060017`.
- [ ] Lane 4. Soil page returns HTTP 500. Save `/tmp/swarm-soil-4/worker-4/fail-run.png`. Pass when the exported run function returns an error and the test fails if that error is nil.
- [ ] Lane 5. Cancel after the count query and before the first geometry page. Save `/tmp/swarm-soil-4/worker-5/cancel.png`. Pass when no row is committed to `soil_polygons`.
- [ ] Lane 6. Run the join twice. Save `/tmp/swarm-soil-4/worker-6/twice.png`. Pass when `count(*)` for the parcel stays equal to the distinct map-unit count.
- [ ] Lane 7. Remove one overlapping map unit from the second snapshot and run again. Save `/tmp/swarm-soil-4/worker-7/drop-stale.png`. Pass when the stale `map_unit_key` is gone.
- [ ] Lane 8. Call the id hook and the soil loader. Save `/tmp/swarm-soil-4/worker-8/hook-ids.png`. Pass when the hook argument type is still `[]string` and the soil loader is not invoked from inside `extractParcelIDs`.
- [ ] Lane 9. `rg -n "source_extras|INSERT INTO parcels" internal/soil/` and a package import list. Save `/tmp/swarm-soil-4/worker-9/boundaries.png`. Pass when neither pattern matches and package `soil` does not import `union`.
- [ ] Lane 10. Kept id list empty. Save `/tmp/swarm-soil-4/worker-10/empty-kept.png`. Pass when the polygon copy can still commit, the join writes zero `parcel_soils` rows, and the run returns nil.

**Verify, perf.** Tests alone are not sufficient verification. A PR is verified only when its unit, live, and perf boxes are all checked.

- [ ] Metric. Soil HTTP calls during one import against a fixture whose count is 11250, and the nil-vs-error result of a failed page. Trunk makes no soil calls. The end state the user waits on is soil rows for the kept ids, or a returned error when the fetch fails.
- [ ] Probe. httptest request counter around the import function, at head, after a trunk run that records zero soil calls. Interleave trunk then head.
- [ ] Baseline. Record trunk soil HTTP calls as 0 first.
- [ ] Rule. Fail if the fixture import issues more than 7 soil HTTP calls for count 11250 and page size 2000. Fail if a soil HTTP 500 still returns nil. Do not report a ratio against trunk.

**Review gate.** None. soil-4 is not review-gated.

**Merge.**

- [ ] Root's clean verdict at the exact head SHA.
- [ ] Bugbot triage done.
- [ ] Rebased onto the stacked persist-1, soil-2, and soil-3 heads after the verdict, with `git patch-id` unchanged.
- [ ] The owner does not merge. The operator lands soil-4 last.

## Close the program

- [ ] Every box above is checked with its evidence.
- [ ] Reply to the operator with each PR id, its verdict, and the head SHA. Do not claim a merge.

## Appendix A. Prototype evidence and layer findings

No prototype branch and no prototype SHA. The user barred Go changes, so the join was settled with live HTTP against the MapServer. The responses below were captured on 2026-11-08. A later owner re-measures the count before trusting the page budget, because the county can republish the layer.

### Service and layer

**Measured.** `GET https://atlas.unioncountync.gov/server/rest/services/OperationalLayers/MapServer?f=json` returned HTTP 200, 17,242 bytes, in 0.129 seconds. `mapName` is `OperationalLayersNew`. `maxRecordCount` is 2000. The layer list has 72 entries. The only name that matched soil, SSURGO, mukey, or hydric is layer id 197, name `Soils`, type Feature Layer, geometry `esriGeometryPolygon`, parent group `Natural` id 196. Layer 215 is `Parcels`, the same geometry type.

**Measured.** The same host's service list at `https://atlas.unioncountync.gov/server/rest/services?f=json` includes `OperationalLayers`. This plan does not crawl the other services for a second soil layer. The GIS source does not query them.

**Read.** `DefaultGISSourceConfig` sets `BaseURL` to `https://atlas.unioncountync.gov/server/rest/services/OperationalLayers/MapServer/215/query`, `RateLimit` 1, and `BatchSize` 50. `internal/connector/union/gis.go` lines 24 to 30.

**Measured.** `GET https://atlas.unioncountync.gov/server/rest/services/OperationalLayers/MapServer/197?f=json` returned HTTP 200, 24,523 bytes, in 0.113 seconds. Layer id 197. Name `Soils`. Type Feature Layer. Geometry `esriGeometryPolygon`. `maxRecordCount` 2000. Capabilities `Map,Query,Data`. Extent spatial reference `wkid` 102100 and `latestWkid` 3857. Layer 215 reports that same pair. `parcels.shape` is `GEOMETRY(Geometry, 3857)` at `db/migrations/000001_init_schema.sql` line 62. `geometryProperties.units` is `esriMeters`. `shapeAreaFieldName` is `Shape.STArea()`. `shapeLengthFieldName` is `Shape.STLength()`. Display field `AREASYMBOL`. Unique index on `OBJECTID_1`. The renderer is unique values on `MUSYM` with 48 entries. `description` and `copyrightText` are empty.

**Measured.** Fields on layer 197, in service order.

| Name | Type | Nullable | Length |
| --- | --- | --- | --- |
| OBJECTID_1 | esriFieldTypeOID | false |  |
| OBJECTID | esriFieldTypeInteger | true |  |
| AREASYMBOL | esriFieldTypeString | true | 20 |
| SPATIALVER | esriFieldTypeDouble | true |  |
| MUSYM | esriFieldTypeString | true | 6 |
| MUKEY | esriFieldTypeString | true | 30 |
| HydrcRatng | esriFieldTypeString | true | 254 |
| Shape_Leng | esriFieldTypeDouble | true |  |
| Shape | esriFieldTypeGeometry | true |  |
| Shape.STArea() | esriFieldTypeDouble | true |  |
| Shape.STLength() | esriFieldTypeDouble | true |  |

**Measured.** No field is a parcel id, a PID, an account number, or a parcel address. The join cannot be an attribute match.

**Measured.** `where=1=1` and `returnCountOnly=true` on layer 197 returned `{"count":11250}`. The same query on layer 215 returned `{"count":118202}`.

**Measured.** Distinct `AREASYMBOL` is only `NC179`. Min and max `SPATIALVER` are both 5. Grouped `HydrcRatng` is `Not Hydric` on 10,790 polygons and `Partially Hydric` on 460. 10,790 plus 460 is 11,250. Distinct `(MUSYM, MUKEY, HydrcRatng, AREASYMBOL, SPATIALVER)` returned 48 rows, one hydric value per map unit. The first sample feature is `OBJECTID_1` 1, `OBJECTID` 1, `AREASYMBOL` `NC179`, `SPATIALVER` 5, `MUSYM` `ChA`, `MUKEY` `115341`, `HydrcRatng` `Partially Hydric`.

**Measured.** Soil Data Access confirms `NC179`. A POST to `https://sdmdataaccess.sc.egov.usda.gov/Tabular/post.rest` with `format=JSON` and `SELECT TOP 3 areasymbol, areaname, lkey FROM legend WHERE areasymbol='NC179'` returned `NC179`, `Union County, North Carolina`, and `lkey` `11048`. `SELECT ... FROM mapunit WHERE areasymbol='NC179'` returned HTTP 400. `areasymbol` is a `legend` column, not a `mapunit` column. The join `mapunit.lkey = legend.lkey` for `MUKEY` `115341` returned `ChA`, the official name, `NC179`, and `Union County, North Carolina`.

**Measured.** A request with `resultRecordCount=5000`, `returnGeometry=false`, and `outFields=MUKEY,MUSYM` returned 2000 features and `exceededTransferLimit` true, in 0.203 seconds and 96,735 bytes. Offset 10,000 with `resultRecordCount=2000` returned 1,250 features and no `exceededTransferLimit`. 5 times 2,000 plus 1,250 is 11,250. The server enforced the 2,000 cap on the call that asked for 5,000 rows.

**Measured.** The first 200 features with geometry and the stored attribute set were 967,418 bytes in 0.365 seconds. `OBJECTID_1` 1 alone was 10,037 bytes and 238 ring points. Twenty features at offset 5,000 were 60,496 bytes in 0.156 seconds.

**Inferred.** Scaling the first 200 features across 11,250 features gives about 54 million bytes. Scaling the offset-5000 sample gives about 34 million bytes. Those are not a measurement of the full layer. The request count does not depend on which one is closer. The limiter on the pull is the 2,000 record cap, which the 5,000 request already hit.

### How a parcel meets a soil polygon

**Measured.** `PID` `01060017` from layer 215, `YEARBLT` 0, `SQFT` 0.0, `FMV_IMPRV` 0.0, `LAND_TYPE` `RURAL ACRES`, `mapped_acres` 10, one ring, 15 points. A POST to layer 197 with `spatialRel=esriSpatialRelIntersects`, `geometryType=esriGeometryPolygon`, and `inSR=102100` returned 3 features, 3 distinct `MUKEY` values, and `{"count":3}` on the count-only form. Attribute keys on the hit were `AREASYMBOL`, `HydrcRatng`, `MUKEY`, `MUSYM`, `SPATIALVER`, and `Shape.STArea()`. There is no overlap-area field. `Shape.STArea()` on those hits was 1,025,837.004717505 for `MUKEY` `115333`, MUSYM `BaB`, 2,618,764.610728555 for `115352`, MUSYM `GsB`, and 30,229.233794095 for `2440232`, MUSYM `BdB2`. The geometry form of the same query returned 972 ring points and 40,636 bytes. The parcel request body was 1,501 bytes.

**Measured.** `PID` `04213010`, `mapped_acres` 1592.464, `YEARBLT` 0, `LAND_TYPE` `RURAL ACRES`, 5 rings, 472 points, returned 84 soil features and 17 distinct `MUKEY` values in one response, 13,119 bytes, 0.145 seconds, `exceededTransferLimit` absent. `BaB` alone accounted for 14 of those polygons.

**Inferred.** A single kept parcel stays under the 2,000 feature cap, so a per-parcel spatial query is one HTTP call for parcels of this size. That is not proven for every parcel in the county. It does not change the request budget below, because the kept set is still tens of thousands of calls.

**Read.** `geometryShape` reads `RawData["geometry"]` and builds a polygon at SRID 3857. `translator.go` lines 311 to 335. `FetchBatches` asks for geometry. `gis.go` lines 123 to 130. `extractParcelIDs` keeps only `record.ParcelID`. `internal/service/parcel_id_importer.go` lines 103 to 125. `ParcelIDBatchHook` is `func(ctx, batchIndex, parcelIDs []string) error` at lines 10 to 12. The hook does not receive a polygon. A spatial query from inside the hook would have to fetch layer 215 again or the importer would have to change.

### Kept-parcel count

filter-1 keeps the live numeric-zero rule and deletes two clauses that match nothing. The gut that numeric `FMV_IMPRV` 0 or every `RURAL ACRES` row is vacant is wrong on this feed.

**Read.** `IsLikelyVacantGISRecord` returns true when `YEARBLT` or `SQFT` is numeric 0, when `FMV_IMPRV` is the string `0` or `$0`, or when `LAND_TYPE` contains `RURAL ACREAGE TRACT`. `vacant_filter.go` lines 18 to 62. `isZeroNumeric` accepts `int`, `int64`, and `float64`. Lines 73 to 82. `isZeroCurrency` accepts only a string. Lines 85 to 92. The tests pass `YEARBLT` as `int` 0 and `FMV_IMPRV` as `$0`. `vacant_filter_test.go` lines 16 to 47.

**Read.** The fetch path stores `gjson.Result.Value()`. `gis.go` lines 214 to 220. In gjson 1.18.0, `Result.Num` is `float64` and `Value()` returns `t.Num` for a JSON number. `gjson.go` lines 63 and 673 to 674. A live JSON number therefore hits the `float64` branch, not the currency-string branch.

**Measured.** Layer 215 has 118,202 parcels. `YEARBLT = 0 OR SQFT = 0` returned 25,885. Both zero returned 25,844. `YEARBLT = 0` returned 25,844. `SQFT = 0` returned 25,885. `YEARBLT > 0` returned 90,396. `SQFT > 0` returned 90,357. `YEARBLT IS NULL` returned 1,962. `SQFT IS NULL` returned 1,960. Both null returned 1,960.

**Measured.** `FMV_IMPRV = 0` returned 116,242. `FMV_IMPRV > 0` returned 0. `FMV_IMPRV IS NULL` returned 1,960. 116,242 plus 1,960 is 118,202. `FMV_IMPRV = 0 AND YEARBLT > 0 AND SQFT > 0` returned 90,357. That is every parcel that has both a year and a square footage. `FMV_IMPRV = 0 AND (YEARBLT = 0 OR SQFT = 0)` returned 25,885, the same set the current function already keeps.

**Measured.** `LAND_TYPE = 'RURAL ACRES'` returned 30,861. `LIKE '%RURAL ACREAGE TRACT%'` returned 0. `LIKE '%RURAL%'` returned 30,861. Every one of those 30,861 rows also has `FMV_IMPRV = 0`. `RURAL ACRES AND (YEARBLT = 0 OR SQFT = 0)` returned 11,720, already inside the 25,885. `RURAL ACRES AND YEARBLT > 0 AND SQFT > 0` returned 19,141. Adding the land type to the filter would keep 45,026 parcels, and the extra 19,141 are those improved rural rows. `YEARBLT IS NULL AND LAND_TYPE = 'RURAL ACRES'` returned 2.

**Measured.** Edge rows, attributes only. `PID` `01201017K` has `YEARBLT` 2025, `SQFT` 3391, `FMV_IMPRV` 0, `FMV_LAND` 0, `FMV_TOTAL` 734200, `LAND_TYPE` `LOT - SIZE COMPONENT`, `STRUCTTYPE` `Site Built Housing`. `PID` `06186022` has `YEARBLT` 1997, `SQFT` 22797, `FMV_IMPRV` 0, `FMV_TOTAL` 7114900, `LAND_TYPE` `RURAL ACRES`, `STRUCTTYPE` `Site Built Housing`. `PID` `04213010` has `YEARBLT` 0, `SQFT` 0, `LAND_TYPE` `RURAL ACRES`, and is already inside the 25,885.

**Inferred.** Numeric improvement value is unpopulated. It is 0 on houses built in 2025 and on rural houses, while `FMV_TOTAL` is large. It cannot mark vacancy. `RURAL ACRES` is the live name of the dead `RURAL ACREAGE TRACT` phrase, and most of those parcels already have a year and a square footage. filter-1 therefore keeps `YEARBLT = 0 OR SQFT = 0`, deletes the currency-string clause and the acreage-tract clause, and does not add either new signal. The live kept count stays 25,885.

### Request budget

The decision this number has to support is bulk load versus one spatial query per kept parcel. A kept set smaller than six parcels would make per-parcel queries cheaper in HTTP calls. Anything larger favors the bulk load. The noise floor is zero here. Both counts are exact server counts, not timers.

**Measured.** Bulk pages are `ceil(11250 / 2000)` which is 6, plus one count query, which is 7 HTTP calls. The server refused to return 5,000 rows in one call. Those page checks sent `orderByFields=OBJECTID_1`. Offset 10000 then returned the last 1,250 rows.

**Read.** `ArcGISParams` has no `orderByFields` field. `gis.go` lines 40 to 49. `FetchBatches` advances `resultOffset` by `i * batchSize` even when a page is short. `gis.go` line 141. A soil pull that reuses that loop without an order can skip polygons. The soil client orders on `OBJECTID_1`, pages 2000 rows, and returns an error when the pages do not add up to the count.

**Measured.** The Go rate limit is 1 request per second, burst 1. `NewRateLimitedClient` at `internal/connector/shared/httpclient/http_client.go` lines 63 to 70. `DefaultGISSourceConfig` passes 1. `Do` waits on `ctx`. Lines 24 to 28. `NewGISSource` stores that client on the source. `gis.go` lines 69 to 75. It is not exported.

**Inferred.** 25,885 spatial queries at that limiter wait about 25,885 seconds, which is about 7.2 hours, before transfer time. Four timed spatial calls returned in 0.085 to 0.173 seconds, so the server was faster than the limiter on those four calls. That sample is not a timing of the full kept set. The request count is the limiter that matters, and it is exact. Why the bulk page count is not three calls is the measured cap. A request for 5,000 rows returned 2,000.

**Inferred.** Doing the spatial query from the id hook adds a second call per parcel to reload geometry, unless the ids are batched. The geometries are already on the `RawRecord` until `extractParcelIDs` drops them. Reloading them is wasted work. The area share is still not in the ArcGIS response, so those calls would download full soil polygons. The 10 acre parcel's three soil polygons arrived as 972 points.

### Struct and table

**Read.** `parcels.county_id` is `TEXT NOT NULL`. `000001_init_schema.sql` line 8. The unique key is `(county_id, source_name, source_parcel_id)` at line 73. There is no `CREATE TYPE` in that file. A PostgreSQL enum for county would not match that text column, so a composite foreign key from a soil table would fail to type-check. The county column stays text, with `CHECK (county_id IN ('union'))` until the next county is added. The Go type is `county.ID` in `internal/county`, with `county.Union` and `Parse`. county-1 in `plans/county-id-package.md` owns that type. Soil does not declare it. Adding a county adds a constant in that package and extends the check list in a new migration. `source_name` stays plain text. Union's value is `union_county_gis`.

**Read.** `internal/models.GISParcel` is the parcel row. The removal plan is moving Union types out of `internal/models`. Soil polygons, overlaps, and national map units are their own domain, so they live in `internal/soil`. The county id does not live in `internal/soil`. `Polygon.County` uses `county.ID` from `internal/county`. Package `soil` does not import `union`. The Union adapter imports `soil` and is the only place that knows layer 197's URL and field names. `internal/models` stays the parcel. A second soil package is not a junk drawer of every future county type.

Two structs, because a soil polygon and a parcel overlap are not the same row. A third table holds the national name.

`soil.Polygon` is one county soil feature. The primary key is county, source name, and `ObjectID` as text, so the next county is not forced to use an integer `OBJECTID_1`. Union writes the decimal form of `OBJECTID_1`. Fields are `County` of type `county.ID`, `SourceName`, `ObjectID`, `MapUnitKey`, `MapUnitSymbol`, `AreaSymbol`, optional `SpatialVersion`, optional source area and length, `Shape` at SRID 3857, and `FetchedAt`. `HydrcRatng` stays on the raw Union feature. It is not a shared column. 11,250 polygons, 48 map units.

`soil.Overlap` is one kept parcel plus one `MUKEY`. The 1,592 acre parcel hit 14 polygons of `BaB` and still only one map unit. Store the summed overlap. Store every group whose intersection area is greater than 0. There is no minimum share. `IntersectionAreaSqM` is `ST_Area` of the intersection in SRID 3857, planar meters. `AreaShare` is that area divided by `ST_Area(parcels.shape)`. The share is a ratio, so 0.25 means 25 percent. A share above 1 stays. Do not store the soil polygon's `Shape.STArea()` on this row.

`soil_map_units` is keyed only by `MUKEY`. Columns are `map_unit_key`, `map_unit_symbol`, `map_unit_name`, `area_symbol`, and `area_name`. Names stay null until filled. The table is not county-specific.

**Measured.** One Soil Data Access query `SELECT mukey, musym, muname FROM mapunit WHERE mukey IN (...)` resolved all 48 Union keys from the distinct-value sample. Zero `MUSYM` mismatches. `115341` is `ChA`, `Chewacla silt loam, 0 to 2 percent slopes, frequently flooded`. `115333` is `BaB`, `Badin channery silt loam, 2 to 8 percent slopes`. `115328` is `AeB`, `Ailey-Appling complex, 2 to 8 percent slopes`. The response for the 48 keys was one HTTP 200. A second host, `https://SDMDataAccess.nrcs.usda.gov/Tabular/post.rest`, returned the same shape for a one-row query.

soil-4 inserts a stub `soil_map_units` row for each distinct key before the polygon insert, then fills null names with one POST per import. A name-service failure does not fail the geometry join. The next import retries rows whose name is still null. Do not call Soil Data Access once per parcel. Do not put this client on the Union GIS limiter. It is a different host.

The overlap row carries a county id of type `county.ID`, plus `SourceName` and `SourceParcelID`, because that is the identity `GISTranslator` writes. `translator.go` lines 57 to 63. The database row also has `parcel_id UUID` so `ON DELETE CASCADE` matches `parcel_sales`. Both foreign keys are filled by one `INSERT ... SELECT` from `parcels`. `map_unit_key` references `soil_map_units`.

County two plugs in without a new interface. Add a `county.ID` constant in `internal/county`, extend the check list in a new migration, add `internal/connector/<county>` with a function that returns `[]soil.Polygon`, and call it from `main`'s county list. Shared tables, the join statement, and the name client do not change. Do not add a swappable interface until that second mapper exists. One implementation is indirection.

`source_name` for Union polygons and overlaps is `union_county_gis`, from `GISSource.Name` at `gis.go` line 79.

### Join statement

`$1` is the county id, `$2` is the source name, and `$3` is the kept ids as `text[]`. Area is planar in SRID 3857. The ratio cancels the Web Mercator scale factor when both shapes use that SRID. The absolute square meters are not geodesic acres.

```sql
WITH overlap AS (
    SELECT
        p.id AS parcel_id,
        p.county_id,
        p.source_name,
        p.source_parcel_id,
        s.map_unit_key,
        ST_Area(p.shape) AS parcel_area,
        sum(ST_Area(ST_Intersection(p.shape, s.shape))) AS intersection_area
    FROM parcels AS p
    JOIN soil_polygons AS s
        ON s.county_id = p.county_id
       AND s.source_name = p.source_name
       AND ST_Intersects(p.shape, s.shape)
    WHERE p.county_id = $1
      AND p.source_name = $2
      AND p.source_parcel_id = ANY($3)
      AND p.shape IS NOT NULL
      AND ST_Area(p.shape) > 0
    GROUP BY p.id, p.county_id, p.source_name, p.source_parcel_id, s.map_unit_key
)
INSERT INTO parcel_soils (
    parcel_id, county_id, source_name, source_parcel_id,
    map_unit_key, intersection_area_sq_m, area_share
)
SELECT
    parcel_id, county_id, source_name, source_parcel_id,
    map_unit_key, intersection_area,
    intersection_area / parcel_area
FROM overlap
WHERE intersection_area > 0;
```

Replace rows for the kept ids inside the same transaction before this insert. Do not delete soil for source parcel ids that are not in `$3`.

### Where it runs

**Read.** `FetchBatches` returns `ctx.Err()` when the context is cancelled, and it returns the hook error unchanged. `gis.go` lines 133 to 154. The importer calls the filter, then the id hook. `parcel_id_importer.go` lines 63 to 71. An empty kept batch returns nil and does not call the hook. Lines 68 to 70.

**Read.** The scraper plan runs `EnrichParcelIDs` inside that id hook, one id at a time, and returns the first error. `plans/scraper-enrichment.md` lines 36 to 47 and 79 to 87. Soil is not that scraper. The scraper is a per-parcel document from a site this plan does not know. Soil is a county polygon layer. Putting soil behind `parcelScraper` would hide a six-call bulk load behind a per-id interface. The scraper plan says to stop and use a batch source if the other site is a bulk feed. Soil is already that case, and it stays next to `GISSource` because it is the same MapServer.

Load the soil polygons once, with `FetchSoilPolygons`, on the GIS source's existing limiter, before `Run`. persist-1 writes kept shapes during `Run`. After `Run` returns nil, write polygons, map-unit stubs, names, and parcel soils in one transaction. If the context is cancelled, commit nothing. If the fetch or the join returns an error, return that error. A missing `parcels.shape` is an error. soil-4 does not insert parcels.

**Read.** No other plan file inserts into `parcels`. A search of the module found no `INSERT INTO parcels`. persist-1 is that writer, and it is a prerequisite of soil-4 inside this plan. `internal/service` does not import `union` or `parcelstore`. `main` already constructs `union.NewGISSource`. `main.go` lines 20 to 23. The soil join call stays in `main` and calls package `soil`. It does not enter `internal/service`.

## Appendix B. Alternatives rejected

Per-parcel `esriSpatialRelIntersects` from the id hook loses on the request count. 25,885 kept parcels are one spatial query each, or two if the hook reloads geometry. The limiter allows one request per second. The bulk load is six pages. The spatial response also omits the overlap area, so the client would still clip. PostGIS already stores `parcels.shape` in the same SRID. `ST_Intersection` is the clip. A Go polygon clipper would be a second geometry engine beside PostGIS.

Union-only tables lose. Every county needs the parcel-soil relationship, so the tables and the Go types are shared. A PostgreSQL enum for county loses because `parcels.county_id` is text and the composite foreign key has to match it. A swappable soil interface loses until a second county mapper exists. Putting the types in `internal/models` loses because that package is the parcel row and the removal plan is already pulling Union types out of it.

`parcels.source_extras` loses. One JSON object cannot be the unique key for many map units, and the scraper plan reserves that column for leftover GIS attributes.

Rewriting `BaseURL` into a MapServer root plus layer ids loses for this change. `TestDefaultGISSourceConfig` and the other GIS tests pass a full query URL. A second explicit `SoilQueryURL` matches the existing field. A third layer is the time to collapse them.

Running the spatial query inside each id batch loses. The soil layer does not change per batch. Interleaving it with parcel pages on a one-request-per-second limiter only stretches the import.

Storing one row per soil polygon per parcel loses. The 1,592 acre parcel would store 84 rows, including 14 for one map unit. The user asked for parcel id plus map unit. `MUKEY` is that key. `MUSYM` is unique inside this one `AREASYMBOL`, and `MUKEY` is the key that stays unique if a second survey area ever appears on the layer.

## Appendix C. Risks

soil-4 waits on persist-1 for shapes and on the removal plan Step 2 for the hook that logs an error and returns nil. persist-1 is in this plan. Until Step 2 is on trunk, soil-4's `main.go` edit stays unstarted. filter-1, soil-1, soil-2, and soil-3 do not need that swallow deleted. persist-1 and soil-4 both edit `main.go`, so persist-1 lands first.

`control-cli` is not installed here. Live lanes drive `go test`, `curl`, and `psql` and save the command output as the screenshot path. A lane that cannot draw still returns that file.

This repo has no `pstack/` tree. The `git show origin/main:pstack/...` lines fail until those files exist on trunk. The owner reads the playbook from the installed poteto-mode skill and records that fallback.

`area_share` can land slightly above 1 when boundaries overlap or when a sliver's planar area mis-adds. The check constraint only requires a share of at least 0. A share above 1 stays visible. Do not clamp it in SQL.

The kept-set count is a property of today's feed. filter-1 does not change the live count. A later change that treats numeric `FMV_IMPRV` 0 as vacant would expand the id list to 116,242 and would mark houses vacant. It would not expand the soil download.

Web Mercator meters are a poor acreage. The share is safe because both areas use SRID 3857. Do not label `intersection_area_sq_m` as geodesic acres in the UI. There is no UI in this plan.

The live layer can gain fields or rows. soil-2 fails the fetch when the paged feature count does not match the count query. It does not fail when a new attribute appears. `ParseSoilFeature` ignores unknown attributes. A renamed `MUKEY` fails the missing-key test.

## Appendix D. Links and reading list

Read before editing.

- `internal/connector/union/gis.go` lines 24 to 30, 40 to 49, 69 to 75, 123 to 154, 173 to 185, and 214 to 220.
- `internal/connector/union/vacant_filter.go` lines 18 to 92.
- `internal/service/parcel_id_importer.go` lines 10 to 12 and 63 to 125.
- `db/migrations/000001_init_schema.sql` lines 5 to 73 and 94 to 97.
- `plans/scraper-enrichment.md` lines 36 to 87.
- `plans/county-id-package.md` for `county.ID`, `Parse`, and the parcels check migration.
- `plans/spatialist-removal-and-critical-fixes.md` Step 2 and Step 6.
- `internal/connector/shared/httpclient/http_client.go` lines 24 to 28 and 63 to 70.

Live URLs the owner re-checks.

- `https://atlas.unioncountync.gov/server/rest/services/OperationalLayers/MapServer?f=json`
- `https://atlas.unioncountync.gov/server/rest/services/OperationalLayers/MapServer/197?f=json`
- `https://atlas.unioncountync.gov/server/rest/services/OperationalLayers/MapServer/197/query?where=1%3D1&returnCountOnly=true&f=json`
- `https://atlas.unioncountync.gov/server/rest/services/OperationalLayers/MapServer/215/query?where=1%3D1&returnCountOnly=true&f=json`
- `https://sdmdataaccess.sc.egov.usda.gov/Tabular/post.rest`

soil-2 and soil-4 get `pstack/skills/how/SKILL.md` before the fetch loop is edited, because the question is where the second query sits on `GISSource`. No PR gets `pstack/skills/interrogate/SKILL.md`. The join choice is already measured. A decision trail for the execution run follows `pstack/skills/show-me-your-work/SKILL.md`. This plan file is the trail for the investigation.

## Appendix E. Decisions recorded

The earlier open questions are closed.

The model is shared. `internal/county` holds the county id. `internal/soil` holds `Polygon` and `Overlap`, and the county field uses `county.ID`. Tables are `soil_polygons`, `parcel_soils`, and `soil_map_units`. Union keeps only the layer 197 URL and the field mapping. County id is check-constrained text, matching `parcels.county_id`.

The vacant filter does not widen. Numeric `FMV_IMPRV` 0 is 116,242 rows, including 90,357 parcels with a year and a square footage. `RURAL ACRES` is 30,861 rows, and 19,141 of them already have a year and a square footage. filter-1 deletes the two dead clauses and keeps the 25,885 numeric-zero parcels.

Every overlap whose intersection area is greater than 0 is stored. There is no minimum share.

Official names come from Soil Data Access `mapunit.muname`. The live query resolved all 48 Union keys. `soil_map_units` caches them. A failed name lookup leaves the name null and does not fail the join.

persist-1 writes kept parcels, including `shape`, before the soil join. soil-4 does not insert parcels. No other plan file owns that writer, so the step is a section of this plan.
