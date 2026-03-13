# Yokai TUI Revamp — btop-Dense Design Plan

## Vision
Transform Yokai's dashboard into a **btop++-grade monitoring interface** — information-dense, braille-resolution charts, gradient color coding, every pixel of terminal real estate used. The aesthetic of btop meets the developer UX of lazygit/k9s.

## Design Direction: "btop Dense" (Mockup #1)
- Maximum data density per terminal row
- Braille-character sparkline/stream charts (U+2800-U+28FF)
- Color gradient graphs: green → yellow → red based on utilization thresholds
- Side-by-side panel grid layout with thin rounded borders
- Tokyo Night color palette (keep existing, enhance)
- Context-sensitive keybindings bar at bottom
- Tab navigation at top

## Architecture Constraints
- **Stack:** Go + BubbleTea + Lipgloss + ntcharts (already in go.mod)
- **DO NOT change:** daemon API, config format, SSH/Docker logic, BubbleTea Model/View/Update pattern
- **Keep:** Tokyo Night theme colors, all existing functionality
- **Branch:** `feat/tui-btop-revamp` off current `feat/tui-tabs`

---

## Phase 1: Dashboard Grid Layout (Highest Visual Impact)

### 1.1 Dashboard Layout Overhaul (`views/dashboard.go`)

Replace the current vertical stack layout with a responsive grid:

```
┌─ Tab Bar ──────────────────────────────────────────────────────────┐
│  1[Dashboard]  2 Devices  3 Deploy  4 Logs  5 Settings             │
├────────────────────────────────────────────────────────────────────┤
│ ╭─ finn · 100.64.0.1 ● ─────────╮ ╭─ beskar · 100.126.187.30 ● ╮│
│ │ RTX Pro 6000 96GB              │ │ Jetson AGX Orin 64GB       ││
│ │ Util 45% [████████░░░░░░] │ │ Util 72% [██████████████░] ││
│ │ VRAM 38.2/96.0 GB             │ │ VRAM 48.1/64.0 GB          ││
│ │ Temp 62°C  Pwr 285/350W       │ │ Temp 55°C  Pwr 45/60W      ││
│ ╰────────────────────────────────╯ ╰────────────────────────────╯│
├────────────────────────────────────────────────────────────────────┤
│ ╭ CPU 45% ──────────╮ ╭ RAM 62% ──────────╮ ╭ GPU 72% ─────────╮│
│ │⣿⣷⣯⡿⣿⣾⣿⣷⣯⡿⣿⣾⣿│ │⣿⣷⣯⡿⣿⣾⣿⣷⣯⡿⣿⣾⣿│ │⣿⣷⣯⡿⣿⣾⣿⣷⣯⡿⣿⣾⣿││
│ │⣿⣷⣯⡿⣿⣾⣿⣷⣯⡿⣿⣾⣿│ │⣿⣷⣯⡿⣿⣾⣿⣷⣯⡿⣿⣾⣿│ │⣿⣷⣯⡿⣿⣾⣿⣷⣯⡿⣿⣾⣿││
│ │⣿⣷⣯⡿⣿⣾⣿⣷⣯⡿⣿⣾⣿│ │⣿⣷⣯⡿⣿⣾⣿⣷⣯⡿⣿⣾⣿│ │⣿⣷⣯⡿⣿⣾⣿⣷⣯⡿⣿⣾⣿││
│ ╰───────────────────╯ ╰───────────────────╯ ╰──────────────────╯│
├────────────────────────────────────────────────────────────────────┤
│ ╭─ Services ──────────────────────────────────────────────────────╮│
│ │ ● vLLM    Qwen3-Next-80B      finn    :8000   142 t/s   4h32m ││
│ │   llama   Mistral-7B-Q4_K_M   beskar  :8080    38 t/s   1d2h  ││
│ │   comfy   ComfyUI             finn    :8188    -         6h15m ││
│ ╰─────────────────────────────────────────────────────────────────╯│
├────────────────────────────────────────────────────────────────────┤
│ yokai > Dashboard          h/l device  j/k svc  n new  ? help  q │
└────────────────────────────────────────────────────────────────────┘
```

**Responsive breakpoints:**
- **Wide (≥140 cols):** Full 3-column layout as shown above
- **Medium (100-139 cols):** 2 device cards per row, 2 charts per row (GPU chart below)
- **Narrow (<100 cols):** Single column stack (current behavior, but with better charts)

### 1.2 Grid Layout Implementation

In `dashboard.go` `View()` method:
- Use `lipgloss.JoinHorizontal(lipgloss.Top, ...)` for side-by-side panels
- Use `lipgloss.JoinVertical(lipgloss.Left, ...)` for stacking rows
- Calculate available width dynamically: `availableWidth = d.width - 4` (margins)
- Device card width: `(availableWidth - gap) / numDevices` (max 2-3 per row)
- Chart width: `(availableWidth - 2*gap) / 3` for 3-column chart row

---

## Phase 2: Chart Upgrades (ntcharts Integration)

### 2.1 Enhanced StreamChart Component (`components/streamchart.go`)

The existing `StreamChart` wrapper works but needs enhancements:

- **Color gradients based on value:** Green (<50%), Yellow (50-80%), Red (>80%)
  - Use multiple data series pushed at different thresholds, or post-process with lipgloss coloring
- **Filled area under curve:** Use `slc.WithFill()` option if available, or use ArcLineStyle for solid lines
- **Y-axis labels:** Show "0%", "50%", "100%" on left edge
- **Current value badge:** Show latest value in the panel title with color coding
- **Chart height:** Default 6 rows for dashboard (braille = 4 dots per row = 24 vertical pixels)

### 2.2 GPU Utilization Charts

Replace the text-based GPU progress bars with **ntcharts streamline charts** for GPU utilization over time:
- One chart per GPU showing utilization % history (60 samples, 2-min window)
- Color: amber/orange (`#e0af68`)
- Overlay current value in panel title

### 2.3 VRAM Bar Charts

Use **ntcharts bar charts** for VRAM comparison across GPUs:
- Horizontal bars showing used/total VRAM
- Color-coded: green (<50%), yellow (50-80%), red (>80%)
- Label: "38.2/96.0 GB" on each bar

### 2.4 Temperature Sparkline

Add a small inline temperature history sparkline next to temp display:
- 30-sample history
- Color zones: green (<60°C), yellow (60-80°C), red (>80°C)
- Compact: fits in 1 line, ~15 chars wide

---

## Phase 3: Component Polish

### 3.1 Progress Bar Enhancement (`components/metricsbar.go`)

The existing `GradientProgressBar` in gpupanel.go is good. Standardize across all components:
- Use fractional block characters (`▏▎▍▌▋▊▉█`) — already implemented
- Add **value-based color gradient within the bar itself:**
  - First 50% of fill: green
  - 50-80%: transitions to yellow
  - 80-100%: transitions to red
- Apply to: GPU util, VRAM, CPU, RAM, disk, temperature bars

### 3.2 Device Card Density (`components/devicecard.go`)

Make device cards more compact and info-dense:
- **Line 1:** `hostname · IP ● online` (keep current)
- **Line 2:** GPU name + count (e.g., `2× RTX 4090`)
- **Line 3:** Util bar + VRAM bar side-by-side (compact)
- **Line 4:** Temp | Power | Fan in one line (keep current)
- Remove excessive padding, maximize information per line

### 3.3 Service Table Enhancement (`components/servicelist.go`)

- **Alternating row backgrounds:** Even rows get subtle highlight (`#283457`)
- **Color-coded status dots:** Green (running/healthy), amber (starting/unhealthy), red (error/stopped)
- **Tok/s with trend arrow:** `142 ↑` or `38 →` based on recent change
- **Sortable columns:** Header click or keybind cycles sort (name, status, tok/s, uptime)
- **Selected row:** Bright highlight with accent border

### 3.4 Tab Bar Polish (`components/tabbar.go`)

Current implementation is solid. Minor enhancements:
- Active tab: filled background block (`▐Dashboard▌`) instead of brackets
- Inactive tabs: dim text, no brackets
- Separator line: thin gradient or accent-colored at active tab position
- Add notification badges (e.g., red dot on Logs tab if errors)

### 3.5 Status Bar Enhancement (`components/statusbar.go`)

- **Left:** Breadcrumbs (yokai > Dashboard > finn) — already implemented
- **Right:** Key bindings — already implemented
- **Add center:** Clock/uptime counter, total GPU utilization summary
- **Accent line:** Top separator with gradient (accent color fading to border color)

---

## Phase 4: View-Level Improvements

### 4.1 Service Detail View (`components/servicedetail.go`)

When pressing Enter on a service, show expanded detail:
- Container info (image, ID, ports, env vars)
- CPU history chart (ntcharts streamline, full width)
- Memory usage chart
- GPU memory chart
- Quick actions: Stop, Restart, Logs, Remove
- Vim-style: Esc or q to go back

### 4.2 Log Viewer Improvements (`views/logs.go`)

- **JSON syntax highlighting:** Color-code keys, values, brackets in JSON log lines
- **Search:** `/` to search, `n`/`N` to navigate matches, highlighted matches
- **Line wrapping toggle:** `w` to toggle wrap
- **Timestamp column:** Dimmed styling, left-aligned
- **Auto-scroll toggle:** `f` to follow (tail mode), manual scroll pauses

### 4.3 Deploy Wizard Polish (`views/deploy.go`)

- **Step indicator:** `Step 2/5 ━━━━━━●━━━━━━━━` with filled/empty segments
- **Animated spinners** during HuggingFace search, Docker image fetch
- **Preview panel:** Show the docker command that will be generated before confirming

---

## Phase 5: Animations & Micro-interactions

### 5.1 Loading States
- Use `bubbles/spinner` component for all async operations
- Deploy: progress bar with percentage during pull
- Connection test: spinner with "Testing SSH..." text
- First data load: skeleton shimmer effect (alternating dim blocks)

### 5.2 Status Indicators
- Online/offline dots: pulsing animation for "connecting" state
- Service health: slow pulse for "starting", solid for running
- Use BubbleTea tick commands for animation frames

### 5.3 Toast Notifications (`components/toast.go`)
- Already exists — enhance with:
  - Slide-in from right animation
  - Color-coded borders: green (success), yellow (warning), red (error)
  - Auto-dismiss after 3 seconds
  - Stack multiple toasts vertically

### 5.4 Panel Focus
- **Focused panel:** Bright border (accent color `#7aa2f7`), double-line border
- **Unfocused panel:** Dim border (`#414868`), single-line border
- Tab/Shift+Tab cycles focus between panels

---

## Implementation Order

1. **Dashboard grid layout** — biggest visual impact, restructure View()
2. **Chart upgrades** — ntcharts streamline with color gradients
3. **Progress bar standardization** — value-based gradient bars everywhere
4. **Device card compaction** — denser info display
5. **Service table polish** — alternating rows, color status, sort
6. **Tab bar & status bar refinement**
7. **Service detail view** — expanded view with charts
8. **Log viewer improvements** — search, highlighting
9. **Deploy wizard polish** — step indicator, spinners
10. **Animations & focus** — micro-interactions last (polish layer)

---

## Files to Modify

### Core Layout
- `internal/tui/views/dashboard.go` — Major rewrite of View() for grid layout

### Components (modify existing)
- `internal/tui/components/streamchart.go` — Color gradients, filled area, Y-axis labels
- `internal/tui/components/gpupanel.go` — Chart-based instead of text bars, temp sparkline
- `internal/tui/components/devicecard.go` — Compact density, side-by-side bars
- `internal/tui/components/servicelist.go` — Alternating rows, color status, sort, trend arrows
- `internal/tui/components/tabbar.go` — Filled active tab, notification badges
- `internal/tui/components/statusbar.go` — Center section, gradient accent line
- `internal/tui/components/metricsbar.go` — Value-based gradient within bar
- `internal/tui/components/servicedetail.go` — Charts in detail view
- `internal/tui/components/toast.go` — Color borders, animation

### Views (modify existing)
- `internal/tui/views/logs.go` — Search, JSON highlighting, wrap toggle
- `internal/tui/views/deploy.go` — Step indicator, spinners, preview

### Theme
- `internal/tui/theme/theme.go` — Add gradient helpers, focus/unfocus styles, new color utilities

### New Files (if needed)
- `internal/tui/components/tempsparkline.go` — Compact inline temperature sparkline
- `internal/tui/theme/gradient.go` — Color gradient interpolation utilities

---

## Color Reference (Tokyo Night)

| Usage | Color | Hex |
|-------|-------|-----|
| Background | Dark navy | `#1a1b26` |
| Border (unfocused) | Gray blue | `#414868` |
| Border (focused) | Bright blue | `#7aa2f7` |
| Accent | Blue | `#7aa2f7` |
| Good/Low | Green | `#9ece6a` |
| Warning/Mid | Amber | `#e0af68` |
| Critical/High | Pink-red | `#f7768e` |
| Text primary | Light blue-white | `#c0caf5` |
| Text muted | Gray | `#565f89` |
| Row highlight | Dark blue | `#283457` |
| Success | Teal | `#73daca` |

## Gradient Thresholds (for all metrics)
- **0-50%:** Green (`#9ece6a`)
- **50-80%:** Yellow/Amber (`#e0af68`)
- **80-100%:** Red/Pink (`#f7768e`)

---

## Testing

- Run `go build ./...` after each phase to ensure compilation
- Test with: `cd /home/dell/clawd/Yokai && go run ./cmd/yokai`
- Verify responsive layout at different terminal widths (80, 120, 160, 200 cols)
- Ensure graceful fallback when daemon is offline (error banner)
- Run existing tests: `go test ./internal/tui/...`
