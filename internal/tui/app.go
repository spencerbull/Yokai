package tui

import (
	"fmt"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/spencerbull/yokai/internal/config"
	"github.com/spencerbull/yokai/internal/tui/views"
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
}

// newApp creates the root app model.
func newApp(cfg *config.Config, version string) *App {
	app := &App{
		cfg:     cfg,
		version: version,
	}

	// If no devices configured, start with onboarding; otherwise dashboard
	if cfg.HasDevices() {
		app.currentView = views.NewDashboard(cfg, version)
	} else {
		app.currentView = views.NewWelcome(cfg, version)
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

	case tea.WindowSizeMsg:
		a.width = msg.Width
		a.height = msg.Height
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

func (a *App) View() string {
	if a.quitting {
		return ""
	}
	if a.width == 0 {
		return "Loading..."
	}

	content := a.currentView.View()

	// Render keybind bar at the bottom
	keybinds := a.currentView.KeyBinds()
	bar := renderKeybindBar(keybinds, a.width)

	return lipgloss.JoinVertical(lipgloss.Left, content, bar)
}

// navigate pushes current view onto stack and switches to the target.
func (a *App) navigate(msg views.NavigateMsg) (tea.Model, tea.Cmd) {
	if msg.Replace {
		a.currentView = msg.Target
	} else {
		a.viewStack = append(a.viewStack, a.currentView)
		a.currentView = msg.Target
	}
	cmd := a.currentView.Init()
	return a, cmd
}

// popView restores the previous view from the stack.
func (a *App) popView() (tea.Model, tea.Cmd) {
	if len(a.viewStack) == 0 {
		return a, nil
	}
	a.currentView = a.viewStack[len(a.viewStack)-1]
	a.viewStack = a.viewStack[:len(a.viewStack)-1]
	return a, nil
}

func renderKeybindBar(binds []views.KeyBind, width int) string {
	if len(binds) == 0 {
		return ""
	}

	var parts []string
	for _, b := range binds {
		key := lipgloss.NewStyle().
			Foreground(lipgloss.Color("#7aa2f7")).
			Bold(true).
			Render(b.Key)
		parts = append(parts, fmt.Sprintf("%s %s", key, b.Help))
	}

	line := ""
	for i, p := range parts {
		if i > 0 {
			line += "  "
		}
		line += p
	}

	return lipgloss.NewStyle().
		Foreground(lipgloss.Color("#565f89")).
		Padding(0, 1).
		Width(width).
		Render(line)
}

// Run starts the TUI application.
func Run(version string) error {
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("loading config: %w", err)
	}

	app := newApp(cfg, version)
	p := tea.NewProgram(app, tea.WithAltScreen())
	_, err = p.Run()
	return err
}
