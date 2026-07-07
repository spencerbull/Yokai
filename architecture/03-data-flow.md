# L3: Data Flow

How data moves through the system for each major operation.

## 1. Metrics Polling

```
                    every 2s
Agent :7474  ──────────────────►  Daemon (aggregator)  ──────►  TUI render
                                       │
  GET /metrics                         │ store in
  Response:                            ▼
  {                              Ring Buffer (per device)
    cpu: 47,                     ┌──────────────────────┐
    ram: { used, total },        │ metric  │ [60 floats]│
    gpus: [{                     │ cpu%    │ [47,48,...]│
      util: 87,                  │ gpu0%   │ [87,85,...]│
      vram_used: 20100,          │ vram0   │ [20.1,...] │
      vram_total: 24576,         │ temp0   │ [72,71,...]│
      temp: 72,                  │ ...     │            │
      power: 312                 └──────────────────────┘
    }],                                │
    containers: [{                     │ TUI reads via
      id, name, status,               ▼ GET daemon:7473/metrics
      cpu%, mem                  Daemon REST API
    }]                           localhost:7473
  }
```

## 2. Deploy Command

```
TUI (deploy wizard)
  │
  │ User fills: workload type, device, image, model, port, args
  │
  │ POST daemon:7473/deploy
  ▼
Daemon
  │
  │ 1. Validate request
  │ 2. Route to correct device's SSH tunnel
  │
  │ POST agent:7474/images/pull   ◄── SSE progress stream
  │   { image: "vllm/vllm-openai:latest" }
  │
  │ POST agent:7474/containers
  ▼   {
Agent     "name": "yokai-vllm-llama31-8b",
  │       "image": "vllm/vllm-openai:latest",
  │       "ports": {"8000": "8000"},
  │       "env": {"HF_TOKEN": "hf_..."},
  │       "gpus": "all",
  │       "command": ["--model", "meta-llama/Llama-3.1-8B-Instruct",
  │                   "--gpu-memory-utilization", "0.9"]
  │     }
  │
  │ Docker SDK:
  │   1. docker pull (if not cached)
  │   2. docker create + start
  │   3. Wait for health check (HTTP 200 on model endpoint)
  │
  ▼
Container running
  │
  │ Agent responds: { container_id, status: "running", port: 8000 }
  │
  ▼
Daemon
  │ Update config.json with new service entry
  │ Start monitoring this container in metrics loop
  ▼
TUI
  │ Service appears in dashboard service list
  ▼
```

## 3. Log Streaming

```
TUI (log viewer)
  │
  │ GET daemon:7473/logs/:service_id
  │ (SSE stream)
  ▼
Daemon
  │
  │ GET agent:7474/containers/:id/logs
  │ (SSE stream, forwarded)
  ▼
Agent
  │
  │ Docker SDK: ContainerLogs(follow=true, tail=100)
  │ Wraps each line as SSE event
  ▼
Docker container stdout/stderr
```

## 4. Config Read/Write

```
Startup:
  config.go: Load ~/.config/yokai/config.json
    │
    ├─ Version check → migrate if needed
    ├─ Defaults for missing fields
    └─ Return Config struct

Save (after any change):
  config.go: Save Config → marshal JSON → atomic write
    │
    ├─ Write to config.json.tmp
    └─ Rename to config.json (atomic on POSIX)

Who writes:
  ├─ TUI: after onboarding (add devices, HF token)
  ├─ Daemon: after deploy/stop (update services array)
  └─ Daemon: watches for external edits (fsnotify)
```

## 5. Docker Image Tag Discovery

```
TUI (deploy wizard, step 3)
  │
  │ GET daemon:7473/images/tags?image=vllm/vllm-openai
  ▼
Daemon
  │
  │ Check local cache (1h TTL)
  │   hit  → return cached tags
  │   miss ↓
  │
  │ Docker Hub API:
  │   GET https://hub.docker.com/v2/repositories/vllm/vllm-openai/tags
  │   Parse: name, last_updated, images[].size
  │
  │ GHCR API (for llama.cpp):
  │   GET https://ghcr.io/v2/ggml-org/llama.cpp/tags/list
  │
  ▼
TUI
  │ Render tag list with version, date, size, nightly label
```

## 6. HuggingFace Model Search

```
TUI (deploy wizard, step 4)
  │
  │ User types search query
  │ Debounce 300ms
  │
  │ GET daemon:7473/hf/models?q=meta-llama
  ▼
Daemon
  │
  │ GET https://huggingface.co/api/models
  │   ?search=meta-llama
  │   &filter=text-generation
  │   &sort=likes
  │   &limit=20
  │   Headers: Authorization: Bearer hf_token
  │
  │ For llama.cpp GGUF:
  │   GET https://huggingface.co/api/models/{id}/tree/main
  │   Filter: *.gguf files → extract quant name + size
  │
  ▼
TUI
  │ Render model list: name, size, likes, downloads
  │ For GGUF: show quantization table
```

## 7. VS Code Settings Merge

```
TUI (copilot screen)
  │
  │ User presses 'a' (auto-configure)
  ▼
vscode/settings.go
  │
  │ 1. Detect settings path:
  │    ~/.config/Code/User/settings.json          (Linux)
  │    ~/Library/Application Support/Code/User/   (macOS)
  │    Check Insiders variant too
  │
  │ 2. Read existing settings.json
  │ 3. Backup → settings.json.yokai.bak
  │
  │ 4. Parse JSON, find or create "chat.models" array
  │ 5. For each running OpenAI-compatible service:
  │      - Check if already present (match by url)
  │      - If not, append:
  │        {
  │          "family": "openai",
  │          "id": "model-name",
  │          "name": "Model Name (yokai)",
  │          "url": "http://host:port/v1",
  │          "apiKey": "none"
  │        }
  │
  │ 6. Marshal with indent, atomic write
  ▼
  settings.json updated
```
