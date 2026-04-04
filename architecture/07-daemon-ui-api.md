# Daemon UI API

This document defines the UI-neutral daemon API contract for Yokai's next frontend.

The goal is to keep the Go backend reusable across multiple interfaces while the Bubble Tea frontend is replaced by a new OpenTUI React client under `ui/tui`.

## Scope

This contract covers daemon endpoints used by:

- the new OpenTUI frontend
- any future web UI or alternate TUI
- tests and tooling that need stable daemon behavior

It does not replace the agent API documented in `06-agent-api.md`.

## Package Decision

- frontend package location: `ui/tui`
- frontend package name: `@yokai/tui`
- backend boundary: existing local daemon process

The frontend talks to the daemon over REST and SSE only. It should not directly:

- read `~/.ssh/config`
- invoke `tailscale`
- write config or history files
- write editor integration files
- bootstrap devices over SSH

Those behaviors belong to daemon-owned services.

## Design Goals

- Keep the daemon UI-agnostic.
- Preserve all current TUI features.
- Use request and response models that are stable across frontends.
- Normalize long-running actions into a shared operations model.
- Keep existing daemon endpoints where they are already useful.

## API Conventions

### Transport

- REST for request/response operations
- SSE for log and long-running operation event streams
- JSON request and response bodies unless otherwise noted

### Base Address

Use the existing daemon listen address, typically `127.0.0.1:7473`.

### Versioning

For the first migration pass, new daemon endpoints can be additive and unversioned.

If existing endpoints need breaking shape changes, prefer introducing a new endpoint family rather than silently changing the current response structure.

### Error Shape

New endpoint families should standardize on one error envelope:

```json
{
  "error": {
    "code": "device_not_found",
    "message": "device \"dev-123\" was not found",
    "details": {
      "device_id": "dev-123"
    }
  }
}
```

Guidelines:

- `code` is stable and machine-readable
- `message` is human-readable
- `details` is optional structured context for clients and logs

### Time Format

All timestamps should use RFC3339 strings in UTC.

### IDs

- device IDs, service IDs, and operation IDs are opaque strings
- clients should not parse meaning from IDs

## Shared Data Models

### Device Summary

```json
{
  "id": "dev-123",
  "label": "gaming-rig",
  "host": "100.64.0.2",
  "connection_type": "tailscale",
  "ssh": {
    "user": "root",
    "port": 22,
    "key_path": "~/.ssh/id_ed25519",
    "uses_agent": false,
    "has_password": false
  },
  "status": {
    "online": true,
    "version": "0.1.0",
    "last_checked_at": "2026-04-03T15:00:00Z",
    "last_error": ""
  },
  "hardware": {
    "gpu_type": "NVIDIA RTX 4090",
    "gpu_count": 1
  },
  "service_count": 3
}
```

### Service Summary

```json
{
  "id": "svc-123",
  "device_id": "dev-123",
  "container_id": "container-123",
  "name": "llama-serve",
  "type": "vllm",
  "image": "vllm/vllm-openai:latest",
  "status": "running",
  "health": "healthy",
  "ports": {
    "8000": "8000"
  },
  "metrics": {
    "cpu_percent": 12.4,
    "memory_used_mb": 1024,
    "gpu_memory_mb": 8192,
    "generation_tok_per_s": 35.5,
    "prompt_tok_per_s": 118.2
  }
}
```

### Operation Summary

```json
{
  "id": "op-123",
  "kind": "bootstrap_device",
  "status": "running",
  "target": {
    "device_id": "dev-123"
  },
  "progress": {
    "current_step": "deploy_agent",
    "percent": 45,
    "message": "Uploading agent binary"
  },
  "result": null,
  "error": null,
  "created_at": "2026-04-03T15:00:00Z",
  "updated_at": "2026-04-03T15:00:04Z"
}
```

### Settings Document

```json
{
  "hf": {
    "configured": true,
    "source": "config",
    "username": "sbull"
  },
  "preferences": {
    "theme": "tokyonight",
    "default_vllm_image": "vllm/vllm-openai:latest",
    "default_llama_image": "ghcr.io/ggml-org/llama.cpp:server-cuda",
    "default_comfyui_image": "spencerbull/yokai-comfyui:latest"
  },
  "history": {
    "images": ["vllm/vllm-openai:latest"],
    "models": ["meta-llama/Llama-3.1-8B-Instruct"]
  },
  "integrations": {
    "vscode": { "available": true, "configured": true, "path": "~/.config/Code/User/settings.json" },
    "opencode": { "available": true, "configured": true, "path": "~/.config/opencode/opencode.json" },
    "openclaw": { "available": true, "configured": false, "path": "~/.openclaw/openclaw.json" }
  }
}
```

Notes:

- `username` may be absent until a token has been validated.
- `available` indicates whether the daemon can resolve the expected config location on the current machine.

## Existing Endpoints To Keep

The following daemon endpoints already fit the long-term backend boundary and should remain available:

- `GET /health`
- `GET /devices`
- `GET /metrics`
- `GET /metrics/{deviceID}`
- `POST /deploy`
- `POST /containers/{deviceID}/{containerID}/stop`
- `POST /containers/{deviceID}/{containerID}/restart`
- `DELETE /containers/{deviceID}/{containerID}/remove`
- `POST /containers/{deviceID}/{containerID}/test`
- `GET /logs/{deviceID}/{containerID}`
- `GET /images/tags`
- `POST /reload`

These can be refined later, but they are already useful for the first OpenTUI migration slices.

## New Endpoint Families

### Discovery

Discovery endpoints replace frontend-local environment inspection.

#### `GET /discovery/ssh-config-hosts`

Returns parsed and normalized host entries from `~/.ssh/config`.

Response:

```json
{
  "hosts": [
    {
      "alias": "workstation",
      "host": "10.0.0.20",
      "user": "root",
      "port": 22,
      "identity_file": "~/.ssh/id_ed25519",
      "identity_file_encrypted": false
    }
  ]
}
```

#### `GET /discovery/tailscale/status`

Implemented in the current migration slice.

Response:

```json
{
  "installed": true,
  "running": true,
  "needs_login": false,
  "backend_state": "Running",
  "self": {
    "hostname": "sbull-desktop",
    "ip": "100.64.0.1"
  },
  "error": "",
  "install_instructions": "Install Tailscale...",
  "tag_help": "Recommended Yokai tag: tag:ai-gpu..."
}
```

#### `GET /discovery/tailscale/peers`

Implemented in the current migration slice.

Response:

```json
{
  "peers": [
    {
      "hostname": "gaming-rig",
      "dns_name": "gaming-rig.example.ts.net",
      "ip": "100.64.0.2",
      "ips": ["100.64.0.2"],
      "os": "linux",
      "online": true,
      "tags": ["tag:ai-gpu", "tag:lab"],
      "highlighted_tags": ["AI GPU"],
      "other_tags": ["tag:lab"],
      "recommended": true
    }
  ]
}
```

#### `GET /discovery/local-network`

This endpoint may begin as parity-only data that matches the current local-network path and can later grow into real subnet discovery.

Response:

```json
{
  "interfaces": [
    {
      "name": "eth0",
      "ip": "192.168.1.42",
      "up": true,
      "loopback": false,
      "recommended": true
    }
  ]
}
```

### Device Management

These endpoints support the device manager and onboarding completion.

Implemented in the current migration slice:

- `GET /devices`
- `POST /devices`
- `PUT /devices/{deviceID}`
- `POST /devices/{deviceID}/test`
- `DELETE /devices/{deviceID}`
- `GET /discovery/ssh-config-hosts`
- `GET /discovery/tailscale/status`
- `GET /discovery/tailscale/peers`

#### `POST /devices`

Create a new device record after a successful bootstrap or manual add flow.

Request:

```json
{
  "label": "gaming-rig",
  "host": "100.64.0.2",
  "connection_type": "tailscale",
  "ssh": {
    "user": "root",
    "port": 22,
    "key_path": "~/.ssh/id_ed25519",
    "password": "",
    "key_passphrase": ""
  },
  "agent": {
    "token": "agent-token"
  }
}
```

#### `PUT /devices/{deviceID}`

Update editable device fields.

#### `POST /devices/{deviceID}/test`

Start a connection test operation.

Response:

```json
{
  "operation_id": "op-test-123"
}
```

#### `POST /devices/{deviceID}/upgrade`

Start an agent upgrade operation.

Planned but not yet implemented:

- `POST /devices/test-all`
- `POST /devices/upgrade-all`
- `POST /devices/{deviceID}/upgrade`
- `DELETE /devices/{deviceID}?cleanup=true`

#### `DELETE /devices/{deviceID}`

Response:

```json
{
  "removed_device_id": "dev-123",
  "cleanup_requested": false,
  "removed_services": 2
}
```

Current behavior removes the device from local config, removes associated local service entries, persists config, and reconnects daemon runtime state. Remote cleanup remains a planned extension.

### Bootstrap and Onboarding

Bootstrap should move to operation-backed daemon services.

#### `POST /bootstrap/preflight`

Request:

```json
{
  "host": "100.64.0.2",
  "connection_type": "tailscale",
  "ssh": {
    "user": "root",
    "port": 22,
    "key_path": "~/.ssh/id_ed25519",
    "password": "",
    "key_passphrase": ""
  }
}
```

Response:

```json
{
  "reachable": true,
  "docker_installed": true,
  "gpu_detected": true,
  "kernel_os": "linux",
  "arch": "amd64",
  "warnings": []
}
```

#### `POST /bootstrap/device`

Implemented in the current migration slice for the OpenTUI add-device wizard.

Current behavior:

- SSH connect to the target host
- run preflight checks
- build a target-specific `yokai` binary locally
- deploy the remote Yokai agent and generate/store an agent token
- persist the device in local config
- hot-reload daemon runtime state so tunnels and polling start automatically

Starts a bootstrap operation.

Request:

```json
{
  "label": "gaming-rig",
  "host": "100.64.0.2",
  "connection_type": "tailscale",
  "ssh": {
    "user": "root",
    "port": 22,
    "key_path": "~/.ssh/id_ed25519",
    "password": "",
    "key_passphrase": ""
  }
}
```

Response:

```json
{
  "device": {
    "id": "100.64.0.2",
    "label": "gaming-rig"
  },
  "preflight": {
    "KernelOS": "Linux",
    "Arch": "x86_64",
    "DockerInstalled": true,
    "GPUDetected": true
  },
  "agent_token": "generated-token",
  "message": "Bootstrapped gaming-rig and deployed the Yokai agent"
}
```

#### `POST /bootstrap/monitoring`

Starts the optional monitoring install operation.

#### `POST /bootstrap/hf-token/validate`

Request:

```json
{
  "token": "hf_xxx"
}
```

Response:

```json
{
  "valid": true,
  "username": "sbull"
}
```

### Settings and History

#### `GET /settings`

Returns the frontend-facing settings document.

#### `PATCH /settings`

Supports partial updates for UI-owned settings sections.

#### `PUT /settings/hf-token`

Request:

```json
{
  "token": "hf_xxx"
}
```

Response:

```json
{
  "configured": true,
  "username": "sbull"
}
```

#### `GET /history/deploy`

Response:

```json
{
  "images": ["vllm/vllm-openai:latest"],
  "models": ["meta-llama/Llama-3.1-8B-Instruct"]
}
```

#### `PUT /history/deploy`

Used to persist updated deploy history.

### Integrations

#### `GET /integrations/openai-endpoints`

Returns normalized candidate endpoints derived from configured running services.

Response:

```json
{
  "endpoints": [
    {
      "service_id": "svc-123",
      "device_id": "dev-123",
      "host": "100.64.0.2",
      "port": 8000,
      "models": ["meta-llama/Llama-3.1-8B-Instruct"],
      "display_name": "gaming-rig / llama-serve"
    }
  ]
}
```

#### `GET /integrations/status`

Returns current integration configuration state.

#### `POST /integrations/configure`

Starts a configuration operation for selected tools.

Request:

```json
{
  "tools": ["vscode", "opencode", "openclaw"],
  "endpoint_service_id": "svc-123"
}
```

Response:

```json
{
  "operation_id": "op-integrations-123"
}
```

## Operations API

Long-running daemon actions should use one shared operations model.

### Supported Kinds

- `bootstrap_device`
- `install_monitoring`
- `device_test`
- `device_upgrade`
- `bulk_device_test`
- `bulk_device_upgrade`
- `configure_integrations`
- `device_cleanup`

### Status Values

- `queued`
- `running`
- `succeeded`
- `failed`
- `cancelled`

### `GET /operations/{id}`

Returns an `OperationSummary`.

### `GET /operations/{id}/events`

Streams operation events over SSE.

Event example:

```text
event: progress
data: {"step":"deploy_agent","percent":45,"message":"Uploading agent binary"}

event: log
data: {"level":"info","message":"docker compose pull complete"}

event: completed
data: {"status":"succeeded","result":{"device_id":"dev-123"}}
```

### Operation Result Shapes

Operation results should be specific to each kind but fit inside one envelope.

Example bootstrap result:

```json
{
  "device_id": "dev-123",
  "agent_token": "agent-token",
  "monitoring_recommended": true
}
```

## Frontend Consumption Rules

The OpenTUI frontend should:

- use polling only for dashboard metrics and operation summaries where SSE is not active
- use SSE for logs and operation event streams
- keep optimistic UI minimal for destructive actions
- treat the daemon as the only backend contract

The frontend should not duplicate backend business logic for:

- config mutation
- connection testing
- bootstrap decision paths
- integration file layout
- Tailscale or SSH discovery

## Delivery Order

Recommended implementation order for this contract:

1. Add settings, history, and discovery endpoints.
2. Add the operations model and device test/upgrade flows.
3. Add bootstrap and monitoring operations.
4. Add integrations endpoints.
5. Port frontend flows incrementally against these APIs.

## Open Questions

- Should current daemon endpoints like `GET /devices` evolve to return richer normalized DTOs, or should new routes be added for the OpenTUI client first?
- Do we want cancellation support for long-running operations in the first pass?
- Should deploy history remain in local config/history files or move under a daemon-owned state abstraction?
- Should local-network discovery remain parity-only for the first migration, or be upgraded to real discovery immediately?
