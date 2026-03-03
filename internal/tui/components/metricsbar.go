package components

import (
	"fmt"
	"strings"

	"github.com/spencerbull/yokai/internal/tui/theme"
)

// MetricsBar renders a labeled progress bar: "CPU [████████░░░░░░░░] 52%"
type MetricsBar struct {
	Label   string
	Percent float64
	Width   int // total width including label
}

// NewMetricsBar creates a new metrics bar with the given parameters.
func NewMetricsBar(label string, percent float64, width int) MetricsBar {
	return MetricsBar{
		Label:   label,
		Percent: percent,
		Width:   width,
	}
}

// Render returns the rendered string representation of the metrics bar.
func (m MetricsBar) Render() string {
	if m.Width < 10 {
		// Too narrow to render meaningfully
		return strings.Repeat(" ", m.Width)
	}

	// Format percentage
	percentStr := fmt.Sprintf("%.0f%%", m.Percent)

	// Calculate available space for the progress bar
	// Format: "LABEL [████████░░░░░░░░] XX%"
	labelLen := len(m.Label)
	percentLen := len(percentStr)
	spacesNeeded := 2 // space before [ and space before %
	bracketsLen := 2  // [ and ]

	totalFixedLen := labelLen + spacesNeeded + bracketsLen + percentLen
	if totalFixedLen >= m.Width {
		// Not enough space, just show label truncated
		if labelLen > m.Width {
			return m.Label[:m.Width]
		}
		return m.Label + strings.Repeat(" ", m.Width-labelLen)
	}

	barWidth := m.Width - totalFixedLen
	if barWidth < 1 {
		barWidth = 1
	}

	progressBar := theme.ProgressBar(m.Percent, barWidth+2) // +2 for brackets

	return fmt.Sprintf("%s %s %s", m.Label, progressBar, percentStr)
}
