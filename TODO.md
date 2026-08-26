# Yokai PR #89 MiaAI Research and Merge Ledger

## Goal

Research current MiaAI Lab repositories for relevant runtime and recipe updates, reconcile justified changes into Yokai PR #89, obtain an independent Claude review, pass local and GitHub gates, merge the PR, and verify `main` remains healthy.

## Done criteria

- Current MiaAI Lab repositories and relevant changes are documented from primary sources.
- PR #89 contains the justified, scoped updates without regressing the validated Finn deployment contract.
- Focused local tests pass; full Docker-backed coverage passes in GitHub CI, with any local socket limitation documented.
- Independent Claude review findings are resolved or explicitly dispositioned.
- Required GitHub checks pass and review-thread gates are satisfied.
- PR #89 is merged into `main`.
- Post-merge `main` CI is green, or any merge-related failure is fixed and reverified.

## Scope and permissions

- Allowed: research public upstream repositories; edit this PR branch; create commits; push PR updates; resolve addressed review threads; merge PR #89; monitor and fix merge-related CI failures.
- Forbidden: production deployment; service/container changes on Finn or other devices; secrets, billing, or customer-data changes; destructive resets; modifying user-owned worktrees.

## Streams

- [x] Pin PR/local/remote state and preserve existing worktrees.
- [x] Research MiaAI Lab repositories and compare against the pinned Yokai BKC/runtime.
- [x] Implement only supported updates needed by PR #89.
- [ ] Run focused and full verification.
- [x] Run independent Claude review and address findings.
- [ ] Push and pass PR gates.
- [ ] Merge and monitor `main`.

## Branches and worktrees

- Primary: `/home/sbull/src/github.com/spencerbull/Yokai`, branch `feature/sglang-runtime-qwen38`; clean at start; local HEAD `5b322db`, initially two commits behind its remote.
- Preserved user worktree: `/home/sbull/worktrees/yokai-finn-qwen-observability`, branch `finn-qwen-observability`; do not modify.

## Required gates

- Focused Go/TUI tests for changed areas.
- Full repository test and lint/build/package-equivalent checks.
- Independent Claude review.
- GitHub required checks: `build (1.25)`, `lint`, `tui`, `release-package`, and `firestore-rules`.
- Required review/thread resolution per repository rulesets.
- Post-merge `main` CI success.

## Workers

- Backend: Herdr (`HERDR_ENV=1`).
- Claude reviewer: `mia_pr_claude` (`claude`), pane `w1H:p5`, current checkout/branch, read-only; upstream audit and final re-review complete with a CLEAN disposition after F1 remediation; cleanup owner is the orchestrator.

## Open questions and checkpoints

- MiaAI Lab's RTX PRO 6000 and DGX Spark Qwen repos moved to the BF16 `lm_head` target on Aug 24; PR commit `c4a5699` already incorporates that target and `14ba810` updates its catalog documentation.
- Audited upstream heads on Aug 26: RTX PRO 6000 Qwen `a0743929`, DGX Spark Qwen `751e29eb`, Ling 3.0 Flash `ca840cb8` (DSpark added by `fc7d4bc3`), two-node DeepSeek V4 Flash `70a7cc4b`, and one-node DeepSeek V4 Flash `fdcd538f`.
- No merged upstream Qwen change supersedes the Finn-validated immutable SGLang digest or its conservative 0.85 memory / 2048 prefill profile. The newer DFlash2 tag and idle-sleep flag remain mutable or unmerged upstream changes.
- MiaAI Lab's new Ling 3.0 Flash DGX Spark recipe and patch-heavy two-node DeepSeek V4 Flash work are separate hardware-validation streams, not scoped additions to Qwen PR #89.
- Claude proposed, then withdrew, `--sleep-on-idle` and `--enable-cache-report`. The former is an unmerged GB10-only measurement and the latter affects per-response cached-token usage rather than Prometheus export, so neither alters the Finn-validated command without a dedicated runtime canary.
- Claude F2 is deferred: the manual Extra Args path recognizes `sglang serve` but not a copied `python3 -m sglang.launch_server` command. All shipped BKCs are unaffected; supporting complete quoted/custom command lines needs a separate argument-parser change shared with vLLM.
- Claude F3 is accepted for this PR: the DSpark rollback digest is an amd64 child manifest validated on RTX PRO 6000, so it must not be repinned blindly. A future platform-gate improvement can fall back to local image inspection when registry metadata omits architecture.
- Claude F4 is intentionally unchanged: the pinned SGLang runtime exposes generation throughput but no prompt-throughput gauge. Yokai's existing `HasPromptTokPerSec` gate correctly omits that series instead of inventing a rate from a counter.
- The two formerly missing remote commits were fast-forwarded locally: `c4a5699` and `14ba810`.
- Merge remains gated on local verification, Claude review, GitHub CI, and repository review rules.

## Evidence

- Prior validated baseline: Finn used Yokai's pinned adaptation with SGLang `0.0.0.dev1+g5f55db35e`, image digest `sha256:616a3e97...9c3cafe`, DFlash2 block size 8, and zero restarts/OOMs.
- Aug 26 read-only Finn verification: the BF16-head container is running healthy on the pinned image digest with exact BKC arguments, zero restarts, and no OOM; model discovery, chat, Responses API, and a required two-turn tool call all passed.
- Race-enabled tests passed for every non-agent Go package and every agent test except the four Docker-socket-dependent cases; `go vet`, `go build`, and golangci-lint pass. Direct full `internal/agent` execution is blocked only by this user's lack of access to `/var/run/docker.sock`; the corresponding required GitHub build coverage is the authoritative gate.
- OpenTUI passes 28 tests, bundle build, and standalone compile; `git diff --check` passes.
- Runtime catalog count verified directly from `bkc.Catalog()`: 94 configs (91 vLLM, three SGLang), 88 unique models, and 25 publishers. README summary/table corrected to match.
- Preferred-BKC provenance now names MiaAI Lab commit `a0743929`; its notes document the intentional high-throughput Mamba cache tier and the remaining workload-specific BF16-state accuracy gate.
- Product requirement restored in the OpenTUI normalizer: `stopped` services remain listed but are neutral rather than alerts; unhealthy states remain alerts.
- Claude final review found one medium issue: sibling BKC choices bypassed hardware affinity. The daemon now exposes only capacity-compatible siblings with the selected recipe's exact target-device and architecture contract; the Qwen DFlash2/DSpark pair remains selectable while GB10/Jetson-only Nemotron images are filtered from generic hosts.
