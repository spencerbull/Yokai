package components

import (
	"fmt"
	"strings"

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
	if len(title) > contentWidth-6 { // Leave space for border decoration
		title = title[:contentWidth-9] + "..."
	}

	// Utilization line: "Util [████████████░░░░░░] 67%   Temp: 72°C"
	utilPercent := float64(g.UtilPercent)
	utilBar := NewMetricsBar("Util", utilPercent, contentWidth/2)
	tempStr := fmt.Sprintf("Temp: %d°C", g.TempC)

	// Calculate spacing for util line
	utilBarStr := utilBar.Render()
	utilLineSpacing := contentWidth - len(utilBarStr) - len(tempStr)
	if utilLineSpacing < 0 {
		utilLineSpacing = 1
	}
	utilLine := utilBarStr + strings.Repeat(" ", utilLineSpacing) + tempStr

	// VRAM line: "VRAM [██████░░░░░░░░░░░░] 12GB/24GB"
	vramPercent := float64(0)
	if g.VRAMTotalMB > 0 {
		vramPercent = float64(g.VRAMUsedMB) / float64(g.VRAMTotalMB) * 100
	}
	vramUsedGB := float64(g.VRAMUsedMB) / 1024
	vramTotalGB := float64(g.VRAMTotalMB) / 1024
	vramLabel := fmt.Sprintf("%.0fGB/%.0fGB", vramUsedGB, vramTotalGB)

	vramBarWidth := contentWidth - len("VRAM ") - len(vramLabel) - 1
	if vramBarWidth < 5 {
		vramBarWidth = 5
	}
	vramBar := theme.ProgressBar(vramPercent, vramBarWidth)
	vramLine := fmt.Sprintf("VRAM %s %s", vramBar, vramLabel)

	// Power line: "Power: 285W/450W              Fan: 55%"
	powerStr := fmt.Sprintf("Power: %dW/%dW", g.PowerDrawW, g.PowerLimitW)
	fanStr := fmt.Sprintf("Fan: %d%%", g.FanPercent)
	powerLineSpacing := contentWidth - len(powerStr) - len(fanStr)
	if powerLineSpacing < 0 {
		powerLineSpacing = 1
	}
	powerLine := powerStr + strings.Repeat(" ", powerLineSpacing) + fanStr

	// Truncate lines if they exceed content width
	if len(utilLine) > contentWidth {
		utilLine = utilLine[:contentWidth]
	}
	if len(vramLine) > contentWidth {
		vramLine = vramLine[:contentWidth]
	}
	if len(powerLine) > contentWidth {
		powerLine = powerLine[:contentWidth]
	}

	// Pad lines to content width
	utilLine = padLine(utilLine, contentWidth)
	vramLine = padLine(vramLine, contentWidth)
	powerLine = padLine(powerLine, contentWidth)

	// Create panel content
	content := utilLine + "\n" + vramLine + "\n" + powerLine

	// Apply panel styling with title
	titleWithDashes := title + " " + strings.Repeat("─", contentWidth-len(title)-1)
	panel := theme.Panel(titleWithDashes).Width(g.Width).Render(content)

	return panel
}

// padLine pads a line to the specified width with spaces.
func padLine(line string, width int) string {
	if len(line) >= width {
		return line
	}
	return line + strings.Repeat(" ", width-len(line))
}
