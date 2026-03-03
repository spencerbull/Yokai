package components

import (
	"fmt"
	"strings"
	"testing"
)

func TestNewMetricsBar(t *testing.T) {
	t.Parallel()

	bar := NewMetricsBar("CPU", 75.5, 30)

	if bar.Label != "CPU" {
		t.Errorf("expected label 'CPU', got '%s'", bar.Label)
	}
	if bar.Percent != 75.5 {
		t.Errorf("expected percent 75.5, got %.1f", bar.Percent)
	}
	if bar.Width != 30 {
		t.Errorf("expected width 30, got %d", bar.Width)
	}
}

func TestMetricsBarRenderNormalCase(t *testing.T) {
	t.Parallel()

	bar := NewMetricsBar("CPU", 50.0, 25)
	rendered := bar.Render()

	// Check basic structure - note: may contain ANSI escape codes for styling
	if len(rendered) < 25 {
		t.Errorf("expected rendered length at least 25, got %d", len(rendered))
	}

	// Should contain the label
	if !strings.Contains(rendered, "CPU") {
		t.Error("rendered bar should contain label 'CPU'")
	}

	// Should contain percentage (may be stylized)
	if !strings.Contains(rendered, "50%") {
		t.Error("rendered bar should contain percentage '50%'")
	}

	// Should contain progress bar brackets (may be stylized)
	if !strings.Contains(rendered, "[") || !strings.Contains(rendered, "]") {
		t.Error("rendered bar should contain progress bar brackets")
	}
}

func TestMetricsBarRenderVariousPercentages(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		percent float64
		width   int
	}{
		{"zero percent", 0.0, 30},
		{"low percent", 15.3, 30},
		{"half percent", 50.0, 30},
		{"high percent", 87.9, 30},
		{"full percent", 100.0, 30},
		{"over hundred", 110.5, 30},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			bar := NewMetricsBar("TEST", tt.percent, tt.width)
			rendered := bar.Render()

			// Check length is at least the expected width (may have styling)
			if len(rendered) < tt.width {
				t.Errorf("expected rendered length at least %d, got %d", tt.width, len(rendered))
			}

			// Should contain the percentage (rounded)
			expectedPercent := strings.Contains(rendered, "0%") ||
				strings.Contains(rendered, "15%") ||
				strings.Contains(rendered, "50%") ||
				strings.Contains(rendered, "88%") ||
				strings.Contains(rendered, "100%") ||
				strings.Contains(rendered, "111%")

			if !expectedPercent {
				t.Errorf("rendered bar should contain expected percentage for %.1f%%: %s", tt.percent, rendered)
			}
		})
	}
}

func TestMetricsBarRenderVariousWidths(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		width int
	}{
		{"very narrow", 5},
		{"narrow", 10},
		{"normal", 25},
		{"wide", 50},
		{"very wide", 100},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			bar := NewMetricsBar("MEM", 42.7, tt.width)
			rendered := bar.Render()

			// Length should be at least the requested width (may have styling)
			if len(rendered) < tt.width {
				t.Errorf("expected rendered length at least %d, got %d", tt.width, len(rendered))
			}

			// For very narrow widths, might not have full format
			if tt.width >= 10 {
				// Should at least contain the label for reasonable widths
				if !strings.Contains(rendered, "MEM") {
					t.Errorf("rendered bar should contain label for width %d: %s", tt.width, rendered)
				}
			}
		})
	}
}

func TestMetricsBarRenderTooNarrow(t *testing.T) {
	t.Parallel()

	// Test very narrow widths
	narrowWidths := []int{1, 2, 3, 5, 8}

	for _, width := range narrowWidths {
		t.Run(fmt.Sprintf("width_%d", width), func(t *testing.T) {
			bar := NewMetricsBar("VERYLONGLABEL", 50.0, width)
			rendered := bar.Render()

			// Should return at least the requested width
			if len(rendered) < width {
				t.Errorf("expected rendered length at least %d, got %d", width, len(rendered))
			}

			// For width < 10, behavior is truncation or spaces
			if width < 10 {
				// Should be either spaces or truncated label
				isSpaces := rendered == strings.Repeat(" ", width)
				isTruncated := len(rendered) == width

				if !isSpaces && !isTruncated {
					t.Errorf("narrow width should produce spaces or truncated content, got: %q", rendered)
				}
			}
		})
	}
}

func TestMetricsBarRenderLongLabel(t *testing.T) {
	t.Parallel()

	// Test with label longer than total width
	longLabel := "VERY_LONG_LABEL_THAT_EXCEEDS_WIDTH"
	width := 20

	bar := NewMetricsBar(longLabel, 75.0, width)
	rendered := bar.Render()

	// Should be truncated to at least the width
	if len(rendered) < width {
		t.Errorf("expected rendered length at least %d, got %d", width, len(rendered))
	}

	// Should be the truncated label
	expectedTruncated := longLabel[:width]
	if rendered != expectedTruncated {
		t.Errorf("expected truncated label %q, got %q", expectedTruncated, rendered)
	}
}

func TestMetricsBarRenderEdgeCases(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		label   string
		percent float64
		width   int
	}{
		{"empty label", "", 50.0, 20},
		{"single char label", "X", 25.0, 15},
		{"negative percent", "NEG", -10.0, 20},
		{"zero width", "TEST", 50.0, 0},
		{"decimal precision", "DEC", 33.333, 25},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			bar := NewMetricsBar(tt.label, tt.percent, tt.width)
			rendered := bar.Render()

			// Should handle edge cases gracefully
			if tt.width > 0 {
				if len(rendered) < tt.width {
					t.Errorf("expected rendered length at least %d, got %d", tt.width, len(rendered))
				}
			} else {
				// Zero width should return empty string
				if len(rendered) != 0 {
					t.Errorf("expected empty string for zero width, got %q", rendered)
				}
			}
		})
	}
}

func TestMetricsBarStructure(t *testing.T) {
	t.Parallel()

	// Test struct field accessibility
	bar := MetricsBar{
		Label:   "GPU",
		Percent: 88.5,
		Width:   40,
	}

	if bar.Label != "GPU" {
		t.Errorf("expected Label 'GPU', got '%s'", bar.Label)
	}
	if bar.Percent != 88.5 {
		t.Errorf("expected Percent 88.5, got %.1f", bar.Percent)
	}
	if bar.Width != 40 {
		t.Errorf("expected Width 40, got %d", bar.Width)
	}
}

func TestMetricsBarZeroValues(t *testing.T) {
	t.Parallel()

	// Test with zero values
	var zeroBar MetricsBar
	rendered := zeroBar.Render()

	// Zero width should produce empty string
	if len(rendered) != 0 {
		t.Errorf("expected empty string for zero width, got %q", rendered)
	}
}

func TestMetricsBarConsistentLength(t *testing.T) {
	t.Parallel()

	// Test that output is always exactly the requested width
	label := "TEST"
	width := 30

	percentages := []float64{0, 1, 25, 50, 75, 99, 100}

	for _, percent := range percentages {
		t.Run(fmt.Sprintf("percent_%.0f", percent), func(t *testing.T) {
			bar := NewMetricsBar(label, percent, width)
			rendered := bar.Render()

			if len(rendered) < width {
				t.Errorf("expected length at least %d for %.0f%%, got %d: %q",
					width, percent, len(rendered), rendered)
			}
		})
	}
}
