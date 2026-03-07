package views

import (
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/spencerbull/yokai/internal/tui/components"
	"github.com/spencerbull/yokai/internal/tui/theme"
)

// ConfirmView presents a confirmation dialog and dispatches callbacks.
type ConfirmView struct {
	dialog    components.ConfirmDialog
	onConfirm tea.Cmd
	onCancel  tea.Cmd
	width     int
	height    int
}

// NewConfirmView creates a confirmation view.
// onConfirm is executed (then view is popped) when user confirms.
// onCancel is executed (then view is popped) when user cancels. Can be nil.
func NewConfirmView(message string, onConfirm, onCancel tea.Cmd) *ConfirmView {
	return &ConfirmView{
		dialog:    components.NewConfirmDialog(message, 0),
		onConfirm: onConfirm,
		onCancel:  onCancel,
	}
}

func (v *ConfirmView) Init() tea.Cmd { return nil }

func (v *ConfirmView) Update(msg tea.Msg) (View, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		v.width = msg.Width
		if v.width > theme.MaxContentWidth-2*theme.ContentPadding {
			v.width = theme.MaxContentWidth - 2*theme.ContentPadding
		}
		v.height = msg.Height
		v.dialog.Width = v.width

	case tea.KeyMsg:
		switch msg.String() {
		case "left", "h":
			v.dialog.YesActive = true
		case "right", "l":
			v.dialog.YesActive = false
		case "y":
			v.dialog.YesActive = true
			return v, tea.Batch(PopView(), v.onConfirm)
		case "enter":
			if v.dialog.YesActive {
				return v, tea.Batch(PopView(), v.onConfirm)
			}
			return v, tea.Batch(PopView(), v.onCancel)
		case "n", "esc":
			return v, tea.Batch(PopView(), v.onCancel)
		}
	}
	return v, nil
}

func (v *ConfirmView) View() string {
	if v.width == 0 {
		v.width = theme.MaxContentWidth - 2*theme.ContentPadding
	}
	v.dialog.Width = v.width

	card := v.dialog.Render()

	return lipgloss.Place(v.width, v.height,
		lipgloss.Center, lipgloss.Center,
		card)
}

func (v *ConfirmView) Name() string       { return "Confirm" }
func (v *ConfirmView) InputActive() bool { return false }

func (v *ConfirmView) KeyBinds() []KeyBind {
	return []KeyBind{
		{Key: "←/→", Help: "select"},
		{Key: "y", Help: "yes"},
		{Key: "n/Esc", Help: "no"},
		{Key: "Enter", Help: "confirm"},
	}
}
