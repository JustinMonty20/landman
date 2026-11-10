# Tax data ingest

You want the amount a likely-vacant owner owes, from public county files, so a balance can flag a likely seller. DevNet wEdge stays out. Its robots file disallows the parcel and tax pages. This is a fetch and a parse, not a scraper.

The county-neutral source reads four Union files. They are the billing file, the delinquent tax notice, the foreclosure sales list, and the upcoming foreclosure list. Personal-property rows are ignored. Each file lands in typed tables shaped like `parcels` and `parcel_sales`. A query joins them on `parcels.id` and, where the file has one, `tax_year`. The run does not insert parcels. `parcels.likely_vacant` comes from persist-1 in `plans/soil-layer.md`. This plan does not add that column.

Run tax-1 first. Run tax-2 after persist-1. Run tax-3, tax-4, tax-5, and tax-7 after tax-1 and tax-2. Run tax-6 after those four. The production run is about twice a year, and the period is config.

## How to read this

One box is one unit of work. Every box names the evidence that checks it. A nested box is a sub-step of the box above it. Check a box only when its evidence exists, a file, a log line, a screenshot, a test run, or a SHA. The body is a how-to. The appendices explain and record.

The program runs `pstack/skills/poteto-mode/playbooks/autopilot-stack.md`. The operator lands tax-1, tax-2, tax-3, tax-4, tax-5, tax-7, and tax-6. This plan stops at merge-ready. Nobody merges from the checklist.

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
  - [ ] `git show origin/main:plans/soil-layer.md`
  - [ ] `git show origin/main:internal/county/county.go`
  - [ ] `git show origin/main:internal/parcelstore/sales.go`
  - [ ] `git show origin/main:db/migrations/000004_parcel_sales_uuid_id.sql`
- [ ] If a `git show` of `pstack/` or `control-cli` fails, read the installed skill with the same name and record the miss. Appendix C is that gap. Do not skip the live lanes.
- [ ] On the operator's go, arm the audit tick as `/loop 1h` with the tick prompt below. Never leave the cadence to memory.
- [ ] Use this tick prompt, verbatim. "Re-read the execution playbook from trunk. Audit the operation against it and fix drift in this tick. Probe every active lane and judge progress by side effects only. Stand down a stuck lane and dispatch its replacement now. Then post a short status message to the operator in chat only when the audit found a tracked change that no earlier status message reported, such as a PR opened, a code-ready head, a round launched or closed, a verdict, a merge, a stuck agent and the action taken, a blocker added or cleared, or a decision only the operator can make. Name every such change and nothing else. Do not repeat a table, the merged list, or an unchanged blocker. If the audit found none, end the turn with no reply text. Either way, log this tick's row in your decision trail. The row names the items reported, or none."
- [ ] On the operator's hold or stand-down, send every owner a zero-writes order at once.

### Spawn owners

- [ ] Spawn one owner per PR with the full lifecycle the execution playbook names.
- [ ] Follow this dependency graph. Start dependent work only after its parent is on trunk. Do not stack these PRs. tax-2, tax-3, tax-4, tax-5, tax-7, and tax-6 each need a parent that is not this program's first PR.

```mermaid
flowchart LR
  persist1[persist-1] --> tax2[tax-2]
  tax1[tax-1] --> tax3[tax-3]
  tax1 --> tax4[tax-4]
  tax1 --> tax5[tax-5]
  tax1 --> tax7[tax-7]
  tax2 --> tax3
  tax2 --> tax4
  tax2 --> tax5
  tax2 --> tax7
  tax3 --> tax6[tax-6]
  tax4 --> tax6
  tax5 --> tax6
  tax7 --> tax6
```

  - [ ] tax-1 is first and branches from trunk.
  - [ ] tax-2 after persist-1 is on trunk. persist-1 is in `plans/soil-layer.md`. It adds `parcels.likely_vacant`. Do not edit that plan. Do not add the column in this program. soil-3 may add `<prefix>_soil.sql` first. tax-2 does not wait on soil-3 for behavior. It takes the next free prefix after every six-digit file present at the start of the PR, including that soil file when it is already there.
  - [ ] tax-3, tax-4, tax-5, and tax-7 start after tax-1 and tax-2 are on trunk. They do not edit the same files. `county.ID` is already on trunk in `internal/county`. They do not wait on a county PR.
  - [ ] tax-6 after tax-3, tax-4, tax-5, and tax-7 are on trunk.
- [ ] Hold the file boundaries. tax-1 touches only `internal/taxsource/`. tax-2 touches only the one new tax migration. tax-3 touches only `internal/connector/union/delinquent.go`, `internal/connector/union/delinquent_test.go`, `internal/connector/union/testdata/tax/delinquent.csv`, `internal/taxstore/delinquent.go`, and `internal/taxstore/delinquent_test.go`. tax-4 touches only `internal/connector/union/billing.go`, `internal/connector/union/billing_test.go`, `internal/connector/union/testdata/tax/billing.csv`, `internal/taxstore/bills.go`, and `internal/taxstore/bills_test.go`. tax-5 touches only `internal/connector/union/foreclosure.go`, `internal/connector/union/foreclosure_test.go`, `internal/connector/union/testdata/tax/foreclosure.html`, `internal/taxstore/foreclosure.go`, and `internal/taxstore/foreclosure_test.go`. tax-7 touches only `internal/connector/union/pipeline.go`, `internal/connector/union/pipeline_test.go`, `internal/connector/union/testdata/tax/pipeline.pdf`, `internal/connector/union/testdata/tax/pipeline.pptx`, `internal/taxstore/pipeline.go`, and `internal/taxstore/pipeline_test.go`. tax-6 touches only `main.go` and `main_test.go`.
- [ ] Hold the review gate. No PR in this program changes an interaction. None wait for screenshots in chat before the operator decides to land them.
- [ ] Do not add a PR that requests `unionnc.devnetwedge.com`. Do not file a records request. Do not add a worker pool, a raw HTML table of DevNet pages, or a per-parcel scrape contract. Do not edit `plans/scraper-enrichment.md` or `plans/soil-layer.md`.

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
- [ ] For tax-1, run the `go test -race` command in the lane. For tax-2, tax-3, tax-4, tax-5, tax-7, and tax-6, start Postgres and PostGIS the way `make migrate-up` does, apply migrations through the head, and wait until `psql` accepts a query. Parser tests that do not open a database stay on `go test` without `DATABASE_URL`.
- [ ] Deliver input only through those commands. Read-only diagnostics are the test output, the `curl` body, and the `psql` rows.
- [ ] Save every screenshot to `/tmp/swarm-<pr-id>/worker-<n>/<slug>.png` and return the paths with the report. A lane with no display saves a text render of the command output at that path.
- [ ] A lane that hits the county site does it once, with a descriptive user agent, and does not loop. Do not download the billing zip in more than one lane. Do not request a path that `robots.txt` disallows.

## Add the file GET and the source interface (tax-1)

**Depends on.** None.

**Files.**

- [ ] Create `internal/taxsource/get.go`.
- [ ] Create `internal/taxsource/get_test.go`.
- [ ] Create `internal/taxsource/source.go`.

**Build.**

- [ ] Add `Get(ctx context.Context, rawURL string, limit int64, timeout time.Duration) (io.ReadCloser, error)`. A zero limit or a zero timeout is an error. The body is capped at `limit`. One byte past the cap returns an error and closes the body.
- [ ] Retry once when the first attempt fails before any body byte is read, and the error is a transport error or an HTTP status from 500 through 599. Do not retry 404. Do not retry when the error matches `context.Canceled` or `context.DeadlineExceeded`. Do not read `Retry-After`. Do not add a rate limiter. Do not fetch `robots.txt`.
- [ ] Add `Meta` with `URL`, `Name`, `SHA256`, and `FetchedAt`. Add `Source[T any]` with `Read(ctx context.Context, yield func(T) error) (Meta, error)`. `Read` is the only method. The shared package does not name a tax column and does not import `internal/county`, `internal/soil`, `internal/connector/union`, or `internal/parcelstore`.
- [ ] Do not add `DoRetry`, a worker pool, or a per-parcel `Scrape` method. A second county implements `Source` in its own package. Union is the first implementation, and it lands in tax-3, tax-4, tax-5, and tax-7.

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
- [ ] Lane 7. Return 404 on the only attempt. Save `/tmp/swarm-tax-1/worker-7/no-retry-404.png`. Pass when the handler ran once.
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
- [ ] The owner does not merge. The operator lands tax-1. It does not wait on persist-1.

## Add the tax tables (tax-2)

**Depends on.** persist-1. That PR adds `parcels.likely_vacant`. This PR reads the column and does not create it.

**Files.**

- [ ] Create one new file under `db/migrations`. List `db/migrations/*.sql`, take the highest six-digit prefix, add 1, and zero-pad to six digits. Name it `db/migrations/<prefix>_tax_files.sql`. On main at `e5ac202` the files are `000001` through `000004`, so the highest prefix is `000004`. Do not assume `000005` is free. persist-1 adds `<prefix>_parcels_likely_vacant.sql`. soil-3 adds `<prefix>_soil.sql`. If either file exists, use the next integer after every six-digit file in the directory. Do not edit `000001_init_schema.sql`, `000002_parcels_storage_srid.sql`, `000003_parcels_county_check.sql`, `000004_parcel_sales_uuid_id.sql`, or a soil migration. `make migrate-create` runs `goose create` with no sequential flag, at `Makefile` lines 82 to 88, so the name it emits is a timestamp. Create this file by hand.

**Build.**

- [ ] Add `tax_delinquent_balances`. `id UUID PRIMARY KEY DEFAULT gen_random_uuid()`. `county_id TEXT NOT NULL` with `CHECK (county_id IN ('union'))`, the same check as `db/migrations/000003_parcels_county_check.sql`. `source_name TEXT NOT NULL`. `parcel_id UUID NOT NULL REFERENCES parcels(id) ON DELETE CASCADE`. `tax_year SMALLINT NOT NULL` with `CHECK (tax_year BETWEEN 1900 AND 2200)`, the same year check as `parcels.parcel_year`. `balance_due NUMERIC(14, 2) NOT NULL` with `CHECK (balance_due >= 0)`, the same money scale as `parcel_sales.amount`. `primary_source TEXT NOT NULL` with `CHECK (primary_source = 'RE')`. `source_url TEXT NOT NULL`. `file_sha256 BYTEA NOT NULL`. `fetched_at TIMESTAMPTZ NOT NULL`. `created_at TIMESTAMPTZ NOT NULL DEFAULT now()`. `updated_at TIMESTAMPTZ NOT NULL DEFAULT now()`. Unique `(county_id, source_name, parcel_id, tax_year)`. Index `(parcel_id, tax_year)`. Do not add `source_parcel_id`. Do not add a `JSONB` column. The stored `source_name` is `union_delinquent`. That name is this file. Parcel lookup still uses `union_county_gis`.
- [ ] Add `tax_bills` with the same `id`, `county_id`, `source_name`, `parcel_id`, `tax_year`, `source_url`, `file_sha256`, `fetched_at`, `created_at`, and `updated_at` rules. `total_taxes NUMERIC(14, 2) NOT NULL` with `CHECK (total_taxes >= 0)`. `property_address TEXT`, `taxpayer_name TEXT`, `mailing_line TEXT`, `mailing_city TEXT`, `mailing_state TEXT`, and `mailing_postal_code TEXT` are nullable. An empty cell is NULL. Do not split one address line into city and postal code. Unique `(county_id, source_name, parcel_id, tax_year)`. Index `(parcel_id, tax_year)`. The stored `source_name` is `union_billing`. No `source_parcel_id`. No `JSONB`.
- [ ] Add `tax_foreclosure_cases` with the same `id`, `county_id`, `source_name`, `source_url`, `file_sha256`, `fetched_at`, `created_at`, and `updated_at` rules. `case_number TEXT` is NULL when the source says `TBD` or the label is absent. Do not store the text `TBD`. `parcel_set_sha256 BYTEA NOT NULL` is the SHA-256 of the sorted normalized PINs joined by newlines. `legal_sha256 BYTEA NOT NULL` is the SHA-256 of the legal description texts in that same PIN order, joined by newlines. `owner_name TEXT`. `sale_on DATE`. `tax_year SMALLINT` uses the same year check and is NULL when the source names no year. `opening_bid NUMERIC(14, 2)`, `current_bid NUMERIC(14, 2)`, and `upset_bid NUMERIC(14, 2)` are each NULL or `>= 0`. `status TEXT`. `redeemed BOOLEAN NOT NULL DEFAULT false`. `bidding_closed BOOLEAN NOT NULL DEFAULT false`. The parser sets those booleans from the status phrase. A query reads the booleans and does not parse the phrase. The stored `source_name` is `union_foreclosure`. No `JSONB`.
- [ ] The case upsert key is a decision for the operator to confirm. A real case number conflicts on `(county_id, source_name, case_number)` where `case_number` is not null. A null case number conflicts on `(county_id, source_name, parcel_set_sha256, legal_sha256)` where `case_number` is null. Use two partial unique indexes. Two `TBD` cases with different PIN sets stay two rows. Two `TBD` cases with the same PIN set and different legal text stay two rows. A second save of the same `TBD` PIN set and the same legal text keeps `id`. A numbered case keeps `id` when the PIN list changes, and the hashes on that row update. This plan does not delete the old `TBD` row if a later file gives that PIN set a case number.
- [ ] Add `tax_foreclosure_parcels`. `id UUID PRIMARY KEY DEFAULT gen_random_uuid()`. `case_id UUID NOT NULL REFERENCES tax_foreclosure_cases(id) ON DELETE CASCADE`. `parcel_id UUID NOT NULL REFERENCES parcels(id) ON DELETE CASCADE`. `tax_value NUMERIC(14, 2)` is NULL or `>= 0`. `situs_line TEXT`. `legal_description TEXT`. `created_at` and `updated_at` match `parcel_sales`. Unique `(case_id, parcel_id)`. Do not put `county_id` or `source_name` here. `parcel_sales` does not copy those columns either. Do not put `source_parcel_id` here. One case that names three PINs is one case row and three join rows. Do not store those PINs in one text column.
- [ ] Add `tax_foreclosure_pipeline`. Same `id`, `county_id`, `source_name`, `source_url`, `file_sha256`, `fetched_at`, `created_at`, and `updated_at` rules. `parcel_id UUID NOT NULL REFERENCES parcels(id) ON DELETE CASCADE`. `owner_name TEXT`. `situs_line TEXT`. `township TEXT`. `acreage TEXT`. `tax_value NUMERIC(14, 2)` is NULL or `>= 0`. `case_number TEXT`. `sale_on DATE`. `schedule_text TEXT`. `status TEXT`. `redeemed BOOLEAN NOT NULL DEFAULT false`. Unique `(county_id, source_name, parcel_id)`. Index `(parcel_id)`. The stored `source_name` is `union_foreclosure_pipeline`. No `tax_year`. The measured file does not name one. No `source_parcel_id`. No `JSONB`. Do not add a boolean column on `parcels`. A parcel is upcoming when this table has its `parcels.id`.
- [ ] Add view `tax_owed`. Read `tax_delinquent_balances` and `parcels` only. Join on `parcels.id = tax_delinquent_balances.parcel_id`. Keep rows where `parcels.likely_vacant` is true and `balance_due > 0`. Return `parcel_id`, `tax_year`, and the summed `balance_due`, grouped by `parcel_id` and `tax_year`. Do not collapse years. A bill joins this view on `parcel_id` and `tax_year`. A foreclosure join row and a pipeline row join on `parcel_id`. Those joins do not parse a string. A foreclosure row does not set the owed flag by itself. A pipeline row does not set it either. Do not add `parcels.likely_vacant`. Do not add `GEOMETRY`. Do not add `JSONB`.
- [ ] Money is `NUMERIC(14, 2)`, matching `parcel_sales.amount` and the parcel valuation columns in `000001_init_schema.sql`. The largest amount in the measured delinquent file is 36800.63 on a personal-property row this program does not store. The largest real-estate balance is 14355.02. Do not store money as `float`. Do not use `INTEGER` for `tax_year`.
- [ ] The upsert in later PRs omits `id` and does not assign `created_at`. On conflict it updates the typed columns and sets `updated_at` to `now()`, the same way `SaveParcel` and `SaveSales` do. The pipeline save also deletes rows for `(county_id, source_name)` whose `parcel_id` is absent from that file, in the same transaction, so the join is the current list. Goose `Up` creates the five tables and the view. Goose `Down` drops the view, then `tax_foreclosure_pipeline`, then `tax_foreclosure_parcels`, then `tax_foreclosure_cases`, then `tax_bills`, then `tax_delinquent_balances`. It does not drop `parcels`.

**You see.**

- [ ] After `make migrate-up` on a database that already has persist-1, `psql` lists `tax_delinquent_balances`, `tax_bills`, `tax_foreclosure_cases`, `tax_foreclosure_parcels`, `tax_foreclosure_pipeline`, and `tax_owed`. `\d tax_delinquent_balances` shows a UUID primary key, `county_id`, `source_name`, `tax_year` as smallint, and no `source_parcel_id` column. `\d parcels` does not show an upcoming-foreclosure column.

**Verify, unit.** Tests alone are not sufficient verification. A PR is verified only when its unit, live, and perf boxes are all checked.

- [ ] `rg -n "likely_vacant_parcels|source_parcel_id|GEOMETRY|JSONB|jsonb|upcoming_foreclosure" db/migrations/<prefix>_tax_files.sql` prints nothing. `rg -n "likely_vacant" db/migrations/<prefix>_tax_files.sql` matches only the view. `rg -n "tax_owed" db/migrations/<prefix>_tax_files.sql` matches the view. `rg -n "SMALLINT" db/migrations/<prefix>_tax_files.sql` matches `tax_year`. `rg -n "tax_foreclosure_parcels" db/migrations/<prefix>_tax_files.sql` matches the join table. `rg -n "tax_foreclosure_pipeline" db/migrations/<prefix>_tax_files.sql` matches the pipeline table. `rg -n "ADD COLUMN likely_vacant" db/migrations/<prefix>_tax_files.sql` prints nothing.
- [ ] Run `go test -race ./... -count=1` with no database. It still passes. `internal/parcelstore` database tests carry `//go:build postgres`, so this command does not open Postgres. Run `go vet ./...` and `staticcheck ./...`. Both print no findings.

**Verify, live.** Tests alone are not sufficient verification. A PR is verified only when its unit, live, and perf boxes are all checked. Ten lanes on `grok-4.7-xhigh-fast` at the PR head, per the boot recipe.

- [ ] Lane 1. Regression lane against trunk. At trunk and at head, run `psql` with `\dt tax_delinquent_balances`. Save `/tmp/swarm-tax-2/worker-1/regression.png`. Pass when trunk has no such table and head lists it. Trunk may already have `parcels.likely_vacant` if persist-1 has landed. Record that. This PR did not add the column.
- [ ] Lane 2. Insert one parcel with `likely_vacant` true and omit `id`. Insert one delinquent row for that `parcels.id`, year 2025, balance 10.00, and omit the tax `id`. Save `/tmp/swarm-tax-2/worker-2/insert-delinquent.png`. Pass when the select returns that balance and both ids are UUIDs.
- [ ] Lane 3. Upsert the same `(county_id, source_name, parcel_id, tax_year)` with balance 12.50 and omit `id` again. Save `/tmp/swarm-tax-2/worker-3/upsert-delinquent.png`. Pass when the count is 1, the balance is 12.50, and `id` is unchanged.
- [ ] Lane 4. Insert a delinquent row whose `parcel_id` is a UUID that is not in `parcels`. Save `/tmp/swarm-tax-2/worker-4/missing-parcel.png`. Pass when the foreign key rejects the row.
- [ ] Lane 5. Insert a negative balance. Save `/tmp/swarm-tax-2/worker-5/negative.png`. Pass when the check rejects the row. Insert `primary_source` `PP`. Pass when that check rejects the row too.
- [ ] Lane 6. Insert one bill and one balance for the same parcel and year. Then insert one foreclosure case with `case_number` null and two join rows for two likely-vacant parcels. Omit every `id`. Save `/tmp/swarm-tax-2/worker-6/insert-bill.png`. Pass when the bill joins the balance on `parcel_id` and `tax_year` as one row, a second insert of the same PIN set and the same legal text keeps the case `id`, and two join rows exist.
- [ ] Lane 7. With the likely-vacant parcel and balance 10.00, select `tax_owed`. Save `/tmp/swarm-tax-2/worker-7/owed-join.png`. Pass when the view shows that `parcels.id` and the summed balance.
- [ ] Lane 8. Insert a delinquent row for a parcel whose `likely_vacant` is false. Save `/tmp/swarm-tax-2/worker-8/owed-empty.png`. Pass when the view returns no row for that parcel and the balance row is still in `tax_delinquent_balances`.
- [ ] Lane 9. Run goose down, then up. Save `/tmp/swarm-tax-2/worker-9/down-up.png`. Pass when the five tables are gone after down, `parcels` remains, and the tables are present after up.
- [ ] Lane 10. Count `parcels` before and after a delinquent insert. Show the column list. Save `/tmp/swarm-tax-2/worker-10/no-parcel-insert.png`. Pass when the `parcels` count is unchanged, `tax_delinquent_balances` has `county_id` and `source_name` and no `source_parcel_id`, `tax_foreclosure_parcels` has `case_id` and `parcel_id` and no `source_parcel_id`, and `tax_foreclosure_pipeline` has `parcel_id` and `source_name` and no boolean on `parcels`.

**Verify, perf.** Tests alone are not sufficient verification. A PR is verified only when its unit, live, and perf boxes are all checked.

- [ ] Metric. Wall time of one upsert into `tax_delinquent_balances`, and the row count after a second upsert of the same key. Trunk has no table. The end state is one row with the same `id`.
- [ ] Probe. At trunk, record that `\dt tax_delinquent_balances` fails. At head, time one upsert, then a second upsert, with `psql`.
- [ ] Baseline. Record trunk as no table and no elapsed time.
- [ ] Rule. Fail if the head upsert takes 5 seconds or longer, or if the second upsert leaves a count other than 1, or if `id` changes. Do not report a ratio against trunk.

**Review gate.** None. tax-2 is not review-gated.

**Merge.**

- [ ] Root's clean verdict at the exact head SHA.
- [ ] Bugbot triage done.
- [ ] Rebased onto current trunk after the verdict, with `git patch-id` unchanged.
- [ ] The owner does not merge. The operator lands tax-2 after persist-1. It does not wait on tax-1.

## Ingest the delinquent tax notice (tax-3)

**Depends on.** tax-1 and tax-2. `county.ID` is already in `internal/county` on trunk.

**Files.**

- [ ] Create `internal/connector/union/delinquent.go`.
- [ ] Create `internal/connector/union/delinquent_test.go`.
- [ ] Create `internal/connector/union/testdata/tax/delinquent.csv`.
- [ ] Create `internal/taxstore/delinquent.go`.
- [ ] Create `internal/taxstore/delinquent_test.go`.

**Build.**

- [ ] Add `DelinquentRow` with `ParcelKey`, `Year`, `Balance`, and `PrimarySource`. The county parcel key stays on this struct until the store looks up `parcels.id`. It is not a database id. Add `DelinquentList` with `Read` so that `var _ taxsource.Source[DelinquentRow] = (*DelinquentList)(nil)` compiles in `delinquent.go`.
- [ ] Resolve the file with `GET` of `https://www.unioncountync.gov/home/showdocument?id=4792`. Do not send the `t` query parameter. Appendix A measured that this URL returns the CSV. If the first record's names do not include `Property Key`, `Year`, `Primary Source`, and `Balance Due`, return an error and yield nothing. Do not guess another document id.
- [ ] Parse with `encoding/csv`. Read columns by those names. Ignore extra empty columns. Trim the key and uppercase it. Do not remove an internal space and do not drop a suffix. `07105038A` stays `07105038A`. `02176011 60` stays `02176011 60`.
- [ ] Yield a row only when `Primary Source` is `RE`. Drop `RR`. Drop `PP`. Personal property is business equipment and vehicles taxed apart from the land. Parse `Balance Due` by removing `$` and commas. `Year` is a smallint. Store dollars and cents. A balance that does not parse is an error and yields nothing. Delete `-` and `.` from the key before lookup, then trim the ends. Do not delete spaces. A key that contains neither character is unchanged.
- [ ] `SaveDelinquent` looks up `parcels.id` with `(county_id, source_name, source_parcel_id)`. `county_id` is `county.Union`. `source_name` for the lookup is `union_county_gis`, the value `GISSource.Name` returns at `internal/connector/union/gis.go` line 82. `source_parcel_id` is the normalized key. Use the same three columns as `lookupParcelSQL` in `internal/parcelstore/sales.go` lines 91 to 97. Do not edit `internal/parcelstore`. Also read `likely_vacant`. No parcel row is an unmatched skip. A parcel with `likely_vacant` false is a not-enriched skip. A true row is upserted. The stored `county_id` is the parcel's `county_id`. The stored `source_name` is `union_delinquent`. Do not insert a parcel. Do not write the county key onto the tax row.
- [ ] The upsert omits `id` and `created_at` and conflicts on `(county_id, source_name, parcel_id, tax_year)`. It replaces `balance_due`, `primary_source`, `source_url`, `file_sha256`, and `fetched_at`, and sets `updated_at` to `now()`. It does not assign `id`. `file_sha256` is the SHA-256 of the file bytes. `SaveDelinquent` returns the saved count, the skipped PP count, the skipped RR count, the unmatched count, and the not-enriched count. Add `Latest(ctx) (time.Time, bool, error)` as the max `fetched_at`, with false when the table is empty. A second `Read` of the same bytes yields the same keys and balances.
- [ ] Put the database test behind `//go:build postgres`, matching `internal/parcelstore/parcels_test.go`. `go test` without that tag does not open Postgres.
- [ ] The golden file is a few RE rows, one PP row, and one RR row copied from the measured file, including one 8-digit key, one 9-character key, and one 11-character key. `-update` rewrites it and is off by default.

**You see.**

- [ ] `go test -race ./internal/connector/union/ -count=1 -run TestDelinquent` prints `PASS` and the golden RE balance matches. `go test -race -tags postgres ./internal/taxstore/ -count=1 -run TestSaveDelinquent` prints `PASS` when `DATABASE_URL` points at a migrated database.

**Verify, unit.** Tests alone are not sufficient verification. A PR is verified only when its unit, live, and perf boxes are all checked.

- [ ] Table-driven tests cover the golden RE rows, a skipped PP row, a dropped RR row, a missing `Property Key` header, a bad balance, and a canceled context that yields nothing. Run `go test -race ./internal/connector/union/ -count=1 -run TestDelinquent`.
- [ ] The postgres test covers a first save, a second save that keeps `id`, an unmatched key, and a parcel with `likely_vacant` false that writes nothing. Run `go test -race -tags postgres ./internal/taxstore/ -count=1 -run TestSaveDelinquent`.
- [ ] Run `go test -race ./internal/connector/union/ -fuzz=FuzzDelinquentMoney -fuzztime=10s` and `go test -race ./internal/connector/union/ -fuzz=FuzzParcelKey -fuzztime=10s`. Both exit 0.
- [ ] Run `go vet ./internal/connector/union/ ./internal/taxstore/` and `staticcheck ./internal/connector/union/ ./internal/taxstore/`. Both print no findings.
- [ ] Run `go test -race -cover ./internal/connector/union/ ./internal/taxstore/ -count=1`. Fail if `union` coverage is below 77.4 percent. That is the trunk measurement in Appendix A. If trunk coverage at the start of this PR is higher, fail below that higher number and record it. Fail if `taxstore` coverage, measured with `-tags postgres`, is below 90.0 percent. That floor is a chosen gate for a new package.

**Verify, live.** Tests alone are not sufficient verification. A PR is verified only when its unit, live, and perf boxes are all checked. Ten lanes on `grok-4.7-xhigh-fast` at the PR head, per the boot recipe.

- [ ] Lane 1. Regression lane against trunk. Run `go test -race ./internal/connector/union/ -count=1` at trunk and at head. Trunk has no `DelinquentList`. Record that, and gate head's new tests plus a green union package. Save `/tmp/swarm-tax-3/worker-1/regression.png`. Pass when trunk's union tests pass and head's `TestDelinquent` passes.
- [ ] Lane 2. Parse the golden file. Save `/tmp/swarm-tax-3/worker-2/golden.png`. Pass when the 8-digit key, the letter-suffix key, and the 11-character key are present and the PP and RR rows are absent.
- [ ] Lane 3. `SaveDelinquent` one golden row for a likely-vacant parcel, then save it again with the same balance. Save `/tmp/swarm-tax-3/worker-3/upsert.png`. Pass when the table has one row, `id` is unchanged, `parcel_id` equals that `parcels.id`, and `file_sha256` is 32 bytes.
- [ ] Lane 4. Change the balance on the second save. Save `/tmp/swarm-tax-3/worker-4/replace.png`. Pass when the count is still 1, `id` is unchanged, and the balance is the second value.
- [ ] Lane 5. Save a key that is not in `parcels`, and save a key whose parcel has `likely_vacant` false. Count `parcels` around the save. Save `/tmp/swarm-tax-3/worker-5/no-parcel.png`. Pass when the `parcels` count is unchanged, the tax table gains no row, and the two skip counts are 1 and 1.
- [ ] Lane 6. Fetch `https://www.unioncountync.gov/home/showdocument?id=4792` once. Save `/tmp/swarm-tax-3/worker-6/live-csv.png`. Pass when the first line contains `Property Key` and `Balance Due`. One request. Do not pass a `t` parameter.
- [ ] Lane 7. From that same saved body, count RE, PP, and RR. Save `/tmp/swarm-tax-3/worker-7/live-counts.png`. Pass when RE is 926, PP is 5013, and RR is 3, or record the new counts and stop if the header names changed.
- [ ] Lane 8. Sum RE `Balance Due` from that body. Save `/tmp/swarm-tax-3/worker-8/live-sum.png`. Pass when the sum is 232755.26, or record the new sum next to the header check from lane 7.
- [ ] Lane 9. Cancel `Read` before the request. Save `/tmp/swarm-tax-3/worker-9/cancel.png`. Pass when the error matches `context.Canceled` and no row is yielded.
- [ ] Lane 10. Run `tax_owed` after saving one RE row for a likely-vacant parcel. Save `/tmp/swarm-tax-3/worker-10/owed.png`. Pass when the view returns that `parcels.id`. A key with no likely-vacant parcel stays out of the view. The skipped PP count for the golden file is 1.

**Verify, perf.** Tests alone are not sufficient verification. A PR is verified only when its unit, live, and perf boxes are all checked.

- [ ] Metric. Wall time to parse the measured 5,942-row delinquent file from disk, and the RE yield count. Trunk has no parser. The end state is 926 RE rows.
- [ ] Probe. At trunk, record that `DelinquentList` does not exist. At head, time one parse of a local copy of the file. Do not include the county download in the timer.
- [ ] Baseline. Record trunk as no parser and no elapsed time.
- [ ] Rule. Fail if the head parse takes 1 second or longer, or if the RE yield count is not 926 for that measured file. A newer county file with a different count fails this box until Appendix A is updated with the new count. Do not report a ratio against trunk.

**Review gate.** None. tax-3 is not review-gated.

**Merge.**

- [ ] Root's clean verdict at the exact head SHA.
- [ ] Bugbot triage done.
- [ ] Rebased onto current trunk after the verdict, with `git patch-id` unchanged.
- [ ] The owner does not merge. The operator lands tax-3 after tax-1 and tax-2.

## Ingest the billing file (tax-4)

**Depends on.** tax-1 and tax-2. `county.ID` is already on trunk.

**Files.**

- [ ] Create `internal/connector/union/billing.go`.
- [ ] Create `internal/connector/union/billing_test.go`.
- [ ] Create `internal/connector/union/testdata/tax/billing.csv`.
- [ ] Create `internal/taxstore/bills.go`.
- [ ] Create `internal/taxstore/bills_test.go`.

**Build.**

- [ ] Add `BillRow` with `ParcelKey`, `Year`, `TotalTaxes`, `PropertyAddress`, and the mailing fields on `tax_bills`. Add `BillingFile` with `Read` so that `var _ taxsource.Source[BillRow] = (*BillingFile)(nil)` compiles. The parcel key is not a database id.
- [ ] Resolve the URL from the Parcels table on `https://www.unioncountync.gov/government/departments-f-p/gis-mapping/downloadable-gis-data`. Use the link whose label is `Billing Data`. Do not hardcode the numeric timestamp in the path. If that label is missing, return an error.
- [ ] `Get` the zip with a limit of 256 MiB. That cap is chosen. Appendix A says this session did not obtain a clean zip length, and the operator measured an uncompressed CSV of about 124 MB on 2026-11-08. Write the body to a temp file. Open it with `archive/zip`. A zip directory is at the end of the file, so do not parse the CSV from the socket. Stream the entry whose name contains `WKBILA` and ends in `.csv` with `encoding/csv`. Fail if the uncompressed entry exceeds 256 MiB. Delete the temp file when `Read` returns.
- [ ] Require `property_key`, `source`, `year`, `TOTAL TAXES AND CHARGES`, `PROPERTY ADDRESS`, `Taxpayer Name 1`, and `Taypayer Address 1`. The address header is spelled `Taypayer` in the operator's column list. Match that spelling. A missing required name fails the parse and yields nothing. Yield `source` `RE` only. Drop `PP` for the same reason as tax-3. `SaveBill` returns the saved count, the skipped PP count, the unmatched count, and the not-enriched count. There is no paid column and no balance column. Do not invent one. `TotalTaxes` is the billed amount, not the amount owed.
- [ ] Use the same key rule and the same lookup as tax-3. Lookup `source_name` is `union_county_gis`. The stored `source_name` is `union_billing`. The stored `county_id` is the parcel's `county_id`. `SaveBill` upserts on `(county_id, source_name, parcel_id, tax_year)`, omits `id`, and sets `updated_at` to `now()`. Add `Latest` for `tax_bills`. The golden file is a few RE rows and one PP row. Include parcel `09336350` when a row is copied from a real file. `-update` is off by default. Database tests use `//go:build postgres`. Do not write a JSON document.
- [ ] Do not download the zip in `go test` unless the test points at a local file. The live sample is one lane.

**You see.**

- [ ] `go test -race ./internal/connector/union/ ./internal/taxstore/ -count=1 -run 'TestBilling|TestSaveBill'` prints `PASS` without contacting the county host. The save test is the postgres-tagged one.

**Verify, unit.** Tests alone are not sufficient verification. A PR is verified only when its unit, live, and perf boxes are all checked.

- [ ] Table-driven tests cover the golden RE row, a skipped PP row, a missing `property_key` header, a zip whose entry exceeds the cap, and a canceled context. Run `go test -race ./internal/connector/union/ -count=1 -run TestBilling`.
- [ ] Run `go test -race ./internal/connector/union/ -fuzz=FuzzBillingMoney -fuzztime=10s`. It exits 0.
- [ ] Run `go vet ./internal/connector/union/ ./internal/taxstore/` and `staticcheck ./internal/connector/union/ ./internal/taxstore/`. Both print no findings.
- [ ] Run `go test -race -cover ./internal/connector/union/ ./internal/taxstore/ -count=1`. Fail if `union` coverage is below 77.4 percent, or below a higher trunk number measured at the start of this PR. Fail if `taxstore` coverage with `-tags postgres` is below 90.0 percent.

**Verify, live.** Tests alone are not sufficient verification. A PR is verified only when its unit, live, and perf boxes are all checked. Ten lanes on `grok-4.7-xhigh-fast` at the PR head, per the boot recipe.

- [ ] Lane 1. Regression lane against trunk. Run `go test -race ./internal/connector/union/ -count=1 -run TestDelinquent` at trunk and at head. Trunk has no billing parser. Record that. Save `/tmp/swarm-tax-4/worker-1/regression.png`. Pass when head's delinquent tests still pass and `TestBilling` passes.
- [ ] Lane 2. Parse the golden CSV. Save `/tmp/swarm-tax-4/worker-2/golden.png`. Pass when parcel `09336350` yields a total and a mailing line, and the PP row is absent.
- [ ] Lane 3. `SaveBill` twice for the same likely-vacant parcel and year. Save `/tmp/swarm-tax-4/worker-3/upsert.png`. Pass when the count is 1 and `id` is unchanged.
- [ ] Lane 4. Count `parcels` around a save of an unmatched key. Save `/tmp/swarm-tax-4/worker-4/no-parcel.png`. Pass when the count is unchanged and no bill row is inserted.
- [ ] Lane 5. Open a local zip with one CSV entry and stream it. Save `/tmp/swarm-tax-4/worker-5/zip-stream.png`. Pass when the row count matches the entry and the process does not hold the entry as one string. The test compares yielded rows with a second streaming pass.
- [ ] Lane 6. Fetch the GIS download page once and print the `Billing Data` href. Save `/tmp/swarm-tax-4/worker-6/billing-href.png`. Pass when the href contains `showpublisheddocument` and does not point at `unionnc.devnetwedge.com`.
- [ ] Lane 7. Download that href once, only in this lane, and keep the raw bytes. Save `/tmp/swarm-tax-4/worker-7/billing-zip.png`. Pass when a zip reader opens the body and an entry name contains `WKBILA` and `.csv`. Record the byte count. Do not repeat this download in another lane. A fetch that replaces invalid bytes with the UTF-8 replacement character is not this pass.
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
- [ ] The owner does not merge. The operator lands tax-4 after tax-1 and tax-2.

## Ingest the foreclosure list (tax-5)

**Depends on.** tax-1 and tax-2. `county.ID` is already on trunk.

**Files.**

- [ ] Create `internal/connector/union/foreclosure.go`.
- [ ] Create `internal/connector/union/foreclosure_test.go`.
- [ ] Create `internal/connector/union/testdata/tax/foreclosure.html`.
- [ ] Create `internal/taxstore/foreclosure.go`.
- [ ] Create `internal/taxstore/foreclosure_test.go`.

**Build.**

- [ ] Add `ForeclosureCase` with `CaseNumber`, `OwnerName`, `SaleOn`, `Status`, the bid amounts, and a slice of PIN values. Each PIN value has the normalized key, situs line, legal description, and tax value. Add `ForeclosureList` with `Read` so that `var _ taxsource.Source[ForeclosureCase] = (*ForeclosureList)(nil)` compiles. One yield is one case. It is not one yield per PIN.
- [ ] Resolve the file with `GET` of `https://www.unioncountync.gov/home/showdocument?id=4787`. Do not send the `t` query parameter. If the body contains zero `Parcel Identification Number` labels, return an error and yield nothing. Do not guess another document id. Do not fetch the upcoming-foreclosure document. tax-7 does that.
- [ ] A block is the text from one case to the next. Delete `-` and `.` from each PIN and then trim the ends. Do not delete spaces. `09-125-043` becomes `09125043`. A block with no PIN is skipped and counted. It is not a personal-property row. This file has no source column. Do not drop a PIN because the page mentions personal property in the statute text.
- [ ] One case can name more than one PIN. Appendix A measured case `26M000471-890` with three PINs. Yield one `ForeclosureCase` whose slice has one entry per PIN. Do not join the PINs into one string. `TBD` for the case number, a date, or an amount becomes NULL. Do not store the text `TBD`. A non-empty amount that is not `TBD` and does not parse as dollars and cents fails the file and yields nothing. Set `redeemed` and `bidding_closed` from the status phrase.
- [ ] `SaveForeclosure` groups one case, writes the case row, and looks up each PIN with the tax-3 lookup. Do not read `likely_vacant`. A stored parcel gets one join row whether or not it has a house. A PIN with no parcel keeps the case row, skips the join row, and increments the unmatched count. Do not insert a parcel. The case upsert uses the partial unique indexes from tax-2 and omits `id`. The join upsert conflicts on `(case_id, parcel_id)` and omits `id`. The save result returns the saved case count, the saved join count, the skipped-block count, and the unmatched count. It does not return a not-enriched count. Add `Latest` for `tax_foreclosure_cases`. Database tests use `//go:build postgres`.
- [ ] The golden file is a short copy of the measured list. Include case `26M000471-890` with three PINs, one `TBD` case with a different PIN, a second `TBD` case with another PIN, and one block with no PIN. `-update` is off by default.

**You see.**

- [ ] `go test -race ./internal/connector/union/ -count=1 -run TestForeclosure` prints `PASS`, and `09-125-043` is one PIN on a case, not a stored id.

**Verify, unit.** Tests alone are not sufficient verification. A PR is verified only when its unit, live, and perf boxes are all checked.

- [ ] Table-driven tests cover the golden PINs, a three-PIN case as one yield, a skipped block with no PIN, a missing PIN label on the whole file, a bad amount, a `TBD` bid stored as NULL, a `TBD` case number stored as NULL, and a canceled context. Run `go test -race ./internal/connector/union/ -count=1 -run TestForeclosure`.
- [ ] The postgres test saves one numbered case twice and keeps the case `id`. It saves two `TBD` cases with different PIN sets and keeps two case rows. It saves one unmatched PIN, keeps that case, and inserts no join row. Run `go test -race -tags postgres ./internal/taxstore/ -count=1 -run TestSaveForeclosure`.
- [ ] Run `go vet ./internal/connector/union/ ./internal/taxstore/` and `staticcheck ./internal/connector/union/ ./internal/taxstore/`. Both print no findings.
- [ ] Run `go test -race -cover ./internal/connector/union/ ./internal/taxstore/ -count=1`. Fail if `union` coverage is below 77.4 percent, or below a higher trunk number measured at the start of this PR. Fail if `taxstore` coverage with `-tags postgres` is below 90.0 percent.

**Verify, live.** Tests alone are not sufficient verification. A PR is verified only when its unit, live, and perf boxes are all checked. Ten lanes on `grok-4.7-xhigh-fast` at the PR head, per the boot recipe.

- [ ] Lane 1. Regression lane against trunk. Run `go test -race ./internal/connector/union/ -count=1 -run 'TestDelinquent|TestBilling'` at trunk and at head. Trunk has no foreclosure parser. Record that. Save `/tmp/swarm-tax-5/worker-1/regression.png`. Pass when head's earlier parser tests still pass and `TestForeclosure` passes.
- [ ] Lane 2. Parse the golden file. Save `/tmp/swarm-tax-5/worker-2/golden.png`. Pass when case `26M000471-890` is one yield with three PINs, `09-125-043` normalizes to `09125043`, and the block with no PIN is absent.
- [ ] Lane 3. `SaveForeclosure` twice for the same `TBD` case with two stored parcels, one of them `likely_vacant` false. Save `/tmp/swarm-tax-5/worker-3/upsert.png`. Pass when one case row exists, `case_number` is NULL, two join rows exist, and the case `id` is unchanged.
- [ ] Lane 4. Save two `TBD` cases with different PIN sets, one of them missing from `parcels`. Count `parcels`. Save `/tmp/swarm-tax-5/worker-4/no-parcel.png`. Pass when two case rows exist, the missing PIN has no join row, the unmatched count is 1, and the `parcels` count is unchanged.
- [ ] Lane 5. Parse a PIN that still contains a space after `-` is removed, using fixture text `02176011 60` with the hyphens the normalizer would see. Save `/tmp/swarm-tax-5/worker-5/keep-space.png`. Pass when the yielded key still contains the space.
- [ ] Lane 6. Fetch `https://www.unioncountync.gov/home/showdocument?id=4787` once. Save `/tmp/swarm-tax-5/worker-6/live-list.png`. Pass when the body contains `Parcel Identification Number` and does not point the parser at `unionnc.devnetwedge.com`. One request. Do not pass a `t` parameter.
- [ ] Lane 7. From that same body, list the PIN strings and the keys after deleting `-`. Save `/tmp/swarm-tax-5/worker-7/live-pins.png`. Pass when the count is recorded. Appendix A counted 9 PIN tokens in the 2026-11-10 markdown conversion, and 8 of those keys were in the delinquent RE set. A difference does not fail the lane. Write the new list in the lane report.
- [ ] Lane 8. Confirm key `09125043` from PIN `09-125-043` is in that list, or record that the PIN is absent. Save `/tmp/swarm-tax-5/worker-8/known-pin.png`. Pass when the report says present or absent. Do not fail only because the county removed that sale.
- [ ] Lane 9. Cancel `Read` before the request. Save `/tmp/swarm-tax-5/worker-9/cancel.png`. Pass when the error matches `context.Canceled` and no row is yielded.
- [ ] Lane 10. Select `tax_owed` after a foreclosure save and no delinquent row. Save `/tmp/swarm-tax-5/worker-10/owed-unchanged.png`. Pass when the view does not gain a row from the foreclosure save alone.

**Verify, perf.** Tests alone are not sufficient verification. A PR is verified only when its unit, live, and perf boxes are all checked.

- [ ] Metric. Wall time to parse the golden foreclosure file, and the yielded case count. Trunk has no parser. The end state is the golden case count.
- [ ] Probe. At trunk, record that `ForeclosureList` does not exist. At head, time 20 parses of the golden file.
- [ ] Baseline. Record trunk as no parser and no elapsed time.
- [ ] Rule. Fail if the average of 20 head parses is 1 second or longer, or if the yielded case count is not the golden case count. Do not report a ratio against trunk.

**Review gate.** None. tax-5 is not review-gated.

**Merge.**

- [ ] Root's clean verdict at the exact head SHA.
- [ ] Bugbot triage done.
- [ ] Rebased onto current trunk after the verdict, with `git patch-id` unchanged.
- [ ] The owner does not merge. The operator lands tax-5 after tax-1 and tax-2.

## Ingest the upcoming foreclosure list (tax-7)

**Depends on.** tax-1 and tax-2. `county.ID` is already on trunk.

**Files.**

- [ ] Create `internal/connector/union/pipeline.go`.
- [ ] Create `internal/connector/union/pipeline_test.go`.
- [ ] Create `internal/connector/union/testdata/tax/pipeline.pdf`.
- [ ] Create `internal/connector/union/testdata/tax/pipeline.pptx`.
- [ ] Create `internal/taxstore/pipeline.go`.
- [ ] Create `internal/taxstore/pipeline_test.go`.

**Build.**

- [ ] Add `PipelineProperty` with the normalized parcel key, owner, situs line, township, acreage text, tax value, case number, sale date, schedule text, status, and redeemed flag. Add `PipelineList` with `Read` so that `var _ taxsource.Source[PipelineProperty] = (*PipelineList)(nil)` compiles. One yield is one parcel. A slide that names three PINs yields three times. Do not join those PINs into one string.
- [ ] Resolve the file from `https://www.unioncountync.gov/government/departments-r-z/taxes-property/collection-payment/property-tax-foreclosure-auction` on every run. Find the anchor whose title or text is `Properties Currently in Tax Foreclosure`. Use that href. Drop the `t` query parameter. Do not hardcode document id `4784`. Appendix A measured that id on 2026-11-10. If the page has no such anchor, GET `taxRunConfig.PipelineURL` when that string is non-empty. If the page has no anchor and `PipelineURL` is empty, or the chosen URL does not return a body, return the error and yield nothing. Do not guess an id. Do not read a PIN list from the operator. Do not file a records request.
- [ ] Branch on the body. A body that starts with `%PDF` is the PDF path. A body that is a zip whose `[Content_Types].xml` names a presentation is the pptx path. Any other body is an error and yields nothing. Appendix A measured a PDF on 2026-11-10. The same document id was a pptx on 2026-01-15. Both paths stay in this PR so a county switch does not become a manual job.
- [ ] The PDF path uses `compress/flate` and the page content streams. Read text showing operators and join glyphs that sit on one line, so a parcel token is `02.239.077` and not a string of spaced letters. Read `/ActualText` and `/Alt` when they exist. Do not render the page. Do not OCR. The operator deferred OCR until a later file actually has a picture-only property page. A page whose only drawings are images, with no text operators and no alt text, is skipped and counted. Appendix A found no such property page in this file. The background JPEG on every page is not the parcel data.
- [ ] The pptx path uses `archive/zip` and `encoding/xml`. Read `a:t` text runs, `a:tbl` cell text, `p:cNvPr` `descr` and `title`, and notes-slide `a:t`. Do not screenshot a slide. Do not add a module requirement. A slide whose shapes are only pictures, with empty text and empty descr and title, is skipped and counted.
- [ ] Delete `-` and `.` from each parcel token and then trim. Do not delete spaces. `02.239.077` becomes `02239077`. `07.027.002J80` becomes `07027002J80`. A label `Parcel` with no token is a skipped slide, not a personal-property row. Parse `Tax Value` by removing `$`, commas, and spaces. An empty value is NULL. A non-empty value that is not dollars and cents fails the file and yields nothing. Acreage stays TEXT, because the file mixes `0.234`, `5.30 acs`, and `Lot 16 College Grove`. A sale phrase with no year, such as `August 27`, leaves `sale_on` NULL and keeps the phrase in `schedule_text`. Set `redeemed` when the status phrase contains `Redeemed`. Store the case number from `Complaint Filed`, `Judicial Foreclosure Case`, or `Court Case`. A missing case number is NULL. Do not use the TBD case hash on this table.
- [ ] `SavePipeline` looks up each key with the tax-3 lookup and does not read `likely_vacant`. A stored parcel is upserted whether or not it has a house. A key with no parcel increments the unmatched count and inserts no row. Do not insert a parcel. The upsert omits `id` and conflicts on `(county_id, source_name, parcel_id)`. In the same transaction, delete pipeline rows for that `county_id` and `union_foreclosure_pipeline` whose `parcel_id` is absent from this file. A second save of the same parcel keeps `id`. The save result returns the saved count, the deleted count, the skipped-page count, and the unmatched count. Add `Latest` for `tax_foreclosure_pipeline`. Database tests use `//go:build postgres`.
- [ ] The golden PDF is one page whose text operators contain parcel `02.239.077`, an owner, and a tax value. The golden pptx is one slide with an `a:t` run, one table cell, a shape `descr`, and a notes `a:t`, each carrying a distinct parcel token. `-update` is off by default. `PipelineURL` is empty unless a test sets it.

**You see.**

- [ ] `go test -race ./internal/connector/union/ -count=1 -run TestPipeline` prints `PASS`, and `02.239.077` yields key `02239077`.

**Verify, unit.** Tests alone are not sufficient verification. A PR is verified only when its unit, live, and perf boxes are all checked.

- [ ] Table-driven tests cover the golden PDF parcel, the golden pptx text run, table cell, descr, and notes, a dotted PIN, a picture-only page that increments the skip count and yields nothing, a page with no anchor and an empty `PipelineURL` that returns an error, a page with no anchor and a set `PipelineURL` that fetches that URL, and a canceled context. Run `go test -race ./internal/connector/union/ -count=1 -run TestPipeline`.
- [ ] The postgres test saves a parcel with `likely_vacant` false, saves it again and keeps `id`, saves an unmatched key and inserts no row, and deletes a pipeline row whose parcel is absent from the second file. Run `go test -race -tags postgres ./internal/taxstore/ -count=1 -run TestSavePipeline`.
- [ ] Run `go vet ./internal/connector/union/ ./internal/taxstore/` and `staticcheck ./internal/connector/union/ ./internal/taxstore/`. Both print no findings. `go list -m all` does not gain a PDF or PowerPoint module.
- [ ] Run `go test -race -cover ./internal/connector/union/ ./internal/taxstore/ -count=1`. Fail if `union` coverage is below 77.4 percent, or below a higher trunk number measured at the start of this PR. Fail if `taxstore` coverage with `-tags postgres` is below 90.0 percent.

**Verify, live.** Tests alone are not sufficient verification. A PR is verified only when its unit, live, and perf boxes are all checked. Ten lanes on `grok-4.7-xhigh-fast` at the PR head, per the boot recipe.

- [ ] Lane 1. Regression lane against trunk. Run `go test -race ./internal/connector/union/ -count=1 -run 'TestDelinquent|TestBilling|TestForeclosure'` at trunk and at head. Trunk has no pipeline parser. Record that. Save `/tmp/swarm-tax-7/worker-1/regression.png`. Pass when head's earlier parser tests still pass and `TestPipeline` passes.
- [ ] Lane 2. Parse the golden PDF. Save `/tmp/swarm-tax-7/worker-2/golden-pdf.png`. Pass when parcel `02.239.077` is one yield with key `02239077` and the tax value is the fixture amount.
- [ ] Lane 3. Parse the golden pptx. Save `/tmp/swarm-tax-7/worker-3/golden-pptx.png`. Pass when the text run, the table cell, the descr, and the notes each yield their own parcel key.
- [ ] Lane 4. `SavePipeline` for a stored parcel with `likely_vacant` false, then again. Save `/tmp/swarm-tax-7/worker-4/upsert.png`. Pass when one row exists, the parcel is joined, and `id` is unchanged.
- [ ] Lane 5. Save a key that is not in `parcels`. Count `parcels`. Save `/tmp/swarm-tax-7/worker-5/unmatched.png`. Pass when the unmatched count is 1, no pipeline row is inserted, and the `parcels` count is unchanged.
- [ ] Lane 6. Fetch the auction page once. Save `/tmp/swarm-tax-7/worker-6/discover.png`. Pass when the body contains `Properties Currently in Tax Foreclosure` and the chosen href contains `showdocument`. Record the href. Do not fail only because the id is no longer `4784`.
- [ ] Lane 7. Fetch that href once with no `t` parameter. Save `/tmp/swarm-tax-7/worker-7/live-file.png`. Pass when the status is 200 and the body starts with `%PDF` or is a presentation zip. Record the byte count and the SHA-256. Appendix A recorded 770854 bytes and sha256 `b0f8926a4d9d893dc022c0225649d9752f2c905ecb8b0ffcf4813d845e5b801b` for the PDF fetched on 2026-11-10. A difference does not fail the lane. Write the new numbers in the lane report.
- [ ] Lane 8. Run the parser on that body and count yields. Save `/tmp/swarm-tax-7/worker-8/live-count.png`. Pass when the count is recorded. Appendix A counted 49 parcel labels on pages 4 through 26. A difference does not fail the lane. Pages 1, 2, 3, and 27 had no parcel label.
- [ ] Lane 9. Cancel `Read` before the request. Save `/tmp/swarm-tax-7/worker-9/cancel.png`. Pass when the error matches `context.Canceled` and no row is yielded.
- [ ] Lane 10. Select parcels that join `tax_foreclosure_pipeline` on `parcels.id` after a save of a parcel with `likely_vacant` false, and select `tax_owed`. Save `/tmp/swarm-tax-7/worker-10/upcoming-join.png`. Pass when the join returns that parcel and `tax_owed` does not gain a row from the pipeline row alone.

**Verify, perf.** Tests alone are not sufficient verification. A PR is verified only when its unit, live, and perf boxes are all checked.

- [ ] Metric. Wall time to parse the golden PDF and the golden pptx, and the yielded counts. Trunk has no parser. The end state is the golden yield count for each file.
- [ ] Probe. At trunk, record that `PipelineList` does not exist. At head, time 20 parses of each golden file.
- [ ] Baseline. Record trunk as no parser and no elapsed time.
- [ ] Rule. Fail if the average of 20 head parses of either golden file is 1 second or longer, or if either yielded count is not the golden count. Do not report a ratio against trunk.

**Review gate.** None. tax-7 is not review-gated.

**Merge.**

- [ ] Root's clean verdict at the exact head SHA.
- [ ] Bugbot triage done.
- [ ] Rebased onto current trunk after the verdict, with `git patch-id` unchanged.
- [ ] The owner does not merge. The operator lands tax-7 after tax-1 and tax-2.

## Wire the tax run (tax-6)

**Depends on.** tax-3, tax-4, tax-5, and tax-7.

**Files.**

- [ ] Edit `main.go`.
- [ ] Create `main_test.go`.

**Build.**

- [ ] Add `taxRunConfig` in `main.go` with `Enabled bool`, `Period time.Duration`, and `PipelineURL string`. The zero value of `Enabled` is false. The zero value of `PipelineURL` is empty. When `Enabled` is false, `main` does what trunk does. It does not download a county file. Do not add `func init`. Do not read `PipelineURL` from the environment.
- [ ] When `Enabled` is true and `Period` is zero, use 180 days. 180 days is a chosen stand-in for about twice a year. It is not a measured county cadence. The operator changes `Period` in this struct. Do not read the period from the environment. Do not add a cron entry, and do not sleep for the period. The process still exits when the run finishes.
- [ ] When `Enabled` is true, call `Latest` for `tax_delinquent_balances`, then `tax_bills`, then `tax_foreclosure_cases`, then `tax_foreclosure_pipeline`. If the returned time is inside `Period`, skip that file. Otherwise call `Read` and the matching save. One file at a time. No worker pool. A canceled context stops the next file. An error from a file is returned. Do not log it and return nil. Print the saved count and the skip counts from each save, including the foreclosure join count, the pipeline saved count, and the unmatched count. The pipeline `Read` discovers its URL from the auction page unless that page has no anchor, in which case it uses `PipelineURL`.
- [ ] Do not insert parcels. Do not call `IsLikelyVacantGISRecord` from the tax functions. Do not write `likely_vacant`. Do not import `internal/taxsource` from `main` if `main` only calls the union parsers and the taxstore functions.

**You see.**

- [ ] With the switch off, the main-package test prints parcel ids the way trunk does and does not mention `showdocument` or `showpublisheddocument`.

**Verify, unit.** Tests alone are not sufficient verification. A PR is verified only when its unit, live, and perf boxes are all checked.

- [ ] A test with `Enabled` false does not call `Read`. A test with `Enabled` true and an empty `Latest` uses local files and writes a delinquent row, a bill row, a foreclosure case, a join row, and a pipeline row for a parcel with `likely_vacant` false. A test with `fetched_at` inside 180 days does not call `Read`. A test with `fetched_at` older than `Period` does call `Read`. Run `go test -race . -count=1` from the module root for the main package tests, and `go test -race -tags postgres ./internal/taxstore/ -count=1`.
- [ ] Run `go vet ./...` and `staticcheck ./...`. Both print no findings.
- [ ] `rg -n "devnetwedge|func init\\(" --glob '*.go'` prints nothing.

**Verify, live.** Tests alone are not sufficient verification. A PR is verified only when its unit, live, and perf boxes are all checked. Ten lanes on `grok-4.7-xhigh-fast` at the PR head, per the boot recipe.

- [ ] Lane 1. Regression lane against trunk. Run `go test -race ./internal/service/ ./internal/connector/union/ -count=1` at trunk and at head. Save `/tmp/swarm-tax-6/worker-1/regression.png`. Pass when both pass. Trunk's `main` has no tax switch. Record that.
- [ ] Lane 2. Run the default path's unit test. Save `/tmp/swarm-tax-6/worker-2/switch-off.png`. Pass when no county file URL is requested.
- [ ] Lane 3. Run the switch-on path against local files for a likely-vacant parcel and a second stored parcel with `likely_vacant` false on the pipeline fixture. Save `/tmp/swarm-tax-6/worker-3/switch-on.png`. Pass when one delinquent row, one bill row, one foreclosure case, one join row, and one pipeline row are stored, and each `id` is a UUID. The bill and the balance join on `parcel_id` and `tax_year`. The pipeline row joins the not-vacant parcel on `parcels.id`.
- [ ] Lane 4. Select `tax_owed` after that run. Save `/tmp/swarm-tax-6/worker-4/owed.png`. Pass when the likely-vacant parcel id is the fixture `parcels.id`, `tax_year` is the fixture year, and the balance is greater than 0. The printed summary includes the skipped PP count. The foreclosure case alone did not create the view row. The pipeline row alone did not create it either.
- [ ] Lane 5. Repeat the switch-on run inside the period. Save `/tmp/swarm-tax-6/worker-5/second-run.png`. Pass when `Read` is not called and the delinquent, bill, case, join, and pipeline ids are unchanged.
- [ ] Lane 6. Set `fetched_at` older than `Period` and run again. Save `/tmp/swarm-tax-6/worker-6/period-elapsed.png`. Pass when `Read` runs and the delinquent, bill, case, join, and pipeline ids are still unchanged.
- [ ] Lane 7. Cancel during the first file. Save `/tmp/swarm-tax-6/worker-7/cancel.png`. Pass when the later files are not read.
- [ ] Lane 8. Count `parcels` around the run, and select `source_extras` for the fixture parcel. Save `/tmp/swarm-tax-6/worker-8/no-insert.png`. Pass when the tax run did not insert a parcel and did not change `source_extras`.
- [ ] Lane 9. Show the `main.go` call order. Save `/tmp/swarm-tax-6/worker-9/order.png`. Pass when delinquent `Read` appears before billing `Read`, billing appears before foreclosure `Read`, foreclosure appears before pipeline `Read`, all four sit inside the switch, and `IsLikelyVacantGISRecord` is not called from the tax functions.
- [ ] Lane 10. Run `go test -race ./internal/connector/union/ -count=1 -run 'TestDelinquent|TestBilling|TestForeclosure|TestPipeline'`. Save `/tmp/swarm-tax-6/worker-10/parsers.png`. Pass when all four pass.

**Verify, perf.** Tests alone are not sufficient verification. A PR is verified only when its unit, live, and perf boxes are all checked.

- [ ] Metric. Wall time of one local switch-on run over the golden files, and the row counts. Trunk has no tax switch. The end state is one delinquent row, one bill row, one foreclosure case, one join row, and one pipeline row.
- [ ] Probe. At trunk, time `go test -race ./internal/service/ -count=1`. At head, time the local switch-on test.
- [ ] Baseline. Record the trunk service test duration. Record trunk tax run as absent.
- [ ] Rule. Fail if the head local switch-on run takes 5 seconds or longer, or if any of those five counts is not 1. Do not report a ratio against the trunk service test. That test does not write tax rows.

**Review gate.** None. tax-6 is not review-gated.

**Merge.**

- [ ] Root's clean verdict at the exact head SHA.
- [ ] Bugbot triage done.
- [ ] Rebased onto current trunk after the verdict, with `git patch-id` unchanged.
- [ ] The owner does not merge. The operator lands tax-6 after tax-3, tax-4, tax-5, and tax-7.

## Close the program

- [ ] Every box above is checked with its evidence.
- [ ] Reply to the operator with the report the execution playbook names. List each PR with its verdict. Do not claim a merge.

## Appendix A. Prototype evidence

**Prototype.** None. There is no prototype branch and no prototype SHA. The rows below are the fetches and the coverage command.

**Measured.** On 2026-11-10 a fetch of `https://www.unioncountync.gov/home/showdocument?id=4792` with no `t` parameter returned the delinquent CSV. `curl` from this host to `www.unioncountync.gov` returned Akamai 403. The successful fetch was not that `curl`. The body was 300,371 bytes and 5,942 data rows. The header names were `Property Display Name`, `Property Key`, `Year`, `Primary Source`, and `Balance Due`, followed by empty columns. Those bytes and counts match the 2026-11-09 measurement. The file did not change.

**Measured.** Primary source counts were PP 5,013, RE 926, and RR 3. Years were 2017 through 2025. RE balance summed to 232755.26. The largest RE balance was 14355.02 on key `09125043` for year 2022. The largest amount in the file was 36800.63, on a PP row. Distinct RE keys were 325. RE key lengths were 8 on 629 rows, 9 on 280 rows, and 11 on 17 rows. Parcel `09336350` was not in this file.

**Measured.** Of the 280 nine-character RE rows, 278 are eight digits plus one letter, for example `07105038A`. Two rows are `U4225003F`. Distinct nine-character RE keys were 109. The 11-character keys include `07027002J80`, `07114083A02`, `02176011 60`, and `N7027327 01`. The space is inside the key. RR keys are `21` and `26`, two digits, not GIS parcel ids. RR is 3 rows and 2 keys.

**Measured.** The GIS download page still lists Billing Data updated 08/27/2026. A fetch of `https://www.unioncountync.gov/home/showpublisheddocument/6045/639234409945470000` saved 9,574,947 bytes. The saved body starts with a zip local header and the entry name `WKBILA-2026-07-22-1222.csv`. It also contains 2,189,061 UTF-8 replacement sequences, so that saved body is not a zip a parser can open. Do not treat 9,574,947 as the file length. This session did not re-count the billing rows.

**Read.** The operator measured the billing zip on 2026-11-08. One CSV, about 124 MB, 138,405 rows, RE 116,421, PP 21,984, year 2026, date due 2027-01-06. Parcel `09336350` was RE, total 2737.12, address `2915 CRIPPLE CREEK CT MONROE NC 28110`. The operator counted 320 of 325 delinquent RE keys on that billing file. This session did not repeat that match. The billing file has no paid column and no balance column.

**Measured.** On 2026-11-10 a fetch of `https://www.unioncountync.gov/home/showdocument?id=4787` with no `t` parameter returned the foreclosure sales list. The markdown conversion contained 9 PIN tokens. They were `09-273-051`, `07-129-160`, `07-129-161`, `07-129-162`, `09-231-101B`, `07-129-046`, `09-228-103`, `09-125-042`, and `09-125-043`. Deleting `-` produced keys. 8 of those 9 keys were in the delinquent RE set. `09273051` was not. `09125043` was, and its 2022 balance is 14355.02. Case `26M000471-890` names three PINs.

**Measured.** The auction page is `https://www.unioncountync.gov/government/departments-r-z/taxes-property/collection-payment/property-tax-foreclosure-auction`. `curl` and headless Chrome from this host got Akamai 403 for that page. A reader fetch on 2026-11-10 returned the anchors. The anchor titled `Properties Currently in Tax Foreclosure` points at `https://www.unioncountync.gov/home/showdocument?id=4784` with `t=637775972934935927`. The same href is in the 2026-05-13 archive HTML. The page sentence above that anchor says the power point lists delinquent properties the Tax Administrator is working to pursue.

**Measured.** Headed Chrome fetched `https://www.unioncountync.gov/home/showdocument?id=4784` with no `t` parameter on 2026-11-10. Status 200. `Content-Type` was `application/pdf`. `Content-Disposition` was `filename="Potential Foreclosures.pdf"`. `Last-Modified` was Mon, 12 Oct 2026 07:20:29 GMT. The body was 770,854 bytes. SHA-256 was `b0f8926a4d9d893dc022c0225649d9752f2c905ecb8b0ffcf4813d845e5b801b`. The file starts with `%PDF-1.7`. `pdfinfo` reports 27 pages, title `PowerPoint Presentation`, creator Microsoft PowerPoint for Microsoft 365, tagged yes, page size 960 by 540 points. `pdfimages -list` shows a background JPEG on every page, plus extra images on pages 1 and 27. `/ActualText` count was 0. `/Alt` count was 0.

**Measured.** `pdftotext -layout` on that PDF recovered the parcel lines from the text layer. Pages 4 through 26 hold 49 parcel labels and 49 distinct keys. Pages 1, 2, 3, and 27 have no parcel label. No property page is image-only. A naive join of text-showing strings inserts spaces inside words. The layout extract does not. One label is `Parcel: 02.239.077` with owner `Unknown Heirs of Freddie Sowell`, address `1009 Taylor Street, Wingate`, tax value `$156,600.00`, acreage `Lot 16 College Grove`, and complaint `25CV002739-890`, plus the phrases `Currently In Upset Bid Process` and `Redeemed/Satisfied October 8, 2026`. Page 14 lists `07.129.160`, `07.129.161`, and `07.129.162`. Page 16 lists `07.027.002J80` and `07.027.002J90`. Acreage text also includes `0.234` and `5.30 acs`. One sale phrase is `August 27` with no year.

**Measured.** The Internet Archive capture `20260215194311` of the same document URL was `application/vnd.openxmlformats-officedocument.presentationml.presentation`, filename `Potential Foreclosures.pptx`, last-modified Thu, 15 Jan 2026, crawler length 8,908,391. The saved body stopped at 5,242,880 bytes and is not a zip. Slide XML for that pptx was not counted. The live file on 2026-11-10 is the PDF above.

**Measured.** `https://unionnc.devnetwedge.com/robots.txt` returned 200 on 2026-11-10. `User-Agent: *` disallows `/parcel/*`, `/taxbill`, `/mobile_home/*`, `/view/*`, `/personalproperty/*`, `/scripts/*`, `/images/*`, and `/PropertyImages/*`. Last-Modified was Wed, 08 Jan 2025 16:24:02 GMT.

**Measured.** `go test -race -cover ./internal/connector/union/ -count=1` on 2026-11-10 printed coverage 77.4 percent of statements. One run. No spread was taken. The Go tree at `e5ac202` is the tree that was measured. That commit did not change Go files.

**Read.** `IsLikelyVacantGISRecord` is in `internal/connector/union/vacant_filter.go` lines 18 to 63. It returns true when `YEARBLT` or `SQFT` is numeric 0, when `FMV_IMPRV` is a zero currency string, or when `LAND_TYPE` contains `RURAL ACREAGE TRACT`. `main.go` lines 13 to 47 pass it to the GIS id stream and print ids. `main` does not call `parcelstore.SaveParcel`. `parcels` has no `likely_vacant` column on this trunk. persist-1 in `plans/soil-layer.md` adds it. The unique key is `(county_id, source_name, source_parcel_id)` at `db/migrations/000001_init_schema.sql` line 73. `parcels.id` is `UUID PRIMARY KEY DEFAULT gen_random_uuid()` at line 5.

**Read.** `internal/county/county.go` lines 15 to 18 declare `type ID string` and `Union ID = "union"`. `Parse`, `Valid`, `String`, and `All` are in that file. `db/migrations/000003_parcels_county_check.sql` checks `county_id IN ('union')`. This plan uses `county.Union`. It does not declare another county type.

**Read.** `internal/parcelstore/sales.go` lines 18 to 26 look up `parcels.id` by the natural key and omit `id` on insert. `lookupParcelSQL` is lines 91 to 97. The sale upsert conflicts on `(parcel_id, sequence)` and returns `id`, at lines 99 to 127. `SaveParcel` in `internal/parcelstore/parcels.go` omits `id` and conflicts on `(county_id, source_name, source_parcel_id)`. `db/migrations/000004_parcel_sales_uuid_id.sql` changes `parcel_sales.id` to UUID. Database tests in that package use `//go:build postgres`.

**Inferred.** Stripping `-` from a foreclosure PIN is the same key space as the delinquent `Property Key`, because 8 of the 9 fetched PINs hit that file. The one miss, `09-273-051`, does not prove a second key format. It may be absent from the delinquent extract.

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

```sql
INSERT INTO tax_delinquent_balances (
    county_id, source_name, parcel_id, tax_year, balance_due, primary_source,
    source_url, file_sha256, fetched_at
) VALUES (
    $1, $2, $3, $4, $5, $6, $7, $8, $9
)
ON CONFLICT (county_id, source_name, parcel_id, tax_year) DO UPDATE
SET balance_due = EXCLUDED.balance_due,
    primary_source = EXCLUDED.primary_source,
    source_url = EXCLUDED.source_url,
    file_sha256 = EXCLUDED.file_sha256,
    fetched_at = EXCLUDED.fetched_at,
    updated_at = now()
RETURNING id;
```

Open questions.

1. What should happen when a required column is renamed? This plan fails the file and yields nothing. An alias list is not defined.
2. What is the billing zip length, and what are today's row counts? The operator's 2026-11-08 counts stand. This session did not open a clean zip.
3. Should delinquent and billing rows also drop the `likely_vacant` gate, now that parcel ingest stores every parcel? This plan still skips those two files when `likely_vacant` is false. Foreclosure sales and the pipeline list do not use that gate. The owed view still requires `likely_vacant`. The recommendation is to drop the write-time gate and keep the filter on `tax_owed` only. The operator has not confirmed that.

## Appendix B. Alternatives rejected

`Read(ctx) ([]T, error)` loses. The billing CSV is about 124 MB and 138,405 rows on the operator's count. A slice holds every row before the first upsert. A cancel mid-file has already paid for the whole parse. The callback yields one row at a time.

A public fetch plus a public parse loses. The caller would open the body and then parse it. A zip has to be on disk before `archive/zip` can read the directory at the end. Hiding that inside `Read` keeps one call. The CSV parser stays a function over an `io.Reader` so the golden file and the fuzz target do not open a socket. That function is not the interface.

A separate `likely_vacant_parcels` table loses. persist-1 in `plans/soil-layer.md` stores `parcels.likely_vacant` on every parcel and does not insert a second identity. A tax row that stored the county parcel number as its key would ignore `parcels.id`. The sale writer already looks up that UUID and omits `id` on conflict. Tax rows do the same. This plan does not add the column and does not add the side table.

One foreclosure row per PIN loses. Case `26M000471-890` names three PINs. Copying the bids onto three rows repeats one case. A single text column of PINs loses because a query would parse that string. The case table holds the case. The join table holds one parcel per row.

Storing the text `TBD` as `case_number` loses. Two unsold cases would share that text and collapse. A null case number plus the PIN-set hash and the legal-text hash keeps them apart. Appendix E records that key for the operator to confirm.

A `JSONB` dump of the source row loses. `parcels.source_extras` is for GIS leftovers. The tax query surface is the typed columns. This plan does not add a document column.

`iter.Seq2` loses as the interface return. A parse error in the middle of a range has no `error` result on the method. A `yield func(T) error` returns that error from `Read`. Go 1.25 could express an iterator. This plan does not.

A per-parcel scraper, a worker pool, `Retry-After`, and a DevNet HTML table lose. The operator rejected the devnetwedge site. The files run about twice a year. They do not need a token bucket or a pool. One retry before the first body byte is the whole retry policy.

The annual 105-369 lien advertisement loses. The operator measured it as a March snapshot with no parcel ids. The 2026 notice lists owner names and street text, not GIS keys. It is not the delinquent tax notice this program parses. That notice is the CSV at `showdocument?id=4792`.

`unionnc.devnetwedge.com` loses. `robots.txt` disallows `/parcel/*` and `/taxbill` for every user agent. This plan does not design around that. A records request loses for the same reason. The public files are the source.

Railroad rows lose. Three rows, keys `21` and `26`. Those are not GIS parcel ids. Real-estate rows stay.

A hardcoded goose prefix loses. `000004_parcel_sales_uuid_id.sql` is already on main. persist-1 and soil-3 may take the next prefixes before this program starts. The owner lists the directory and adds 1.

Putting the foreclosure row into `tax_owed` loses. The operator's flag is the amount owed on a likely-vacant parcel. A scheduled sale is stored so a later pass can use it. It does not, by itself, mark a likely seller. A pipeline row does not set that flag either. The upcoming question is the pipeline join.

A boolean column on `parcels` for upcoming foreclosure loses. The pipeline file carries owner, address, tax value, acreage, a case number, and a status. A boolean would drop those fields and would write a tax source onto `parcels`. `parcel_sales` does not add a boolean either. A parcel is upcoming when `tax_foreclosure_pipeline.parcel_id` equals `parcels.id`. Houses stay in that join. The operator asked for the mark on the parcels we already ingest, and the join is that mark.

Screen-reading the pipeline deck loses. The live file is a PDF exported from PowerPoint, and the parcel lines are text operators. A screenshot or an OCR pass would throw away a text layer `pdftotext -layout` already reads. The pptx branch reads slide XML for the same reason. A picture-only page is counted and skipped. The operator deferred OCR until a file has such a page.

## Appendix C. Risks

`curl` from this host to `www.unioncountync.gov` returned Akamai 403 on 2026-11-10. A different fetch of the same document URLs returned the CSV and the foreclosure list. Live lanes that download those files can fail on a host Akamai blocks. That failure is real. Do not treat 403 as a pass.

The billing body saved in this session was not a valid zip after byte replacement. tax-4 lane 7 is the clean byte count. Until that count exists, the 256 MiB cap is a chosen ceiling at about twice the operator's 124 MB CSV, not a measured zip length. A zip directory sits at the end of the file, so the download finishes before the CSV parse starts.

The GIS download page says the data is presented as is and that the user holds the county harmless. The operator chose these files anyway. The plan does not add a click-through.

`control-cli` is not in this repo. Live lanes drive `go test`, `curl`, and `psql`, and they save a text render at the `.png` path. This repo has no `pstack/` tree. `git show origin/main:pstack/...` fails until those files exist on trunk.

`tax_owed` is empty until persist-1 has stored likely-vacant parcels and a delinquent balance is greater than 0. The tax tables do not insert parcels. A delinquent key that is not a likely-vacant parcel stays out of the table and out of the view.

`parcels.likely_vacant` is not on main at `e5ac202`. tax-2 cannot migrate until persist-1 has added it. Starting tax-2 earlier fails in `CREATE VIEW`.

The header spelling `Taypayer Address 1` is the operator's reading of the billing file. Lane 8 records the spelling this program actually sees. A mismatch fails the parse.

The foreclosure list is prose, not a CSV. A county edit that keeps the PIN label and moves the other labels can store a null owner and still pass the PIN check. Lane 7 records the PIN list so that drift is visible.

A `TBD` case that later receives a case number becomes a second row. The old null-number row stays, because this plan does not delete cases that disappear from a file. The operator can reject that and ask for a delete.

The 180 day period is a chosen default. A real county cadence that is not about twice a year is a config change in `taxRunConfig.Period`, not a code change in the parsers.

Headless Chrome and `curl` both got Akamai 403 for the pipeline document. Headed Chrome got the PDF. tax-7 uses the same `Get` as the other county files. A 403 is an error, not a pass, and it is not a prompt for the operator to type PINs.

The January 2026 pptx in the Internet Archive is truncated at 5,242,880 bytes of a crawler length 8,908,391, so that copy cannot prove slide XML. The pptx parser is still in tax-7 because the same URL has served a pptx. The golden pptx is a small fixture, not that archive body.

## Appendix D. Links and reading list

Read before editing.

- `internal/connector/union/vacant_filter.go` lines 18 to 63.
- `main.go` lines 13 to 47.
- `internal/county/county.go` for `county.ID` and `county.Union`.
- `internal/parcelstore/sales.go` lines 18 to 26 and 91 to 127, for the lookup and the upsert that omits `id`.
- `internal/parcelstore/parcels.go` for `SaveParcel`.
- `db/migrations/000001_init_schema.sql` lines 5 to 73.
- `db/migrations/000003_parcels_county_check.sql`.
- `db/migrations/000004_parcel_sales_uuid_id.sql`.
- `plans/soil-layer.md` persist-1, for `parcels.likely_vacant`. Do not edit that file.
- `plans/scraper-enrichment.md` is the per-parcel scraper plan still on main. This program does not edit it and does not implement it.
- The foreclosure auction page, the GIS download page, and the document URLs named in Appendix A.
- Effective Go and Go Code Review Comments. Small interfaces, accept interfaces and return structs, `context.Context` first, and do not repeat the package name in the type.

tax-3, tax-4, tax-5, and tax-7 get `pstack/skills/how/SKILL.md` before the parser is split from `Read`, because the question is which package owns the columns. The execution trail follows `pstack/skills/show-me-your-work/SKILL.md`.

## Appendix E. Decisions recorded

The operator said the devnetwedge tax site is out, and said he does not want a records request if the public files are enough. He asked whether this is still web scraping or a data fetching step. The answer he accepted is a fetch-and-parse step.

On the open questions he said, "Don't worry about the schedule yet because we are still building that should just be configurable somewhere down the line. Probably should store likely-vacant somewhere. don't think we care about the personal property not sure where that is coming from and idk about renamed columns just yet."

The schedule is config. tax-6 keeps the run off unless `Enabled` is set. The production period is about twice a year. The chosen default is 180 days, and the process does not loop. A later runner can read `Period`. This plan does not add cron.

Likely-vacant is `parcels.likely_vacant`, written by persist-1, not by this program. Delinquent and billing rows are still written only when that flag is true. `tax_owed` sums `balance_due` for those parcels and is the likely-seller flag. Whether those two files should also store every parcel is open question 3. Foreclosure sales and the pipeline list are not gated on the flag. The operator said upcoming foreclosures are wanted even when the parcel has a house, because parcel ingest stores every parcel.

Personal property is business equipment and vehicles taxed apart from the land. The delinquent file and the billing file drop those rows. The save result counts how many were skipped. The foreclosure list has no source column. Blocks without a PIN are skipped and are not called personal property.

A renamed required column still fails the file. What to do instead stays open.

`county.ID` is `internal/county` on main. This plan does not declare the type and does not edit `plans/soil-layer.md`.

Real-estate rows are stored. Railroad rows are dropped. Money is `NUMERIC(14, 2)`. Tax year is `SMALLINT` where the file has a year. Dates are `DATE` and are NULL when the source says `TBD` or names a month and day with no year. Every new table has a UUID primary key, `created_at`, and `updated_at`. Balances, bills, foreclosure cases, and pipeline rows store `county_id` and `source_name`, because those rows are source records the way `parcels` is. The foreclosure join table does not copy them, the way `parcel_sales` does not. The county parcel number is only the ingest lookup through `(county_id, source_name, source_parcel_id)` with `source_name` `union_county_gis`. Hyphens and dots are stripped for that lookup and are not stored. Spaces stay. The upsert omits `id`, so a rerun keeps the UUID. The tax write does not insert parcels.

A foreclosure case is one row. Each PIN that matches a stored parcel is one join row, house or not. A PIN that is not in `parcels` keeps the case, skips the join, and increments the unmatched count. The case upsert key for a null case number is `(county_id, source_name, parcel_set_sha256, legal_sha256)`. A real case number uses `(county_id, source_name, case_number)`. The operator confirms that key. Until then, tax-5 implements it as written.

A pipeline property is one row in `tax_foreclosure_pipeline` per stored parcel, with `source_name` `union_foreclosure_pipeline`. The program discovers the document from the auction page on each run and parses the text layer. It does not ask the operator to type PINs. A parcel leaves the table when the next file omits it. There is no upcoming column on `parcels`.

OCR is out of this program. A pipeline page with no text and no alt text is skipped and counted. The operator said to cross that bridge when a file actually needs it.

The stable delinquent URL is `showdocument?id=4792` with no `t` parameter. The billing link is read from the GIS download page so a changed timestamp does not stick in code. The foreclosure sales list is `showdocument?id=4787` with no `t` parameter. The upcoming list is whatever href the auction page publishes for `Properties Currently in Tax Foreclosure`. On 2026-11-10 that href was `showdocument?id=4784` and the body was a PDF.

`plans/scraper-enrichment.md` stays as it is on main. It does not say this plan replaced it. This PR does not turn it into a pointer. The soil plan still links it.
