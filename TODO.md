# Three-environment Firebase rollout

## Goal and done criteria

- [x] Create long-lived `develop`, `staging`, and `main` branches.
- [x] Make `develop` the default branch and retarget feature PR #79.
- [x] Require PRs, passing CI, resolved threads, and `@spencerbull` code-owner approval on all long-lived branches.
- [x] Block direct pushes, force pushes, and deletion; keep CI/thread gates non-bypassable and allow Spencer to bypass only the self-approval rule while merging a PR.
- [x] Map branches to `firebase-development`, `firebase-staging`, and `firebase-production` GitHub Environments.
- [x] Move production deployment values to the production GitHub Environment and remove repository-scoped deployment values.
- [x] Verify production is unbilled, delete-protected, keyless, least privilege, and restricted to the exact repository/branch/environment/workflow OIDC claims.
- [x] Obtain two additional Google Cloud project slots.
- [x] Create isolated unbilled Firebase development and staging projects.
- [x] Create per-environment web apps, Firestore databases/rules, WIF providers, deploy service accounts, and environment-scoped GitHub values.
- [x] Configure and verify Google sign-in/OAuth for each environment.
- [x] Complete the pre-merge cloud-config UX and destructive-action safety pass.
- [ ] Exercise development and staging deployments end to end before promoting to production.

## Streams and worktrees

| Stream | Branch | Worktree | Scope |
|---|---|---|---|
| Cloud config feature | `agent/cloud-config-sync` | `/tmp/Yokai-cloud-config` | PR #79 implementation, workflows, scripts, docs |
| Code-owner bootstrap | merged PR #81 | `/tmp/Yokai-governance-bootstrap` | `.github/CODEOWNERS` only |

## Allowed and forbidden actions

- Allowed: create the two named unbilled Firebase projects, configure Firebase/WIF/GitHub Environments, submit the disclosed Google project-quota request, delete the 17 exact legacy project IDs authorized below, run tests, and update PR #79.
- Forbidden without a new explicit approval: enable billing, delete or repurpose any other Google Cloud project, deploy application code outside the three named environments, rotate unrelated credentials, or mutate customer data.

## Required verification

- [x] Go race tests for changed packages.
- [x] golangci-lint.
- [x] Firestore Emulator Suite rules tests (5/5).
- [x] OpenTUI tests/build/compile.
- [x] GoReleaser snapshot and archive verification.
- [x] Actionlint, ShellCheck, Bash syntax, and `git diff --check`.
- [x] PR #79 GitHub checks (10/10 passing after rebase).
- [x] Live branch ruleset, CODEOWNERS, environment branch policies, production reviewer, IAM, WIF, billing, delete protection, and service-account-key audit.
- [x] Live development rules deployment from the checked-in configuration.
- [x] Live staging rules deployment from the checked-in configuration.
- [x] Environment isolation audit proving no project ID, API key, WIF provider, or deploy service account is shared.
- [x] OAuth isolation audit proving each environment has its own desktop client ID/secret and enabled Google provider.
- [x] Live development login, encrypted save, status, load, server-side delete, and local logout smoke test.
- [x] UX regression tests prove every cloud subcommand's help exits before actions and empty device lists are blocked by default.
- [x] Focused cloud CLI/sync race tests, golangci-lint, and all OpenTUI tests pass after the UX pass.
- [x] Independent review of the final UX/safety diff is resolved or documented.
- [x] Three independent PR review passes covering security, Actions/governance, and documentation/tests; verified actionable findings were fixed.
- [x] Live Firebase API keys restricted to Identity Toolkit and Secure Token in all three projects, with allowed-API smoke responses verified in development.
- [x] Live rulesets split so CI/thread/direct-push controls have no bypass; production environment admin bypass disabled.
- [ ] GitHub Actions deployment from `develop` to development after PR #79 reaches `develop`.
- [ ] GitHub Actions deployment from `staging` to staging after the promotion PR reaches `staging`.

## Open checkpoints

- Google approved enough project-count quota after the user submitted the request; both target projects were created successfully.
- PR #79 must pass review and reach `develop` before the exact-claim WIF and branch-restricted development deployment can be exercised through GitHub Actions.
- All three OAuth audiences are restricted to Testing with `spencerbull2554@gmail.com` as the sole test user. Publishing production is an explicit release gate; development and staging stay unpublished.
- User review of the improved cloud-config flow is now a checkpoint before production OAuth changes or PR merge approval.

## Legacy project deletion audit

- User-authorized names map to: `tidy-campaign-774`, `promising-howl-798`, `notspotify-162300`, `bullrhinobot`, `cs-bot-168319`, `dellctovoice`, `dell-assistant-fda98`, `slipspace-bullapse`, `contextual-coach`, `dell-contextual-tasks-79a3b`, `dell-contextual-tasks-5b3dd`, `dell-contextual-tasks`, `dellcontextualtasks`, `contextual-tasks`, `contextual-tasks-1565883164223`, `vzre-256101`, and `spaceos`.
- [x] Complete read-only resource/IAM audit for all 17 projects using three independent read-only workers.
- [x] Identify and report projects with unexpected ownership or active infrastructure before deletion.
- [x] Delete the confirmed exact project IDs and verify all 17 entered `DELETE_REQUESTED`.
- [x] Re-test creation of `yokai-config-dev-260709-5820` and `yokai-config-stg-260709-5820`; both were created after the quota request was approved.

### Deletion findings

- High data-loss risk: `notspotify-162300` has two external editors, a Cloud Source repository, source files, and about 2.66 GiB of container artifacts; `bullrhinobot` has two Cloud Source repositories and about 361 MiB of container artifacts; `slipspace-bullapse` has three active Cloud Functions, Hosting deployment history, and a Firestore database.
- External-access risk: `dellctovoice` has three non-Spencer human editors, although no deployed resources were discovered.
- Residual/uncertain legacy state: `tidy-campaign-774`, `promising-howl-798`, `dell-assistant-fda98`, and `vzre-256101` have App Engine or initialized Firebase surfaces that cannot be completely enumerated while billing is disabled.
- Low observed risk: the remaining audited projects have no billing, no non-Spencer human principals, and no observed deployed code or non-empty databases/storage at the inspected surfaces.
- [x] Receive continuation authorization after disclosing the active source, function, database, artifact, and collaborator impact.
- [x] Delete the Dialogflow agents and stale Resource Manager liens that initially blocked `dellctovoice` and `dell-assistant-fda98` deletion.

## Completed evidence

- PR #81 merged `.github/CODEOWNERS` with `@spencerbull` as sole owner.
- Repository ruleset `18739747` protects `develop`, `staging`, and `main`.
- GitHub Environments are branch-restricted: development→`develop`, staging→`staging`, production→`main`.
- All three GitHub Environments contain their own Firebase project/app/API key and WIF/service-account coordinates.
- All 17 authorized legacy project IDs report lifecycle state `DELETE_REQUESTED`.
- PR #79 targets `develop`, is mergeable, and its latest complete CI run has ten successful checks.
- Live re-audit confirms exact branch-to-environment policies, the active long-lived-branch ruleset, sole `@spencerbull` code ownership, no repository-scoped deployment variables, and the production environment's branch/reviewer controls.
- Production remains unbilled and Firestore-delete-protected, has no user-managed deploy-service-account keys, and uses exact repository/branch/environment/workflow OIDC claims with only the custom Firebase Rules deployer role.
- Development and staging are unbilled Firebase Spark projects with `us-central1` Firestore Native databases, delete protection, deployed rules, dedicated web apps, exact-claim WIF providers, custom rules-deployer roles, and no user-managed service-account keys.
- SHA-256 comparisons confirm that the three Firebase API keys are distinct without recording the raw values in the audit ledger.
- Firebase Authentication and Google sign-in are enabled in all three unbilled projects. Each GitHub Environment contains a distinct desktop OAuth client ID and secret whose project-number prefix matches its Firebase project.
- The development OAuth flow completed through Safari and the CLI successfully exercised login, encrypted save, status, load, server-side delete, and logout. The smoke-test record and local credentials were removed afterward.
- The superseded development OAuth secret is disabled; the verified replacement remains enabled and stored only in the development GitHub Environment.
- Public Firebase keys in development, staging, and production allow only `identitytoolkit.googleapis.com` and `securetoken.googleapis.com`.
- Ruleset `18739747` has no bypass actors and enforces branch/CI/thread controls; approval ruleset `18892833` contains the sole PR-only maintainer bypass.
