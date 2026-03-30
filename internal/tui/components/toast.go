package components

import (
	"fmt"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/spencerbull/yokai/internal/tui/theme"
)

// ToastLevel defines the severity of a toast notification.
type ToastLevel int

const (
	ToastInfo ToastLevel = iota
	ToastSuccess
	ToastWarn
	ToastError
)

const (
	defaultToastDuration = 3 * time.Second
	toastMaxWidth        = 40
)

// ShowToastMsg is sent by views to display a toast notification.
type ShowToastMsg struct {
	Message  string
	Level    ToastLevel
	Duration time.Duration
}

// toastExpiredMsg is sent when a toast's timer expires.
type toastExpiredMsg struct {
	id int
}

type toast struct {
	id        int
	message   string
	level     ToastLevel
	createdAt time.Time
	duration  time.Duration
}

// ToastManager manages a stack of toast notifications.
type ToastManager struct {
	toasts []toast
	nextID int
}

// NewToastManager creates an empty ToastManager.
func NewToastManager() ToastManager {
	return ToastManager{}
}

// Add creates a new toast and returns a tick command for auto-dismiss.
func (tm *ToastManager) Add(msg ShowToastMsg) tea.Cmd {
	dur := msg.Duration
	if dur == 0 {
		dur = defaultToastDuration
	}

	t := toast{
		id:        tm.nextID,
		message:   msg.Message,
		level:     msg.Level,
		createdAt: time.Now(),
		duration:  dur,
	}
	tm.nextID++
	tm.toasts = append(tm.toasts, t)

	id := t.id
	return tea.Tick(dur, func(time.Time) tea.Msg {
		return toastExpiredMsg{id: id}
	})
}

// Update handles toast expiry messages. Returns a command if needed.
func (tm *ToastManager) Update(msg tea.Msg) tea.Cmd {
	if exp, ok := msg.(toastExpiredMsg); ok {
		for i, t := range tm.toasts {
			if t.id == exp.id {
				tm.toasts = append(tm.toasts[:i], tm.toasts[i+1:]...)
				break
			}
		}
	}
	return nil
}

// View renders the toast stack. Returns empty string if no toasts.
// The caller (overlayRight) handles right-alignment onto the output.
func (tm *ToastManager) View(termWidth int) string {
	if len(tm.toasts) == 0 {
		return ""
	}

	maxW := toastMaxWidth
	if termWidth-2 < maxW {
		maxW = termWidth - 2
	}

	var rendered []string
	for _, t := range tm.toasts {
		rendered = append(rendered, renderToast(t, maxW))
	}

	return lipgloss.JoinVertical(lipgloss.Right, rendered...)
}

func renderToast(t toast, maxWidth int) string {
	var icon string
	var borderColor lipgloss.Color

	switch t.level {
	case ToastInfo:
		icon = "i"
		borderColor = theme.Accent
	case ToastSuccess:
		icon = "✓"
		borderColor = theme.Good
	case ToastWarn:
		icon = "!"
		borderColor = theme.Warn
	case ToastError:
		icon = "✗"
		borderColor = theme.Crit
	}

	content := fmt.Sprintf("%s %s", icon, t.message)

	return lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(borderColor).
		Foreground(theme.TextPrimary).
		Padding(0, 1).
		MaxWidth(maxWidth).
		Render(content)
}
