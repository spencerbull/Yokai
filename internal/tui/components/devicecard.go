package components

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/spencerbull/yokai/internal/tui/theme"
)

// DeviceCard renders a device summary card.
type DeviceCard struct {
	Label        string
	Host         string
	Online       bool
	GPUType      string
	GPUCount     int
	CPUPercent   float64
	RAMPercent   float64
	ServiceCount int
	Width        int
}

// NewDeviceCard creates a new device card with the given parameters.
func NewDeviceCard(label, host string, online bool, gpuType string, gpuCount int, cpuPercent, ramPercent float64, serviceCount, width int) DeviceCard {
	return DeviceCard{
		Label:        label,
		Host:         host,
		Online:       online,
		GPUType:      gpuType,
		GPUCount:     gpuCount,
		CPUPercent:   cpuPercent,
		RAMPercent:   ramPercent,
		ServiceCount: serviceCount,
		Width:        width,
	}
}

// Render returns the rendered string representation of the device card.
func (d DeviceCard) Render() string {
	if d.Width <= 0 {
		return ""
	}
	if d.Width < 20 {
		return strings.Repeat(" ", d.Width)
	}

	// Calculate inner content width (subtract 4 for borders and padding)
	contentWidth := d.Width - 4
	if contentWidth < 10 {
		contentWidth = 10
	}

	// Title line: "finn  ● online" (label left, status right-aligned)
	statusDot := ""
	statusText := ""
	if d.Online {
		statusDot = theme.StatusOnline()
		statusText = "online"
	} else {
		statusDot = theme.StatusOffline()
		statusText = "offline"
	}

	titlePart := d.Label
	statusPart := fmt.Sprintf("%s %s", statusDot, statusText)

	// Right-align status within content width
	titleLen := lipgloss.Width(titlePart)
	statusLen := lipgloss.Width(statusPart)
	gap := contentWidth - titleLen - statusLen
	if gap < 1 {
		gap = 1
	}

	title := titlePart + strings.Repeat(" ", gap) + statusPart

	// Truncate title if still too long
	if lipgloss.Width(title) > contentWidth {
		maxTitleLen := contentWidth - statusLen - 2
		if maxTitleLen > 3 {
			titlePart = titlePart[:min(len(titlePart), maxTitleLen-3)] + "..."
			gap = contentWidth - lipgloss.Width(titlePart) - statusLen
			if gap < 1 {
				gap = 1
			}
			title = titlePart + strings.Repeat(" ", gap) + statusPart
		}
	}

	// GPU line: show actual name like "RTX Pro 6000", fallback to type
	gpuLine := ""
	if d.GPUCount > 0 && d.GPUType != "" {
		gpuDisplay := d.GPUType
		// Strip common prefixes for cleaner display
		gpuDisplay = strings.TrimPrefix(gpuDisplay, "NVIDIA ")
		if d.GPUCount == 1 {
			gpuLine = fmt.Sprintf("GPU  %s", gpuDisplay)
		} else {
			gpuLine = fmt.Sprintf("GPU  %dx %s", d.GPUCount, gpuDisplay)
		}
	} else if d.GPUCount > 0 {
		gpuLine = fmt.Sprintf("GPU  %d device(s)", d.GPUCount)
	} else {
		gpuLine = "GPU  none"
	}

	// Metrics line: "CPU [████░░░░░░] 32%  RAM [██████░░] 67%"
	// Calculate bar widths
	cpuLabel := "CPU"
	ramLabel := "RAM"
	cpuPercentStr := fmt.Sprintf("%.0f%%", d.CPUPercent)
	ramPercentStr := fmt.Sprintf("%.0f%%", d.RAMPercent)

	// Space for: "CPU [...] XX%  RAM [...] XX%"
	fixedWidth := len(cpuLabel) + 1 + len(cpuPercentStr) + 2 + len(ramLabel) + 1 + len(ramPercentStr) + 4 // spaces and brackets
	barSpaceTotal := contentWidth - fixedWidth
	if barSpaceTotal < 4 {
		barSpaceTotal = 4
	}

	cpuBarWidth := barSpaceTotal / 2
	ramBarWidth := barSpaceTotal - cpuBarWidth

	cpuBar := theme.ProgressBar(d.CPUPercent, cpuBarWidth+2) // +2 for brackets
	ramBar := theme.ProgressBar(d.RAMPercent, ramBarWidth+2)

	metricsLine := fmt.Sprintf("%s %s %s  %s %s %s",
		cpuLabel, cpuBar, cpuPercentStr,
		ramLabel, ramBar, ramPercentStr)

	// Services line
	var servicesLine string
	switch d.ServiceCount {
	case 0:
		servicesLine = "Svc  none"
	case 1:
		servicesLine = "Svc  1 running"
	default:
		servicesLine = fmt.Sprintf("Svc  %d running", d.ServiceCount)
	}

	// Truncate and pad lines to content width
	gpuLine = d.truncateAndPad(gpuLine, contentWidth)
	metricsLine = d.truncateAndPad(metricsLine, contentWidth)
	servicesLine = d.truncateAndPad(servicesLine, contentWidth)

	// Create panel content
	content := gpuLine + "\n" + metricsLine + "\n" + servicesLine

	// Apply panel styling with title
	panel := theme.Panel(title).Width(d.Width).Render(content)

	return panel
}

// truncateAndPad pads a line to the specified visual width.
// It uses lipgloss.Width to correctly handle strings containing
// ANSI escape codes (styled text) and multi-byte Unicode characters.
func (d DeviceCard) truncateAndPad(line string, width int) string {
	visualWidth := lipgloss.Width(line)
	if visualWidth < width {
		line = line + strings.Repeat(" ", width-visualWidth)
	}
	return line
}
