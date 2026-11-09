# Give each sale its own UUID

A sale row keeps `parcel_id` so it still points at `parcels.id`, and it gains its own UUID. That UUID is the application id for the sale. The county parcel number is only the lookup key used on ingest.

Soil is a geometry, not a parcel. A soil polygon gets its own UUID. It meets parcels through a join of polygon id, parcel id, and overlap area.

The program is uuid-1, then uuid-2. uuid-1 changes the sale primary key. uuid-2 adds writers that keep those ids stable on a second save.

## How to read this

One box is one unit of work. Every box names the evidence that checks it. A nested box is a sub-step of the box above it. Check a box only when its evidence exists, a file, a log line, a screenshot, a test run, or a SHA. The body is a how-to. The appendices explain and record.

The program runs `pstack/skills/poteto-mode/playbooks/autopilot-stack.md`. That file is not on `main`. Read the installed poteto-mode copy of `playbooks/autopilot-stack.md` when `git show` cannot find it. Implementers stop at merge-ready. The operator lands uuid-1, then uuid-2.

Tests alone are not sufficient verification. A PR is verified only when its unit, live, and perf boxes are all checked.

## Key rows by our ids

Per-parcel data references `parcels.id`. Sales do that with `parcel_sales.parcel_id`. A future tax table does the same. On ingest, find `parcels.id` with `(county_id, source_name, source_parcel_id)`. Do not store the county parcel number as the application id.

`county_id` is `county.ID` in `internal/county`. The column is `TEXT` plus a `CHECK`. Geometry is `GEOMETRY(Geometry, 3857)`. Spatial joins stay in PostGIS. Parcel persistence is its own step, which is uuid-2. County-specific field names stay in `internal/connector/<county>`.

A soil polygon is not keyed by a parcel. Give the polygon a UUID, a MUKEY, and a survey version. The join table holds the soil polygon id, the parcel id, and the overlap area. The map paints the polygon, then asks which parcels it touches.

Do not delete parcels that a county split or merge retired. That waits for a later parcel diff. uuid-1 and uuid-2 do not add that delete.

## Program checklist

### Arm the program

- [ ] State the protocol and this plan to the operator, then stop. Start execution only on the operator's explicit go.
- [ ] Read these from trunk at program start. Re-read them at every tick.
  - [ ] `git show origin/main:pstack/skills/poteto-mode/playbooks/autopilot-stack.md`
  - [ ] `git show origin/main:pstack/skills/swarm/SKILL.md`
  - [ ] `git show origin/main:pstack/skills/control-cli/SKILL.md`
  - [ ] `git show origin/main:pstack/skills/poteto-mode/playbooks/opening-a-pr.md`
  - [ ] `git show origin/main:pstack/skills/poteto-mode/playbooks/shipping.md`
- [ ] When `git show` fails, read the installed poteto-mode playbooks. Do not add a `pstack/` tree in uuid-1 or uuid-2.
- [ ] On the operator's go, arm the audit tick as `/loop 1h` with the tick prompt below. Never leave the cadence to memory.
- [ ] Use this tick prompt, verbatim. "Re-read the execution playbook from trunk. Audit the operation against it and fix drift in this tick. Probe every active lane and judge progress by side effects only. Stand down a stuck lane and dispatch its replacement now. Then post a short status message to the operator in chat only when the audit found a tracked change that no earlier status message reported, such as a PR opened, a code-ready head, a round launched or closed, a verdict, a merge, a stuck agent and the action taken, a blocker added or cleared, or a decision only the operator can make. Name every such change and nothing else. Do not repeat a table, the merged list, or an unchanged blocker. If the audit found none, end the turn with no reply text. Either way, log this tick's row in your decision trail. The row names the items reported, or none."
- [ ] On the operator's hold or stand-down, send every owner a zero-writes order at once.

### Spawn owners

- [ ] Spawn one owner per PR with the full lifecycle in `playbooks/autopilot-stack.md`. Each owner builds, opens the PR, and stops at merge-ready.
- [ ] Follow this dependency graph. uuid-2 is stacked on uuid-1 because the sale writer needs the UUID column.
  - [ ] uuid-1 is first. It branches from `main`.
  - [ ] uuid-2 after uuid-1. Its branch starts at the uuid-1 head, and its PR base is the uuid-1 branch.
- [ ] Hold the file boundaries. uuid-1 touches only `db/migrations/000004_parcel_sales_uuid_id.sql`, or the next free prefix named in that PR. uuid-2 touches only `internal/parcelstore/`, `internal/models/gis_parcel.go`, `go.mod`, and `go.sum`.
- [ ] Hold the review gate. Neither PR changes an interaction. Neither waits for a review video.

### PR mechanics, for every PR

- [ ] Resolve the forge once. Default to `gh`. If `command -v origin` succeeds and Origin can resolve the repository, use `origin pr` for every PR operation. Record any fallback to `gh`. Never require `gt`.
- [ ] Open the PR ready, never draft, per **Opening a PR**. Use the run's built-in PR tool when it has one, else `origin pr create --status open --base <base-branch>` or `gh pr create --base <base-branch>` according to the resolved forge. A stack child targets its parent branch.
- [ ] Run the repo's lint and typecheck once before the PR-facing push. Push with hooks on. The repo command is `go test -race ./... -count=1`.
- [ ] Run `/deslop` before each commit and `/no-comments` before review. If those skills are not installed, record the miss in the PR and continue. Do not add the skills in these PRs.
- [ ] Triage every Bugbot and security-reviewer comment per `../references/bugbot-triage.md`.
- [ ] Rebase onto current trunk before the code-ready report and babysit. uuid-2 rebases onto the uuid-1 branch instead, until uuid-1 has merged. Keep that merge base in fix rounds. Rebase again only at merge prep, on a `git merge-tree` conflict with trunk, on a CI failure that comes from a change on trunk, or when a known concurrent change to the same files lands.

### Verdict and merge, for every PR

- [ ] A full swarm runs once, at the code-ready head. Another full swarm runs only for new behavior, a redesign, or a large new diff. A rebase, a merge-conflict resolution, or a minor fix gets a single verifier at the new head, and a wording-only change gets none. Run each full swarm per `pstack/skills/swarm/SKILL.md`, or the installed swarm skill when that path is absent on trunk. One gates lane. The ten live lanes from the PR's **Verify, live** block. The perf lane from its **Verify, perf** block. Two or more audit lanes, each with its own focus, that read the diff and the receipts and distrust the PR body. The root audits the receipts in the merge-ready report before the verdict.
- [ ] Judge every finding by the execution playbook's blocking rule. The verdict is clean when none blocks. A failure on trunk behavior in a lane marked `Regression lane against trunk.` is recorded against trunk and does not block the PR. A new head gets a fresh verdict, except for results that stay valid under the patch-id rule in `playbooks/shipping.md`.
- [ ] The owner stops at merge-ready and does not merge. The operator lands the stack bottom-up, uuid-1 then uuid-2. A verdict survives a restack only when `git patch-id` is unchanged, per `playbooks/shipping.md`.

### Boot recipe, for every live lane

Each live lane runs on its own cloud VM at the PR head. `control-cli` is not installed and this repo has no control skill. Drive the lane with `psql` and `go test`. Read-only checks are `SELECT` and `\d`.

- [ ] `git fetch origin <head-branch> && git checkout <head SHA>`.
- [ ] Start Postgres the way the repo does. `docker compose up -d postgis`, then `make db-create`, then `make migrate-up`. Wait until `psql` can run `SELECT 1`.
- [ ] Deliver input only through `psql` and `go test -race ./... -count=1`. Name the read-only diagnostics as `SELECT` and `\d`.
- [ ] Save every text render to `/tmp/swarm-<pr-id>/worker-<n>/<slug>.png` and return the paths with the report.

## Change the sale id to a UUID (uuid-1)

**Depends on.** None.

**Files.**

- [ ] Create `db/migrations/000004_parcel_sales_uuid_id.sql`.
- [ ] Do not edit `db/migrations/000001_init_schema.sql`, `db/migrations/000002_parcels_storage_srid.sql`, or `db/migrations/000003_parcels_county_check.sql`.
- [ ] Do not edit Go files. No Go file reads `parcel_sales.id`.

**Build.**

- [ ] List `db/migrations/*.sql`. If `000004` is already taken, use the next free six-digit prefix and keep the suffix `parcel_sales_uuid_id`. On `main` at `d4654f6` the highest prefix is `000003`.
- [ ] In the goose Up section, drop the identity, change `parcel_sales.id` to `uuid` with `USING gen_random_uuid()`, and set the default to `gen_random_uuid()`. Leave `parcel_id`, the foreign key, and `parcel_sales_parcel_sequence_unique` in place. Do not add `pgcrypto`. `000001` already creates it.
- [ ] In the goose Down section, drop the primary key, drop `id`, add `id BIGINT GENERATED BY DEFAULT AS IDENTITY`, and add the primary key again. The down keeps the sale rows and assigns new bigint ids. It does not keep the UUID values.
- [ ] Use the statements below. They are the ones the scratch database accepted.

```sql
-- +goose Up
ALTER TABLE parcel_sales ALTER COLUMN id DROP IDENTITY;
ALTER TABLE parcel_sales ALTER COLUMN id TYPE uuid USING gen_random_uuid();
ALTER TABLE parcel_sales ALTER COLUMN id SET DEFAULT gen_random_uuid();

-- +goose Down
ALTER TABLE parcel_sales DROP CONSTRAINT parcel_sales_pkey;
ALTER TABLE parcel_sales DROP COLUMN id;
ALTER TABLE parcel_sales ADD COLUMN id BIGINT GENERATED BY DEFAULT AS IDENTITY;
ALTER TABLE parcel_sales ADD PRIMARY KEY (id);
```

**You see.**

- [ ] After `make migrate-up`, `information_schema.columns` shows `parcel_sales.id` with `data_type` `uuid` and `column_default` `gen_random_uuid()`.
- [ ] `pg_constraint` still lists `parcel_sales_pkey`, `parcel_sales_parcel_id_fkey`, and `parcel_sales_parcel_sequence_unique`.

**Verify, unit.** Tests alone are not sufficient verification. A PR is verified only when its unit, live, and perf boxes are all checked.

- [ ] No new Go test. `parcel_sales.id` has no Go reader. Run `go test -race ./... -count=1`. Pass when the exit code is 0.

**Verify, live.** Tests alone are not sufficient verification. A PR is verified only when its unit, live, and perf boxes are all checked. Ten lanes on `grok-4.7-xhigh-fast` at the PR head, per the boot recipe.

- [ ] Lane 1. Regression lane against trunk. Run `go test -race ./... -count=1` at trunk and at head. Trunk keeps `parcel_sales.id` as `bigint`. Record that, and gate a head migrate whose `parcel_sales.id` is `uuid` plus the same test run. Save `/tmp/swarm-uuid-1/worker-1/regression.png`. Pass when both test runs exit 0 and the head column `data_type` is `uuid`.
- [ ] Lane 2. Migrate a database that already has versions 1, 2, and 3. Save `/tmp/swarm-uuid-1/worker-2/migrate-up.png`. Pass when `goose version` prints the numeric prefix of the new file and `\d parcel_sales` shows `id uuid` and `parcel_id uuid`.
- [ ] Lane 3. Insert one parcel and one sale. Omit `parcel_sales.id` so the default fills it. Save `/tmp/swarm-uuid-1/worker-3/insert-uuid.png`. Pass when the returned id is a UUID and `sequence` is 1.
- [ ] Lane 4. Insert one parcel and one sale, then upsert that sale on `(parcel_id, sequence)` with a new `amount`. Do not set `id`. Save `/tmp/swarm-uuid-1/worker-4/upsert-keeps-id.png`. Pass when the id is unchanged and `amount` is the new value.
- [ ] Lane 5. Insert one sale, record its id, delete that row, and insert the same parcel and sequence again. Save `/tmp/swarm-uuid-1/worker-5/delete-changes-id.png`. Pass when the new id is a different UUID. This is the write uuid-2 must not use.
- [ ] Lane 6. Insert a second sale with the same `parcel_id` and `sequence`. Save `/tmp/swarm-uuid-1/worker-6/unique.png`. Pass when the insert errors and exactly one row remains.
- [ ] Lane 7. Insert a sale whose `parcel_id` is a random UUID. Save `/tmp/swarm-uuid-1/worker-7/fk.png`. Pass when the insert errors and the sale table gains no row.
- [ ] Lane 8. Insert one parcel and one sale, then delete that parcel. Save `/tmp/swarm-uuid-1/worker-8/cascade.png`. Pass when the sale row is gone and the parcels row is gone.
- [ ] Lane 9. Run goose down for this migration, insert one sale, then run goose up. Save `/tmp/swarm-uuid-1/worker-9/down-up.png`. Pass when down shows `data_type` `bigint`, the insert succeeds, up shows `data_type` `uuid`, and the sale row still exists.
- [ ] Lane 10. Insert `county_id` `wake` into `parcels`, and list columns of `parcel_sales`. Save `/tmp/swarm-uuid-1/worker-10/county.png`. Pass when the parcel insert fails the county check and `parcel_sales` has no `county_id` column.

**Verify, perf.** Tests alone are not sufficient verification. A PR is verified only when its unit, live, and perf boxes are all checked.

- [ ] Metric. Wall time of a second upsert of 1000 sales, and the count of distinct sale ids, at trunk and at head. Both sides run the same statement and omit `id`.
- [ ] Probe. Time three second-upsert runs at trunk, then three at head, then three at trunk. Record each time and the distinct id count.
- [ ] Baseline. Record the trunk mean of the three runs first.
- [ ] Rule. Fail when the head mean is greater than twice the trunk mean, or when the distinct sale id count is not 1000. A doubled mean is the failing number. Do not use a ratio from two different tables.

**Review gate.** None. uuid-1 is not review-gated.

**Merge.**

- [ ] Root's clean verdict at the exact head SHA.
- [ ] Bugbot triage done.
- [ ] Rebased onto current trunk after the verdict, with `git patch-id` unchanged.
- [ ] The owner does not merge. The operator lands uuid-1 before uuid-2.

## Keep parcel and sale ids stable on a rerun (uuid-2)

**Depends on.** uuid-1.

**Files.**

- [ ] Create `internal/parcelstore/parcels.go`.
- [ ] Create `internal/parcelstore/parcels_test.go`.
- [ ] Create `internal/parcelstore/sales.go`.
- [ ] Create `internal/parcelstore/sales_test.go`.
- [ ] Edit `internal/models/gis_parcel.go`.
- [ ] Edit `go.mod` and `go.sum` so the Postgres driver is a direct requirement.
- [ ] Do not edit `main.go`, `internal/service/parcel_id_importer.go`, or `internal/connector/union/`.

**Build.**

- [ ] Add `ParcelID` and `SaleID` as distinct string types in `internal/models`. Put `ID ParcelID` on `GISParcel`. Put `ID SaleID` and `ParcelID ParcelID` on `ParcelSale`. Leave `Sequence` as `int`. Leave `CountyID` as `string`.
- [ ] Add `SaveParcel`. Insert the parcel and omit `id`. On conflict on `(county_id, source_name, source_parcel_id)`, update the mapped columns and do not set `id`. Return the stored `parcels.id`. Do not delete a parcel.
- [ ] Add `SaveSales`. Look up `parcels.id` with that same natural key. Upsert each sale on `(parcel_id, sequence)`, omit `id`, update the sale columns, and do not set `id`. Return the stored sale ids. Do not delete a sale that the caller left out.
- [ ] Return an error when the parcel natural key is missing, and when `SaveSales` cannot find that parcel. Do not insert a sale with a caller-supplied parcel UUID that was not looked up.
- [ ] Put `//go:build postgres` on the two test files. `TestSaveParcelKeepsID` saves one parcel twice and requires the same id and one row. `TestSaveSaleKeepsID` saves one sale twice with a new amount and requires the same sale id, the new amount, and one row. A third case saves two sales, then saves only the first, and requires the second row to remain.

**You see.**

- [ ] `go test -race -tags postgres ./internal/parcelstore/ -count=1` prints the same parcel id on both saves and the same sale id on both saves.
- [ ] A sale that was saved once and then omitted from the next `SaveSales` call is still in `parcel_sales`.

**Verify, unit.** Tests alone are not sufficient verification. A PR is verified only when its unit, live, and perf boxes are all checked.

- [ ] `parcels_test.go` gains `TestSaveParcelKeepsID`. `sales_test.go` gains `TestSaveSaleKeepsID` and the omitted-sale case. Run `go test -race ./... -count=1`, then `go test -race -tags postgres ./internal/parcelstore/ -count=1` with `DATABASE_URL` set. Pass when the first command exits 0 and the tagged command exits 0.

**Verify, live.** Tests alone are not sufficient verification. A PR is verified only when its unit, live, and perf boxes are all checked. Ten lanes on `grok-4.7-xhigh-fast` at the PR head, per the boot recipe.

- [ ] Lane 1. Regression lane against trunk. Run `go test -race ./... -count=1` on the uuid-1 head and on this head. The parent has no `internal/parcelstore` package. Record that, and gate the tagged store tests plus the existing suite. Save `/tmp/swarm-uuid-2/worker-1/regression.png`. Pass when the parent suite exits 0 and the head suite exits 0.
- [ ] Lane 2. Call `SaveParcel` twice for one natural key, with a new `mapped_acres` the second time. Save `/tmp/swarm-uuid-2/worker-2/upsert-parcel.png`. Pass when one `parcels` row exists, the id matches, and `mapped_acres` matches the second save.
- [ ] Lane 3. Call `SaveSales` twice for sequence 1 with a new amount. Save `/tmp/swarm-uuid-2/worker-3/upsert-sale.png`. Pass when one sale row exists, the sale id matches, and `amount` matches the second save.
- [ ] Lane 4. Call `SaveSales` with an empty `ParcelSale.ID`. Save `/tmp/swarm-uuid-2/worker-4/omit-id.png`. Pass when the stored id is a UUID generated by the database and the returned id equals that stored id.
- [ ] Lane 5. Save two parcels with different `source_parcel_id` values. Save `/tmp/swarm-uuid-2/worker-5/two-parcels.png`. Pass when the two `parcels.id` values differ.
- [ ] Lane 6. Save sequence 1 and sequence 2 for one parcel. Save `/tmp/swarm-uuid-2/worker-6/two-slots.png`. Pass when the two sale ids differ and both `parcel_id` values equal that parcel.
- [ ] Lane 7. Save two sales, then call `SaveSales` with only the first. Save `/tmp/swarm-uuid-2/worker-7/no-delete.png`. Pass when the omitted sale row and its id are still present.
- [ ] Lane 8. Call `SaveParcel` with `county_id` `wake`. Save `/tmp/swarm-uuid-2/worker-8/bad-county.png`. Pass when the call returns an error and `parcels` has no `wake` row.
- [ ] Lane 9. Call `SaveSales` for a natural key that was never saved. Save `/tmp/swarm-uuid-2/worker-9/missing-parcel.png`. Pass when the call returns an error and `parcel_sales` gains no row.
- [ ] Lane 10. Run `go test -race -tags postgres ./internal/parcelstore/ -count=1` twice. Save `/tmp/swarm-uuid-2/worker-10/twice.png`. Pass when both runs exit 0 and each run reports the same id across its two saves.

**Verify, perf.** Tests alone are not sufficient verification. A PR is verified only when its unit, live, and perf boxes are all checked.

- [ ] Metric. Wall time of the second `SaveSales` call for one sale, and whether that sale id changed. The parent has no `SaveSales`. The end state is one sale row whose id equals the first save.
- [ ] Probe. Time the second call at the head after the database connection is already open. Record that the parent has no function to time.
- [ ] Baseline. Record the parent as no `SaveSales` and no elapsed time.
- [ ] Rule. Fail when the second call takes 1 second or longer, or when the sale id changes. Do not report a ratio against the parent. The parent has no writer, so a ratio would compare unlike work.

**Review gate.** None. uuid-2 is not review-gated.

**Merge.**

- [ ] Root's clean verdict at the exact head SHA.
- [ ] Bugbot triage done.
- [ ] Rebased onto the uuid-1 branch after the verdict, with `git patch-id` unchanged. After the operator lands uuid-1, rebase onto trunk only at merge prep.
- [ ] The owner does not merge. The operator lands uuid-2 after uuid-1.

## Close the program

- [ ] Every box above is checked with its evidence.
- [ ] Reply to the operator with each PR, its verdict, and the note that nothing was merged.

## Appendix A. Prototype evidence

The schema under test is `main` at `d4654f6d580a64201f0468888f91a7dcb5925b2f`. The scratch work was not committed. There is no prototype branch. The databases `uuid_proto` and `goose_proto` are throwaway.

**Measured.** PostgreSQL 16.15 accepted the current Up sections of `000001`, `000002`, and `000003`. A parcel insert on conflict of `(county_id, source_name, source_parcel_id)` returned `4e178ca4-5b2c-4bdf-b216-4041ef8f4020` on the first save and the same id on the second save. `mapped_acres` changed from 1 to 2.5. One row remained.

**Measured.** A sale upsert on `(parcel_id, sequence)` that omits `id` kept bigint id `1` while `amount` changed from 10 to 99.

**Measured.** `ALTER COLUMN id DROP IDENTITY`, then `TYPE uuid USING gen_random_uuid()`, then `SET DEFAULT gen_random_uuid()` left `data_type` `uuid` and `column_default` `gen_random_uuid()`. The constraints that remained were `parcel_sales_pkey`, `parcel_sales_parcel_id_fkey`, `parcel_sales_parcel_sequence_unique`, `parcel_sales_sequence_check`, and `parcel_sales_amount_nonnegative_check`. The following upsert kept sale id `f8c51679-b0a0-465c-8a0e-6339dc9db24e` while `amount` became 100. The parcel id stayed `4e178ca4-5b2c-4bdf-b216-4041ef8f4020`.

**Measured.** Deleting that sale and inserting it again assigned `663619c2-39eb-4196-9c38-4dde959cc7fb`. The parcel id did not change. A writer that deletes and inserts will mint a new sale UUID on every rerun.

**Measured.** The down statements on a table of 1001 sales restored `data_type` `bigint`, kept 1001 rows, and numbered `id` from 1 to 1001. The UUID values were gone. The primary key, the parcel foreign key, and the sequence unique constraint remained.

**Measured.** goose v3.24.3 applied versions 1, 2, and 3, then reported version 3. Appending a comment to a copy of `000001_init_schema.sql` and running `goose up` again printed `no migrations to run. current version: 3`. `goose_db_version` stayed at versions 0, 1, 2, and 3. Editing `000001` does not change a database that has already applied it.

**Measured.** Three matched upserts of 1000 rows, same columns and the same unique key, took 10.796 ms, 10.124 ms, and 10.544 ms with a bigint identity. The UUID table took 11.423 ms, 9.264 ms, and 10.803 ms. Both sides kept 1000 distinct ids. The UUID spread is about 2.2 ms. A head mean greater than twice the trunk mean sits outside that spread. The rows were inserted on the same local server immediately before the timed runs, so the number is a warm local upsert, not a Docker or production rate. No profile was taken, so this appendix does not name a CPU or disk limiter.

**Measured.** An earlier pair was not matched. The real `parcel_sales` table, already converted to UUID and carrying its indexes, took 13.284 ms, 10.097 ms, and 13.605 ms. A bare bigint table without those indexes took 7.415 ms, 8.225 ms, and 7.844 ms. Do not use that pair as a ratio. The uuid-1 rule says both sides run the same statement on `parcel_sales`.

**Read.** No Go file calls `database/sql`, `pgx`, `rows.Scan`, or `INSERT INTO`. `go.mod` lists `github.com/jackc/pgx/v5` only as an indirect requirement. `GISParcel` has no id field. `ParcelSale` has no id field. `internal/models/gis_parcel.go` lines 92 to 103.

**Read.** The only current definition of `parcel_sales.id` as an integer is `db/migrations/000001_init_schema.sql` line 95, `id BIGINT GENERATED BY DEFAULT AS IDENTITY PRIMARY KEY`. Line 96 is `parcel_id UUID NOT NULL REFERENCES parcels(id) ON DELETE CASCADE`. Line 110 is `UNIQUE (parcel_id, sequence)`.

Go sites that must change because `parcel_sales.id` becomes a UUID. The count is 0.

Go and SQL sites that mention an integer and must be left alone. They are not `parcel_sales.id`.

- `internal/models/gis_parcel.go` line 93. `Sequence int` is the sale slot. The column is `SMALLINT` at migration line 97.
- `internal/models/gis_parcel.go` line 95. `OGSaleDate *int64` is epoch milliseconds. Migration line 99.
- `internal/connector/union/translator_test.go` line 241 compares `OGSaleDate` with `int64(1589500800000)`.
- `internal/models/gis_parcel.go` line 37. `ParcelYear *int64` is a year, not a table id.
- `internal/models/gis_parcel.go` line 77. `YearBuilt *int64` is a year, not a table id.
- `internal/connector/union/extras.go` line 12. `OBJECTID *int64` is an ArcGIS feature id stored in JSON.
- `internal/connector/union/extras.go` line 13. `OBJECTID_1 *int64` is the same kind of feature id.
- `internal/connector/union/extras.go` line 28. `IDCOL *int64` is a county field stored in JSON.
- `internal/connector/union/extras.go` line 30. `OG_DATE_CRT *int64` is an epoch value.
- `internal/connector/union/gis.go` line 161. `queryTotalParcels` returns an `int64` count of features, not a row id.
- `internal/connector/datasource.go` line 12. `ParcelID string` is the county parcel number.
- `internal/service/parcel_id_importer.go` line 12. The hook takes `[]string` parcel ids.
- `internal/county/county.go` line 15. `type ID string`.

`docs/ARCHITECTURE.md` lines 522, 554, and 589 show an example parcel `"id": 123` and a `parcel_id` that looks like a county number. That sample is not a scanned column. Do not treat it as a Go call site. Do not edit it in this program.

Unproven on this run. The Docker image in `docker-compose.yml` line 3 is `postgis/postgis:18-3.6-alpine`. Docker was not installed here, so those statements were not run on that image. uuid-1 lanes 2 and 4 are the re-run. No Go `SaveParcel` exists yet, so the uuid-2 tests were not executed.

## Appendix B. Alternatives rejected

Editing `000001_init_schema.sql` loses. `plans/use-uuid-parcel-ids.md` line 6 said to edit that file because the initial migration was still being audited. Versions `000002` and `000003` are now on `main`. goose v3.24.3 does not reapply an edited file once version 3 is recorded. A database that has migrated would keep the bigint sale id forever. A new file changes both a fresh database and an already migrated one.

A second integer column next to a new UUID loses. Nothing reads `parcel_sales.id` from Go. There is no caller to migrate in place. The end state is one UUID primary key.

Letting uuid-2 delete sales that the caller omitted loses. A delete followed by an insert minted a new UUID in the scratch database. Leaving the extra row keeps the id. Removing a stale slot belongs with the later parcel diff, not with this writer.

Putting the sale writer in uuid-1 loses. The column change is visible in `psql` with no Go. The writer is a second behavior, and it needs the column to exist first.

Keying a soil polygon by `(county_id, source_name, object_id)` loses for this program's rule. That shape is already written in `plans/soil-layer.md`. This program does not rewrite that file. The rule above is the one a later soil change has to meet.

## Appendix C. Risks

uuid-1 can collide with the soil migration number. `plans/soil-layer.md` says soil-3 may be `000004` once `000003_parcels_county_check.sql` exists. The uuid-1 owner lists the directory at the start and takes the next free prefix. The owner does not edit the soil plan.

uuid-2 creates `internal/parcelstore/parcels.go`. `plans/soil-layer.md` persist-1 also creates that file and calls it `parcelstore.Save`. If persist-1 lands first, uuid-2 edits the existing file so the upsert still omits `id` and still does not delete. If uuid-2 lands first, persist-1 has to keep those two clauses. Neither PR edits `main.go`. The call from `main` stays with persist-1.

`Sequence` is the county slot `s1`, `s2`, or `s3`, assigned in `internal/connector/union/translator.go` lines 376 to 382. The UUID stays on the slot. If the county moves an old sale from slot 1 to slot 2, the slot 1 id remains and the facts on that row change. See the open question in Appendix E.

A sale omitted from a later save stays forever until the parcel diff exists. uuid-2 lane 7 locks that in on purpose.

`control-cli` is not installed. A search of the plugin tree found no `control-cli` skill. The live lanes use `psql` and `go test` instead. This repo also has no `pstack/` tree. A glob of `pstack/**` at the program write returned no files. Owners read the installed poteto-mode playbooks. They do not add `pstack/` here.

The `deslop` skill from `cursor-team-kit` is not installed. `/no-comments` is installed with poteto-mode. The checklist still names both. A missing skill is recorded, not invented.

The 1 second budget in uuid-2 was not timed with a Go call. It is a stuck-call ceiling. The timed evidence is the SQL batch in Appendix A. The owner still records the head time.

PostGIS 18 was not booted. If `DROP IDENTITY` or `TYPE uuid USING gen_random_uuid()` fails on that image, uuid-1 stops and the lane log is the bug. Do not edit `000001` to get past it.

## Appendix D. Links and reading list

Read these before editing.

- `db/migrations/000001_init_schema.sql` lines 1 to 6, 62 to 73, and 94 to 113. Parcel UUID, natural key, geometry, sale id, sale foreign key, sequence unique constraint.
- `db/migrations/000002_parcels_storage_srid.sql` lines 1 to 7. Comments only. It does not change ids.
- `db/migrations/000003_parcels_county_check.sql` lines 1 to 8. The county check. The next prefix after this file is `000004` unless a later file has landed.
- `internal/models/gis_parcel.go` lines 13 to 28 and 92 to 103. No parcel id and no sale id.
- `internal/county/county.go` lines 13 to 18. `county.ID` is a string. `Union` is `"union"`.
- `internal/connector/union/translator.go` lines 376 to 397. Sale slots.
- `Makefile` lines 12 to 18 and 70 to 71. `make migrate-up` runs goose in Docker.
- `docker-compose.yml` lines 2 to 11. The local database image.
- `plans/use-uuid-parcel-ids.md` lines 6 to 11. It kept the sale id numeric. This plan supersedes that sentence and does not edit the file.
- `playbooks/autopilot-stack.md` in the installed poteto-mode skill. Owners stop before merge.
- `playbooks/shipping.md` in that same skill. The `git patch-id` rule.
- `pstack/skills/swarm/SKILL.md` is not on `main`. The installed swarm skill is the one the lanes use. Worker model is `grok-4.7-xhigh-fast`.
- uuid-1 and uuid-2 do not need `pstack/skills/how/SKILL.md` or `pstack/skills/interrogate/SKILL.md`. The column change and the upsert were measured. A later soil redesign is the piece that would need that review, and it is outside this program.
- The decision trail for the execution run is the one `playbooks/autopilot-stack.md` asks the root to keep. This plan file is the record for the planning run.

### Where the other plans disagree

`plans/soil-layer.md` lines 323 to 335. `soil_polygons` uses `PRIMARY KEY (county_id, source_name, object_id)`. The polygon has no UUID. The county feature id is the key. That conflicts with a polygon UUID plus MUKEY plus survey version.

`plans/soil-layer.md` lines 344 to 361. `parcel_soils.id` is `BIGINT GENERATED BY DEFAULT AS IDENTITY`. The row is one parcel plus one MUKEY, and it also stores `source_parcel_id`. That conflicts with a join of soil polygon id, parcel id, and overlap area.

`plans/soil-layer.md` lines 564 to 565. Several polygons that share a MUKEY collapse into one overlap row. The map cannot paint each polygon from that row.

`plans/soil-layer.md` lines 421 to 424. A second run replaces polygon rows and `parcel_soils` rows, which deletes them. New ids would be minted on every rerun.

`plans/soil-layer.md` lines 305 and 573. `parcel_soils.parcel_id` does reference `parcels.id`. That part matches the parcel link. The table around it does not match the polygon rule.

`plans/soil-layer.md` lines 125 to 138. persist-1 will create `internal/parcelstore/parcels.go` and upsert parcels on the natural key. That part matches uuid-2. It does not save sales, and it does not say that `id` is omitted.

`plans/scraper-enrichment.md` lines 61 to 68. The scrape table is unique on `(county_id, source_name, source_parcel_id)` and has no `parcels.id` column. Per-parcel documents then do not reference the application id. No plan file is named tax. The tax name and tax owner columns already live on `parcels`, at `000001_init_schema.sql` lines 18 to 21, so those values sit on the parcel row. `plans/db-schema-type-audit.md` lines 4 and 22 keep `tax_owner_id` as `TEXT` on that row.

This program does not edit those plan files.

## Appendix E. Decisions recorded

The user said "go ahead and do the plan for the parcel_sales for their UUID. It should have the parcel_id so it can be linked and then have its own uuid as well right? and then soil should be separate from the parcel since its just the geometry."

The user said "I think we want to use our own ids for the pieces internally the parcel id is great but to fetch the combination of all the info we have gathered should be our application id."

`parcel_sales.parcel_id` stays the link to `parcels.id`. `parcel_sales.id` becomes the sale's own UUID. The county parcel number stays `source_parcel_id` and is only the ingest lookup. Soil polygons are a separate row with their own UUID.

The migration is a new goose file. The default name is `db/migrations/000004_parcel_sales_uuid_id.sql` because `000002` and `000003` exist and an edited `000001` does not reapply. That choice is not left open.

uuid-2 does not edit `main.go`. persist-1 already owns the call from `main`. If that split is wrong, say to wire `SaveParcel` and `SaveSales` from `main` in uuid-2.

### Open question

1. A sale id stays on its slot. Slot 1 is the `s1` sale, slot 2 is `s2`, and slot 3 is `s3`. If the county puts a new sale in slot 1 and shifts the old sale to slot 2, the old id stays on slot 1 and then describes the new sale. The plan keeps that, because the unique key is the parcel plus the slot. If the id should follow the deed instead, say so. The conflict key would have to change, and uuid-2 would not start from `(parcel_id, sequence)`.
