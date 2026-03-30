package components

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	zone "github.com/lrstanley/bubblezone"
	"github.com/spencerbull/yokai/internal/tui/theme"
)

// ServiceRow represents a single service in the list.
type ServiceRow struct {
	Name                string
	Type                string // vllm, llamacpp, comfyui
	Model               string
	Status              string // running, stopped, error
	Health              string // healthy, unhealthy, starting, ""
	Device              string // device label
	Port                int
	CPUPercent          float64
	MemUsedMB           int64
	GPUMemMB            int64
	Uptime              string
	Selected            bool
	GenerationTokPerSec float64
	PromptTokPerSec     float64
}

// ServiceList renders a table of services.
type ServiceList struct {
	Services          []ServiceRow
	Cursor            int
	Width             int
	ForceDeviceColumn bool
	ZonePrefix        string
	MarqueeOffset     int
	MarqueeGap        int
}

// NewServiceList creates a new service list with the given parameters.
func NewServiceList(services []ServiceRow, width int) ServiceList {
	return ServiceList{
		Services: services,
		Cursor:   0,
		Width:    width,
	}
}

// column definitions
type column struct {
	header   string
	minWidth int
	flex     int // relative share of extra space (0 = fixed)
	id       string
}

// allColumns defines the superset of columns; some may be hidden dynamically.
var allColumns = []column{
	{id: "health", header: "", minWidth: 2, flex: 0},
	{id: "service", header: "Service", minWidth: 12, flex: 3},
	{id: "model", header: "Model", minWidth: 10, flex: 4},
	{id: "device", header: "Device", minWidth: 8, flex: 1},
	{id: "port", header: "Port", minWidth: 5, flex: 0},
	{id: "gpumem", header: "GPU Mem", minWidth: 7, flex: 0},
	{id: "toks", header: "Tok/s", minWidth: 6, flex: 0},
	{id: "prefill", header: "Prefill", minWidth: 7, flex: 0},
	{id: "uptime", header: "Uptime", minWidth: 6, flex: 0},
}

// activeColumns returns columns that have data, hiding empty ones.
func (s ServiceList) activeColumns() []column {
	hasModel := false
	hasGPUMem := false
	hasToks := false
	hasPrefill := false
	hasMultipleDevices := false
	deviceName := ""
	for _, svc := range s.Services {
		if svc.Model != "" {
			hasModel = true
		}
		if svc.GPUMemMB > 0 {
			hasGPUMem = true
		}
		if svc.GenerationTokPerSec > 0 {
			hasToks = true
		}
		if svc.PromptTokPerSec > 0 {
			hasPrefill = true
		}
		if svc.Device != "" {
			if deviceName == "" {
				deviceName = svc.Device
			} else if svc.Device != deviceName {
				hasMultipleDevices = true
			}
		}
	}

	var cols []column
	for _, col := range allColumns {
		if col.id == "device" && !s.ForceDeviceColumn && !hasMultipleDevices {
			continue
		}
		if col.id == "model" && !hasModel {
			continue
		}
		if col.id == "gpumem" && !hasGPUMem {
			continue
		}
		if col.id == "toks" && !hasToks {
			continue
		}
		if col.id == "prefill" && !hasPrefill {
			continue
		}
		cols = append(cols, col)
	}
	return cols
}

// Render returns the rendered string representation of the service list.
func (s ServiceList) Render() string {
	if s.Width < 20 {
		s.Width = 20
	}

	if len(s.Services) == 0 {
		emptyMsg := "No services running. Press 'n' to deploy."
		return theme.MutedStyle.Render(emptyMsg)
	}

	cols := s.activeColumns()
	widths := s.columnWidths(cols)
	var lines []string

	// Header
	lines = append(lines, s.renderHeader(widths, cols))

	// Thin separator
	sep := theme.MutedStyle.Render(strings.Repeat("─", s.Width))
	lines = append(lines, sep)

	// Rows
	for i, svc := range s.Services {
		svc.Selected = (i == s.Cursor)
		lines = append(lines, s.renderRow(svc, widths, cols, i))
	}

	return strings.Join(lines, "\n")
}

func (s ServiceList) columnWidths(cols []column) []int {
	widths := make([]int, len(cols))
	totalMin := 0
	totalFlex := 0
	for i, col := range cols {
		widths[i] = col.minWidth
		totalMin += col.minWidth
		totalFlex += col.flex
	}
	totalMin += len(cols) - 1 // column gaps

	extra := s.Width - totalMin
	if extra > 0 && totalFlex > 0 {
		for i, col := range cols {
			if col.flex > 0 {
				widths[i] += extra * col.flex / totalFlex
			}
		}
	}
	return widths
}

func (s ServiceList) renderHeader(widths []int, cols []column) string {
	headerStyle := lipgloss.NewStyle().
		Foreground(theme.TextMuted).
		Bold(true).
		Underline(true)

	var parts []string
	for i, col := range cols {
		parts = append(parts, pad(col.header, widths[i]))
	}
	return headerStyle.Render(strings.Join(parts, " "))
}

// cellValue returns the value for a given column id.
func (s ServiceList) cellValue(svc ServiceRow, col column, width int) string {
	switch col.id {
	case "health":
		return s.healthIndicator(svc)
	case "service":
		if svc.Selected {
			return marquee(svc.Name, width, s.MarqueeOffset, s.marqueeGap())
		}
		return truncate(svc.Name, width)
	case "model":
		model := svc.Model
		if idx := strings.LastIndex(model, "/"); idx >= 0 && idx < len(model)-1 {
			model = model[idx+1:]
		}
		return truncate(model, width)
	case "device":
		return truncate(svc.Device, width)
	case "port":
		if svc.Port > 0 {
			return fmt.Sprintf("%d", svc.Port)
		}
		return ""
	case "gpumem":
		return formatMem(svc.GPUMemMB)
	case "toks":
		if svc.GenerationTokPerSec > 0 {
			arrow := lipgloss.NewStyle().Foreground(theme.Good).Render("↑")
			return fmt.Sprintf("%.1f%s", svc.GenerationTokPerSec, arrow)
		}
		return lipgloss.NewStyle().Foreground(theme.TextMuted).Render("-")
	case "prefill":
		if svc.PromptTokPerSec > 0 {
			return fmt.Sprintf("%.1f", svc.PromptTokPerSec)
		}
		return lipgloss.NewStyle().Foreground(theme.TextMuted).Render("-")
	case "uptime":
		return s.statusText(svc)
	default:
		return ""
	}
}

func (s ServiceList) renderRow(svc ServiceRow, widths []int, cols []column, rowIdx int) string {
	var parts []string
	for i, col := range cols {
		cell := s.cellValue(svc, col, widths[i])
		if col.id == "health" {
			parts = append(parts, cell+strings.Repeat(" ", max(0, widths[i]-ansi.StringWidth(cell))))
		} else {
			parts = append(parts, pad(cell, widths[i]))
		}
	}

	row := strings.Join(parts, " ")

	zoneID := ServiceRowZoneIDWithPrefix(s.ZonePrefix, rowIdx)

	if svc.Selected {
		bar := lipgloss.NewStyle().
			Foreground(theme.Accent).
			Bold(true).
			Render("▌")
		content := lipgloss.NewStyle().
			Background(theme.Highlight).
			Foreground(theme.TextPrimary).
			Bold(true).
			Width(max(0, s.Width-1)).
			Render(row)
		return zone.Mark(zoneID, bar+content)
	}

	// Alternating row backgrounds for readability
	if rowIdx%2 == 1 {
		return zone.Mark(zoneID, lipgloss.NewStyle().
			Background(lipgloss.Color("#1e2030")).
			Foreground(theme.TextPrimary).
			Width(s.Width).
			Render(row))
	}
	return zone.Mark(zoneID, lipgloss.NewStyle().
		Foreground(theme.TextPrimary).
		Width(s.Width).
		Render(row))
}

// statusText returns a short, color-coded status string.
func (s ServiceList) statusText(svc ServiceRow) string {
	h := svc.Health
	if h == "" {
		h = svc.Status
	}
	switch h {
	case "healthy", "running":
		return lipgloss.NewStyle().Foreground(theme.Good).Render(svc.Uptime)
	case "starting":
		return lipgloss.NewStyle().Foreground(theme.Warn).Render("starting")
	case "created":
		return lipgloss.NewStyle().Foreground(theme.Warn).Render("created")
	case "restarting":
		return lipgloss.NewStyle().Foreground(theme.Warn).Render("restarting")
	case "unhealthy", "error":
		return lipgloss.NewStyle().Foreground(theme.Crit).Render("error")
	case "stopped", "dead", "exited":
		return lipgloss.NewStyle().Foreground(theme.TextMuted).Render("stopped")
	default:
		return lipgloss.NewStyle().Foreground(theme.TextMuted).Render(svc.Uptime)
	}
}

func (s ServiceList) healthIndicator(svc ServiceRow) string {
	// Prefer explicit health, fall back to status
	h := svc.Health
	if h == "" {
		h = svc.Status
	}

	switch h {
	case "healthy", "running":
		return lipgloss.NewStyle().Foreground(theme.Good).Render("●")
	case "starting":
		return lipgloss.NewStyle().Foreground(theme.Warn).Render("◐")
	case "created", "restarting":
		return lipgloss.NewStyle().Foreground(theme.Warn).Render("◌")
	case "unhealthy", "error":
		return lipgloss.NewStyle().Foreground(theme.Crit).Render("●")
	case "stopped", "dead", "exited":
		return lipgloss.NewStyle().Foreground(theme.TextMuted).Render("○")
	default:
		return lipgloss.NewStyle().Foreground(theme.Warn).Render("!")
	}
}

// ServiceRowZoneID returns the zone ID for a service row at the given index.
// Used by both the service list renderer and the dashboard mouse handler.
func ServiceRowZoneID(rowIdx int) string {
	return ServiceRowZoneIDWithPrefix("", rowIdx)
}

func ServiceRowZoneIDWithPrefix(prefix string, rowIdx int) string {
	if prefix == "" {
		return fmt.Sprintf("svc-row-%d", rowIdx)
	}
	return fmt.Sprintf("%s-svc-row-%d", prefix, rowIdx)
}

func (s ServiceList) marqueeGap() int {
	if s.MarqueeGap > 0 {
		return s.MarqueeGap
	}
	return 3
}

// helpers

func pad(s string, width int) string {
	if width <= 0 {
		return ""
	}
	s = ansi.Truncate(s, width, "")
	if sw := ansi.StringWidth(s); sw < width {
		return s + strings.Repeat(" ", width-sw)
	}
	return s
}

func truncate(s string, width int) string {
	if width <= 0 {
		return ""
	}
	if ansi.StringWidth(s) <= width {
		return s
	}
	if width == 1 {
		return ansi.Truncate(s, width, "")
	}
	return ansi.Truncate(s, width, "…")
}

func marquee(s string, width, offset, gap int) string {
	if width <= 0 {
		return ""
	}
	if ansi.StringWidth(s) <= width {
		return pad(s, width)
	}
	if gap < 1 {
		gap = 1
	}
	cycle := s + strings.Repeat(" ", gap) + s
	cycleWidth := ansi.StringWidth(s) + gap
	if cycleWidth <= 0 {
		return pad(s, width)
	}
	start := offset % cycleWidth
	window := ansi.Cut(cycle, start, start+width)
	return pad(window, width)
}

func formatMem(mb int64) string {
	if mb <= 0 {
		return "-"
	}
	if mb < 1024 {
		return fmt.Sprintf("%dM", mb)
	}
	return fmt.Sprintf("%.1fG", float64(mb)/1024)
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
