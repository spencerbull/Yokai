# L5: TUI Screen Map

View hierarchy, navigation state machine, and keybindings.

## Screen Inventory

| # | Screen | File | Trigger | Parent |
|---|--------|------|---------|--------|
| 1 | Welcome | `welcome.go` | First run (no devices in config) | — |
| 1a | Local Network | `localnet.go` | Select "Local" in Welcome | Welcome |
| 1b | Tailscale | `tailscale.go` | Select "Tailscale" in Welcome | Welcome |
| 1c | Manual Entry | `manual.go` | Select "Manual" in Welcome | Welcome |
| 2 | SSH Credentials | `sshcreds.go` | After device selection | 1a/1b/1c |
| 3 | Bootstrap | `bootstrap.go` | After SSH connect | SSH Creds |
| 4 | HF Token | `hftoken.go` | After all devices bootstrapped | Bootstrap |
| 5 | Dashboard | `dashboard.go` | After onboarding / default view | — (home) |
| 5a | Service Detail | `dashboard.go` | Enter on service row | Dashboard |
| 6 | Deploy Wizard | `deploy.go` | `n` from Dashboard | Dashboard |
| 7 | Device Manager | `devices.go` | `d` from Dashboard | Dashboard |
| 8 | Log Viewer | `logs.go` | `l` from Dashboard | Dashboard |
| 9 | Copilot Endpoints | `copilot.go` | `c` from Dashboard | Dashboard |
| 10 | Help Overlay | `help.go` | `?` from anywhere | Any |

## Navigation State Machine

```
                              ┌──────────┐
                              │ Welcome  │  (first run only,
                              │          │   no devices in config)
                              └────┬─────┘
                       ┌───────────┼───────────┐
                       ▼           ▼           ▼
                  ┌─────────┐ ┌──────────┐ ┌────────┐
                  │Local IP │ │Tailscale │ │Manual  │
                  │ (1a)    │ │ (1b)     │ │ (1c)   │
                  └────┬────┘ └────┬─────┘ └───┬────┘
                       │          │            │
                       │    ┌─────┘      ┌─────┘
                       │    │            │
                       ▼    ▼            ▼
                    ┌───────────────────────┐
                    │   SSH Credentials (2) │  ← loops per device
                    └───────────┬───────────┘
                                │
                                ▼
                    ┌───────────────────────┐
                    │   Bootstrap (3)       │  ← loops per device
                    └───────────┬───────────┘
                                │
                                ▼
                    ┌───────────────────────┐
                    │   HF Token (4)        │
                    └───────────┬───────────┘
                                │
                                ▼
  ┌─────────────────────────────────────────────────────────┐
  │                    DASHBOARD (5)                         │  ◄── HOME
  │                                                         │
  │  Global keybinds:                                       │
  │    n → Deploy Wizard (6)                                │
  │    d → Device Manager (7)                               │
  │    l → Log Viewer (8)     (requires selected service)  │
  │    c → Copilot (9)                                      │
  │    g → Open Grafana (external browser, no screen change)│
  │    ? → Help Overlay (10)                                │
  │    q → Quit                                             │
  │                                                         │
  │  In-dashboard:                                          │
  │    Tab/Shift+Tab → cycle panel focus                    │
  │    ↑/↓           → navigate within panel                │
  │    1-9           → quick-switch device                  │
  │    Enter          → expand service detail (5a)          │
  │    s             → stop selected service                │
  │    r             → restart selected service             │
  │                                                         │
  └─────┬───────┬───────┬───────┬───────┬───────────────────┘
        │       │       │       │       │
   n    │  d    │  l    │  c    │  ?    │
        ▼       ▼       ▼       ▼       ▼
  ┌──────┐ ┌──────┐ ┌──────┐ ┌──────┐ ┌──────┐
  │Deploy│ │Device│ │ Log  │ │Copilt│ │ Help │
  │Wizard│ │ Mgr  │ │Viewer│ │      │ │Ovrlay│
  │ (6)  │ │ (7)  │ │ (8)  │ │ (9)  │ │ (10) │
  └──┬───┘ └──┬───┘ └──┬───┘ └──┬───┘ └──┬───┘
     │        │        │        │        │
     └────────┴────────┴────────┴────────┘
                    │
              Esc = pop back to Dashboard
```

## Deploy Wizard Internal Steps

```
Step 1: Workload Type  ──Enter──►  Step 2: Target Device
                                        │
                                   Enter │
                                        ▼
Step 3: Docker Image   ◄──Backspace── Step 2
     │
Enter│
     ▼
Step 4: Model Selection
     │
Enter│  (if llama.cpp: sub-step GGUF picker)
     ▼
Step 5: Configuration + Confirm
     │
Enter│ → Deploy (progress view)
     │     │
     │     Esc → background, return to Dashboard
     │     │
     │     Complete → return to Dashboard
     │
Esc at any step → cancel, return to Dashboard
Backspace at any step → go to previous step
```

## Keybind Summary

### Global (active from Dashboard)

| Key | Action | Target Screen |
|-----|--------|---------------|
| `n` | New service | Deploy Wizard |
| `d` | Device manager | Device Manager |
| `l` | View logs | Log Viewer |
| `c` | Copilot endpoints | Copilot |
| `g` | Open Grafana | External browser |
| `?` | Help | Help Overlay |
| `q` | Quit | — |

### Dashboard

| Key | Action |
|-----|--------|
| `Tab` | Cycle panel focus |
| `Shift+Tab` | Reverse cycle |
| `↑`/`↓` | Navigate within panel |
| `1`-`9` | Quick-switch device |
| `Enter` | Expand/collapse service detail |
| `s` | Stop selected service |
| `r` | Restart selected service |

### Log Viewer

| Key | Action |
|-----|--------|
| `↑`/`↓` | Scroll |
| `PgUp`/`PgDn` | Page scroll |
| `f` | Toggle follow mode |
| `/` | Search logs |
| `Esc` | Back to Dashboard |

### Wizards

| Key | Action |
|-----|--------|
| `Enter` | Next step / confirm |
| `Backspace` | Previous step |
| `Space` | Toggle selection (multi-select) |
| `Tab` | Next field |
| `Esc` | Cancel |

### Device Manager

| Key | Action |
|-----|--------|
| `a` | Add device |
| `e` | Edit selected device |
| `x` | Remove selected device |
| `t` | Test connection |
| `Esc` | Back to Dashboard |

### Help Overlay

| Key | Action |
|-----|--------|
| Any key | Close overlay |
