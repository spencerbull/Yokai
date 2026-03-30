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

	// Utilization bar — single line
	utilBarWidth := contentWidth - len("Util ") - 5 // "Util [bar] XX%"
	if utilBarWidth < 5 {
		utilBarWidth = 5
	}
	utilBar := GradientProgressBar(float64(g.UtilPercent), utilBarWidth, theme.Accent)
	utilLine := fmt.Sprintf("Util %s %3d%%", utilBar, g.UtilPercent)

	// VRAM bar — single line
	vramPercent := float64(0)
	if g.VRAMTotalMB > 0 {
		vramPercent = float64(g.VRAMUsedMB) / float64(g.VRAMTotalMB) * 100
	}
	vramUsedGB := float64(g.VRAMUsedMB) / 1024
	vramTotalGB := float64(g.VRAMTotalMB) / 1024

	vramColor := theme.Good
	if vramPercent > 80 {
		vramColor = theme.Crit
	} else if vramPercent > 50 {
		vramColor = theme.Warn
	}

	vramLabel := fmt.Sprintf("%.0fG/%.0fG", vramUsedGB, vramTotalGB)
	vramBarWidth := contentWidth - len("VRAM ") - len(vramLabel) - 1
	if vramBarWidth < 5 {
		vramBarWidth = 5
	}
	vramBar := GradientProgressBar(vramPercent, vramBarWidth, vramColor)
	vramLine := fmt.Sprintf("VRAM %s %s", vramBar, vramLabel)

	// Temp + Power + Fan — all on one line
	tColor := tempColor(g.TempC)
	tempStyle := lipgloss.NewStyle().Foreground(tColor)
	tempStr := tempStyle.Render(fmt.Sprintf("%d°C", g.TempC))

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
	powerStr := powerStyle.Render(fmt.Sprintf("%dW/%dW", g.PowerDrawW, g.PowerLimitW))
	fanStr := fmt.Sprintf("Fan %d%%", g.FanPercent)

	infoLine := fmt.Sprintf("Temp %s  Pwr %s  %s", tempStr, powerStr, fanStr)

	// Create panel content (3 lines for compact display)
	content := utilLine + "\n" + vramLine + "\n" + infoLine

	// Apply panel styling with title
	titleWithDashes := title + " " + strings.Repeat("─", max(0, contentWidth-len(title)-1))
	panel := theme.Panel(titleWithDashes).Width(g.Width).Render(content)

	return panel
}

// GradientProgressBar renders a square-block progress bar with the given color.
func GradientProgressBar(percent float64, width int, color lipgloss.Color) string {
	if width < 2 {
		width = 2
	}
	innerWidth := width - 2 // account for [ ]

	fillFloat := percent / 100.0 * float64(innerWidth)
	if fillFloat < 0 {
		fillFloat = 0
	}
	if fillFloat > float64(innerWidth) {
		fillFloat = float64(innerWidth)
	}

	filledChars := int(fillFloat + 0.5)
	if filledChars > innerWidth {
		filledChars = innerWidth
	}

	fillStyle := lipgloss.NewStyle().Foreground(color)
	emptyStyle := lipgloss.NewStyle().Foreground(theme.TextMuted)

	var bar strings.Builder
	bar.WriteString("[")

	// Full filled characters
	for i := 0; i < filledChars; i++ {
		bar.WriteString(fillStyle.Render("■"))
	}

	// Empty space
	remaining := innerWidth - filledChars
	if remaining > 0 {
		bar.WriteString(emptyStyle.Render(strings.Repeat("■", remaining)))
	}

	bar.WriteString("]")
	return bar.String()
}
