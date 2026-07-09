#!/usr/bin/env bash
set -euo pipefail

: "${GCP_PROJECT_ID:?Set GCP_PROJECT_ID to the Firebase project ID}"

GITHUB_REPOSITORY="${GITHUB_REPOSITORY:-spencerbull/Yokai}"
POOL_ID="${WIF_POOL_ID:-github-actions}"
PROVIDER_ID="${WIF_PROVIDER_ID:-yokai-repository}"
SERVICE_ACCOUNT_ID="${DEPLOY_SERVICE_ACCOUNT_ID:-github-firestore-deployer}"
SERVICE_ACCOUNT="${SERVICE_ACCOUNT_ID}@${GCP_PROJECT_ID}.iam.gserviceaccount.com"

command -v gcloud >/dev/null || { printf 'gcloud is required\n' >&2; exit 1; }
command -v gh >/dev/null || { printf 'gh is required\n' >&2; exit 1; }

PROJECT_NUMBER="$(gcloud projects describe "${GCP_PROJECT_ID}" --format='value(projectNumber)')"
gcloud services enable iamcredentials.googleapis.com sts.googleapis.com --project "${GCP_PROJECT_ID}"

if ! gcloud iam workload-identity-pools describe "${POOL_ID}" --location=global --project "${GCP_PROJECT_ID}" >/dev/null 2>&1; then
  gcloud iam workload-identity-pools create "${POOL_ID}" \
    --location=global \
    --project "${GCP_PROJECT_ID}" \
    --display-name="GitHub Actions"
fi

if ! gcloud iam workload-identity-pools providers describe "${PROVIDER_ID}" --workload-identity-pool="${POOL_ID}" --location=global --project "${GCP_PROJECT_ID}" >/dev/null 2>&1; then
  gcloud iam workload-identity-pools providers create-oidc "${PROVIDER_ID}" \
    --workload-identity-pool="${POOL_ID}" \
    --location=global \
    --project "${GCP_PROJECT_ID}" \
    --display-name="${GITHUB_REPOSITORY}" \
    --issuer-uri="https://token.actions.githubusercontent.com" \
    --attribute-mapping="google.subject=assertion.sub,attribute.repository=assertion.repository,attribute.ref=assertion.ref" \
    --attribute-condition="assertion.repository=='${GITHUB_REPOSITORY}' && assertion.ref=='refs/heads/main'"
fi

gcloud iam workload-identity-pools providers update-oidc "${PROVIDER_ID}" \
  --workload-identity-pool="${POOL_ID}" \
  --location=global \
  --project "${GCP_PROJECT_ID}" \
  --attribute-condition="assertion.repository=='${GITHUB_REPOSITORY}' && assertion.ref=='refs/heads/main'" >/dev/null

if ! gcloud iam service-accounts describe "${SERVICE_ACCOUNT}" --project "${GCP_PROJECT_ID}" >/dev/null 2>&1; then
  gcloud iam service-accounts create "${SERVICE_ACCOUNT_ID}" \
    --project "${GCP_PROJECT_ID}" \
    --display-name="GitHub Firestore Rules Deployer"
fi

for role in roles/firebaserules.admin roles/firebase.viewer roles/serviceusage.serviceUsageConsumer; do
  gcloud projects add-iam-policy-binding "${GCP_PROJECT_ID}" \
    --member="serviceAccount:${SERVICE_ACCOUNT}" \
    --role="${role}" \
    --condition=None >/dev/null
done

gcloud iam service-accounts add-iam-policy-binding "${SERVICE_ACCOUNT}" \
  --project "${GCP_PROJECT_ID}" \
  --role="roles/iam.workloadIdentityUser" \
  --member="principalSet://iam.googleapis.com/projects/${PROJECT_NUMBER}/locations/global/workloadIdentityPools/${POOL_ID}/attribute.repository/${GITHUB_REPOSITORY}" >/dev/null

PROVIDER="projects/${PROJECT_NUMBER}/locations/global/workloadIdentityPools/${POOL_ID}/providers/${PROVIDER_ID}"
gh variable set FIREBASE_PROJECT_ID --repo "${GITHUB_REPOSITORY}" --body "${GCP_PROJECT_ID}"
gh variable set GCP_WORKLOAD_IDENTITY_PROVIDER --repo "${GITHUB_REPOSITORY}" --body "${PROVIDER}"
gh variable set GCP_FIREBASE_DEPLOY_SERVICE_ACCOUNT --repo "${GITHUB_REPOSITORY}" --body "${SERVICE_ACCOUNT}"

printf 'Configured keyless Firebase deployment for %s\n' "${GITHUB_REPOSITORY}"
printf 'Workload identity provider: %s\n' "${PROVIDER}"
printf 'Service account: %s\n' "${SERVICE_ACCOUNT}"
