# L2: Component Architecture

Internal structure of each major component, mapped to Go packages.

## Package Dependency Graph

```
cmd/yokai/main.go
├── internal/tui/          (TUI mode)
├── internal/daemon/       (daemon mode)
└── internal/agent/        (agent mode)

internal/tui/
├── app.go                 ← Root Bubbletea model
├── views/                 ← One file per screen
│   ├── welcome.go
│   ├── localnet.go
│   ├── tailscale.go
│   ├── manual.go
│   ├── sshcreds.go
│   ├── bootstrap.go
│   ├── hftoken.go
│   ├── dashboard.go
│   ├── deploy.go
│   ├── devices.go
│   ├── logs.go
│   ├── copilot.go
│   └── help.go
├── components/            ← Reusable UI widgets
│   ├── metricsbar.go
│   ├── sparkline.go
│   ├── gpupanel.go
│   ├── servicelist.go
│   ├── devicecard.go
│   ├── keybinds.go
│   ├── stepper.go
│   └── overlay.go
└── theme/
    └── theme.go           ← Colors, borders, styles

internal/daemon/
├── daemon.go              ← HTTP server on :7473, lifecycle
├── aggregator.go          ← Polls agents, stores ring buffers
└── tunnel.go              ← SSH tunnel pool management

internal/agent/
├── server.go              ← HTTP server on :7474
├── handlers.go            ← Route handlers
├── metrics.go             ← nvidia-smi / rocm-smi / procfs
└── docker.go              ← Docker SDK wrapper

internal/ssh/
├── client.go              ← Connect, exec, SCP
└── bootstrap.go           ← Deploy agent binary + systemd

internal/tailscale/
└── tailscale.go           ← CLI wrapper (status --json)

internal/docker/
├── catalog.go             ← Fetch tags from Docker Hub / GHCR
└── compose.go             ← Monitoring stack template

internal/hf/
└── client.go              ← HuggingFace API (model search, GGUF files)

internal/config/
└── config.go              ← JSON load/save/migrate/defaults

internal/vscode/
└── settings.go            ← Read/merge/write VS Code settings.json
```

## Component Details

### TUI (`internal/tui/`)

```
┌─ app.go ─────────────────────────────────────────────────────────┐
│                                                                   │
│  type App struct {                                                │
│      currentView   View        // active screen                  │
│      viewStack     []View      // navigation history (for Esc)   │
│      daemonClient  *DaemonAPI  // HTTP client to localhost:7473  │
│      config        *Config     // loaded from disk               │
│      width, height int         // terminal dimensions            │
│  }                                                                │
│                                                                   │
│  View interface {                                                 │
│      Init() tea.Cmd                                               │
│      Update(msg tea.Msg) (View, tea.Cmd)                         │
│      View() string                                                │
│      KeyBinds() []KeyBind   // for the keybind bar               │
│  }                                                                │
│                                                                   │
│  Navigation:                                                      │
│    pushView(v)  → push current to stack, set v as active         │
│    popView()    → restore previous from stack (Esc behavior)     │
│    replaceView(v) → swap without pushing to stack                │
│                                                                   │
└───────────────────────────────────────────────────────────────────┘
```

### Daemon (`internal/daemon/`)

```
┌─ daemon.go ──────────────────────────────────────────────────────┐
│                                                                   │
│  Lifecycle:                                                       │
│    1. Load config.json                                            │
│    2. For each device: establish SSH tunnel → agent :7474         │
│    3. Start metrics polling goroutines (one per device)           │
│    4. Serve REST API on localhost:7473                            │
│    5. Watch config file for changes (fsnotify)                    │
│                                                                   │
│  ┌─ aggregator.go ────────────────────────────────────────────┐  │
│  │  Per-device goroutine:                                      │  │
│  │    loop every 2s:                                           │  │
│  │      GET agent:7474/metrics → parse → store in ring buffer  │  │
│  │      ring buffer: 60 points per metric (2min window)        │  │
│  │    on error: mark device offline, retry every 30s           │  │
│  └─────────────────────────────────────────────────────────────┘  │
│                                                                   │
│  ┌─ tunnel.go ────────────────────────────────────────────────┐  │
│  │  SSH tunnel pool:                                           │  │
│  │    - One tunnel per device                                  │  │
│  │    - Local port → remote :7474                              │  │
│  │    - Keepalive every 30s                                    │  │
│  │    - Auto-reconnect on disconnect                           │  │
│  └─────────────────────────────────────────────────────────────┘  │
│                                                                   │
└───────────────────────────────────────────────────────────────────┘
```

### Agent (`internal/agent/`)

```
┌─ agent (runs on target device) ──────────────────────────────────┐
│                                                                   │
│  server.go                                                        │
│    - chi router on :7474                                         │
│    - Bearer token auth middleware                                 │
│    - CORS disabled (not browser-facing)                          │
│                                                                   │
│  handlers.go                                                      │
│    - GET  /health          → { version, uptime }                 │
│    - GET  /system/info     → { gpus, cpu, ram, disk, docker }    │
│    - GET  /metrics         → { cpu%, ram, gpu[], containers[] }  │
│    - GET  /containers      → list managed containers             │
│    - POST /containers      → deploy workload (JSON spec)         │
│    - DEL  /containers/:id  → stop + remove                      │
│    - GET  /containers/:id/logs → SSE streaming                   │
│    - POST /containers/:id/restart                                │
│    - POST /images/pull     → pull image, SSE progress            │
│    - GET  /images/tags/:img → fetch registry tags                │
│                                                                   │
│  metrics.go                                                       │
│    - nvidia-smi --query-gpu (utilization, memory, temp, power)   │
│    - /proc/stat, /proc/meminfo (CPU, RAM)                        │
│    - df (disk)                                                    │
│    - docker stats per container                                   │
│                                                                   │
│  docker.go                                                        │
│    - Docker SDK client                                            │
│    - Container lifecycle: create, start, stop, remove, logs      │
│    - Image pull with progress callback                            │
│    - List containers filtered by yokai-* prefix               │
│    - Re-adopt existing containers on agent startup                │
│                                                                   │
└───────────────────────────────────────────────────────────────────┘
```
