FROM vllm/vllm-openai:v0.20.0

# Nemotron Nano Omni audio inputs and videos with audio tracks require the
# optional vLLM audio dependencies before `vllm serve` starts.
RUN python3 -m pip install --no-cache-dir "vllm[audio]==0.20.0"
