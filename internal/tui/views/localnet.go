package views

import (
	"fmt"
	"net"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/spencerbull/yokai/internal/config"
	"github.com/spencerbull/yokai/internal/tui/theme"
)

type localIP struct {
	iface string
	addr  string
}

// LocalNet shows local network interfaces for device discovery.
type LocalNet struct {
	cfg      *config.Config
	version  string
	ips      []localIP
	cursor   int
	scanning bool
	err      string
	width    int
	height   int
}

type localIPsMsg struct {
	ips []localIP
	err error
}

// NewLocalNet creates the local network IP selection view.
func NewLocalNet(cfg *config.Config, version string) *LocalNet {
	return &LocalNet{
		cfg:      cfg,
		version:  version,
		scanning: true,
	}
}

func (l *LocalNet) Init() tea.Cmd {
	return scanLocalIPs
}

func scanLocalIPs() tea.Msg {
	ifaces, err := net.Interfaces()
	if err != nil {
		return localIPsMsg{err: err}
	}

	var ips []localIP
	for _, iface := range ifaces {
		if iface.Flags&net.FlagLoopback != 0 || iface.Flags&net.FlagUp == 0 {
			continue
		}
		addrs, err := iface.Addrs()
		if err != nil {
			continue
		}
		for _, addr := range addrs {
			var ip net.IP
			switch v := addr.(type) {
			case *net.IPNet:
				ip = v.IP
			case *net.IPAddr:
				ip = v.IP
			}
			if ip == nil || ip.IsLoopback() || ip.To4() == nil {
				continue
			}
			ips = append(ips, localIP{
				iface: iface.Name,
				addr:  ip.String(),
			})
		}
	}

	return localIPsMsg{ips: ips}
}

func (l *LocalNet) Update(msg tea.Msg) (View, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		l.width = msg.Width
		l.height = msg.Height

	case localIPsMsg:
		l.scanning = false
		if msg.err != nil {
			l.err = msg.err.Error()
		} else {
			l.ips = msg.ips
		}

	case tea.KeyMsg:
		switch msg.String() {
		case "up", "k":
			if l.cursor > 0 {
				l.cursor--
			}
		case "down", "j":
			if l.cursor < len(l.ips)-1 {
				l.cursor++
			}
		case "enter":
			if len(l.ips) > 0 {
				selected := l.ips[l.cursor]
				_ = selected // TODO: derive subnet, scan for devices, or navigate to SSH creds
				return l, Navigate(NewSSHCreds(l.cfg, l.version, selected.addr, "local"))
			}
		case "esc":
			return l, PopView()
		}
	}
	return l, nil
}

func (l *LocalNet) View() string {
	title := lipgloss.NewStyle().Foreground(theme.Accent).Bold(true).Render("Local Network — Select Interface")

	var body string
	if l.scanning {
		body = theme.MutedStyle.Render("Scanning network interfaces...")
	} else if l.err != "" {
		body = theme.CritStyle.Render("Error: " + l.err)
	} else if len(l.ips) == 0 {
		body = theme.WarnStyle.Render("No active network interfaces found.")
	} else {
		var lines []string
		for i, ip := range l.ips {
			cursor := "  "
			style := theme.PrimaryStyle
			if i == l.cursor {
				cursor = "> "
				style = lipgloss.NewStyle().Foreground(theme.Accent).Bold(true)
			}
			line := fmt.Sprintf("%s%s  %s", cursor, style.Render(ip.addr), theme.MutedStyle.Render("("+ip.iface+")"))
			lines = append(lines, line)
		}
		body = strings.Join(lines, "\n")
	}

	// Responsive width
	cardWidth := 50
	if l.width > 0 && l.width < 60 {
		cardWidth = l.width - 10
		if cardWidth < 35 {
			cardWidth = 35
		}
	}

	card := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(theme.Border).
		Padding(1, 2).
		Width(cardWidth).
		Render(title + "\n\n" + body)

	return lipgloss.NewStyle().Padding(2, 0).Render(card)
}

func (l *LocalNet) KeyBinds() []KeyBind {
	return []KeyBind{
		{Key: "↑/↓", Help: "navigate"},
		{Key: "Enter", Help: "select"},
		{Key: "Esc", Help: "back"},
	}
}
