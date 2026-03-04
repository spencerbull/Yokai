package components

import (
	"github.com/NimbleMarkets/ntcharts/barchart"
	"github.com/charmbracelet/lipgloss"
	"github.com/spencerbull/yokai/internal/tui/theme"
)

// BarChartData represents a single bar in the chart.
type BarChartData struct {
	Label string
	Value float64
	Max   float64
	Color lipgloss.Color
}

// BarChart wraps ntcharts BarChart for VRAM/utilization visualization.
type BarChart struct {
	Title  string
	Bars   []BarChartData
	Width  int
	Height int
}

// NewBarChart creates a bar chart with the given parameters.
func NewBarChart(title string, bars []BarChartData, width, height int) BarChart {
	return BarChart{
		Title:  title,
		Bars:   bars,
		Width:  width,
		Height: height,
	}
}

// Render returns the rendered bar chart string.
func (bc BarChart) Render() string {
	if len(bc.Bars) == 0 || bc.Width < 6 || bc.Height < 2 {
		return ""
	}

	// Find max value for scaling
	maxVal := float64(0)
	for _, b := range bc.Bars {
		if b.Max > maxVal {
			maxVal = b.Max
		}
		if b.Value > maxVal {
			maxVal = b.Value
		}
	}
	if maxVal == 0 {
		maxVal = 100
	}

	axisStyle := lipgloss.NewStyle().Foreground(theme.TextMuted)
	labelStyle := lipgloss.NewStyle().Foreground(theme.TextMuted)

	chart := barchart.New(bc.Width, bc.Height,
		barchart.WithMaxValue(maxVal),
		barchart.WithNoAutoMaxValue(),
		barchart.WithBarGap(1),
		barchart.WithStyles(axisStyle, labelStyle),
	)

	// Build bar data
	var barData []barchart.BarData
	for _, b := range bc.Bars {
		style := lipgloss.NewStyle().Foreground(b.Color)
		bd := barchart.BarData{
			Label: b.Label,
			Values: []barchart.BarValue{
				{Name: b.Label, Value: b.Value, Style: style},
			},
		}
		barData = append(barData, bd)
	}

	chart.PushAll(barData)
	chart.Draw()

	return chart.View()
}
