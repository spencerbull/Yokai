package components

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/spencerbull/yokai/internal/tui/theme"
)

// GPUPanel renders GPU metrics in a bordered panel.
type GPUPanel struct {
	Index       int
	Name        string
	TempC       int
	UtilPercent int
	VRAMUsedMB  int64
	VRAMTotalMB int64
	PowerDrawW  int
	PowerLimitW int
	FanPercent  int
	Width       int
}

// NewGPUPanel creates a new GPU panel with the given parameters.
func NewGPUPanel(index int, name string, tempC, utilPercent int, vramUsedMB, vramTotalMB int64, powerDrawW, powerLimitW, fanPercent, width int) GPUPanel {
	return GPUPanel{
		Index:       index,
		Name:        name,
		TempC:       tempC,
		UtilPercent: utilPercent,
		VRAMUsedMB:  vramUsedMB,
		VRAMTotalMB: vramTotalMB,
		PowerDrawW:  powerDrawW,
		PowerLimitW: powerLimitW,
		FanPercent:  fanPercent,
		Width:       width,
	}
}

// tempColor returns a color based on temperature thresholds.
func tempColor(tempC int) lipgloss.Color {
	switch {
	case tempC >= 80:
		return theme.Crit
	case tempC >= 60:
		return theme.Warn
	default:
		return theme.Good
	}
}

// Render returns the rendered string representation of the GPU panel.
func (g GPUPanel) Render() string {
	if g.Width < 20 {
		return strings.Repeat(" ", g.Width)
	}

	// Calculate inner content width (subtract 4 for borders and padding)
	contentWidth := g.Width - 4
	if contentWidth < 10 {
		contentWidth = 10
	}

	// Title line: "GPU 0: NVIDIA RTX 4090"
	title := fmt.Sprintf("GPU %d: %s", g.Index, g.Name)
	if len(title) > contentWidth-6 {
		title = title[:contentWidth-9] + "..."
	}

	// Utilization bar with gradient
	utilPercent := float64(g.UtilPercent)
	utilBar := NewMetricsBar("Util", utilPercent, contentWidth/2)

	// Temperature with color-coding
	tColor := tempColor(g.TempC)
	tempStyle := lipgloss.NewStyle().Foreground(tColor)
	tempStr := tempStyle.Render(fmt.Sprintf("🌡 %d°C", g.TempC))

	// Calculate spacing for util line using visual width (not byte length)
	// to account for ANSI escape codes in styled strings
	utilBarStr := utilBar.Render()
	utilBarVisualWidth := lipgloss.Width(utilBarStr)
	tempPlainLen := lipgloss.Width(tempStr)
	utilLineSpacing := contentWidth - utilBarVisualWidth - tempPlainLen
	if utilLineSpacing < 1 {
		utilLineSpacing = 1
	}
	utilLine := utilBarStr + strings.Repeat(" ", utilLineSpacing) + tempStr

	// VRAM visualization with ntcharts bar chart
	vramPercent := float64(0)
	if g.VRAMTotalMB > 0 {
		vramPercent = float64(g.VRAMUsedMB) / float64(g.VRAMTotalMB) * 100
	}
	vramUsedGB := float64(g.VRAMUsedMB) / 1024
	vramTotalGB := float64(g.VRAMTotalMB) / 1024

	// Choose VRAM bar color based on usage
	vramColor := theme.Good
	if vramPercent > 80 {
		vramColor = theme.Crit
	} else if vramPercent > 50 {
		vramColor = theme.Warn
	}

	vramLabel := fmt.Sprintf("%.0fGB/%.0fGB", vramUsedGB, vramTotalGB)
	vramBarWidth := contentWidth - len("VRAM ") - len(vramLabel) - 1
	if vramBarWidth < 5 {
		vramBarWidth = 5
	}
	vramBar := GradientProgressBar(vramPercent, vramBarWidth, vramColor)
	vramLine := fmt.Sprintf("VRAM %s %s", vramBar, vramLabel)

	// Power and fan line with visual indicators
	powerPercent := float64(0)
	if g.PowerLimitW > 0 {
		powerPercent = float64(g.PowerDrawW) / float64(g.PowerLimitW) * 100
	}
	powerColor := theme.Good
	if powerPercent > 80 {
		powerColor = theme.Crit
	} else if powerPercent > 60 {
		powerColor = theme.Warn
	}
	powerStyle := lipgloss.NewStyle().Foreground(powerColor)
	powerStr := fmt.Sprintf("⚡ %s", powerStyle.Render(fmt.Sprintf("%dW/%dW", g.PowerDrawW, g.PowerLimitW)))

	fanStr := fmt.Sprintf("💨 %d%%", g.FanPercent)
	// Use lipgloss.Width for visual width that handles emojis and ANSI codes
	powerPlainLen := lipgloss.Width(powerStr)
	fanPlainLen := lipgloss.Width(fanStr)
	powerLineSpacing := contentWidth - powerPlainLen - fanPlainLen
	if powerLineSpacing < 1 {
		powerLineSpacing = 1
	}
	powerLine := powerStr + strings.Repeat(" ", powerLineSpacing) + fanStr

	// Create panel content
	content := utilLine + "\n" + vramLine + "\n" + powerLine

	// Apply panel styling with title
	titleWithDashes := title + " " + strings.Repeat("─", max(0, contentWidth-len(title)-1))
	panel := theme.Panel(titleWithDashes).Width(g.Width).Render(content)

	return panel
}

// GradientProgressBar renders a progress bar with fractional block characters
// for smoother fill and the given color.
func GradientProgressBar(percent float64, width int, color lipgloss.Color) string {
	if width < 2 {
		width = 2
	}
	innerWidth := width - 2 // account for [ ]

	// Calculate fill using fractional blocks for sub-character precision
	// Fractional blocks: ▏▎▍▌▋▊▉█ (1/8 to 8/8)
	fractionalBlocks := []string{"▏", "▎", "▍", "▌", "▋", "▊", "▉", "█"}

	fillFloat := percent / 100.0 * float64(innerWidth)
	if fillFloat < 0 {
		fillFloat = 0
	}
	if fillFloat > float64(innerWidth) {
		fillFloat = float64(innerWidth)
	}

	fullChars := int(fillFloat)
	remainder := fillFloat - float64(fullChars)
	fracIdx := int(remainder * 8)
	if fracIdx >= 8 {
		fracIdx = 7
	}

	fillStyle := lipgloss.NewStyle().Foreground(color)
	emptyStyle := lipgloss.NewStyle().Foreground(theme.TextMuted)

	var bar strings.Builder
	bar.WriteString("[")

	// Full filled characters
	for i := 0; i < fullChars; i++ {
		bar.WriteString(fillStyle.Render("█"))
	}

	// Fractional character
	if fullChars < innerWidth && fracIdx > 0 {
		bar.WriteString(fillStyle.Render(fractionalBlocks[fracIdx-1]))
		fullChars++ // Count the fractional char
	}

	// Empty space
	remaining := innerWidth - fullChars
	if remaining > 0 {
		bar.WriteString(emptyStyle.Render(strings.Repeat("░", remaining)))
	}

	bar.WriteString("]")
	return bar.String()
}
