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

## How to Read

- Start with **L1** for the big picture
- **L2** shows how code is organized internally
- **L3** traces specific data paths end-to-end
- **L4** covers networking, security, and connectivity
- **L5** maps every TUI screen and how users navigate between them
- **L6** is the API reference for the agent service

All diagrams are rendered in markdown using ASCII art — no external tools required.
