package components

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
	zone "github.com/lrstanley/bubblezone"
	"github.com/spencerbull/yokai/internal/tui/theme"
)

// ServiceRow represents a single service in the list.
type ServiceRow struct {
	Name       string
	Type       string // vllm, llamacpp, comfyui
	Model      string
	Status     string // running, stopped, error
	Health     string // healthy, unhealthy, starting, ""
	Device     string // device label
	Port       int
	CPUPercent float64
	MemUsedMB  int64
	GPUMemMB   int64
	Uptime     string
	Selected   bool
}

// ServiceList renders a table of services.
type ServiceList struct {
	Services []ServiceRow
	Cursor   int
	Width    int
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
}

var columns = []column{
	{header: "", minWidth: 2, flex: 0}, // health indicator
	{header: "Service", minWidth: 12, flex: 3},
	{header: "Model", minWidth: 10, flex: 4},
	{header: "Device", minWidth: 8, flex: 1},
	{header: "Port", minWidth: 5, flex: 0},
	{header: "GPU Mem", minWidth: 7, flex: 0},
	{header: "Uptime", minWidth: 6, flex: 0},
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

	widths := s.columnWidths()
	var lines []string

	// Header
	lines = append(lines, s.renderHeader(widths))

	// Thin separator
	sep := theme.MutedStyle.Render(strings.Repeat("─", s.Width))
	lines = append(lines, sep)

	// Rows
	for i, svc := range s.Services {
		svc.Selected = (i == s.Cursor)
		lines = append(lines, s.renderRow(svc, widths, i))
	}

	return strings.Join(lines, "\n")
}

func (s ServiceList) columnWidths() []int {
	widths := make([]int, len(columns))
	totalMin := 0
	totalFlex := 0
	for i, col := range columns {
		widths[i] = col.minWidth
		totalMin += col.minWidth
		totalFlex += col.flex
	}
	totalMin += len(columns) - 1 // column gaps

	extra := s.Width - totalMin
	if extra > 0 && totalFlex > 0 {
		for i, col := range columns {
			if col.flex > 0 {
				widths[i] += extra * col.flex / totalFlex
			}
		}
	}
	return widths
}

func (s ServiceList) renderHeader(widths []int) string {
	headerStyle := lipgloss.NewStyle().
		Foreground(theme.TextMuted).
		Bold(true).
		Underline(true)

	var parts []string
	for i, col := range columns {
		parts = append(parts, pad(col.header, widths[i]))
	}
	return headerStyle.Render(strings.Join(parts, " "))
}

func (s ServiceList) renderRow(svc ServiceRow, widths []int, rowIdx int) string {
	// Health indicator — use a fixed-width cell so alignment doesn't break
	healthIcon := s.healthIndicator(svc)

	// Short model: strip org prefix for display (e.g. "Qwen/Qwen3.5-35B" -> "Qwen3.5-35B")
	model := svc.Model
	if idx := strings.LastIndex(model, "/"); idx >= 0 && idx < len(model)-1 {
		model = model[idx+1:]
	}

	portStr := ""
	if svc.Port > 0 {
		portStr = fmt.Sprintf("%d", svc.Port)
	}

	gpuStr := formatMem(svc.GPUMemMB)

	// Status text with color
	statusStr := s.statusText(svc)

	cells := []string{
		healthIcon,
		truncate(svc.Name, widths[1]),
		truncate(model, widths[2]),
		truncate(svc.Device, widths[3]),
		pad(portStr, widths[4]),
		pad(gpuStr, widths[5]),
		pad(statusStr, widths[6]),
	}

	// Pad each cell (except health icon which is special)
	var parts []string
	for i, cell := range cells {
		if i == 0 {
			// Health icon: pad the plain-text width to the column width
			parts = append(parts, cell+strings.Repeat(" ", max(0, widths[0]-1)))
		} else {
			parts = append(parts, pad(cell, widths[i]))
		}
	}

	row := strings.Join(parts, " ")

	zoneID := ServiceRowZoneID(rowIdx)

	if svc.Selected {
		return zone.Mark(zoneID, lipgloss.NewStyle().
			Background(theme.Highlight).
			Foreground(theme.TextPrimary).
			Width(s.Width).
			Render(row))
	}

	// Alternating row backgrounds for readability
	if rowIdx%2 == 1 {
		return zone.Mark(zoneID, lipgloss.NewStyle().
			Background(lipgloss.Color("#1e1f2b")).
			Foreground(theme.TextPrimary).
			Width(s.Width).
			Render(row))
	}
	return zone.Mark(zoneID, theme.PrimaryStyle.Render(row))
}

// statusText returns a short, color-coded status string.
func (s ServiceList) statusText(svc ServiceRow) string {
	h := svc.Health
	if h == "" {
		h = svc.Status
	}
	switch h {
	case "healthy", "running":
		return svc.Uptime
	case "starting":
		return "starting"
	case "unhealthy", "error":
		return "error"
	case "stopped":
		return "stopped"
	default:
		return svc.Uptime
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
	case "unhealthy", "error":
		return lipgloss.NewStyle().Foreground(theme.Crit).Render("●")
	case "stopped":
		return lipgloss.NewStyle().Foreground(theme.TextMuted).Render("○")
	default:
		return lipgloss.NewStyle().Foreground(theme.TextMuted).Render("?")
	}
}

// ServiceRowZoneID returns the zone ID for a service row at the given index.
// Used by both the service list renderer and the dashboard mouse handler.
func ServiceRowZoneID(rowIdx int) string {
	return fmt.Sprintf("svc-row-%d", rowIdx)
}

// helpers

func pad(s string, width int) string {
	if len(s) >= width {
		return s[:width]
	}
	return s + strings.Repeat(" ", width-len(s))
}

func truncate(s string, width int) string {
	if width <= 0 {
		return ""
	}
	if len(s) <= width {
		return s
	}
	if width <= 3 {
		return s[:width]
	}
	return s[:width-1] + "…"
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
