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

// RenderPanel renders content inside a bordered panel with a visible title line.
func RenderPanel(title string, content string, width int) string {
	panel := PanelStyle().Width(width - 2).Render(content)
	if title == "" {
		return panel
	}
	titleLine := title
	return lipgloss.JoinVertical(lipgloss.Left, titleLine, panel)
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

// fractionalBlocks provides sub-character precision for progress bars.
// From 1/8 fill to full fill: ▏▎▍▌▋▊▉█
var fractionalBlocks = []string{"▏", "▎", "▍", "▌", "▋", "▊", "▉", "█"}

// ProgressBar renders a gradient progress bar with sub-character precision.
// width is the total bar width in characters including brackets.
func ProgressBar(percent float64, width int) string {
	if width < 2 {
		width = 2
	}
	innerWidth := width - 2 // account for [ ]

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

	// Calculate fill with fractional precision
	fillFloat := percent / 100.0 * float64(innerWidth)
	if fillFloat < 0 {
		fillFloat = 0
	}
	if fillFloat > float64(innerWidth) {
		fillFloat = float64(innerWidth)
	}

	fullChars := int(fillFloat)
	remainder := fillFloat - float64(fullChars)

	var bar string
	bar += "["

	// Full filled characters
	bar += fillStyle.Render(repeat("█", fullChars))

	// Fractional character for sub-cell precision
	fracIdx := int(remainder * 8)
	if fracIdx > 0 && fracIdx <= 8 && fullChars < innerWidth {
		if fracIdx > 7 {
			fracIdx = 7
		}
		bar += fillStyle.Render(fractionalBlocks[fracIdx-1])
		fullChars++
	}

	// Empty space
	empty := innerWidth - fullChars
	if empty > 0 {
		bar += emptyStyle.Render(repeat("░", empty))
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
