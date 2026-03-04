package views

import (
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/spencerbull/yokai/internal/config"
	"github.com/spencerbull/yokai/internal/tui/theme"
)

// Manual allows the user to type in a hostname or IP address.
type Manual struct {
	cfg     *config.Config
	version string
	input   string
	err     string
	width   int
	height  int
}

// NewManual creates the manual IP entry view.
func NewManual(cfg *config.Config, version string) *Manual {
	return &Manual{
		cfg:     cfg,
		version: version,
	}
}

func (m *Manual) Init() tea.Cmd {
	return nil
}

func (m *Manual) Update(msg tea.Msg) (View, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		if m.width > theme.MaxContentWidth-2*theme.ContentPadding {
			m.width = theme.MaxContentWidth - 2*theme.ContentPadding
		}
		m.height = msg.Height

	case tea.KeyMsg:
		switch msg.String() {
		case "enter":
			if m.input == "" {
				m.err = "Please enter a hostname or IP"
				return m, nil
			}
			m.err = ""
			return m, Navigate(NewSSHCreds(m.cfg, m.version, m.input, "manual"))
		case "esc":
			return m, PopView()
		case "backspace":
			if len(m.input) > 0 {
				m.input = m.input[:len(m.input)-1]
			}
		case "ctrl+u":
			m.input = ""
		default:
			if len(msg.String()) == 1 || msg.String() == "." || msg.String() == ":" || msg.String() == "-" {
				m.input += msg.String()
			}
		}
	}
	return m, nil
}

func (m *Manual) View() string {
	title := lipgloss.NewStyle().Foreground(theme.Accent).Bold(true).Render("Manual Entry")

	prompt := theme.PrimaryStyle.Render("Enter hostname or IP address:")

	// Responsive widths
	inputWidth := 40
	cardWidth := 50
	if m.width > 0 && m.width < 60 {
		cardWidth = m.width - 10
		inputWidth = cardWidth - 10
		if cardWidth < 35 {
			cardWidth = 35
		}
		if inputWidth < 25 {
			inputWidth = 25
		}
	}

	inputBox := lipgloss.NewStyle().
		Border(lipgloss.NormalBorder()).
		BorderForeground(theme.Accent).
		Padding(0, 1).
		Width(inputWidth).
		Render(m.input + "█")

	var errLine string
	if m.err != "" {
		errLine = "\n" + theme.CritStyle.Render(m.err)
	}

	hint := theme.MutedStyle.Render("Examples: 192.168.1.42, gaming-rig.local, 100.64.0.2")

	card := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(theme.Border).
		Padding(1, 2).
		Width(cardWidth).
		Render(title + "\n\n" + prompt + "\n\n" + inputBox + errLine + "\n\n" + hint)

	return lipgloss.NewStyle().Padding(2, 0).Render(card)
}

func (m *Manual) KeyBinds() []KeyBind {
	return []KeyBind{
		{Key: "Enter", Help: "connect"},
		{Key: "Ctrl+U", Help: "clear"},
		{Key: "Esc", Help: "back"},
	}
}
