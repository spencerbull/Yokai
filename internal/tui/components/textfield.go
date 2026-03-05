package components

import (
	"fmt"

	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/lipgloss"
	"github.com/spencerbull/yokai/internal/tui/theme"
)

// NewTextField creates a styled textinput.Model with the app's theme.
// It accepts an optional placeholder string.
func NewTextField(placeholder string) textinput.Model {
	ti := textinput.New()
	ti.Placeholder = placeholder
	ti.PromptStyle = lipgloss.NewStyle().Foreground(theme.Accent)
	ti.TextStyle = lipgloss.NewStyle().Foreground(theme.TextPrimary)
	ti.PlaceholderStyle = lipgloss.NewStyle().Foreground(theme.TextMuted)
	ti.Cursor.Style = lipgloss.NewStyle().Foreground(theme.Accent)
	ti.CharLimit = 256
	ti.Width = 36
	return ti
}

// NewPasswordField creates a styled textinput.Model that masks input.
func NewPasswordField(placeholder string) textinput.Model {
	ti := NewTextField(placeholder)
	ti.EchoMode = textinput.EchoPassword
	ti.EchoCharacter = '•'
	return ti
}

// NewPortField creates a styled textinput.Model that only accepts digits.
func NewPortField(defaultValue string) textinput.Model {
	ti := NewTextField("")
	ti.SetValue(defaultValue)
	ti.CharLimit = 5
	ti.Width = 8
	ti.Validate = func(s string) error {
		for _, c := range s {
			if c < '0' || c > '9' {
				return fmt.Errorf("digits only")
			}
		}
		return nil
	}
	return ti
}
