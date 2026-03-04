package views

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/spencerbull/yokai/internal/config"
	"github.com/spencerbull/yokai/internal/openclaw"
	"github.com/spencerbull/yokai/internal/opencode"
	"github.com/spencerbull/yokai/internal/tui/theme"
	"github.com/spencerbull/yokai/internal/vscode"
)

type aiToolsState int

const (
	atsListing aiToolsState = iota
	atsConfiguring
	atsDone
	atsError
)

// toolEntry represents a configurable AI tool with its selection state.
type toolEntry struct {
	name     string
	selected bool
}

// toolResult holds the per-tool success/failure from configuration.
type toolResult struct {
	name string
	ok   bool
	err  string
}

// AITools shows OpenAI-compatible endpoints and lets the user select
// which AI coding tools to configure.
type AITools struct {
	cfg     *config.Config
	version string
	state   aiToolsState
	err     string

	// Tool selection
	tools  []toolEntry
	cursor int

	results []toolResult
	width   int
	height  int
}

type aiToolsConfigMsg struct {
	results []toolResult
	err     error // overall error (e.g., no services)
}

// NewAITools creates the AI tools configuration view.
func NewAITools(cfg *config.Config, version string) *AITools {
	return &AITools{
		cfg:     cfg,
		version: version,
		state:   atsListing,
		tools: []toolEntry{
			{name: "VS Code Copilot", selected: true},
			{name: "OpenCode", selected: true},
			{name: "OpenClaw", selected: true},
		},
	}
}

func (a *AITools) Init() tea.Cmd {
	return nil
}

func (a *AITools) Update(msg tea.Msg) (View, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		a.width = msg.Width
		a.height = msg.Height

	case aiToolsConfigMsg:
		if msg.err != nil {
			a.state = atsError
			a.err = msg.err.Error()
		} else {
			a.state = atsDone
			a.results = msg.results
		}

	case tea.KeyMsg:
		switch a.state {
		case atsListing:
			return a.updateListing(msg)
		case atsDone, atsError:
			switch msg.String() {
			case "esc":
				return a, PopView()
			case "enter":
				a.state = atsListing
				a.results = nil
			}
		case atsConfiguring:
			if msg.String() == "esc" {
				return a, PopView()
			}
		}
	}
	return a, nil
}

func (a *AITools) updateListing(msg tea.KeyMsg) (View, tea.Cmd) {
	switch msg.String() {
	case "esc":
		return a, PopView()
	case "up", "k":
		if a.cursor > 0 {
			a.cursor--
		}
	case "down", "j":
		if a.cursor < len(a.tools)-1 {
			a.cursor++
		}
	case " ":
		a.tools[a.cursor].selected = !a.tools[a.cursor].selected
	case "a":
		// Select all
		for i := range a.tools {
			a.tools[i].selected = true
		}
	case "enter":
		selected := a.selectedTools()
		if len(selected) == 0 {
			return a, nil
		}
		a.state = atsConfiguring
		return a, a.configureSelected(selected)
	}
	return a, nil
}

// selectedTools returns the names of currently selected tools.
func (a *AITools) selectedTools() []string {
	var names []string
	for _, t := range a.tools {
		if t.selected {
			names = append(names, t.name)
		}
	}
	return names
}

func (a *AITools) configureSelected(selected []string) tea.Cmd {
	services := a.cfg.Services
	devices := a.cfg.Devices

	// Build a set for quick lookup
	selectedSet := make(map[string]bool)
	for _, name := range selected {
		selectedSet[name] = true
	}

	return func() tea.Msg {
		// Build endpoint list from running services
		type endpoint struct {
			host      string
			port      int
			modelID   string
			modelName string
			serviceID string
		}

		// Build device lookup set
		deviceByID := make(map[string]config.Device)
		for _, dev := range devices {
			deviceByID[dev.ID] = dev
		}

		var endpoints []endpoint
		for _, svc := range services {
			if svc.Type == "comfyui" {
				continue // ComfyUI isn't OpenAI-compatible
			}
			dev, ok := deviceByID[svc.DeviceID]
			if !ok {
				continue // skip orphaned services whose device was removed
			}
			modelName := svc.Model
			if modelName == "" {
				modelName = svc.Type
			}

			endpoints = append(endpoints, endpoint{
				host:      dev.Host,
				port:      svc.Port,
				modelID:   svc.ID,
				modelName: modelName,
				serviceID: svc.ID,
			})
		}

		if len(endpoints) == 0 {
			return aiToolsConfigMsg{err: fmt.Errorf("no OpenAI-compatible services running")}
		}

		var results []toolResult

		// --- VS Code Copilot ---
		if selectedSet["VS Code Copilot"] {
			var vscodeEndpoints []vscode.Endpoint
			for _, ep := range endpoints {
				vscodeEndpoints = append(vscodeEndpoints, vscode.Endpoint{
					Family: "openai",
					ID:     ep.serviceID,
					Name:   fmt.Sprintf("%s (yokai)", ep.modelName),
					URL:    fmt.Sprintf("http://%s:%d/v1", ep.host, ep.port),
					APIKey: "none",
				})
			}
			if err := vscode.AddEndpoints(vscodeEndpoints); err != nil {
				results = append(results, toolResult{name: "VS Code Copilot", ok: false, err: err.Error()})
			} else {
				results = append(results, toolResult{name: "VS Code Copilot", ok: true})
			}
		}

		// --- OpenCode ---
		if selectedSet["OpenCode"] {
			// Migrate any legacy .opencode.json config first
			if legacyPath, err := opencode.DetectConfigPath(); err == nil {
				_ = opencode.MigrateLegacyConfig(legacyPath)
			}

			var opencodeEndpoints []opencode.Endpoint
			for _, ep := range endpoints {
				opencodeEndpoints = append(opencodeEndpoints, opencode.Endpoint{
					BaseURL:   fmt.Sprintf("http://%s:%d/v1", ep.host, ep.port),
					ModelID:   ep.modelName,
					ModelName: fmt.Sprintf("%s (yokai)", ep.modelName),
				})
			}
			if err := opencode.AddEndpoints(opencodeEndpoints); err != nil {
				results = append(results, toolResult{name: "OpenCode", ok: false, err: err.Error()})
			} else {
				results = append(results, toolResult{name: "OpenCode", ok: true})
			}
		}

		// --- OpenClaw ---
		if selectedSet["OpenClaw"] {
			var openclawEndpoints []openclaw.Endpoint
			for _, ep := range endpoints {
				openclawEndpoints = append(openclawEndpoints, openclaw.Endpoint{
					BaseURL:   fmt.Sprintf("http://%s:%d/v1", ep.host, ep.port),
					ModelID:   ep.modelName,
					ModelName: fmt.Sprintf("%s (yokai)", ep.modelName),
				})
			}
			if err := openclaw.AddEndpoints(openclawEndpoints); err != nil {
				results = append(results, toolResult{name: "OpenClaw", ok: false, err: err.Error()})
			} else {
				results = append(results, toolResult{name: "OpenClaw", ok: true})
			}
		}

		return aiToolsConfigMsg{results: results}
	}
}

func (a *AITools) View() string {
	title := lipgloss.NewStyle().Foreground(theme.Accent).Bold(true).Render("AI Tools Configuration")

	var body string

	// Build device lookup set for display
	devByID := make(map[string]config.Device)
	for _, dev := range a.cfg.Devices {
		devByID[dev.ID] = dev
	}

	// List current endpoints
	compatibleServices := 0
	var serviceLines []string
	for _, svc := range a.cfg.Services {
		if svc.Type == "comfyui" {
			continue
		}
		dev, ok := devByID[svc.DeviceID]
		if !ok {
			continue // skip orphaned services whose device was removed
		}
		compatibleServices++
		url := fmt.Sprintf("http://%s:%d/v1", dev.Host, svc.Port)
		serviceLines = append(serviceLines,
			fmt.Sprintf("  %s %s\n    %s",
				theme.StatusOnline(),
				theme.PrimaryStyle.Render(svc.Model),
				theme.MutedStyle.Render(url),
			),
		)
	}

	if compatibleServices == 0 {
		body = theme.WarnStyle.Render("No OpenAI-compatible services running.") + "\n" +
			theme.MutedStyle.Render("Deploy a vLLM or llama.cpp service first.")
	} else {
		body = theme.PrimaryStyle.Render(fmt.Sprintf("%d endpoint(s) available:", compatibleServices)) + "\n\n"
		body += strings.Join(serviceLines, "\n\n")
	}

	// Show tool selection list
	body += "\n\n" + lipgloss.NewStyle().Foreground(theme.Warn).Bold(true).Render("Configure tools:") + "\n"
	for i, tool := range a.tools {
		cursor := "  "
		if i == a.cursor && a.state == atsListing {
			cursor = "> "
		}

		checkbox := "[ ]"
		if tool.selected {
			checkbox = "[x]"
		}

		nameStyle := theme.PrimaryStyle
		if i == a.cursor && a.state == atsListing {
			nameStyle = lipgloss.NewStyle().Foreground(theme.Accent).Bold(true)
		}

		body += fmt.Sprintf("%s%s %s\n", cursor,
			theme.MutedStyle.Render(checkbox),
			nameStyle.Render(tool.name),
		)
	}

	// State-specific additions
	switch a.state {
	case atsListing:
		selectedCount := 0
		for _, t := range a.tools {
			if t.selected {
				selectedCount++
			}
		}
		if selectedCount > 0 && compatibleServices > 0 {
			body += "\n" + theme.SuccessStyle.Render("Press Enter to configure selected tools")
		} else if selectedCount == 0 {
			body += "\n" + theme.MutedStyle.Render("Select at least one tool to configure")
		}
	case atsConfiguring:
		body += "\n" + theme.StatusLoading() + " " + theme.PrimaryStyle.Render("Configuring selected tools...")
	case atsDone:
		body += "\n"
		for _, r := range a.results {
			if r.ok {
				body += theme.SuccessStyle.Render(fmt.Sprintf("  + %s configured", r.name)) + "\n"
			} else {
				body += theme.CritStyle.Render(fmt.Sprintf("  x %s: %s", r.name, r.err)) + "\n"
			}
		}
		body += "\n" + theme.MutedStyle.Render("  Backups saved as *.yokai.bak")
		body += "\n" + theme.MutedStyle.Render("  Press Enter to return, Esc to go back")
	case atsError:
		body += "\n" + theme.CritStyle.Render("x "+a.err)
		body += "\n" + theme.MutedStyle.Render("  Press Enter to return, Esc to go back")
	}

	// Responsive width
	cardWidth := 65
	if a.width > 0 && a.width < 75 {
		cardWidth = a.width - 10
		if cardWidth < 50 {
			cardWidth = 50
		}
	}

	card := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(theme.Border).
		Padding(1, 2).
		Width(cardWidth).
		Render(title + "\n\n" + body)

	return lipgloss.NewStyle().Padding(1, 0).Render(card)
}

func (a *AITools) KeyBinds() []KeyBind {
	return []KeyBind{
		{Key: "j/k", Help: "navigate"},
		{Key: "Space", Help: "toggle"},
		{Key: "a", Help: "select all"},
		{Key: "Enter", Help: "configure"},
		{Key: "Esc", Help: "back"},
	}
}
