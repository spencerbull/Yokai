#!/usr/bin/env bash
set -euo pipefail

: "${GCP_PROJECT_ID:?Set GCP_PROJECT_ID to the Firebase project ID}"

GITHUB_REPOSITORY="${GITHUB_REPOSITORY:-spencerbull/Yokai}"
POOL_ID="${WIF_POOL_ID:-github-actions}"
PROVIDER_ID="${WIF_PROVIDER_ID:-yokai-repository}"
SERVICE_ACCOUNT_ID="${DEPLOY_SERVICE_ACCOUNT_ID:-github-firestore-deployer}"
SERVICE_ACCOUNT="${SERVICE_ACCOUNT_ID}@${GCP_PROJECT_ID}.iam.gserviceaccount.com"
CUSTOM_ROLE_ID="${DEPLOY_CUSTOM_ROLE_ID:-firebaseRulesDeployer}"
CUSTOM_ROLE="projects/${GCP_PROJECT_ID}/roles/${CUSTOM_ROLE_ID}"
DEPLOY_ENVIRONMENT="firebase-production"

command -v gcloud >/dev/null || { printf 'gcloud is required\n' >&2; exit 1; }
command -v gh >/dev/null || { printf 'gh is required\n' >&2; exit 1; }

PROJECT_NUMBER="$(gcloud projects describe "${GCP_PROJECT_ID}" --format='value(projectNumber)')"
GITHUB_REPOSITORY_ID="$(gh api "repos/${GITHUB_REPOSITORY}" --jq '.id')"
WORKFLOW_REF="${GITHUB_REPOSITORY}/.github/workflows/deploy-firebase.yml@refs/heads/main"
ATTRIBUTE_MAPPING="google.subject=assertion.sub,attribute.repository_id=assertion.repository_id,attribute.ref=assertion.ref,attribute.environment=assertion.environment,attribute.workflow_ref=assertion.workflow_ref"
ATTRIBUTE_CONDITION="assertion.repository_id=='${GITHUB_REPOSITORY_ID}' && assertion.ref=='refs/heads/main' && assertion.environment=='${DEPLOY_ENVIRONMENT}' && assertion.workflow_ref=='${WORKFLOW_REF}'"

gh api -X PUT "repos/${GITHUB_REPOSITORY}/environments/${DEPLOY_ENVIRONMENT}" --input - >/dev/null <<'JSON'
{"deployment_branch_policy":{"protected_branches":false,"custom_branch_policies":true}}
JSON
if [[ "$(gh api "repos/${GITHUB_REPOSITORY}/environments/${DEPLOY_ENVIRONMENT}/deployment-branch-policies" --jq 'any(.branch_policies[]; .name == "main" and .type == "branch")')" != "true" ]]; then
  gh api -X POST "repos/${GITHUB_REPOSITORY}/environments/${DEPLOY_ENVIRONMENT}/deployment-branch-policies" --input - >/dev/null <<'JSON'
{"name":"main"}
JSON
fi

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
    --attribute-mapping="${ATTRIBUTE_MAPPING}" \
    --attribute-condition="${ATTRIBUTE_CONDITION}"
fi

gcloud iam workload-identity-pools providers update-oidc "${PROVIDER_ID}" \
  --workload-identity-pool="${POOL_ID}" \
  --location=global \
  --project "${GCP_PROJECT_ID}" \
  --attribute-mapping="${ATTRIBUTE_MAPPING}" \
  --attribute-condition="${ATTRIBUTE_CONDITION}" >/dev/null

if ! gcloud iam service-accounts describe "${SERVICE_ACCOUNT}" --project "${GCP_PROJECT_ID}" >/dev/null 2>&1; then
  gcloud iam service-accounts create "${SERVICE_ACCOUNT_ID}" \
    --project "${GCP_PROJECT_ID}" \
    --display-name="GitHub Firestore Rules Deployer"
fi

ROLE_PERMISSIONS="firebase.projects.get,firebaserules.releases.create,firebaserules.releases.get,firebaserules.releases.list,firebaserules.releases.update,firebaserules.rulesets.create,firebaserules.rulesets.get,firebaserules.rulesets.list,firebaserules.rulesets.test,resourcemanager.projects.get,serviceusage.services.get,serviceusage.services.use"
if gcloud iam roles describe "${CUSTOM_ROLE_ID}" --project "${GCP_PROJECT_ID}" >/dev/null 2>&1; then
  gcloud iam roles update "${CUSTOM_ROLE_ID}" --project "${GCP_PROJECT_ID}" --permissions="${ROLE_PERMISSIONS}" --stage=GA >/dev/null
else
  gcloud iam roles create "${CUSTOM_ROLE_ID}" --project "${GCP_PROJECT_ID}" --title="Firebase Rules Deployer" --description="Deploy Firestore Security Rules from GitHub Actions" --permissions="${ROLE_PERMISSIONS}" --stage=GA >/dev/null
fi
gcloud projects add-iam-policy-binding "${GCP_PROJECT_ID}" \
  --member="serviceAccount:${SERVICE_ACCOUNT}" \
  --role="${CUSTOM_ROLE}" \
  --condition=None >/dev/null

for legacy_role in roles/firebaserules.admin roles/firebase.viewer roles/serviceusage.serviceUsageConsumer; do
  gcloud projects remove-iam-policy-binding "${GCP_PROJECT_ID}" \
    --member="serviceAccount:${SERVICE_ACCOUNT}" \
    --role="${legacy_role}" \
    --condition=None >/dev/null 2>&1 || true
done

gcloud iam service-accounts add-iam-policy-binding "${SERVICE_ACCOUNT}" \
  --project "${GCP_PROJECT_ID}" \
  --role="roles/iam.workloadIdentityUser" \
  --member="principalSet://iam.googleapis.com/projects/${PROJECT_NUMBER}/locations/global/workloadIdentityPools/${POOL_ID}/attribute.repository_id/${GITHUB_REPOSITORY_ID}" >/dev/null

gcloud iam service-accounts remove-iam-policy-binding "${SERVICE_ACCOUNT}" \
  --project "${GCP_PROJECT_ID}" \
  --role="roles/iam.workloadIdentityUser" \
  --member="principalSet://iam.googleapis.com/projects/${PROJECT_NUMBER}/locations/global/workloadIdentityPools/${POOL_ID}/attribute.repository/${GITHUB_REPOSITORY}" >/dev/null 2>&1 || true

PROVIDER="projects/${PROJECT_NUMBER}/locations/global/workloadIdentityPools/${POOL_ID}/providers/${PROVIDER_ID}"
gh variable set FIREBASE_PROJECT_ID --repo "${GITHUB_REPOSITORY}" --body "${GCP_PROJECT_ID}"
gh variable set GCP_WORKLOAD_IDENTITY_PROVIDER --repo "${GITHUB_REPOSITORY}" --body "${PROVIDER}"
gh variable set GCP_FIREBASE_DEPLOY_SERVICE_ACCOUNT --repo "${GITHUB_REPOSITORY}" --body "${SERVICE_ACCOUNT}"

printf 'Configured keyless Firebase deployment for %s\n' "${GITHUB_REPOSITORY}"
printf 'Workload identity provider: %s\n' "${PROVIDER}"
printf 'Service account: %s\n' "${SERVICE_ACCOUNT}"
