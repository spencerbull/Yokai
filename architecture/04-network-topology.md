# L4: Network Topology

Ports, protocols, authentication, SSH tunnels, and connectivity.

## Port Map

| Port | Service | Location | Protocol | Auth |
|------|---------|----------|----------|------|
| 7473 | Daemon REST API | User machine (localhost only) | HTTP | None (loopback) |
| 7474 | Agent REST API | Target device | HTTP | Bearer token |
| 22 | SSH | Target device | SSH | Key / agent / password |
| 8000 | vLLM (default) | Target device | HTTP | None |
| 8080 | llama.cpp (default) | Target device | HTTP | None |
| 8188 | ComfyUI (default) | Target device | HTTP | None |
| 9090 | Prometheus | Target device | HTTP | None |
| 3000 | Grafana | Target device | HTTP | admin/admin default |

## Connection Topology

### Tailscale Connection

```
User Machine                                Target Device
(100.64.0.1)                               (100.64.0.2)

┌──────────────┐    Tailscale WireGuard    ┌──────────────┐
│ Daemon       │◄══════════════════════════►│ Agent :7474  │
│  SSH tunnel  │    encrypted tunnel        │              │
│  localhost   │    (100.64.0.1 ↔           │ Docker       │
│  :random ────┼──► 100.64.0.2:22)         │ ├─ vLLM      │
│              │                            │ ├─ llama.cpp │
│ TUI          │                            │ └─ ComfyUI   │
│  localhost   │                            │              │
│  :7473 ──────┤                            │ Prometheus   │
│              │                            │ Grafana      │
└──────────────┘                            └──────────────┘
```

Tailscale provides:
- Encrypted WireGuard tunnel between all devices
- Stable IP addresses (100.64.x.x) regardless of physical network
- NAT traversal — works across different networks, firewalls
- No port forwarding needed on routers

### LAN Connection

```
User Machine                                Target Device
(192.168.1.10)                              (192.168.1.42)

┌──────────────┐    Local Network           ┌──────────────┐
│ Daemon       │◄──────────────────────────►│ Agent :7474  │
│  SSH tunnel  │    SSH over LAN            │              │
│  localhost   │    (192.168.1.10 →          │ Docker       │
│  :random ────┼──► 192.168.1.42:22)        │ ├─ vLLM      │
└──────────────┘                            └──────────────┘
```

LAN requires:
- Both devices on the same network (or routable)
- SSH port 22 accessible on target
- Agent port 7474 accessible (or tunneled via SSH)

## SSH Tunnel Setup

The daemon establishes SSH tunnels to avoid exposing agent ports directly:

```
Daemon                          Target Device
  │                                  │
  │  1. SSH connect                  │
  │  ─────────────────────────────►  │
  │     user@host:22                 │
  │     key: ~/.ssh/id_ed25519       │
  │                                  │
  │  2. Request port forward         │
  │  ─────────────────────────────►  │
  │     local :0 → remote :7474     │
  │     (OS picks random local port) │
  │                                  │
  │  3. Tunnel established           │
  │  ◄─────────────────────────────  │
  │     local :48721 → remote :7474 │
  │                                  │
  │  4. HTTP via tunnel              │
  │  GET localhost:48721/metrics     │
  │  ─────────────────────────────►  │
  │     → forwarded to :7474        │
  │                                  │
  │  5. Keepalive every 30s         │
  │  ─────────────────────────────►  │
  │                                  │
  │  6. On disconnect: reconnect     │
  │     (exponential backoff,        │
  │      max 5min between retries)  │
```

## Authentication

### Agent API Auth

```
Bootstrap generates:
  agent_token = crypto/rand 32 bytes → hex encoded

Stored in:
  config.json (user machine): devices[].agent_token
  /etc/yokai/agent.json (target): { "token": "..." }

Every request to agent:
  Authorization: Bearer <agent_token>

Agent middleware:
  if header != stored token → 401 Unauthorized
```

### SSH Auth Resolution

```
Priority order:
  1. config.json ssh_key field for this device
  2. SSH agent (SSH_AUTH_SOCK)
  3. ~/.ssh/id_ed25519
  4. ~/.ssh/id_rsa
  5. Interactive password prompt (TUI only, not daemon)
```

## Firewall Considerations

| Scenario | What needs to be open | Notes |
|----------|----------------------|-------|
| Tailscale | Nothing extra | WireGuard handles traversal |
| LAN | SSH :22 on target | Agent accessed via SSH tunnel |
| LAN + direct | SSH :22 + agent :7474 | If not using SSH tunnel |
| Workload access | :8000, :8080, :8188 | For VS Code / API consumers |
| Grafana access | :3000 | Browser on user machine → target |

## Endpoint Access from External Consumers

When VS Code or other tools want to hit the model endpoint:

```
VS Code (user machine)
  │
  │ http://100.64.0.2:8000/v1/chat/completions
  │ (Tailscale IP, direct)
  │
  │ OR
  │
  │ http://192.168.1.42:8000/v1/chat/completions
  │ (LAN IP, direct)
  │
  ▼
Target Device
  │
  │ Docker port mapping: -p 8000:8000
  ▼
vLLM container
```

Model endpoints are accessed directly (not through the daemon/agent).
The daemon only manages lifecycle and metrics — actual inference traffic goes direct.
