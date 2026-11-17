# Review verification

Use this procedure for every PR. Select checks before review, and use it again before you reuse a result after a commit changes.

## Select checks for a PR

Before the first PR-facing push, run the local equivalent of the repository's CI checks and focused tests for each changed behavior. Open the PR after this candidate passes; let CI check the pushed head. Add a PostgreSQL test when code changes database behavior. Use checked-in fixtures in CI when possible. Keep private-file tests separate and use only an approved disposable database.

At the code-ready head, choose independent checks for the changed behavior and its main failure risks. For new PRs, use relevant plan lane lists as catalogs, not fixed ten-lane requirements. Completed plans remain historical records; do not edit them to change this policy. Record the chosen checks and the reason for any relevant lane not selected in the PR body. One final independent review must inspect the diff and the saved results. Add more lanes or a full swarm only when the change crosses independent components, changes a destructive database path, or leaves a failure risk that focused checks cannot measure. State that reason before the extra work starts. The live gate passes when the selected lanes pass; unselected catalog entries are not failures.

After a small fix, check saved receipts and run one focused verifier for the affected behavior at the new head. Repeat the full selection only for new behavior, a redesign, or a large diff. A wording-only change needs no new test. Do not call a PR ready while a selected check is incomplete. A plan's explicit release inputs, performance limits, and acceptance behavior still apply; selection changes the number of independent lanes, not the product contract.

A **profile** is the file that lists an agent's tools. A **receipt** is a saved file with test output. A **patch ID** is a Git value for the content of a change. It can stay the same when a rebase changes a commit ID.

## Check reviewer tools

Find the selected agent's profile in the installed pi-subagents agent directory. Set `REVIEW_AGENT_PROFILE` to that file. Before you start a review child, check the profile:

```sh
python3 .github/scripts/verify-review.py profile "$REVIEW_AGENT_PROFILE"
```

The check requires `read` and `bash` in the profile. It does not prove that the child can run Git. Make the child's first command this check:

```sh
python3 .github/scripts/verify-review.py self-check --require go
```

Add one `--require` for each command the lane needs, such as `docker` or `psql`. Stop that review if either check fails. Report a tool limit, not a code failure. Use an agent with the required tools for a committed-code review. The parent must run the profile check; this repository cannot block every Pi child launch.

## Save and check test evidence

Save receipts outside Git. Include each file that can change the test result. Commit code before you record or check evidence; the tool rejects a dirty worktree. It records the head commit, base commit, patch ID, full-diff SHA-256 hash, and SHA-256 hash of each receipt and input file. It does not copy input bytes into the record.

```sh
mkdir -p artifacts/verify
python3 .github/scripts/verify-review.py record \
  --base origin/main \
  --receipt "$LANE_LOG" \
  --input billing=internal/connector/union/testdata/tax/billing.csv \
  --output artifacts/verify/billing.json
python3 .github/scripts/verify-review.py check artifacts/verify/billing.json
```

Use one `--receipt` for each output file and one named `--input` for each file the lane reads. Include private inputs by path only when the operator permits the check. Keep the record in the ignored `artifacts/verify/` directory. Do not commit receipts, private paths, or input files.

After a rebase, pass the new base:

```sh
python3 .github/scripts/verify-review.py check artifacts/verify/billing.json --base origin/main
```

`EXACT` means the head and file hashes match in a clean worktree. `UNCHANGED PATCH` means the file hashes, patch ID, and full diff match, but the head changed. The tax plan permits reuse after an unchanged rebase. Check any new base behavior before you use that result. `STALE` means a file or the patch changed. Use the plan's focused verifier rule for a small fix. Run a new full set of lanes only for new behavior, a redesign, or a large diff. A record cannot prove that a test passed: inspect each receipt before you save it.
