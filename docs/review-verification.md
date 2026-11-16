# Review verification

Use this procedure before a review that needs Git or shell commands. Use it again before you reuse a test result after a commit changes.

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
