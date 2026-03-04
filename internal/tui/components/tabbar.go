package components

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
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

	activeStyle := lipgloss.NewStyle().
		Foreground(theme.Accent).
		Bold(true).
		Underline(true).
		Padding(0, 1)

	inactiveStyle := lipgloss.NewStyle().
		Foreground(theme.TextMuted).
		Padding(0, 1)

	keyStyle := lipgloss.NewStyle().
		Foreground(theme.TextMuted).
		Faint(true)

	separatorStyle := lipgloss.NewStyle().
		Foreground(theme.Border)

	var parts []string
	for i, tab := range tb.Tabs {
		keyHint := keyStyle.Render(tab.Key)
		var tabStr string
		if i == tb.ActiveIdx {
			tabStr = keyHint + activeStyle.Render(tab.Label)
		} else {
			tabStr = keyHint + inactiveStyle.Render(tab.Label)
		}
		parts = append(parts, tabStr)
		if i < len(tb.Tabs)-1 {
			parts = append(parts, separatorStyle.Render("│"))
		}
	}

	bar := strings.Join(parts, "")

	// Bottom border line
	borderLine := lipgloss.NewStyle().
		Foreground(theme.Border).
		Render(strings.Repeat("─", tb.Width))

	return bar + "\n" + borderLine
}
