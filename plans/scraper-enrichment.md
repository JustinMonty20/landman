# Parcel scraper plan

Scraping `https://unionnc.devnetwedge.com` parcel and tax pages is disallowed. `robots.txt` for every user agent disallows `/parcel/*`, `/taxbill`, `/mobile_home/*`, `/view/*`, `/personalproperty/*`, `/scripts/*`, `/images/*`, and `/PropertyImages/*`. The tax pages named for this program are in that set. This plan does not design a crawl, a year loop, a tax table, a worker pool, or fixtures taken from those URLs. It does not use `/search/quick` to get around the disallow.

You want one shared scraper contract for every website, and a Union scraper that implements it. The contract package imports `internal/county` and no site package. Union's scraper lives in `internal/connector/union`, and `main` constructs it. There is no registry and no `init`.

Scraped bytes go in their own table. The scraper does not insert into `parcels` and does not write `source_extras`. The devnetwedge host is not the fixture client. The fixture client does not dial it.

Run scrape-2 and scrape-3 first. Run scrape-1 after county-1 is on trunk. Run scrape-4 after scrape-1, scrape-3, and persist-1. Run scrape-5 after scrape-2 and scrape-4.

## How to read this

One box is one unit of work. Every box names the evidence that checks it. A nested box is a sub-step of the box above it. Check a box only when its evidence exists, a file, a log line, a screenshot, a test run, or a SHA. The body is a how-to. The appendices explain and record.

The program runs `pstack/skills/poteto-mode/playbooks/autopilot-stack.md`. The operator lands scrape-2, scrape-3, scrape-1, scrape-4, and scrape-5. This plan stops at merge-ready. Nobody merges from the checklist.

Tests alone are not sufficient verification. A PR is verified only when its unit, live, and perf boxes are all checked.

## Program checklist

### Arm the program

- [ ] State the protocol and this plan to the operator, then stop. Start execution only on the operator's explicit go.
- [ ] Read these from trunk at program start. Re-read them at every tick.
  - [ ] `git show origin/main:pstack/skills/poteto-mode/playbooks/autopilot-stack.md`
  - [ ] `git show origin/main:pstack/skills/swarm/SKILL.md`
  - [ ] `git show origin/main:pstack/skills/poteto-mode/playbooks/opening-a-pr.md`
  - [ ] `git show origin/main:pstack/skills/control-cli/SKILL.md`
  - [ ] `git show origin/main:plans/scraper-enrichment.md`
  - [ ] `git show origin/main:plans/soil-layer.md`
  - [ ] `git show origin/main:plans/spatialist-removal-and-critical-fixes.md`
- [ ] If a `git show` of `pstack/` or `control-cli` fails, read the installed skill with the same name and record the miss. Appendix C is that gap. Do not skip the live lanes.
- [ ] On the operator's go, arm the audit tick as `/loop 1h` with the tick prompt below. Never leave the cadence to memory.
- [ ] Use this tick prompt, verbatim. "Re-read the execution playbook from trunk. Audit the operation against it and fix drift in this tick. Probe every active lane and judge progress by side effects only. Stand down a stuck lane and dispatch its replacement now. Then post a short status message to the operator in chat only when the audit found a tracked change that no earlier status message reported, such as a PR opened, a code-ready head, a round launched or closed, a verdict, a merge, a stuck agent and the action taken, a blocker added or cleared, or a decision only the operator can make. Name every such change and nothing else. Do not repeat a table, the merged list, or an unchanged blocker. If the audit found none, end the turn with no reply text. Either way, log this tick's row in your decision trail. The row names the items reported, or none."
- [ ] On the operator's hold or stand-down, send every owner a zero-writes order at once.

### Spawn owners

- [ ] Spawn one owner per PR with the full lifecycle the execution playbook names.
- [ ] Follow this dependency graph. Start dependent work only after its parent is on trunk. Do not stack these PRs. scrape-4 and scrape-5 each need more than one parent, and a stack has one parent.
  - [ ] scrape-2 and scrape-3 are independent and first. Both branch from trunk.
  - [ ] scrape-1 after county-1 is on trunk. county-1 is not a PR of this program. Another agent owns that plan and PR. Do not edit `plans/soil-layer.md`.
  - [ ] scrape-4 after scrape-1, scrape-3, and persist-1 are on trunk. persist-1 is in `plans/soil-layer.md`.
  - [ ] scrape-5 after scrape-2 and scrape-4 are on trunk.
- [ ] Hold the file boundaries. scrape-2 touches only `internal/connector/shared/httpclient/`. scrape-3 touches only the new migration file. scrape-1 touches only `internal/scrape/`. scrape-4 touches only `internal/scrapestore/`. scrape-5 touches only `internal/connector/union/web.go`, `internal/connector/union/web_test.go`, `internal/connector/union/testdata/scrape/`, `main.go`, and the `init` sample in `docs/ARCHITECTURE.md`.
- [ ] Hold the review gate. No PR in this program changes an interaction. None wait for screenshots in chat before the operator decides to land them.
- [ ] Do not add a PR that requests `unionnc.devnetwedge.com`. Appendix A records the robots disallow. This program has no tax-year table, no worker pool, and no saved parcel page from that host.

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

Each live lane runs on its own cloud VM at the PR head. This repo has no browser UI for the scraper. Drive `go test`, `curl`, and `psql`. There is no `control-cli` skill in the repo. Appendix C records that gap. The commands in the lane are the drive.

- [ ] `git fetch origin <head-branch> && git checkout <head SHA>`.
- [ ] For scrape-1, scrape-2, and scrape-5, run the `go test -race` command in the lane. For scrape-3 and scrape-4, start Postgres and PostGIS the way `make migrate-up` does, apply migrations, and wait until `psql` accepts a query.
- [ ] Deliver input only through those commands. Read-only diagnostics are the test output, the `curl` body, and the `psql` rows.
- [ ] Save every screenshot to `/tmp/swarm-<pr-id>/worker-<n>/<slug>.png` and return the paths with the report. A lane with no display saves a text render of the command output at that path.

## Add per-scraper retry on the shared HTTP client (scrape-2)

**Depends on.** None.

**Files.**

- [ ] Edit `internal/connector/shared/httpclient/http_client.go`.
- [ ] Edit `internal/connector/shared/httpclient/http_client_test.go`.

**Build.**

- [ ] Add `Retry` and `DoRetry`. Leave `RateLimitedClient.Do` as a single wait plus `Client.Do`. GIS keeps calling `Do`. A scraper calls `DoRetry` on its own `RateLimitedClient`.
- [ ] `Retry.Attempts` of 0 or less becomes 3. Values above 5 become 5. `Retry.UserAgent` empty becomes `ParcelDataCollection/0.0.1`. `Retry.MaxBody` of 0 or less becomes 8 MiB. That cap is a chosen ceiling. Appendix A says no page size was measured.
- [ ] On 404 or 410, return `ErrNotFound` inside `*StatusError` and do not retry. On 401 or 403, return `ErrForbidden` and do not retry. On 408, 429, or 500 through 599, return `ErrTransient` and retry until the attempts are spent. Join a network error with `ErrTransient` while the context is still active.
- [ ] On 429, if `Retry-After` is a delay or an HTTP date, wait that long when it is longer than the limiter wait. Otherwise the limiter is the gap.
- [ ] If the error matches `context.Canceled` or `context.DeadlineExceeded`, return that error and do not wrap it with `ErrTransient`. Do not substitute `ctx.Err()` when `ctx.Err()` is nil. A response-header timeout matches `context.DeadlineExceeded` while `ctx.Err()` is nil, and returning `ctx.Err()` there yields a nil error.
- [ ] An error from the limiter `Wait` is not `ErrTransient` and is not retried. A `Wait` that cannot finish before the deadline does not match `context.DeadlineExceeded`. Wrapping it would burn the remaining attempts without a handler call.
- [ ] Retry a transient failure only for GET or HEAD with an empty body. A POST that returns 500 hits the handler once.
- [ ] Read at most `MaxBody` plus one byte. A larger body is an error, is not `ErrTransient`, and is not retried. Set `User-Agent` only when the request left it empty.
- [ ] Do not add a registry, a robots fetch, or a county import. Do not read the environment.

**You see.**

- [ ] `go test -race ./internal/connector/shared/httpclient/ -count=1 -run 'TestDoRetry|TestRateLimitedClient'` prints `PASS`.

**Verify, unit.** Tests alone are not sufficient verification. A PR is verified only when its unit, live, and perf boxes are all checked.

- [ ] Add table-driven `t.Run` cases for 200, 404, 401, 403, 410, 429 with `Retry-After`, 500 then 200, three 500s, a POST that returns 500 once, a canceled context, a response-header timeout whose `ctx.Err` is nil, a second call whose limiter `Wait` cannot finish before the deadline, a redirect that stays on the test server, an empty `User-Agent`, a preset `User-Agent`, and a body over the cap. Run `go test -race ./internal/connector/shared/httpclient/ -count=1`. The header-timeout case returns a non-nil error. The limiter-deadline case does not match `ErrTransient` and the handler count stays 0.
- [ ] Run `go vet ./internal/connector/shared/httpclient/` and `staticcheck ./internal/connector/shared/httpclient/`. Both print no findings.
- [ ] Run `go test -race -cover ./internal/connector/shared/httpclient/ -count=1`. Fail if the coverage line is below 100.0 percent. Trunk measured 100.0 percent. Appendix A.

**Verify, live.** Tests alone are not sufficient verification. A PR is verified only when its unit, live, and perf boxes are all checked. Ten lanes on `grok-4.7-xhigh-fast` at the PR head, per the boot recipe.

- [ ] Lane 1. Regression lane against trunk. Run `go test -race ./internal/connector/shared/httpclient/ -count=1 -run TestRateLimitedClient_Do_RateLimiting` at trunk and at head. Save `/tmp/swarm-scrape-2/worker-1/regression.png`. Pass when both runs pass and the head test still requires at least 900ms for three requests at 2 per second.
- [ ] Lane 2. Return 200 with body `ok`. Save `/tmp/swarm-scrape-2/worker-2/status-200.png`. Pass when `DoRetry` returns those bytes and the handler ran once.
- [ ] Lane 3. Return 404. Save `/tmp/swarm-scrape-2/worker-3/status-404.png`. Pass when `errors.Is` matches `ErrNotFound`, `errors.As` yields status 404, and the handler ran once.
- [ ] Lane 4. Return 403. Save `/tmp/swarm-scrape-2/worker-4/status-403.png`. Pass when `errors.Is` matches `ErrForbidden` and the handler ran once.
- [ ] Lane 5. Return 429 with `Retry-After: 1`, then 200. Save `/tmp/swarm-scrape-2/worker-5/retry-after.png`. Pass when the body is the 200 body, the handler ran twice, and the gap is at least the header delay.
- [ ] Lane 6. Return 500, then 200, with `Attempts` 3. Save `/tmp/swarm-scrape-2/worker-6/retry-500.png`. Pass when the body is the 200 body and the handler ran twice.
- [ ] Lane 7. Cancel the context before `DoRetry`. Save `/tmp/swarm-scrape-2/worker-7/cancel.png`. Pass when the error matches `context.Canceled`, does not match `ErrTransient`, and the handler ran zero times.
- [ ] Lane 8. Answer the first request with a redirect to a second path on the same `httptest` server. Save `/tmp/swarm-scrape-2/worker-8/redirect.png`. Pass when the body is the second path's body.
- [ ] Lane 9. Send a request with an empty `User-Agent`. Save `/tmp/swarm-scrape-2/worker-9/user-agent.png`. Pass when the handler sees `ParcelDataCollection/0.0.1`.
- [ ] Lane 10. Return a body one byte over `MaxBody`. Save `/tmp/swarm-scrape-2/worker-10/oversize.png`. Pass when the error does not match `ErrTransient` and the handler ran once.

**Verify, perf.** Tests alone are not sufficient verification. A PR is verified only when its unit, live, and perf boxes are all checked.

- [ ] Metric. Wall time of one local 200 through `DoRetry`, and wall time of the existing three-request rate test. Trunk has `Do` and has no `DoRetry`. The end state is a 200 body from `DoRetry` plus a rate test that still waits.
- [ ] Probe. At trunk, run `go test -race ./internal/connector/shared/httpclient/ -count=1 -run TestRateLimitedClient_Do_RateLimiting`. At head, run that test, then a one-request `DoRetry` against `httptest`, then the rate test again.
- [ ] Baseline. Record the trunk rate-test duration first. Record trunk `DoRetry` as absent.
- [ ] Rule. Fail if the head rate test finishes in under 900ms, or if one local 200 through `DoRetry` takes 1 second or longer. Do not report a ratio of `DoRetry` against trunk `Do`. The 1 second budget is the hang gate from Appendix A.

**Review gate.** None. scrape-2 is not review-gated.

**Merge.**

- [ ] Root's clean verdict at the exact head SHA.
- [ ] Bugbot triage done.
- [ ] Rebased onto current trunk after the verdict, with `git patch-id` unchanged.
- [ ] The owner does not merge. The operator lands scrape-2. It does not wait on scrape-3.

## Add the parcel page table (scrape-3)

**Depends on.** None.

**Files.**

- [ ] Create the next free migration. The name to use while `000003` is absent is `db/migrations/000003_parcel_page.sql`. Do not edit `000001_init_schema.sql` or `000002_parcels_storage_srid.sql`. If `000003` exists on trunk at the start of this PR, pick the next integer prefix and record it.

**Build.**

- [ ] Add `parcel_pages` with `county_id TEXT NOT NULL`, `parcel_id TEXT NOT NULL`, `source_name TEXT NOT NULL`, `fetched_at TIMESTAMPTZ NOT NULL`, `raw BYTEA NOT NULL`, and `raw_sha256 BYTEA NOT NULL`.
- [ ] Primary key `(county_id, parcel_id, source_name)`. Add `CHECK (county_id IN ('union'))`. Do not add a PostgreSQL enum. Do not add `GEOMETRY`. Do not add a foreign key to `parcels`.
- [ ] `parcel_id` is the caller's parcel id string, the same string the GIS hook passes. It is not `parcels.id`.
- [ ] Goose `Up` creates the table. Goose `Down` drops it. Do not write a Go inserter in this PR.

**You see.**

- [ ] After `make migrate-up`, `psql` lists `parcel_pages` and `\d parcel_pages` shows the primary key and the county check.

**Verify, unit.** Tests alone are not sufficient verification. A PR is verified only when its unit, live, and perf boxes are all checked.

- [ ] `rg -n "GEOMETRY|source_extras" db/migrations/000003_parcel_page.sql` prints nothing. `rg -n "parcel_pages_county_check" db/migrations/000003_parcel_page.sql` matches the check. Adjust the filename if the prefix changed.
- [ ] Run `go test -race ./... -count=1` with no database. It still passes. Run `go vet ./...` and `staticcheck ./...`. Both print no findings.

**Verify, live.** Tests alone are not sufficient verification. A PR is verified only when its unit, live, and perf boxes are all checked. Ten lanes on `grok-4.7-xhigh-fast` at the PR head, per the boot recipe.

- [ ] Lane 1. Regression lane against trunk. At trunk and at head, run `psql` with `\dt parcel_pages`. Save `/tmp/swarm-scrape-3/worker-1/regression.png`. Pass when trunk has no `parcel_pages` and head lists the table.
- [ ] Lane 2. Insert one row for county `union`, parcel `100`, source `union_county_scrape`. Save `/tmp/swarm-scrape-3/worker-2/insert.png`. Pass when the select returns that parcel id and a non-null `raw`.
- [ ] Lane 3. Upsert the same key with a new `raw` and `fetched_at`. Save `/tmp/swarm-scrape-3/worker-3/upsert.png`. Pass when the table still has one row and `raw` matches the second write.
- [ ] Lane 4. Insert county `wake`. Save `/tmp/swarm-scrape-3/worker-4/bad-county.png`. Pass when the county check rejects the row.
- [ ] Lane 5. Select `column_name` from `information_schema.columns` for `parcel_pages`. Save `/tmp/swarm-scrape-3/worker-5/columns.png`. Pass when `raw` and `raw_sha256` exist and `shape` does not.
- [ ] Lane 6. Select `source_extras` from `parcels` if that table has rows, and show the migration text. Save `/tmp/swarm-scrape-3/worker-6/extras.png`. Pass when the new file does not mention `source_extras` and `parcels` gained no column.
- [ ] Lane 7. Run goose down for this migration, then up again. Save `/tmp/swarm-scrape-3/worker-7/down-up.png`. Pass when the table is gone after down and present after up.
- [ ] Lane 8. Insert a row whose `raw_sha256` is not 32 bytes. Save `/tmp/swarm-scrape-3/worker-8/hash-length.png`. Pass when the insert is accepted by SQL. The length check belongs to the Go writer in scrape-4, not to this table.
- [ ] Lane 9. Insert two sources for the same parcel id. Save `/tmp/swarm-scrape-3/worker-9/two-sources.png`. Pass when both rows remain.
- [ ] Lane 10. Count rows in `parcels` before and after the parcel_pages insert. Save `/tmp/swarm-scrape-3/worker-10/no-parcel-insert.png`. Pass when the `parcels` count is unchanged.

**Verify, perf.** Tests alone are not sufficient verification. A PR is verified only when its unit, live, and perf boxes are all checked.

- [ ] Metric. Wall time of one upsert into `parcel_pages`, and the row count after a second upsert of the same key. Trunk has no table. The end state is one row.
- [ ] Probe. At trunk, record that `\dt parcel_pages` fails. At head, time one upsert, then a second upsert, with `psql`.
- [ ] Baseline. Record trunk as no table and no elapsed time.
- [ ] Rule. Fail if the head upsert takes 5 seconds or longer, or if the second upsert leaves a count other than 1. Do not report a ratio against trunk.

**Review gate.** None. scrape-3 is not review-gated.

**Merge.**

- [ ] Root's clean verdict at the exact head SHA.
- [ ] Bugbot triage done.
- [ ] Rebased onto current trunk after the verdict, with `git patch-id` unchanged.
- [ ] The owner does not merge. The operator lands scrape-3. It does not wait on scrape-2.

## Add the scrape contract and the fixture client (scrape-1)

**Depends on.** county-1.

**Files.**

- [ ] Create `internal/scrape/client.go`.
- [ ] Create `internal/scrape/errors.go`.
- [ ] Create `internal/scrape/robots.go`.
- [ ] Create `internal/scrape/fixture.go`.
- [ ] Create `internal/scrape/conform.go`.
- [ ] Create `internal/scrape/client_test.go`.
- [ ] Create `internal/scrape/robots_test.go`.
- [ ] Create `internal/scrape/fixture_test.go`.
- [ ] Create `internal/scrape/testdata/fixture-page.bin`.
- [ ] Create `internal/scrape/testdata/fixture-page.golden`.
- [ ] Create `internal/scrape/testdata/robots.txt`.

**Build.**

- [ ] Add `Request`, `Result`, and `Client` in package `scrape`. `Request` holds `county.CountyID` and `ParcelID`. `Result` holds `County`, `ParcelID`, `Source`, `FetchedAt`, `Raw`, and `Sum`. Use `county.CountyID` from `internal/county`. Do not declare another county type.
- [ ] Add `ErrNotFound`, `ErrForbidden`, `ErrTransient`, and `ErrParse`. Add `StatusError` with `Unwrap`. Package `scrape` imports `internal/county` and the standard library only.
- [ ] Add `RobotsAllow(body []byte, path, userAgent string) (bool, error)`. A disallow returns false. Do not fetch a URL inside this function.
- [ ] Add `Fixture` with `Scrape`. It copies `Raw` before return, sets `Sum` to SHA-256 of that copy, returns `ErrNotFound` for an unknown parcel id, and returns `ErrParse` for the bad fixture id. A later `Scrape` must not overwrite an earlier `Result.Raw`. A canceled context returns `ctx.Err()` before a lookup.
- [ ] Add `Conform(t *testing.T, client Client, county county.CountyID, source, knownID, missingID, badID string)`. Every later scraper calls it. The helper asserts the known id, the missing id, the bad id, an empty parcel id, cancellation, and a second call with the same `Sum`.
- [ ] Add `var _ Client = (*Fixture)(nil)` in `client.go`.
- [ ] Do not import `internal/connector/union`, `internal/service`, or `internal/models`. Do not add a registry.

**You see.**

- [ ] `go test -race ./internal/scrape/ -count=1 -run 'TestConformFixture|TestRobotsAllow'` prints `PASS` and names the known fixture id.

**Verify, unit.** Tests alone are not sufficient verification. A PR is verified only when its unit, live, and perf boxes are all checked.

- [ ] Table-driven tests with `t.Run` cover the known page, a missing id, a bad page, an empty parcel id, and a canceled context. The golden test compares the known `Sum` and `Raw` to `testdata/fixture-page.golden`. `-update` rewrites that file and is off by default. Run `go test -race ./internal/scrape/ -count=1`.
- [ ] Run `go test -race ./internal/scrape/ -fuzz=FuzzRobotsAllow -fuzztime=10s` and `go test -race ./internal/scrape/ -fuzz=FuzzFixtureParse -fuzztime=10s`. Both exit 0.
- [ ] Run `go vet ./internal/scrape/` and `staticcheck ./internal/scrape/`. Both print no findings.
- [ ] Run `go test -race -cover ./internal/scrape/ -count=1`. Fail if coverage is below 90.0 percent. That floor is a chosen gate for a new package. Appendix A.

**Verify, live.** Tests alone are not sufficient verification. A PR is verified only when its unit, live, and perf boxes are all checked. Ten lanes on `grok-4.7-xhigh-fast` at the PR head, per the boot recipe.

- [ ] Lane 1. Regression lane against trunk. Run `go test -race ./internal/connector/union/ ./internal/connector/shared/httpclient/ ./internal/service/ -count=1` at trunk and at head. Trunk has no `internal/scrape`. Record that, and also run `go test -race ./internal/scrape/ -count=1` at head. Save `/tmp/swarm-scrape-1/worker-1/regression.png`. Pass when trunk's existing packages pass and head's scrape tests pass.
- [ ] Lane 2. Run `Conform` on `Fixture` for the known id. Save `/tmp/swarm-scrape-1/worker-2/conform-known.png`. Pass when `County` is `union`, `Source` is `fixture`, and `Sum` matches the golden file.
- [ ] Lane 3. Scrape the missing id. Save `/tmp/swarm-scrape-1/worker-3/not-found.png`. Pass when the error matches `ErrNotFound` and `Raw` is empty.
- [ ] Lane 4. Scrape the bad id. Save `/tmp/swarm-scrape-1/worker-4/parse.png`. Pass when the error matches `ErrParse` and does not match `ErrTransient`.
- [ ] Lane 5. Cancel the context, then scrape the known id. Save `/tmp/swarm-scrape-1/worker-5/cancel.png`. Pass when the error matches `context.Canceled` and does not match `ErrTransient`.
- [ ] Lane 6. Scrape an empty `ParcelID`. Save `/tmp/swarm-scrape-1/worker-6/empty-id.png`. Pass when the error is non-nil and the golden bytes were not returned.
- [ ] Lane 7. Compare `Raw` to `testdata/fixture-page.bin`. Save `/tmp/swarm-scrape-1/worker-7/golden.png`. Pass when the bytes match and a second call returns the same `Sum`.
- [ ] Lane 8. Scrape parcel A, then scrape parcel B on the same `Fixture`. Save `/tmp/swarm-scrape-1/worker-8/copy.png`. Pass when the first `Result.Raw` is still parcel A's bytes, `Sum` is the SHA-256 of those bytes, and mutating the caller's copy of `Raw` leaves `Result.Raw` unchanged.
- [ ] Lane 9. Parse `testdata/robots.txt` with `RobotsAllow` for an allowed path and a disallowed path. Save `/tmp/swarm-scrape-1/worker-9/robots.png`. Pass when the allowed path is true, the disallowed path is false, and the function opened no socket.
- [ ] Lane 10. Parse the Go files in `internal/scrape` and list imports. Save `/tmp/swarm-scrape-1/worker-10/imports.png`. Pass when the only module import is `internal/county`.

**Verify, perf.** Tests alone are not sufficient verification. A PR is verified only when its unit, live, and perf boxes are all checked.

- [ ] Metric. Wall time of one `Fixture.Scrape` of the known id, and the `Sum` it returns. Trunk has no fixture. The end state is a `Result` whose `Sum` matches the golden file.
- [ ] Probe. At trunk, record that `./internal/scrape` does not exist. At head, time 20 calls of `Fixture.Scrape` in a test and print the average.
- [ ] Baseline. Record trunk as no package and no elapsed time. The scratch probe in Appendix A is not this binary.
- [ ] Rule. Fail if the average of 20 head calls is 1 second or longer, or if `Sum` differs from the golden file. Do not report a ratio against trunk.

**Review gate.** None. scrape-1 is not review-gated.

**Merge.**

- [ ] Root's clean verdict at the exact head SHA.
- [ ] Bugbot triage done.
- [ ] Rebased onto current trunk after the verdict, with `git patch-id` unchanged. county-1 is already on that trunk.
- [ ] The owner does not merge. The operator lands scrape-1 after county-1.

## Upsert a scraped page (scrape-4)

**Depends on.** scrape-1, scrape-3, and persist-1.

**Files.**

- [ ] Create `internal/scrapestore/pages.go`.
- [ ] Create `internal/scrapestore/pages_test.go`.

**Build.**

- [ ] Add `Save(ctx context.Context, db, page scrape.Result) error`. On conflict of `(county_id, parcel_id, source_name)`, update `raw`, `raw_sha256`, and `fetched_at`.
- [ ] Reject a `Sum` that is not the SHA-256 of `Raw`. Reject a county that fails `Valid`. Do not insert into `parcels`. Do not write `source_extras`. Do not read the environment. The constructor takes the connection, the same way persist-1 takes a connection string.
- [ ] One statement does the upsert. A second `Save` of the same key leaves one row. A canceled context writes nothing.

**You see.**

- [ ] A fixture `Result` becomes one `parcel_pages` row, and a second `Save` with a new `Raw` updates that row.

**Verify, unit.** Tests alone are not sufficient verification. A PR is verified only when its unit, live, and perf boxes are all checked.

- [ ] Table-driven tests cover a first insert, a second upsert, a mismatched `Sum`, an invalid county, and a canceled context. Run `go test -race ./internal/scrapestore/ -count=1` against the test database.
- [ ] Run `go vet ./internal/scrapestore/` and `staticcheck ./internal/scrapestore/`. Both print no findings.
- [ ] Run `go test -race -cover ./internal/scrapestore/ -count=1`. Fail if coverage is below 90.0 percent. That floor is a chosen gate for a new package. Appendix A.

**Verify, live.** Tests alone are not sufficient verification. A PR is verified only when its unit, live, and perf boxes are all checked. Ten lanes on `grok-4.7-xhigh-fast` at the PR head, per the boot recipe.

- [ ] Lane 1. Regression lane against trunk. Run `go test -race ./internal/service/ -count=1` at trunk and at head. Trunk has no `internal/scrapestore`. Record that, and run the new package tests at head. Save `/tmp/swarm-scrape-4/worker-1/regression.png`. Pass when trunk's service tests pass and head's `Save` tests pass.
- [ ] Lane 2. `Save` one fixture page. Save `/tmp/swarm-scrape-4/worker-2/insert.png`. Pass when `parcel_id` is the fixture id and `raw_sha256` equals `Sum`.
- [ ] Lane 3. `Save` the same key with different `Raw`. Save `/tmp/swarm-scrape-4/worker-3/upsert.png`. Pass when the count is 1 and `raw` matches the second save.
- [ ] Lane 4. `Save` a `Result` whose `Sum` does not match `Raw`. Save `/tmp/swarm-scrape-4/worker-4/bad-sum.png`. Pass when `Save` returns an error and the table gains no row.
- [ ] Lane 5. `Save` county `nope` if `Valid` rejects it. Save `/tmp/swarm-scrape-4/worker-5/bad-county.png`. Pass when `Save` returns an error. If county-1's `Valid` accepts only `union`, this is that rejection.
- [ ] Lane 6. Count `parcels` before and after `Save`. Save `/tmp/swarm-scrape-4/worker-6/no-parcel.png`. Pass when the count is unchanged.
- [ ] Lane 7. Select `source_extras` for a persisted parcel. Save `/tmp/swarm-scrape-4/worker-7/extras.png`. Pass when `Save` did not change it.
- [ ] Lane 8. Cancel the context before `Save`. Save `/tmp/swarm-scrape-4/worker-8/cancel.png`. Pass when no row is committed.
- [ ] Lane 9. Call `Save` twice with the same bytes. Save `/tmp/swarm-scrape-4/worker-9/same-bytes.png`. Pass when one row remains and `fetched_at` moved to the second call.
- [ ] Lane 10. Call `Save` with a good page, then call `Save` with a bad sum. Save `/tmp/swarm-scrape-4/worker-10/two-calls.png`. Pass when the failed second `Save` does not remove the first committed row, and the first row is still one row.

**Verify, perf.** Tests alone are not sufficient verification. A PR is verified only when its unit, live, and perf boxes are all checked.

- [ ] Metric. Wall time of one `Save` of the fixture page, and the row count after a second `Save`. Trunk has no writer. The end state is one `parcel_pages` row.
- [ ] Probe. At trunk, record that `internal/scrapestore` does not exist. At head, time one `Save`, then a second `Save`.
- [ ] Baseline. Record trunk as no writer and no elapsed time.
- [ ] Rule. Fail if one head `Save` takes 5 seconds or longer, or if the second `Save` leaves a count other than 1. Do not report a ratio against trunk.

**Review gate.** None. scrape-4 is not review-gated.

**Merge.**

- [ ] Root's clean verdict at the exact head SHA.
- [ ] Bugbot triage done.
- [ ] Rebased onto current trunk after the verdict, with `git patch-id` unchanged.
- [ ] The owner does not merge. The operator lands scrape-4 after scrape-1, scrape-3, and persist-1.

## Wire the Union site scraper (scrape-5)

**Depends on.** scrape-2 and scrape-4.

**Files.**

- [ ] Create `internal/connector/union/web.go`.
- [ ] Create `internal/connector/union/web_test.go`.
- [ ] Create `internal/connector/union/testdata/scrape/fixture-page.bin`.
- [ ] Edit `main.go`.
- [ ] Edit the `init` sample in `docs/ARCHITECTURE.md`.

**Build.**

- [ ] Add `WebSource`, `NewWebSource`, and `DefaultWebSourceConfig` in package `union`. `DefaultWebSourceConfig` leaves `BaseURL` empty and sets `Enabled` false. `Scrape(ctx, req scrape.Request) (scrape.Result, error)` implements `scrape.Client`.
- [ ] Add `var _ scrape.Client = (*WebSource)(nil)` in `web.go`.
- [ ] When `Enabled` is false, `Scrape` is not called from `main`. When `BaseURL` is empty, `Scrape` serves the fixture bytes for the known id, returns `ErrNotFound` for any other id, and does not dial. When `BaseURL` is set, build the request, call `httpclient.DoRetry` on a limiter owned by this `WebSource`, then `parse`. `parse` returns `ErrParse` for every body until a page schema exists. `parcelURL` returns `ErrNotConfigured` until the pattern exists, and it does not use the GIS host.
- [ ] `main` builds `NewWebSource` only when `Enabled` is true. The default path still prints GIS ids and returns the importer error. Do not log a scrape error and return nil. Do not add `func init`. Do not add a registry. Delete the `init` sample in `docs/ARCHITECTURE.md`.
- [ ] When `Enabled` is true, the parcel-id hook calls `WebSource.Scrape` and then `scrapestore.Save` for each success. That call sits after the parcel save persist-1 added. The hook does not insert a parcel. Package `union` does not import `scrapestore`. The `Enabled` false path calls neither `Scrape` nor `Save`.
- [ ] `web_test.go` calls `scrape.Conform` on a fixture `WebSource`. Do not import `internal/service` from a new file. `service` still does not import `union`.
- [ ] Do not set `BaseURL` to `https://unionnc.devnetwedge.com`. `parcelURL` must not build `/parcel/`, `/taxbill`, `/view/`, `/mobile_home/`, or `/personalproperty/`. A test fails if `DefaultWebSourceConfig` or the fixture client contains that host.

**You see.**

- [ ] With the default config, `go test -race ./internal/connector/union/ -count=1 -run TestWebSource` prints `PASS`, and a run of `main` still prints parcel ids and does not open a scrape socket.

**Verify, unit.** Tests alone are not sufficient verification. A PR is verified only when its unit, live, and perf boxes are all checked.

- [ ] Table-driven tests cover the known fixture id, an unknown id, an empty parcel id, a canceled context, an empty `BaseURL` against an `httptest` server with zero hits, and `NewWebSource` with `Enabled` false. Run `go test -race ./internal/connector/union/ -count=1`.
- [ ] Run `go vet ./internal/connector/union/` and `staticcheck ./internal/connector/union/`. Both print no findings.
- [ ] Run `go test -race -cover ./internal/connector/union/ -count=1`. Fail if coverage is below 76.8 percent. Trunk measured 76.8 percent. Appendix A.
- [ ] `rg -n "func init\\(" --glob '*.go'` prints nothing. `rg -n "func init\\(" docs/ARCHITECTURE.md` prints nothing. `rg -n "SingleParcelFetcher|BatchEnricher" --glob '*.go'` prints nothing.
- [ ] `rg -n "scrapestore.Save" main.go` matches the `Enabled` branch and no other branch. The default config keeps `Enabled` false.

**Verify, live.** Tests alone are not sufficient verification. A PR is verified only when its unit, live, and perf boxes are all checked. Ten lanes on `grok-4.7-xhigh-fast` at the PR head, per the boot recipe.

- [ ] Lane 1. Regression lane against trunk. Run `go test -race ./internal/connector/union/ -count=1` at trunk and at head. Trunk has no `WebSource`. Record that, and gate the new tests plus a green union package at head. Save `/tmp/swarm-scrape-5/worker-1/regression.png`. Pass when trunk's union tests pass and head's `TestWebSource` passes.
- [ ] Lane 2. Run `Conform` on the fixture `WebSource`. Save `/tmp/swarm-scrape-5/worker-2/conform.png`. Pass when the known id returns `Source` `union_county_scrape` and the missing id matches `ErrNotFound`.
- [ ] Lane 3. Point `BaseURL` at `httptest` while `parcelURL` still returns `ErrNotConfigured`. Save `/tmp/swarm-scrape-5/worker-3/no-dial.png`. Pass when the handler count is 0 and the error matches `ErrNotConfigured`.
- [ ] Lane 4. Cancel the context before `Scrape`. Save `/tmp/swarm-scrape-5/worker-4/cancel.png`. Pass when the error matches `context.Canceled`.
- [ ] Lane 5. Build `main` with the default config and run it only if the GIS host is reachable. If it is not, run the unit test that the default config has `Enabled` false. Save `/tmp/swarm-scrape-5/worker-5/main-default.png`. Pass when the test or the process shows `Enabled` false and no scrape request.
- [ ] Lane 6. Search Go files for `func init(`. Save `/tmp/swarm-scrape-5/worker-6/no-init.png`. Pass when the search prints nothing.
- [ ] Lane 7. Compare the fixture `Raw` to `testdata/scrape/fixture-page.bin`. Save `/tmp/swarm-scrape-5/worker-7/golden.png`. Pass when the bytes match.
- [ ] Lane 8. Call `Scrape` twice on the known id. Save `/tmp/swarm-scrape-5/worker-8/twice.png`. Pass when both `Sum` values match.
- [ ] Lane 9. Show that `WebSource` does not implement `connector.DataSource`, with the type assert in `web_test.go`. Save `/tmp/swarm-scrape-5/worker-9/not-datasource.png`. Pass when the assert is false.
- [ ] Lane 10. Run `go test -race ./internal/service/ -count=1`. Save `/tmp/swarm-scrape-5/worker-10/service.png`. Pass when the service tests pass and `internal/service` still does not import `union`.

**Verify, perf.** Tests alone are not sufficient verification. A PR is verified only when its unit, live, and perf boxes are all checked.

- [ ] Metric. Wall time of one fixture `WebSource.Scrape`, and the union package test duration at trunk and at head. Trunk has no `WebSource`. The end state is a fixture `Result` with the golden `Sum`.
- [ ] Probe. At trunk, time `go test -race ./internal/connector/union/ -count=1`. At head, time that command, then time 20 fixture `Scrape` calls.
- [ ] Baseline. Record the trunk union test duration first. Record trunk `WebSource` as absent.
- [ ] Rule. Fail if the average of 20 head fixture calls is 1 second or longer. Do not fail the PR because the head package test is slower than trunk. The package test gained cases. The 1 second gate is only the fixture call.

**Review gate.** None. scrape-5 is not review-gated.

**Merge.**

- [ ] Root's clean verdict at the exact head SHA.
- [ ] Bugbot triage done.
- [ ] Rebased onto current trunk after the verdict, with `git patch-id` unchanged.
- [ ] The owner does not merge. The operator lands scrape-5 after scrape-2 and scrape-4.

## Close the program

- [ ] Every box above is checked with its evidence.
- [ ] Reply to the operator with the report the execution playbook names. List each PR with its verdict. Do not claim a merge.

## Appendix A. Prototype evidence

**Measured.** Scratch module `/tmp/scrape-shapes`, commit `23bd1c5aef6a8ddc0c72c421b090f85552d09c46`, compiler `go1.22.2`. The repo's `go.mod` says `go 1.25.0`. A `[]Client` of two fake sites passed `go test -count=1`. A pure parser that copies its input passed. `Scraper[int]` and `Scraper[string]` each compiled alone. `[]Scraper[any]{intSite{}, stringSite{}}` failed. The compiler said `intSite` does not implement `Scraper[any]` because `Scrape` returns `int`, not `any`.

**Measured.** The same scratch module timed 20 local `httptest` fetches and 20 parses. Fetch average was 116.371 microseconds. Parse average was 125 nanoseconds. That is loopback on a 12 byte body, with no limiter and no reported spread. It is not a remote site budget. The 1 second fail rule in the perf boxes is about 8000 times that fetch average, so it catches a hang. It is not a tuning target.

**Measured.** A copy of `RateLimitedClient` under `/tmp/interrogate-scrape` failed three classification checks and one alias check. Wrapping a canceled `Do` with `ErrTransient` made `errors.Is` match both, and the loop ran 3 attempts with zero handler hits. A header timeout matched `context.DeadlineExceeded` while `ctx.Err()` was nil, so returning `ctx.Err()` returned nil. A limiter `Wait` that would miss the deadline did not match `context.DeadlineExceeded`, and wrapping it matched `ErrTransient`. A fixture that returned its own buffer changed the first `Raw` to the second parcel's bytes, while `Sum` stayed the hash of the first bytes.

**Measured.** On 2026-11-09, `GET https://unionnc.devnetwedge.com/robots.txt` returned 200, `content-type` `text/plain`, 205 bytes, `last-modified` `Wed, 08 Jan 2025 16:24:02 GMT`. The body is below. The next request was the site root, after a 3 second pause, with the same user agent `LandmanResearch/0.1 (public county parcel research; one-time polite sample; not a bulk crawl)`. The root returned 200, `text/html`, 15,672 bytes, title `Union County Property Tax Inquiry`. The visible footer says `© 2026 DEVNET, Inc`, `wEdge Version: 5.1.9693.29933`, and `Data updated: 2026-11-06 19:45:00`. The root lists no terms, privacy, or disclaimer link. No parcel, tax bill, or search URL was requested.

```
User-Agent: *
Disallow: /parcel/*
Disallow: /taxbill
Disallow: /mobile_home/*
Disallow: /view/*
Disallow: /personalproperty/*
Disallow: /scripts/*
Disallow: /images/*
Disallow: /PropertyImages/*
```

**Read.** DEVNET describes wEdge as the public web front for county property data, including tax bills and amounts due, at `https://www.devnetinc.com/wedge/`. Other hosts on the same product name include `https://clearfieldpa.devnetwedge.com/`, `https://pemiscotmo.devnetwedge.com/`, and `https://hancockil.devnetwedge.com/`. A shared parser would belong in a vendor package if a crawl were allowed. This plan does not add that package. The Union homepage also links `http://www.devnetinc.com` and `https://assets.devnetwedge.com/`.

**Read.** GIS parcel ids in `plans/soil-layer.md` include `01060017` and `01201017K`. The operator named `09336350` on the tax site. Those strings were not compared on a fetched tax page. The translator fixture `12-345-678` is a test value in `internal/connector/union/translator_test.go`, not a measured tax-site id.

**Measured.** On this tree, `go test -race -cover` printed `httpclient` 100.0 percent, `union` 76.8 percent, and `service` 84.3 percent. `internal/models` has no tests. `make test` is `go test -v -race ./...` at `Makefile` lines 27 to 28. There is no coverage floor, fuzz target, `testdata` directory, or `staticcheck` target in the repo.

**Read.** `RateLimitedClient.Do` waits, then calls `Client.Do`, at `internal/connector/shared/httpclient/http_client.go` lines 24 to 29. It does not retry. `NewRateLimitedClient` turns 0 into 1 and passes a negative rate through, lines 63 to 71. GIS sets `User-Agent` to `ParcelDataCollection/0.0.1` at `internal/connector/union/gis.go` line 219 and clamps a non-positive rate at lines 65 to 67. The only production `https` host in Go is the layer 215 query at `gis.go` line 27. No scraper website is named. Do not use that GIS host as the scraper target.

**Read.** `type CountyID` is not in Go on this branch. county-1 adds it in `internal/county`. This plan does not declare that type and does not edit `plans/soil-layer.md`. `parcels.county_id` is `TEXT NOT NULL` with no check, at `db/migrations/000001_init_schema.sql` line 8. The unique parcel key is line 73. `source_extras` is line 68. Nothing in Go inserts `parcels`. `000002_parcels_storage_srid.sql` already exists, so this program must not create `000002_soil.sql` or another `000002` file.

**Read.** Removal plan Steps 1, 2, and 5 are already on `main`. `main.go` is the GIS id stream. There is no `func init(` in Go. `service` does not import `union`. Only `main` imports `internal/connector/union`.

The chosen signatures are the contract. They are not a second copy of `county.CountyID`.

```go
package scrape

type Request struct {
	County   county.CountyID
	ParcelID string
}

type Result struct {
	County    county.CountyID
	ParcelID  string
	Source    string
	FetchedAt time.Time
	Raw       []byte
	Sum       [32]byte
}

type Client interface {
	Scrape(ctx context.Context, req Request) (Result, error)
}
```

```go
package httpclient

type Retry struct {
	Attempts  int
	UserAgent string
	MaxBody   int
}

func DoRetry(ctx context.Context, c *RateLimitedClient, req *http.Request, policy Retry) ([]byte, error)
```

```go
package union

func (s *WebSource) Scrape(ctx context.Context, req scrape.Request) (scrape.Result, error)

var _ scrape.Client = (*WebSource)(nil)
```

Open questions.

1. The operator named `https://unionnc.devnetwedge.com`, `/search/quick?q=` plus a GIS parcel id, and `/parcel/view/{PID}/{year}`. `robots.txt` disallows `/parcel/*`. Those parcel pages were not fetched. The PID match, the search result shape, and the unknown-PID status are unmeasured.
2. Tax fields on the live pages are unmeasured. No fixture was saved from that host. `Result.Raw` remains the re-parse payload for a site that allows a fetch.
3. Whether one page lists every tax year is unmeasured. Do not guess years, and do not design that crawl, until the disallow changes.
4. Skip versus stop for a live run stays unresolved. The operator's later rule is that a not-found skips and is recorded, a transient error retries, and a forbidden response or a 429 storm stops the run. That rule applies only after a site allows the fetch. This program does not implement that run.
5. If county-1 names `Valid` something else, match county-1. Do not keep a second method.
6. The 10 minute `main` timeout versus a full tax run is unmeasured. No parcel page was fetched, so there is no per-request time and no full-run estimate.
7. The 8 MiB body cap is a chosen ceiling. No page was measured. Change it when a permitted response has a size.

## Appendix B. Alternatives rejected

The base is the concrete HTTP helper and the Union type in package `union`. The shared `Client` interface is grafted on because the operator required it and the scratch compile showed a slice of one result type works.

**(a) loses as the whole design, and its one method stays.** `WebSource.Scrape` is that method. `main` calls it only when `Enabled` is true. The candidates rejected an exported interface because one production client does not earn a port, and because a `Raw` field leaks bytes. The operator asked for the interface now, and for raw bytes or a hash so a later parse can run again. `Result.Raw` is that payload. It is not an `http.Response`. The fixture client and `WebSource` are two implementations, so the interface is not a port with a single deleted body. Per laziness protocol, the interface has one method. Per subtract before you add, the registry stays out.

**(b) loses as the caller's API.** A public `Fetcher` plus a public `Parser` makes `main` fetch and then parse. The pure parser stays, unexported, so golden files and fuzz hit a function over bytes. `DoRetry` hides status, retry, `Retry-After`, and the body cap. Per boundary discipline, status is classified at the HTTP edge and `parse` does not see `*http.Response`.

**(c) loses.** `Scraper[T any]` compiles for one `T` and does not compile for a slice of two result types. The probe output is in Appendix A. A second site with a different `T` cannot sit in one list. `Result` is one type, so a second site implements `Client` without a type parameter. Per type system discipline, the county id is `county.CountyID`, not a second string type and not a type parameter.

**A child package `internal/connector/union/scrape` loses.** That package name collides with `internal/scrape` once both are imported. The child that imports parent `union` cannot be imported back by the parent. `WebSource` stays in package `union`, which is the directory the operator named.

**A registry or `init` loses.** One site does not need a plugin list. `main` calls `NewWebSource`. The audit and the operator agree on that point. The audit's "interface beside the consumer" is why the old plan put `parcelScraper` in package `union`. The operator overrode the package. The consumer is `main`, which cannot hold the interface, so the interface lives in `internal/scrape` and `main` does not import it. `main` imports `union`.

**Retries inside `RateLimitedClient.Do` lose.** GIS would start retrying. `DoRetry` is the scraper path. Each `WebSource` builds its own limiter. Per separate before serializing shared state, the GIS limiter and the scrape limiter are different values.

**Putting the interface in package `connector` loses.** `union` already imports `connector`. A `connector` type that named a Union record would cycle. `Client` does not name a Union type.

## Appendix C. Risks

`https://unionnc.devnetwedge.com/robots.txt` disallows the parcel and tax paths for every user agent. A crawl of those paths, a search used to reach them, a year loop, a tax table, and a worker pool are out of this program. The generic fixture client stays. It does not dial that host.

`control-cli` is not in this repo. Live lanes drive `go test`, `curl`, and `psql`, and they save a text render at the `.png` path. A lane that cannot draw still returns that file. Live lanes do not request the disallowed host.

This repo has no `pstack/` tree. `git show origin/main:pstack/...` fails until those files exist on trunk. The owner reads the installed poteto-mode skill and records that fallback. `deslop` was not installed in the planning session. The owner runs it when it is installed.

Claude seats for the arena cross-judge and for interrogate were rejected for weekly usage. The judge was skipped. A later attack proved four breaks. scrape-2 covers the wrapped cancel, the header timeout that becomes nil, and the limiter deadline that is not transient. scrape-1 covers the fixture buffer alias. Appendix A records the probe output.

soil-3 still says to create `db/migrations/000002_soil.sql`. That number is already `000002_parcels_storage_srid.sql`. scrape-3 must not take `000002`. If soil-3 lands first and takes `000003`, scrape-3 takes the next integer.

`WebSource` has a fixture branch and a later HTTP branch in one method. Delete the fixture branch in the same change that sets a real `BaseURL`. Leaving both is two ways to scrape.

`parcel_pages.parcel_id` does not reference `parcels.id`. A direct `Save` can store a page for a parcel row that does not exist. There is no foreign key. scrape-5 calls `Save` only from the `Enabled` branch, after the parcel save.

The default `main` path does not call `Scrape`. A later edit that sets `Enabled` true with an empty `BaseURL` would return `ErrNotFound` for real GIS ids and stop the import. Keep `Enabled` false until `parcelURL` is real.

`httpclient` tests that pass `server.Client()` do not exercise `NewHTTPClient` timeouts. scrape-2's header-timeout case sets `ResponseHeaderTimeout` on its own client. A cancel must not be wrapped as `ErrTransient`.

Package `scrape` imports `internal/county` and does not import `internal/soil`. county-1 must be on trunk first. If county-1's method is not `Valid() bool`, scrape-1 matches the method county-1 shipped.

The 10 minute timeout in `main` is a risk once scraping is enabled. It is open question 6.

## Appendix D. Links and reading list

Read before editing.

- `main.go` lines 13 to 47.
- `internal/connector/shared/httpclient/http_client.go` lines 18 to 72.
- `internal/connector/shared/httpclient/http_client_test.go` lines 131 to 192.
- `internal/connector/union/gis.go` lines 25 to 30, 65 to 70, and 213 to 242.
- `internal/connector/datasource.go` lines 8 to 15 and 28 to 49.
- `internal/connector/batch.go` lines 6 to 14.
- `internal/service/parcel_id_importer.go` lines 10 to 12 and 63 to 71.
- `internal/models/gis_parcel.go` lines 13 to 31.
- `db/migrations/000001_init_schema.sql` lines 5 to 73.
- `db/migrations/000002_parcels_storage_srid.sql`.
- `plans/soil-layer.md` persist-1 and the soil migration number. Do not edit that file. county-1 is the other agent's plan.
- `plans/spatialist-removal-and-critical-fixes.md` line 5.
- Effective Go and Go Code Review Comments. Small interfaces, accept interfaces and return structs, define the interface at the consumer, `context.Context` first, wrap with `%w`, and check sentinels with `errors.Is`.

scrape-1 and scrape-5 get `pstack/skills/how/SKILL.md` before the import list is edited, because the question is which package owns `Client`. scrape-2 gets `pstack/skills/interrogate/SKILL.md` on the retry loop before the code-ready swarm, because a wrapped `context.Canceled` that also matches `ErrTransient` is the break to look for. The execution trail follows `pstack/skills/show-me-your-work/SKILL.md`.

## Appendix E. Decisions recorded

The operator required a generic interface in a shared package, neutral to county and site, and a site scraper per website. Union is the first. It lives in `internal/connector/union`. `main` wires it. There is no `init` registry. That decision overrides the previous plan, which put `parcelScraper` in package `union`.

The operator required solid tests. Each implementation PR has unit, live, and perf boxes. The patterns are table-driven tests, `httptest`, golden files with `-update`, `Conform`, fuzz, `-race`, a compile-time assertion, `go vet`, `staticcheck`, and a coverage floor per package.

`CountyID` moves to `internal/county` in county-1. That plan and PR belong to another agent. This plan uses `county.CountyID` and does not declare the type. It does not edit `plans/soil-layer.md`. The Go type is not on `main`.

Scraped data goes in `parcel_pages`, keyed by county id, parcel id, and source name, with `fetched_at`, `raw`, and `raw_sha256`. It does not go in `parcels.source_extras`. persist-1 writes parcels first. The scraper never inserts parcels. `Save` is an upsert.

No registry until a second site exists. The fixture client is the test double, not a plugin.

The operator named the Union tax site, the search URL, and the parcel URL with a tax year. The operator asked whether every year has to be fetched, asked for parallel scraping, and asked to store what the owner owes because a balance due marks a property that might sell. Those answers are recorded here. `robots.txt` disallows the parcel and tax paths, so none of that crawl is designed.

The operator decided that `CountyID` leaves `internal/soil`. The package path in this plan is `internal/county`. If county-1 settles a different path, match county-1 and keep scrape-1's only module import equal to that path. scrape-1 depends on county-1, not on soil-1.

scrape-1 ships the interface and the fixture. scrape-5's default does not dial. The default base URL is not `https://unionnc.devnetwedge.com`.

The cross-judge was skipped after Claude weekly usage rejected `claude-opus-5-5-xhigh` and `claude-sonnet-5-5-high`. Both arena sketches were written by Grok. The pick is recorded in Appendix B.
