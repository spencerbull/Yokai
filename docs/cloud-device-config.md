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

## Set up the Google backend

Prerequisites:

1. Install and authenticate the Google Cloud and Firebase CLIs.
2. Use a Google Cloud project without a billing account to stay on Firebase's
   Spark plan.
3. Run the setup script from the repository root:

```bash
export GCP_PROJECT_ID="your-project-id"
./deploy/gcp/setup-firebase.sh
```

The script adds Firebase, creates the single free Firestore database in
`us-central1`, creates a Firebase Web App if necessary, and deploys
`firestore.rules`. It prints the Firebase project configuration containing the
public API key.

### Continuous deployment

Configure keyless GitHub Actions access once:

```bash
export GCP_PROJECT_ID="your-project-id"
./deploy/gcp/setup-github-actions.sh
```

The setup creates a dedicated service account with a narrow custom rules-deployer
role and a workload identity provider restricted to this repository's immutable
GitHub ID, the `main` branch, the `firebase-production` environment, and this
specific deployment workflow. It also creates the required GitHub repository
variables. It creates no service-account key or GitHub secret. Pull requests run
the rules against the Firestore emulator; changes merged to `main` rerun those
tests and deploy the rules only after they pass.

Next, in **Google Auth Platform** for the same project:

1. Configure the branding/audience and add test users while the app is in testing.
2. Enable Google as a Firebase Authentication sign-in provider.
3. Create an OAuth client with application type **Desktop app**.
4. Configure release builds with the OAuth client values:

```bash
gh variable set YOKAI_GOOGLE_CLIENT_ID --body "000000000000-example.apps.googleusercontent.com"
gh variable set YOKAI_GOOGLE_CLIENT_SECRET --body "desktop-client-secret"
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
