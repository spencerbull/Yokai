package components

import (
	"github.com/NimbleMarkets/ntcharts/canvas/runes"
	slc "github.com/NimbleMarkets/ntcharts/linechart/streamlinechart"
	"github.com/charmbracelet/lipgloss"
)

// StreamChart wraps ntcharts StreamLineChart for live monitoring.
// It provides braille-resolution streaming charts with labeled Y-axis.
type StreamChart struct {
	Title  string
	Values []float64
	Width  int
	Height int
	Color  lipgloss.Color
	YMin   float64
	YMax   float64
}

// NewStreamChart creates a streaming chart with the given parameters.
func NewStreamChart(title string, values []float64, width, height int, color lipgloss.Color) StreamChart {
	return StreamChart{
		Title:  title,
		Values: values,
		Width:  width,
		Height: height,
		Color:  color,
		YMin:   0,
		YMax:   100,
	}
}

// Render returns the rendered string representation of the stream chart.
func (s StreamChart) Render() string {
	if s.Width < 8 || s.Height < 3 {
		// Fall back to basic sparkline for very small sizes
		sp := NewSparkline(s.Values, s.Width, s.Color)
		return sp.Render()
	}

	// Chart dimensions: leave room for Y-axis labels (5 chars) and title
	chartWidth := s.Width - 6
	if chartWidth < 4 {
		chartWidth = 4
	}
	chartHeight := s.Height
	if chartHeight < 2 {
		chartHeight = 2
	}

	style := lipgloss.NewStyle().Foreground(s.Color)
	axisStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#565f89"))
	labelStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#565f89"))

	chart := slc.New(chartWidth, chartHeight,
		slc.WithYRange(s.YMin, s.YMax),
		slc.WithStyles(runes.ArcLineStyle, style),
		slc.WithAxesStyles(axisStyle, labelStyle),
	)

	// Push all historical values
	for _, v := range s.Values {
		chart.Push(v)
	}

	// Draw the chart
	chart.DrawAll()

	return chart.View()
}
