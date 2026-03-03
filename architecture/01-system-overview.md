# L1: System Overview

Bird's-eye view of yokai — what runs where and how components connect.

## System Diagram

```
┌─────────────────────────────────────────────────────────────────────────┐
│                           USER'S MACHINE                                │
│                                                                         │
│  ┌───────────────────────────────────────────────────────────────────┐  │
│  │  yokai TUI                                                     │  │
│  │  ┌─────────────────────────────────────────────────────────────┐  │  │
│  │  │  Bubbletea Event Loop                                       │  │  │
│  │  │  ├─ View Router (welcome → dashboard → wizards)             │  │  │
│  │  │  ├─ Renders btop-style metrics panels                       │  │  │
│  │  │  └─ Sends commands to Daemon via localhost:7473              │  │  │
│  │  └─────────────────────────────────────────────────────────────┘  │  │
│  └───────────────────────────────────────────────────────────────────┘  │
│         ▲                                                               │
│         │ HTTP (localhost:7473)                                          │
│         ▼                                                               │
│  ┌───────────────────────────────────────────────────────────────────┐  │
│  │  yokai daemon (background service)                             │  │
│  │  ├─ Maintains SSH tunnels to all registered devices               │  │
│  │  ├─ Polls agents for metrics (every 2s)                           │  │
│  │  ├─ Aggregates metrics into ring buffers                          │  │
│  │  ├─ Forwards deploy/stop/restart commands to agents               │  │
│  │  └─ Exposes REST API on localhost:7473 for TUI consumption        │  │
│  └───────────────────────────────────────────────────────────────────┘  │
│         │                                                               │
│  ┌──────┴────────────────────────────────────────────────────────────┐  │
│  │  ~/.config/yokai/config.json                                   │  │
│  │  ├─ Device registry (hosts, SSH keys, connection types)           │  │
│  │  ├─ Service definitions (images, models, ports)                   │  │
│  │  ├─ HuggingFace token                                             │  │
│  │  └─ Preferences (theme, defaults, poll intervals)                 │  │
│  └───────────────────────────────────────────────────────────────────┘  │
│                                                                         │
└────────────────────────────┬────────────────────────────────────────────┘
                             │
                             │  SSH tunnels / REST API
                             │  (over Tailscale VPN or LAN)
                             │
          ┌──────────────────┼──────────────────┐
          │                  │                  │
          ▼                  ▼                  ▼
┌──────────────────┐ ┌──────────────────┐ ┌──────────────────┐
│  TARGET DEVICE 1 │ │  TARGET DEVICE 2 │ │  TARGET DEVICE N │
│  (gaming-rig)    │ │  (workstation)   │ │  (...)           │
│                  │ │                  │ │                  │
│  yokai agent  │ │  yokai agent  │ │  yokai agent  │
│  ├─ REST :7474   │ │  ├─ REST :7474   │ │  ├─ REST :7474   │
│  ├─ Docker SDK   │ │  ├─ Docker SDK   │ │  ├─ Docker SDK   │
│  ├─ GPU metrics  │ │  ├─ GPU metrics  │ │  ├─ GPU metrics  │
│  └─ Sys metrics  │ │  └─ Sys metrics  │ │  └─ Sys metrics  │
│                  │ │                  │ │                  │
│  Monitoring:     │ │  Monitoring:     │ │  Monitoring:     │
│  ├─ Prometheus   │ │  ├─ Prometheus   │ │  ├─ Prometheus   │
│  ├─ Grafana      │ │  ├─ Grafana      │ │  ├─ Grafana      │
│  ├─ node-export  │ │  ├─ node-export  │ │  ├─ node-export  │
│  └─ dcgm-export  │ │  └─ dcgm-export  │ │  └─ dcgm-export  │
│                  │ │                  │ │                  │
│  Workloads:      │ │  Workloads:      │ │  Workloads:      │
│  ├─ vLLM :8000   │ │  ├─ llama.cpp    │ │  └─ (none yet)   │
│  └─ ComfyUI :8188│ │  │   :8080       │ │                  │
│                  │ │  └─ vLLM :8000   │ │                  │
└──────────────────┘ └──────────────────┘ └──────────────────┘
```

## Key Concepts

### Single Binary
`yokai` is one Go binary with three modes:
- `yokai` — launches the TUI (default)
- `yokai daemon` — runs the local background service
- `yokai agent` — runs the remote agent on target devices

The TUI SCPs itself to target devices during bootstrap and runs `yokai agent` there.

### Three-Tier Architecture
1. **TUI** — pure view layer, stateless, can quit/relaunch freely
2. **Daemon** — persistent local service, maintains connections and state
3. **Agent** — runs on each target device, manages Docker containers and collects metrics

### Connection Types
- **Tailscale** — devices connected via Tailscale VPN mesh (100.x.x.x addresses)
- **LAN** — devices on the same local network (192.168.x.x, 10.x.x.x)
- **Manual** — any reachable hostname or IP

### Config Portability
`~/.config/yokai/config.json` contains the entire fleet state. Copy it to a new machine, run `yokai`, and the daemon reconnects to all existing agents automatically.
