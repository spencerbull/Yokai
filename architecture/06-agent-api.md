# L6: Agent REST API

Full REST API contract for the yokai agent running on target devices (port 7474).

## Authentication

All endpoints require:
```
Authorization: Bearer <agent_token>
```

Token is a 64-character hex string generated during device bootstrap and stored in:
- User machine: `~/.config/yokai/config.json` → `devices[].agent_token`
- Target device: `/etc/yokai/agent.json` → `token`

Unauthorized requests receive `401 Unauthorized`.

---

## Endpoints

### GET /health

Health check and version info.

**Response** `200 OK`
```json
{
  "status": "ok",
  "version": "0.1.0",
  "uptime_seconds": 86400,
  "hostname": "gaming-rig"
}
```

---

### GET /system/info

Static system information (doesn't change frequently).

**Response** `200 OK`
```json
{
  "hostname": "gaming-rig",
  "os": "Ubuntu 24.04 LTS",
  "arch": "x86_64",
  "kernel": "6.8.0-45-generic",
  "cpu": {
    "model": "AMD Ryzen 9 7950X",
    "cores": 16,
    "threads": 32
  },
  "ram_total_mb": 32768,
  "disk": {
    "total_gb": 500,
    "free_gb": 142
  },
  "gpus": [
    {
      "index": 0,
      "name": "NVIDIA RTX 4090",
      "vram_total_mb": 24576,
      "driver_version": "550.54.15",
      "cuda_version": "12.4"
    }
  ],
  "docker": {
    "version": "27.1.2",
    "runtime": "nvidia"
  }
}
```

---

### GET /metrics

Live system and GPU metrics. Called every 2 seconds by daemon.

**Response** `200 OK`
```json
{
  "timestamp": "2025-03-02T21:42:15Z",
  "cpu": {
    "percent": 47.2,
    "per_core": [62.1, 33.4, 51.0, 42.8]
  },
  "ram": {
    "used_mb": 18636,
    "total_mb": 32768,
    "percent": 56.9
  },
  "swap": {
    "used_mb": 102,
    "total_mb": 8192
  },
  "disk": {
    "used_gb": 358,
    "total_gb": 500,
    "free_gb": 142
  },
  "gpus": [
    {
      "index": 0,
      "name": "NVIDIA RTX 4090",
      "utilization_percent": 87,
      "vram_used_mb": 20582,
      "vram_total_mb": 24576,
      "temperature_c": 72,
      "power_draw_w": 312,
      "power_limit_w": 450,
      "fan_percent": 65
    }
  ],
  "containers": [
    {
      "id": "a3f7c2d...",
      "name": "yokai-vllm-llama31-8b",
      "status": "running",
      "cpu_percent": 34.2,
      "memory_used_mb": 1240,
      "gpu_memory_mb": 9318,
      "uptime_seconds": 12240
    }
  ]
}
```

---

### GET /metrics/prometheus

Prometheus scrape endpoint. Returns metrics in Prometheus exposition format.

**Response** `200 OK` (text/plain)
```
# HELP yokai_cpu_percent CPU utilization percentage
# TYPE yokai_cpu_percent gauge
yokai_cpu_percent 47.2

# HELP yokai_ram_used_bytes RAM used in bytes
# TYPE yokai_ram_used_bytes gauge
yokai_ram_used_bytes 19541893120

# HELP yokai_gpu_utilization GPU utilization percentage
# TYPE yokai_gpu_utilization gauge
yokai_gpu_utilization{gpu="0",name="RTX 4090"} 87

# HELP yokai_gpu_vram_used_bytes VRAM used in bytes
# TYPE yokai_gpu_vram_used_bytes gauge
yokai_gpu_vram_used_bytes{gpu="0",name="RTX 4090"} 21578424320

# HELP yokai_gpu_temperature GPU temperature in Celsius
# TYPE yokai_gpu_temperature gauge
yokai_gpu_temperature{gpu="0",name="RTX 4090"} 72
```

---

### GET /containers

List all containers managed by yokai (prefix `yokai-*`).

**Response** `200 OK`
```json
{
  "containers": [
    {
      "id": "a3f7c2d89e1f",
      "name": "yokai-vllm-llama31-8b",
      "image": "vllm/vllm-openai:latest",
      "status": "running",
      "state": "healthy",
      "ports": {"8000": "8000"},
      "created": "2025-03-02T18:34:12Z",
      "uptime_seconds": 12240,
      "labels": {
        "yokai.type": "vllm",
        "yokai.model": "meta-llama/Llama-3.1-8B-Instruct",
        "yokai.port": "8000"
      }
    }
  ]
}
```

---

### POST /containers

Deploy a new workload container.

**Request**
```json
{
  "name": "yokai-vllm-llama31-8b",
  "image": "vllm/vllm-openai:latest",
  "ports": {
    "8000": "8000"
  },
  "env": {
    "HF_TOKEN": "hf_..."
  },
  "gpus": "all",
  "command": [
    "--model", "meta-llama/Llama-3.1-8B-Instruct",
    "--gpu-memory-utilization", "0.9",
    "--max-model-len", "4096"
  ],
  "labels": {
    "yokai.type": "vllm",
    "yokai.model": "meta-llama/Llama-3.1-8B-Instruct",
    "yokai.port": "8000"
  },
  "ipc_host": true
}
```

**Response** `201 Created`
```json
{
  "container_id": "a3f7c2d89e1f",
  "name": "yokai-vllm-llama31-8b",
  "status": "running"
}
```

**Error** `409 Conflict` — container with same name already exists
**Error** `400 Bad Request` — invalid spec

---

### DELETE /containers/:id

Stop and remove a container.

**Response** `200 OK`
```json
{
  "container_id": "a3f7c2d89e1f",
  "status": "removed"
}
```

**Error** `404 Not Found` — container not found

---

### POST /containers/:id/restart

Restart a container.

**Response** `200 OK`
```json
{
  "container_id": "a3f7c2d89e1f",
  "status": "running"
}
```

---

### GET /containers/:id/logs

Stream container logs via Server-Sent Events (SSE).

**Query Parameters**
| Param | Type | Default | Description |
|-------|------|---------|-------------|
| `tail` | int | 100 | Number of historical lines |
| `follow` | bool | true | Stream new lines |

**Response** `200 OK` (text/event-stream)
```
data: {"timestamp":"2025-03-02T21:42:15Z","stream":"stdout","line":"INFO: Loading model..."}

data: {"timestamp":"2025-03-02T21:42:16Z","stream":"stdout","line":"INFO: Model loaded in 42s"}

data: {"timestamp":"2025-03-02T21:42:16Z","stream":"stderr","line":"WARNING: High VRAM usage"}
```

---

### POST /images/pull

Pull a Docker image with streaming progress.

**Request**
```json
{
  "image": "vllm/vllm-openai:latest"
}
```

**Response** `200 OK` (text/event-stream)
```
data: {"status":"pulling","layer":"sha256:abc123","progress":0.0}

data: {"status":"pulling","layer":"sha256:abc123","progress":0.45,"current_mb":230,"total_mb":512}

data: {"status":"pulling","layer":"sha256:def456","progress":0.12}

data: {"status":"complete","image":"vllm/vllm-openai:latest","size_mb":14200}
```

**Error** `400 Bad Request` — invalid image name
**Error** `500` — pull failed (auth, network, etc.)

---

### GET /images/tags/:image

Fetch available tags for a Docker image from its registry.

**URL**: `/images/tags/vllm%2Fvllm-openai` (URL-encoded image name)

**Response** `200 OK`
```json
{
  "image": "vllm/vllm-openai",
  "registry": "docker.io",
  "tags": [
    {
      "name": "latest",
      "digest": "sha256:abc123...",
      "last_updated": "2025-02-28T12:00:00Z",
      "size_mb": 14200
    },
    {
      "name": "v0.6.4",
      "last_updated": "2025-02-28T12:00:00Z",
      "size_mb": 14200
    },
    {
      "name": "nightly",
      "last_updated": "2025-03-02T04:00:00Z",
      "size_mb": 14500,
      "nightly": true
    }
  ],
  "cached_until": "2025-03-02T23:42:15Z"
}
```

---

## Error Response Format

All errors follow:
```json
{
  "error": "short_code",
  "message": "Human-readable description",
  "details": {}
}
```

| HTTP Code | Error Code | Meaning |
|-----------|------------|---------|
| 400 | `bad_request` | Invalid request body |
| 401 | `unauthorized` | Missing or invalid token |
| 404 | `not_found` | Resource not found |
| 409 | `conflict` | Resource already exists |
| 500 | `internal` | Server error |

---

## SSE Streaming Convention

For long-running operations (logs, image pull), the agent uses Server-Sent Events:

- Content-Type: `text/event-stream`
- Each event is a JSON object prefixed with `data: `
- Events separated by double newline
- Stream ends when the connection closes (client disconnects or operation completes)
- Client should handle reconnection for log streams
