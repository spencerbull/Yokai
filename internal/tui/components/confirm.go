package components

import (
	"github.com/charmbracelet/lipgloss"
	"github.com/spencerbull/yokai/internal/tui/theme"
)

// ConfirmDialog renders a centered modal-style confirmation card.
type ConfirmDialog struct {
	Message   string
	YesActive bool // true = "Yes" highlighted, false = "No" highlighted
	Width     int
}

// NewConfirmDialog creates a confirm dialog defaulting to "No" selected.
func NewConfirmDialog(message string, width int) ConfirmDialog {
	return ConfirmDialog{
		Message:   message,
		YesActive: false,
		Width:     width,
	}
}

// Render returns the styled confirmation dialog string.
func (c ConfirmDialog) Render() string {
	cardWidth := 50
	if c.Width > 0 && c.Width < 60 {
		cardWidth = c.Width - 10
		if cardWidth < 30 {
			cardWidth = 30
		}
	}

	msg := lipgloss.NewStyle().
		Foreground(theme.TextPrimary).
		Width(cardWidth - 6).
		Render(c.Message)

	activeStyle := lipgloss.NewStyle().
		Foreground(theme.Background).
		Background(theme.Accent).
		Padding(0, 2).
		Bold(true)

	inactiveStyle := lipgloss.NewStyle().
		Foreground(theme.TextMuted).
		Padding(0, 2)

	var yesBtn, noBtn string
	if c.YesActive {
		yesBtn = activeStyle.Render("Yes")
		noBtn = inactiveStyle.Render("No")
	} else {
		yesBtn = inactiveStyle.Render("Yes")
		noBtn = activeStyle.Render("No")
	}

	buttons := lipgloss.JoinHorizontal(lipgloss.Center, yesBtn, "  ", noBtn)

	content := lipgloss.JoinVertical(lipgloss.Center, msg, "", buttons)

	card := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(theme.Border).
		Padding(1, 2).
		Width(cardWidth).
		Render(content)

	return card
}
