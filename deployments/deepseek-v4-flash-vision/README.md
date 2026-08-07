# DeepSeek V4 Flash vision gateway

This deployment keeps `deepseek-ai/DeepSeek-V4-Flash-0731` on Beskar and
Kyber as the primary reasoning model and adds local image understanding through
`Qwen/Qwen3-VL-30B-A3B-Instruct-FP8` on Coleman.

Clients use an authenticated OpenAI Responses endpoint:

```text
http://coleman.trout-rooster.ts.net:38440/v1
```

Moon Bridge removes image bytes before calling the text-only DeepSeek model,
offers `visual_brief` and `visual_qa` tools, sends only selected images to the
local Qwen service, and feeds the resulting evidence back to DeepSeek for the
final answer.

## Security and isolation

- Docker publishes Qwen and Moon Bridge only on Coleman's Tailscale address.
- The client-facing gateway, Qwen backend, and DeepSeek upstream use three
  distinct protected bearer keys.
- Secrets are staged separately from the live bind mounts and swapped only at
  activation; every post-cutover gate is covered by automatic rollback.
- Each Moon Bridge build receives a release-specific image tag, so rollback
  restores the exact prior binary. Run-stamped deployment and secret backups
  are removed only after acceptance or a successful rollback.
- Raw Moon Bridge request tracing is disabled.
- Qwen receives at most four images and video input is disabled.
- No local-media filesystem path or remote-code trust is enabled.
- The direct Kyber endpoint remains available for rollback.

## Pinned inputs

| Component | Pin |
| --- | --- |
| Qwen model | `Qwen/Qwen3-VL-30B-A3B-Instruct-FP8` |
| Qwen revision | `d9748a51ae66354c4dad665aab2c71f26cf2c8cd` |
| vLLM image | `nvcr.io/nvidia/vllm@sha256:95c498a475142c20c989c65e5d223348c09fed83ba17ddf44f117610c0bd3268` (`26.07-py3`, vLLM 0.24.0) |
| Moon Bridge | `c9ae8a8ee824c02ce1166893543838f3e414fde0` |
| Moon Bridge archive | `sha256:4ef3fdbe273a2846ce7e034de57e5335e1241899185e11b654c1cde52a0683df` |
| Go build image | `golang:1.25.5-alpine3.23@sha256:ac09a5f469f307e5da71e766b0bd59c9c49ea460a528cc3e6686513d64a6f1fb` |
| Runtime image | `alpine:3.23.2@sha256:865b95f46d98cf867a156fe4a135ad3fe50d2056aa3f25ed31662dff6da4eb62` |

## Deploy and verify

The scripts use Tailscale SSH because Coleman's ordinary SSH host-key record is
stale. They preserve the existing `qwen36-27b-mtp.service` and refuse success if
its identity changes. The workstation must have ImageMagick and Liberation Sans
for the deterministic OCR fixture.

```bash
./deployments/deepseek-v4-flash-vision/deploy.sh
./deployments/deepseek-v4-flash-vision/verify.sh
```

The deployment starts with a 32K visual context, two Qwen scheduler slots, and
40% vLLM memory utilization so it can coexist with Coleman's existing 27B
llama.cpp service. Eager execution plus explicit CUTLASS linear and Triton MoE
backends avoid the incompatible FP8 auto-selections observed on GB10. Increase
those values only after measuring unified-memory headroom under concurrent load.

## Client selectors and compaction

```bash
codex -p deepseek-spark-vision
opencode2 run --model deepseek-spark-vision/deepseek-v4-flash-0731-vision
```

Codex advertises a 1,048,576-token context and auto-compacts at 917,504 tokens.
OpenCode enables automatic compaction and pruning, preserves six recent turns
and 32K recent tokens, and reserves 128K tokens. Finn's OpenClaw catalog exposes
`deepseek-spark-vision/deepseek-v4-flash-0731-vision` using its own file-backed
client secret.

Text-only Responses stream natively. Image requests are deliberately buffered
while Moon Bridge obtains local Qwen visual evidence and lets DeepSeek synthesize
the answer or return the client's function call. The final DeepSeek route keeps
the 1M context; each individual Qwen visual analysis is capped at 32K.

Codex `--image` and OpenCode `--file` attachments are embedded in the request
and are the supported path for local screenshots and files. A bare HTTP(S)
image URL must be anonymously reachable from Coleman; private browser-session
URLs such as authenticated chat attachments cannot carry browser cookies through
the API gateway. Download those images and attach the local file instead.
