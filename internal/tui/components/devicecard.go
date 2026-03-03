package components

import (
	"fmt"
	"strings"

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
	if d.Width < 20 {
		return strings.Repeat(" ", d.Width)
	}

	// Calculate inner content width (subtract 4 for borders and padding)
	contentWidth := d.Width - 4
	if contentWidth < 10 {
		contentWidth = 10
	}

	// Title line: "My Server (192.168.1.100) ── ● online ──"
	statusDot := ""
	statusText := ""
	if d.Online {
		statusDot = theme.StatusOnline()
		statusText = "online"
	} else {
		statusDot = theme.StatusOffline()
		statusText = "offline"
	}

	titlePart := fmt.Sprintf("%s (%s)", d.Label, d.Host)
	statusPart := fmt.Sprintf("%s %s", statusDot, statusText)

	// Calculate padding for title line
	titleLen := len(titlePart)
	statusLen := len(statusPart)
	dashesNeeded := contentWidth - titleLen - statusLen - 6 // -6 for spaces and dash separators
	if dashesNeeded < 0 {
		dashesNeeded = 0
	}

	leftDashes := dashesNeeded / 2
	rightDashes := dashesNeeded - leftDashes

	var title string
	if dashesNeeded > 0 {
		title = titlePart + " " + strings.Repeat("─", leftDashes+1) + " " + statusPart + " " + strings.Repeat("─", rightDashes)
	} else {
		// Not enough space for dashes, just use basic format
		title = titlePart + " " + statusPart
	}

	// Truncate title if it's still too long
	if len(title) > contentWidth {
		if contentWidth > 3 {
			title = title[:contentWidth-3] + "..."
		} else {
			title = title[:contentWidth]
		}
	}

	// GPU line: "GPU: 2x NVIDIA RTX 4090"
	gpuLine := ""
	if d.GPUCount > 0 && d.GPUType != "" {
		if d.GPUCount == 1 {
			gpuLine = fmt.Sprintf("GPU: %s", d.GPUType)
		} else {
			gpuLine = fmt.Sprintf("GPU: %dx %s", d.GPUCount, d.GPUType)
		}
	} else if d.GPUCount > 0 {
		if d.GPUCount == 1 {
			gpuLine = "GPU: 1 device"
		} else {
			gpuLine = fmt.Sprintf("GPU: %d devices", d.GPUCount)
		}
	} else {
		gpuLine = "GPU: None"
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

	// Services line: "Services: 3 running"
	var servicesLine string
	switch d.ServiceCount {
	case 0:
		servicesLine = "Services: None"
	case 1:
		servicesLine = "Services: 1 running"
	default:
		servicesLine = fmt.Sprintf("Services: %d running", d.ServiceCount)
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

// truncateAndPad truncates a line if too long and pads to the specified width.
func (d DeviceCard) truncateAndPad(line string, width int) string {
	if len(line) > width {
		if width > 3 {
			line = line[:width-3] + "..."
		} else {
			line = line[:width]
		}
	}
	if len(line) < width {
		line = line + strings.Repeat(" ", width-len(line))
	}
	return line
}
