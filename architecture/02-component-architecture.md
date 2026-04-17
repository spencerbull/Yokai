# L2: Component Architecture

Current Yokai component structure, centered on a Go daemon backend and a single OpenTUI frontend.

## Package Dependency Graph

```text
cmd/yokai/main.go
├── internal/opentui/      (launch OpenTUI client)
├── internal/daemon/       (daemon mode)
├── internal/agent/        (agent mode)
├── internal/cli/          (JSON-first non-TUI commands)
└── internal/upgrade/      (self-update)

internal/opentui/
└── launcher.go            ← starts daemon, launches bundled/source OpenTUI runtime

ui/tui/
├── src/app/               ← shell frame, routes, keymap
├── src/features/          ← dashboard, deploy, devices, logs, settings, onboarding
├── src/services/          ← daemon REST and SSE clients
├── src/contracts/         ← shared frontend-side data contracts
└── src/theme/             ← theme resolution and OpenTUI styling tokens

internal/daemon/
├── daemon.go              ← HTTP server on :7473, lifecycle
├── handlers_*.go          ← REST endpoint families
├── ui_*.go                ← UI-neutral helpers for frontend-facing flows
└── tunnel.go              ← SSH tunnel pool management

internal/agent/
├── server.go              ← HTTP server on :7474
├── handlers.go            ← route handlers
├── metrics.go             ← nvidia-smi / rocm-smi / procfs
└── docker*.go             ← Docker SDK wrappers and service operations

internal/config/
└── config.go              ← JSON load/save/migrate/defaults
```

## Frontend Structure

### OpenTUI Frontend (`ui/tui/`)

```text
src/
├── app/
│   ├── App.tsx            ← root shell, route switching, global keyboard handling
│   ├── keymap.ts          ← footer keymap definitions per route/mode
│   └── shell/
│       ├── ShellFrame.tsx ← chrome, route tabs, footer keymap bar
│       └── layout.ts      ← shell sizing helpers
├── features/
│   ├── dashboard/         ← fleet overview, service detail, logs routing, polling controllers
│   ├── deploy/            ← deploy wizard controller and screens
│   ├── devices/           ← device manager, overlays, import flows
│   ├── logs/              ← log pane and SSE stream hook
│   ├── onboarding/        ← first-run setup route
│   └── settings/          ← theme, defaults, and integration configuration
├── services/
│   ├── daemon-client.ts   ← REST client for daemon endpoints
│   └── sse.ts             ← SSE stream reader
├── contracts/             ← typed frontend models for daemon payloads
└── theme/                 ← terminal theme resolution and runtime context
```

Responsibilities:

- Keep UI state local to the OpenTUI shell and feature controllers.
- Route all system actions through daemon APIs.
- Treat logs as SSE streams and everything else as REST operations.
- Avoid direct filesystem, SSH, or external-tool access from the frontend.

## Backend Structure

### Daemon (`internal/daemon/`)

The daemon is the system boundary for the frontend.

Responsibilities:

- load and persist config
- discover and manage devices
- maintain SSH tunnels and device connectivity
- poll metrics from agents
- expose fleet, deploy, logs, settings, and integration APIs
- own side effects such as bootstrap, deploy, restart, delete, and config writes

### Agent (`internal/agent/`)

The agent runs on target devices and exposes container and metrics operations to the daemon.

Responsibilities:

- collect CPU, RAM, GPU, and container metrics
- manage service lifecycle operations
- stream logs and run service tests

## Interaction Boundaries

- `cmd/yokai/main.go` launches OpenTUI by default.
- `internal/opentui/launcher.go` ensures the daemon is available, then starts the OpenTUI runtime.
- `ui/tui` talks to the daemon over REST and SSE only.
- The daemon talks to agents and local system services.

## Design Notes

- There is a single supported terminal frontend: OpenTUI.
- The daemon remains UI-agnostic so future frontends can reuse the same APIs.
- Retired frontend code has been removed from the active architecture.
