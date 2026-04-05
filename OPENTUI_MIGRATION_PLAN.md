# OpenTUI Migration Plan

Living plan for migrating Yokai's current Bubble Tea TUI to an OpenTUI-based frontend while keeping the Go backend independent and reusable for future UIs.

## Status

| Area | Status | Notes |
|---|---|---|
| Discovery and current-state inventory | Done | Current TUI architecture and feature surface mapped |
| Migration direction | Done | Backend remains UI-agnostic; frontend becomes OpenTUI client |
| Plan document | Done | This file is the execution tracker |
| Backend API expansion | Not started | Needed to move frontend-only logic behind daemon APIs |
| OpenTUI frontend scaffold | In progress | `ui/tui` package boundary selected and initial files created |
| Feature migration | Not started | Dashboard first, then remaining flows |

## Locked Decisions

- Keep the backend service independent from the frontend.
- Replace the current Bubble Tea frontend with a new OpenTUI frontend.
- Preserve full feature parity with the current TUI.
- Improve flows where UX can be made clearer or more intuitive.
- Keep the daemon as the main backend boundary so future interfaces can reuse the same APIs.
- Use OpenTUI React as the framework for the new frontend.

## Framework Choice

**Framework:** `@opentui/react`

Why this is the default choice:

- Yokai already has complex view state, wizards, overlays, and cross-screen shared state.
- React is the best fit for a shell-oriented app with route stacks, reducers, and reusable UI primitives.
- OpenTUI React has the right primitives for this work: `useKeyboard`, `useTerminalDimensions`, `useTimeline`, test rendering, and declarative component composition.
- We can still drop to `@opentui/core` later for a custom high-performance widget if profiling shows a real need.

## OpenTUI References Consulted

- `.opencode/skill/opentui/references/react/REFERENCE.md`
- `.opencode/skill/opentui/references/react/configuration.md`
- `.opencode/skill/opentui/references/react/patterns.md`
- `.opencode/skill/opentui/references/react/api.md`
- `.opencode/skill/opentui/references/react/gotchas.md`
- `.opencode/skill/opentui/references/layout/REFERENCE.md`
- `.opencode/skill/opentui/references/layout/patterns.md`
- `.opencode/skill/opentui/references/keyboard/REFERENCE.md`
- `.opencode/skill/opentui/references/animation/REFERENCE.md`
- `.opencode/skill/opentui/references/testing/REFERENCE.md`
- `.opencode/skill/opentui/references/components/containers.md`
- `.opencode/skill/opentui/references/components/inputs.md`
- `.opencode/skill/opentui/references/components/text-display.md`
- `.opencode/skill/opentui/references/core/gotchas.md`
- `architecture/07-daemon-ui-api.md`

## Current State Summary

Current entrypoint and shell:

- `cmd/yokai/main.go` launches the TUI by default.
- `internal/tui/app.go` owns the root Bubble Tea shell.
- The current shell manages view stack, tabs, status bar, toasts, window sizing, and global navigation.

Current user-visible areas:

- Onboarding: welcome, local/manual/Tailscale/SSH config flows, SSH credentials, bootstrap, Hugging Face token.
- Dashboard: fleet overview, device detail, service detail/actions, polling, sparkline history, marquee overflow handling.
- Deploy: workload/device/image/model/config/deploy wizard, Hugging Face search, presets, vLLM memory helper.
- Devices: add, edit, test, upgrade, bulk actions, remove with optional cleanup.
- Logs: SSE streaming logs with follow and paging.
- Settings: AI tool configuration for VS Code Copilot, OpenCode, and OpenClaw.
- Shared UX: help overlay, confirm dialogs, status bar, toasts, top tabs.

Current migration concerns already identified:

- Dashboard is the highest-risk view because it combines polling, metrics shaping, selection state, actions, history buffers, and layout in one large file.
- Some frontend logic currently performs local machine work directly and must move behind backend APIs.
- Some docs and current behavior have drifted, so parity should follow actual code behavior first, then intentional UX improvements.

## Migration Goals

- Deliver a polished OpenTUI frontend with clear focus handling, responsive layout, and restrained animation.
- Keep all existing capabilities available to the user.
- Move all UI-triggered system actions behind explicit backend APIs.
- Make the daemon usable by multiple frontends, not just the OpenTUI client.
- Improve shell consistency, navigation clarity, and context-sensitive flows.

## Non-Goals

- Rewriting the agent or core device-management logic unless required for API cleanup.
- Replacing the daemon with a frontend-coupled service.
- Adding large net-new features before parity is reached.
- Turning the backend into an OpenTUI-specific API.

## High-Level Target Architecture

### Backend

The Go daemon remains the system boundary and source of truth for:

- device inventory
- service inventory
- metrics polling
- container lifecycle operations
- deploy requests
- log streaming
- config persistence
- onboarding-related host operations
- external tool integration configuration

The backend should expose UI-neutral REST and SSE endpoints. The frontend should never shell out, inspect SSH config directly, call Tailscale directly, or write integration config files directly.

### Frontend

New frontend app:

- runtime: Bun
- language: TypeScript
- framework: OpenTUI React
- renderer: `createCliRenderer`
- tests: OpenTUI test renderer + Bun test runner

Proposed frontend layout:

```text
ui/tui/
  package.json
  tsconfig.json
  src/
    index.tsx
    app/
      App.tsx
      routes.ts
      keymap.ts
      shell/
        ShellFrame.tsx
        RouteStack.tsx
        TabBar.tsx
        StatusBar.tsx
        ToastViewport.tsx
        ModalLayer.tsx
    services/
      daemon-client.ts
      sse.ts
      polling.ts
      operations.ts
    contracts/
      api.ts
      models.ts
    features/
      onboarding/
      dashboard/
      deploy/
      devices/
      logs/
      settings/
    ui/
      primitives/
      widgets/
      theme/
    testing/
      fixtures/
      snapshots/
      interactions/
```

### Frontend State Boundaries

Shell state:

- active top-level route
- route stack
- modal visibility
- toast queue
- terminal dimensions
- global help visibility

Feature-local state:

- onboarding progression
- dashboard selection, focus, inspector state
- deploy wizard data and validation state
- device manager selection and action state
- logs follow mode and viewport state
- settings selection and action state

Server state:

- device list
- metrics snapshots
- operation progress
- log streams
- configuration-backed user settings and history

## Backend API Strategy

### Keep Existing API Families

The daemon already exposes useful endpoints for:

- health
- devices
- metrics
- deploy
- container stop/restart/remove/test
- logs SSE
- image tags
- daemon reload

These should remain backend-owned and continue to serve all future frontends.

### Add Missing UI-Neutral API Families

The current TUI still performs several frontend-only operations directly. Those responsibilities should move into backend-owned APIs.

#### Discovery APIs

- `GET /discovery/ssh-config-hosts`
- `GET /discovery/tailscale/status`
- `GET /discovery/tailscale/peers`
- `GET /discovery/local-network`

Notes:

- Preserve current parity for local network first if needed.
- If real subnet discovery is implemented, do it in the backend, not the frontend.

#### Device Lifecycle APIs

- `POST /devices`
- `PATCH /devices/{deviceID}`
- `POST /devices/{deviceID}/test`
- `POST /devices/{deviceID}/upgrade`
- `DELETE /devices/{deviceID}` with optional cleanup flag
- `POST /devices/test-all`
- `POST /devices/upgrade-all`

#### Onboarding and Bootstrap APIs

- `POST /bootstrap/preflight`
- `POST /bootstrap/device`
- `POST /bootstrap/monitoring`
- `POST /bootstrap/hf-token/validate`
- `PUT /settings/hf-token`

#### History and Settings APIs

- `GET /history/deploy`
- `PUT /history/deploy`
- `GET /settings`
- `PATCH /settings`

#### Integration APIs

- `GET /integrations/openai-endpoints`
- `POST /integrations/configure`
- `GET /integrations/status`

### Long-Running Operations Pattern

Several actions are slow enough that they should be modeled as operations instead of blocking requests.

Use one consistent shape for:

- bootstrap
- monitoring install
- device test
- device upgrade
- bulk test
- bulk upgrade
- integration configuration

Suggested endpoint family:

- `POST /operations/{kind}`
- `GET /operations/{id}`
- `GET /operations/{id}/events`

Benefits:

- backend stays reusable for non-TUI clients
- frontend gets a uniform progress model
- logs, spinners, and progress panels become simpler to implement

### Contract Rules

- Keep request and response models UI-neutral.
- Prefer explicit DTOs over leaking internal config structs directly.
- Avoid route names tied to screen names.
- Return enough status information for a web UI or another TUI to consume later.
- Use SSE or polling only where it adds value.

## Frontend UX Redesign

Feature parity remains mandatory, but the flows can be reorganized for clarity.

### Shell

Redesign goals:

- persistent shell after onboarding
- consistent top-level navigation
- contextual inspectors instead of extra full-screen views where possible
- consistent focus styling and modal handling

Proposed top-level tabs:

- Dashboard
- Devices
- Deploy
- Settings

Notes:

- Logs should become a contextual route, not a primary tab.
- Help should be a global overlay from anywhere appropriate.

### Onboarding

Current behavior should be preserved, but reorganized into a more coherent single wizard.

Proposed flow:

1. Choose source: Quick Start, Tailscale, SSH Config, Manual.
2. Search and select target.
3. Review and edit connection details.
4. Run preflight and bootstrap.
5. Optionally install monitoring.
6. Set or validate Hugging Face token.
7. Enter Dashboard.

UX improvements:

- unify all discovery lists into one shared searchable layout
- show more explicit status and next-step messaging
- use progress panels for bootstrap instead of fragmented screens

### Dashboard

This is the main product surface and the first migration priority.

Proposed layout on wide terminals:

- left rail: devices and fleet summary
- center: service tables and key metrics
- right inspector: selected device or selected service details

Behavioral goals:

- maintain overview and detail functionality without forcing unnecessary screen changes
- logs open contextually from the selected service
- keep key service actions available from the current selection
- preserve polling and history charts
- only animate focus and user-triggered transitions

### Deploy

Keep full capability while simplifying the decision tree.

Proposed stages:

1. Workload type
2. Target device
3. Image and model
4. Runtime config and review
5. Progress and completion

Required parity items:

- image history
- model history
- Hugging Face search
- BKC presets
- vLLM memory helper
- custom image/model entry
- config preview before deploy

### Devices

Use a split view instead of a single list-only manager.

Proposed layout:

- left: device list
- right: details, last status, actions, and edit affordances

Required parity items:

- add
- edit
- test selected
- upgrade selected
- bulk test
- bulk upgrade
- remove with optional remote cleanup

### Logs

Logs become contextual rather than a top-level tab.

Proposed behavior:

- open from selected service
- full screen on narrow terminals
- inspector or slide-over panel on wide terminals
- preserve follow mode, paging, and connection status

### Settings

Settings should become broader than the current AI tools view.

Proposed sections:

- AI integrations
- Hugging Face token
- deploy defaults and history
- future UI preferences if needed

Keep current AI-tool configuration capability as part of this area.

## Animation and Motion Rules

OpenTUI gives us animation tools, but we should stay disciplined.

Use motion for:

- tab transitions
- modal and toast enter/exit
- focus transitions
- progress bars
- route-stack transitions where they improve orientation
- overflow marquee for the actively focused row only

Do not animate:

- live metrics updates
- incoming log lines
- full table refreshes
- large background polling changes

Implementation guidance:

- prefer `easeOutQuad` or `easeOutCubic`
- keep durations short
- round animated character-cell values
- clip overflow by default and only animate the active/focused item

## OpenTUI Implementation Notes

Important framework notes that should guide implementation:

- Use Bun, not Node, for the OpenTUI frontend runtime.
- Never call `process.exit()` directly; use `renderer.destroy()`.
- Input and select controls must be explicitly focused.
- Text does not wrap by default, so widths and overflow behavior must be deliberate.
- Mouse events are supported, but keyboard flow must remain first-class.
- Multiple `useKeyboard` handlers can conflict; keep routing disciplined.

## Feature Parity Matrix

| Area | Current capability | Migration target | Backend work needed | Frontend status |
|---|---|---|---|---|
| Shell | tabs, status bar, toasts, modal stack | persistent OpenTUI shell | minimal | Not started |
| Welcome | choose onboarding path | unified onboarding entry | none or light | Not started |
| Local network | current local-network path | searchable discovery path | likely yes | Not started |
| Tailscale | status, peer list, search, tags | shared discovery list with details | yes | Not started |
| SSH config | discover and choose hosts | shared discovery list with details | yes | Not started |
| Manual entry | host/IP entry | connection details form | yes | Not started |
| SSH credentials | user, port, key, passphrase, password | connection details form | yes | Not started |
| Bootstrap | preflight, deploy agent, optional monitoring | operation-driven progress flow | yes | Not started |
| HF token | detect, input, validate, save | onboarding + settings section | yes | Not started |
| Dashboard overview | fleet, device list, service previews | multi-pane dashboard | minimal | Not started |
| Dashboard detail | device detail, service detail | inspector-based workflow | minimal | Not started |
| Service actions | logs, stop, restart, delete, test | dashboard actions and contextual logs | minimal | Not started |
| Deploy | full multi-step wizard | simplified but equivalent wizard | some settings/history APIs | Not started |
| Devices | add, edit, test, upgrade, bulk, remove | split device manager | yes | Not started |
| Logs | SSE, follow, page scroll | contextual log viewer | minimal | Not started |
| AI tools | endpoint discovery + tool config | settings/integrations section | yes | Not started |
| Help and confirm | overlay and confirms | global help + shared modals | none | Not started |

## Phased Delivery Plan

### Phase 0: Contract Freeze and Project Prep

Goal: freeze the migration architecture and prepare the repo for a parallel frontend.

Checklist:

- [x] Confirm the frontend package location and naming.
- [ ] Decide how `yokai` launches or bundles the new frontend.
- [ ] Write API contracts for all missing backend-owned operations.
- [ ] Define shared DTOs for devices, services, metrics, operations, settings, and discovery data.
- [ ] Decide whether any existing daemon endpoints should be versioned or reshaped.
- [ ] Create a migration tracking convention for this file.

### Phase 1: Backend API Expansion

Goal: make the backend fully capable of serving any frontend without UI-local side effects.

Checklist:

- [ ] Add discovery APIs for SSH config, Tailscale, and local-network flows.
- [ ] Add CRUD-style device APIs and bulk-action APIs.
- [ ] Add bootstrap and monitoring operation endpoints.
- [ ] Add settings and history endpoints.
- [ ] Add integrations endpoints for AI-tool setup.
- [ ] Add operation status and event streaming where needed.
- [ ] Add backend tests for new API families.

### Phase 2: OpenTUI Frontend Scaffold

Goal: create the OpenTUI frontend foundation without feature work yet.

Checklist:

- [ ] Create the Bun + TypeScript OpenTUI React app.
- [ ] Configure TypeScript, test runner, and scripts.
- [ ] Build the app shell, route stack, keymap, and modal/toast layers.
- [ ] Add API client, SSE client, and polling abstractions.
- [ ] Add theme primitives, panel primitives, and focus styling.
- [ ] Add test-render utilities and initial snapshots.

### Phase 3: Dashboard and Logs

Goal: port the highest-value read path first.

Checklist:

- [ ] Implement dashboard shell layout with responsive wide/narrow modes.
- [ ] Implement device rail and summary widgets.
- [ ] Implement service tables and inspector panels.
- [ ] Implement metrics polling and history charts.
- [ ] Implement contextual logs viewer over SSE.
- [ ] Port service actions: test, stop, restart, delete.
- [ ] Add dashboard interaction tests and visual snapshots.

### Phase 4: Devices and Shared Operations

Goal: port the device-management workflows and mutation-heavy interactions.

Checklist:

- [ ] Implement the device manager split view.
- [ ] Implement add-device entry into onboarding.
- [ ] Implement edit-device flow.
- [ ] Implement selected and bulk test/upgrade actions.
- [ ] Implement remove-device flow with optional cleanup.
- [ ] Add device-manager tests.

### Phase 5: Onboarding and Bootstrap

Goal: replace the first-run experience end to end.

Checklist:

- [ ] Implement onboarding entry screen.
- [ ] Implement shared discovery list UI.
- [ ] Implement connection details forms.
- [ ] Implement bootstrap progress route using backend operations.
- [ ] Implement optional monitoring install step.
- [ ] Implement Hugging Face token validation and save flow.
- [ ] Add onboarding interaction tests.

### Phase 6: Deploy and Settings

Goal: complete parity for deploy workflows and integration management.

Checklist:

- [ ] Implement the deploy wizard shell.
- [ ] Implement device, image, and model pickers.
- [ ] Implement Hugging Face search integration.
- [ ] Implement BKC preset application.
- [ ] Implement vLLM memory helper UI.
- [ ] Implement deploy progress and completion handling.
- [ ] Implement settings sections for integrations, HF token, and defaults/history.
- [ ] Add deploy and settings tests.

### Phase 7: Polish, Hardening, and Cutover

Goal: make the new frontend production-ready.

Checklist:

- [ ] Add motion polish using the approved animation rules.
- [ ] Tune overflow handling and active-row marquee.
- [ ] Verify keyboard and mouse parity where intended.
- [ ] Run full parity audit against the current Bubble Tea TUI.
- [ ] Decide cutover strategy and fallback behavior.
- [ ] Update docs and screenshots.

## Testing Strategy

Backend:

- add handler tests for new APIs
- add operation-state tests for long-running flows
- add integration tests where config persistence or side effects matter

Frontend:

- snapshot tests for shell, dashboard, deploy, onboarding, and settings layouts
- interaction tests for keyboard navigation, focus, modal behavior, and list selection
- fixture-based tests for metrics polling and log streaming
- responsive tests for wide and narrow terminal sizes

Parity verification:

- maintain a checklist of current features mapped to the new frontend
- mark each item complete only after both behavior and UX pass are verified

## Risks and Mitigations

| Risk | Impact | Mitigation |
|---|---|---|
| Backend API gap is larger than expected | High | Move UI-local logic behind small, explicit APIs first |
| Dashboard rerender cost or poor layout performance | High | Keep dashboard state local and profile early with realistic fixtures |
| Packaging Bun frontend into the existing product is awkward | Medium | decide launch and packaging approach during Phase 0 |
| Parity drift between docs and current behavior | Medium | use current code as source of truth, then apply intentional redesign |
| Scope growth before parity | High | hold net-new features until parity checklist is complete |
| Motion makes the app noisy | Medium | follow strict animation rules and test on real terminal sizes |

## Definition of Done

The migration is complete when all of the following are true:

- the OpenTUI frontend covers the same feature surface as the current TUI
- backend-owned APIs fully replace frontend-local system operations
- dashboard, deploy, onboarding, devices, logs, and settings are all ported
- interaction and snapshot tests cover the primary flows
- the old Bubble Tea frontend can be retired or intentionally kept as a fallback
- docs are updated to reflect the new architecture and user flows

## Progress Log

Use this section to keep a short running log as work lands.

- 2026-04-03: Completed current-state inventory and selected OpenTUI React as the target frontend framework.
- 2026-04-03: Locked the backend strategy to remain UI-agnostic and reusable across future interfaces.
- 2026-04-03: Created this migration tracking document.
- 2026-04-03: Selected `ui/tui` as the OpenTUI frontend package location and `@yokai/tui` as the package name.
- 2026-04-03: Added `architecture/07-daemon-ui-api.md` to define the daemon-facing UI contract.
- 2026-04-03: Created the initial `ui/tui` Bun + OpenTUI React scaffold.
- 2026-04-03: Implemented daemon-owned settings and deploy-history endpoints plus the first typed frontend daemon client module.
- 2026-04-03: Added read-only OpenTUI dashboard parity slices for fleet overview, per-device summary, inspector, and contextual logs.
- 2026-04-03: Added OpenTUI dashboard service actions for stop, restart, test, and delete with inline confirm/status UX.
- 2026-04-03: Moved service-config cleanup on delete into the daemon so the frontend no longer owns that mutation.
- 2026-04-03: Added daemon-backed device management APIs for device upsert/update/delete, device test, SSH-config discovery, and tunnel error reporting.
- 2026-04-03: Added the first OpenTUI Devices route with device inventory, details, add/edit modal, SSH-config import modal, test, and remove flows.
- 2026-04-03: Added daemon-backed Tailscale discovery APIs and an OpenTUI Tailscale import modal with AI GPU tag highlighting and fuzzy peer filtering.
- 2026-04-03: Reworked device add into a guided wizard flow that routes users through Manual, SSH Config, or Tailscale instead of separate top-level import keybinds.
- 2026-04-03: Changed add-device create mode to bootstrap the remote Yokai agent by default, with manual existing-agent add still available when an agent token is supplied.
- 2026-04-03: Added a first-run OpenTUI onboarding route that appears automatically when no devices are configured and reuses the shared bootstrap/device setup flow.
- 2026-04-03: Restored the optional monitoring install prompt in the shared bootstrap flow so onboarding and device add can deploy Prometheus/Grafana during bootstrap.
- 2026-04-03: Restored SSH auth selection in the shared setup flow with SSH agent, key-file passphrase, and password-based bootstrap support.
- 2026-04-03: Replaced the placeholder Settings route with a daemon-backed settings screen for HF token validation/save, deploy defaults, integration status, and AI tool configuration.
- 2026-04-03: Replaced the placeholder Deploy route with a daemon-backed deploy wizard using defaults/history, Hugging Face model search, and backend-owned service persistence.
- 2026-04-03: Added daemon-backed selected-device upgrade plus bulk test/upgrade actions to the Devices route, and fixed clickable route tabs plus non-conflicting Settings shortcuts.

## Immediate Next Steps

- [x] Finalize the exact frontend package location and naming.
- [x] Define the missing backend API contracts in detail.
- [x] Scaffold the OpenTUI frontend package.
- [ ] Add daemon DTOs and handlers for the first new endpoint family.
- [ ] Add the first typed daemon client module in `ui/tui`.
- [x] Add daemon DTOs and handlers for the first new endpoint family.
- [x] Add the first typed daemon client module in `ui/tui`.
- [ ] Start implementation with dashboard and logs once the API surface is ready.
- [x] Start implementation with dashboard and logs once the API surface is ready.
- [ ] Add richer dashboard history/sparkline views.
- [x] Port device manager mutation flows after dashboard service actions settle.
- [ ] Add true onboarding/bootstrap flows in OpenTUI.
- [x] Add true onboarding/bootstrap flows in OpenTUI.
- [x] Connect Tailscale discovery into the future onboarding/bootstrap flow.
- [x] Implement a real OpenTUI deploy route with daemon-backed deploy submission and useful defaults/history.
- [x] Add daemon and OpenTUI support for selected-device upgrade plus bulk test/upgrade flows.
