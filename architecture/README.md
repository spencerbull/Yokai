# yokai Architecture

This directory contains multi-level architecture documentation for yokai, a TUI + distributed agent system for managing LLM services across GPU devices.

## Diagram Index

| Level | Document | Description |
|-------|----------|-------------|
| L1 | [System Overview](01-system-overview.md) | Bird's-eye view — user machine ↔ target devices |
| L2 | [Component Architecture](02-component-architecture.md) | Internal components: TUI, Daemon, Agent with Go package boundaries |
| L3 | [Data Flow](03-data-flow.md) | Metrics polling, deploy commands, config, log streaming |
| L4 | [Network Topology](04-network-topology.md) | SSH tunnels, Tailscale mesh, ports, auth |
| L5 | [TUI Screen Map](05-tui-screen-map.md) | View hierarchy, navigation state machine, keybinds |
| L6 | [Agent API](06-agent-api.md) | REST API contract, request/response schemas, SSE streaming |
| L7 | [Daemon UI API](07-daemon-ui-api.md) | UI-neutral daemon contract for the OpenTUI frontend and future clients |
| L8 | [Multi-Device Deployments](08-multi-device-deployments.md) | Atomic two-device deployment state, validation, ownership, and rollback |

## How to Read

- Start with **L1** for the big picture
- **L2** shows how code is organized internally
- **L3** traces specific data paths end-to-end
- **L4** covers networking, security, and connectivity
- **L5** maps every TUI screen and how users navigate between them
- **L6** is the API reference for the agent service
- **L7** defines the daemon-facing API used by the new OpenTUI frontend and any future UI
- **L8** defines the additive coordinated-deployment transaction and its secret/ownership boundaries

All diagrams are rendered in markdown using ASCII art — no external tools required.

## Qwen coordinator bootstrap invariant

Each Qwen-capable agent must be bootstrapped with its stable coordinator-side device ID and the coordinator public verification key in `agent.json`. The daemon signs the exact deployment binding device ID, and the agent compares that signed target with its local configured identity before replay consumption or Docker/image work. Re-run `yokai devices bootstrap <device-id>` after adding this authorization version, changing a device ID, or rotating the coordinator signing key; a binary-only upgrade intentionally leaves the Qwen capability unavailable when identity, verifier, or replay state is absent.
