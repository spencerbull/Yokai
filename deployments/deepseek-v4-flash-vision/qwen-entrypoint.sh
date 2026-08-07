#!/usr/bin/env bash
set -Eeuo pipefail

readonly secret_file="/run/secrets/qwen-api-key"
readonly model_revision="d9748a51ae66354c4dad665aab2c71f26cf2c8cd"
readonly model_path="/models/qwen3-vl/snapshots/${model_revision}"

if [[ ! -r "${secret_file}" ]]; then
  echo "Qwen API key is unavailable" >&2
  exit 1
fi
if [[ ! -f "${model_path}/model.safetensors.index.json" ]]; then
  echo "Pinned Qwen3-VL snapshot is unavailable" >&2
  exit 1
fi

export VLLM_API_KEY
VLLM_API_KEY="$(tr -d '\r\n' <"${secret_file}")"
if [[ ! "${VLLM_API_KEY}" =~ ^[A-Za-z0-9._-]+$ ]]; then
  echo "Qwen API key contains unsupported characters" >&2
  exit 1
fi

exec vllm serve "${model_path}" \
  --served-model-name qwen3-vl-30b-a3b-instruct-fp8 \
  --host 0.0.0.0 \
  --port 8000 \
  --tensor-parallel-size 1 \
  --max-model-len 32768 \
  --max-num-seqs 2 \
  --max-num-batched-tokens 4096 \
  --gpu-memory-utilization 0.40 \
  --kv-cache-dtype auto \
  --enforce-eager \
  --linear-backend cutlass \
  --moe-backend triton \
  --limit-mm-per-prompt '{"image":4,"video":0}' \
  --generation-config vllm \
  --disable-access-log-for-endpoints /health,/metrics,/ping
