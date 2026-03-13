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
	title = d.truncateAndPad(title, contentWidth)
	gpuLine = d.truncateAndPad(gpuLine, contentWidth)
	metricsLine = d.truncateAndPad(metricsLine, contentWidth)
	servicesLine = d.truncateAndPad(servicesLine, contentWidth)

	// Create panel content with title as first line
	content := title + "\n" + gpuLine + "\n" + metricsLine + "\n" + servicesLine

	// Apply panel styling
	panel := theme.Panel("").Width(d.Width).Render(content)

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
	Focused      bool // When true, use accent-colored border
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

	// Line 1: "hostname · ip ● online/offline"
	statusDot := ""
	statusText := ""
	if d.Online {
		statusDot = theme.StatusOnline()
		statusText = lipgloss.NewStyle().Foreground(theme.Good).Render("online")
	} else {
		statusDot = theme.StatusOffline()
		statusText = lipgloss.NewStyle().Foreground(theme.TextMuted).Render("offline")
	}

	titlePart := lipgloss.NewStyle().Foreground(theme.TextPrimary).Bold(true).Render(d.Label)
	if d.Host != "" {
		titlePart += lipgloss.NewStyle().Foreground(theme.TextMuted).Render(" · " + d.Host)
	}
	statusPart := fmt.Sprintf("%s %s", statusDot, statusText)

	titleLen := lipgloss.Width(titlePart)
	statusLen := lipgloss.Width(statusPart)
	gap := contentWidth - titleLen - statusLen
	if gap < 1 {
		gap = 1
	}
	titleLine := titlePart + strings.Repeat(" ", gap) + statusPart

	var lines []string
	lines = append(lines, d.padLine(titleLine, contentWidth))

	if len(d.GPUs) > 0 {
		gpu := d.GPUs[0]
		gpuName := strings.TrimPrefix(gpu.Name, "NVIDIA ")

		// Line 2: GPU name + count
		gpuCountStr := ""
		if len(d.GPUs) > 1 {
			gpuCountStr = fmt.Sprintf("%d× ", len(d.GPUs))
		}
		gpuLine := lipgloss.NewStyle().Foreground(theme.TextMuted).Render("GPU") +
			"  " + lipgloss.NewStyle().Foreground(theme.TextPrimary).Render(gpuCountStr+gpuName)
		lines = append(lines, d.padLine(gpuLine, contentWidth))

		// Line 3: Util bar + VRAM bar side-by-side (compact)
		utilVal := float64(gpu.UtilPercent)
		utilColor := utilThresholdColor(utilVal)
		utilSuffix := lipgloss.NewStyle().Foreground(utilColor).Render(fmt.Sprintf("%d%%", gpu.UtilPercent))

		vramPercent := float64(0)
		if gpu.VRAMTotalMB > 0 {
			vramPercent = float64(gpu.VRAMUsedMB) / float64(gpu.VRAMTotalMB) * 100
		}
		vramColor := utilThresholdColor(vramPercent)
		vramUsedGB := float64(gpu.VRAMUsedMB) / 1024
		vramTotalGB := float64(gpu.VRAMTotalMB) / 1024
		vramSuffix := lipgloss.NewStyle().Foreground(vramColor).Render(fmt.Sprintf("%.1f/%.0fG", vramUsedGB, vramTotalGB))

		// "Util [bar] XX%  VRAM [bar] X.X/YYG"
		utilLabel := lipgloss.NewStyle().Foreground(theme.TextMuted).Render("Util")
		vramLabel := lipgloss.NewStyle().Foreground(theme.TextMuted).Render("VRAM")

		fixedLen := lipgloss.Width(utilLabel) + 1 + lipgloss.Width(utilSuffix) + 2 +
			lipgloss.Width(vramLabel) + 1 + lipgloss.Width(vramSuffix) + 4 + 4 // bars + brackets
		barSpace := contentWidth - fixedLen
		if barSpace < 8 {
			barSpace = 8
		}
		utilBarW := barSpace / 2
		vramBarW := barSpace - utilBarW
		utilBar := ThresholdProgressBar(utilVal, utilBarW+2)
		vramBar := ThresholdProgressBar(vramPercent, vramBarW+2)
		line3 := fmt.Sprintf("%s %s %s  %s %s %s", utilLabel, utilBar, utilSuffix, vramLabel, vramBar, vramSuffix)
		lines = append(lines, d.padLine(line3, contentWidth))

		// Line 4: Temp | Pwr | Fan in one line
		tColor := tempColor(gpu.TempC)
		tempStyle := lipgloss.NewStyle().Foreground(tColor)
		tempStr := tempStyle.Render(fmt.Sprintf("%d°C", gpu.TempC))

		powerPercent := float64(0)
		if gpu.PowerLimitW > 0 {
			powerPercent = gpu.PowerDrawW / gpu.PowerLimitW * 100
		}
		powerColor := utilThresholdColor(powerPercent)
		powerStyle := lipgloss.NewStyle().Foreground(powerColor)
		powerStr := powerStyle.Render(fmt.Sprintf("%.0f/%.0fW", gpu.PowerDrawW, gpu.PowerLimitW))

		fanColor := theme.TextMuted
		if gpu.FanPercent > 70 {
			fanColor = theme.Warn
		}
		fanStr := lipgloss.NewStyle().Foreground(fanColor).Render(fmt.Sprintf("Fan %d%%", gpu.FanPercent))

		mutedDot := lipgloss.NewStyle().Foreground(theme.Border).Render("·")
		line4 := fmt.Sprintf("Temp %s  %s  Pwr %s  %s  Svc %d",
			tempStr, mutedDot, powerStr, fanStr, d.ServiceCount)
		lines = append(lines, d.padLine(line4, contentWidth))

		// Line 5: CPU + RAM bars
		cpuSuffix := fmt.Sprintf("%.0f%%", d.CPUPercent)
		ramSuffix := fmt.Sprintf("%.0f%%", d.RAMPercent)
		fixedLen2 := len("CPU ") + 1 + len(cpuSuffix) + 3 + len("RAM ") + 1 + len(ramSuffix) + 4
		barSpace2 := contentWidth - fixedLen2
		if barSpace2 >= 8 {
			cpuBarW := barSpace2 / 2
			ramBarW := barSpace2 - cpuBarW
			cpuBar := ThresholdProgressBar(d.CPUPercent, cpuBarW+2)
			ramBar := ThresholdProgressBar(d.RAMPercent, ramBarW+2)
			metricsLine := fmt.Sprintf("%s %s %s   %s %s %s",
				lipgloss.NewStyle().Foreground(theme.TextMuted).Render("CPU"),
				cpuBar, cpuSuffix,
				lipgloss.NewStyle().Foreground(theme.TextMuted).Render("RAM"),
				ramBar, ramSuffix)
			lines = append(lines, d.padLine(metricsLine, contentWidth))
		} else {
			singleBarW := contentWidth - len("CPU ") - 1 - len(cpuSuffix) - 2
			if singleBarW < 4 {
				singleBarW = 4
			}
			cpuBar := ThresholdProgressBar(d.CPUPercent, singleBarW+2)
			ramBar := ThresholdProgressBar(d.RAMPercent, singleBarW+2)
			lines = append(lines, d.padLine(fmt.Sprintf("CPU %s %s", cpuBar, cpuSuffix), contentWidth))
			lines = append(lines, d.padLine(fmt.Sprintf("RAM %s %s", ramBar, ramSuffix), contentWidth))
		}
	} else {
		// No GPU: simpler layout
		cpuSuffix := fmt.Sprintf("%.0f%%", d.CPUPercent)
		ramSuffix := fmt.Sprintf("%.0f%%", d.RAMPercent)

		fixedLen := len("CPU ") + 1 + len(cpuSuffix) + 3 + len("RAM ") + 1 + len(ramSuffix) + 4
		barSpace := contentWidth - fixedLen
		if barSpace >= 8 {
			cpuBarW := barSpace / 2
			ramBarW := barSpace - cpuBarW
			cpuBar := ThresholdProgressBar(d.CPUPercent, cpuBarW+2)
			ramBar := ThresholdProgressBar(d.RAMPercent, ramBarW+2)
			line2 := fmt.Sprintf("CPU %s %s   RAM %s %s", cpuBar, cpuSuffix, ramBar, ramSuffix)
			lines = append(lines, d.padLine(line2, contentWidth))
		} else {
			singleBarW := contentWidth - len("CPU ") - 1 - len(cpuSuffix) - 2
			if singleBarW < 4 {
				singleBarW = 4
			}
			cpuBar := ThresholdProgressBar(d.CPUPercent, singleBarW+2)
			ramBar := ThresholdProgressBar(d.RAMPercent, singleBarW+2)
			lines = append(lines, d.padLine(fmt.Sprintf("CPU %s %s", cpuBar, cpuSuffix), contentWidth))
			lines = append(lines, d.padLine(fmt.Sprintf("RAM %s %s", ramBar, ramSuffix), contentWidth))
		}

		svcLine := fmt.Sprintf("Svc  %d running", d.ServiceCount)
		lines = append(lines, d.padLine(svcLine, contentWidth))
	}

	content := strings.Join(lines, "\n")

	// Use focused (accent) or unfocused (dim) border
	borderColor := theme.Border
	if d.Focused {
		borderColor = theme.Accent
	}
	panel := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(borderColor).
		Padding(0, 1).
		Width(d.Width - 2).
		Render(content)
	return panel
}

// utilThresholdColor returns a Tokyo Night color based on utilization thresholds.
func utilThresholdColor(percent float64) lipgloss.Color {
	switch {
	case percent >= 80:
		return theme.Crit
	case percent >= 50:
		return theme.Warn
	default:
		return theme.Good
	}
}

// ThresholdProgressBar renders a progress bar with color based on value threshold.
// The fill color transitions: green (<50%) → amber (50-80%) → red (>80%).
func ThresholdProgressBar(percent float64, width int) string {
	if width < 2 {
		width = 2
	}
	color := utilThresholdColor(percent)
	return GradientProgressBar(percent, width, color)
}

func (d DeviceCardFull) padLine(line string, width int) string {
	visualWidth := lipgloss.Width(line)
	if visualWidth < width {
		line = line + strings.Repeat(" ", width-visualWidth)
	}
	return line
}
