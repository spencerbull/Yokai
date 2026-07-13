#!/usr/bin/env bash
set -euo pipefail

: "${GCP_PROJECT_ID:?Set GCP_PROJECT_ID to the environment-specific Google Cloud project ID}"
: "${DEPLOY_STAGE:?Set DEPLOY_STAGE to development, staging, or production}"

case "${DEPLOY_STAGE}" in
  development|staging|production) ;;
  *) printf 'DEPLOY_STAGE must be development, staging, or production\n' >&2; exit 1 ;;
esac

if [[ "${DEPLOY_STAGE}" == "production" ]]; then
  FIREBASE_APP_DISPLAY_NAME="${FIREBASE_APP_DISPLAY_NAME:-Yokai CLI}"
else
  FIREBASE_APP_DISPLAY_NAME="${FIREBASE_APP_DISPLAY_NAME:-Yokai CLI ${DEPLOY_STAGE}}"
fi

REGION="${GCP_REGION:-us-central1}"
PROJECT_DISPLAY_NAME="${GCP_PROJECT_DISPLAY_NAME:-Yokai Config ${DEPLOY_STAGE}}"
REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
# shellcheck source=setup-common.sh
source "${REPO_ROOT}/deploy/gcp/setup-common.sh"

command -v gcloud >/dev/null || { printf 'gcloud is required\n' >&2; exit 1; }
command -v firebase >/dev/null || { printf 'firebase-tools is required\n' >&2; exit 1; }
command -v jq >/dev/null || { printf 'jq is required\n' >&2; exit 1; }

FIREBASE_LIST_DIAGNOSTIC=""
firebase_project_state() {
  local error_file output
  if ! error_file="$(mktemp)"; then
    FIREBASE_LIST_DIAGNOSTIC="Unable to create a temporary file for Firebase CLI diagnostics."
    return 2
  fi
  if output="$(firebase projects:list --json 2>"${error_file}")"; then
    FIREBASE_LIST_DIAGNOSTIC="$(<"${error_file}")"
  else
    FIREBASE_LIST_DIAGNOSTIC="$(<"${error_file}")"
    rm -f "${error_file}"
    [[ -n "${output}" ]] && FIREBASE_LIST_DIAGNOSTIC+=$'\n'"${output}"
    return 2
  fi
  rm -f "${error_file}"

  if ! jq -e '.result | type == "array"' <<<"${output}" >/dev/null 2>&1; then
    FIREBASE_LIST_DIAGNOSTIC+=$'\nFirebase projects:list returned unexpected JSON.'
    return 2
  fi
  if jq -e --arg project "${GCP_PROJECT_ID}" \
    '.result[] | select(.projectId == $project)' <<<"${output}" >/dev/null; then
    return 0
  fi
  return 1
}

wait_for_firebase_project() {
  local attempt state
  for attempt in {1..8}; do
    state=0
    firebase_project_state || state=$?
    [[ "${state}" == "0" ]] && return 0
    (( attempt < 8 )) && sleep 3
  done
  return "${state}"
}

if ! gcloud projects describe "${GCP_PROJECT_ID}" >/dev/null 2>&1; then
  if [[ "${CREATE_PROJECT:-0}" != "1" ]]; then
    printf 'project %s does not exist; set CREATE_PROJECT=1 to create it\n' "${GCP_PROJECT_ID}" >&2
    exit 1
  fi
  gcloud projects create "${GCP_PROJECT_ID}" --name="${PROJECT_DISPLAY_NAME}"
fi

verify_spark_billing "${GCP_PROJECT_ID}" "${ALLOW_BILLED_PROJECT:-0}"

gcloud services enable \
  firebase.googleapis.com \
  firebaserules.googleapis.com \
  firestore.googleapis.com \
  identitytoolkit.googleapis.com \
  securetoken.googleapis.com \
  --project "${GCP_PROJECT_ID}"

FIREBASE_STATE=0
firebase_project_state || FIREBASE_STATE=$?
if [[ "${FIREBASE_STATE}" == "2" ]]; then
  printf '%s\n' "${FIREBASE_LIST_DIAGNOSTIC}" >&2
  exit 1
fi
if [[ "${FIREBASE_STATE}" == "1" ]]; then
  # addFirebase can finish server-side before the project appears in listProjects.
  # Treat a failed add as recoverable only when the project subsequently becomes visible.
  ADD_FIREBASE_OUTPUT=""
  if ADD_FIREBASE_OUTPUT="$(firebase projects:addfirebase "${GCP_PROJECT_ID}" 2>&1)"; then
    ADD_FIREBASE_STATUS=0
  else
    ADD_FIREBASE_STATUS=$?
  fi
  FIREBASE_STATE=0
  wait_for_firebase_project || FIREBASE_STATE=$?
  if [[ "${FIREBASE_STATE}" != "0" ]]; then
    [[ "${ADD_FIREBASE_STATUS}" != "0" ]] && printf '%s\n' "${ADD_FIREBASE_OUTPUT}" >&2
    [[ -n "${FIREBASE_LIST_DIAGNOSTIC}" ]] && printf '%s\n' "${FIREBASE_LIST_DIAGNOSTIC}" >&2
    printf 'Firebase project %s did not become ready after addFirebase\n' "${GCP_PROJECT_ID}" >&2
    exit 1
  fi
fi

if ! gcloud firestore databases describe --project "${GCP_PROJECT_ID}" --database='(default)' >/dev/null 2>&1; then
  gcloud firestore databases create \
    --project "${GCP_PROJECT_ID}" \
    --database='(default)' \
    --location="${REGION}" \
    --type=firestore-native
fi

for attempt in {1..8}; do
  if UPDATE_OUTPUT="$(gcloud firestore databases update \
    --project "${GCP_PROJECT_ID}" \
    --database='(default)' \
    --delete-protection \
    --quiet 2>&1)"; then
    break
  fi

  if [[ "${UPDATE_OUTPUT}" != *"ABORTED"* && "${UPDATE_OUTPUT}" != *"PERMISSION_DENIED"* ]]; then
    printf '%s\n' "${UPDATE_OUTPUT}" >&2
    exit 1
  fi
  if (( attempt == 8 )); then
    printf '%s\n' "${UPDATE_OUTPUT}" >&2
    printf 'Firestore delete protection did not become writable in time\n' >&2
    exit 1
  fi
  sleep 3
done

APPS="$(firebase apps:list WEB --project "${GCP_PROJECT_ID}" --json)"
APP_MATCH_COUNT="$(firebase_app_match_count "${APPS}" "${FIREBASE_APP_DISPLAY_NAME}")"
if [[ "${APP_MATCH_COUNT}" == "0" ]]; then
  firebase apps:create WEB "${FIREBASE_APP_DISPLAY_NAME}" --project "${GCP_PROJECT_ID}"
  APPS="$(firebase apps:list WEB --project "${GCP_PROJECT_ID}" --json)"
  APP_MATCH_COUNT="$(firebase_app_match_count "${APPS}" "${FIREBASE_APP_DISPLAY_NAME}")"
fi
if [[ "${APP_MATCH_COUNT}" != "1" ]]; then
  printf 'expected exactly one Firebase web app named %q, found %s\n' "${FIREBASE_APP_DISPLAY_NAME}" "${APP_MATCH_COUNT}" >&2
  exit 1
fi

firebase deploy --only firestore:rules --project "${GCP_PROJECT_ID}" --config "${REPO_ROOT}/firebase.json"

APP_ID="$(firebase_app_id "${APPS}" "${FIREBASE_APP_DISPLAY_NAME}")"
SDK_CONFIG="$(firebase apps:sdkconfig WEB "${APP_ID}" --project "${GCP_PROJECT_ID}" --json)"
API_KEY="$(jq -r '.result.sdkConfig.apiKey' <<<"${SDK_CONFIG}")"
API_KEY_NAME="$(gcloud services api-keys lookup "${API_KEY}" --project "${GCP_PROJECT_ID}" --format='value(name)')"
gcloud services api-keys update "${API_KEY_NAME}" \
  --project "${GCP_PROJECT_ID}" \
  --api-target=service=identitytoolkit.googleapis.com \
  --api-target=service=securetoken.googleapis.com \
  --quiet >/dev/null

printf 'Firebase %s backend is ready.\n' "${DEPLOY_STAGE}"
printf 'Project ID: %s\n' "${GCP_PROJECT_ID}"
printf 'App ID: %s\n' "$(jq -r '.result.sdkConfig.appId' <<<"${SDK_CONFIG}")"
printf 'API key: %s\n' "${API_KEY}"
