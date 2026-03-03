# gpufleet

A single-binary TUI + distributed agent system for managing **vLLM**, **llama.cpp**, and **ComfyUI** across your GPU fleet via Docker.

```
╭─ gpufleet ──────────────────────────────────────────────────── 2 devices ─╮
│ ╭─ gaming-rig · 100.64.0.2 ● ──────────────────────────────────────────╮ │
│ │ GPU 0: RTX 4090  Util 87% [█████████████████░░░]  VRAM 20.1/24.0 GB │ │
│ ╰──────────────────────────────────────────────────────────────────────╯ │
│ ╭─ Services ───────────────────────────────────────────────────────────╮ │
│ │▸ vLLM  Llama-3.1-8B-Instruct  gaming-rig  ● live  :8000  142 t/s  │ │
│ │  llama.cpp  Mistral-7B-Q4_K_M  gaming-rig  ● live  :8080   38 t/s  │ │
│ ╰──────────────────────────────────────────────────────────────────────╯ │
│ ╭─ Keys ──────────────────────────────────────────────────────────────╮  │
│ │ n new  s stop  l logs  d devices  g grafana  c copilot  ? help  q quit│ │
│ ╰──────────────────────────────────────────────────────────────────────╯ │
╰──────────────────────────────────────────────────────────────────────────╯
```

## Features

- **Onboarding wizard** — connect via LAN, Tailscale, or manual IP
- **Device fleet management** — SSH bootstrap, pre-flight checks, agent deployment
- **Workload deployment** — deploy vLLM, llama.cpp, or ComfyUI with a guided wizard
- **btop-style dashboard** — live GPU/CPU/RAM metrics with sparklines and gradient bars
- **HuggingFace integration** — search models, pick GGUF quantizations, auto-download
- **Docker image catalog** — browse official tags including nightlies, or use custom images
- **Prometheus + Grafana** — auto-deployed monitoring stack per device
- **VS Code Copilot integration** — auto-configure local models as OpenAI-compatible endpoints
- **Portable config** — `~/.config/gpufleet/config.json` — copy to a new machine and reconnect

## Quick Start

```bash
# Install
curl -fsSL https://raw.githubusercontent.com/spencerbull/gpufleet/main/install.sh | sh

# Or build from source
git clone https://github.com/spencerbull/gpufleet.git
cd gpufleet
make build
./bin/gpufleet
```

## How It Works

```
User Machine                          Target Devices
┌──────────────────┐                  ┌──────────────────┐
│ gpufleet (TUI)   │                  │ gpufleet agent   │
│ gpufleet daemon  │◄── SSH tunnel ──►│ Docker SDK       │
│ config.json      │                  │ GPU metrics      │
└──────────────────┘                  │ vLLM / llama.cpp │
                                      └──────────────────┘
```

1. Run `gpufleet` — onboarding wizard helps you connect devices
2. Agent is deployed automatically via SSH
3. Deploy models through the workload wizard
4. Monitor everything on the btop-style dashboard
5. Press `c` to auto-configure VS Code Copilot endpoints

## Architecture

See [architecture/README.md](architecture/README.md) for detailed multi-level architecture documentation:

- [L1: System Overview](architecture/01-system-overview.md)
- [L2: Component Architecture](architecture/02-component-architecture.md)
- [L3: Data Flow](architecture/03-data-flow.md)
- [L4: Network Topology](architecture/04-network-topology.md)
- [L5: TUI Screen Map](architecture/05-tui-screen-map.md)
- [L6: Agent API](architecture/06-agent-api.md)

## Requirements

- **User machine**: Go 1.22+ (build only), any OS
- **Target devices**: Linux, Docker, NVIDIA GPU + nvidia-container-toolkit (for GPU workloads)
- **Optional**: Tailscale for cross-network connectivity

## Config

All state is stored in `~/.config/gpufleet/config.json`:

```json
{
  "version": 1,
  "hf_token": "hf_...",
  "devices": [
    {
      "id": "gaming-rig",
      "host": "100.64.0.2",
      "connection_type": "tailscale",
      "gpu_type": "nvidia"
    }
  ],
  "services": [
    {
      "type": "vllm",
      "model": "meta-llama/Llama-3.1-8B-Instruct",
      "device_id": "gaming-rig",
      "port": 8000
    }
  ]
}
```

## License

MIT
