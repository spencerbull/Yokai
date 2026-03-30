package views

import (
	"sort"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/spencerbull/yokai/internal/config"
	"github.com/spencerbull/yokai/internal/tailscale"
	"github.com/spencerbull/yokai/internal/tui/components"
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
	cfg         *config.Config
	version     string
	state       tsState
	peers       []tailscale.Peer
	cursor      int
	searchInput textinput.Model
	searching   bool
	showTagHelp bool
	selected    map[string]bool
	err         string
	width       int
	height      int
}

type tsStatusMsg struct {
	installed bool
	status    *tailscale.Status
	err       error
}

type peerMatch struct {
	idx   int
	score int
}

// NewTailscaleView creates the Tailscale view.
func NewTailscaleView(cfg *config.Config, version string) *TailscaleView {
	searchInput := components.NewTextField("Filter hostname, IP, OS, or tag")
	searchInput.Blur()
	return &TailscaleView{
		cfg:         cfg,
		version:     version,
		state:       tsChecking,
		searchInput: searchInput,
		selected:    make(map[string]bool),
	}
}

func (t *TailscaleView) Init() tea.Cmd {
	return checkTailscale
}

func checkTailscale() tea.Msg {
	if !tailscale.IsInstalled() {
		return tsStatusMsg{installed: false}
	}

	status, err := tailscale.GetStatus()
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
		t.searchInput.Width = clampInt(t.width-18, 24, 40)

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
		t.cursor = 0
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
			case "h":
				t.showTagHelp = !t.showTagHelp
			}

		case tsPeerList:
			if t.searching {
				switch msg.String() {
				case "esc":
					t.clearSearch()
					return t, nil
				case " ":
					if peer, ok := t.currentPeer(); ok {
						key := t.peerKey(peer)
						t.selected[key] = !t.selected[key]
					}
					return t, nil
				case "up", "k":
					t.moveCursor(-1)
					return t, nil
				case "down", "j":
					t.moveCursor(1)
					return t, nil
				case "enter":
					if peer, ok := t.currentPeer(); ok {
						return t, Navigate(NewSSHCreds(t.cfg, t.version, peer.TailAddr, peer.HostName, "tailscale", peer.Tags))
					}
					return t, nil
				}

				var cmd tea.Cmd
				t.searchInput, cmd = t.searchInput.Update(msg)
				t.clampCursor()
				return t, cmd
			}

			switch msg.String() {
			case "up", "k":
				t.moveCursor(-1)
			case "down", "j":
				t.moveCursor(1)
			case " ":
				if peer, ok := t.currentPeer(); ok {
					key := t.peerKey(peer)
					t.selected[key] = !t.selected[key]
				}
			case "/":
				t.searching = true
				return t, t.searchInput.Focus()
			case "enter":
				if t.isHelpRowSelected() {
					t.showTagHelp = !t.showTagHelp
					return t, nil
				}
				selectedPeers := t.getSelectedPeers()
				if len(selectedPeers) > 0 {
					peer := selectedPeers[0]
					return t, Navigate(NewSSHCreds(t.cfg, t.version, peer.TailAddr, peer.HostName, "tailscale", peer.Tags))
				}
			case "h":
				t.showTagHelp = !t.showTagHelp
			case "esc":
				return t, PopView()
			}
		}
	}

	return t, nil
}

func (t *TailscaleView) getSelectedPeers() []tailscale.Peer {
	visible := t.visiblePeerIndices()
	if len(visible) == 0 {
		return nil
	}

	hasSelection := false
	for _, selected := range t.selected {
		if selected {
			hasSelection = true
			break
		}
	}

	if !hasSelection {
		return []tailscale.Peer{t.peers[visible[t.cursor]]}
	}

	var result []tailscale.Peer
	for _, idx := range visible {
		peer := t.peers[idx]
		if t.selected[t.peerKey(peer)] {
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
			theme.PrimaryStyle.Render(tailscale.InstallInstructions()) + "\n\n" +
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
		visible := t.visiblePeerIndices()
		content = title.Render("Tailscale — Select Devices") + "\n\n" + t.renderSearchBox()
		if len(t.peers) == 0 {
			content += "\n\n" + theme.WarnStyle.Render("No online peers found on your Tailscale network.")
		} else if len(visible) == 0 {
			content += "\n\n" + theme.WarnStyle.Render("No devices match the current filter.")
		} else {
			var lines []string
			for i, idx := range visible {
				peer := t.peers[idx]
				lines = append(lines, t.renderPeerRow(peer, i == t.cursor, t.selected[t.peerKey(peer)]))
			}
			content += "\n\n" + strings.Join(lines, "\n")
		}
		content += "\n\n" + t.renderTagHelpToggle()
	}

	if t.state != tsChecking && t.state != tsPeerList {
		content += "\n\n" + t.renderTagHelpToggle()
	}
	if t.state != tsChecking && t.showTagHelp {
		content += "\n\n" + theme.PrimaryStyle.Render(tailscale.EnrollmentTagHelp())
	}

	cardWidth := 72
	if t.width > 0 && t.width < 82 {
		cardWidth = clampInt(t.width-10, 40, 72)
	}

	card := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(theme.Border).
		Padding(1, 2).
		Width(cardWidth).
		Render(content)

	return lipgloss.NewStyle().Padding(2, 0).Render(card)
}

func (t *TailscaleView) InputActive() bool { return t.searching }

func (t *TailscaleView) Name() string { return "Tailscale" }

func (t *TailscaleView) KeyBinds() []KeyBind {
	switch t.state {
	case tsPeerList:
		if t.searching {
			return []KeyBind{
				{Key: "Type", Help: "fuzzy filter"},
				{Key: "↑/↓", Help: "move match"},
				{Key: "Space", Help: "multi-select"},
				{Key: "Enter", Help: "connect"},
				{Key: "Esc", Help: "cancel search"},
			}
		}
		return []KeyBind{
			{Key: "↑/↓", Help: "navigate"},
			{Key: "/", Help: "search"},
			{Key: "Space", Help: "multi-select"},
			{Key: "h", Help: "tag help"},
			{Key: "Enter", Help: "connect"},
			{Key: "Esc", Help: "back"},
		}
	default:
		return []KeyBind{
			{Key: "r", Help: "retry"},
			{Key: "h", Help: "tag help"},
			{Key: "Esc", Help: "back"},
		}
	}
}

func (t *TailscaleView) renderSearchBox() string {
	label := "Search"
	if t.searching {
		label = "Search (type to fuzzy filter, Esc cancels)"
	}
	search := theme.MutedStyle.Render(label+":") + "\n" + t.searchInput.View()
	if strings.TrimSpace(t.searchInput.Value()) == "" {
		search += "\n" + theme.MutedStyle.Render("Press / to filter by hostname, IP, OS, or tag")
	}
	return search
}

func (t *TailscaleView) renderTagHelpToggle() string {
	label := "[+] How to add the proper Tailscale tag"
	if t.showTagHelp {
		label = "[-] How to add the proper Tailscale tag"
	}
	if t.isHelpRowSelected() {
		return lipgloss.NewStyle().Foreground(theme.Accent).Bold(true).Render(label)
	}
	return theme.MutedStyle.Render(label)
}

func (t *TailscaleView) renderPeerRow(peer tailscale.Peer, active, selected bool) string {
	rowPrefix := "  "
	nameStyle := theme.PrimaryStyle
	metaStyle := theme.MutedStyle
	if active {
		rowPrefix = "> "
		nameStyle = lipgloss.NewStyle().Foreground(theme.Accent).Bold(true)
		metaStyle = lipgloss.NewStyle().Foreground(theme.TextPrimary)
	}

	marker := theme.MutedStyle.Render("○")
	if selected {
		marker = theme.GoodStyle.Render("●")
	}

	firstLine := rowPrefix + marker + " "
	if peer.HasTag(tailscale.AIGPUTag) {
		firstLine += t.renderAIGPUBadge() + " "
	}
	firstLine += nameStyle.Render(peer.HostName)

	details := []string{peer.TailAddr, peer.OS}
	if peer.HasTag(tailscale.AIGPUTag) {
		details = append(details, theme.SuccessStyle.Render("recommended for Yokai"), tailscale.AIGPUTag)
	}
	if other := peer.OtherTags(); len(other) > 0 {
		details = append(details, "tags: "+strings.Join(other, ", "))
	} else if !peer.HasTag(tailscale.AIGPUTag) {
		details = append(details, "no tags")
	}

	secondLine := "    " + metaStyle.Render(strings.Join(details, "  •  "))
	return firstLine + "\n" + secondLine
}

func (t *TailscaleView) renderAIGPUBadge() string {
	return lipgloss.NewStyle().
		Foreground(theme.TextPrimary).
		Background(theme.Accent).
		Bold(true).
		Padding(0, 1).
		Render("RECOMMENDED AI GPU")
}

func (t *TailscaleView) visiblePeerIndices() []int {
	if len(t.peers) == 0 {
		return nil
	}
	query := normalizeSearchQuery(t.searchInput.Value())
	matches := make([]peerMatch, 0, len(t.peers))
	for idx, peer := range t.peers {
		target := normalizeSearchQuery(strings.Join([]string{
			peer.HostName,
			peer.DNSName,
			peer.TailAddr,
			peer.OS,
			strings.Join(peer.Tags, " "),
		}, " "))
		score := fuzzyScore(query, target)
		if score <= 0 {
			continue
		}
		matches = append(matches, peerMatch{idx: idx, score: score})
	}
	sort.SliceStable(matches, func(i, j int) bool {
		if matches[i].score != matches[j].score {
			return matches[i].score > matches[j].score
		}
		leftTagged := t.peers[matches[i].idx].HasTag(tailscale.AIGPUTag)
		rightTagged := t.peers[matches[j].idx].HasTag(tailscale.AIGPUTag)
		if leftTagged != rightTagged {
			if query == "" {
				return leftTagged
			}
		}
		return matches[i].idx < matches[j].idx
	})
	visible := make([]int, 0, len(matches))
	for _, match := range matches {
		visible = append(visible, match.idx)
	}
	return visible
}

func (t *TailscaleView) peerKey(peer tailscale.Peer) string {
	return peer.TailAddr + "|" + peer.HostName
}

func (t *TailscaleView) currentPeer() (tailscale.Peer, bool) {
	visible := t.visiblePeerIndices()
	if len(visible) == 0 || t.cursor < 0 || t.cursor >= len(visible) {
		return tailscale.Peer{}, false
	}
	return t.peers[visible[t.cursor]], true
}

func (t *TailscaleView) moveCursor(delta int) {
	visible := t.visiblePeerIndices()
	t.cursor += delta
	if t.cursor < 0 {
		t.cursor = 0
	}
	maxCursor := len(visible) - 1
	if !t.searching {
		maxCursor = len(visible)
	}
	if maxCursor < 0 {
		maxCursor = 0
	}
	if t.cursor > maxCursor {
		t.cursor = maxCursor
	}
}

func (t *TailscaleView) clampCursor() {
	visible := t.visiblePeerIndices()
	maxCursor := len(visible) - 1
	if !t.searching {
		maxCursor = len(visible)
	}
	if maxCursor < 0 {
		maxCursor = 0
	}
	if t.cursor > maxCursor {
		t.cursor = maxCursor
	}
}

func (t *TailscaleView) isHelpRowSelected() bool {
	if t.state != tsPeerList || t.searching {
		return false
	}
	return t.cursor == len(t.visiblePeerIndices())
}

func (t *TailscaleView) clearSearch() {
	t.searchInput.SetValue("")
	t.searchInput.Blur()
	t.searching = false
	t.cursor = 0
}

func clampInt(v, minV, maxV int) int {
	if v < minV {
		return minV
	}
	if v > maxV {
		return maxV
	}
	return v
}
