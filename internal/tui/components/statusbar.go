package components

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/spencerbull/yokai/internal/tui/theme"
)

// StatusBarKeybind is a key/help pair displayed in the status bar.
type StatusBarKeybind struct {
	Key  string
	Help string
}

// StatusBar renders a full-width bottom bar with breadcrumbs and keybinds.
type StatusBar struct {
	Breadcrumbs []string
	Keybinds    []StatusBarKeybind
	Width       int
}

// Render produces a 2-line status bar: a separator line and the bar content.
func (sb StatusBar) Render() string {
	w := sb.Width
	if w <= 0 {
		return ""
	}

	barStyle := lipgloss.NewStyle().
		Width(w).
		Background(theme.Highlight).
		Padding(0, 1)

	// Left side: breadcrumbs
	left := sb.renderBreadcrumbs()

	// Right side: keybinds
	right := sb.renderKeybinds(w - lipgloss.Width(left) - 4) // padding + gap

	if right == "" {
		return barStyle.Render(left)
	}

	// Fill middle space
	leftW := lipgloss.Width(left)
	rightW := lipgloss.Width(right)
	gap := w - leftW - rightW - 2 // account for padding
	if gap < 1 {
		gap = 1
	}

	line := left + strings.Repeat(" ", gap) + right
	return barStyle.Render(line)
}

func (sb StatusBar) renderBreadcrumbs() string {
	if len(sb.Breadcrumbs) == 0 {
		return ""
	}

	sep := theme.MutedStyle.Render(" > ")
	var parts []string
	for i, crumb := range sb.Breadcrumbs {
		if i == len(sb.Breadcrumbs)-1 {
			// Current view: accent colored
			parts = append(parts, theme.KeybindKeyStyle.Render(crumb))
		} else {
			parts = append(parts, theme.MutedStyle.Render(crumb))
		}
	}
	return strings.Join(parts, sep)
}

func (sb StatusBar) renderKeybinds(maxWidth int) string {
	if len(sb.Keybinds) == 0 || maxWidth <= 0 {
		return ""
	}

	var pairs []string
	for _, kb := range sb.Keybinds {
		key := theme.KeybindKeyStyle.
			Copy().
			Background(theme.Highlight).
			Render(kb.Key)
		help := theme.MutedStyle.
			Copy().
			Background(theme.Highlight).
			Render(kb.Help)
		pairs = append(pairs, fmt.Sprintf("%s %s", key, help))
	}

	const gapStr = "  "
	// Build progressively, truncating if too wide
	result := pairs[0]
	for i := 1; i < len(pairs); i++ {
		candidate := result + gapStr + pairs[i]
		if lipgloss.Width(candidate) > maxWidth {
			ellipsis := theme.MutedStyle.Copy().Background(theme.Highlight).Render("...")
			result = result + gapStr + ellipsis
			break
		}
		result = candidate
	}

	return result
}
