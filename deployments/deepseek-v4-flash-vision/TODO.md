# DeepSeek V4 Flash vision rollout

## Goal

Expose a tailnet-only OpenAI Responses endpoint that keeps DeepSeek V4 Flash
0731 on Beskar and Kyber as the primary model while routing image analysis to a
local Qwen3-VL service on Coleman.

## Done criteria

- Coleman serves the pinned Qwen3-VL checkpoint from a pinned container.
- Moon Bridge accepts authenticated `/v1/responses` requests on Coleman's full
  MagicDNS name and preserves text, streaming, reasoning, and tool behavior.
- Image requests invoke the local vision model and return a DeepSeek-authored
  answer without sending image data outside the tailnet.
- Codex and OpenCode expose a separate selectable vision model and keep the
  direct Kyber model as rollback.
- Client-side compaction is configured and manually verified.
- Prometheus and Grafana show Coleman host, Qwen3-VL, and Moon Bridge health
  without changing the meaning of the two-node DeepSeek metrics.
- The deployment survives container restarts and all verification scripts pass.

## Stream and branch

- Branch: `deepseek-vision-gateway`
- Worktree: `/home/sbull/src/github.com/spencerbull/Yokai`
- Deployment: `deployments/deepseek-v4-flash-vision`
- Observability: `deployments/deepseek-v4-flash-observability`

## Allowed actions

- Add version-controlled deployment, verification, and dashboard files.
- Deploy containers and monitoring configuration to Coleman and Beskar.
- Update the user's Codex and OpenCode model selections after live API gates.
- Preserve and verify the direct Kyber endpoint as rollback.

## Forbidden actions

- Do not stop or alter the Beskar/Kyber DeepSeek serving containers.
- Do not delete cached models, Docker images, dashboards, or unrelated services.
- Do not publish image payloads, bearer tokens, or raw request traces.
- Do not expose the gateway or vision backend outside the tailnet.

## Required gates

- [x] Static configuration validation and focused tests.
- [x] Qwen3-VL text and image smoke tests.
- [x] Moon Bridge authenticated text, streaming, image, and tool tests.
- [x] Codex image request through `/v1/responses`.
- [x] OpenCode image request through `/v1/responses`.
- [x] Compaction configuration parsed and exercised by both client profiles.
  The production 917,504-token threshold was not artificially forced.
- [x] Direct Kyber rollback generation remains healthy.
- [x] Prometheus targets and rules healthy.
- [x] Grafana dashboard rendered and visually checked.
- [x] Independent review findings resolved or documented.

## Checkpoints

- Complete: pinned Coleman Qwen3-VL and Moon Bridge services deployed.
- Complete: Codex, OpenCode, and Finn OpenClaw selectors configured.
- Complete: Coleman monitoring added without entering two-node DeepSeek math.
- Complete: independent review findings fixed and re-verified live.

## Evidence

- Full acceptance passed for auth, catalog, text, SSE, OCR/color image routing,
  mixed image/client tools, direct rollback generation, metrics, and bindings.
- The final hardened release used a unique Moon Bridge image tag; its successful
  acceptance removed every run-stamped deployment and secret backup.
- Codex returned `CODEX_IMAGE_OK HELLO RED`; OpenCode returned
  `OPENCODE_IMAGE_OK HELLO RED` from the deterministic fixture. Final post-cutover
  client reruns returned `CODEX_FINAL_IMAGE_OK HELLO RED` and
  `OPENCODE_FINAL_IMAGE_OK HELLO RED`.
- Finn OpenClaw returned `OPENCLAW_FINAL_MODEL_OK` on the configured vision
  provider/model with no fallback and a 1,048,576-token context budget.
- Grafana provisioned 96 panels with 10/10 healthy Prometheus targets; the
  rendered Coleman readiness panel was green.
- Existing DeepSeek containers and Coleman's `qwen36-27b-mtp.service` retained
  their identities across monitoring and gateway deployments.
