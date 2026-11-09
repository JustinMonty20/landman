# Tax data ingest

You want the amount a likely-vacant owner owes, from the two public county files, about twice a year. The devnetwedge site is out. Its robots file disallows the parcel and tax pages. This is a fetch of two files and a parse, not a scraper.

The delinquent list and the billing zip are the sources. Real-estate rows are kept. Railroad rows are dropped. Personal property stays an open question. The run does not insert parcels.

Run tax-1 and tax-2 first. Run tax-3 and tax-4 after those and after county-1. Run tax-5 after tax-3, tax-4, and persist-1.

## How to read this

One box is one unit of work. Every box names the evidence that checks it. A nested box is a sub-step of the box above it. Check a box only when its evidence exists, a file, a log line, a screenshot, a test run, or a SHA. The body is a how-to. The appendices explain and record.

The program runs `pstack/skills/poteto-mode/playbooks/autopilot-stack.md`. The operator lands tax-1, tax-2, tax-3, tax-4, and tax-5. This plan stops at merge-ready. Nobody merges from the checklist.

Tests alone are not sufficient verification. A PR is verified only when its unit, live, and perf boxes are all checked.

## Program checklist

### Arm the program

- [ ] State the protocol and this plan to the operator, then stop. Start execution only on the operator's explicit go.
- [ ] Read these from trunk at program start. Re-read them at every tick.
  - [ ] `git show origin/main:pstack/skills/poteto-mode/playbooks/autopilot-stack.md`
  - [ ] `git show origin/main:pstack/skills/swarm/SKILL.md`
  - [ ] `git show origin/main:pstack/skills/poteto-mode/playbooks/opening-a-pr.md`
  - [ ] `git show origin/main:pstack/skills/control-cli/SKILL.md`
  - [ ] `git show origin/main:plans/tax-data-ingest.md`
  - [ ] `git show origin/main:plans/county-id-package.md`
  - [ ] `git show origin/main:plans/soil-layer.md`
- [ ] If a `git show` of `pstack/` or `control-cli` fails, read the installed skill with the same name and record the miss. Appendix C is that gap. Do not skip the live lanes.
- [ ] On the operator's go, arm the audit tick as `/loop 1h` with the tick prompt below. Never leave the cadence to memory.
- [ ] Use this tick prompt, verbatim. "Re-read the execution playbook from trunk. Audit the operation against it and fix drift in this tick. Probe every active lane and judge progress by side effects only. Stand down a stuck lane and dispatch its replacement now. Then post a short status message to the operator in chat only when the audit found a tracked change that no earlier status message reported, such as a PR opened, a code-ready head, a round launched or closed, a verdict, a merge, a stuck agent and the action taken, a blocker added or cleared, or a decision only the operator can make. Name every such change and nothing else. Do not repeat a table, the merged list, or an unchanged blocker. If the audit found none, end the turn with no reply text. Either way, log this tick's row in your decision trail. The row names the items reported, or none."
- [ ] On the operator's hold or stand-down, send every owner a zero-writes order at once.

### Spawn owners

- [ ] Spawn one owner per PR with the full lifecycle the execution playbook names.
- [ ] Follow this dependency graph. Start dependent work only after its parent is on trunk. Do not stack these PRs. tax-3, tax-4, and tax-5 each need more than one parent.
  - [ ] tax-1 and tax-2 are independent and first. Both branch from trunk.
  - [ ] tax-3 and tax-4 start after tax-1, tax-2, and county-1 are on trunk. They do not edit the same files. county-1 is [PR 12](https://github.com/JustinMonty20/landman/pull/12), `plans/county-id-package.md`. It is not a PR of this program.
  - [ ] tax-5 after tax-3, tax-4, and persist-1 are on trunk. persist-1 is in `plans/soil-layer.md`. Do not edit that file.
- [ ] Hold the file boundaries. tax-1 touches only `internal/taxsource/`. tax-2 touches only the new migration. tax-3 touches only `internal/connector/union/delinquent.go`, `internal/connector/union/delinquent_test.go`, `internal/connector/union/testdata/tax/delinquent.csv`, `internal/taxstore/delinquent.go`, and `internal/taxstore/delinquent_test.go`. tax-4 touches only `internal/connector/union/billing.go`, `internal/connector/union/billing_test.go`, `internal/connector/union/testdata/tax/billing.csv`, `internal/taxstore/bills.go`, and `internal/taxstore/bills_test.go`. tax-5 touches only `main.go`.
- [ ] Hold the review gate. No PR in this program changes an interaction. None wait for screenshots in chat before the operator decides to land them.
- [ ] Do not add a PR that requests `unionnc.devnetwedge.com`. Do not add a worker pool, a raw HTML table, or a per-parcel scrape contract.

### PR mechanics, for every PR

- [ ] Resolve the forge once. Default to `gh`. If `command -v origin` succeeds and Origin can resolve the repository, use `origin pr` for every PR operation. Record any fallback to `gh`. Never require `gt`.
- [ ] Open the PR ready, never draft, per **Opening a PR**. Use the run's built-in PR tool when it has one, else `origin pr create --status open --base <base-branch>` or `gh pr create --base <base-branch>` according to the resolved forge. A stack child targets its parent branch. These PRs are not stacked, so every base is trunk.
- [ ] Run `gofmt`, `go vet ./...`, and `staticcheck ./...` once before the PR-facing push. Push with hooks on. This repo has no `staticcheck` Make target. Install `staticcheck` and fail the push on any finding.
- [ ] Run `/deslop` before each commit and `/no-comments` before review. If `deslop` is not installed, apply the unslop skill to the diff and record the miss.
- [ ] Triage every Bugbot and security-reviewer comment per `pstack/skills/poteto-mode/references/bugbot-triage.md`.
- [ ] Rebase onto current trunk before the code-ready report and babysit. Keep that merge base in fix rounds. Rebase again only at merge prep, on a `git merge-tree` conflict with trunk, on a CI failure that comes from a change on trunk, or when a known concurrent change to the same files lands.

### Verdict and merge, for every PR

- [ ] A full swarm runs once, at the code-ready head. Another full swarm runs only for new behavior, a redesign, or a large new diff. A rebase, a merge-conflict resolution, or a minor fix gets a single verifier at the new head, and a wording-only change gets none. Run each full swarm per `pstack/skills/swarm/SKILL.md`. One gates lane. The ten live lanes from the PR's **Verify, live** block. The perf lane from its **Verify, perf** block. Two or more audit lanes, each with its own focus, that read the diff and the receipts and distrust the PR body. The root audits the receipts in the merge-ready report before the verdict.
- [ ] Judge every finding by the execution playbook's blocking rule. The verdict is clean when none blocks. A failure on trunk behavior in a lane marked `Regression lane against trunk.` is recorded against trunk and does not block the PR. A new head gets a fresh verdict, except for results that stay valid under the patch-id rule in `playbooks/shipping.md`.
- [ ] Stop when the PR is verified and green at its current head, then hand it to the operator. Do not merge. A restack that leaves `git patch-id` unchanged keeps the verdict. After the handoff, nobody pushes without the operator's go.

### Boot recipe, for every live lane

Each live lane runs on its own cloud VM at the PR head. This repo has no browser UI for tax ingest. Drive `go test`, `curl`, and `psql`. There is no `control-cli` skill in the repo. Appendix C records that gap. The commands in the lane are the drive.

- [ ] `git fetch origin <head-branch> && git checkout <head SHA>`.
- [ ] For tax-1, tax-3, and tax-4, run the `go test -race` command in the lane. For tax-2 and tax-5, start Postgres and PostGIS the way `make migrate-up` does, apply migrations, and wait until `psql` accepts a query.
- [ ] Deliver input only through those commands. Read-only diagnostics are the test output, the `curl` body, and the `psql` rows.
- [ ] Save every screenshot to `/tmp/swarm-<pr-id>/worker-<n>/<slug>.png` and return the paths with the report. A lane with no display saves a text render of the command output at that path.
- [ ] A lane that hits the county site does it once, with a descriptive user agent, and does not loop. Do not download the billing zip in more than one lane.

## Add the file GET and the source interface (tax-1)

**Depends on.** None.

**Files.**

- [ ] Create `internal/taxsource/get.go`.
- [ ] Create `internal/taxsource/get_test.go`.
- [ ] Create `internal/taxsource/source.go`.

**Build.**

- [ ] Add `Get(ctx context.Context, url string, limit int64) (io.ReadCloser, error)`. Use an `http.Client` with a timeout argument on the same function, or a `Get` that takes a timeout duration. A zero limit or a zero timeout is an error. The body is capped at `limit`. One byte past the cap returns an error and closes the body.
- [ ] Retry once when the first attempt fails before any body byte is read, and the error is a transport error or an HTTP status from 500 through 599. Do not retry 404. Do not retry when the error matches `context.Canceled` or `context.DeadlineExceeded`. Do not read `Retry-After`. Do not add a rate limiter. Do not fetch `robots.txt`.
- [ ] Add `Meta` with `URL`, `Name`, `SHA256`, and `FetchedAt`. Add `Source[T any]` with `Read(ctx context.Context, yield func(T) error) (Meta, error)`. `Read` is the only method. The shared package does not name a tax column and does not import `internal/county`, `internal/soil`, or `internal/connector/union`.
- [ ] Do not add `DoRetry`, a worker pool, or a per-parcel `Scrape` method.

**You see.**

- [ ] `go test -race ./internal/taxsource/ -count=1 -run TestGet` prints `PASS`.

**Verify, unit.** Tests alone are not sufficient verification. A PR is verified only when its unit, live, and perf boxes are all checked.

- [ ] Table-driven `t.Run` cases cover a 200 body, a 404 with one hit, a 500 then a 200 with two hits, a body one byte over the limit, a canceled context with zero hits, and a second attempt that is not started after a 404. Run `go test -race ./internal/taxsource/ -count=1`.
- [ ] Run `go vet ./internal/taxsource/` and `staticcheck ./internal/taxsource/`. Both print no findings.
- [ ] Run `go test -race -cover ./internal/taxsource/ -count=1`. Fail if coverage is below 90.0 percent. That floor is a chosen gate for a new package.

**Verify, live.** Tests alone are not sufficient verification. A PR is verified only when its unit, live, and perf boxes are all checked. Ten lanes on `grok-4.7-xhigh-fast` at the PR head, per the boot recipe.

- [ ] Lane 1. Regression lane against trunk. Run `go test -race ./internal/connector/shared/httpclient/ -count=1` at trunk and at head. Trunk has no `internal/taxsource`. Record that, and run `go test -race ./internal/taxsource/ -count=1` at head. Save `/tmp/swarm-tax-1/worker-1/regression.png`. Pass when trunk's httpclient tests pass and head's `TestGet` passes.
- [ ] Lane 2. Return 200 with body `ok`. Save `/tmp/swarm-tax-1/worker-2/status-200.png`. Pass when `Get` returns those bytes.
- [ ] Lane 3. Return 404. Save `/tmp/swarm-tax-1/worker-3/status-404.png`. Pass when the error is non-nil and the handler ran once.
- [ ] Lane 4. Return 500, then 200. Save `/tmp/swarm-tax-1/worker-4/retry-500.png`. Pass when the body is the 200 body and the handler ran twice.
- [ ] Lane 5. Return a body one byte over the limit. Save `/tmp/swarm-tax-1/worker-5/oversize.png`. Pass when `Get` returns an error and the body is not returned as success.
- [ ] Lane 6. Cancel the context before `Get`. Save `/tmp/swarm-tax-1/worker-6/cancel.png`. Pass when the error matches `context.Canceled` and the handler ran zero times.
- [ ] Lane 7. Return 404 twice worth of attempts. Save `/tmp/swarm-tax-1/worker-7/no-retry-404.png`. Pass when the handler ran once.
- [ ] Lane 8. Start the handler only after the client has given up. Use a timeout shorter than the handler delay. Save `/tmp/swarm-tax-1/worker-8/timeout.png`. Pass when the error matches `context.DeadlineExceeded` and is not retried into a success.
- [ ] Lane 9. Confirm `Source` is the only interface in the package and it has one method. Save `/tmp/swarm-tax-1/worker-9/one-method.png`. Pass when `go doc` for the package lists `Read` and does not list `Scrape`.
- [ ] Lane 10. Run `go test -race ./internal/connector/shared/httpclient/ -count=1 -run TestRateLimitedClient_Do_RateLimiting`. Save `/tmp/swarm-tax-1/worker-10/gis-limiter.png`. Pass when that existing test still requires at least 900ms for three requests at 2 per second. This PR did not change that limiter.

**Verify, perf.** Tests alone are not sufficient verification. A PR is verified only when its unit, live, and perf boxes are all checked.

- [ ] Metric. Wall time of one local 200 through `Get` of a 300,371 byte body, the measured delinquent file size. Trunk has no `Get`. The end state is a body of that length.
- [ ] Probe. At trunk, record that `internal/taxsource` does not exist. At head, time one `Get` against `httptest` serving 300,371 bytes.
- [ ] Baseline. Record trunk as no package and no elapsed time.
- [ ] Rule. Fail if that head `Get` takes 1 second or longer. Do not report a ratio against trunk. The 1 second gate is a hang check. The file is 300,371 bytes on loopback, so the limiter is the local server, not the county host.

**Review gate.** None. tax-1 is not review-gated.

**Merge.**

- [ ] Root's clean verdict at the exact head SHA.
- [ ] Bugbot triage done.
- [ ] Rebased onto current trunk after the verdict, with `git patch-id` unchanged.
- [ ] The owner does not merge. The operator lands tax-1. It does not wait on tax-2.

## Add the tax tables (tax-2)

**Depends on.** None.

**Files.**

- [ ] Create the next free migration. The name to use while `000003` is absent is `db/migrations/000003_tax_files.sql`. Do not edit `000001_init_schema.sql` or `000002_parcels_storage_srid.sql`. If `000003` exists on trunk at the start of this PR, pick the next integer prefix and record it.

**Build.**

- [ ] Add `tax_delinquent_balances` with `county_id TEXT NOT NULL`, `parcel_id TEXT NOT NULL`, `tax_year INTEGER NOT NULL`, `balance_due NUMERIC(12,2) NOT NULL`, `primary_source TEXT NOT NULL`, `source_url TEXT NOT NULL`, `file_sha256 BYTEA NOT NULL`, and `fetched_at TIMESTAMPTZ NOT NULL`. Primary key `(county_id, parcel_id, tax_year)`. Check `county_id IN ('union')`. Check `balance_due >= 0`.
- [ ] Add `tax_bills` with `county_id TEXT NOT NULL`, `parcel_id TEXT NOT NULL`, `tax_year INTEGER NOT NULL`, `total_taxes NUMERIC(12,2) NOT NULL`, `property_address TEXT NOT NULL`, `taxpayer_name TEXT NOT NULL`, `mailing_line TEXT NOT NULL`, `mailing_city TEXT NOT NULL`, `mailing_state TEXT NOT NULL`, `mailing_postal_code TEXT NOT NULL`, `source_url TEXT NOT NULL`, `file_sha256 BYTEA NOT NULL`, and `fetched_at TIMESTAMPTZ NOT NULL`. Primary key `(county_id, parcel_id, tax_year)`. The same county check. Check `total_taxes >= 0`.
- [ ] Add view `tax_owed`. It joins `tax_delinquent_balances` to `parcels` on `county_id` and `parcel_id = source_parcel_id`, keeps rows with `balance_due > 0`, and sums `balance_due` per parcel. Do not add a column to `parcels`. Do not add `GEOMETRY`. Do not add a foreign key. A tax row may exist before the parcel row.
- [ ] `NUMERIC(12,2)` is the money type. The source amounts are dollars and cents. Numeric is exact. 12 digits covers the measured maximum of 36800.63 with room. Do not store money as `float`.
- [ ] Goose `Up` creates both tables and the view. Goose `Down` drops the view and then the tables.

**You see.**

- [ ] After `make migrate-up`, `psql` lists `tax_delinquent_balances`, `tax_bills`, and `tax_owed`.

**Verify, unit.** Tests alone are not sufficient verification. A PR is verified only when its unit, live, and perf boxes are all checked.

- [ ] `rg -n "parcel_pages|source_extras|GEOMETRY" db/migrations/000003_tax_files.sql` prints nothing. `rg -n "tax_owed" db/migrations/000003_tax_files.sql` matches the view. Adjust the filename if the prefix changed.
- [ ] Run `go test -race ./... -count=1` with no database. It still passes. Run `go vet ./...` and `staticcheck ./...`. Both print no findings.

**Verify, live.** Tests alone are not sufficient verification. A PR is verified only when its unit, live, and perf boxes are all checked. Ten lanes on `grok-4.7-xhigh-fast` at the PR head, per the boot recipe.

- [ ] Lane 1. Regression lane against trunk. At trunk and at head, run `psql` with `\dt tax_delinquent_balances`. Save `/tmp/swarm-tax-2/worker-1/regression.png`. Pass when trunk has no such table and head lists it.
- [ ] Lane 2. Insert one delinquent row for county `union`, parcel `09336350`, year 2025, balance 10.00. Save `/tmp/swarm-tax-2/worker-2/insert-delinquent.png`. Pass when the select returns that balance.
- [ ] Lane 3. Upsert the same key with balance 12.50. Save `/tmp/swarm-tax-2/worker-3/upsert-delinquent.png`. Pass when the count is 1 and the balance is 12.50.
- [ ] Lane 4. Insert county `wake`. Save `/tmp/swarm-tax-2/worker-4/bad-county.png`. Pass when the county check rejects the row.
- [ ] Lane 5. Insert a negative balance. Save `/tmp/swarm-tax-2/worker-5/negative.png`. Pass when the check rejects the row.
- [ ] Lane 6. Insert one bill row for the same parcel and year. Save `/tmp/swarm-tax-2/worker-6/insert-bill.png`. Pass when `total_taxes` and `mailing_line` are stored.
- [ ] Lane 7. Insert a `parcels` row with `source_parcel_id` `09336350` and select `tax_owed`. Save `/tmp/swarm-tax-2/worker-7/owed-join.png`. Pass when the view shows that parcel and the summed balance.
- [ ] Lane 8. Select `tax_owed` with no matching parcel row. Save `/tmp/swarm-tax-2/worker-8/owed-empty.png`. Pass when the view returns no row for a delinquent key that is absent from `parcels`.
- [ ] Lane 9. Run goose down, then up. Save `/tmp/swarm-tax-2/worker-9/down-up.png`. Pass when the tables are gone after down and present after up.
- [ ] Lane 10. Count `parcels` before and after a delinquent insert. Save `/tmp/swarm-tax-2/worker-10/no-parcel-insert.png`. Pass when the `parcels` count is unchanged.

**Verify, perf.** Tests alone are not sufficient verification. A PR is verified only when its unit, live, and perf boxes are all checked.

- [ ] Metric. Wall time of one upsert into `tax_delinquent_balances`, and the row count after a second upsert of the same key. Trunk has no table. The end state is one row.
- [ ] Probe. At trunk, record that `\dt tax_delinquent_balances` fails. At head, time one upsert, then a second upsert, with `psql`.
- [ ] Baseline. Record trunk as no table and no elapsed time.
- [ ] Rule. Fail if the head upsert takes 5 seconds or longer, or if the second upsert leaves a count other than 1. Do not report a ratio against trunk.

**Review gate.** None. tax-2 is not review-gated.

**Merge.**

- [ ] Root's clean verdict at the exact head SHA.
- [ ] Bugbot triage done.
- [ ] Rebased onto current trunk after the verdict, with `git patch-id` unchanged.
- [ ] The owner does not merge. The operator lands tax-2. It does not wait on tax-1.

## Ingest the delinquent list (tax-3)

**Depends on.** tax-1, tax-2, and county-1.

**Files.**

- [ ] Create `internal/connector/union/delinquent.go`.
- [ ] Create `internal/connector/union/delinquent_test.go`.
- [ ] Create `internal/connector/union/testdata/tax/delinquent.csv`.
- [ ] Create `internal/taxstore/delinquent.go`.
- [ ] Create `internal/taxstore/delinquent_test.go`.

**Build.**

- [ ] Add `DelinquentRow` with `County county.ID`, `ParcelID`, `Year`, `Balance`, and `PrimarySource`. Add `DelinquentList` with `Read` so that `var _ taxsource.Source[DelinquentRow] = (*DelinquentList)(nil)` compiles in `delinquent.go`.
- [ ] Resolve the file with `GET` of `https://www.unioncountync.gov/home/showdocument?id=4792`. Do not send the `t` query parameter. Appendix A measured that this URL returns the CSV. If the first record's names do not include `Property Key`, `Year`, `Primary Source`, and `Balance Due`, return an error and yield nothing. Do not guess another document id. Do not fetch the foreclosure HTML for a new link in this PR.
- [ ] Parse with `encoding/csv`. Read columns by those names. Ignore extra empty columns. Trim the key and uppercase it. Do not remove an internal space and do not drop a suffix. `07105038A` stays `07105038A`. `02176011 60` stays `02176011 60`.
- [ ] Yield a row only when `Primary Source` is `RE`. Drop `RR`. Skip `PP` and count the skip. Parse `Balance Due` by removing `$` and commas. Store dollars and cents. A balance that does not parse is an error. The county on every yielded row is `county.Union`.
- [ ] `SaveDelinquent` upserts on `(county_id, parcel_id, tax_year)` and replaces `balance_due`, `primary_source`, `source_url`, `file_sha256`, and `fetched_at`. `file_sha256` is the SHA-256 of the file bytes. Do not insert into `parcels`. A second `Read` of the same bytes yields the same keys and balances.
- [ ] The golden file is a few RE rows, one PP row, and one RR row copied from the measured file, including one 8-digit key, one 9-character key, and one 11-character key. `-update` rewrites it and is off by default.

**You see.**

- [ ] `go test -race ./internal/connector/union/ ./internal/taxstore/ -count=1 -run 'TestDelinquent|TestSaveDelinquent'` prints `PASS` and the golden RE balance matches.

**Verify, unit.** Tests alone are not sufficient verification. A PR is verified only when its unit, live, and perf boxes are all checked.

- [ ] Table-driven tests cover the golden RE rows, a skipped PP row, a dropped RR row, a missing `Property Key` header, a bad balance, and a canceled context that yields nothing. Run `go test -race ./internal/connector/union/ -count=1 -run TestDelinquent`.
- [ ] Run `go test -race ./internal/connector/union/ -fuzz=FuzzDelinquentMoney -fuzztime=10s` and `go test -race ./internal/connector/union/ -fuzz=FuzzParcelKey -fuzztime=10s`. Both exit 0.
- [ ] Run `go vet ./internal/connector/union/ ./internal/taxstore/` and `staticcheck ./internal/connector/union/ ./internal/taxstore/`. Both print no findings.
- [ ] Run `go test -race -cover ./internal/connector/union/ ./internal/taxstore/ -count=1`. Fail if `union` coverage is below 76.8 percent. Trunk measured 76.8 percent. Fail if `taxstore` coverage is below 90.0 percent. That floor is a chosen gate for a new package.

**Verify, live.** Tests alone are not sufficient verification. A PR is verified only when its unit, live, and perf boxes are all checked. Ten lanes on `grok-4.7-xhigh-fast` at the PR head, per the boot recipe.

- [ ] Lane 1. Regression lane against trunk. Run `go test -race ./internal/connector/union/ -count=1` at trunk and at head. Trunk has no `DelinquentList`. Record that, and gate head's new tests plus a green union package. Save `/tmp/swarm-tax-3/worker-1/regression.png`. Pass when trunk's union tests pass and head's `TestDelinquent` passes.
- [ ] Lane 2. Parse the golden file. Save `/tmp/swarm-tax-3/worker-2/golden.png`. Pass when the 8-digit key, the letter-suffix key, and the 11-character key are present and the PP and RR rows are absent.
- [ ] Lane 3. `SaveDelinquent` one golden row, then save it again with the same balance. Save `/tmp/swarm-tax-3/worker-3/upsert.png`. Pass when the table has one row and `file_sha256` is 32 bytes.
- [ ] Lane 4. Change the balance on the second save. Save `/tmp/swarm-tax-3/worker-4/replace.png`. Pass when the count is still 1 and the balance is the second value.
- [ ] Lane 5. Count `parcels` around the save. Save `/tmp/swarm-tax-3/worker-5/no-parcel.png`. Pass when the count is unchanged.
- [ ] Lane 6. Fetch `https://www.unioncountync.gov/home/showdocument?id=4792` once. Save `/tmp/swarm-tax-3/worker-6/live-csv.png`. Pass when the first line contains `Property Key` and `Balance Due`. One request. Do not pass a `t` parameter.
- [ ] Lane 7. From that same saved body, count RE, PP, and RR. Save `/tmp/swarm-tax-3/worker-7/live-counts.png`. Pass when RE is 926, PP is 5013, and RR is 3, or record the new counts and stop if the header names changed.
- [ ] Lane 8. Sum RE `Balance Due` from that body. Save `/tmp/swarm-tax-3/worker-8/live-sum.png`. Pass when the sum is 232755.26, or record the new sum next to the header check from lane 7.
- [ ] Lane 9. Cancel `Read` before the request. Save `/tmp/swarm-tax-3/worker-9/cancel.png`. Pass when the error matches `context.Canceled` and no row is yielded.
- [ ] Lane 10. Run `tax_owed` after saving one RE row and inserting a matching `parcels` row in the test database. Save `/tmp/swarm-tax-3/worker-10/owed.png`. Pass when the view returns that parcel. A parcel id with no `parcels` row stays out of the view.

**Verify, perf.** Tests alone are not sufficient verification. A PR is verified only when its unit, live, and perf boxes are all checked.

- [ ] Metric. Wall time to parse the measured 5,942-row delinquent file from disk, and the RE yield count. Trunk has no parser. The end state is 926 RE rows.
- [ ] Probe. At trunk, record that `DelinquentList` does not exist. At head, time one parse of a local copy of the file. Do not include the county download in the timer.
- [ ] Baseline. Record trunk as no parser and no elapsed time.
- [ ] Rule. Fail if the head parse takes 1 second or longer, or if the RE yield count is not 926 for that measured file. A newer county file with a different count fails this box until Appendix A is updated with the new count. Do not report a ratio against trunk.

**Review gate.** None. tax-3 is not review-gated.

**Merge.**

- [ ] Root's clean verdict at the exact head SHA.
- [ ] Bugbot triage done.
- [ ] Rebased onto current trunk after the verdict, with `git patch-id` unchanged. county-1 is already on that trunk.
- [ ] The owner does not merge. The operator lands tax-3 after tax-1, tax-2, and county-1.

## Ingest the billing file (tax-4)

**Depends on.** tax-1, tax-2, and county-1.

**Files.**

- [ ] Create `internal/connector/union/billing.go`.
- [ ] Create `internal/connector/union/billing_test.go`.
- [ ] Create `internal/connector/union/testdata/tax/billing.csv`.
- [ ] Create `internal/taxstore/bills.go`.
- [ ] Create `internal/taxstore/bills_test.go`.

**Build.**

- [ ] Add `BillRow` with `County county.ID`, `ParcelID`, `Year`, `TotalTaxes`, `PropertyAddress`, and the mailing fields on `tax_bills`. Add `BillingFile` with `Read` so that `var _ taxsource.Source[BillRow] = (*BillingFile)(nil)` compiles.
- [ ] Resolve the URL from the Parcels table on `https://www.unioncountync.gov/government/departments-f-p/gis-mapping/downloadable-gis-data`. Use the link whose label is `Billing Data`. Do not hardcode the numeric timestamp in the path. If that label is missing, return an error.
- [ ] `Get` the zip with a limit of 256 MiB. That cap is chosen. Appendix A says this session did not obtain a clean zip length, and the operator measured an uncompressed CSV of about 124 MB on 2026-11-08. Write the body to a temp file. Open it with `archive/zip`. A zip directory is at the end of the file, so do not parse the CSV from the socket. Stream the `WKBILA-2026-07-22-1222.csv` entry with `encoding/csv`. Fail if the uncompressed entry exceeds 256 MiB. Delete the temp file when `Read` returns.
- [ ] Require `property_key`, `source`, `year`, `TOTAL TAXES AND CHARGES`, `PROPERTY ADDRESS`, `Taxpayer Name 1`, and `Taypayer Address 1`. The address header is spelled `Taypayer` in the operator's column list. Match that spelling. A missing required name fails the parse and yields nothing. Yield `source` `RE` only. Skip `PP`. There is no paid column and no balance column. Do not invent one. `TotalTaxes` is the billed amount.
- [ ] Use the same key rule as tax-3. The county is `county.Union`. `SaveBill` upserts on `(county_id, parcel_id, tax_year)`. The golden file is a few RE rows and one PP row, including parcel `09336350` if a row is copied from a real file. `-update` is off by default.
- [ ] Do not download the zip in `go test` unless the test points at a local file. The live sample is one lane.

**You see.**

- [ ] `go test -race ./internal/connector/union/ ./internal/taxstore/ -count=1 -run 'TestBilling|TestSaveBill'` prints `PASS` without contacting the county host.

**Verify, unit.** Tests alone are not sufficient verification. A PR is verified only when its unit, live, and perf boxes are all checked.

- [ ] Table-driven tests cover the golden RE row, a skipped PP row, a missing `property_key` header, a zip whose entry exceeds the cap, and a canceled context. Run `go test -race ./internal/connector/union/ -count=1 -run TestBilling`.
- [ ] Run `go test -race ./internal/connector/union/ -fuzz=FuzzBillingMoney -fuzztime=10s`. It exits 0.
- [ ] Run `go vet ./internal/connector/union/ ./internal/taxstore/` and `staticcheck ./internal/connector/union/ ./internal/taxstore/`. Both print no findings.
- [ ] Run `go test -race -cover ./internal/connector/union/ ./internal/taxstore/ -count=1`. Fail if `union` coverage is below 76.8 percent or `taxstore` coverage is below 90.0 percent.

**Verify, live.** Tests alone are not sufficient verification. A PR is verified only when its unit, live, and perf boxes are all checked. Ten lanes on `grok-4.7-xhigh-fast` at the PR head, per the boot recipe.

- [ ] Lane 1. Regression lane against trunk. Run `go test -race ./internal/connector/union/ -count=1 -run TestDelinquent` at trunk and at head. Trunk has no billing parser. Record that. Save `/tmp/swarm-tax-4/worker-1/regression.png`. Pass when head's delinquent tests still pass and `TestBilling` passes.
- [ ] Lane 2. Parse the golden CSV. Save `/tmp/swarm-tax-4/worker-2/golden.png`. Pass when parcel `09336350` yields a total and a mailing line, and the PP row is absent.
- [ ] Lane 3. `SaveBill` twice for the same key. Save `/tmp/swarm-tax-4/worker-3/upsert.png`. Pass when the count is 1.
- [ ] Lane 4. Count `parcels` around the save. Save `/tmp/swarm-tax-4/worker-4/no-parcel.png`. Pass when the count is unchanged.
- [ ] Lane 5. Open a local zip with one CSV entry and stream it. Save `/tmp/swarm-tax-4/worker-5/zip-stream.png`. Pass when the row count matches the entry and the process does not hold the entry as one string. The test compares yielded rows with a second streaming pass.
- [ ] Lane 6. Fetch the GIS download page once and print the `Billing Data` href. Save `/tmp/swarm-tax-4/worker-6/billing-href.png`. Pass when the href contains `showpublisheddocument` and does not point at `unionnc.devnetwedge.com`.
- [ ] Lane 7. Download that href once, only in this lane. Save `/tmp/swarm-tax-4/worker-7/billing-zip.png`. Pass when the body starts with a zip signature and an entry name contains `WKBILA` and `.csv`. Record the byte count. Do not repeat this download in another lane.
- [ ] Lane 8. Read the CSV header from that zip. Save `/tmp/swarm-tax-4/worker-8/billing-header.png`. Pass when `property_key` and `TOTAL TAXES AND CHARGES` are present. If `Taypayer Address 1` is absent, fail and record the address header that is present.
- [ ] Lane 9. Count `source` values and the distinct `year` in that CSV. Save `/tmp/swarm-tax-4/worker-9/billing-counts.png`. Pass when the counts are recorded. The operator's 2026-11-08 counts were RE 116,421, PP 21,984, and year 2026 only. A difference does not fail the lane. Write the new counts in the lane report.
- [ ] Lane 10. Find `property_key` `09336350` in that CSV. Save `/tmp/swarm-tax-4/worker-10/example-parcel.png`. Pass when the row is RE, year 2026, and the address contains `CRIPPLE CREEK`, or record that the key is absent.

**Verify, perf.** Tests alone are not sufficient verification. A PR is verified only when its unit, live, and perf boxes are all checked.

- [ ] Metric. Wall time to stream-parse the golden billing CSV, and the peak row buffer. Trunk has no billing parser. The end state is the golden row count. The full county zip is not this metric.
- [ ] Probe. At trunk, record that `BillingFile` does not exist. At head, time 20 parses of the golden file.
- [ ] Baseline. Record trunk as no parser and no elapsed time. Lane 7's byte count is the download size, not this timer.
- [ ] Rule. Fail if the average of 20 head parses of the golden file is 1 second or longer. Do not fail because the real zip is large. Do not report a ratio against trunk.

**Review gate.** None. tax-4 is not review-gated.

**Merge.**

- [ ] Root's clean verdict at the exact head SHA.
- [ ] Bugbot triage done.
- [ ] Rebased onto current trunk after the verdict, with `git patch-id` unchanged.
- [ ] The owner does not merge. The operator lands tax-4 after tax-1, tax-2, and county-1.

## Wire the tax run (tax-5)

**Depends on.** tax-3, tax-4, and persist-1.

**Files.**

- [ ] Edit `main.go`.

**Build.**

- [ ] Add a default-off switch. When it is off, `main` does what it does on trunk after persist-1. It does not download either county file.
- [ ] When the switch is on, call `DelinquentList.Read` and `SaveDelinquent`, then `BillingFile.Read` and `SaveBill`. One file at a time. No worker pool. A canceled context stops the second file. An error from either file is returned. Do not log it and return nil.
- [ ] Do not insert parcels. Do not call the vacant filter inside the tax parse. `tax_owed` is the join to `parcels`. persist-1 is what writes the parcel rows the view can see. There is no `likely_vacant` column. Appendix A records that.
- [ ] Do not import `internal/taxsource` from `main` if `main` only calls the union and taxstore functions. Do not add `func init`.

**You see.**

- [ ] With the switch off, `go test` of the main package, or a short run that does not reach the GIS host, prints parcel ids the way trunk does and does not mention `showdocument` or `showpublisheddocument`.

**Verify, unit.** Tests alone are not sufficient verification. A PR is verified only when its unit, live, and perf boxes are all checked.

- [ ] A test with the switch off does not call `Read`. A test with the switch on uses local files and writes both tables. Run `go test -race . -count=1` from the module root for the main package tests, and `go test -race ./internal/taxstore/ -count=1`.
- [ ] Run `go vet ./...` and `staticcheck ./...`. Both print no findings.
- [ ] `rg -n "devnetwedge|func init\\(" --glob '*.go'` prints nothing.

**Verify, live.** Tests alone are not sufficient verification. A PR is verified only when its unit, live, and perf boxes are all checked. Ten lanes on `grok-4.7-xhigh-fast` at the PR head, per the boot recipe.

- [ ] Lane 1. Regression lane against trunk. Run `go test -race ./internal/service/ ./internal/connector/union/ -count=1` at trunk and at head. Save `/tmp/swarm-tax-5/worker-1/regression.png`. Pass when both pass. Trunk's `main` has no tax switch. Record that.
- [ ] Lane 2. Run the default path's unit test. Save `/tmp/swarm-tax-5/worker-2/switch-off.png`. Pass when no county file URL is requested.
- [ ] Lane 3. Run the switch-on path against local files. Save `/tmp/swarm-tax-5/worker-3/switch-on.png`. Pass when one delinquent row and one bill row are stored.
- [ ] Lane 4. Select `tax_owed` after that run and a matching `parcels` row. Save `/tmp/swarm-tax-5/worker-4/owed.png`. Pass when the parcel id is the local fixture id and the balance is greater than 0.
- [ ] Lane 5. Repeat the switch-on run. Save `/tmp/swarm-tax-5/worker-5/second-run.png`. Pass when the counts stay 1 and 1.
- [ ] Lane 6. Cancel during the first file. Save `/tmp/swarm-tax-5/worker-6/cancel.png`. Pass when the second file is not read.
- [ ] Lane 7. Count `parcels` around the run. Save `/tmp/swarm-tax-5/worker-7/no-insert.png`. Pass when the tax run did not insert a parcel.
- [ ] Lane 8. Select `source_extras` for the fixture parcel. Save `/tmp/swarm-tax-5/worker-8/extras.png`. Pass when the tax run did not change it.
- [ ] Lane 9. Show the `main.go` call order. Save `/tmp/swarm-tax-5/worker-9/order.png`. Pass when delinquent `Read` appears before billing `Read`, both inside the switch, and `IsLikelyVacantGISRecord` is not called from the tax functions.
- [ ] Lane 10. Run `go test -race ./internal/connector/union/ -count=1 -run 'TestDelinquent|TestBilling'`. Save `/tmp/swarm-tax-5/worker-10/parsers.png`. Pass when both pass.

**Verify, perf.** Tests alone are not sufficient verification. A PR is verified only when its unit, live, and perf boxes are all checked.

- [ ] Metric. Wall time of one local switch-on run over the golden files, and the row counts. Trunk has no tax switch. The end state is one delinquent row and one bill row.
- [ ] Probe. At trunk, time `go test -race ./internal/service/ -count=1`. At head, time the local switch-on test.
- [ ] Baseline. Record the trunk service test duration. Record trunk tax run as absent.
- [ ] Rule. Fail if the head local switch-on run takes 5 seconds or longer, or if either count is not 1. Do not report a ratio against the trunk service test. That test does not write tax rows.

**Review gate.** None. tax-5 is not review-gated.

**Merge.**

- [ ] Root's clean verdict at the exact head SHA.
- [ ] Bugbot triage done.
- [ ] Rebased onto current trunk after the verdict, with `git patch-id` unchanged.
- [ ] The owner does not merge. The operator lands tax-5 after tax-3, tax-4, and persist-1.

## Close the program

- [ ] Every box above is checked with its evidence.
- [ ] Reply to the operator with the report the execution playbook names. List each PR with its verdict. Do not claim a merge.

## Appendix A. Prototype evidence

**Measured.** On 2026-11-09 a fetch of `https://www.unioncountync.gov/home/showdocument?id=4792` with no `t` parameter returned the delinquent CSV. The body was 300,371 bytes and 5,942 data rows. The header names were `Property Display Name`, `Property Key`, `Year`, `Primary Source`, and `Balance Due`, followed by empty columns. A direct `curl` from this agent host to the same county site returned Akamai 403. The successful fetch was not that `curl`.

**Measured.** Primary source counts were PP 5,013, RE 926, and RR 3. Years were 2017 through 2025. RE balance summed to 232755.26. The largest RE balance in the sample head was 14355.02 on key `09125043`. Distinct RE keys were 325. RE key lengths were 8 on 629 rows, 9 on 280 rows, and 11 on 17 rows. Parcel `09336350` was not in this file.

**Measured.** Of the 280 nine-character RE keys, 278 are eight digits plus one letter, for example `07105038A`. Two rows are `U4225003F`. The 11-character keys include `07027002J80`, `07114083A02`, `02176011 60`, and `N7027327 01`. The space is inside the key. RR keys are `21` and `26`, two digits, not GIS parcel ids.

**Measured.** The GIS download page lists Billing Data updated 08/27/2026. A fetch of `https://www.unioncountync.gov/home/showpublisheddocument/6045/639234409945470000` began with a zip signature and the entry name `WKBILA-2026-07-22-1222.csv`. The copy saved by that fetch was 9,574,947 bytes and was not a valid zip. It contained UTF-8 replacement bytes. This session did not re-count the billing rows.

**Read.** The operator measured the billing zip on 2026-11-08. One CSV, about 124 MB, 138,405 rows, RE 116,421, PP 21,984, year 2026, date due 2027-01-06. Parcel `09336350` was RE, total 2737.12, address `2915 CRIPPLE CREEK CT MONROE NC 28110`. The operator counted 320 of 325 delinquent RE keys on that billing file. This session did not repeat that match. The billing file has no paid column and no balance column.

**Read.** `IsLikelyVacantGISRecord` is a function in `internal/connector/union/vacant_filter.go`. It returns true when `YEARBLT` or `SQFT` is numeric 0, when `FMV_IMPRV` is a zero currency string, or when `LAND_TYPE` contains `RURAL ACREAGE TRACT`. `main.go` passes it to the GIS id stream. `parcels` has no vacant column. The unique key is `(county_id, source_name, source_parcel_id)` at `db/migrations/000001_init_schema.sql` line 73. Nothing in Go inserts `parcels` on this branch.

**Read.** county-1 in `plans/county-id-package.md` on PR 12 defines `type ID string` in `internal/county`, with `county.Union`, `Parse`, and `Valid`. This plan uses `county.ID`. It does not declare another county type.

The chosen signatures are the contract.

```go
package taxsource

type Meta struct {
	URL       string
	Name      string
	SHA256    [32]byte
	FetchedAt time.Time
}

type Source[T any] interface {
	Read(ctx context.Context, yield func(T) error) (Meta, error)
}
```

```go
func Get(ctx context.Context, rawURL string, limit int64, timeout time.Duration) (io.ReadCloser, error)
```

Open questions.

1. How often should the run execute? The operator said maybe twice a year. This plan does not pick a calendar.
2. Where is the likely-vacant set stored? The predicate is a Go function. There is no column. `tax_owed` joins `parcels`. persist-1 is not on trunk. Whether that table will hold only the filtered ids is still the soil plan's question.
3. Should personal-property rows be stored? This plan skips them and counts the skip. It does not decide to keep them.
4. What should happen when a required column is renamed? This plan fails the file and yields nothing. An alias list is not defined.

## Appendix B. Alternatives rejected

`Read(ctx) ([]T, error)` loses. The billing CSV is about 124 MB and 138,405 rows on the operator's count. A slice holds every row before the first upsert. A cancel mid-file has already paid for the whole parse. The callback yields one row at a time.

A public fetch plus a public parse loses. The caller would open the body and then parse it. A zip has to be on disk before `archive/zip` can read the directory at the end. Hiding that inside `Read` keeps one call. The CSV parser stays a function over an `io.Reader` so the golden file and the fuzz target do not open a socket. That function is not the interface.

`iter.Seq2` loses as the interface return. A parse error in the middle of a range has no `error` result on the method. A `yield func(T) error` returns that error from `Read`. Go 1.25 could express an iterator. This plan does not.

A per-parcel scraper, a worker pool, `Retry-After`, and a `parcel_pages` HTML table lose. The operator rejected the devnetwedge site. Two files a couple of times a year do not need a token bucket or a pool. One retry before the first body byte is the whole retry policy.

The annual 105-369 lien PDF loses. The operator measured it as a March snapshot with no parcel ids. The search result for the 2025 notice lists owner names and street text, not GIS keys. It is not a source.

`unionnc.devnetwedge.com` loses. `robots.txt` disallows `/parcel/*` and `/taxbill` for every user agent. This plan does not design around that.

Railroad rows lose. Three rows, keys `21` and `26`. Those are not GIS parcel ids. Real-estate rows stay.

## Appendix C. Risks

`curl` from this agent host to `www.unioncountync.gov` returned Akamai 403. A later fetch of the same document URL returned the CSV. Live lane 6 of tax-3 can fail on a host Akamai blocks. That failure is real. Do not treat 403 as a pass.

The billing zip saved in this session was corrupt. tax-4 lane 7 is the clean byte count. Until that count exists, the 256 MiB cap is a chosen ceiling at about twice the operator's 124 MB CSV, not a measured zip length. A zip directory sits at the end of the file, so the download finishes before the CSV parse starts.

The GIS download page says the data is presented as is and that the user holds the county harmless. The operator chose these two files anyway. The plan does not add a click-through.

`control-cli` is not in this repo. Live lanes drive `go test`, `curl`, and `psql`, and they save a text render at the `.png` path. This repo has no `pstack/` tree. `git show origin/main:pstack/...` fails until those files exist on trunk.

`tax_owed` is empty until persist-1 writes parcel rows. The tax tables do not insert those rows. A delinquent key that is not a GIS PID stays out of the view and stays in `tax_delinquent_balances`.

county-1 stacks on this docs branch today. Implementation PRs wait until county-1 is on trunk. They do not import `internal/soil` for the county id.

The header spelling `Taypayer Address 1` is the operator's reading of the billing file. Lane 8 records the spelling this program actually sees. A mismatch fails the parse.

## Appendix D. Links and reading list

Read before editing.

- `internal/connector/union/vacant_filter.go` lines 18 to 62.
- `main.go` lines 13 to 47.
- `db/migrations/000001_init_schema.sql` lines 5 to 73.
- `plans/county-id-package.md` for `county.ID`.
- `plans/soil-layer.md` persist-1. Do not edit that file.
- The foreclosure page and the GIS download page named in Appendix A.
- Effective Go and Go Code Review Comments. Small interfaces, accept interfaces and return structs, `context.Context` first, and do not repeat the package name in the type.

tax-3 and tax-4 get `pstack/skills/how/SKILL.md` before the parser is split from `Read`, because the question is which package owns the CSV columns. The execution trail follows `pstack/skills/show-me-your-work/SKILL.md`.

## Appendix E. Decisions recorded

The operator said the devnetwedge tax site is out, and said he does not want a records request if the two public files are enough. He asked whether this is still web scraping or a data fetching step. The answer he accepted is a fetch-and-parse step, for now. The run is periodic. He said maybe twice a year. It targets likely-vacant parcels, not every parcel.

This plan replaces the scraper plan. The old path stays as a pointer so links in the soil plan still resolve. The work is tax-1 through tax-5.

`county.ID` comes from county-1 in `internal/county`. This plan does not declare the type and does not edit `plans/soil-layer.md`.

Real-estate rows are stored. Railroad rows are dropped. Personal property is skipped until open question 3 is answered. Money is `NUMERIC(12,2)`. The file hash and `fetched_at` sit on each row so a rerun upserts in place. The write does not insert parcels. `tax_owed` is the join that limits the priority list to parcel ids already stored.

The stable delinquent URL is `showdocument?id=4792` with no `t` parameter. The billing link is read from the GIS download page so a changed timestamp does not stick in code.
