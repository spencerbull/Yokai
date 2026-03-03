package components

import (
	"fmt"
	"strings"

	"github.com/spencerbull/yokai/internal/tui/theme"
)

// ServiceRow represents a single service in the list.
type ServiceRow struct {
	Name       string
	Type       string // vllm, llamacpp, comfyui
	Model      string
	Status     string // running, stopped, error
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

// Render returns the rendered string representation of the service list.
func (s ServiceList) Render() string {
	if len(s.Services) == 0 {
		emptyMsg := "No services running"
		padding := (s.Width - len(emptyMsg)) / 2
		if padding < 0 {
			padding = 0
		}
		return strings.Repeat(" ", padding) + theme.MutedStyle.Render(emptyMsg)
	}

	var lines []string

	// Header
	header := s.renderHeader()
	lines = append(lines, header)

	// Separator
	separator := strings.Repeat("─", s.Width)
	lines = append(lines, theme.MutedStyle.Render(separator))

	// Service rows
	for i, service := range s.Services {
		service.Selected = (i == s.Cursor)
		row := s.renderServiceRow(service)
		lines = append(lines, row)
	}

	return strings.Join(lines, "\n")
}

// renderHeader renders the table header.
func (s ServiceList) renderHeader() string {
	// Column headers: Status | Name | Type | Model | Port | CPU% | Mem | GPU Mem | Uptime
	headers := []string{"●", "Name", "Type", "Model", "Port", "CPU%", "Mem", "GPU Mem", "Uptime"}
	widths := s.calculateColumnWidths()

	var parts []string
	for i, header := range headers {
		if i < len(widths) {
			part := s.padColumn(header, widths[i])
			parts = append(parts, part)
		}
	}

	headerLine := strings.Join(parts, " ")
	return theme.TitleStyle.Render(headerLine)
}

// renderServiceRow renders a single service row.
func (s ServiceList) renderServiceRow(service ServiceRow) string {
	// Status dot
	statusDot := ""
	switch service.Status {
	case "running":
		statusDot = theme.StatusOnline()
	case "stopped":
		statusDot = theme.StatusOffline()
	case "error":
		statusDot = theme.CritStyle.Render("●")
	default:
		statusDot = theme.StatusLoading()
	}

	// Format values
	portStr := fmt.Sprintf("%d", service.Port)
	cpuStr := fmt.Sprintf("%.1f%%", service.CPUPercent)
	memStr := s.formatMemory(service.MemUsedMB)
	gpuMemStr := s.formatMemory(service.GPUMemMB)

	values := []string{
		statusDot,
		service.Name,
		service.Type,
		service.Model,
		portStr,
		cpuStr,
		memStr,
		gpuMemStr,
		service.Uptime,
	}

	widths := s.calculateColumnWidths()
	var parts []string
	for i, value := range values {
		if i < len(widths) {
			part := s.padColumn(value, widths[i])
			parts = append(parts, part)
		}
	}

	rowLine := strings.Join(parts, " ")

	// Apply selection highlighting if this row is selected
	if service.Selected {
		return theme.HighlightStyle.Render(rowLine)
	}
	return theme.PrimaryStyle.Render(rowLine)
}

// calculateColumnWidths calculates the width for each column based on available space.
func (s ServiceList) calculateColumnWidths() []int {
	// Minimum widths for each column
	minWidths := []int{1, 8, 6, 10, 4, 5, 6, 7, 6} // Status, Name, Type, Model, Port, CPU%, Mem, GPU Mem, Uptime

	// Calculate total minimum width needed
	totalMin := 0
	for _, w := range minWidths {
		totalMin += w
	}
	totalMin += len(minWidths) - 1 // spaces between columns

	if totalMin >= s.Width {
		// Not enough space, use minimum widths
		return minWidths
	}

	// Distribute extra space proportionally to name and model columns (indices 1 and 3)
	extraSpace := s.Width - totalMin
	widths := make([]int, len(minWidths))
	copy(widths, minWidths)

	// Give most extra space to Name and Model columns
	nameExtra := extraSpace * 40 / 100
	modelExtra := extraSpace * 40 / 100
	remaining := extraSpace - nameExtra - modelExtra

	widths[1] += nameExtra  // Name
	widths[3] += modelExtra // Model
	widths[8] += remaining  // Uptime gets the rest

	return widths
}

// padColumn pads or truncates a column value to the specified width.
func (s ServiceList) padColumn(value string, width int) string {
	if len(value) > width {
		if width > 3 {
			return value[:width-3] + "..."
		}
		return value[:width]
	}
	return value + strings.Repeat(" ", width-len(value))
}

// formatMemory formats memory values in a human-readable format.
func (s ServiceList) formatMemory(memMB int64) string {
	if memMB < 1024 {
		return fmt.Sprintf("%dM", memMB)
	}
	return fmt.Sprintf("%.1fG", float64(memMB)/1024)
}
