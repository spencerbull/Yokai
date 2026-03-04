package components

import (
	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/spencerbull/yokai/internal/tui/theme"
)

// LoadingSpinner wraps the bubbles spinner for consistent loading states.
type LoadingSpinner struct {
	spinner spinner.Model
	Label   string
	Active  bool
}

// NewLoadingSpinner creates a spinner with a label.
func NewLoadingSpinner(label string) LoadingSpinner {
	s := spinner.New()
	s.Spinner = spinner.Dot
	s.Style = lipgloss.NewStyle().Foreground(theme.Accent)
	return LoadingSpinner{
		spinner: s,
		Label:   label,
		Active:  true,
	}
}

// NewPulseSpinner creates a pulsing spinner variant.
func NewPulseSpinner(label string) LoadingSpinner {
	s := spinner.New()
	s.Spinner = spinner.Pulse
	s.Style = lipgloss.NewStyle().Foreground(theme.Warn)
	return LoadingSpinner{
		spinner: s,
		Label:   label,
		Active:  true,
	}
}

// Init returns the spinner's init command.
func (ls LoadingSpinner) Init() tea.Cmd {
	return ls.spinner.Tick
}

// Update processes spinner tick messages.
func (ls LoadingSpinner) Update(msg tea.Msg) (LoadingSpinner, tea.Cmd) {
	if !ls.Active {
		return ls, nil
	}
	var cmd tea.Cmd
	ls.spinner, cmd = ls.spinner.Update(msg)
	return ls, cmd
}

// View renders the spinner with its label.
func (ls LoadingSpinner) View() string {
	if !ls.Active {
		return ""
	}
	labelStyle := lipgloss.NewStyle().Foreground(theme.TextPrimary)
	return ls.spinner.View() + " " + labelStyle.Render(ls.Label)
}
