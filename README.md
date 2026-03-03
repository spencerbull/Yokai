# yokai

[![CI](https://github.com/spencerbull/yokai/actions/workflows/ci.yml/badge.svg)](https://github.com/spencerbull/yokai/actions/workflows/ci.yml)
[![Go Report Card](https://goreportcard.com/badge/github.com/spencerbull/yokai)](https://goreportcard.com/report/github.com/spencerbull/yokai)
[![Latest Release](https://img.shields.io/github/v/release/spencerbull/yokai)](https://github.com/spencerbull/yokai/releases/latest)
[![License: MIT](https://img.shields.io/badge/License-MIT-blue.svg)](LICENSE)

**One binary to deploy, monitor, and manage LLM inference across all your GPUs.**

yokai is a terminal-based fleet manager for running **vLLM**, **llama.cpp**, and **ComfyUI** on any number of GPU machines. Connect your devices, deploy models through a guided wizard, and watch everything on a btop-style dashboard -- all from a single binary with zero dependencies on the target machines.

```
╭─ yokai ──────────────────────────────────────────────────── 2 devices ─╮
│ ╭─ gaming-rig · 100.64.0.2 ● ──────────────────────────────────────────╮ │
│ │ GPU 0: RTX 4090  Util 87% [█████████████████░░░]  VRAM 20.1/24.0 GB │ │
│ ╰──────────────────────────────────────────────────────────────────────╯ │
│ ╭─ Services ───────────────────────────────────────────────────────────╮ │
│ │▸ vLLM  Llama-3.1-8B-Instruct  gaming-rig  ● live  :8000  142 t/s  │ │
│ │  llama.cpp  Mistral-7B-Q4_K_M  gaming-rig  ● live  :8080   38 t/s  │ │
│ ╰──────────────────────────────────────────────────────────────────────╯ │
│ n new  s stop  l logs  d devices  g grafana  c copilot  ? help  q quit  │
╰──────────────────────────────────────────────────────────────────────────╯
```

---

## Why yokai?

If you're running local LLMs across multiple machines, you already know the pain:

- **SSH into each box** just to check if your GPU is melting or idle
- **Copy-paste 20-flag `docker run` commands** every time you want to swap a model
- **Juggle separate monitoring dashboards** for each machine
- **Manually edit VS Code settings** every time an endpoint changes

yokai solves all of this with a single binary. Install it, point it at your machines, and you're running models in minutes -- not hours.

---

## Features

### Fleet Management
- **Onboarding wizard** -- connect devices via LAN scan, Tailscale peer discovery, or manual IP entry
- **SSH bootstrap** -- pre-flight checks (Docker, GPU, disk space), agent deployment, and systemd service installation in one step
- **Device manager** -- add, edit, remove, and test connectivity for all devices from the TUI
- **Secure by default** -- auto-generated bearer tokens for agent authentication, SSH key resolution with agent/key/password fallback

### Workload Deployment
- **Guided deploy wizard** -- 5-step flow: pick workload type, target device, Docker image, model, and configuration
- **HuggingFace integration** -- search models directly, browse GGUF quantizations, auto-download during deployment
- **Docker image catalog** -- browse official tags from Docker Hub and GHCR, including nightly builds
- **GPU-aware deployment** -- automatic `--gpus` flag configuration, VRAM estimation, multi-GPU tensor parallelism support

### Live Monitoring
- **btop-style dashboard** -- live GPU utilization, VRAM, temperature, power draw, and fan speed per GPU
- **System metrics** -- CPU/RAM sparklines with 60-sample rolling history, disk usage, swap
- **Container metrics** -- per-container CPU, memory, GPU memory, and uptime
- **Service lifecycle** -- stop, restart, and remove containers directly from the dashboard

### Monitoring Stack
- **Auto-deployed** -- Prometheus + Grafana + node_exporter + dcgm-exporter deployed during device bootstrap
- **Pre-built dashboards** -- Grafana dashboard with GPU utilization, temperature, power, and system metrics panels
- **One-key access** -- press `g` to open Grafana in your browser

### VS Code Integration
- **Auto-configure Copilot** -- press `c` to write OpenAI-compatible endpoints into your VS Code `settings.json`
- **Backup-safe** -- creates a backup of your settings before modifying them
- **Multi-endpoint** -- registers all running inference services as available endpoints

### Self-Updating
- **`yokai upgrade`** -- checks GitHub Releases, downloads the correct binary for your OS/arch, and replaces itself in place
- **Cross-platform** -- builds for Linux (amd64/arm64) and macOS (arm64)
- **One-line install** -- `curl | sh` installer that detects your platform automatically

---

## Quick Start

### Install

**One-line installer** (Linux/macOS):

```bash
curl -fsSL https://raw.githubusercontent.com/spencerbull/yokai/main/install.sh | sh
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
# 1. Launch yokai -- the onboarding wizard starts automatically
yokai

# 2. Select how to connect (LAN / Tailscale / Manual IP)
# 3. Enter SSH credentials -- yokai tests the connection
# 4. Bootstrap runs: pre-flight checks → agent deploy → monitoring stack
# 5. Set up your HuggingFace token (optional, for gated models)
# 6. You're on the dashboard -- press 'n' to deploy your first model
```

### Running the Daemon

The daemon is a background process on your local machine that maintains SSH tunnels to your devices and polls agents for live metrics. Start it before (or alongside) the TUI:

```bash
# Start the daemon in the background
yokai daemon &

# Launch the TUI (connects to daemon automatically)
yokai
```

---

## Usage

### Commands

| Command | Description |
|---|---|
| `yokai` | Launch the TUI (default) |
| `yokai agent [port]` | Run the agent on a target device (default port: 7474) |
| `yokai daemon` | Start the local background daemon |
| `yokai upgrade` | Update to the latest release |
| `yokai version` | Print version and build info |

### Dashboard Keybinds

| Key | Action |
|---|---|
| `n` | Deploy a new service |
| `s` | Stop selected service |
| `r` | Restart selected service |
| `l` | View logs for selected service |
| `d` | Open device manager |
| `g` | Open Grafana in browser |
| `c` | Configure VS Code Copilot endpoints |
| `?` | Show help overlay |
| `j`/`k` | Navigate service list |
| `q` | Quit |

---

## How It Works

yokai uses a three-tier architecture: **TUI** (what you see), **Daemon** (runs locally), and **Agent** (runs on each GPU device).

```
Your Machine                              GPU Device(s)
┌──────────────────────┐                  ┌──────────────────────┐
│                      │                  │  yokai agent         │
│  yokai (TUI)         │   HTTP :7473     │  ├── REST API :7474  │
│  ├── Dashboard       │◄───────────────► │  ├── nvidia-smi      │
│  ├── Deploy Wizard   │                  │  ├── Docker engine   │
│  ├── Device Manager  │                  │  └── System metrics  │
│  └── Log Viewer      │                  │                      │
│                      │                  │  Docker containers   │
│  yokai daemon        │   SSH tunnel     │  ├── vLLM :8000      │
│  ├── SSH Tunnels     │◄═══════════════► │  ├── llama.cpp :8080 │
│  ├── Metrics Agg.    │                  │  └── ComfyUI :8188   │
│  └── Command Proxy   │                  │                      │
│                      │                  │  Monitoring stack    │
│  ~/.config/yokai/    │                  │  ├── Prometheus      │
│  └── config.json     │                  │  ├── Grafana         │
└──────────────────────┘                  │  └── Exporters       │
                                          └──────────────────────┘
```

**Data flow:**
1. The **daemon** opens SSH tunnels to each device and creates local port forwards to each agent
2. Every 2 seconds, the daemon polls each agent's `/metrics` endpoint and caches the results
3. The **TUI** reads from the daemon's REST API (`localhost:7473`) and renders the dashboard
4. Deploy and lifecycle commands (stop/restart/remove) flow from TUI -> daemon -> agent -> Docker

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
      "gpu_type": "nvidia"
    }
  ],
  "services": [
    {
      "id": "yokai-vllm-llama3",
      "device_id": "gaming-rig",
      "type": "vllm",
      "image": "vllm/vllm-openai:latest",
      "model": "meta-llama/Llama-3.1-8B-Instruct",
      "port": 8000
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
| macOS | arm64 (Apple Silicon) | Supported |

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

---

## Project Structure

```
yokai/
├── cmd/yokai/          # Binary entry point and subcommand routing
├── internal/
│   ├── agent/             # Remote agent: REST API, Docker ops, system metrics
│   ├── config/            # Config load/save/migrate (~/.config/yokai/)
│   ├── daemon/            # Local daemon: SSH tunnels, metrics aggregation
│   ├── docker/            # Docker Hub/GHCR tag catalog, Compose generation
│   ├── hf/                # HuggingFace API: model search, GGUF listing
│   ├── ssh/               # SSH client, SCP upload, bootstrap/deploy
│   ├── tailscale/         # Tailscale CLI wrapper for peer discovery
│   ├── tui/               # Bubbletea app shell and view router
│   │   ├── components/    # Reusable widgets (metrics bar, sparkline, GPU panel)
│   │   ├── theme/         # Tokyo Night color palette and styles
│   │   └── views/         # All TUI screens (dashboard, deploy, devices, etc.)
│   ├── upgrade/           # Self-update from GitHub Releases
│   └── vscode/            # VS Code settings.json manipulation
├── assets/
│   ├── grafana/           # Pre-built dashboard JSON and provisioning
│   ├── prometheus/        # Prometheus scrape configuration
│   └── systemd/           # Agent systemd service template
├── architecture/          # Multi-level architecture documentation
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
