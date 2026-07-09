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

The setup creates a dedicated service account, a workload identity provider
restricted to the `spencerbull/Yokai` repository's `main` branch, and the required
GitHub repository variables. It creates no service-account key or GitHub secret.
Pull requests run the rules against the Firestore emulator; changes merged to
`main` rerun those tests and deploy the rules only after they pass.

Next, in **Google Auth Platform** for the same project:

1. Configure the branding/audience and add test users while the app is in testing.
2. Enable Google as a Firebase Authentication sign-in provider.
3. Create an OAuth client with application type **Desktop app**.

Desktop OAuth client secrets identify an installed app but cannot be kept
confidential. Yokai uses the secret only for the authorization-code exchange and
does not save it after login.

## Sign in and use

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

Interactive commands read the encryption passphrase without echo. Use a long,
unique passphrase and keep it in a password manager. Automation can set
`YOKAI_CLOUD_PASSPHRASE`, but a process environment may be visible to other
processes owned by the same operating-system user.

`yokai cloud load` replaces only devices and writes a timestamped local backup.
If the local daemon is running, the CLI asks it to reload the new device list.

## Security operations

- Deploy the checked-in Firestore rules before enabling sign-in.
- Keep the Firebase-provisioned API key restricted to Firebase APIs. It is a
  public project identifier, not authorization; Security Rules authorize data.
- Do not grant application users IAM roles in the Google Cloud project.
- Do not enable phone authentication, Cloud Functions, or other paid products.
- Review Firebase Authentication users and Firestore usage periodically.
- Rotate the desktop OAuth client if it is abused. Existing Firebase refresh
  sessions can be revoked from Firebase Authentication.
