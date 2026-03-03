package views

import (
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/spencerbull/yokai/internal/tui/theme"
)

// Help shows the keybind reference overlay.
type Help struct {
	version string
	width   int
	height  int
}

// NewHelp creates the help overlay view.
func NewHelp(version string) *Help {
	return &Help{version: version}
}

func (h *Help) Init() tea.Cmd {
	return nil
}

func (h *Help) Update(msg tea.Msg) (View, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		h.width = msg.Width
		h.height = msg.Height

	case tea.KeyMsg:
		// Any key dismisses the overlay
		return h, PopView()
	}
	return h, nil
}

func (h *Help) View() string {
	title := lipgloss.NewStyle().Foreground(theme.Accent).Bold(true).Render("yokai — Help")

	sections := []struct {
		heading string
		binds   [][2]string
	}{
		{
			heading: "Dashboard",
			binds: [][2]string{
				{"n", "Deploy new service"},
				{"s", "Stop selected service"},
				{"r", "Restart selected service"},
				{"l", "View logs"},
				{"d", "Device manager"},
				{"c", "AI tools config"},
				{"g", "Open Grafana (browser)"},
				{"Tab", "Cycle panel focus"},
				{"1-9", "Quick-switch device"},
				{"Enter", "Expand service detail"},
			},
		},
		{
			heading: "Navigation",
			binds: [][2]string{
				{"↑/↓ or j/k", "Move cursor"},
				{"Enter", "Select / confirm"},
				{"Esc", "Go back"},
				{"Backspace", "Previous wizard step"},
				{"Space", "Toggle multi-select"},
			},
		},
		{
			heading: "Log Viewer",
			binds: [][2]string{
				{"f", "Toggle follow mode"},
				{"PgUp/PgDn", "Page scroll"},
			},
		},
		{
			heading: "Global",
			binds: [][2]string{
				{"?", "Toggle this help"},
				{"Ctrl+C", "Quit"},
				{"q", "Quit"},
			},
		},
	}

	var body string

	// Responsive widths
	keyWidth := 16
	cardWidth := 50
	if h.width > 0 && h.width < 60 {
		cardWidth = h.width - 10
		keyWidth = 12
		if cardWidth < 35 {
			cardWidth = 35
			keyWidth = 10
		}
	}

	keyStyle := lipgloss.NewStyle().Foreground(theme.Accent).Bold(true).Width(keyWidth)
	descStyle := theme.PrimaryStyle

	for _, sec := range sections {
		body += "\n" + lipgloss.NewStyle().Foreground(theme.Warn).Bold(true).Render(sec.heading) + "\n"
		for _, b := range sec.binds {
			body += "  " + keyStyle.Render(b[0]) + descStyle.Render(b[1]) + "\n"
		}
	}

	body += "\n" + theme.MutedStyle.Render("Press any key to close")

	card := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(theme.Accent).
		Padding(1, 3).
		Width(cardWidth).
		Render(title + body)

	return lipgloss.NewStyle().Padding(1, 0).Render(card)
}

func (h *Help) KeyBinds() []KeyBind {
	return nil
}
