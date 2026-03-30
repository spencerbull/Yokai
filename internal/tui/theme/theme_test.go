package theme

import (
	"strings"
	"testing"
)

func TestProgressBarUsesSquareBlockGlyphs(t *testing.T) {
	bar := ProgressBar(42, 12)
	if !strings.Contains(bar, "■") {
		t.Fatalf("expected square block glyphs in progress bar, got %q", bar)
	}
	for _, oldGlyph := range []string{"█", "░", "▏", "▎", "▍", "▌", "▋", "▊", "▉"} {
		if strings.Contains(bar, oldGlyph) {
			t.Fatalf("expected progress bar to avoid legacy glyph %q, got %q", oldGlyph, bar)
		}
	}
}

func TestGradientBarUsesSquareBlockGlyphs(t *testing.T) {
	bar := GradientBar(73, 14)
	if !strings.Contains(bar, "■") {
		t.Fatalf("expected square block glyphs in gradient bar, got %q", bar)
	}
	for _, oldGlyph := range []string{"█", "░", "▏", "▎", "▍", "▌", "▋", "▊", "▉"} {
		if strings.Contains(bar, oldGlyph) {
			t.Fatalf("expected gradient bar to avoid legacy glyph %q, got %q", oldGlyph, bar)
		}
	}
}
