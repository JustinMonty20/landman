#!/usr/bin/env bash
# Check a pull request title, head branch, and plan file against the naming convention.
# QA parses the title to find the plan, so this fails the PR when that key is wrong.
#
# Inputs (environment):
#   PR_TITLE  pull request title
#   HEAD_REF  head branch name
#   ROOT      repository root that holds plans/<slug>.md (default: .)
#
# Side effects: writes each failure to stderr. On GitHub Actions, also writes a
# ::error:: annotation to stdout. Exits 0 when the title, branch, and plan file
# agree. Exits 1 otherwise.

set -euo pipefail

title_re='^\[(plan|fix|chore):([a-z0-9]+(-[a-z0-9]+)*)\] [^[:space:]].*$'
expected_title='[plan:<slug>] <summary>, [fix:<slug>] <summary>, or [chore:<slug>] <summary>'

ROOT=${ROOT:-.}
PR_TITLE=${PR_TITLE:-}
HEAD_REF=${HEAD_REF:-}

# Print one line, collapsing breaks so a title cannot forge log lines.
plain() {
  local value=$1
  value=${value//$'\n'/ }
  value=${value//$'\r'/ }
  printf '%s' "$value"
}

# Write a failure. In Actions, mirror it to a single-line annotation.
report() {
  printf '%s\n' "$1" >&2
  if [[ -z ${GITHUB_ACTIONS:-} ]]; then
    return 0
  fi
  local flat=${1//$'\n'/ }
  flat=${flat//'%'/'%25'}
  flat=${flat//$'\r'/'%0D'}
  printf '::error::%s\n' "$flat"
}

# Reject a title that QA cannot parse. Exits 1.
fail_title() {
  local shown
  shown=$(plain "$PR_TITLE")
  report "$(printf '%s\n' \
    "PR title does not match the naming convention." \
    "Got title: \"${shown}\"" \
    "Expected title: ${expected_title}" \
    "A slug is lowercase kebab-case, such as pr-naming-convention or soil-layer." \
    "The summary is required and starts after a single space.")"
  exit 1
}

# Set TYPE and SLUG from PR_TITLE. Exit 1 when the title does not match.
check_title() {
  [[ "$PR_TITLE" =~ $title_re ]] || fail_title
  TYPE=${BASH_REMATCH[1]}
  SLUG=${BASH_REMATCH[2]}
}

# Require the head branch to be <type>/<slug> from the title.
check_branch() {
  local expected="${TYPE}/${SLUG}"
  [[ "$HEAD_REF" == "$expected" ]] && return 0
  report "$(printf '%s\n' \
    "Head branch does not match the PR title." \
    "Got branch: \"$(plain "$HEAD_REF")\"" \
    "Got title: \"$(plain "$PR_TITLE")\"" \
    "Expected branch: \"${expected}\"")"
  return 1
}

# For plan work, require plans/<slug>.md on the tree ROOT points at.
check_plan_file() {
  [[ "$TYPE" == "plan" ]] || return 0
  local rel="plans/${SLUG}.md"
  [[ -f "${ROOT}/${rel}" ]] && return 0
  report "$(printf '%s\n' \
    "Plan file does not exist on the PR head." \
    "Got title: \"$(plain "$PR_TITLE")\"" \
    "Expected file: ${rel}")"
  return 1
}

check_title
status=0
check_branch || status=1
check_plan_file || status=1
exit "$status"
