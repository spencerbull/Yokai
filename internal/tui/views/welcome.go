package views

import (
	"fmt"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/spencerbull/yokai/internal/config"
	"github.com/spencerbull/yokai/internal/tui/theme"
)

type connectionChoice int

const (
	choiceLocal connectionChoice = iota
	choiceTailscale
	choiceManual
)

var choices = []struct {
	label string
	desc  string
}{
	{"Local network (LAN)", "Connect to devices on your local network"},
	{"Tailscale VPN", "Connect via Tailscale mesh network"},
	{"Manual (enter IP/host)", "Specify device addresses manually"},
}

// Welcome is the first-run onboarding screen.
type Welcome struct {
	cfg     *config.Config
	version string
	cursor  connectionChoice
	width   int
	height  int
}

// NewWelcome creates the welcome/onboarding view.
func NewWelcome(cfg *config.Config, version string) *Welcome {
	return &Welcome{
		cfg:     cfg,
		version: version,
		cursor:  choiceLocal,
	}
}

func (w *Welcome) Init() tea.Cmd {
	return nil
}

func (w *Welcome) Update(msg tea.Msg) (View, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		w.width = msg.Width
		if w.width > theme.MaxContentWidth-2*theme.ContentPadding {
			w.width = theme.MaxContentWidth - 2*theme.ContentPadding
		}
		w.height = msg.Height

	case tea.KeyMsg:
		switch msg.String() {
		case "up", "k":
			if w.cursor > 0 {
				w.cursor--
			}
		case "down", "j":
			if w.cursor < choiceManual {
				w.cursor++
			}
		case "enter":
			return w, w.handleSelect()
		case "q":
			return w, tea.Quit
		}
	}
	return w, nil
}

func (w *Welcome) handleSelect() tea.Cmd {
	switch w.cursor {
	case choiceLocal:
		return Navigate(NewLocalNet(w.cfg, w.version))
	case choiceTailscale:
		return Navigate(NewTailscaleView(w.cfg, w.version))
	case choiceManual:
		return Navigate(NewManual(w.cfg, w.version))
	}
	return nil
}

func (w *Welcome) View() string {
	titleStyle := lipgloss.NewStyle().
		Foreground(theme.Accent).
		Bold(true)

	subtitleStyle := lipgloss.NewStyle().
		Foreground(theme.TextMuted)

	// Build the choice list
	choiceList := ""
	for i, c := range choices {
		cursor := "  "
		style := lipgloss.NewStyle().Foreground(theme.TextPrimary)
		if connectionChoice(i) == w.cursor {
			cursor = "> "
			style = lipgloss.NewStyle().Foreground(theme.Accent).Bold(true)
		}
		choiceList += cursor + style.Render(c.label) + "\n"
	}

	// Inner card content
	inner := fmt.Sprintf(
		"%s\n%s\n\n%s\n\n%s",
		titleStyle.Render("GPU Fleet Manager"),
		subtitleStyle.Render("v"+w.version),
		lipgloss.NewStyle().Foreground(theme.TextPrimary).Render("Manage LLM services\nacross your GPU devices.\n\nHow will you connect?"),
		choiceList,
	)

	// Card with rounded border - responsive width
	cardWidth := 40
	if w.width > 0 && w.width < 50 {
		cardWidth = w.width - 10 // Leave some margin
		if cardWidth < 30 {
			cardWidth = 30 // Minimum width
		}
	}

	card := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(theme.Border).
		Padding(1, 3).
		Width(cardWidth).
		Render(inner)

	// Center the card
	centered := lipgloss.NewStyle().
		Padding(2, 0).
		Render(card)

	return centered
}

func (w *Welcome) InputActive() bool { return false }

func (w *Welcome) KeyBinds() []KeyBind {
	return []KeyBind{
		{Key: "↑/↓", Help: "navigate"},
		{Key: "Enter", Help: "select"},
		{Key: "q", Help: "quit"},
	}
}
