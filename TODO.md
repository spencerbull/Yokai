# Three-environment Firebase rollout

## Firestore validation hardening (2026-07-20)

### Goal and done criteria

- [x] Reconcile current `main` into the cloud-sync development line without losing the Tailscale documentation or production workflow safeguards.
- [x] Make Firestore envelope validation match the Go encryption format exactly.
- [x] Cover valid create/read/update/delete plus malformed, unauthorized, query/list, type, length, encoding, and size rejection paths in the emulator.
- [x] Pass focused cloud/rules tests, full repository checks, workflow/static validation, and independent security review.
- [x] Push `sbull-agent/firestore-hardening` and open draft PR #88 targeting `develop`; do not merge or promote it in this loop.

### Stream

| Branch | Worktree | Base | Scope |
|---|---|---|---|
| `sbull-agent/firestore-hardening` | `/Users/spencerbull/src/github.com/spencerbull/Yokai-firestore-hardening-develop` | `origin/develop` + current `origin/main` | rules, emulator tests, branch reconciliation, CI/release preservation |

### Allowed and forbidden actions

- Allowed: isolated worktree/branch changes, local emulators, tests, static checks, push, and a PR targeting `develop`.
- Forbidden without a new explicit approval: merge or promote the PR, approve `firebase-production`, deploy production rules, change secrets/billing/IAM, or touch real user data.

### Required gates

- [x] Firestore emulator validity/invalidity suite (55/55).
- [x] Focused Go race tests and vet for cloud/config/daemon ownership.
- [x] Full CI-equivalent build, lint, TUI, and release-package checks.
- [x] Action/workflow contract checks and clean diff.
- [x] Independent security and branch-reconciliation reviews resolved.
- [x] GitHub push and PR checks green (runs `29801252009` and `29801264647`).

### Current status

- [x] Persistent isolated worktree created from current `origin/develop` (`162b476`).
- [x] Merge current `origin/main` (`0f170d69`) and resolve the CI workflow intentionally.
- [x] Implement and locally validate the hardening diff.
- [x] Preserve the superseded staging-based work as `sbull-agent/firestore-hardening-staging-scratch` until the development PR lands.
- [x] Resolve final-review findings for canonical Base64, method-specific conflict errors, and missing Firestore update times.

### Open checkpoints

- Production remains a human gate even after staging validation passes.
- The production environment variable names and branch policy exist; values are not considered proven until the gated production workflows validate them.

## Goal and done criteria

- [x] Create long-lived `develop`, `staging`, and `main` branches.
- [x] Retarget feature PR #79 to `develop`; `main` remains the public default branch.
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
- [x] Exercise development and staging deployments end to end before promoting to production.

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
- [x] GitHub Actions deployment from `develop` to development after PR #79 reaches `develop` (run `29292844587`).
- [x] GitHub Actions deployment from `staging` to staging after the promotion PR reaches `staging` (run `29293130769`).

## Open checkpoints

- Google approved enough project-count quota after the user submitted the request; both target projects were created successfully.
- Development and staging exact-claim WIF deployments have passed through GitHub Actions; production promotion remains blocked on its separate release-readiness gate.
- All three OAuth audiences are restricted to Testing with `spencerbull2554@gmail.com` as the sole test user. Publishing production is an explicit release gate; development and staging stay unpublished.
- The user approved merging through staging after reviewing the cloud-config flow; production OAuth publication and promotion remain separate human gates.

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
- PR #79 merged to `develop`, and promotion PR #84 merged the resulting history from `develop` to `staging` after all required checks passed.
- Live re-audit confirms exact branch-to-environment policies, the active long-lived-branch ruleset, sole `@spencerbull` code ownership, no repository-scoped deployment variables, and the production environment's branch/reviewer controls.
- Production remains unbilled and Firestore-delete-protected, has no user-managed deploy-service-account keys, and uses exact repository/branch/environment/workflow OIDC claims with only the custom Firebase Rules deployer role.
- Development and staging are unbilled Firebase Spark projects with `us-central1` Firestore Native databases, delete protection, deployed rules, dedicated web apps, exact-claim WIF providers, custom rules-deployer roles, and no user-managed service-account keys.
- SHA-256 comparisons confirm that the three Firebase API keys are distinct without recording the raw values in the audit ledger.
- Firebase Authentication and Google sign-in are enabled in all three unbilled projects. Each GitHub Environment contains a distinct desktop OAuth client ID and secret whose project-number prefix matches its Firebase project.
- The development OAuth flow completed through Safari and the CLI successfully exercised login, encrypted save, status, load, server-side delete, and logout. The smoke-test record and local credentials were removed afterward.
- GitHub Actions run `29292844587` passed the emulator rules suite, exact-claim WIF authentication, and the Firestore rules deployment to `firebase-development` from `develop`.
- GitHub Actions run `29293130769` passed the emulator rules suite, exact-claim WIF authentication, and the Firestore rules deployment to `firebase-staging` from `staging`.
- The superseded development OAuth secret is disabled; the verified replacement remains enabled and stored only in the development GitHub Environment.
- Public Firebase keys in development, staging, and production allow only `identitytoolkit.googleapis.com` and `securetoken.googleapis.com`.
- Ruleset `18739747` has no bypass actors and enforces branch/CI/thread controls; approval ruleset `18892833` contains the sole PR-only maintainer bypass.
