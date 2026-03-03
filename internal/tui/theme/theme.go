package theme

import "github.com/charmbracelet/lipgloss"

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

// Panel creates a rounded-border panel with an optional title.
func Panel(title string) lipgloss.Style {
	border := lipgloss.RoundedBorder()
	s := lipgloss.NewStyle().
		Border(border).
		BorderForeground(Border).
		Padding(0, 1)
	if title != "" {
		s = s.BorderTop(true)
	}
	return s
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

// ProgressBar renders a gradient progress bar.
// width is the total bar width in characters.
func ProgressBar(percent float64, width int) string {
	if width < 2 {
		width = 2
	}
	innerWidth := width - 2 // account for [ ]
	filled := int(percent / 100.0 * float64(innerWidth))
	if filled > innerWidth {
		filled = innerWidth
	}
	if filled < 0 {
		filled = 0
	}
	empty := innerWidth - filled

	// Pick color based on severity
	var color lipgloss.Color
	switch {
	case percent > 80:
		color = Crit
	case percent > 50:
		color = Warn
	default:
		color = Good
	}

	fillStyle := lipgloss.NewStyle().Foreground(color)
	emptyStyle := lipgloss.NewStyle().Foreground(TextMuted)

	bar := "[" +
		fillStyle.Render(repeat("█", filled)) +
		emptyStyle.Render(repeat("░", empty)) +
		"]"
	return bar
}

func repeat(s string, n int) string {
	result := ""
	for i := 0; i < n; i++ {
		result += s
	}
	return result
}
