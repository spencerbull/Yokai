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

// GPUMetrics holds GPU data for inline display in a device card.
type GPUMetrics struct {
	Name        string
	UtilPercent int
	VRAMUsedMB  int64
	VRAMTotalMB int64
	TempC       int
	PowerDrawW  float64
	PowerLimitW float64
	FanPercent  int
}

// DeviceCardFull renders a device card with GPU metrics inline.
type DeviceCardFull struct {
	Label        string
	Host         string
	Online       bool
	CPUPercent   float64
	RAMPercent   float64
	ServiceCount int
	GPUs         []GPUMetrics
	Width        int
}

// NewDeviceCardFull creates a device card with inline GPU metrics.
func NewDeviceCardFull(label, host string, online bool, cpuPercent, ramPercent float64, serviceCount int, gpus []GPUMetrics, width int) DeviceCardFull {
	return DeviceCardFull{
		Label:        label,
		Host:         host,
		Online:       online,
		CPUPercent:   cpuPercent,
		RAMPercent:   ramPercent,
		ServiceCount: serviceCount,
		GPUs:         gpus,
		Width:        width,
	}
}

// Render returns the rendered string for the full device card.
func (d DeviceCardFull) Render() string {
	if d.Width <= 0 {
		return ""
	}
	if d.Width < 20 {
		return strings.Repeat(" ", d.Width)
	}

	contentWidth := d.Width - 4
	if contentWidth < 10 {
		contentWidth = 10
	}

	// Line 1: "<label> (<host>)" right-aligned "● online"
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

	titleLen := lipgloss.Width(titlePart)
	statusLen := lipgloss.Width(statusPart)
	gap := contentWidth - titleLen - statusLen
	if gap < 1 {
		gap = 1
	}
	title := titlePart + strings.Repeat(" ", gap) + statusPart

	var lines []string

	if len(d.GPUs) > 0 {
		gpu := d.GPUs[0]
		gpuName := strings.TrimPrefix(gpu.Name, "NVIDIA ")

		// Line 2: "GPU: <name>   CPU [bar] X%   RAM [bar] X%"
		gpuPrefix := fmt.Sprintf("GPU: %s", gpuName)
		cpuSuffix := fmt.Sprintf("%.0f%%", d.CPUPercent)
		ramSuffix := fmt.Sprintf("%.0f%%", d.RAMPercent)

		// Compute bar widths from remaining space
		fixedLen := len(gpuPrefix) + 3 + len("CPU ") + 1 + len(cpuSuffix) + 3 + len("RAM ") + 1 + len(ramSuffix)
		barSpace := contentWidth - fixedLen
		if barSpace < 4 {
			barSpace = 4
		}
		cpuBarW := barSpace / 2
		ramBarW := barSpace - cpuBarW
		cpuBar := theme.ProgressBar(d.CPUPercent, cpuBarW+2)
		ramBar := theme.ProgressBar(d.RAMPercent, ramBarW+2)

		line2 := fmt.Sprintf("%s   CPU %s %s   RAM %s %s", gpuPrefix, cpuBar, cpuSuffix, ramBar, ramSuffix)
		lines = append(lines, d.padLine(line2, contentWidth))

		// Line 3: "Services: N   Util [bar] X%   VRAM [bar] X/YG"
		svcPart := fmt.Sprintf("Services: %d", d.ServiceCount)
		utilSuffix := fmt.Sprintf("%d%%", gpu.UtilPercent)
		vramUsedGB := float64(gpu.VRAMUsedMB) / 1024
		vramTotalGB := float64(gpu.VRAMTotalMB) / 1024
		vramSuffix := fmt.Sprintf("%.0f/%.0fG", vramUsedGB, vramTotalGB)

		fixedLen2 := len(svcPart) + 3 + len("Util ") + 1 + len(utilSuffix) + 3 + len("VRAM ") + 1 + len(vramSuffix)
		barSpace2 := contentWidth - fixedLen2
		if barSpace2 < 4 {
			barSpace2 = 4
		}
		utilBarW := barSpace2 / 2
		vramBarW := barSpace2 - utilBarW
		utilBar := GradientProgressBar(float64(gpu.UtilPercent), utilBarW+2, theme.Accent)
		vramPercent := float64(0)
		if gpu.VRAMTotalMB > 0 {
			vramPercent = float64(gpu.VRAMUsedMB) / float64(gpu.VRAMTotalMB) * 100
		}
		vramColor := theme.Good
		if vramPercent > 80 {
			vramColor = theme.Crit
		} else if vramPercent > 50 {
			vramColor = theme.Warn
		}
		vramBar := GradientProgressBar(vramPercent, vramBarW+2, vramColor)

		line3 := fmt.Sprintf("%s   Util %s %s   VRAM %s %s", svcPart, utilBar, utilSuffix, vramBar, vramSuffix)
		lines = append(lines, d.padLine(line3, contentWidth))

		// Line 4: power, temp, fan
		tColor := tempColor(gpu.TempC)
		tempStyle := lipgloss.NewStyle().Foreground(tColor)
		tempStr := tempStyle.Render(fmt.Sprintf("%d°C", gpu.TempC))

		powerPercent := float64(0)
		if gpu.PowerLimitW > 0 {
			powerPercent = gpu.PowerDrawW / gpu.PowerLimitW * 100
		}
		powerColor := theme.Good
		if powerPercent > 80 {
			powerColor = theme.Crit
		} else if powerPercent > 60 {
			powerColor = theme.Warn
		}
		powerStyle := lipgloss.NewStyle().Foreground(powerColor)
		powerStr := powerStyle.Render(fmt.Sprintf("%.0fW/%.0fW", gpu.PowerDrawW, gpu.PowerLimitW))

		line4 := fmt.Sprintf("%s  %s  Fan %d%%", powerStr, tempStr, gpu.FanPercent)
		lines = append(lines, d.padLine(line4, contentWidth))
	} else {
		// No GPU: simpler layout
		// Line 2: "CPU [bar] X%   RAM [bar] X%"
		cpuSuffix := fmt.Sprintf("%.0f%%", d.CPUPercent)
		ramSuffix := fmt.Sprintf("%.0f%%", d.RAMPercent)
		fixedLen := len("CPU ") + 1 + len(cpuSuffix) + 3 + len("RAM ") + 1 + len(ramSuffix)
		barSpace := contentWidth - fixedLen
		if barSpace < 4 {
			barSpace = 4
		}
		cpuBarW := barSpace / 2
		ramBarW := barSpace - cpuBarW
		cpuBar := theme.ProgressBar(d.CPUPercent, cpuBarW+2)
		ramBar := theme.ProgressBar(d.RAMPercent, ramBarW+2)

		line2 := fmt.Sprintf("CPU %s %s   RAM %s %s", cpuBar, cpuSuffix, ramBar, ramSuffix)
		lines = append(lines, d.padLine(line2, contentWidth))

		// Line 3: "Services: N"
		line3 := fmt.Sprintf("Services: %d", d.ServiceCount)
		lines = append(lines, d.padLine(line3, contentWidth))
	}

	content := strings.Join(lines, "\n")
	panel := theme.Panel(title).Width(d.Width).Render(content)
	return panel
}

func (d DeviceCardFull) padLine(line string, width int) string {
	visualWidth := lipgloss.Width(line)
	if visualWidth < width {
		line = line + strings.Repeat(" ", width-visualWidth)
	}
	return line
}
