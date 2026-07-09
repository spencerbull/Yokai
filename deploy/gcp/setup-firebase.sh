#!/usr/bin/env bash
set -euo pipefail

: "${GCP_PROJECT_ID:?Set GCP_PROJECT_ID to the environment-specific Google Cloud project ID}"
: "${DEPLOY_STAGE:?Set DEPLOY_STAGE to development, staging, or production}"

case "${DEPLOY_STAGE}" in
  development|staging|production) ;;
  *) printf 'DEPLOY_STAGE must be development, staging, or production\n' >&2; exit 1 ;;
esac

REGION="${GCP_REGION:-us-central1}"
PROJECT_DISPLAY_NAME="${GCP_PROJECT_DISPLAY_NAME:-Yokai Config ${DEPLOY_STAGE}}"
REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"

command -v gcloud >/dev/null || { printf 'gcloud is required\n' >&2; exit 1; }
command -v firebase >/dev/null || { printf 'firebase-tools is required\n' >&2; exit 1; }
command -v jq >/dev/null || { printf 'jq is required\n' >&2; exit 1; }

if ! gcloud projects describe "${GCP_PROJECT_ID}" >/dev/null 2>&1; then
  if [[ "${CREATE_PROJECT:-0}" != "1" ]]; then
    printf 'project %s does not exist; set CREATE_PROJECT=1 to create it\n' "${GCP_PROJECT_ID}" >&2
    exit 1
  fi
  gcloud projects create "${GCP_PROJECT_ID}" --name="${PROJECT_DISPLAY_NAME}"
fi

BILLING_ENABLED="$(gcloud billing projects describe "${GCP_PROJECT_ID}" --format='value(billingEnabled)' 2>/dev/null || true)"
if [[ "${BILLING_ENABLED}" == "True" && "${ALLOW_BILLED_PROJECT:-0}" != "1" ]]; then
  printf 'project %s has billing enabled; use an unbilled project for Spark or set ALLOW_BILLED_PROJECT=1\n' "${GCP_PROJECT_ID}" >&2
  exit 1
fi

gcloud services enable \
  firebase.googleapis.com \
  firebaserules.googleapis.com \
  firestore.googleapis.com \
  identitytoolkit.googleapis.com \
  securetoken.googleapis.com \
  --project "${GCP_PROJECT_ID}"

if ! firebase projects:list --json | jq -e --arg project "${GCP_PROJECT_ID}" '.result[] | select(.projectId == $project)' >/dev/null; then
  firebase projects:addfirebase "${GCP_PROJECT_ID}"
fi

if ! gcloud firestore databases describe --project "${GCP_PROJECT_ID}" --database='(default)' >/dev/null 2>&1; then
  gcloud firestore databases create \
    --project "${GCP_PROJECT_ID}" \
    --database='(default)' \
    --location="${REGION}" \
    --type=firestore-native
fi

gcloud firestore databases update \
  --project "${GCP_PROJECT_ID}" \
  --database='(default)' \
  --delete-protection \
  --quiet >/dev/null

APPS="$(firebase apps:list WEB --project "${GCP_PROJECT_ID}" --json)"
if ! jq -e '.result[0].appId' <<<"${APPS}" >/dev/null; then
  firebase apps:create WEB "Yokai CLI ${DEPLOY_STAGE}" --project "${GCP_PROJECT_ID}"
  APPS="$(firebase apps:list WEB --project "${GCP_PROJECT_ID}" --json)"
fi

firebase deploy --only firestore:rules --project "${GCP_PROJECT_ID}" --config "${REPO_ROOT}/firebase.json"

APP_ID="$(jq -r '.result[0].appId' <<<"${APPS}")"
SDK_CONFIG="$(firebase apps:sdkconfig WEB "${APP_ID}" --project "${GCP_PROJECT_ID}" --json)"
printf 'Firebase %s backend is ready.\n' "${DEPLOY_STAGE}"
printf 'Project ID: %s\n' "${GCP_PROJECT_ID}"
printf 'App ID: %s\n' "$(jq -r '.result.sdkConfig.appId' <<<"${SDK_CONFIG}")"
printf 'API key: %s\n' "$(jq -r '.result.sdkConfig.apiKey' <<<"${SDK_CONFIG}")"
