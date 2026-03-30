package tui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	zone "github.com/lrstanley/bubblezone"
	"github.com/spencerbull/yokai/internal/config"
	"github.com/spencerbull/yokai/internal/tui/components"
	"github.com/spencerbull/yokai/internal/tui/theme"
	"github.com/spencerbull/yokai/internal/tui/views"
)

// tabIndex constants for the main navigation tabs.
const (
	tabDashboard = 0
	tabDevices   = 1
	tabDeploy    = 2
	tabLogs      = 3
	tabSettings  = 4
)

// App is the root Bubbletea model.
type App struct {
	currentView views.View
	viewStack   []views.View
	cfg         *config.Config
	version     string
	width       int
	height      int
	quitting    bool
	activeTab   int
	showTabs    bool // show tab bar (hidden during onboarding)
	toasts      components.ToastManager
}

// newApp creates the root app model.
func newApp(cfg *config.Config, version string) *App {
	app := &App{
		cfg:     cfg,
		version: version,
		toasts:  components.NewToastManager(),
	}

	// If no devices configured, start with onboarding; otherwise dashboard
	if cfg.HasDevices() {
		app.currentView = views.NewDashboard(cfg, version)
		app.showTabs = true
	} else {
		app.currentView = views.NewWelcome(cfg, version)
		app.showTabs = false
	}

	return app
}

func (a *App) Init() tea.Cmd {
	return a.currentView.Init()
}

func (a *App) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c":
			a.quitting = true
			return a, tea.Quit
		}

		// Tab bar navigation (only when tabs are shown, view stack is empty,
		// and the current view doesn't have an active text input that needs
		// raw key events like tab, digits, etc.)
		if a.showTabs && len(a.viewStack) == 0 && !a.currentView.InputActive() {
			if a.currentView.Name() == "Dashboard" && (msg.String() == "tab" || msg.String() == "shift+tab") {
				break
			}
			if cmd := a.handleTabKey(msg.String()); cmd != nil {
				return a, cmd
			}
		}

	case tea.MouseMsg:
		// Handle tab bar clicks
		if a.showTabs && len(a.viewStack) == 0 && msg.Action == tea.MouseActionRelease {
			tabs := components.DefaultTabs()
			for i, tab := range tabs {
				if zone.Get(tab.Label).InBounds(msg) {
					if i != a.activeTab {
						return a, a.switchToTab(i)
					}
					break
				}
			}
		}

	case tea.WindowSizeMsg:
		a.width = msg.Width
		a.height = msg.Height
	}

	// Handle toast messages
	if toastCmd := a.toasts.Update(msg); toastCmd != nil {
		return a, toastCmd
	}
	if showMsg, ok := msg.(components.ShowToastMsg); ok {
		cmd := a.toasts.Add(showMsg)
		return a, cmd
	}

	// Check for navigation messages
	switch msg := msg.(type) {
	case views.NavigateMsg:
		return a.navigate(msg)
	case views.PopViewMsg:
		return a.popView()
	}

	// Forward to current view
	newView, cmd := a.currentView.Update(msg)
	a.currentView = newView
	return a, cmd
}

// handleTabKey processes tab-switching key presses.
// Returns a command if a tab switch occurred, nil otherwise.
func (a *App) handleTabKey(key string) tea.Cmd {
	targetTab := -1

	switch key {
	case "tab":
		targetTab = (a.activeTab + 1) % 5
	case "shift+tab":
		targetTab = (a.activeTab + 4) % 5
	case "1":
		targetTab = tabDashboard
	case "2":
		targetTab = tabDevices
	case "3":
		targetTab = tabDeploy
	case "4":
		// Logs needs a container context, skip direct tab
		return nil
	case "5":
		targetTab = tabSettings
	}

	if targetTab < 0 || targetTab == a.activeTab {
		return nil
	}

	return a.switchToTab(targetTab)
}

// switchToTab creates the view for the given tab index and switches to it.
func (a *App) switchToTab(tab int) tea.Cmd {
	var target views.View

	switch tab {
	case tabDashboard:
		target = views.NewDashboard(a.cfg, a.version)
	case tabDevices:
		target = views.NewDeviceManager(a.cfg, a.version)
	case tabDeploy:
		target = views.NewDeploy(a.cfg, a.version)
	case tabSettings:
		target = views.NewAITools(a.cfg, a.version)
	default:
		return nil
	}

	a.activeTab = tab
	a.currentView = target
	a.viewStack = nil // Clear stack on tab switch

	// Forward current window size so the new view can lay out correctly
	// before the next terminal-driven WindowSizeMsg arrives.
	if a.width > 0 {
		sized, _ := a.currentView.Update(tea.WindowSizeMsg{Width: a.width, Height: a.height})
		a.currentView = sized
	}

	return a.currentView.Init()
}

func (a *App) View() string {
	if a.quitting {
		return ""
	}
	if a.width == 0 {
		return "Loading..."
	}

	// Calculate the content width capped at MaxContentWidth
	contentWidth := a.width
	if contentWidth > theme.MaxContentWidth {
		contentWidth = theme.MaxContentWidth
	}

	var tabBarView string
	if a.showTabs {
		tabBar := components.NewTabBar(components.DefaultTabs(), a.activeTab, contentWidth)
		tabBarView = tabBar.Render()
	}

	content := a.currentView.View()

	statusBar := a.buildStatusBar(contentWidth)
	statusBarView := statusBar.Render()

	output := a.renderShell(tabBarView, content, statusBarView)

	// Overlay toasts in the top-right corner with a 1-char right margin
	if toastView := a.toasts.View(a.width); toastView != "" {
		toastLines := strings.Split(toastView, "\n")
		outputLines := strings.Split(output, "\n")
		for i, tl := range toastLines {
			if i < len(outputLines) {
				outputLines[i] = overlayRight(outputLines[i], tl+" ", a.width)
			}
		}
		output = strings.Join(outputLines, "\n")
	}

	return zone.Scan(output)
}

func (a *App) renderShell(tabBarView, content, statusBarView string) string {
	if a.currentView.Name() == "Dashboard" {
		return a.renderCenteredDashboardShell(tabBarView, content, statusBarView)
	}

	var sections []string
	if tabBarView != "" {
		sections = append(sections, tabBarView)
	}
	sections = append(sections, content, statusBarView)
	assembled := lipgloss.JoinVertical(lipgloss.Center, sections...)
	return lipgloss.Place(a.width, a.height, lipgloss.Center, lipgloss.Top, assembled)
}

func (a *App) renderCenteredDashboardShell(tabBarView, content, statusBarView string) string {
	tabHeight := renderedLineCount(tabBarView)
	statusHeight := renderedLineCount(statusBarView)
	bodyHeight := a.height - tabHeight - statusHeight
	contentHeight := renderedLineCount(content)
	if bodyHeight <= 0 || contentHeight >= bodyHeight {
		return a.renderShellFallback(tabBarView, content, statusBarView)
	}

	var sections []string
	if tabBarView != "" {
		sections = append(sections, lipgloss.Place(a.width, tabHeight, lipgloss.Center, lipgloss.Top, tabBarView))
	}
	sections = append(sections, lipgloss.Place(a.width, bodyHeight, lipgloss.Center, lipgloss.Center, content))
	sections = append(sections, lipgloss.Place(a.width, statusHeight, lipgloss.Center, lipgloss.Bottom, statusBarView))
	return stackRenderedBlocks(sections...)
}

func (a *App) renderShellFallback(tabBarView, content, statusBarView string) string {
	var sections []string
	if tabBarView != "" {
		sections = append(sections, tabBarView)
	}
	sections = append(sections, content, statusBarView)
	assembled := lipgloss.JoinVertical(lipgloss.Center, sections...)
	return lipgloss.Place(a.width, a.height, lipgloss.Center, lipgloss.Top, assembled)
}

func renderedLineCount(s string) int {
	if s == "" {
		return 0
	}
	return strings.Count(s, "\n") + 1
}

func stackRenderedBlocks(blocks ...string) string {
	var lines []string
	for _, block := range blocks {
		if block == "" {
			continue
		}
		lines = append(lines, strings.Split(block, "\n")...)
	}
	return strings.Join(lines, "\n")
}

// navigate pushes current view onto stack and switches to the target.
func (a *App) navigate(msg views.NavigateMsg) (tea.Model, tea.Cmd) {
	if msg.Replace {
		if _, ok := msg.Target.(*views.Dashboard); ok {
			a.viewStack = nil
			a.showTabs = true
			a.activeTab = tabDashboard
		}
		a.currentView = msg.Target
	} else {
		a.viewStack = append(a.viewStack, a.currentView)
		a.currentView = msg.Target
	}

	// Forward current window size so the new view renders at the right width.
	if a.width > 0 {
		sized, _ := a.currentView.Update(tea.WindowSizeMsg{Width: a.width, Height: a.height})
		a.currentView = sized
	}

	cmd := a.currentView.Init()
	return a, cmd
}

// popView restores the previous view from the stack and re-initializes it
// so polling loops (e.g., the dashboard's metrics ticker) resume immediately.
func (a *App) popView() (tea.Model, tea.Cmd) {
	if len(a.viewStack) == 0 {
		return a, nil
	}
	a.currentView = a.viewStack[len(a.viewStack)-1]
	a.viewStack = a.viewStack[:len(a.viewStack)-1]

	// Refresh window size in case terminal was resized while another view was active.
	if a.width > 0 {
		sized, _ := a.currentView.Update(tea.WindowSizeMsg{Width: a.width, Height: a.height})
		a.currentView = sized
	}

	return a, a.currentView.Init()
}

// buildStatusBar constructs a StatusBar from the current view stack.
func (a *App) buildStatusBar(width int) components.StatusBar {
	// Build breadcrumbs from the view stack + current view
	var crumbs []string
	for _, v := range a.viewStack {
		crumbs = append(crumbs, v.Name())
	}
	crumbs = append(crumbs, a.currentView.Name())

	// Convert keybinds
	var keybinds []components.StatusBarKeybind
	for _, kb := range a.currentView.KeyBinds() {
		keybinds = append(keybinds, components.StatusBarKeybind{Key: kb.Key, Help: kb.Help})
	}

	return components.StatusBar{
		Breadcrumbs: crumbs,
		Keybinds:    keybinds,
		Width:       width,
	}
}

// overlayRight places the overlay string at the right edge of the base string,
// respecting terminal width. Both strings may contain ANSI escape sequences.
func overlayRight(base, overlay string, width int) string {
	overlayW := ansi.StringWidth(overlay)
	baseW := ansi.StringWidth(base)

	if overlayW >= width {
		return overlay
	}

	startCol := width - overlayW
	if startCol > baseW {
		// Pad base to reach the overlay position
		return base + strings.Repeat(" ", startCol-baseW) + overlay
	}

	// Truncate base and append overlay
	truncated := ansi.Truncate(base, startCol, "")
	return truncated + overlay
}

// Run starts the TUI application.
func Run(version string) error {
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("loading config: %w", err)
	}

	zone.NewGlobal()
	app := newApp(cfg, version)
	p := tea.NewProgram(app, tea.WithAltScreen(), tea.WithMouseCellMotion())
	_, err = p.Run()
	return err
}
