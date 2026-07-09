# Cloud device configuration

Yokai can save and restore the `devices` section of `~/.config/yokai/config.json`
with Firebase Authentication and Cloud Firestore. Other local settings, deploy
history, and the Hugging Face token are not synchronized.

## Architecture

```text
Yokai CLI
  ├─ Google desktop OAuth (openid + email only)
  ├─ Exchange Google ID token for a Firebase user session
  ├─ Argon2id passphrase derivation + AES-256-GCM encryption
  └─ Firestore REST request containing ciphertext
          │
          ▼
Firestore device_configs/{Firebase UID}
  └─ Security Rules allow only request.auth.uid == document UID
```

The Google account controls which Firestore document Firebase can access. A
separate encryption passphrase never leaves the machine and prevents Firebase,
project operators, or a database export from reading device hosts, SSH details,
or agent tokens. Losing the passphrase makes the cloud copy unrecoverable.

Firebase refresh credentials are stored locally at
`~/.config/yokai/cloud-auth.json` with mode `0600`. The initial Google grant only
requests `openid` and `email`; it does not grant Yokai access to Google Drive,
Gmail, or the user's Google Cloud resources. The Google OAuth refresh token and
desktop client secret are not persisted by Yokai.

## Why Firebase Spark

The no-cost Spark plan requires no billing account or payment method. Google
currently includes social sign-in and a Firestore quota of 1 GiB stored data,
50,000 document reads/day, 20,000 writes/day, 20,000 deletes/day, and 10 GiB
outbound transfer/month. If a Spark quota is exhausted, the product is paused
instead of generating an overage bill.

Those quotas are shared by the whole project, so an authenticated user can still
exhaust the free allowance and temporarily deny service to everyone. Keep the
OAuth app in **Testing** with an explicit test-user allowlist until usage is
understood. Before making sign-in public, place a rate-limited service in front
of Firestore or accept this availability tradeoff.

This deliberately avoids Cloud Run, Cloud Build, Artifact Registry, Secret
Manager, and a privileged runtime service account. It also leaves only one
deployed security boundary: Firestore Security Rules.

Current references:

- [Firebase pricing plans](https://firebase.google.com/docs/projects/billing/firebase-pricing-plans)
- [Firestore free quota](https://firebase.google.com/docs/firestore/pricing)
- [Firebase API keys are public identifiers](https://firebase.google.com/docs/projects/api-keys)
- [Google OAuth for desktop apps](https://developers.google.com/identity/protocols/oauth2/native-app)

## Environment isolation

Yokai uses three persistent, unbilled Firebase Spark projects. Data, users,
quotas, API keys, deployment identities, and security-rule releases are isolated
between them.

| Git branch | GitHub environment | Firebase project | Purpose |
|---|---|---|---|
| `develop` | `firebase-development` | `yokai-config-dev-260709-5820` | Integrated development |
| `staging` | `firebase-staging` | `yokai-config-stg-260709-5820` | Production-like verification |
| `main` | `firebase-production` | `yokai-config-260709-5820` | User data and releases |

Feature pull requests target `develop`. Promotion pull requests move the same
commit from `develop` to `staging`, then from `staging` to `main`. Every pull
request runs the Firestore emulator tests. A merge to a long-lived branch deploys
only to its matching Firebase project; production deployment and releases also
wait for approval from `spencerbull`.

## Set up the Google backends

Prerequisites:

1. Install and authenticate the Google Cloud and Firebase CLIs.
2. Keep all three projects disconnected from billing to stay on Firebase's Spark
   plan.
3. Run the setup script for each environment from the repository root:

```bash
CREATE_PROJECT=1 DEPLOY_STAGE=development \
  GCP_PROJECT_ID=yokai-config-dev-260709-5820 \
  ./deploy/gcp/setup-firebase.sh

CREATE_PROJECT=1 DEPLOY_STAGE=staging \
  GCP_PROJECT_ID=yokai-config-stg-260709-5820 \
  ./deploy/gcp/setup-firebase.sh

DEPLOY_STAGE=production \
  GCP_PROJECT_ID=yokai-config-260709-5820 \
  ./deploy/gcp/setup-firebase.sh
```

The script creates a project when explicitly allowed, verifies billing is off,
adds Firebase, creates the free Firestore database in
`us-central1` with delete protection, creates a Firebase Web App, and deploys
`firestore.rules`. It prints the public app ID and API key.

### Continuous deployment

Configure keyless GitHub Actions access for each project:

```bash
DEPLOY_STAGE=development GCP_PROJECT_ID=yokai-config-dev-260709-5820 \
  ./deploy/gcp/setup-github-actions.sh
DEPLOY_STAGE=staging GCP_PROJECT_ID=yokai-config-stg-260709-5820 \
  ./deploy/gcp/setup-github-actions.sh
DEPLOY_STAGE=production GCP_PROJECT_ID=yokai-config-260709-5820 \
  ./deploy/gcp/setup-github-actions.sh

./deploy/github/setup-branch-governance.sh
```

Each environment receives a dedicated service account with a narrow custom
rules-deployer role and its own workload identity provider. Trust is restricted
to this repository's immutable GitHub ID, the environment's exact branch, the
matching GitHub environment, and this deployment workflow. Public client values
and deployment coordinates are environment-scoped GitHub variables. No
service-account key or GitHub secret is created.

The branch-governance script blocks direct pushes, force pushes, and deletion of
`develop`, `staging`, and `main`; requires the complete CI suite and resolved
review threads; and requires code-owner approval from `spencerbull`. Spencer can
bypass approval only while merging a pull request, which permits a sole
maintainer to merge their own reviewed work without allowing direct pushes.

Next, in **Google Auth Platform** for each project:

1. Configure the branding/audience and add test users while the app is in testing.
2. Enable Google as a Firebase Authentication sign-in provider.
3. Create an OAuth client with application type **Desktop app**.
4. Configure release builds with the OAuth client values:

```bash
gh variable set YOKAI_GOOGLE_CLIENT_ID --env firebase-production --body "000000000000-example.apps.googleusercontent.com"
gh variable set YOKAI_GOOGLE_CLIENT_SECRET --env firebase-production --body "desktop-client-secret"
```

Desktop OAuth client secrets identify an installed app but cannot be kept
confidential. Yokai uses the secret only for the authorization-code exchange and
does not save it after login.

## Sign in and use

Official releases include the public Firebase project configuration and the
desktop OAuth client configured by the release workflow, so sign-in is normally:

```bash
yokai cloud login
```

For a self-hosted backend or a locally built binary, supply the configuration
explicitly:

```bash
yokai cloud login \
  --project-id "your-project-id" \
  --api-key "firebase-public-api-key" \
  --client-id "000000000000-example.apps.googleusercontent.com" \
  --client-secret "desktop-client-secret"

# Encrypt and upload only device records
yokai cloud save

# Restore device records; existing config is backed up first
yokai cloud load

# Check or remove local login state
yokai cloud status
yokai cloud logout

# Permanently delete the server-side ciphertext
yokai cloud delete --yes
```

Interactive commands read the encryption passphrase without echo. `cloud save`
asks for it twice before replacing the existing cloud snapshot, reducing the
chance that a typo makes the last usable snapshot inaccessible. Use a long,
unique passphrase and keep it in a password manager. Automation can set
`YOKAI_CLOUD_PASSPHRASE` to bypass the prompt, but a process environment may be
visible to other processes owned by the same operating-system user.

`yokai cloud load` replaces only devices and writes a timestamped local backup.
If the local daemon is running, the CLI asks it to reload the new device list.

## Security operations

- Deploy the checked-in Firestore rules before enabling sign-in.
- Keep the Firebase-provisioned API key restricted to Firebase APIs. It is a
  public project identifier, not authorization; Security Rules authorize data.
- Do not grant application users IAM roles in the Google Cloud project.
- Do not enable phone authentication, Cloud Functions, or other paid products.
- Review Firebase Authentication users and Firestore usage periodically.
- Treat an unexpected usage spike as abuse: disable the Google sign-in provider
  or remove untrusted test users until the source is understood.
- Rotate the desktop OAuth client if it is abused. Existing Firebase refresh
  sessions can be revoked from Firebase Authentication.
