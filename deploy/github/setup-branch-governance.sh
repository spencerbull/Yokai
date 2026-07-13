#!/usr/bin/env bash
set -euo pipefail

GITHUB_REPOSITORY="${GITHUB_REPOSITORY:-spencerbull/Yokai}"
REQUIRED_APPROVER="${REQUIRED_APPROVER:-spencerbull}"
RULESET_NAME="${RULESET_NAME:-Protected long-lived branches}"
APPROVAL_RULESET_NAME="${APPROVAL_RULESET_NAME:-Long-lived branch approval}"

command -v gh >/dev/null || { printf 'gh is required\n' >&2; exit 1; }
command -v jq >/dev/null || { printf 'jq is required\n' >&2; exit 1; }

APPROVER_ID="$(gh api "users/${REQUIRED_APPROVER}" --jq '.id')"
PAYLOAD="$(jq -n \
  --arg name "${RULESET_NAME}" \
  '{
    name: $name,
    target: "branch",
    enforcement: "active",
    bypass_actors: [],
    conditions: {
      ref_name: {
        include: ["refs/heads/develop", "refs/heads/staging", "refs/heads/main"],
        exclude: []
      }
    },
    rules: [
      {type: "deletion"},
      {type: "non_fast_forward"},
      {
        type: "pull_request",
        parameters: {
          allowed_merge_methods: ["merge", "squash", "rebase"],
          dismiss_stale_reviews_on_push: true,
          require_code_owner_review: false,
          require_last_push_approval: false,
          required_approving_review_count: 0,
          required_review_thread_resolution: true
        }
      },
      {
        type: "required_status_checks",
        parameters: {
          do_not_enforce_on_create: false,
          strict_required_status_checks_policy: true,
          required_status_checks: [
            {context: "build (1.25)"},
            {context: "lint"},
            {context: "tui"},
            {context: "release-package"},
            {context: "firestore-rules"}
          ]
        }
      }
    ]
  }')"

APPROVAL_PAYLOAD="$(jq -n \
  --arg name "${APPROVAL_RULESET_NAME}" \
  --argjson approver "${APPROVER_ID}" \
  '{
    name: $name,
    target: "branch",
    enforcement: "active",
    bypass_actors: [
      {actor_id: $approver, actor_type: "User", bypass_mode: "pull_request"}
    ],
    conditions: {
      ref_name: {
        include: ["refs/heads/develop", "refs/heads/staging", "refs/heads/main"],
        exclude: []
      }
    },
    rules: [
      {
        type: "pull_request",
        parameters: {
          allowed_merge_methods: ["merge", "squash", "rebase"],
          dismiss_stale_reviews_on_push: true,
          require_code_owner_review: true,
          require_last_push_approval: false,
          required_approving_review_count: 1,
          required_review_thread_resolution: false
        }
      }
    ]
  }')"

RULESET_ID="$(gh api "repos/${GITHUB_REPOSITORY}/rulesets" --paginate | jq -r --arg name "${RULESET_NAME}" '.[] | select(.name == $name) | .id' | head -1)"
if [[ -n "${RULESET_ID}" ]]; then
  gh api -X PUT "repos/${GITHUB_REPOSITORY}/rulesets/${RULESET_ID}" --input - >/dev/null <<<"${PAYLOAD}"
else
  RULESET_ID="$(gh api -X POST "repos/${GITHUB_REPOSITORY}/rulesets" --input - --jq '.id' <<<"${PAYLOAD}")"
fi

APPROVAL_RULESET_ID="$(gh api "repos/${GITHUB_REPOSITORY}/rulesets" --paginate | jq -r --arg name "${APPROVAL_RULESET_NAME}" '.[] | select(.name == $name) | .id' | head -1)"
if [[ -n "${APPROVAL_RULESET_ID}" ]]; then
  gh api -X PUT "repos/${GITHUB_REPOSITORY}/rulesets/${APPROVAL_RULESET_ID}" --input - >/dev/null <<<"${APPROVAL_PAYLOAD}"
else
  APPROVAL_RULESET_ID="$(gh api -X POST "repos/${GITHUB_REPOSITORY}/rulesets" --input - --jq '.id' <<<"${APPROVAL_PAYLOAD}")"
fi

printf 'Configured ruleset %s (%s) for develop, staging, and main.\n' "${RULESET_NAME}" "${RULESET_ID}"
printf 'Configured approval ruleset %s (%s).\n' "${APPROVAL_RULESET_NAME}" "${APPROVAL_RULESET_ID}"
printf 'Pull requests require passing checks, resolved threads, and approval from code owner @%s.\n' "${REQUIRED_APPROVER}"
printf '@%s may bypass only the approval ruleset while merging a pull request.\n' "${REQUIRED_APPROVER}"
