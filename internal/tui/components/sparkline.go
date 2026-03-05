package components

import (
	"math"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// Sparkline renders a braille sparkline from a slice of values.
type Sparkline struct {
	Values []float64
	Width  int
	Height int // height in terminal rows (typically 1-2)
	Color  lipgloss.Color
}

// NewSparkline creates a new sparkline with the given parameters.
func NewSparkline(values []float64, width int, color lipgloss.Color) Sparkline {
	return Sparkline{
		Values: values,
		Width:  width,
		Height: 1, // Default to single row
		Color:  color,
	}
}

// Render returns the rendered string representation of the sparkline.
func (s Sparkline) Render() string {
	if s.Width <= 0 || len(s.Values) == 0 {
		if s.Width > 0 {
			return strings.Repeat(" ", s.Width)
		}
		return ""
	}

	// Use block characters for a single-row sparkline
	blocks := []string{" ", "▁", "▂", "▃", "▄", "▅", "▆", "▇", "█"}

	// Take the most recent Width values
	values := s.Values
	if len(values) > s.Width {
		values = values[len(values)-s.Width:]
	}

	// Find min and max for scaling
	if len(values) == 0 {
		return strings.Repeat(" ", s.Width)
	}

	minVal, maxVal := values[0], values[0]
	for _, v := range values {
		if v < minVal {
			minVal = v
		}
		if v > maxVal {
			maxVal = v
		}
	}

	// Avoid division by zero
	if maxVal == minVal {
		// All values are the same, show middle level
		midBlock := blocks[len(blocks)/2]
		style := lipgloss.NewStyle().Foreground(s.Color)
		result := style.Render(strings.Repeat(midBlock, len(values)))
		// Pad to full width if needed
		if len(values) < s.Width {
			result += strings.Repeat(" ", s.Width-len(values))
		}
		return result
	}

	var result strings.Builder
	style := lipgloss.NewStyle().Foreground(s.Color)

	for _, value := range values {
		// Scale value to 0-8 range (indices for blocks array)
		normalized := (value - minVal) / (maxVal - minVal)
		blockIndex := int(math.Round(normalized * float64(len(blocks)-1)))
		if blockIndex < 0 {
			blockIndex = 0
		}
		if blockIndex >= len(blocks) {
			blockIndex = len(blocks) - 1
		}
		result.WriteString(blocks[blockIndex])
	}

	// Pad to full width if needed
	if len(values) < s.Width {
		result.WriteString(strings.Repeat(" ", s.Width-len(values)))
	}

	return style.Render(result.String())
}
