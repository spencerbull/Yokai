# L8: Multi-Device Deployments

Yokai's coordinated deployment subsystem is additive. Existing single-device BKC selection, `POST /deploy`, managed container inventory, metrics, and service lifecycle behavior keep their current contracts.

## State and API boundary

Cluster BKCs add optional typed `MultiDevice` metadata. They are rejected by `POST /deploy` and accepted only by the `/deployments` family:

- `POST /deployments`
- `GET /deployments`
- `GET /deployments/{deploymentID}`
- `POST /deployments/{deploymentID}/test`
- `POST /deployments/{deploymentID}/start`
- `POST /deployments/{deploymentID}/stop`
- `POST /deployments/{deploymentID}/rollback`

Deployment groups use a separate, versioned `deployments.json` store. Writes use a temporary file, file sync, atomic rename, and directory sync. Version zero (including a missing pre-release version field) is read as version one and rewritten on the next normal transition; future versions are rejected rather than rewritten. The state is intentionally outside `config.json`, whose older writers do not know how to preserve deployment groups.

Deployment responses and records contain bindings, members, ownership, generation, status, tests, rollback results, and an ordered list of runtime-patch labels, source paths, and pristine/patched hashes. A legacy persisted singular `runtime_patch` is read as exactly one historical provenance entry rather than being represented as the current two-patch set; subsequent writes use `runtime_patches`. Records never contain an API key, container environment, rendered command line, or host-local model path.

## Validation and capability gate

Before journaling or Docker mutation, create validates:

1. the exact pinned BKC and its immutable model revision/image digest;
2. exactly one `head` rank-0 binding and one `worker` rank-1 binding;
3. distinct configured devices and distinct valid, non-loopback fabric IPs;
4. a separate explicit rank-0 client/monitor IP and the recipe-pinned service port;
5. online agents with all recipe-required capabilities;
6. exactly one GB10 GPU exposed by each agent;
7. an optional clean absolute local snapshot path that is an existing readable directory on each node;
8. requested service and fabric addresses that exist on the corresponding node; and
9. rendezvous and service listeners on the exact requested local address and port that are either free or provably owned by the explicitly selected old container.

Agents advertise additive capabilities through `/health`. `GET /containers` remains managed-only by default; `GET /containers?scope=all` exposes sanitized container identity, status, ownership, and allowlisted management/provenance labels. It never returns container environment variables or arbitrary labels. Metrics remain managed-only.

For an occupied service or head rendezvous endpoint, the agent inspects the selected container to a full Docker ID and filters extended `ss` data to the exact requested local address and port. It prefers cgroup ownership (`docker-<id>.scope`), accepting a shortened cgroup ID only when it is at least 12 characters and resolves unambiguously to that exact container. PID ownership from `docker top` is a compatibility fallback when socket cgroup identity is unavailable.

## Transaction

The deployment engine holds a deployment/device transaction lock and performs the following ordered sequence:

1. Validate the request and recipe without mutation.
2. Resolve only explicitly selected old container names or IDs to exact members.
3. Precompute deterministic candidate names and run both capability/local-resource preflights against those exact selections.
4. Persist the candidate names, exact selected previous members, a `pending` phase, and per-effect progress before mutation.
5. Pull/stage the pinned candidate on head, then worker.
6. Stop, but never remove, selected old IDs.
7. Launch the head candidate, then the worker candidate.
8. Run a bounded readiness loop (45 minutes by default) that fails immediately if either rank exits.
9. Retry rank-0 model discovery, exact normalized `ok` chat semantics, and authenticated native SGLang `/metrics` until the readiness deadline.
10. Atomically promote the group to `running`.

On failure, every launch-attempted deterministic candidate name is removed in reverse order, including ambiguous launch timeouts. Before each removal, rollback best-effort captures and durably records the candidate's final 2,000 Docker log lines within a 256 KiB raw-byte cap. It strips unsafe control bytes and exact or credential-shaped API-key values; raw suffix truncation first drops the potentially partial boundary line so a secret fragment cannot evade redaction. A nonzero `docker logs` exit with drained bytes preserves the sanitized partial capture marked truncated, while an exit with no bytes remains a capture error. Log collection failure never blocks removal.

Coordinated launch is a bounded two-sided protocol rather than the lifetime of the caller's HTTP request: before any Docker image validation or launch command, the agent registers the deterministic candidate name and starts a five-minute detached deadline. Every manifest inspection and `docker run` uses that context. `CommandContext` waits for the Docker client process to exit, and a run error then gets a five-minute deterministic-name settling/cleanup pass that removes only the complete expected provenance tuple. The daemon waits independently for up to eleven minutes, which is longer than the command plus settling bounds. A per-name agent launch barrier also makes a concurrent deployment-member delete wait for that cleanup, and rollback always crosses that delete barrier even when its preliminary inspect saw the name absent. Docker CLI performs create before start and cannot issue another Engine request after its process has exited; the settling pass covers a create already accepted by the Engine but not yet visible when the client was terminated. If the agent or transport is unavailable before the barrier can answer, rollback remains `rollback_failed`, does not restore previous members, and cannot certify safety. Create-time rollback receives its own bounded cleanup context so a disconnected or timed-out caller cannot suppress that attempt. Already-absent candidates are normal only after the barrier. Once every candidate is known absent, selected old members are restored head then worker, and each must be observed running before `rolled_back` is persisted. Partial rollback progress is durable and retryable through explicit rollback, idempotent create recovery, and daemon-start reconciliation. External containers are never auto-adopted.

## Ownership and lifecycle

Members use `managed`, `adopted`, or `observed` ownership. Observed external containers are selected only by exact name or an unambiguous ID prefix of at least 12 characters, and a container already labeled as a grouped member cannot be selected into another group. Every grouped stop, restart, and removal uses an agent deployment-member endpoint that validates `managed=true`, ownership `managed`, deployment ID, generation, role, and deterministic name before mutation. Generic agent lifecycle endpoints reject grouped containers and fail closed with an unavailable response on every non-not-found identity-inspection error; a truly absent identity falls through to the legacy handler's existing not-found check. Stopped groups and stopped containers remain ordinary visible state. Group start first inspects both ranks; a truly running group returns unchanged, while an exited managed rank triggers a coordinated stop and authenticated head-then-worker recovery. Wrong identity fails closed and durably removes the green state. Failed-start cleanup gets its own bounded context, so caller cancellation cannot leave a partially restarted group running. The operator must re-supply the original launch key retained in rank 0's Docker command; start cannot rotate that key because Yokai deliberately stores neither the secret nor enough launch input to recreate the group. A wrong key fails readiness and returns successfully stopped members to `stopped`. Key rotation requires a new deployment.

Daemon restart reconciliation is serialized by the same engine transaction mutex as HTTP lifecycle requests. It is state-specific and does not need the API key: `pending` and `rollback_failed` retry create rollback; `starting` is made stopped in worker-then-head cleanup order; `stopping` finishes that same order; and `failed` consults the latest durable lifecycle action. Failed Start resumes safe-stop, while failed Stop or `verify_running` performs provenance-checked safe-stop of exact managed ranks and never removes the promoted group or restores `PreviousMembers`. Only a failed record whose durable progress still identifies incomplete creation enters create rollback. Replaying the original create idempotency key uses the same classifier and returns an explicit recovery conflict after lifecycle reconciliation; it cannot reinterpret a later lifecycle failure as incomplete create. All cleanup contexts are bounded and detached from the canceled startup/request context. A missing or provenance-mismatched rank is never mutated and leaves the record `failed` or `rollback_failed`, while other exact managed ranks still receive their safe cleanup attempt.

## GLM-5.3-Flash dual-GB10 recipe

`glm-5-3-flash-nvfp4-dual-gb10` runs `LibertAIDAI/GLM-5.3-Flash-NVFP4` at revision `aa28e1f54130286c95fee10d0705c74ce8743734` in the pinned SGLang image digest. It uses host networking, host IPC, `/dev/infiniband`, `IPC_LOCK`, one GPU per node, TP=2, and torch distributed rendezvous at the rank-0 private fabric address on port 25000. The request supplies rank 0's distinct client/monitor address (a Tailnet or other routable IP), and the recipe requires port 8000. Rank 1 binds only its own private fabric address. Neither role binds `0.0.0.0`, and no host-specific address is embedded in the BKC. Both `--dist-timeout` and `--watchdog-timeout` are pinned to 3600 seconds: an unset distributed timeout inherits PyTorch/NCCL's shorter collective guard, while SGLang's scheduler watchdog defaults to 300 seconds.

The pinned image contains a separate 480-second `UNBALANCED_MODEL_LOADING_TIMEOUT_S` guard, but it does **not** contain the CUDA GB10 DSA tile override required by this recipe. Yokai therefore carries two independently provenance-pinned source patches in order. The existing loader patch targets `/sgl-workspace/sglang/python/sglang/srt/model_executor/model_runner_components/load_model_utils.py`, requires pristine SHA-256 `f0193bfaab96053919e3a260f9ccb10e2137ad108cfae03816c867628f611e1f` and exactly one original 480-second line, and requires patched SHA-256 `fc3ae35cce5f712fd3681dc4bcef05157df6d232ac991ce60b78dae492784ff5`. The `sglang-dsa-gb10-tile-tp2-v1` patch then targets `/sgl-workspace/sglang/python/sglang/kernels/ops/attention/dsa/tilelang_kernel.py`, requires pristine SHA-256 `526988aa5fd8fa61529f2d3cf245d1061e30982d2bd0d6e64ea2e51e31f30f7f`, adds `block_I=32`, `num_stages=1`, and `threads=128` only when `tail_dim == 0`, and requires patched SHA-256 `3150ec691843c84bb5db9ed5bf763c4da03842fde4666489e107bf7a9fcddd7b`.

Before writing either file, the structured-argv bootstrap reads and preflights both complete-file hashes and the unique old/new line counts, and also checks `torch.cuda.get_device_properties(0).shared_memory_per_block_optin`. It requires at least 100352 bytes and emits the observed limit with the TP=2 32/1/128 tile identity. Only after every preflight succeeds does it patch pristine files in order; exact already-patched files and mixed pristine/patched states are idempotent. It then verifies both final hashes/counts and execs the original normalized `sglang serve` argv directly, without a shell. Docker labels use the deterministic value `sglang-unbalanced-model-loading-timeout-3600-v1+sglang-dsa-gb10-tile-tp2-v1`, and the bootstrap is accepted only for this exact BKC, model or fixed local snapshot, revision, image digest, and TP=2 command.

TP=2 is correctness-bearing, not just topology metadata. Before pipeline overhead, the selected tile has a 100352-byte unaliased shared-memory request at TP=2; TP=1 exceeds the GB10 limit. Canary 4 loaded every checkpoint shard and then failed before readiness because the stock CUDA tile requested 169984 bytes while GB10 exposed 101376 bytes. That exact request is consistent only with the `tail_dim == 0` v1 decode branch and, with TP=2, 32 heads per rank. The hardware guard proves only that a device can opt in to the base request; the compiler remains the authority on final aliasing and overhead. Canary 5 validated the pinned combination on two GB10s: both ranks reported `required=100352 observed=101376`, crossed the old 480-second barrier, compiled and warmed the 32/1/128 tile without the shared-memory error, and passed readiness in 13m09s. Exact chat, Responses, two-turn tool use, image input, 7.8K/31.8K/59.8K retrieval, two-request concurrency, metrics, secret-log, restart/OOM, and thermal gates then passed. That evidence applies to these exact model, image, source hashes, flags, and TP=2 topology; it is not a portable claim for a drifted runtime. Yokai's readiness window remains a separate, bounded 45-minute rollback gate. The one-hour guards are intentionally conservative and should be retuned only after more cold-start history is collected.

The canary pins the known-good `enP2p1s0f0np0`/`roceP2p1s0f0` Gloo/NCCL fabric environment without embedding an IP address. Both ranks use Docker restart policy `no`; legacy single-device deployments retain the default `unless-stopped` policy.

The canary pins `--enable-metrics` and `--log-level warning`. Yokai requires a non-empty API key, omits it from the BKC, deployment store, deployment responses, inventory, and Yokai log messages, and sends one structured `--api-key=<value>` argv token only to rank 0. SGLang has no documented secret-file-descriptor input: Docker `Config.Cmd` and the rank-0 process argv necessarily retain the key for the container lifetime. Supervised canary inspection must account for that exposure and verify the key is absent from Yokai/SGLang logs. Rollback does not persist or recover credentials, so semantic verification of a restored authenticated old service remains a supervised live gate. No MTP or speculative decoding is enabled.

When `local_model_path` is supplied, it must already exist at the same absolute path on both nodes. Yokai mounts it read-only at the fixed container path `/models/yokai-deployment`. Yokai does not copy snapshots, infer a host path, or change legacy volume behavior.
