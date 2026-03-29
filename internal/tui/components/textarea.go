package components

import (
	"github.com/charmbracelet/bubbles/textarea"
	"github.com/charmbracelet/lipgloss"
	"github.com/spencerbull/yokai/internal/tui/theme"
)

// NewTextAreaField creates a styled multiline textarea with the app's theme.
func NewTextAreaField(placeholder string) textarea.Model {
	ta := textarea.New()
	ta.Placeholder = placeholder
	ta.Prompt = ""
	ta.SetWidth(36)
	ta.SetHeight(6)
	ta.CharLimit = 4096
	ta.ShowLineNumbers = false
	ta.FocusedStyle.Base = lipgloss.NewStyle().Foreground(theme.TextPrimary)
	ta.FocusedStyle.CursorLine = lipgloss.NewStyle().Foreground(theme.TextPrimary)
	ta.FocusedStyle.Placeholder = lipgloss.NewStyle().Foreground(theme.TextMuted)
	ta.FocusedStyle.Prompt = lipgloss.NewStyle().Foreground(theme.Accent)
	ta.Cursor.Style = lipgloss.NewStyle().Foreground(theme.Accent)
	ta.BlurredStyle = ta.FocusedStyle
	return ta
}
