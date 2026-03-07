package views

import (
	"fmt"
	"strconv"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/spencerbull/yokai/internal/config"
	sshpkg "github.com/spencerbull/yokai/internal/ssh"
	"github.com/spencerbull/yokai/internal/tui/theme"
)

// SSHConfigPicker shows SSH config hosts as a selectable list.
type SSHConfigPicker struct {
	cfg     *config.Config
	version string
	hosts   []sshpkg.SSHHost
	cursor  int
	width   int
	height  int
}

// NewSSHConfigPicker creates the SSH config discovery view.
func NewSSHConfigPicker(cfg *config.Config, version string) *SSHConfigPicker {
	return &SSHConfigPicker{
		cfg:     cfg,
		version: version,
		hosts:   sshpkg.DiscoverSSHHosts(),
	}
}

func (p *SSHConfigPicker) Init() tea.Cmd { return nil }

func (p *SSHConfigPicker) Update(msg tea.Msg) (View, tea.Cmd) {
	// Total items = hosts + "Manual entry" + "Back"
	total := len(p.hosts) + 2

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		p.width = msg.Width
		if p.width > theme.MaxContentWidth-2*theme.ContentPadding {
			p.width = theme.MaxContentWidth - 2*theme.ContentPadding
		}
		p.height = msg.Height

	case tea.KeyMsg:
		switch msg.String() {
		case "up", "k":
			if p.cursor > 0 {
				p.cursor--
			}
		case "down", "j":
			if p.cursor < total-1 {
				p.cursor++
			}
		case "enter":
			return p, p.handleSelect()
		case "esc":
			return p, PopView()
		}
	}
	return p, nil
}

func (p *SSHConfigPicker) handleSelect() tea.Cmd {
	// Host selected
	if p.cursor < len(p.hosts) {
		h := p.hosts[p.cursor]
		host := h.HostName
		if host == "" {
			host = h.Alias
		}

		sshPort := 22
		if h.Port != "" {
			if p, err := strconv.Atoi(h.Port); err == nil {
				sshPort = p
			}
		}

		user := h.User
		if user == "" {
			user = "root"
		}

		return Navigate(NewBootstrap(
			p.cfg, p.version,
			host, h.Alias, "ssh-config",
			user, h.IdentityFile, "", "",
			sshPort,
		))
	}

	// "Manual entry"
	if p.cursor == len(p.hosts) {
		return Navigate(NewWelcome(p.cfg, p.version))
	}

	// "Back"
	return PopView()
}

func (p *SSHConfigPicker) View() string {
	title := lipgloss.NewStyle().Foreground(theme.Accent).Bold(true).
		Render("SSH Config Hosts")

	subtitle := theme.MutedStyle.Render("Discovered from ~/.ssh/config")

	var list string
	for i, h := range p.hosts {
		cursor := "  "
		style := theme.PrimaryStyle
		if i == p.cursor {
			cursor = "> "
			style = lipgloss.NewStyle().Foreground(theme.Accent).Bold(true)
		}

		hostname := h.HostName
		if hostname == "" {
			hostname = h.Alias
		}
		detail := hostname
		if h.User != "" {
			detail = h.User + "@" + detail
		}
		if h.Port != "" {
			detail += ":" + h.Port
		}

		list += fmt.Sprintf("%s%s  %s\n",
			cursor,
			style.Render(h.Alias),
			theme.MutedStyle.Render(detail),
		)
	}

	// Separator
	if len(p.hosts) > 0 {
		list += "\n"
	}

	// "Manual entry" option
	idx := len(p.hosts)
	{
		cursor := "  "
		style := theme.PrimaryStyle
		if p.cursor == idx {
			cursor = "> "
			style = lipgloss.NewStyle().Foreground(theme.Accent).Bold(true)
		}
		list += fmt.Sprintf("%s%s\n", cursor, style.Render("Manual entry..."))
	}

	// "Back" option
	idx++
	{
		cursor := "  "
		style := theme.MutedStyle
		if p.cursor == idx {
			cursor = "> "
			style = lipgloss.NewStyle().Foreground(theme.Accent).Bold(true)
		}
		list += fmt.Sprintf("%s%s\n", cursor, style.Render("Back"))
	}

	cardWidth := 55
	if p.width > 0 && p.width < 65 {
		cardWidth = p.width - 10
		if cardWidth < 40 {
			cardWidth = 40
		}
	}

	card := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(theme.Border).
		Padding(1, 2).
		Width(cardWidth).
		Render(title + "\n" + subtitle + "\n\n" + list)

	return lipgloss.NewStyle().Padding(2, 0).Render(card)
}

func (p *SSHConfigPicker) Name() string       { return "SSH Config" }
func (p *SSHConfigPicker) InputActive() bool { return false }

func (p *SSHConfigPicker) KeyBinds() []KeyBind {
	return []KeyBind{
		{Key: "↑/↓", Help: "navigate"},
		{Key: "Enter", Help: "select"},
		{Key: "Esc", Help: "back"},
	}
}
