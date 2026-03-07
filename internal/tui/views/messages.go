package views

import (
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/spencerbull/yokai/internal/tui/components"
)

// View is the interface all TUI screens implement.
type View interface {
	Init() tea.Cmd
	Update(msg tea.Msg) (View, tea.Cmd)
	View() string
	KeyBinds() []KeyBind
	// Name returns a short display name for the view (used in the status bar breadcrumbs).
	Name() string
	// InputActive returns true when the view has a focused text input
	// that should receive raw key events (tab, digits, arrows, etc.)
	// without the app-level tab bar intercepting them.
	InputActive() bool
}

// KeyBind represents a keybind shown in the bottom bar.
type KeyBind struct {
	Key  string
	Help string
}

// NavigateMsg tells the App to switch to a new view.
type NavigateMsg struct {
	Target  View
	Replace bool // if true, replace current view instead of pushing to stack
}

// PopViewMsg tells the App to go back to the previous view.
type PopViewMsg struct{}

// Navigate returns a tea.Cmd that sends a NavigateMsg.
func Navigate(target View) tea.Cmd {
	return func() tea.Msg {
		return NavigateMsg{Target: target}
	}
}

// NavigateReplace returns a tea.Cmd that replaces the current view.
func NavigateReplace(target View) tea.Cmd {
	return func() tea.Msg {
		return NavigateMsg{Target: target, Replace: true}
	}
}

// PopView returns a tea.Cmd that pops back to the previous view.
func PopView() tea.Cmd {
	return func() tea.Msg {
		return PopViewMsg{}
	}
}

// Toast level aliases so views don't need to import components directly.
const (
	ToastInfo    = components.ToastInfo
	ToastSuccess = components.ToastSuccess
	ToastWarn    = components.ToastWarn
	ToastError   = components.ToastError
)

// ShowToast returns a tea.Cmd that shows a toast notification.
func ShowToast(message string, level components.ToastLevel) tea.Cmd {
	return func() tea.Msg {
		return components.ShowToastMsg{
			Message: message,
			Level:   level,
		}
	}
}

// ShowToastWithDuration returns a tea.Cmd that shows a toast with a custom duration.
func ShowToastWithDuration(message string, level components.ToastLevel, duration time.Duration) tea.Cmd {
	return func() tea.Msg {
		return components.ShowToastMsg{
			Message:  message,
			Level:    level,
			Duration: duration,
		}
	}
}
