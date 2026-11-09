# County id package plan

Give the county id its own leaf package, `internal/county`, before soil or scrape defines one. Callers import `county.ID`. The database keeps `TEXT` and a check that allows `union`. county-1 is the only PR. The operator lands it.

This file is the plan. It changes no Go code, no migration, and no config.

## How to read this

One box is one unit of work. Every box names the evidence that checks it. A nested box is a sub-step of the box above it. Check a box only when its evidence exists, a file, a log line, a screenshot, a test run, or a SHA. The body is a how-to. The appendices explain and record.

The program runs `pstack/skills/poteto-mode/playbooks/autopilot-stack.md`. The operator lands county-1. This plan stops at merge-ready. Nobody merges from the checklist.

Tests alone are not sufficient verification. A PR is verified only when its unit, live, and perf boxes are all checked.

## Program checklist

### Arm the program

- [ ] State the protocol and this plan to the operator, then stop. Start execution only on the operator's explicit go.
- [ ] Read these from trunk at program start. Re-read them at every tick.
  - [ ] `git show origin/main:pstack/skills/poteto-mode/playbooks/autopilot-stack.md`
  - [ ] `git show origin/main:pstack/skills/swarm/SKILL.md`
  - [ ] `git show origin/main:pstack/skills/poteto-mode/playbooks/opening-a-pr.md`
  - [ ] `git show origin/main:pstack/skills/control-cli/SKILL.md`
  - [ ] `git show origin/main:pstack/skills/principle-type-system-discipline/SKILL.md`
  - [ ] `git show origin/main:pstack/skills/principle-boundary-discipline/SKILL.md`
  - [ ] `git show origin/main:pstack/skills/principle-model-the-domain/SKILL.md`
  - [ ] `git show origin/main:pstack/skills/principle-subtract-before-you-add/SKILL.md`
  - [ ] `git show origin/main:pstack/skills/principle-test-behavior-not-implementation/SKILL.md`
  - [ ] `git show origin/main:pstack/skills/principle-prove-it-works/SKILL.md`
  - [ ] `git show origin/main:pstack/skills/principle-explain-the-number/SKILL.md`
  - [ ] `git show origin/main:plans/county-id-package.md`
  - [ ] `git show origin/main:plans/soil-layer.md`
  - [ ] `git show origin/main:db/migrations/000001_init_schema.sql`
  - [ ] `git show origin/main:db/migrations/000002_parcels_storage_srid.sql`
- [ ] On the operator's go, arm the audit tick as `/loop 1h` with the tick prompt below. Never leave the cadence to memory.
- [ ] Use this tick prompt, verbatim. "Re-read the execution playbook from trunk. Audit the operation against it and fix drift in this tick. Probe every active lane and judge progress by side effects only. Stand down a stuck lane and dispatch its replacement now. Then post a short status message to the operator in chat only when the audit found a tracked change that no earlier status message reported, such as a PR opened, a code-ready head, a round launched or closed, a verdict, a merge, a stuck agent and the action taken, a blocker added or cleared, or a decision only the operator can make. Name every such change and nothing else. Do not repeat a table, the merged list, or an unchanged blocker. If the audit found none, end the turn with no reply text. Either way, log this tick's row in your decision trail. The row names the items reported, or none."
- [ ] On the operator's hold or stand-down, send every owner a zero-writes order at once.
- [ ] When a `git show origin/main:pstack/...` command fails, read the installed poteto-mode skill and record that fallback. Appendix C names the missing `pstack/` tree and the missing `control-cli` skill. Do not invent a UI driver.

### Spawn owners

- [ ] Spawn one owner for county-1 with the full lifecycle the execution playbook names.
- [ ] Follow this dependency graph. county-1 has no parent.
  - [ ] county-1 is the only PR. It branches from `main`.
- [ ] Hold the file boundaries. county-1 touches only `internal/county/` and one new file under `db/migrations/`. It does not edit `internal/models/`, `internal/connector/`, `internal/soil/`, `internal/scrape/`, `plans/soil-layer.md`, `plans/scraper-enrichment.md`, `go.mod`, `Makefile`, or config.
- [ ] Hold the review gate. county-1 changes no interaction. It does not wait for screenshots in chat before the operator decides to land it.

### PR mechanics, for every PR

- [ ] Resolve the forge once. Default to `gh`. If `command -v origin` succeeds and Origin can resolve the repository, use `origin pr` for every PR operation. Record any fallback to `gh`. Never require `gt`.
- [ ] Open the PR ready, never draft, per **Opening a PR**. Use the run's built-in PR tool when it has one, else `origin pr create --status open --base <base-branch>` or `gh pr create --base <base-branch>` according to the resolved forge. A stack child targets its parent branch.
- [ ] Run the repo's lint and typecheck once before the PR-facing push. For this repo that command is `make test`. Also run the `go vet` and `staticcheck` commands in the unit box. Push with hooks on.
- [ ] Run `/deslop` before each commit and `/no-comments` before review.
- [ ] Triage every Bugbot and security-reviewer comment per `pstack/skills/poteto-mode/references/bugbot-triage.md`.
- [ ] Rebase onto current trunk before the code-ready report and babysit. Keep that merge base in fix rounds. Rebase again only at merge prep, on a `git merge-tree` conflict with trunk, on a CI failure that comes from a change on trunk, or when a known concurrent change to the same files lands.

### Verdict and merge, for every PR

- [ ] A full swarm runs once, at the code-ready head. Another full swarm runs only for new behavior, a redesign, or a large new diff. A rebase, a merge-conflict resolution, or a minor fix gets a single verifier at the new head, and a wording-only change gets none. Run each full swarm per `pstack/skills/swarm/SKILL.md`. One gates lane. The ten live lanes from the PR's **Verify, live** block. The perf lane from its **Verify, perf** block. Two or more audit lanes, each with its own focus, that read the diff and the receipts and distrust the PR body. The root audits the receipts in the merge-ready report before the verdict.
- [ ] Judge every finding by the execution playbook's blocking rule. The verdict is clean when none blocks. A failure on trunk behavior in a lane marked `Regression lane against trunk.` is recorded against trunk and does not block the PR. A new head gets a fresh verdict, except for results that stay valid under the patch-id rule in `playbooks/shipping.md`.
- [ ] Stop when the PR is verified and green at its current head, then hand it to the operator. Do not merge. A restack that leaves `git patch-id` unchanged keeps the verdict. After the handoff, nobody pushes without the operator's go.

### Boot recipe, for every live lane

Each live lane runs on its own cloud VM at the PR head. This repo has no browser UI for the county id. Drive `go test`, `psql`, and text renders of their output. There is no `control-cli` skill in this repo. Appendix C records that gap. The commands below are the drive.

- [ ] `git fetch origin <head-branch> && git checkout <head SHA>`.
- [ ] For lanes 1, 7, 8, and 9, copy `.env.example` to `.env` when `.env` is absent, run `docker compose up -d`, wait until `pg_isready` succeeds, then `make db-create` and `make migrate-up`. Wait until `psql` accepts `SELECT 1`. Lanes that only run `go test` do not start Postgres.
- [ ] Deliver input only through those commands. Read-only diagnostics are the test output and the `psql` result rows.
- [ ] Save every screenshot to `/tmp/swarm-<pr-id>/worker-<n>/<slug>.png` and return the paths with the report. A lane with no display saves a text render of the command output at that path.

## Add the county id package (county-1)

**Depends on.** None.

**Files.**

- [ ] Create `internal/county/county.go`.
- [ ] Create `internal/county/county_test.go` with `package county`, the same package as `county.go`. Existing tests use that form. `internal/connector/union/vacant_filter_test.go` line 1 is `package union`.
- [ ] Create one goose migration, `db/migrations/<prefix>_parcels_county_check.sql`. At implementation time, list `db/migrations/*.sql`, take the highest six-digit prefix, add 1, and zero-pad to six digits. On main at `1bdf7b8cd51f8031f5ff108be623f6a354ef70b1` the highest prefix is `000002`, so the file is `db/migrations/000003_parcels_county_check.sql` unless a higher prefix has already landed. Do not call `make migrate-create` for the name. `Makefile` lines 82 to 88 run `goose create` with no sequential flag, and the name it emits is a timestamp.
- [ ] Do not edit `db/migrations/000001_init_schema.sql` or `db/migrations/000002_parcels_storage_srid.sql`.

**Build.**

- [ ] Add package `county` with the declarations below. The directory is `internal/county`. The package name is the base name of that directory. Exported names do not repeat `county`.

```go
package county

type ID string

const Union ID = "union"

var ErrUnknown error

func Parse(raw string) (ID, error)
func (id ID) Valid() bool
func (id ID) String() string
func All() []ID
```

- [ ] `ErrUnknown` is `errors.New("unknown county id")`. `Parse` accepts only a string equal to one value of `All`, compared byte for byte. `Parse("union")` returns `Union` and a nil error. Every other string returns the zero `ID` and `fmt.Errorf("%w: %q", ErrUnknown, raw)`. `errors.Is` on that error finds `ErrUnknown`. `UNION`, `Union`, the empty string, a leading space, and a trailing space are unknown.
- [ ] `Valid` is true only when the receiver equals one value of `All`. `Union.Valid()` is true. `ID("union").Valid()` is true, because a conversion can build that value without `Parse`. `ID("wake").Valid()` is false. The zero `ID` is false. Call `Parse` on raw text at a boundary. Call `Valid` only when the `ID` did not come from `Parse`. Do not call `Valid` again on a value `Parse` returned.
- [ ] `String` returns the underlying text, including text that fails `Valid`. `Union.String()` is `union`. `fmt.Sprint(Union)` is `union`.
- [ ] `All` returns a new slice on every call, containing `Union` only, in that order. A caller that appends to the slice must not change the next call.
- [ ] The non-test imports are `errors` and `fmt` only. Do not import `internal/soil`, `internal/models`, `internal/scrape`, `database/sql`, or `database/sql/driver`. Do not add `sql.Scanner` or `driver.Valuer`.
- [ ] Do not add a second county constant in this PR. Do not change `models.ParcelIdentity.CountyID`. That field stays `string` at `internal/models/gis_parcel.go` line 31. Do not edit `internal/connector/union/translator.go`.
- [ ] `Up` adds `CONSTRAINT parcels_county_id_check CHECK (county_id IN ('union'))` on `parcels`. `Down` drops `parcels_county_id_check`. The quoted list is the `String` values of `All`, in the same order.
- [ ] To add a county after county-1 has merged, do all of the following in a new PR. Do not do them in county-1.
  1. Add `const <Name> ID = "<value>"` next to `Union`. `<value>` is the lowercase text the check will store.
  2. Append that constant inside `All`, in the same order as the SQL list.
  3. Add a new goose migration. Do not edit the county-1 migration. Drop `parcels_county_id_check` and add it again with the full `IN` list. When `soil_polygons_county_check` or `parcel_soils_county_check` exists, drop and re-add those the same way. The quoted strings match `All`, in order.
  4. Extend `TestParse`, `TestValid`, and `TestAllMatchesMigrationCheck` with the new value.

**You see.**

- [ ] `go test -race ./internal/county/ -count=1` prints `PASS`. After `make migrate-up`, `psql` `\d parcels` shows `parcels_county_id_check`.

**Verify, unit.** Tests alone are not sufficient verification. A PR is verified only when its unit, live, and perf boxes are all checked.

- [ ] `county_test.go` gains table-driven `TestParse`, `TestValid`, and `TestString`. `TestParse` accepts `union` and rejects `wake`, `UNION`, `Union`, the empty string, ` union`, and `union `, using `errors.Is` against `ErrUnknown`. `TestValid` covers `Union`, `ID("union")`, `ID("wake")`, and the zero `ID`. `TestString` covers `Union.String()` and `fmt.Sprint(Union)`. `TestAllReturnsACopy` appends to the slice from `All` and requires the next `All` call to stay one element. `TestAllMatchesMigrationCheck` parses the single-quoted strings inside `parcels_county_id_check` in the new migration file and requires those strings, in order, to equal `All`. `TestImportsAreStandardLibrary` parses the non-test `.go` files in the package and requires every import path to contain no dot. Run `go test -race -count=1 -coverprofile=/tmp/county.out ./internal/county/`. Pass when the tests pass and `go tool cover -func=/tmp/county.out` prints `100.0%` on the total line.
- [ ] Run `go vet ./internal/county/` and require exit 0. Run `go run honnef.co/go/tools/cmd/staticcheck@latest ./internal/county/` and require exit 0. Do not add `staticcheck` to `go.mod`. Record the version the command prints. Run `make test` and require `PASS`.

**Verify, live.** Tests alone are not sufficient verification. A PR is verified only when its unit, live, and perf boxes are all checked. Ten lanes on `grok-4.7-xhigh-fast` at the PR head, per the boot recipe.

- [ ] Lane 1. Regression lane against trunk. Run `go test -race ./... -count=1` at trunk and at head. Trunk has no `internal/county` directory. Record that, and gate the new package tests plus a green `./...` run at head. On trunk, `psql` `\d parcels` shows `county_id` as `TEXT NOT NULL` and does not show `parcels_county_id_check`. Run `SELECT count(*) FROM parcels WHERE county_id IS DISTINCT FROM 'union'` at trunk before the head migration. Save `/tmp/swarm-county-1/worker-1/regression.png`. Pass when trunk's tests pass, the count is 0, head's tests pass, and head's `\d parcels` shows `parcels_county_id_check`.
- [ ] Lane 2. Run `TestParse`. Save `/tmp/swarm-county-1/worker-2/parse.png`. Pass when `union` returns `Union` and a nil error, and `wake`, `UNION`, `Union`, the empty string, ` union`, and `union ` each return the zero `ID` and an error that matches `ErrUnknown`.
- [ ] Lane 3. Run `TestValid`. Save `/tmp/swarm-county-1/worker-3/valid.png`. Pass when `Union.Valid()` and `ID("union").Valid()` are true, and `ID("wake").Valid()` and the zero `ID` are false.
- [ ] Lane 4. Run `TestString`. Save `/tmp/swarm-county-1/worker-4/string.png`. Pass when `Union.String()` is `union` and `fmt.Sprint(Union)` is `union`.
- [ ] Lane 5. Run `TestAllMatchesMigrationCheck` and `TestAllReturnsACopy`. Save `/tmp/swarm-county-1/worker-5/check-list.png`. Pass when the quoted strings in `parcels_county_id_check` equal `All` in order, and an append does not change the next `All` call.
- [ ] Lane 6. In `internal/county`, run `go list -f '{{join .Imports "\n"}}' .`. Save `/tmp/swarm-county-1/worker-6/imports.png`. Pass when the only imports are `errors` and `fmt`.
- [ ] Lane 7. Insert one row with `county_id` `union`, `county_code` `union`, `source_name` `union_county_gis`, and `source_parcel_id` `county-1-lane-7`. Save `/tmp/swarm-county-1/worker-7/insert-union.png`. Pass when the insert returns no error and `SELECT county_id FROM parcels WHERE source_parcel_id = 'county-1-lane-7'` returns `union`.
- [ ] Lane 8. Insert `county_id` `wake`, then insert `county_id` `UNION`, each with a new `source_parcel_id`. Save `/tmp/swarm-county-1/worker-8/reject-wake.png`. Pass when both inserts fail the check and neither `source_parcel_id` is stored.
- [ ] Lane 9. Run goose down for the county migration, insert `county_id` `wake`, delete that row, run goose up, and insert `wake` again. Save `/tmp/swarm-county-1/worker-9/down-up.png`. Pass when down drops `parcels_county_id_check`, the first `wake` insert succeeds, up restores the constraint, and the second `wake` insert fails.
- [ ] Lane 10. Run `go test -race -count=2 -coverprofile=/tmp/county.out ./internal/county/` and `go tool cover -func=/tmp/county.out`. Save `/tmp/swarm-county-1/worker-10/cover.png`. Pass when both runs pass and the total line says `100.0%`.

**Verify, perf.** Tests alone are not sufficient verification. A PR is verified only when its unit, live, and perf boxes are all checked.

- [ ] Metric. Wall time of 200,000 iterations of `Parse("union")` plus `Parse("wake")` at head, and the end state that the first call returns `Union` and the second matches `ErrUnknown`. Trunk has no `internal/county` package, so trunk's metric is the missing-package result and no elapsed time.
- [ ] Probe. `go test -bench=BenchmarkParse -benchtime=200000x -count=1 ./internal/county/` at trunk, then at head, then at trunk. `BenchmarkParse` calls that pair once per iteration. Trunk must produce the missing-package result. Head must produce an `ns/op` line.
- [ ] Baseline. Record the trunk missing-package result first. Do not invent a trunk duration.
- [ ] Rule. Fail if head's `ns/op` is greater than 5000. That is 1 second for 200,000 iterations. Fail if `Parse("union")` is not `Union` or if `Parse("wake")` does not match `ErrUnknown`. Do not report a ratio against trunk. The 1 second line fails the lane when `Parse` does I/O or sleeps. A pass is not a speed claim. Before treating the number as a regression, apply explain-the-number. Record three head repeats and their spread. The benchmark also times the test harness.

**Review gate.** None. county-1 is not review-gated.

**Merge.**

- [ ] Root's clean verdict at the exact head SHA.
- [ ] Bugbot triage done.
- [ ] Rebased onto current trunk after the verdict, with `git patch-id` unchanged.
- [ ] The owner does not merge. The operator lands county-1.

## Close the program

- [ ] Every box above is checked with its evidence.
- [ ] Reply to the operator with the report `playbooks/autopilot-stack.md` names. List county-1 and its verdict. Do not merge.

## Appendix A. Prototype evidence

No prototype branch, SHA, or screenshot. `playbooks/prototype.md` step 1 says no prototype when no run can decide the question. The open questions were the package name, `Parse` versus `Valid`, and whether `Scanner` belongs on the type. Those are settled by the Go naming docs and by reading this tree. A throwaway module would still be named `county.ID` under those rules. Compiling one was not done. That compile stays unproven, and it would not change the name.

**Measured.** `git rev-parse HEAD` on this checkout of `main` is `1bdf7b8cd51f8031f5ff108be623f6a354ef70b1`. `git status --short` printed nothing before these plan edits. `ls db/migrations` printed `000001_init_schema.sql` and `000002_parcels_storage_srid.sql` only.

**Measured.** A search for `type CountyID`, `type ID`, `func Parse`, `const Union`, `database/sql`, `sql.Scanner`, and `driver.Valuer` in `*.go` on that commit printed no matches. No Go type named `CountyID` exists on `main`.

**Read.** Package clauses on that commit are `main` (`main.go` line 1), `service` (`internal/service/parcel_id_importer.go` line 1), `connector` (`internal/connector/batch.go` line 1), `httpclient` (`internal/connector/shared/httpclient/http_client.go` line 1), `union` (`internal/connector/union/gis.go` line 1), and `models` (`internal/models/gis_parcel.go` line 1). There is no package `county`, `domain`, or `soil`.

**Read.** `ParcelIdentity.CountyID` is a `string` field, not a defined type, at `internal/models/gis_parcel.go` line 31. The Union translator assigns the raw string `"union"` at `internal/connector/union/translator.go` line 83. `internal/models/gis_parcel.go` lines 3 to 8 import `encoding/json`, `time`, and `github.com/twpayne/go-geom`.

**Read.** `parcels.county_id` is `TEXT NOT NULL` at `db/migrations/000001_init_schema.sql` line 8. The unique key is line 73. The `CHECK` constraints in that file are lines 74 to 91. None of them name `county_id`. `db/migrations/000002_parcels_storage_srid.sql` only comments on `parcels.shape` and `parcels.srid`.

**Read.** `Makefile` line 28 runs `go test -v -race ./...`. Lines 82 to 88 create a goose migration with no sequential prefix. No `staticcheck` target exists.

**Measured.** `gh` on PR 11 at commit `a38f56303546f456055d58b52e0f206b1885d84a` shows branch `cursor/scraper-enrichment-plan-1e02`. The plan file blob is `01ed6510810e23e6c642bd250d3c7fb10e9f057c`. A search of that blob found no `soil.CountyID`. It found `county.CountyID` at lines 206, 210, 410, 416, 421, 472, and 532.

**Read.** Effective Go, package names, says the package name is short, lower case, and one word, with no underscores or mixedCaps. The `bufio` package exports `Reader`, not `BufReader`, because clients write `bufio.Reader`. https://go.dev/doc/effective_go#package-names

**Read.** Go Code Review Comments, Package Names, says to omit the package name from the identifier. In package `chubby`, the type is `File`, so clients write `chubby.File`, not `chubby.ChubbyFile`. It also says to avoid meaningless names such as `util`, `common`, `misc`, `api`, `types`, and `interfaces`. https://go.dev/wiki/CodeReviewComments#package-names

## Appendix B. Alternatives rejected

`county.CountyID` loses. Clients would write `county.CountyID`, which repeats the package name. Effective Go uses `bufio.Reader` for that reason. Code Review Comments uses `chubby.File`. The same rule makes the constant `county.Union`, not `county.CountyUnion`. `county.ID` does not collide with a package or a defined type on `main`. The packages on `main` are `main`, `connector`, `httpclient`, `union`, `models`, and `service`. Package `union` is the adapter under `internal/connector/union`. A file can import `county` and `union` together. The qualifiers differ. The field name `CountyID` on `ParcelIdentity` stays a field in package `models`. It is not this type. `DataSource.County` at `internal/connector/datasource.go` line 33 and `GISSource.County` at `internal/connector/union/gis.go` line 80 are methods. A file can call `src.County()` and use `county.ID` together. Those names do not clash.

Keeping the type in `internal/soil` loses. `plans/soil-layer.md` soil-1 was going to declare `type CountyID string` in `internal/soil/county.go`. Scrape would then import `internal/soil` to name a county. Soil polygons use a county id. They do not own it. `parcels.county_id` already exists, and no soil table exists on `main`. That import would make scrape depend on soil for a value that is not a soil polygon. Per model-the-domain, the id gets its own small package. Per subtract-before-you-add, do not add the type to soil and move it later.

`internal/models` loses. That package is the parcel row, and it imports `go-geom` at `gis_parcel.go` line 7. A county id stored there is not a standard-library leaf. Soil and scrape would import the geometry package to name a county. The soil plan already rejects putting soil types in `internal/models` for that reason.

`internal/domain` loses. No such directory exists. Code Review Comments rejects package names that do not say what the package holds, such as `util` and `common`. A generic domain package collects unrelated types and stops being a leaf the first time two of them need each other.

`sql.Scanner` and `driver.Valuer` lose for county-1. No Go file on `main` imports `database/sql`. Nothing scans a county id. Per subtract-before-you-add, do not add `Scan` and `Value` before a caller scans. Per boundary discipline, the persistence package calls `Parse` on the text column when that caller exists. The leaf stays on `errors` and `fmt`. The database rule is still `TEXT` plus `CHECK (county_id IN ('union'))`, matching the column type of `parcels.county_id`.

`Valid` alone loses. External text is untyped until `Parse` returns an `ID` or an error. That is the boundary. `Valid` alone leaves every caller to remember the check, which is the scattered guard boundary discipline rejects. `Parse` alone is not enough either. `ID` has underlying type `string`, so any package can convert `ID("wake")`. `Valid` is the predicate for that conversion. Callers that hold a value returned by `Parse` do not call `Valid` again.

A second implementation PR that only changes `ParcelIdentity.CountyID` from `string` to `county.ID` loses for this program. There is no legacy `CountyID` type on `main` to delete. The string field is a different fact. Editing the translator in county-1 would touch `translator.go` while the soil and scraper plans are still open. Appendix C keeps that field as a follow-up.

## Appendix C. Risks

`control-cli` is not installed here, and this repo has no `pstack/` tree. `git show origin/main:pstack/...` fails until those files exist on trunk. The owner reads the installed poteto-mode skill and records that fallback. Live lanes drive `go test` and `psql` and save the command output as the screenshot path. A lane that cannot draw still returns that file. `deslop` was not installed in the planning session. The owner runs it when it is installed.

scrape-1 must depend on county-1 and must use `county.ID`. Do not edit `plans/scraper-enrichment.md` in the county program. Another agent owns it on PR 11, branch `cursor/scraper-enrichment-plan-1e02`, commit `a38f56303546f456055d58b52e0f206b1885d84a`.

**Measured.** That commit does not contain `soil.CountyID`. scrape-1 already depends on county-1 at line 188, and it already says package `scrape` imports `internal/county` and not `internal/soil`. The remaining edit is the repeated package name. Replace `county.CountyID` with `county.ID` at lines 206, 210, 410, 416, 421, 472, and 532. Line 406 says county-1 adds `type CountyID`. Change that sentence so it names `county.ID`. Line 540 keeps the import path `internal/county`. It does not hardcode `county.CountyID`. If a later commit of that plan still says `soil.CountyID`, or still imports `internal/soil` for the id, replace that with `county.ID` and `internal/county`. Do not declare a second county type. `Valid` stays in county-1, so the sentence in that plan about `Valid` still matches.

`ParcelIdentity.CountyID` stays a `string`. A raw string can still be stored on the parcel struct. The Union translator writes `"union"` today, which matches the check. The database rejects other text after county-1. The Go field does not. The plan that changes that field calls `Parse` in the translator and sets the field type to `county.ID`. county-1 does not do that.

Migration numbers collide if two owners hardcode `000003`. On main the next prefix is `000003`. scrape-3 on PR 11 uses `db/migrations/000003_parcel_page.sql` while `000003` is absent, and it already says to pick the next integer if `000003` exists. soil-3 must do the same. county-1 lists `db/migrations` at the start and uses max plus 1. If scrape-3 has landed `000003_parcel_page.sql`, county-1 uses `000004`. soil-3 runs after county-1 is on trunk and uses the same rule, so it does not reuse county-1's file and it does not use `000002`.

`Up` fails if any `parcels.county_id` is not `union`. Lane 1 records the count first. Do not drop rows and do not widen the check to make `Up` pass.

The conversion `ID("wake")` still compiles. `Valid` rejects that value in Go, and the check rejects it in Postgres. Do not tell callers that the type system forbids every bad string.

## Appendix D. Links and reading list

Read before editing.

- https://go.dev/doc/effective_go#package-names
- https://go.dev/wiki/CodeReviewComments#package-names
- `internal/models/gis_parcel.go` lines 1 to 38.
- `internal/connector/union/translator.go` lines 81 to 90.
- `db/migrations/000001_init_schema.sql` lines 5 to 92.
- `db/migrations/000002_parcels_storage_srid.sql`.
- `Makefile` lines 27 to 28 and 82 to 88.
- `plans/soil-layer.md` soil-1 and soil-3.
- PR 11 `plans/scraper-enrichment.md` at commit `a38f56303546f456055d58b52e0f206b1885d84a`, scrape-1 and the `county.CountyID` lines named in Appendix C.

county-1 gets `pstack/skills/how/SKILL.md` before the file is added, because the question is which package owns the id. No PR gets `pstack/skills/interrogate/SKILL.md`. The name choice is the Go rule in Appendix B, not a second design. The execution trail follows `pstack/skills/show-me-your-work/SKILL.md`. This plan file is the trail for the naming decision.

## Appendix E. Decisions recorded

The operator said to move the CountyID as well, in a separate plan, into its own package, as long as that makes sense from a Go perspective.

It does. The package is `internal/county`. The type is `county.ID`. The constant is `county.Union`. The constructor at a boundary is `Parse`. `Valid` remains for conversions. `String` returns the stored text. `All` is the list the migration check must match. There is no `Scanner` and no `Valuer` in county-1.

Per type-system discipline, the id is its own type, and external text stays untyped until `Parse`. Per boundary discipline, that parse sits at the edge, and internal holders of a parsed `ID` do not re-check it. Per model-the-domain, the county id is not a soil type and not a package of unrelated types. Per subtract-before-you-add, county-1 does not invent a domain package, a database interface, or a second county. One PR holds the type, the tests, and the parcels check, so the list and the SQL cannot disagree across merges.

The operator lands county-1. soil-1 and scrape-1 start only after that PR is on trunk.
