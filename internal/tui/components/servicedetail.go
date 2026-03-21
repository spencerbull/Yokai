package components

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/spencerbull/yokai/internal/tui/theme"
)

// ServiceDetailData holds all the info needed for the detail panel.
type ServiceDetailData struct {
	Name        string
	Image       string
	Model       string
	ContainerID string
	Status      string
	Health      string
	Port        int
	Device      string
	CPUPercent  float64
	MemUsedMB   int64
	GPUMemMB    int64
	Uptime      string
	CPUHistory  []float64
}

// ServiceDetail renders an expanded detail card for a selected service.
type ServiceDetail struct {
	Data  ServiceDetailData
	Width int
}

// NewServiceDetail creates a new service detail component.
func NewServiceDetail(data ServiceDetailData, width int) ServiceDetail {
	return ServiceDetail{Data: data, Width: width}
}

// Render returns the rendered detail panel.
func (s ServiceDetail) Render() string {
	if s.Width < 30 {
		s.Width = 30
	}

	label := theme.MutedStyle.Render
	value := theme.PrimaryStyle.Render

	// Truncate container ID to 12 chars
	cid := s.Data.ContainerID
	if len(cid) > 12 {
		cid = cid[:12]
	}

	portStr := "-"
	if s.Data.Port > 0 {
		portStr = fmt.Sprintf("%d", s.Data.Port)
	}

	cpuStr := fmt.Sprintf("%.1f%%", s.Data.CPUPercent)
	memStr := formatMem(s.Data.MemUsedMB)
	gpuMemStr := formatMem(s.Data.GPUMemMB)

	// Health indicator dot
	healthDot := s.healthDot()

	// Left column: Name, Image, Model, Container ID
	leftPairs := []struct{ k, v string }{
		{"Name", s.Data.Name},
		{"Image", s.Data.Image},
		{"Model", s.Data.Model},
		{"Container", cid},
	}

	// Right column: Status, Port, CPU, Memory, GPU Mem, Uptime
	statusDisplay := healthDot + " " + s.statusLabel()
	rightPairs := []struct{ k, v string }{
		{"Status", statusDisplay},
		{"Port", portStr},
		{"CPU", cpuStr},
		{"Memory", memStr},
		{"GPU Mem", gpuMemStr},
		{"Uptime", s.Data.Uptime},
	}

	// Calculate column widths
	innerWidth := s.Width - 4 // border + padding
	colWidth := (innerWidth - 3) / 2

	leftLines := renderPairs(leftPairs, colWidth, label, value)
	rightLines := renderPairs(rightPairs, colWidth, label, value)

	// Pad to same height
	for len(leftLines) < len(rightLines) {
		leftLines = append(leftLines, strings.Repeat(" ", colWidth))
	}
	for len(rightLines) < len(leftLines) {
		rightLines = append(rightLines, strings.Repeat(" ", colWidth))
	}

	leftCol := lipgloss.NewStyle().Width(colWidth).Render(strings.Join(leftLines, "\n"))
	rightCol := lipgloss.NewStyle().Width(colWidth).Render(strings.Join(rightLines, "\n"))

	body := lipgloss.JoinHorizontal(lipgloss.Top, leftCol, "   ", rightCol)

	// Mini CPU sparkline if history available
	if len(s.Data.CPUHistory) > 0 {
		sparkWidth := innerWidth
		if sparkWidth > 60 {
			sparkWidth = 60
		}
		cpuLabel := label("CPU ") + value(cpuStr) + " "
		spark := NewSparkline(s.Data.CPUHistory, sparkWidth-len("CPU 00.0% "), theme.Good)
		body += "\n\n" + cpuLabel + spark.Render()
	}

	// Action hints
	hints := theme.MutedStyle.Render("l") + theme.PrimaryStyle.Render(":logs") + "  " +
		theme.MutedStyle.Render("s") + theme.PrimaryStyle.Render(":stop") + "  " +
		theme.MutedStyle.Render("r") + theme.PrimaryStyle.Render(":restart") + "  " +
		theme.MutedStyle.Render("x") + theme.PrimaryStyle.Render(":delete") + "  " +
		theme.MutedStyle.Render("Esc") + theme.PrimaryStyle.Render(":collapse")
	body += "\n\n" + hints

	// Title with service name
	title := theme.TitleStyle.Render(" " + s.Data.Name)
	return theme.RenderPanel(title, body, s.Width)
}

func (s ServiceDetail) healthDot() string {
	h := s.Data.Health
	if h == "" {
		h = s.Data.Status
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

func (s ServiceDetail) statusLabel() string {
	h := s.Data.Health
	if h == "" {
		h = s.Data.Status
	}
	switch h {
	case "healthy":
		return lipgloss.NewStyle().Foreground(theme.Good).Render("healthy")
	case "running":
		return lipgloss.NewStyle().Foreground(theme.Good).Render("running")
	case "starting":
		return lipgloss.NewStyle().Foreground(theme.Warn).Render("starting")
	case "unhealthy":
		return lipgloss.NewStyle().Foreground(theme.Crit).Render("unhealthy")
	case "error":
		return lipgloss.NewStyle().Foreground(theme.Crit).Render("error")
	case "stopped":
		return lipgloss.NewStyle().Foreground(theme.TextMuted).Render("stopped")
	default:
		return theme.PrimaryStyle.Render(h)
	}
}

func renderPairs(pairs []struct{ k, v string }, width int, labelFn, valueFn func(...string) string) []string {
	var lines []string
	for _, p := range pairs {
		k := labelFn(pad(p.k+":", 12))
		v := valueFn(p.v)
		line := k + v
		lines = append(lines, line)
	}
	return lines
}
