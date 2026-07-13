#!/usr/bin/env bash
set -euo pipefail

: "${GCP_PROJECT_ID:?Set GCP_PROJECT_ID to the Firebase project ID}"
: "${DEPLOY_STAGE:?Set DEPLOY_STAGE to development, staging, or production}"

case "${DEPLOY_STAGE}" in
  development)
    DEPLOY_ENVIRONMENT="firebase-development"
    DEPLOY_BRANCH="develop"
    ;;
  staging)
    DEPLOY_ENVIRONMENT="firebase-staging"
    DEPLOY_BRANCH="staging"
    ;;
  production)
    DEPLOY_ENVIRONMENT="firebase-production"
    DEPLOY_BRANCH="main"
    ;;
  *) printf 'DEPLOY_STAGE must be development, staging, or production\n' >&2; exit 1 ;;
esac

if [[ "${DEPLOY_STAGE}" == "production" ]]; then
  FIREBASE_APP_DISPLAY_NAME="${FIREBASE_APP_DISPLAY_NAME:-Yokai CLI}"
else
  FIREBASE_APP_DISPLAY_NAME="${FIREBASE_APP_DISPLAY_NAME:-Yokai CLI ${DEPLOY_STAGE}}"
fi

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
# shellcheck source=setup-common.sh
source "${REPO_ROOT}/deploy/gcp/setup-common.sh"

GITHUB_REPOSITORY="${GITHUB_REPOSITORY:-spencerbull/Yokai}"
GITHUB_PRODUCTION_REVIEWER="${GITHUB_PRODUCTION_REVIEWER:-spencerbull}"
POOL_ID="${WIF_POOL_ID:-github-actions}"
PROVIDER_ID="${WIF_PROVIDER_ID:-yokai-repository}"
SERVICE_ACCOUNT_ID="${DEPLOY_SERVICE_ACCOUNT_ID:-github-firestore-deployer}"
SERVICE_ACCOUNT="${SERVICE_ACCOUNT_ID}@${GCP_PROJECT_ID}.iam.gserviceaccount.com"
CUSTOM_ROLE_ID="${DEPLOY_CUSTOM_ROLE_ID:-firebaseRulesDeployer}"
CUSTOM_ROLE="projects/${GCP_PROJECT_ID}/roles/${CUSTOM_ROLE_ID}"

command -v gcloud >/dev/null || { printf 'gcloud is required\n' >&2; exit 1; }
command -v gh >/dev/null || { printf 'gh is required\n' >&2; exit 1; }
command -v firebase >/dev/null || { printf 'firebase-tools is required\n' >&2; exit 1; }
command -v jq >/dev/null || { printf 'jq is required\n' >&2; exit 1; }

PROJECT_NUMBER="$(gcloud projects describe "${GCP_PROJECT_ID}" --format='value(projectNumber)')"
GITHUB_REPOSITORY_ID="$(gh api "repos/${GITHUB_REPOSITORY}" --jq '.id')"
WORKFLOW_REF="${GITHUB_REPOSITORY}/.github/workflows/deploy-firebase.yml@refs/heads/${DEPLOY_BRANCH}"
ATTRIBUTE_MAPPING="google.subject=assertion.sub,attribute.repository_id=assertion.repository_id,attribute.ref=assertion.ref,attribute.environment=assertion.environment,attribute.workflow_ref=assertion.workflow_ref"
ATTRIBUTE_CONDITION="assertion.repository_id=='${GITHUB_REPOSITORY_ID}' && assertion.ref=='refs/heads/${DEPLOY_BRANCH}' && assertion.environment=='${DEPLOY_ENVIRONMENT}' && assertion.workflow_ref=='${WORKFLOW_REF}'"

if [[ "${DEPLOY_STAGE}" == "production" ]]; then
  REVIEWER_ID="$(gh api "users/${GITHUB_PRODUCTION_REVIEWER}" --jq '.id')"
  ENVIRONMENT_CONFIG="$(jq -n --argjson reviewer "${REVIEWER_ID}" '{wait_timer: 0, prevent_self_review: false, can_admins_bypass: false, reviewers: [{type: "User", id: $reviewer}], deployment_branch_policy: {protected_branches: false, custom_branch_policies: true}}')"
else
  ENVIRONMENT_CONFIG='{"wait_timer":0,"prevent_self_review":false,"reviewers":[],"deployment_branch_policy":{"protected_branches":false,"custom_branch_policies":true}}'
fi
gh api -X PUT "repos/${GITHUB_REPOSITORY}/environments/${DEPLOY_ENVIRONMENT}" --input - >/dev/null <<<"${ENVIRONMENT_CONFIG}"

POLICIES="$(gh api "repos/${GITHUB_REPOSITORY}/environments/${DEPLOY_ENVIRONMENT}/deployment-branch-policies")"
while IFS= read -r policy_id; do
  [[ -z "${policy_id}" ]] && continue
  gh api -X DELETE "repos/${GITHUB_REPOSITORY}/environments/${DEPLOY_ENVIRONMENT}/deployment-branch-policies/${policy_id}" >/dev/null
done < <(jq -r --arg branch "${DEPLOY_BRANCH}" '.branch_policies[] | select(.name != $branch or .type != "branch") | .id' <<<"${POLICIES}")
if ! jq -e --arg branch "${DEPLOY_BRANCH}" '.branch_policies[] | select(.name == $branch and .type == "branch")' <<<"${POLICIES}" >/dev/null; then
  jq -n --arg branch "${DEPLOY_BRANCH}" '{name: $branch}' | \
    gh api -X POST "repos/${GITHUB_REPOSITORY}/environments/${DEPLOY_ENVIRONMENT}/deployment-branch-policies" --input - >/dev/null
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
    --display-name="${GITHUB_REPOSITORY} ${DEPLOY_STAGE}" \
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
    --display-name="GitHub Firestore Rules Deployer (${DEPLOY_STAGE})"
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

APPS="$(firebase apps:list WEB --project "${GCP_PROJECT_ID}" --json)"
APP_MATCH_COUNT="$(firebase_app_match_count "${APPS}" "${FIREBASE_APP_DISPLAY_NAME}")"
if [[ "${APP_MATCH_COUNT}" != "1" ]]; then
  printf 'expected exactly one Firebase web app named %q, found %s; run setup-firebase.sh first\n' "${FIREBASE_APP_DISPLAY_NAME}" "${APP_MATCH_COUNT}" >&2
  exit 1
fi
APP_ID="$(firebase_app_id "${APPS}" "${FIREBASE_APP_DISPLAY_NAME}")"
SDK_CONFIG="$(firebase apps:sdkconfig WEB "${APP_ID}" --project "${GCP_PROJECT_ID}" --json)"
API_KEY="$(jq -r '.result.sdkConfig.apiKey' <<<"${SDK_CONFIG}")"
PROVIDER="projects/${PROJECT_NUMBER}/locations/global/workloadIdentityPools/${POOL_ID}/providers/${PROVIDER_ID}"
gh variable set FIREBASE_PROJECT_ID --repo "${GITHUB_REPOSITORY}" --env "${DEPLOY_ENVIRONMENT}" --body "${GCP_PROJECT_ID}"
gh variable set FIREBASE_APP_ID --repo "${GITHUB_REPOSITORY}" --env "${DEPLOY_ENVIRONMENT}" --body "${APP_ID}"
gh variable set FIREBASE_API_KEY --repo "${GITHUB_REPOSITORY}" --env "${DEPLOY_ENVIRONMENT}" --body "${API_KEY}"
gh variable set GCP_WORKLOAD_IDENTITY_PROVIDER --repo "${GITHUB_REPOSITORY}" --env "${DEPLOY_ENVIRONMENT}" --body "${PROVIDER}"
gh variable set GCP_FIREBASE_DEPLOY_SERVICE_ACCOUNT --repo "${GITHUB_REPOSITORY}" --env "${DEPLOY_ENVIRONMENT}" --body "${SERVICE_ACCOUNT}"

printf 'Configured keyless %s Firebase deployment for %s\n' "${DEPLOY_STAGE}" "${GITHUB_REPOSITORY}"
printf 'GitHub environment: %s (%s only)\n' "${DEPLOY_ENVIRONMENT}" "${DEPLOY_BRANCH}"
printf 'Workload identity provider: %s\n' "${PROVIDER}"
printf 'Service account: %s\n' "${SERVICE_ACCOUNT}"
