#!/usr/bin/env bash
set -euo pipefail

: "${GCP_PROJECT_ID:?Set GCP_PROJECT_ID to a Google Cloud project without billing}"

REGION="${GCP_REGION:-us-central1}"
REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"

command -v gcloud >/dev/null || { printf 'gcloud is required\n' >&2; exit 1; }
command -v firebase >/dev/null || { printf 'firebase-tools is required\n' >&2; exit 1; }

BILLING_ENABLED="$(gcloud billing projects describe "${GCP_PROJECT_ID}" --format='value(billingEnabled)' 2>/dev/null || true)"
if [[ "${BILLING_ENABLED}" == "True" && "${ALLOW_BILLED_PROJECT:-0}" != "1" ]]; then
  printf 'project %s has billing enabled; use an unbilled project for Spark or set ALLOW_BILLED_PROJECT=1\n' "${GCP_PROJECT_ID}" >&2
  exit 1
fi

gcloud config set project "${GCP_PROJECT_ID}"
gcloud services enable \
  firebase.googleapis.com \
  firebaserules.googleapis.com \
  firestore.googleapis.com \
  identitytoolkit.googleapis.com \
  securetoken.googleapis.com

if ! firebase projects:list --json | grep -q "\"projectId\": \"${GCP_PROJECT_ID}\""; then
  firebase projects:addfirebase "${GCP_PROJECT_ID}"
fi

if ! gcloud firestore databases describe --database='(default)' >/dev/null 2>&1; then
  gcloud firestore databases create \
    --database='(default)' \
    --location="${REGION}" \
    --type=firestore-native
fi

if ! firebase apps:list WEB --project "${GCP_PROJECT_ID}" --json | grep -q 'appId'; then
  firebase apps:create WEB "Yokai CLI" --project "${GCP_PROJECT_ID}"
fi

firebase deploy --only firestore:rules --project "${GCP_PROJECT_ID}" --config "${REPO_ROOT}/firebase.json"

APP_ID="$(firebase apps:list WEB --project "${GCP_PROJECT_ID}" --json | sed -n 's/.*"appId": "\([^"]*\)".*/\1/p' | head -1)"
printf 'Firebase backend is ready. Public client configuration:\n'
firebase apps:sdkconfig WEB "${APP_ID}" --project "${GCP_PROJECT_ID}"
