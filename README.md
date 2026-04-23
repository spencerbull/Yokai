# yokai

[![CI](https://github.com/spencerbull/yokai/actions/workflows/ci.yml/badge.svg)](https://github.com/spencerbull/yokai/actions/workflows/ci.yml)
[![Go Report Card](https://goreportcard.com/badge/github.com/spencerbull/yokai)](https://goreportcard.com/report/github.com/spencerbull/yokai)
[![Latest Release](https://img.shields.io/github/v/release/spencerbull/yokai)](https://github.com/spencerbull/yokai/releases/latest)
[![License: MIT](https://img.shields.io/badge/License-MIT-blue.svg)](LICENSE)

**One binary to deploy, monitor, and manage LLM inference across all your GPUs.**

yokai is a terminal-based fleet manager for running **vLLM**, **llama.cpp**, and **ComfyUI** on any number of GPU machines. Connect your devices, deploy curated Best-Known-Configs (or roll your own through a guided wizard), and watch everything on a btop-style dashboard -- all from a single binary with zero dependencies on the target machines.

```
╭─ yokai ─────────[ Dashboard │ Devices │ Deploy │ Settings ]──── 2 devices ─╮
│ ╭─ gaming-rig · 100.64.0.2 ● ────────────────────────────────────────────╮ │
│ │ GPU 0: RTX 4090  Util 87% [█████████████████░░░]  VRAM 20.1/24.0 GB   │ │
│ ╰────────────────────────────────────────────────────────────────────────╯ │
│ ╭─ Services ─────────────────────────────────────────────────────────────╮ │
│ │▸ vLLM       Llama-3.1-8B-Instruct  gaming-rig  ● live  :8000  142 t/s │ │
│ │  llama.cpp  Mistral-7B-Q4_K_M      gaming-rig  ● live  :8080   38 t/s │ │
│ ╰────────────────────────────────────────────────────────────────────────╯ │
│ Tab switch · j/k move · enter open · g home · esc esc to quit              │
╰────────────────────────────────────────────────────────────────────────────╯
```

---

## Why yokai?

If you're running local LLMs across multiple machines, you already know the pain:

- **SSH into each box** just to check if your GPU is melting or idle
- **Copy-paste 20-flag `docker run` commands** every time you want to swap a model
- **Juggle separate monitoring dashboards** for each machine
- **Manually edit tool configs** every time an endpoint changes

yokai solves all of this with a single binary. Install it, point it at your machines, and you're running models in minutes -- not hours.

---

## Features

### Fleet Management
- **Onboarding wizard** -- connect devices via LAN scan, Tailscale peer discovery, or manual IP entry
- **SSH bootstrap** -- pre-flight checks (Docker, GPU, disk space), agent deployment, and systemd service installation in one step
- **Device manager** -- add, edit, remove, and test connectivity for all devices from the TUI
- **Secure by default** -- auto-generated bearer tokens for agent authentication, SSH key resolution with agent/key/password fallback

### Workload Deployment
- **Best Known Configs (BKC)** -- pick from a built-in catalog of pre-validated vLLM and llama.cpp deploys grouped by vendor (NVIDIA, OpenAI, Meta/Llama, Google, Mistral, Qwen, DeepSeek, GLM, Moonshot, Microsoft, and more) and filtered to the GPUs each device actually has
- **Guided deploy wizard** -- pick workload type, target device, Docker image, model, and runtime config when you want to deviate from the catalog
- **HuggingFace integration** -- search models directly, browse GGUF quantizations, auto-download during deployment
- **VRAM estimator** -- `hf-mem`-backed memory estimate for vLLM weights + KV cache before you commit to a deploy
- **Docker image catalog** -- browse official tags from Docker Hub and GHCR, including nightly builds
- **Plugin system** -- model-specific add-ons (e.g. Nemotron Super V3 reasoning parser) that fetch assets, mount them into the container, and append the right runtime flags
- **GPU-aware deployment** -- automatic `--gpus` flag configuration, multi-GPU tensor parallelism support

### Live Monitoring
- **btop-style dashboard** -- live GPU utilization, VRAM, temperature, power draw, and fan speed per GPU
- **System metrics** -- CPU/RAM sparklines with 60-sample rolling history, disk usage, swap
- **Container metrics** -- per-container CPU, memory, GPU memory, and uptime
- **Service lifecycle** -- stop, restart, and remove containers directly from the dashboard

### Monitoring Stack
- **Auto-provisioned** -- Prometheus + Grafana + node_exporter + dcgm-exporter seeded onto each device during bootstrap, with the agent bearer token wired in for authenticated scrapes
- **Pre-built dashboards** -- Grafana dashboard with GPU utilization, temperature, power, and system metrics panels
- **Live in the dashboard** -- the monitoring stack shows up as its own services panel alongside your AI workloads

### AI Coding Tool Integration
- **One-shot config** -- the Settings route auto-configures every supported AI coding tool at once
- **VS Code Copilot** -- appends OpenAI-compatible models to `chat.models[]` in the VS Code user `settings.json`
- **OpenCode** -- registers a per-host provider in `~/.config/opencode/opencode.json` via `@ai-sdk/openai-compatible`
- **OpenClaw** -- adds a yokai provider under `models.providers` in `~/.openclaw/openclaw.json`
- **Claude Code** -- writes `ANTHROPIC_BASE_URL`, `ANTHROPIC_MODEL`, and `ANTHROPIC_CUSTOM_MODEL_OPTION` into `~/.claude/settings.json`
- **Codex** -- adds a `[model_providers.yokai]` section to `~/.codex/config.toml`
- **Backup-safe** -- writes a `.yokai.bak` of every config before modifying it
- **Multi-endpoint** -- registers every running inference service as an available endpoint

### Self-Updating
- **`yokai upgrade`** -- checks GitHub Releases, downloads the correct binary for your OS/arch, and replaces itself in place
- **Cross-platform** -- builds for Linux, macOS, and Windows (amd64 + arm64)
- **One-line install** -- `curl | sh` installer that detects your platform automatically

---

## Quick Start

### Install

**One-line installer** (Linux/macOS):

```bash
curl -fsSL https://raw.githubusercontent.com/spencerbull/Yokai/main/install.sh | sh
```

**From GitHub Releases:**

Download the latest binary for your platform from the [Releases page](https://github.com/spencerbull/yokai/releases/latest), extract it, and move it to your PATH.

**Build from source:**

```bash
git clone https://github.com/spencerbull/yokai.git
cd yokai
make build
./bin/yokai
```

### First Run

```bash
# 1. Launch yokai -- OpenTUI starts and auto-starts the local daemon if needed
yokai

# 2. Select how to connect (LAN / Tailscale / Manual IP)
# 3. Enter SSH credentials -- yokai tests the connection
# 4. Bootstrap runs: pre-flight checks → agent deploy → monitoring stack
# 5. Set up your HuggingFace token (optional, for gated models)
# 6. Tab over to "Deploy" and pick a Best-Known-Config to run your first model
```

### Running the Daemon

The daemon is a background process on your local machine that maintains SSH tunnels to your devices and polls agents for live metrics. `yokai` now auto-starts it on first launch, but you can still run it manually:

```bash
# Start the daemon in the background
yokai daemon &

# Launch OpenTUI (auto-starts daemon if needed)
yokai
```

---

## Usage

### Commands

**Core**

| Command | Description |
|---|---|
| `yokai` | Launch OpenTUI (default; auto-starts the daemon) |
| `yokai agent [port]` | Run the agent on a target device (default port: 7474) |
| `yokai daemon` | Start the local background daemon (default `127.0.0.1:7473`) |
| `yokai upgrade` | Update to the latest release |
| `yokai version` | Print version and build info |

**Device management**

| Command | Description |
|---|---|
| `yokai devices list` | List all configured devices |
| `yokai devices add --host <host> [flags]` | Add a device |
| `yokai devices remove <device-id>` | Remove a device |
| `yokai devices test <device-id>` | Test SSH + agent connectivity |
| `yokai devices bootstrap <device-id>` | Install or upgrade the agent on a device |

**Service management**

| Command | Description |
|---|---|
| `yokai services list [--device <id>]` | List containers across the fleet (or one device) |
| `yokai services deploy --device <id> [flags]` | Deploy a service |
| `yokai services stop <device-id> <cid>` | Stop a container |
| `yokai services restart <device-id> <cid>` | Restart a container |
| `yokai services logs [--follow] <did> <cid>` | Stream container logs |

**Fleet status & config**

| Command | Description |
|---|---|
| `yokai status` | Fleet overview (JSON) |
| `yokai metrics [--device <id>]` | Detailed device metrics (JSON) |
| `yokai config show` | Dump config (tokens redacted) |
| `yokai config set <key> <value>` | Set a config value |
| `yokai config path` | Print the config file path |

All non-TUI commands emit JSON on stdout and JSON-formatted errors on stderr, so they're scriptable from shell or another tool.

### Keybinds

**Global**

| Key | Action |
|---|---|
| `Tab` / `Shift+Tab` | Cycle between Dashboard / Devices / Deploy / Settings |
| `1` … `4` | Jump to a top-level route by number |
| `g` | Return to the landing screen |
| `Esc` (twice) or `Ctrl+C` (twice) | Quit |

**Dashboard**

| Key | Action |
|---|---|
| `j` / `k` (or `↑` / `↓`) | Move selection within the service list |
| `Tab` | Switch between AI services and the monitoring stack |
| `Enter` or `l` | Open the selected service's detail view |
| `Shift+L` | Open logs for the selected service |
| `s` / `r` / `t` / `x` | Stop / restart / test / delete (in service detail) |
| `y` / `n` | Confirm or cancel a destructive action |

---

## How It Works

yokai uses a three-tier architecture: **OpenTUI** (what you see, a TypeScript app rendered in your terminal), the **Daemon** (runs locally and brokers everything), and the **Agent** (runs on each GPU device).

```
Your Machine                              GPU Device(s)
┌──────────────────────┐                  ┌──────────────────────┐
│  OpenTUI (Bun/Node)  │                  │  yokai agent         │
│  ├── Dashboard       │   HTTP :7473     │  ├── REST API :7474  │
│  ├── Deploy Wizard   │◄───────────────► │  ├── nvidia-smi      │
│  ├── Device Manager  │                  │  ├── Docker engine   │
│  └── Log Viewer      │                  │  └── System metrics  │
│         ▲            │                  │                      │
│         │ launches   │                  │  Docker containers   │
│         ▼            │   SSH tunnel     │  ├── vLLM :8000      │
│  yokai daemon        │◄═══════════════► │  ├── llama.cpp :8080 │
│  ├── SSH Tunnels     │                  │  └── ComfyUI :8188   │
│  ├── Metrics Agg.    │                  │                      │
│  ├── BKC Catalog     │                  │  Monitoring stack    │
│  ├── HF + hf-mem     │                  │  ├── Prometheus      │
│  └── Tool Configs    │                  │  ├── Grafana         │
│                      │                  │  ├── node_exporter   │
│  ~/.config/yokai/    │                  │  └── dcgm-exporter   │
│  └── config.json     │                  │                      │
└──────────────────────┘                  └──────────────────────┘
```

**Launch sequence**

1. `yokai` checks whether the daemon is healthy; if not, it spawns `yokai daemon` in the background and waits for `/health`
2. It then runs the bundled `yokai-tui` binary (preferred) or, if you're on a dev checkout with `bun` installed, `bun run src/index.tsx` from `ui/tui/` -- the daemon URL is passed via `YOKAI_DAEMON_URL`
3. The TUI talks to the daemon over HTTP on `127.0.0.1:7473`

**Data flow**

1. The daemon opens SSH tunnels to each device and forwards a local port to each agent's `:7474` REST API
2. Every few seconds it polls each agent's `/metrics` endpoint and caches the results in memory
3. The TUI reads aggregated metrics from the daemon and renders the dashboard
4. Deploy and lifecycle commands (deploy/stop/restart/remove/logs) flow TUI -> daemon -> agent -> Docker

---

## Configuration

All state lives in `~/.config/yokai/config.json`. Copy this file to another machine to reconnect to your fleet instantly.

```json
{
  "version": 1,
  "hf_token": "hf_...",
  "daemon": {
    "listen": "127.0.0.1:7473",
    "metrics_poll_interval_s": 2,
    "reconnect_interval_s": 30
  },
  "devices": [
    {
      "id": "gaming-rig",
      "label": "Gaming Rig",
      "host": "100.64.0.2",
      "ssh_user": "user",
      "connection_type": "tailscale",
      "agent_port": 7474,
      "agent_token": "a1b2c3...",
      "gpu_type": "nvidia",
      "tags": ["rtx-4090"],
      "monitoring_installed": true
    }
  ],
  "services": [
    {
      "id": "yokai-vllm-llama3",
      "device_id": "gaming-rig",
      "type": "vllm",
      "image": "vllm/vllm-openai:latest",
      "model": "meta-llama/Llama-3.1-8B-Instruct",
      "port": 8000,
      "plugins": [],
      "runtime": { "ipc_mode": "host", "shm_size": "16g" }
    }
  ],
  "preferences": {
    "theme": "tokyonight",
    "default_vllm_image": "vllm/vllm-openai:latest",
    "default_llama_image": "ghcr.io/ggml-org/llama.cpp:server-cuda",
    "default_comfyui_image": "yanwk/comfyui-boot:latest"
  }
}
```

---

## Supported Platforms

### User Machine (where you run `yokai`)

| OS | Architecture | Status |
|---|---|---|
| Linux | amd64 | Supported |
| Linux | arm64 | Supported |
| macOS | amd64 | Supported |
| macOS | arm64 (Apple Silicon) | Supported |
| Windows | amd64 | Supported |
| Windows | arm64 | Supported |

### Target Devices (where models run)

| Requirement | Details |
|---|---|
| OS | Linux (Ubuntu 20.04+, Debian 11+, or similar) |
| Docker | 20.10+ with the Docker CLI available |
| GPU | NVIDIA with `nvidia-container-toolkit` installed |
| Network | Reachable via SSH (LAN, Tailscale, or public IP) |
| Disk | 20GB+ free recommended (vLLM images are 10GB+) |

---

## Architecture Documentation

Detailed multi-level architecture docs are available in the [`architecture/`](architecture/README.md) directory:

| Level | Document | What It Covers |
|---|---|---|
| L1 | [System Overview](architecture/01-system-overview.md) | Bird's-eye view of the system |
| L2 | [Component Architecture](architecture/02-component-architecture.md) | Go package structure and dependencies |
| L3 | [Data Flow](architecture/03-data-flow.md) | Metrics, deployment, logging, and config data paths |
| L4 | [Network Topology](architecture/04-network-topology.md) | SSH tunnels, ports, authentication, Tailscale |
| L5 | [TUI Screen Map](architecture/05-tui-screen-map.md) | View hierarchy and navigation state machine |
| L6 | [Agent API](architecture/06-agent-api.md) | Full REST API contract with JSON schemas |
| L7 | [Daemon UI API](architecture/07-daemon-ui-api.md) | Daemon REST API consumed by OpenTUI |

---

## Project Structure

```
yokai/
├── cmd/yokai/             # Binary entry point and subcommand dispatch
├── internal/
│   ├── agent/             # Remote agent: REST API, Docker ops, system metrics (port :7474)
│   ├── bkc/               # Best Known Configs catalog (per-vendor catalog_*.go files)
│   ├── claudecode/        # Claude Code (~/.claude/settings.json) endpoint registration
│   ├── cli/               # Non-TUI subcommand handlers (devices/services/status/metrics/config)
│   ├── codex/             # Codex (~/.codex/config.toml) endpoint registration
│   ├── config/            # Config load/save/migrate, deploy history (~/.config/yokai/)
│   ├── daemon/            # Local daemon: REST API (:7473), SSH tunnels, metrics aggregation
│   ├── docker/            # Docker Hub/GHCR tag catalog and image helpers
│   ├── hf/                # HuggingFace API: model search, GGUF listing
│   ├── hfmem/             # `hf-mem` wrapper for vLLM weight + KV-cache memory estimation
│   ├── monitoring/        # Provisions Prometheus/Grafana/exporters onto remote devices
│   ├── openclaw/          # OpenClaw openclaw.json provider registration
│   ├── opencode/          # OpenCode opencode.json provider registration
│   ├── opentui/           # OpenTUI launcher (daemon health check + bundled/Bun spawn)
│   ├── platform/          # Cross-platform shims (e.g. chmod no-op on Windows)
│   ├── plugins/           # Plugin catalog: assets, mounts, runtime-flag overrides per model
│   ├── ssh/               # SSH client, SCP upload, bootstrap/deploy
│   ├── tailscale/         # Tailscale CLI wrapper for peer discovery
│   ├── upgrade/           # Self-update from GitHub Releases
│   └── vscode/            # VS Code settings.json (chat.models[]) manipulation
├── ui/tui/                # OpenTUI: TypeScript/React TUI run by Bun
│   ├── src/
│   │   ├── app/           # App shell and routing
│   │   ├── contracts/     # Daemon API types
│   │   ├── features/      # Dashboard, deploy, devices, logs, integrations, etc.
│   │   ├── services/      # Daemon HTTP client
│   │   └── theme/         # Color palette and styles
│   └── package.json
├── assets/
│   ├── grafana/           # Pre-built dashboard JSON and provisioning
│   ├── prometheus/        # Prometheus scrape configuration
│   └── systemd/           # Agent systemd service template
├── architecture/          # Multi-level architecture documentation
├── docker/                # Reference Dockerfiles (e.g. comfyui/)
├── .github/workflows/     # CI (build/test/lint) and release (GoReleaser)
├── .goreleaser.yml        # Cross-compilation and release config
├── install.sh             # curl-pipe-sh installer
├── Makefile               # Build, test, lint, cross-compile targets
└── go.mod
```

---

## Contributing

Contributions are welcome. See [CONTRIBUTING.md](CONTRIBUTING.md) for development setup, coding guidelines, and the pull request process.

---

## License

[MIT](LICENSE)
