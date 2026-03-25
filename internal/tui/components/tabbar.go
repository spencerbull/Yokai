package components

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
	zone "github.com/lrstanley/bubblezone"
	"github.com/spencerbull/yokai/internal/tui/theme"
)

// Tab represents a single tab in the tab bar.
type Tab struct {
	Label string
	Key   string // number key shortcut (e.g. "1", "2")
}

// TabBar renders a horizontal tab bar with active tab highlighting.
type TabBar struct {
	Tabs      []Tab
	ActiveIdx int
	Width     int
}

// NewTabBar creates a tab bar with the given tabs.
func NewTabBar(tabs []Tab, activeIdx, width int) TabBar {
	return TabBar{
		Tabs:      tabs,
		ActiveIdx: activeIdx,
		Width:     width,
	}
}

// DefaultTabs returns the standard Yokai navigation tabs.
func DefaultTabs() []Tab {
	return []Tab{
		{Label: "Dashboard", Key: "1"},
		{Label: "Devices", Key: "2"},
		{Label: "Deploy", Key: "3"},
		{Label: "Logs", Key: "4"},
		{Label: "Settings", Key: "5"},
	}
}

// Render returns the rendered tab bar string.
func (tb TabBar) Render() string {
	if len(tb.Tabs) == 0 {
		return ""
	}

	// Active tab: filled background (btop style)
	activeLabelStyle := lipgloss.NewStyle().
		Foreground(theme.Background).
		Background(theme.Accent).
		Bold(true).
		Padding(0, 1)

	// Inactive tabs: dim text, no background
	inactiveStyle := lipgloss.NewStyle().
		Foreground(theme.TextMuted).
		Padding(0, 1)

	keyStyle := lipgloss.NewStyle().
		Foreground(theme.Border).
		Faint(true)

	var parts []string
	for i, tab := range tb.Tabs {
		keyHint := keyStyle.Render(tab.Key)
		var tabStr string
		if i == tb.ActiveIdx {
			tabStr = keyHint + activeLabelStyle.Render(tab.Label)
		} else {
			tabStr = keyHint + inactiveStyle.Render(tab.Label)
		}
		// Wrap in a zone mark so mouse clicks can target this tab
		parts = append(parts, zone.Mark(tab.Label, tabStr))
	}

	bar := strings.Join(parts, "")

	// Bottom border line with accent color under active tab position
	borderLine := lipgloss.NewStyle().
		Foreground(theme.Border).
		Render(strings.Repeat("─", tb.Width))

	return bar + "\n" + borderLine
}
