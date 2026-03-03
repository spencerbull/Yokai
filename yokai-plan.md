# LLM Fleet Manager — Refined Architecture & Implementation Plan

A single-binary Go TUI + distributed agent system for managing vLLM, llama.cpp, and ComfyUI across local and Tailscale-connected GPU devices, with btop-style metrics, HuggingFace model provisioning, and VS Code Copilot endpoint auto-configuration.

---

## Pre-Implementation Step: Architecture Documentation

Before writing any code, create markdown architecture diagrams at multiple levels of detail in `architecture/` and a `README.md` that links to them. This serves as the living design reference throughout implementation.

### Files to Create

```
architecture/
├── README.md                    # Index linking all diagrams, rendered in GitHub
├── 01-system-overview.md        # L1: Bird's-eye — user machine ↔ devices
├── 02-component-architecture.md # L2: Internal components (daemon, agent, TUI)
├── 03-data-flow.md              # L3: Metrics, commands, config data paths
├── 04-network-topology.md       # L4: SSH tunnels, Tailscale, ports, auth
├── 05-tui-screen-map.md         # L5: View hierarchy, navigation state machine
└── 06-agent-api.md              # L6: REST API contract, request/response schemas
```

### Diagram Levels

- **L1 System Overview** — boxes for User Machine and N Target Devices, connection lines (SSH/Tailscale/LAN), what lives where.
- **L2 Component Architecture** — zoomed into each box: TUI (Bubbletea model tree), Daemon (tunnels, aggregator, REST), Agent (HTTP server, Docker SDK, metrics collectors). Shows Go package boundaries.
- **L3 Data Flow** — arrows showing: metrics polling path, deploy command path, config read/write, log streaming, HF API calls, Docker Hub tag queries.
- **L4 Network Topology** — ports, protocols, auth tokens, SSH tunnel setup, Tailscale mesh, firewall considerations. Shows exactly what connects to what.
- **L5 TUI Screen Map** — state machine showing every view, transitions between them, keybinds that trigger transitions. References the mockups below.
- **L6 Agent API** — full REST contract with request/response JSON schemas, auth, SSE streaming endpoints.

The project root `README.md` should include a quick-start, feature list, and an Architecture section linking to `architecture/README.md`.

---

## Decisions to Resolve

Recommendations marked with ✅.

| # | Decision | Recommendation |
|---|----------|----------------|
| 1 | **Project name** | `yokai` ✅ — clear, implies multi-device GPU management |
| 2 | **Binary model** | Same binary, subcommand (`yokai`, `yokai agent`, `yokai daemon`) ✅ |
| 3 | **Orchestration** | Hybrid ✅ — manual targeting with VRAM visibility; auto-placement later |
| 4 | **Agent auth** | Shared bearer token ✅ — generated at bootstrap, LAN/Tailscale-only |
| 5 | **Apple Silicon** | Defer — Linux + NVIDIA/AMD Docker first; macOS native mode later |
| 6 | **ComfyUI image** | `yanwk/comfyui-boot:latest` ✅ — most maintained, allow override |

---

## TUI Screen Flows — Complete Mockups

Every screen the user will encounter, in order. These are the implementation reference.

### Screen 1: Welcome (First Run)

The entry point. Full-screen centered card. Appears only when `config.json` has no devices.

```
╭─────────────────────────────────────────────────────────────────────────╮
│                                                                         │
│                                                                         │
│                         ╭─ yokai ─────────────────╮                  │
│                         │                             │                  │
│                         │  GPU Fleet Manager          │                  │
│                         │  v0.1.0                     │                  │
│                         │                             │                  │
│                         │  Manage LLM services        │                  │
│                         │  across your GPU devices.   │                  │
│                         │                             │                  │
│                         │  How will you connect?      │                  │
│                         │                             │                  │
│                         │  > Local network (LAN)      │                  │
│                         │    Tailscale VPN             │                  │
│                         │    Manual (enter IP/host)   │                  │
│                         │                             │                  │
│                         ╰─────────────────────────────╯                  │
│                                                                         │
│  ↑/↓ navigate   Enter select   q quit                                   │
╰─────────────────────────────────────────────────────────────────────────╯
```

**Behavior**: Arrow keys to highlight, Enter to select. No letter shortcuts on this screen — keep it clean. After selection, transition to the appropriate sub-flow.

---

### Screen 1a: Local Network — IP Selection

```
╭─ Local Network Setup ──────────────────────────────────────────────────╮
│                                                                         │
│  Detected network interfaces:                                          │
│                                                                         │
│  > eth0       192.168.1.42    ● up                                     │
│    wlan0      192.168.1.105   ● up                                     │
│    docker0    172.17.0.1      ● up                                     │
│    lo         127.0.0.1       ● up                                     │
│                                                                         │
│  Select the interface other devices can reach this machine on.         │
│                                                                         │
│  ↑/↓ navigate   Enter confirm   Esc back                               │
╰─────────────────────────────────────────────────────────────────────────╯
```

After confirm → adds device as local, jumps to **Screen 2: SSH Credentials**.

---

### Screen 1b: Tailscale — Detection + Peer List

Three sub-states rendered in the same view:

**State: Not Installed**
```
╭─ Tailscale ────────────────────────────────────────────────────────────╮
│                                                                         │
│  ✗ Tailscale is not installed                                          │
│                                                                         │
│  ╭─ Install Instructions ───────────────────────────────────────────╮  │
│  │                                                                   │  │
│  │  Linux:                                                           │  │
│  │    curl -fsSL https://tailscale.com/install.sh | sh               │  │
│  │                                                                   │  │
│  │  macOS:                                                           │  │
│  │    brew install tailscale                                         │  │
│  │                                                                   │  │
│  │  Then authenticate:                                               │  │
│  │    sudo tailscale up                                              │  │
│  │                                                                   │  │
│  ╰───────────────────────────────────────────────────────────────────╯  │
│                                                                         │
│  r retry   s skip to manual IP   Esc back                              │
╰─────────────────────────────────────────────────────────────────────────╯
```

**State: Installed, Not Connected**
```
╭─ Tailscale ────────────────────────────────────────────────────────────╮
│                                                                         │
│  ⚠ Tailscale is installed but not connected                            │
│                                                                         │
│  Run this command to connect:                                          │
│                                                                         │
│    sudo tailscale up                                                    │
│                                                                         │
│  You'll be given a URL to authenticate in your browser.               │
│  Return here when connected.                                           │
│                                                                         │
│  r retry   s skip to manual IP   Esc back                              │
╰─────────────────────────────────────────────────────────────────────────╯
```

**State: Connected — Peer Selection**
```
╭─ Tailscale Peers ──────────────────────────────────────── 3 peers, 2 online ─╮
│                                                                               │
│  This machine: sbull-desktop (100.64.0.1)                                    │
│                                                                               │
│  ┌──────────────────────────────────────────────────────────────────────────┐ │
│  │    HOST            IP             OS       STATUS       LAST SEEN       │ │
│  │                                                                          │ │
│  │ [x] gaming-rig     100.64.0.2     Linux    ● online     now             │ │
│  │ [x] workstation    100.64.0.3     Linux    ● online     now             │ │
│  │ [ ] macstudio      100.64.0.4     macOS    ○ offline    2h ago          │ │
│  │                                                                          │ │
│  └──────────────────────────────────────────────────────────────────────────┘ │
│                                                                               │
│  Space toggle   a select all   Enter confirm (2 selected)   Esc back         │
╰───────────────────────────────────────────────────────────────────────────────╯
```

**Behavior**: Multi-select with spacebar. `a` toggles all. Offline devices are dimmed but selectable (bootstrap will fail gracefully). Enter proceeds to **Screen 2** for each selected device sequentially.

---

### Screen 1c: Manual Entry

```
╭─ Manual Device ────────────────────────────────────────────────────────╮
│                                                                         │
│  Enter hostname or IP address of the target device.                    │
│  You can add multiple devices separated by commas.                     │
│                                                                         │
│  Host(s): 192.168.1.50, 10.0.0.20█                                    │
│                                                                         │
│  Enter confirm   Esc back                                               │
╰─────────────────────────────────────────────────────────────────────────╯
```

---

### Screen 2: SSH Credentials

Shown per-device. If multiple devices selected, this loops.

```
╭─ SSH Connection — gaming-rig (100.64.0.2) ─────────────────────────────╮
│                                                                         │
│  Username:   sbull█                    (default: current user)          │
│                                                                         │
│  Auth method:                                                           │
│    > SSH key   ~/.ssh/id_ed25519                                       │
│      SSH key   ~/.ssh/id_rsa                                           │
│      SSH agent (3 keys loaded)                                         │
│      Password                                                           │
│                                                                         │
│  ╭─ Connection Test ────────────────────────────────────────────────╮  │
│  │  [⟳] Testing SSH connection...                                    │  │
│  ╰───────────────────────────────────────────────────────────────────╯  │
│                                                                         │
│  Enter connect   Tab next field   Esc back                              │
╰─────────────────────────────────────────────────────────────────────────╯
```

**After successful connection** → transition to **Screen 3: Bootstrap**.

---

### Screen 3: Device Bootstrap

Full-screen progress view. Steps execute sequentially with live status updates.

```
╭─ Bootstrapping: gaming-rig (100.64.0.2) ──────────────────────────────╮
│                                                                         │
│  SSH                                                                    │
│    [✓] Connected as sbull                                              │
│                                                                         │
│  Pre-flight Checks                                                      │
│    [✓] OS: Ubuntu 24.04 LTS (x86_64)                                  │
│    [✓] Docker v27.1.2                                                   │
│    [✓] NVIDIA GPU: RTX 4090 (24576 MB VRAM)                           │
│    [✓] nvidia-container-toolkit v1.16.1                                │
│    [✓] Docker nvidia runtime available                                  │
│    [✓] Disk: 142 GB free                                               │
│                                                                         │
│  Agent Deployment                                                       │
│    [✓] Uploaded yokai binary (48.2 MB)                              │
│    [✓] Installed systemd service                                        │
│    [⟳] Starting agent...                                               │
│    [ ] Health check on :7474                                            │
│                                                                         │
│  Monitoring Stack                                                       │
│    [ ] Deploy Prometheus + Grafana + node-exporter + dcgm-exporter     │
│    [ ] Seed Grafana dashboards                                          │
│                                                                         │
│  ─────────────────────────────────────────────────────────────          │
│  Overall: ████████████████░░░░░░░░░░░░  62%                            │
│                                                                         │
╰─────────────────────────────────────────────────────────────────────────╯
```

**Pre-flight failure — Docker missing:**
```
│  Pre-flight Checks                                                      │
│    [✓] OS: Ubuntu 24.04 LTS (x86_64)                                  │
│    [✗] Docker not found                                                 │
│                                                                         │
│    ╭─ Docker Required ──────────────────────────────────────────────╮  │
│    │                                                                 │  │
│    │  Docker is required to run GPU workloads.                       │  │
│    │                                                                 │  │
│    │  Install on Ubuntu/Debian:                                      │  │
│    │    curl -fsSL https://get.docker.com | sh                       │  │
│    │    sudo usermod -aG docker $USER                                │  │
│    │                                                                 │  │
│    │  [I] Run install command via SSH                                │  │
│    │  [R] Retry detection                                            │  │
│    │  [S] Skip this device                                           │  │
│    │                                                                 │  │
│    ╰─────────────────────────────────────────────────────────────────╯  │
```

**After all devices bootstrapped** → **Screen 4: HF Token**.

---

### Screen 4: HuggingFace Token

```
╭─ HuggingFace Token ───────────────────────────────────────────────────╮
│                                                                         │
│  A HuggingFace token is needed to download gated models               │
│  (Llama, Mistral, Gemma, etc).                                        │
│                                                                         │
│  ╭─ Token Source ───────────────────────────────────────────────────╮  │
│  │                                                                   │  │
│  │  [✓] Found $HF_TOKEN in environment                              │  │
│  │      hf_abcd••••••••wxyz                                         │  │
│  │                                                                   │  │
│  ╰───────────────────────────────────────────────────────────────────╯  │
│                                                                         │
│  > Use this token                                                       │
│    Enter a different token                                              │
│    Skip (public models only)                                            │
│                                                                         │
│  Get a token: https://huggingface.co/settings/tokens                   │
│                                                                         │
│  ↑/↓ navigate   Enter select   Esc back                                │
╰─────────────────────────────────────────────────────────────────────────╯
```

**If entering manually:**
```
│  Token: hf_████████████████████████████████████████                     │
│         (paste token — it will be masked)                               │
│                                                                         │
│  [✓] Token valid — access to 847 gated repos                          │
```

After token → **Screen 5: Dashboard** (onboarding complete).

---

### Screen 5: Dashboard (Main View) — btop-Inspired

The primary screen. This is where the user spends most of their time. Layout adapts to terminal width.

**Full layout (≥120 cols):**
```
╭─ yokai ──────────────────────────────────────────────────── 2 devices ─╮
│                                                                            │
│ ╭─ gaming-rig · 100.64.0.2 ● ──────────────────────────────────────────╮ │
│ │ ╭─ CPU ──────────────────────────╮ ╭─ Memory ───────────────────────╮ │ │
│ │ │ Total  47% [████████░░░░░░░░░] │ │ RAM  18.2/32.0 GB [██████░░░░] │ │ │
│ │ │ Core0  62% [███████████░░░░░░] │ │ Swap  0.1/ 8.0 GB [░░░░░░░░░░] │ │ │
│ │ │ Core1  33% [██████░░░░░░░░░░░] │ │ Disk 142G free /500G           │ │ │
│ │ │ Core2  51% [█████████░░░░░░░░] │ ╰────────────────────────────────╯ │ │
│ │ │ Core3  42% [████████░░░░░░░░░] │                                     │ │
│ │ │ ▁▂▃▅▆▅▃▂▁▂▃▄▅▆▅▄▃▂▃▄▅▆▇▆▅▄▃  │                                     │ │
│ │ ╰────────────────────────────────╯                                     │ │
│ │ ╭─ GPU 0: RTX 4090 ─────────────────────────────────────────────────╮ │ │
│ │ │ Util  87% [██████████████████████████░░░░] ▆▇▇█▇▇▆▇█▇▇█▇▆▇█▇▇█  │ │ │
│ │ │ VRAM  20.1/24.0 GB [████████████████████░░░░]  84%                │ │ │
│ │ │ Temp  72°C [█████████████░░░░░░░]  Fan 65%   Power 312/450W      │ │ │
│ │ ╰───────────────────────────────────────────────────────────────────╯ │ │
│ ╰──────────────────────────────────────────────────────────────────────╯ │
│                                                                            │
│ ╭─ Services ───────────────────────────────────────────────────────────╮  │
│ │                                                                       │  │
│ │  TYPE       MODEL                       DEVICE       STATUS    PORT  │  │
│ │  ──────────────────────────────────────────────────────────────────── │  │
│ │▸ vLLM       Llama-3.1-8B-Instruct       gaming-rig   ● live   8000  │  │
│ │  ├ Throughput  142 t/s [████████████████░░░░░] ▁▂▄▆▇█▇▆▇█▇▅▃▄▆▇    │  │
│ │  ├ VRAM        9.1/24.0 GB               Queue 2     TTFT 183ms     │  │
│ │  └ Uptime      3h 24m                    Requests 1,847 total       │  │
│ │                                                                       │  │
│ │  llama.cpp  Mistral-7B-Q4_K_M           gaming-rig   ● live   8080  │  │
│ │  ├ Throughput   38 t/s [████░░░░░░░░░░░░░░░░░] ▁▁▂▂▁▁▂▃▂▁▁▁▂▃▂▁    │  │
│ │  ├ VRAM        4.1/24.0 GB               Queue 0     TTFT  42ms     │  │
│ │  └ Uptime      1h 12m                    Requests    312 total      │  │
│ │                                                                       │  │
│ │  ComfyUI    —                            gaming-rig   ● live   8188  │  │
│ │  └ VRAM        3.2/24.0 GB               Jobs 0 queued              │  │
│ │                                                                       │  │
│ ╰───────────────────────────────────────────────────────────────────────╯  │
│                                                                            │
│ ╭─ Keys ───────────────────────────────────────────────────────────────╮  │
│ │ n new  s stop  l logs  d devices  g grafana  c copilot  ? help  q quit│  │
│ ╰───────────────────────────────────────────────────────────────────────╯  │
╰────────────────────────────────────────────────────────────────────────────╯
```

**Multi-device view** — when multiple devices are registered, device panels are stacked or side-by-side depending on terminal width:

```
╭─ yokai ──────────────────────────────────────────────────── 2 devices ─╮
│                                                                            │
│ ╭─ [1] gaming-rig ● ──────────────╮ ╭─ [2] workstation ● ──────────────╮ │
│ │ CPU  47% [████████░░░░░░░░░░░░] │ │ CPU  12% [██░░░░░░░░░░░░░░░░░░] │ │
│ │ RAM  18.2/32.0 GB               │ │ RAM   8.4/64.0 GB               │ │
│ │ GPU0 87% [█████████████████░░░] │ │ GPU0  5% [█░░░░░░░░░░░░░░░░░░] │ │
│ │ VRAM 20.1/24.0 GB  72°C        │ │ VRAM  1.2/24.0 GB  38°C        │ │
│ │ ▆▇▇█▇▇▆▇█▇▇█▇▆▇█▇▇█▇▇█▇▆▇█▇▇  │ │ ▁▁▁▁▁▁▁▁▁▁▁▁▁▁▁▁▁▁▁▁▁▁▁▁▁▁▁▁  │ │
│ ╰──────────────────────────────────╯ ╰──────────────────────────────────╯ │
│                                                                            │
│ ╭─ Services ───────────────────────────────────────────────────────────╮  │
│ │  ... (same as above, showing services across all devices)            │  │
│ ╰───────────────────────────────────────────────────────────────────────╯  │
```

**Narrow layout (<100 cols)** — panels stack vertically, sparklines omitted, bars shortened.

**Navigation**:
- `Tab` / `Shift+Tab`: cycle focus between device panels → service list → keybind bar
- `↑↓`: move selection within focused panel
- `1-9`: quick-switch device focus
- `Enter` on a service: expand detail view (inline)
- All keybinds shown in bottom bar are active globally

---

### Screen 5a: Service Detail (Expanded Row)

Pressing Enter on a service in the dashboard expands it inline:

```
│ │▸ vLLM · Llama-3.1-8B-Instruct                     gaming-rig ● live │  │
│ │  ╭─ Details ─────────────────────────────────────────────────────╮   │  │
│ │  │ Image:     vllm/vllm-openai:v0.6.4                           │   │  │
│ │  │ Model:     meta-llama/Llama-3.1-8B-Instruct                  │   │  │
│ │  │ Port:      8000                                                │   │  │
│ │  │ Endpoint:  http://100.64.0.2:8000/v1                          │   │  │
│ │  │ Container: yokai-vllm-llama31-8b (a3f7c2d)                 │   │  │
│ │  │ Started:   2025-03-02 18:34:12 (3h 24m ago)                   │   │  │
│ │  │ Args:      --gpu-memory-utilization 0.9 --max-model-len 4096  │   │  │
│ │  │                                                                │   │  │
│ │  │ Throughput ────────────────────────────────────────────────── │   │  │
│ │  │  142 t/s  peak: 218 t/s  avg: 127 t/s                        │   │  │
│ │  │  ▁▂▃▅▆▅▃▂▁▂▃▄▅▆▅▄▃▂▃▄▅▆▇▆▅▄▃▂▃▅▆▇▇▆▅▄▃▂▁▂▃▄▅▆▇█▇▆▅▄▃▄▅▆▇  │   │  │
│ │  │                                                                │   │  │
│ │  │  l logs  s stop  r restart  c copy endpoint  Esc collapse     │   │  │
│ │  ╰────────────────────────────────────────────────────────────────╯   │  │
```

---

### Screen 6: Deploy Wizard (New Service)

Press `n` from dashboard. Multi-step wizard with a step indicator.

**Step 1/5 — Workload Type**
```
╭─ New Service ─────────────────────────────── Step 1 of 5: Workload Type ─╮
│                                                                            │
│  What would you like to deploy?                                           │
│                                                                            │
│  > vLLM                                                                    │
│    OpenAI-compatible inference server. Best throughput on NVIDIA GPUs.    │
│    Supports continuous batching, PagedAttention, tensor parallelism.      │
│    Default port: 8000                                                      │
│                                                                            │
│    llama.cpp                                                               │
│    Lightweight inference. CPU + GPU. Uses GGUF quantized models.         │
│    Default port: 8080                                                      │
│                                                                            │
│    ComfyUI                                                                 │
│    Node-based image generation UI. Stable Diffusion, Flux, etc.          │
│    Default port: 8188                                                      │
│                                                                            │
│  ↑/↓ navigate   Enter next   Esc cancel                                   │
╰────────────────────────────────────────────────────────────────────────────╯
```

**Step 2/5 — Target Device**
```
╭─ New Service ─────────────────────────────── Step 2 of 5: Target Device ─╮
│                                                                            │
│  Select target device(s). VRAM availability shown.                        │
│                                                                            │
│  ┌──────────────────────────────────────────────────────────────────────┐ │
│  │    DEVICE          GPU              VRAM FREE     VRAM TOTAL   SRVCS│ │
│  │                                                                      │ │
│  │ [x] gaming-rig     RTX 4090        14.8 GB       24.0 GB       2   │ │
│  │ [ ] workstation    RTX 3090        22.8 GB       24.0 GB       0   │ │
│  └──────────────────────────────────────────────────────────────────────┘ │
│                                                                            │
│  Estimated VRAM for Llama-3.1-8B-Instruct (FP16): ~16 GB                │
│  ⚠ gaming-rig may not have enough free VRAM                              │
│  ✓ workstation has sufficient VRAM                                        │
│                                                                            │
│  Space toggle   Enter next   Backspace prev   Esc cancel                  │
╰────────────────────────────────────────────────────────────────────────────╯
```

**Step 3/5 — Docker Image**
```
╭─ New Service ─────────────────────────────── Step 3 of 5: Docker Image ──╮
│                                                                            │
│  Select image tag for vLLM:                                               │
│                                                                            │
│  ╭─ Official Tags (vllm/vllm-openai) ──────────────────────────────────╮ │
│  │                                                                       │ │
│  │ > latest                        (v0.6.4, 2025-02-28)               │ │
│  │   v0.6.4                        (stable release)                    │ │
│  │   v0.6.3.post1                  (stable release)                    │ │
│  │   v0.6.2                        (stable release)                    │ │
│  │   nightly                       (2025-03-02)        ⚠ NIGHTLY      │ │
│  │   ───────────────────────────────────────────────────────────────── │ │
│  │   Custom: type image:tag...                                         │ │
│  │                                                                       │ │
│  ╰───────────────────────────────────────────────────────────────────────╯ │
│                                                                            │
│  Tags fetched from Docker Hub · cached for 1h · last refresh: 2m ago     │
│                                                                            │
│  ↑/↓ navigate   Enter next   Backspace prev   Esc cancel                  │
╰────────────────────────────────────────────────────────────────────────────╯
```

**llama.cpp variant:**
```
│  ╭─ Official Tags (ghcr.io/ggml-org/llama.cpp) ────────────────────────╮ │
│  │                                                                       │ │
│  │ > server-cuda                   (NVIDIA GPU)                        │ │
│  │   server                        (CPU only)                          │ │
│  │   server-rocm                   (AMD GPU)                           │ │
│  │   full-cuda                     (+ tools, NVIDIA)                   │ │
│  │   full                          (+ tools, CPU)                      │ │
│  │   ───────────────────────────────────────────────────────────────── │ │
│  │   Custom: type image:tag...                                         │ │
│  │                                                                       │ │
│  ╰───────────────────────────────────────────────────────────────────────╯ │
```

**Step 4/5 — Model Selection**
```
╭─ New Service ──────────────────────────── Step 4 of 5: Model Selection ──╮
│                                                                            │
│  Search HuggingFace models:                                               │
│                                                                            │
│  🔍 meta-llama█                                                           │
│                                                                            │
│  ┌─────────────────────────────────────────────────────────────────────┐  │
│  │  MODEL                                SIZE     LIKES   DOWNLOADS  │  │
│  │                                                                     │  │
│  │> meta-llama/Llama-3.1-8B-Instruct     4.9 GB   12.4k   2.1M     │  │
│  │  meta-llama/Llama-3.1-70B-Instruct   37.2 GB    8.1k   890k     │  │
│  │  meta-llama/Llama-3.2-3B-Instruct     2.1 GB    5.6k   1.4M     │  │
│  │  meta-llama/Llama-3.1-8B              4.9 GB    4.2k   780k     │  │
│  │  meta-llama/Llama-3.3-70B-Instruct   37.2 GB    3.8k   420k     │  │
│  └─────────────────────────────────────────────────────────────────────┘  │
│                                                                            │
│  ⓘ Gated model — your HF token will be used for access                   │
│                                                                            │
│  Type to search   ↑/↓ navigate   Enter next   Backspace prev              │
╰────────────────────────────────────────────────────────────────────────────╯
```

**llama.cpp GGUF sub-step** (appears after model selection for llama.cpp):
```
╭─ GGUF Quantization ────────────────────────────────────────────────────╮
│                                                                         │
│  Select quantization for meta-llama/Llama-3.1-8B-Instruct:            │
│                                                                         │
│  ┌──────────────────────────────────────────────────────────────────┐  │
│  │  QUANT         SIZE      QUALITY    SPEED     VRAM EST          │  │
│  │                                                                  │  │
│  │  Q8_0          8.5 GB    ★★★★★      ★★★       ~9.5 GB          │  │
│  │> Q5_K_M        5.7 GB    ★★★★☆      ★★★★      ~6.7 GB          │  │
│  │  Q4_K_M        4.9 GB    ★★★★☆      ★★★★★     ~5.9 GB          │  │
│  │  Q4_K_S        4.6 GB    ★★★☆☆      ★★★★★     ~5.6 GB          │  │
│  │  Q3_K_M        3.8 GB    ★★★☆☆      ★★★★★     ~4.8 GB          │  │
│  │  Q2_K          3.2 GB    ★★☆☆☆      ★★★★★     ~4.2 GB          │  │
│  └──────────────────────────────────────────────────────────────────┘  │
│                                                                         │
│  ↑/↓ navigate   Enter next   Backspace prev                            │
╰─────────────────────────────────────────────────────────────────────────╯
```

**Step 5/5 — Configuration + Confirm**
```
╭─ New Service ──────────────────────────── Step 5 of 5: Configuration ────╮
│                                                                            │
│  ╭─ Service Configuration ──────────────────────────────────────────────╮ │
│  │                                                                       │ │
│  │  Port:                 8000                                          │ │
│  │  GPU Mem Utilization:  0.90       (vLLM: --gpu-memory-utilization)  │ │
│  │  Max Model Length:     4096       (vLLM: --max-model-len)           │ │
│  │  Tensor Parallel:      1          (vLLM: --tensor-parallel-size)    │ │
│  │  Extra Args:           __________                                    │ │
│  │                                                                       │ │
│  ╰───────────────────────────────────────────────────────────────────────╯ │
│                                                                            │
│  ╭─ Preview Command ────────────────────────────────────────────────────╮ │
│  │                                                                       │ │
│  │  docker run -d --gpus all --name yokai-vllm-llama31-8b \          │ │
│  │    -p 8000:8000 \                                                     │ │
│  │    -e HF_TOKEN=hf_**** \                                             │ │
│  │    --ipc=host \                                                       │ │
│  │    vllm/vllm-openai:latest \                                          │ │
│  │    --model meta-llama/Llama-3.1-8B-Instruct \                        │ │
│  │    --gpu-memory-utilization 0.90 \                                    │ │
│  │    --max-model-len 4096                                               │ │
│  │                                                                       │ │
│  ╰───────────────────────────────────────────────────────────────────────╯ │
│                                                                            │
│  Target: gaming-rig (100.64.0.2)                                          │
│                                                                            │
│  Enter deploy   e edit command   Tab next field   Backspace prev   Esc cancel│
╰────────────────────────────────────────────────────────────────────────────╯
```

**Deploying state** (after pressing Enter):
```
╭─ Deploying ────────────────────────────────────────────────────────────╮
│                                                                         │
│  vLLM · Llama-3.1-8B-Instruct → gaming-rig                            │
│                                                                         │
│  [✓] Pulling vllm/vllm-openai:latest                                   │
│      ███████████████████████████████████████████ 100% (14.2 GB)         │
│                                                                         │
│  [⟳] Starting container...                                              │
│                                                                         │
│  [ ] Downloading model weights from HuggingFace                        │
│  [ ] Loading model into GPU memory                                      │
│  [ ] Health check: http://100.64.0.2:8000/v1/models                    │
│                                                                         │
│  ╭─ Live Logs ──────────────────────────────────────────────────────╮  │
│  │ INFO:     Loading model meta-llama/Llama-3.1-8B-Instruct...      │  │
│  │ INFO:     Downloading shards: 100% 2/2 [00:42<00:00]             │  │
│  │ INFO:     Loading model weights: [████████████░░░] 78%            │  │
│  ╰───────────────────────────────────────────────────────────────────╯  │
│                                                                         │
│  Esc background (continue on dashboard)                                 │
╰─────────────────────────────────────────────────────────────────────────╯
```

---

### Screen 7: Device Manager

Press `d` from dashboard.

```
╭─ Devices ──────────────────────────────────────────────── 2 registered ──╮
│                                                                            │
│  ┌──────────────────────────────────────────────────────────────────────┐ │
│  │    DEVICE          HOST           CONN     GPU           STATUS     │ │
│  │                                                                      │ │
│  │ ▸  gaming-rig      100.64.0.2     TS       RTX 4090     ● online   │ │
│  │    workstation     100.64.0.3     TS       RTX 3090     ● online   │ │
│  │    macstudio       192.168.1.50   LAN      M2 Ultra     ○ offline  │ │
│  └──────────────────────────────────────────────────────────────────────┘ │
│                                                                            │
│  ╭─ gaming-rig ────────────────────────────────────────────────────────╮ │
│  │ Host:       100.64.0.2                  Connection: Tailscale       │ │
│  │ SSH User:   sbull                       SSH Key:  ~/.ssh/id_ed25519 │ │
│  │ Agent:      v0.1.0 on :7474 ● healthy  Uptime:   14d 3h           │ │
│  │ GPU:        NVIDIA RTX 4090 (24 GB)     Driver:   550.54.15        │ │
│  │ OS:         Ubuntu 24.04 LTS            Docker:   v27.1.2          │ │
│  │ Monitoring: Prometheus :9090  Grafana :3000                         │ │
│  │ Services:   3 running                                               │ │
│  ╰─────────────────────────────────────────────────────────────────────╯ │
│                                                                            │
│  a add device   e edit   x remove   t test connection   Esc back          │
╰────────────────────────────────────────────────────────────────────────────╯
```

---

### Screen 8: Live Log Viewer

Press `l` with a service selected, or `l` from the expanded service detail.

```
╭─ Logs · vLLM · Llama-3.1-8B-Instruct · gaming-rig ────────────────────╮
│                                                                         │
│ 2025-03-02 21:42:15 INFO:     Received request 0ac3f2: prompt=142 tok │
│ 2025-03-02 21:42:15 INFO:     Running 3 requests. Pending: 0          │
│ 2025-03-02 21:42:16 INFO:     Request 0ac3f2 completed: 89 tokens,    │
│                               628 ms, TTFT=42ms                        │
│ 2025-03-02 21:42:18 INFO:     Received request 7b1e4a: prompt=56 tok  │
│ 2025-03-02 21:42:18 INFO:     Running 3 requests. Pending: 0          │
│ 2025-03-02 21:42:19 INFO:     Request 7b1e4a completed: 215 tokens,   │
│                               1.2s, TTFT=38ms                          │
│ 2025-03-02 21:42:22 INFO:     Received request c9d821: prompt=1024 tok│
│ 2025-03-02 21:42:22 WARNING:  Request c9d821 queued (3 active)        │
│ 2025-03-02 21:42:23 INFO:     Request c9d821 started processing      │
│ █                                                                       │
│                                                                         │
│  ╭─ Status ─────────────────────────────────────────────────────────╮  │
│  │ Streaming · 847 lines · auto-scroll ●                            │  │
│  ╰───────────────────────────────────────────────────────────────────╯  │
│                                                                         │
│  ↑/↓/PgUp/PgDn scroll   f toggle follow   / search   Esc back          │
╰─────────────────────────────────────────────────────────────────────────╯
```

**Features**:
- SSE streaming from agent
- Auto-scroll (follow mode) toggled with `f`
- `/` opens search within logs
- Color-coded log levels (INFO=default, WARNING=yellow, ERROR=red)

---

### Screen 9: VS Code Copilot Integration

Press `c` from dashboard.

```
╭─ VS Code Copilot Endpoints ───────────────────────────────────────────╮
│                                                                         │
│  Running OpenAI-compatible services:                                    │
│                                                                         │
│  ┌──────────────────────────────────────────────────────────────────┐  │
│  │  SERVICE                        ENDPOINT                 STATUS │  │
│  │                                                                  │  │
│  │▸ vLLM · Llama-3.1-8B           http://100.64.0.2:8000/v1  ● ✓ │  │
│  │  llama.cpp · Mistral-7B        http://100.64.0.2:8080/v1  ● ✓ │  │
│  └──────────────────────────────────────────────────────────────────┘  │
│                                                                         │
│  ╭─ VS Code Configuration ─────────────────────────────────────────╮  │
│  │                                                                   │  │
│  │  Settings file: ~/.config/Code/User/settings.json                │  │
│  │  Status: 1 endpoint configured, 1 new                            │  │
│  │                                                                   │  │
│  │  Will add to "chat.models":                                      │  │
│  │  {                                                                │  │
│  │    "family": "openai",                                            │  │
│  │    "id": "Llama-3.1-8B-Instruct",                                │  │
│  │    "name": "Llama-3.1-8B (yokai)",                            │  │
│  │    "url": "http://100.64.0.2:8000/v1",                           │  │
│  │    "apiKey": "none"                                               │  │
│  │  }                                                                │  │
│  │                                                                   │  │
│  ╰───────────────────────────────────────────────────────────────────╯  │
│                                                                         │
│  a auto-configure VS Code   c copy endpoint   g open Grafana   Esc back │
╰─────────────────────────────────────────────────────────────────────────╯
```

**After auto-configure**:
```
│  ╭─ Result ─────────────────────────────────────────────────────────╮  │
│  │                                                                   │  │
│  │  [✓] Backed up settings.json → settings.json.yokai.bak       │  │
│  │  [✓] Added Llama-3.1-8B endpoint to chat.models                 │  │
│  │  [✓] Added Mistral-7B endpoint to chat.models                    │  │
│  │                                                                   │  │
│  │  Restart VS Code to pick up changes.                              │  │
│  │                                                                   │  │
│  ╰───────────────────────────────────────────────────────────────────╯  │
```

---

### Screen 10: Help Overlay

Press `?` from anywhere. Rendered as a centered overlay on top of current view.

```
                 ╭─ Keyboard Shortcuts ──────────────────╮
                 │                                        │
                 │  GLOBAL                                │
                 │  n       New service                   │
                 │  d       Device manager                │
                 │  c       Copilot endpoints             │
                 │  g       Open Grafana in browser       │
                 │  ?       This help screen              │
                 │  q       Quit                          │
                 │                                        │
                 │  DASHBOARD                             │
                 │  Tab     Cycle panel focus              │
                 │  1-9     Quick-switch device            │
                 │  Enter   Expand service detail          │
                 │  l       View logs (selected service)  │
                 │  s       Stop service                  │
                 │  r       Restart service                │
                 │                                        │
                 │  LOG VIEWER                            │
                 │  f       Toggle follow mode             │
                 │  /       Search logs                   │
                 │  PgUp/Dn Scroll                        │
                 │                                        │
                 │  WIZARDS                               │
                 │  Enter      Next step                  │
                 │  Backspace  Previous step               │
                 │  Esc        Cancel                     │
                 │                                        │
                 │  Press any key to close                │
                 ╰────────────────────────────────────────╯
```

---

## TUI State Machine (View Navigation)

```
                              ┌──────────┐
                              │ Welcome  │ (first run only)
                              └────┬─────┘
                       ┌───────────┼───────────┐
                       ▼           ▼           ▼
                  ┌─────────┐ ┌──────────┐ ┌────────┐
                  │Local IP │ │Tailscale │ │Manual  │
                  └────┬────┘ └────┬─────┘ └───┬────┘
                       └───────────┼───────────┘
                                   ▼
                            ┌──────────────┐
                            │SSH Credentials│ (per device)
                            └──────┬───────┘
                                   ▼
                            ┌──────────────┐
                            │  Bootstrap   │ (per device)
                            └──────┬───────┘
                                   ▼
                            ┌──────────────┐
                            │  HF Token    │
                            └──────┬───────┘
                                   ▼
                    ┌──────────────────────────────┐
             ┌──── │        DASHBOARD              │ ◄───── (home)
             │     └──────┬────┬────┬────┬────┬────┘
             │            │    │    │    │    │
             │     n      │ d  │ l  │ c  │ ?  │
             │     ▼      ▼    ▼    ▼    ▼    ▼
             │  ┌──────┐┌────┐┌───┐┌──────┐┌────┐
             │  │Deploy││Dev ││Log││Copilt││Help│
             │  │Wizard││Mgr ││   ││      ││ovrl│
             │  └──┬───┘└──┬─┘└─┬─┘└──┬───┘└──┬─┘
             │     │       │    │     │       │
             │     └───────┴────┴─────┴───────┘
             │               Esc = back to Dashboard
             │
             │  g = open Grafana (external browser)
             │  q = quit
             └─────────────────────────────────────
```

---

## Design Language Reference

### Color Palette (Tokyo Night)

```
Background:     #1a1b26   near-black
Panel border:   #414868   muted blue-gray
Title/Accent:   #7aa2f7   bright blue
Good (<50%):    #9ece6a   green
Warn (50-80%):  #e0af68   amber
Crit (>80%):    #f7768e   red/pink
Text primary:   #c0caf5   light blue-white
Text muted:     #565f89   dim gray
Selected row:   #283457   highlight blue
Success:        #73daca   teal
```

### Visual Components

| Component | Style | Implementation |
|-----------|-------|----------------|
| Panel borders | Rounded `╭╮╰╯` with title in top border | `lipgloss.RoundedBorder()` |
| Progress bars | `[████████░░░░░░]` gradient green→yellow→red | Custom renderer, threshold at 50%/80% |
| Sparklines | Braille `▁▂▃▄▅▆▇█` rolling 60-point history | `ntcharts` or custom braille |
| Metric rows | `LABEL  VAL  [BAR]  SPARKLINE` single line | Compound widget |
| Status dots | `●` green=live, `○` gray=offline, `⟳` yellow=starting | Unicode + lipgloss color |
| Selected row | Inverted bg `#283457` | Lipgloss `.Background()` |
| Step indicator | `Step 2 of 5: Target Device` in panel title | Title string |
| Keybind bar | `n new  s stop  l logs  d devices  q quit` | Fixed bottom row |

### LLM-Specific Metrics

| Metric | Source | Display |
|--------|--------|---------|
| Tokens/sec | vLLM `/metrics`, llama.cpp `/health` | Bar + sparkline in service row |
| Queue depth | vLLM pending requests | Numeric + color (0=green, >5=yellow, >20=red) |
| TTFT | vLLM metrics | Numeric in service detail |
| Active requests | vLLM `/metrics` | Numeric in service detail |
| VRAM per model | Container GPU memory | Bar in GPU panel + service row |

---

## Architecture Overview

```
┌──────────────────────────────────────┐
│          USER'S MACHINE              │
│                                      │
│  yokai (TUI binary)               │
│  ├─ Bubbletea event loop             │
│  ├─ Reads ~/.config/yokai/        │
│  │   └─ config.json                  │
│  ├─ Local background daemon          │
│  │   └─ Maintains persistent SSH     │
│  │      tunnels to all agents        │
│  │   └─ Aggregates metrics           │
│  │   └─ REST API on localhost:7473   │
│  └─ TUI connects to local daemon     │
│      (quit/relaunch without losing   │
│       connections)                   │
└──────────────┬───────────────────────┘
               │ SSH tunnel / REST over Tailscale or LAN
    ┌──────────▼───────────────────────────────┐
    │         TARGET DEVICE(s)                 │
    │                                          │
    │  yokai agent (systemd service)        │
    │  ├─ REST API :7474                       │
    │  ├─ Docker SDK → manage containers       │
    │  ├─ System metrics (CPU/RAM/disk)        │
    │  ├─ GPU metrics (nvidia-smi / rocm-smi)  │
    │  └─ Exposes /metrics (Prometheus format) │
    │                                          │
    │  Monitoring Stack (Docker Compose):       │
    │  ├─ Prometheus :9090                     │
    │  ├─ Grafana :3000 (pre-seeded dashboards)│
    │  ├─ node-exporter                        │
    │  └─ dcgm-exporter (NVIDIA)               │
    │                                          │
    │  Workload Containers:                    │
    │  ├─ vllm/vllm-openai :8000              │
    │  ├─ ghcr.io/ggml-org/llama.cpp :8080     │
    │  └─ comfyui :8188                        │
    └──────────────────────────────────────────┘
```

### Local Daemon

`yokai daemon` runs on the user's machine as a background service:
- Maintains SSH tunnels and agent connections persistently
- Aggregates metrics from all devices into a local cache (60-point ring buffers)
- Exposes `localhost:7473` REST API that the TUI reads from
- TUI is purely a view layer — quit and relaunch instantly
- Daemon handles reconnection, retries, health checks
- Auto-started by TUI if not running

---

## Config Schema (`~/.config/yokai/config.json`)

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
      "ssh_user": "sbull",
      "ssh_key": "~/.ssh/id_ed25519",
      "connection_type": "tailscale",
      "agent_port": 7474,
      "agent_token": "random-secret-generated-at-bootstrap",
      "gpu_type": "nvidia",
      "tags": ["primary"]
    }
  ],
  "services": [
    {
      "id": "vllm-llama31-8b",
      "device_id": "gaming-rig",
      "type": "vllm",
      "image": "vllm/vllm-openai:latest",
      "model": "meta-llama/Llama-3.1-8B-Instruct",
      "port": 8000,
      "extra_args": "--gpu-memory-utilization 0.9",
      "container_id": "abc123..."
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

Portable — copy to a new machine and `yokai` reconnects to the fleet.

---

## Agent REST API (port 7474)

| Endpoint | Method | Description |
|----------|--------|-------------|
| `/health` | GET | Ping, version, uptime |
| `/system/info` | GET | GPU type/count/VRAM, CPU, RAM, disk, Docker version, OS |
| `/metrics` | GET | Live CPU%, RAM, per-GPU util/VRAM/temp/power, per-container stats |
| `/metrics/prometheus` | GET | Prometheus scrape format |
| `/containers` | GET | List managed containers + status |
| `/containers` | POST | Deploy new workload (JSON spec) |
| `/containers/:id` | DELETE | Stop + remove |
| `/containers/:id/logs` | GET | SSE streaming logs |
| `/containers/:id/restart` | POST | Restart container |
| `/images/pull` | POST | Pull image, SSE progress |
| `/images/tags/:image` | GET | Fetch available tags from registry |

All require `Authorization: Bearer <agent_token>`.

---

## Tech Stack

| Layer | Choice | Rationale |
|-------|--------|-----------|
| Language | **Go 1.22+** | Single binary, fast, great SSH/Docker libs |
| TUI | **Bubbletea + Lipgloss + Bubbles** | opencode-style, composable |
| Charts | **NimbleMarkets/ntcharts** | Braille sparklines for Bubbletea |
| SSH | **golang.org/x/crypto/ssh** | Native, no deps |
| Docker | **docker/docker/client** SDK | Direct control |
| HTTP | **net/http** or **chi** | Lightweight |
| Config | **JSON** (`encoding/json`) | Portable |
| Tailscale | CLI wrapper (`tailscale status --json`) | No SDK needed |
| GPU | **nvidia-smi** / **rocm-smi** CLI | Parsed by agent |
| Monitoring | **Prometheus + Grafana** (Docker) | Industry standard |

---

## Project Structure

```
yokai/
├── architecture/                # ← NEW: markdown architecture docs
│   ├── README.md                # Index of all diagrams
│   ├── 01-system-overview.md
│   ├── 02-component-architecture.md
│   ├── 03-data-flow.md
│   ├── 04-network-topology.md
│   ├── 05-tui-screen-map.md
│   └── 06-agent-api.md
├── cmd/
│   └── yokai/
│       └── main.go              # tui / agent / daemon subcommands
├── internal/
│   ├── tui/
│   │   ├── app.go               # Root Bubbletea model, view router
│   │   ├── views/
│   │   │   ├── welcome.go       # Screen 1: connection type picker
│   │   │   ├── localnet.go      # Screen 1a: local IP selection
│   │   │   ├── tailscale.go     # Screen 1b: Tailscale peer list
│   │   │   ├── manual.go        # Screen 1c: manual IP entry
│   │   │   ├── sshcreds.go      # Screen 2: SSH credentials
│   │   │   ├── bootstrap.go     # Screen 3: device bootstrap
│   │   │   ├── hftoken.go       # Screen 4: HF token setup
│   │   │   ├── dashboard.go     # Screen 5: main dashboard
│   │   │   ├── deploy.go        # Screen 6: workload wizard
│   │   │   ├── devices.go       # Screen 7: device manager
│   │   │   ├── logs.go          # Screen 8: live log viewer
│   │   │   ├── copilot.go       # Screen 9: VS Code endpoints
│   │   │   └── help.go          # Screen 10: help overlay
│   │   ├── components/
│   │   │   ├── metricsbar.go    # Gradient progress bar
│   │   │   ├── sparkline.go     # Braille sparkline widget
│   │   │   ├── gpupanel.go      # GPU metrics panel
│   │   │   ├── servicelist.go   # Service table with live stats
│   │   │   ├── devicecard.go    # Device summary card
│   │   │   ├── keybinds.go      # Bottom keybind bar
│   │   │   ├── stepper.go       # Multi-step wizard component
│   │   │   └── overlay.go       # Modal overlay component
│   │   └── theme/
│   │       └── theme.go         # Lipgloss styles, Tokyo Night palette
│   ├── daemon/
│   │   ├── daemon.go            # Local background daemon
│   │   ├── aggregator.go        # Metrics aggregation from agents
│   │   └── tunnel.go            # SSH tunnel manager
│   ├── agent/
│   │   ├── server.go            # HTTP server
│   │   ├── handlers.go          # API handlers
│   │   ├── metrics.go           # System + GPU metric collection
│   │   └── docker.go            # Docker SDK wrapper
│   ├── ssh/
│   │   ├── client.go            # SSH connect, exec, SCP
│   │   └── bootstrap.go         # Agent deploy + systemd install
│   ├── tailscale/
│   │   └── tailscale.go         # CLI wrapper: status, peer list
│   ├── docker/
│   │   ├── catalog.go           # Image tag fetching from registries
│   │   └── compose.go           # Monitoring stack compose template
│   ├── hf/
│   │   └── client.go            # HF API: model search, GGUF file list
│   ├── config/
│   │   └── config.go            # Load/save/migrate config.json
│   └── vscode/
│       └── settings.go          # VS Code settings.json read/merge/write
├── assets/
│   ├── grafana/                 # Pre-built dashboard JSON
│   ├── prometheus/              # prometheus.yml templates
│   └── systemd/                 # yokai-agent.service template
├── go.mod
├── go.sum
├── Makefile
└── README.md
```

---

## Phased Implementation Plan

### Phase 0: Architecture Docs (Day 1)
- [ ] Create `architecture/` directory with all 6 diagram docs
- [ ] Create `architecture/README.md` index
- [ ] Create project root `README.md` with overview and link to architecture
- [ ] All diagrams use markdown (renders in GitHub, no external tools)

### Phase 1: Scaffold + Core (Week 1)
- [ ] `go mod init`, project structure, Makefile
- [ ] Config package: load/save/defaults for `~/.config/yokai/config.json`
- [ ] Basic Bubbletea app shell with view router
- [ ] Theme package: Tokyo Night palette, rounded borders, component styles
- [ ] SSH client: connect, exec, SCP upload
- [ ] Agent HTTP server skeleton: `/health`, `/system/info`

### Phase 2: Onboarding TUI (Week 2)
- [ ] Screen 1: Welcome (Local / Tailscale / Manual)
- [ ] Screen 1a: Local IP detection + selection
- [ ] Screen 1b: Tailscale detect, install instructions, peer list multi-select
- [ ] Screen 1c: Manual IP entry
- [ ] Screen 2: SSH credentials form + connection test
- [ ] Screen 3: Bootstrap (pre-flight checks, agent deploy, monitoring stack)
- [ ] Screen 4: HF token setup

### Phase 3: Agent + Metrics (Week 2-3)
- [ ] Agent: nvidia-smi GPU metrics parser
- [ ] Agent: CPU/RAM/disk metrics (procfs)
- [ ] Agent: Docker container list + stats
- [ ] Agent: full REST API implementation
- [ ] Agent: systemd service template

### Phase 4: Dashboard (Week 3)
- [ ] Screen 5: Dashboard layout with device panels + service list
- [ ] Components: metricsbar, sparkline, gpupanel, servicelist, devicecard
- [ ] Screen 5a: Service detail (expanded row)
- [ ] Multi-device view (side-by-side or stacked)
- [ ] Keybind bar
- [ ] Daemon: background service + metrics aggregation + tunnel management

### Phase 5: Workload Deployment (Week 4)
- [ ] Screen 6: Deploy wizard (5-step flow)
- [ ] Docker image catalog: fetch tags from Docker Hub / GHCR APIs
- [ ] HF model search API integration
- [ ] GGUF quantization picker
- [ ] Deploy pipeline: pull → run → health check → register
- [ ] Screen 8: Live log viewer
- [ ] Stop/restart/remove service actions

### Phase 6: Monitoring Stack (Week 5)
- [ ] Docker Compose template for Prometheus + Grafana + exporters
- [ ] Auto-deploy monitoring stack during bootstrap
- [ ] Grafana dashboard JSON seeding
- [ ] `g` keybind to open Grafana

### Phase 7: VS Code + Devices (Week 5)
- [ ] Screen 9: VS Code Copilot endpoint configuration
- [ ] Screen 7: Device manager (add/edit/remove/test)
- [ ] Screen 10: Help overlay

### Phase 8: Polish + Release (Week 6)
- [ ] Single binary build with subcommands
- [ ] Cross-compile: linux/amd64, linux/arm64, darwin/arm64
- [ ] `yokai upgrade` self-update
- [ ] GitHub Actions CI: build, release, checksums
- [ ] Install script + brew tap
- [ ] README with demo GIF/screenshot

---

## Edge Cases & Considerations

- **Port conflicts**: Check if ports are in use; suggest next available
- **Firewall**: Tailscale handles it; for LAN, warn if agent unreachable
- **Disk space**: Check before pulling (vLLM images are 10GB+); warn if <20GB free
- **VRAM estimation**: Estimate from model size, warn if won't fit (shown in Step 2 of wizard)
- **Multiple GPUs**: Support `--tensor-parallel` for vLLM
- **Container naming**: Deterministic `yokai-{type}-{model}` names
- **Agent crash recovery**: Re-adopt existing `yokai-*` containers on startup
- **Config migration**: Version field; auto-migrate on schema changes
- **Concurrent deploys**: Daemon serializes per device
- **Terminal resize**: All views must handle dynamic resize gracefully
- **SSH keepalive**: Daemon sends keepalive every 30s to prevent tunnel drops

---

## Open Questions

1. **Name**: Going with `yokai`?
2. **Daemon**: Worth the complexity of a persistent local background service? Alternative: TUI manages everything directly (simpler, but connections drop on exit).
3. **AMD ROCm**: Needed in v1, or NVIDIA-only?
4. **Model presets**: Ship a curated quick-pick list, or always search/type?
