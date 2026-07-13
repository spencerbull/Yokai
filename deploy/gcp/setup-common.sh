#!/usr/bin/env bash

verify_spark_billing() {
  local project_id="$1" allow_billed_project="$2" billing_enabled
  if ! billing_enabled="$(gcloud billing projects describe "${project_id}" --format='value(billingEnabled)')"; then
    printf 'unable to verify billing state for project %s; refusing to provision\n' "${project_id}" >&2
    return 1
  fi
  case "${billing_enabled}" in
    False) return 0 ;;
    True)
      if [[ "${allow_billed_project}" == "1" ]]; then
        return 0
      fi
      printf 'project %s has billing enabled; use an unbilled project for Spark or set ALLOW_BILLED_PROJECT=1\n' "${project_id}" >&2
      return 1
      ;;
    *)
      printf 'unexpected billing state %q for project %s; refusing to provision\n' "${billing_enabled}" "${project_id}" >&2
      return 1
      ;;
  esac
}

firebase_app_match_count() {
  local apps_json="$1" display_name="$2"
  jq -r --arg name "${display_name}" '[.result[] | select(.displayName == $name)] | length' <<<"${apps_json}"
}

firebase_app_id() {
  local apps_json="$1" display_name="$2" count
  count="$(firebase_app_match_count "${apps_json}" "${display_name}")"
  if [[ "${count}" != "1" ]]; then
    printf 'expected exactly one Firebase web app named %q, found %s\n' "${display_name}" "${count}" >&2
    return 1
  fi
  jq -r --arg name "${display_name}" '.result[] | select(.displayName == $name) | .appId' <<<"${apps_json}"
}
