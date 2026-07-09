# Three-environment Firebase rollout

## Goal and done criteria

- [x] Create long-lived `develop`, `staging`, and `main` branches.
- [x] Make `develop` the default branch and retarget feature PR #79.
- [x] Require PRs, passing CI, resolved threads, and `@spencerbull` code-owner approval on all long-lived branches.
- [x] Block direct pushes, force pushes, and deletion; allow Spencer to bypass approval only while merging a PR.
- [x] Map branches to `firebase-development`, `firebase-staging`, and `firebase-production` GitHub Environments.
- [x] Move production deployment values to the production GitHub Environment and remove repository-scoped deployment values.
- [x] Verify production is unbilled, delete-protected, keyless, least privilege, and restricted to the exact repository/branch/environment/workflow OIDC claims.
- [ ] Obtain two additional Google Cloud project slots.
- [ ] Create isolated unbilled Firebase development and staging projects.
- [ ] Create per-environment web apps, Firestore databases/rules, WIF providers, deploy service accounts, and environment-scoped GitHub values.
- [ ] Configure and verify Google sign-in/OAuth for each environment.
- [ ] Exercise development and staging deployments end to end before promoting to production.

## Streams and worktrees

| Stream | Branch | Worktree | Scope |
|---|---|---|---|
| Cloud config feature | `agent/cloud-config-sync` | `/tmp/Yokai-cloud-config` | PR #79 implementation, workflows, scripts, docs |
| Code-owner bootstrap | merged PR #81 | `/tmp/Yokai-governance-bootstrap` | `.github/CODEOWNERS` only |

## Allowed and forbidden actions

- Allowed: create the two named unbilled Firebase projects, configure Firebase/WIF/GitHub Environments, submit the disclosed Google project-quota request, run tests, and update PR #79.
- Forbidden without a new explicit approval: enable billing, delete or repurpose unrelated Google Cloud projects, deploy application code outside the three named environments, rotate unrelated credentials, or mutate customer data.

## Required verification

- [x] Go race tests for changed packages.
- [x] golangci-lint.
- [x] Firestore Emulator Suite rules tests (5/5).
- [x] OpenTUI tests/build/compile.
- [x] GoReleaser snapshot and archive verification.
- [x] Actionlint, ShellCheck, Bash syntax, and `git diff --check`.
- [x] PR #79 GitHub checks (10/10 passing after rebase).
- [x] Live branch ruleset, CODEOWNERS, environment branch policies, production reviewer, IAM, WIF, billing, delete protection, and service-account-key audit.
- [ ] Live development deployment.
- [ ] Live staging deployment.
- [ ] Environment isolation audit proving no project ID, API key, WIF provider, or deploy service account is shared.

## Open checkpoints

- Google account project-count quota is exhausted. The quota form is paused while 17 user-approved legacy projects are audited for deletion. Google documents that soft-deleted projects may continue counting until final deletion, so retain the quota-request fallback.
- Google may require interactive sign-in, passkey verification, or CAPTCHA completion by the user.
- OAuth client creation is a persistent credential action and requires action-time confirmation in the Google Cloud console.

## Legacy project deletion audit

- User-authorized names map to: `tidy-campaign-774`, `promising-howl-798`, `notspotify-162300`, `bullrhinobot`, `cs-bot-168319`, `dellctovoice`, `dell-assistant-fda98`, `slipspace-bullapse`, `contextual-coach`, `dell-contextual-tasks-79a3b`, `dell-contextual-tasks-5b3dd`, `dell-contextual-tasks`, `dellcontextualtasks`, `contextual-tasks`, `contextual-tasks-1565883164223`, `vzre-256101`, and `spaceos`.
- [x] Complete read-only resource/IAM audit for all 17 projects using three independent read-only workers.
- [x] Identify and report projects with unexpected ownership or active infrastructure before deletion.
- [ ] Delete the confirmed exact project IDs and record operation results.
- [ ] Re-test creation of the development and staging project IDs.

### Deletion findings

- High data-loss risk: `notspotify-162300` has two external editors, a Cloud Source repository, source files, and about 2.66 GiB of container artifacts; `bullrhinobot` has two Cloud Source repositories and about 361 MiB of container artifacts; `slipspace-bullapse` has three active Cloud Functions, Hosting deployment history, and a Firestore database.
- External-access risk: `dellctovoice` has three non-Spencer human editors, although no deployed resources were discovered.
- Residual/uncertain legacy state: `tidy-campaign-774`, `promising-howl-798`, `dell-assistant-fda98`, and `vzre-256101` have App Engine or initialized Firebase surfaces that cannot be completely enumerated while billing is disabled.
- Low observed risk: the remaining audited projects have no billing, no non-Spencer human principals, and no observed deployed code or non-empty databases/storage at the inspected surfaces.
- [ ] Receive final confirmation to delete all 17 exact project IDs despite the disclosed active source, function, database, artifact, and collaborator impact.

## Completed evidence

- PR #81 merged `.github/CODEOWNERS` with `@spencerbull` as sole owner.
- Repository ruleset `18739747` protects `develop`, `staging`, and `main`.
- GitHub Environments are branch-restricted: development→`develop`, staging→`staging`, production→`main`.
- Production environment contains its Firebase project/app/API key and WIF/service-account coordinates; development and staging are intentionally empty until their projects exist.
- PR #79 head `386afca` targets `develop` and has ten successful checks.
