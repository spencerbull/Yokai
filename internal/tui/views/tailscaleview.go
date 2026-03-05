package views

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/spencerbull/yokai/internal/config"
	ts "github.com/spencerbull/yokai/internal/tailscale"
	"github.com/spencerbull/yokai/internal/tui/theme"
)

type tsState int

const (
	tsChecking tsState = iota
	tsNotInstalled
	tsNotConnected
	tsPeerList
)

// TailscaleView handles the Tailscale device selection flow.
type TailscaleView struct {
	cfg      *config.Config
	version  string
	state    tsState
	peers    []ts.Peer
	cursor   int
	selected map[int]bool // multi-select
	err      string
	width    int
	height   int
}

type tsStatusMsg struct {
	installed bool
	status    *ts.Status
	err       error
}

// NewTailscaleView creates the Tailscale view.
func NewTailscaleView(cfg *config.Config, version string) *TailscaleView {
	return &TailscaleView{
		cfg:      cfg,
		version:  version,
		state:    tsChecking,
		selected: make(map[int]bool),
	}
}

func (t *TailscaleView) Init() tea.Cmd {
	return checkTailscale
}

func checkTailscale() tea.Msg {
	if !ts.IsInstalled() {
		return tsStatusMsg{installed: false}
	}

	status, err := ts.GetStatus()
	if err != nil {
		return tsStatusMsg{installed: true, err: err}
	}

	return tsStatusMsg{installed: true, status: status}
}

func (t *TailscaleView) Update(msg tea.Msg) (View, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		t.width = msg.Width
		if t.width > theme.MaxContentWidth-2*theme.ContentPadding {
			t.width = theme.MaxContentWidth - 2*theme.ContentPadding
		}
		t.height = msg.Height

	case tsStatusMsg:
		if !msg.installed {
			t.state = tsNotInstalled
			return t, nil
		}
		if msg.err != nil {
			t.err = msg.err.Error()
			t.state = tsNotConnected
			return t, nil
		}
		if !msg.status.IsRunning() {
			t.state = tsNotConnected
			return t, nil
		}
		t.peers = msg.status.OnlinePeers()
		t.state = tsPeerList

	case tea.KeyMsg:
		switch t.state {
		case tsNotInstalled, tsNotConnected:
			switch msg.String() {
			case "esc":
				return t, PopView()
			case "r":
				t.state = tsChecking
				return t, checkTailscale
			}
		case tsPeerList:
			switch msg.String() {
			case "up", "k":
				if t.cursor > 0 {
					t.cursor--
				}
			case "down", "j":
				if t.cursor < len(t.peers)-1 {
					t.cursor++
				}
			case " ":
				// Toggle multi-select
				t.selected[t.cursor] = !t.selected[t.cursor]
			case "enter":
				selectedPeers := t.getSelectedPeers()
				if len(selectedPeers) > 0 {
					// Navigate to SSH creds for first selected peer
					// TODO: handle multi-device flow
					peer := selectedPeers[0]
					return t, Navigate(NewSSHCreds(t.cfg, t.version, peer.TailAddr, peer.HostName, "tailscale"))
				}
			case "esc":
				return t, PopView()
			}
		}
	}
	return t, nil
}

func (t *TailscaleView) getSelectedPeers() []ts.Peer {
	// If none explicitly selected, use cursor position
	hasSelection := false
	for _, v := range t.selected {
		if v {
			hasSelection = true
			break
		}
	}

	if !hasSelection && len(t.peers) > 0 {
		return []ts.Peer{t.peers[t.cursor]}
	}

	var result []ts.Peer
	for i, peer := range t.peers {
		if t.selected[i] {
			result = append(result, peer)
		}
	}
	return result
}

func (t *TailscaleView) View() string {
	title := lipgloss.NewStyle().Foreground(theme.Accent).Bold(true)

	var content string

	switch t.state {
	case tsChecking:
		content = title.Render("Tailscale") + "\n\n" +
			theme.MutedStyle.Render("Checking Tailscale status...")

	case tsNotInstalled:
		content = title.Render("Tailscale — Not Installed") + "\n\n" +
			theme.CritStyle.Render("Tailscale CLI not found.") + "\n\n" +
			theme.PrimaryStyle.Render(ts.InstallInstructions()) + "\n\n" +
			theme.MutedStyle.Render("Press 'r' to retry after installing.")

	case tsNotConnected:
		msg := "Tailscale is not connected."
		if t.err != "" {
			msg = "Error: " + t.err
		}
		content = title.Render("Tailscale — Not Connected") + "\n\n" +
			theme.WarnStyle.Render(msg) + "\n\n" +
			theme.PrimaryStyle.Render("Run: sudo tailscale up") + "\n\n" +
			theme.MutedStyle.Render("Press 'r' to retry.")

	case tsPeerList:
		if len(t.peers) == 0 {
			content = title.Render("Tailscale — No Peers Online") + "\n\n" +
				theme.WarnStyle.Render("No online peers found on your Tailscale network.")
		} else {
			var lines []string
			for i, peer := range t.peers {
				cursor := "  "
				check := "○"
				style := theme.PrimaryStyle

				if i == t.cursor {
					cursor = "> "
					style = lipgloss.NewStyle().Foreground(theme.Accent).Bold(true)
				}
				if t.selected[i] {
					check = "●"
				}

				line := fmt.Sprintf("%s%s %s  %s  %s",
					cursor,
					theme.GoodStyle.Render(check),
					style.Render(peer.HostName),
					theme.MutedStyle.Render(peer.TailAddr),
					theme.MutedStyle.Render(peer.OS),
				)
				lines = append(lines, line)
			}
			content = title.Render("Tailscale — Select Devices") + "\n\n" +
				strings.Join(lines, "\n")
		}
	}

	// Responsive width
	cardWidth := 60
	if t.width > 0 && t.width < 70 {
		cardWidth = t.width - 10
		if cardWidth < 40 {
			cardWidth = 40
		}
	}

	card := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(theme.Border).
		Padding(1, 2).
		Width(cardWidth).
		Render(content)

	return lipgloss.NewStyle().Padding(2, 0).Render(card)
}

func (t *TailscaleView) KeyBinds() []KeyBind {
	switch t.state {
	case tsPeerList:
		return []KeyBind{
			{Key: "↑/↓", Help: "navigate"},
			{Key: "Space", Help: "multi-select"},
			{Key: "Enter", Help: "connect"},
			{Key: "Esc", Help: "back"},
		}
	default:
		return []KeyBind{
			{Key: "r", Help: "retry"},
			{Key: "Esc", Help: "back"},
		}
	}
}
