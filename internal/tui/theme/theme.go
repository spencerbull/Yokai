package theme

import "github.com/charmbracelet/lipgloss"

// Layout constants for centered max-width container.
const (
	MaxContentWidth = 140
	ContentPadding  = 2
)

// Tokyo Night color palette — matches btop's dark default
var (
	Background  = lipgloss.Color("#1a1b26")
	Border      = lipgloss.Color("#414868")
	Accent      = lipgloss.Color("#7aa2f7")
	Good        = lipgloss.Color("#9ece6a")
	Warn        = lipgloss.Color("#e0af68")
	Crit        = lipgloss.Color("#f7768e")
	TextPrimary = lipgloss.Color("#c0caf5")
	TextMuted   = lipgloss.Color("#565f89")
	Highlight   = lipgloss.Color("#283457")
	Success     = lipgloss.Color("#73daca")
)

// PanelStyle returns the base style for a bordered panel.
func PanelStyle() lipgloss.Style {
	border := lipgloss.RoundedBorder()
	return lipgloss.NewStyle().
		Border(border).
		BorderForeground(Border).
		Padding(0, 1)
}

// Panel creates a rounded-border panel with an optional title.
// NOTE: The title is stored but only used as a hint for BorderTop.
// Use RenderPanel() to actually display a title above panel content.
func Panel(title string) lipgloss.Style {
	return PanelStyle()
}

// RenderPanel renders content inside a bordered panel with a visible title line above.
func RenderPanel(title string, content string, width int) string {
	panelWidth := width - 2
	if panelWidth < 1 {
		panelWidth = 1
	}
	panel := PanelStyle().Width(panelWidth).Render(content)
	if title == "" {
		return panel
	}
	titleLine := title
	return lipgloss.JoinVertical(lipgloss.Left, titleLine, panel)
}

// RenderFocusedPanel renders a panel with an accent-colored border (focused).
func RenderFocusedPanel(title string, content string, width int) string {
	panelWidth := width - 2
	if panelWidth < 1 {
		panelWidth = 1
	}
	focusedStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(Accent).
		Padding(0, 1).
		Width(panelWidth)
	panel := focusedStyle.Render(content)
	if title == "" {
		return panel
	}
	return lipgloss.JoinVertical(lipgloss.Left, title, panel)
}

// TitleStyle renders panel titles (bold, accent colored).
var TitleStyle = lipgloss.NewStyle().
	Foreground(Accent).
	Bold(true)

// MutedStyle for secondary/hint text.
var MutedStyle = lipgloss.NewStyle().
	Foreground(TextMuted)

// PrimaryStyle for main text content.
var PrimaryStyle = lipgloss.NewStyle().
	Foreground(TextPrimary)

// SuccessStyle for success indicators.
var SuccessStyle = lipgloss.NewStyle().
	Foreground(Success)

// WarnStyle for warning indicators.
var WarnStyle = lipgloss.NewStyle().
	Foreground(Warn)

// CritStyle for critical/error indicators.
var CritStyle = lipgloss.NewStyle().
	Foreground(Crit)

// GoodStyle for healthy/ok indicators.
var GoodStyle = lipgloss.NewStyle().
	Foreground(Good)

// HighlightStyle for selected rows.
var HighlightStyle = lipgloss.NewStyle().
	Background(Highlight).
	Foreground(TextPrimary)

// KeybindBarStyle for the bottom keybind bar.
var KeybindBarStyle = lipgloss.NewStyle().
	Foreground(TextMuted).
	Padding(0, 1)

// KeybindKeyStyle for keybind letters.
var KeybindKeyStyle = lipgloss.NewStyle().
	Foreground(Accent).
	Bold(true)

// StatusOnline renders the green online dot.
func StatusOnline() string {
	return lipgloss.NewStyle().Foreground(Good).Render("●")
}

// StatusOffline renders the gray offline dot.
func StatusOffline() string {
	return lipgloss.NewStyle().Foreground(TextMuted).Render("○")
}

// StatusLoading renders the yellow loading indicator.
func StatusLoading() string {
	return lipgloss.NewStyle().Foreground(Warn).Render("⟳")
}

const (
	meterBlockGlyph = "■"
)

// ProgressBar renders a square-block progress bar.
// width is the total bar width in characters including brackets.
// The fill color is value-based: green (<50%), amber (50-80%), red (>80%).
func ProgressBar(percent float64, width int) string {
	if width < 2 {
		width = 2
	}
	innerWidth := width - 2 // account for [ ]

	// Pick color based on severity thresholds
	var color lipgloss.Color
	switch {
	case percent >= 80:
		color = Crit
	case percent >= 50:
		color = Warn
	default:
		color = Good
	}

	fillStyle := lipgloss.NewStyle().Foreground(color)
	emptyStyle := lipgloss.NewStyle().Foreground(TextMuted)

	// Calculate fill using whole square blocks for a btop-style meter.
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

	var bar string
	bar += "["

	bar += fillStyle.Render(repeat(meterBlockGlyph, filledChars))

	// Empty space
	empty := innerWidth - filledChars
	if empty > 0 {
		bar += emptyStyle.Render(repeat(meterBlockGlyph, empty))
	}

	bar += "]"
	return bar
}

// GradientBar renders a multi-segment square-block progress bar where each segment is
// colored based on its position: first 50% green, 50-80% amber, 80-100% red.
// This gives a gradient effect within the bar itself.
func GradientBar(percent float64, width int) string {
	if width < 2 {
		width = 2
	}
	innerWidth := width - 2

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

	// Segment thresholds in character positions
	greenEnd := int(0.50 * float64(innerWidth))
	warnEnd := int(0.80 * float64(innerWidth))

	greenStyle := lipgloss.NewStyle().Foreground(Good)
	warnStyle := lipgloss.NewStyle().Foreground(Warn)
	critStyle := lipgloss.NewStyle().Foreground(Crit)
	emptyStyle := lipgloss.NewStyle().Foreground(TextMuted)

	var bar string
	bar += "["

	for i := 0; i < filledChars; i++ {
		switch {
		case i < greenEnd:
			bar += greenStyle.Render(meterBlockGlyph)
		case i < warnEnd:
			bar += warnStyle.Render(meterBlockGlyph)
		default:
			bar += critStyle.Render(meterBlockGlyph)
		}
	}

	// Empty space
	empty := innerWidth - filledChars
	if empty > 0 {
		bar += emptyStyle.Render(repeat(meterBlockGlyph, empty))
	}

	bar += "]"
	return bar
}

func repeat(s string, n int) string {
	result := ""
	for i := 0; i < n; i++ {
		result += s
	}
	return result
}
