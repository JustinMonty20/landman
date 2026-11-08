#!/usr/bin/env bash
# Exercise check-pr-naming.sh against accepted and rejected titles, branches, and plan files.
# Exits 0 when every case agrees with the convention. Exits 1 and names the failing case otherwise.

set -euo pipefail

script_dir=$(cd "$(dirname "$0")" && pwd)
repo_root=$(cd "${script_dir}/../.." && pwd)
checker="${script_dir}/check-pr-naming.sh"

fixture=$(mktemp -d)
mkdir -p "${fixture}/plans"
printf 'plan\n' > "${fixture}/plans/soil-layer.md"
printf 'plan\n' > "${fixture}/plans/use-uuid-parcel-ids.md"
trap 'rm -rf "$fixture"' EXIT

failures=0
checker_rc=0
checker_out=

run_checker() {
  set +e
  checker_out=$(PR_TITLE="$1" HEAD_REF="$2" ROOT="$3" bash "$checker" 2>&1)
  checker_rc=$?
  set -e
}

pass_case() {
  local name=$1
  run_checker "$2" "$3" "$4"
  if [[ "$checker_rc" -eq 0 && -z "$checker_out" ]]; then
    return 0
  fi
  failures=$((failures + 1))
  printf 'FAIL %s (expected pass, rc=%s)\n%s\n' "$name" "$checker_rc" "$checker_out" >&2
}

fail_case() {
  local name=$1
  local title=$2
  local branch=$3
  local root=$4
  shift 4
  local needle missing=0
  run_checker "$title" "$branch" "$root"
  if [[ "$checker_rc" -eq 0 ]]; then
    failures=$((failures + 1))
    printf 'FAIL %s (expected failure)\n%s\n' "$name" "$checker_out" >&2
    return 0
  fi
  for needle in "$@"; do
    if [[ "${needle:0:1}" == "!" ]]; then
      needle=${needle:1}
      if grep -F -q -- "$needle" <<<"$checker_out"; then
        missing=1
        printf 'FAIL %s (forbidden text present: %s)\n%s\n' "$name" "$needle" "$checker_out" >&2
      fi
    elif ! grep -F -q -- "$needle" <<<"$checker_out"; then
      missing=1
      printf 'FAIL %s (missing text: %s)\n%s\n' "$name" "$needle" "$checker_out" >&2
    fi
  done
  if [[ "$missing" -ne 0 ]]; then
    failures=$((failures + 1))
  fi
}

pass_case "chore" \
  "[chore:pr-naming-convention] Standardize PR naming and add title check" \
  "chore/pr-naming-convention" \
  "$fixture"

pass_case "fix without a plan file" \
  "[fix:rate-limit] Cap GIS retries" \
  "fix/rate-limit" \
  "$fixture"

pass_case "fix slug that also names a plan" \
  "[fix:soil-layer] Correct a soil mapping" \
  "fix/soil-layer" \
  "$fixture"

pass_case "plan fixture" \
  "[plan:soil-layer] Join county soil attributes" \
  "plan/soil-layer" \
  "$fixture"

pass_case "multi-segment plan slug" \
  "[plan:use-uuid-parcel-ids] Use UUID parcel ids" \
  "plan/use-uuid-parcel-ids" \
  "$fixture"

pass_case "minimal slug and summary" \
  "[fix:a] x" \
  "fix/a" \
  "$fixture"

pass_case "digit in slug" \
  "[fix:layer2] Refresh layer 2" \
  "fix/layer2" \
  "$fixture"

pass_case "slug that starts with a digit" \
  "[chore:2-layer] Refresh layer 2" \
  "chore/2-layer" \
  "$fixture"

pass_case "summary with brackets" \
  "[chore:pr-naming-convention] Accept a [plan:slug] example" \
  "chore/pr-naming-convention" \
  "$fixture"

pass_case "trailing space in summary" \
  "[fix:rate-limit] Cap GIS retries " \
  "fix/rate-limit" \
  "$fixture"

for plan_path in "$repo_root"/plans/*.md; do
  slug=$(basename "$plan_path" .md)
  pass_case "repo plan ${slug}" \
    "[plan:${slug}] Check ${slug}" \
    "plan/${slug}" \
    "$repo_root"
done

fail_case "prose title" \
  "Add soil attributes" \
  "plan/soil-layer" \
  "$fixture" \
  "PR title does not match the naming convention." \
  "Got title: \"Add soil attributes\"" \
  "Expected title: [plan:<slug>] <summary>, [fix:<slug>] <summary>, or [chore:<slug>] <summary>" \
  "!Expected branch:"

fail_case "missing summary" \
  "[fix:rate-limit]" \
  "fix/rate-limit" \
  "$fixture" \
  "Expected title:" \
  "The summary is required and starts after a single space." \
  "!Expected branch:"

fail_case "blank summary" \
  "[fix:rate-limit] " \
  "fix/rate-limit" \
  "$fixture" \
  "Expected title:" \
  "!Expected branch:"

fail_case "double space before summary" \
  "[fix:rate-limit]  Cap GIS retries" \
  "fix/rate-limit" \
  "$fixture" \
  "Expected title:" \
  "!Expected branch:"

fail_case "uppercase slug" \
  "[fix:Rate-Limit] Cap GIS retries" \
  "fix/rate-limit" \
  "$fixture" \
  "Got title: \"[fix:Rate-Limit] Cap GIS retries\"" \
  "Expected title:" \
  "!Expected branch:"

fail_case "underscore slug" \
  "[fix:rate_limit] Cap GIS retries" \
  "fix/rate_limit" \
  "$fixture" \
  "Expected title:"

fail_case "double hyphen" \
  "[fix:rate--limit] Cap GIS retries" \
  "fix/rate--limit" \
  "$fixture" \
  "Expected title:"

fail_case "leading hyphen" \
  "[fix:-rate] Cap GIS retries" \
  "fix/-rate" \
  "$fixture" \
  "Expected title:"

fail_case "trailing hyphen" \
  "[fix:rate-] Cap GIS retries" \
  "fix/rate-" \
  "$fixture" \
  "Expected title:"

fail_case "unknown type" \
  "[docs:readme] Update the readme" \
  "docs/readme" \
  "$fixture" \
  "Expected title:" \
  "!Expected branch:"

fail_case "text before the token" \
  "WIP [fix:rate-limit] Cap GIS retries" \
  "fix/rate-limit" \
  "$fixture" \
  "Expected title:"

fail_case "empty title" \
  "" \
  "fix/rate-limit" \
  "$fixture" \
  "Got title: \"\"" \
  "Expected title:"

fail_case "branch type disagrees with title" \
  "[plan:soil-layer] Join county soil attributes" \
  "fix/soil-layer" \
  "$fixture" \
  "Head branch does not match the PR title." \
  "Got branch: \"fix/soil-layer\"" \
  "Expected branch: \"plan/soil-layer\"" \
  "!PR title does not match the naming convention." \
  "!Expected file:"

fail_case "branch slug disagrees with title" \
  "[chore:pr-naming-convention] Standardize PR naming and add title check" \
  "chore/other-work" \
  "$fixture" \
  "Expected branch: \"chore/pr-naming-convention\""

fail_case "cursor prefix is not the convention" \
  "[chore:pr-naming-convention] Standardize PR naming and add title check" \
  "cursor/pr-naming-convention-4ae2" \
  "$fixture" \
  "Expected branch: \"chore/pr-naming-convention\""

fail_case "extra branch segment" \
  "[plan:soil-layer] Join county soil attributes" \
  "plan/soil-layer/extra" \
  "$fixture" \
  "Expected branch: \"plan/soil-layer\""

fail_case "empty branch" \
  "[fix:rate-limit] Cap GIS retries" \
  "" \
  "$fixture" \
  "Got branch: \"\"" \
  "Expected branch: \"fix/rate-limit\""

fail_case "missing plan file" \
  "[plan:missing-plan] Describe the missing work" \
  "plan/missing-plan" \
  "$fixture" \
  "Plan file does not exist on the PR head." \
  "Got title: \"[plan:missing-plan] Describe the missing work\"" \
  "Expected file: plans/missing-plan.md" \
  "!Expected branch:"

fail_case "branch and plan file both wrong" \
  "[plan:missing-plan] Describe the missing work" \
  "chore/missing-plan" \
  "$fixture" \
  "Expected branch: \"plan/missing-plan\"" \
  "Expected file: plans/missing-plan.md" \
  "!PR title does not match the naming convention."

default_root_rc=0
(
  cd "$repo_root"
  unset ROOT
  PR_TITLE='[plan:soil-layer] Join soil' HEAD_REF='plan/soil-layer' bash "$checker"
) || default_root_rc=$?
if [[ "$default_root_rc" -ne 0 ]]; then
  failures=$((failures + 1))
  printf 'FAIL default ROOT plan check (rc=%s)\n' "$default_root_rc" >&2
fi

set +e
annotation_out=$(GITHUB_ACTIONS=true PR_TITLE='100% done' HEAD_REF='fix/rate-limit' ROOT="$fixture" bash "$checker" 2>&1)
annotation_rc=$?
set -e
if [[ "$annotation_rc" -eq 0 ]] || ! grep -F -q '::error::' <<<"$annotation_out" || ! grep -F -q '100%25 done' <<<"$annotation_out" || ! grep -F -q 'Got title: "100% done"' <<<"$annotation_out"; then
  failures=$((failures + 1))
  printf 'FAIL annotation (rc=%s)\n%s\n' "$annotation_rc" "$annotation_out" >&2
fi

if [[ "$failures" -ne 0 ]]; then
  printf '%s case(s) failed\n' "$failures" >&2
  exit 1
fi

printf 'check-pr-naming: all cases passed\n'
