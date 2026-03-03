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

// toolResult holds the per-tool success/failure from auto-configure.
type toolResult struct {
	name string
	ok   bool
	err  string
}

// AITools shows OpenAI-compatible endpoints and auto-configures AI coding tools.
type AITools struct {
	cfg     *config.Config
	version string
	state   aiToolsState
	err     string
	results []toolResult
	width   int
	height  int
}

type aiToolsConfigMsg struct {
	results []toolResult
	err     error // overall error (e.g., no services)
}

// NewAITools creates the unified AI tools configuration view.
func NewAITools(cfg *config.Config, version string) *AITools {
	return &AITools{
		cfg:     cfg,
		version: version,
		state:   atsListing,
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
		switch msg.String() {
		case "a":
			if a.state == atsListing {
				a.state = atsConfiguring
				return a, a.autoConfigure()
			}
		case "esc":
			return a, PopView()
		case "enter":
			if a.state == atsDone || a.state == atsError {
				a.state = atsListing
				a.results = nil
			}
		}
	}
	return a, nil
}

func (a *AITools) autoConfigure() tea.Cmd {
	services := a.cfg.Services
	devices := a.cfg.Devices
	return func() tea.Msg {
		// Build endpoint list from running services
		type endpoint struct {
			host      string
			port      int
			modelID   string
			modelName string
			serviceID string
		}

		var endpoints []endpoint
		for _, svc := range services {
			if svc.Type == "comfyui" {
				continue // ComfyUI isn't OpenAI-compatible
			}
			host := "localhost"
			for _, dev := range devices {
				if dev.ID == svc.DeviceID {
					host = dev.Host
					break
				}
			}
			modelName := svc.Model
			if modelName == "" {
				modelName = svc.Type
			}

			endpoints = append(endpoints, endpoint{
				host:      host,
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

		// --- OpenCode ---
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

		// --- OpenClaw ---
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

		return aiToolsConfigMsg{results: results}
	}
}

func (a *AITools) View() string {
	title := lipgloss.NewStyle().Foreground(theme.Accent).Bold(true).Render("AI Tools Configuration")

	var body string

	// List current endpoints
	compatibleServices := 0
	var serviceLines []string
	for _, svc := range a.cfg.Services {
		if svc.Type == "comfyui" {
			continue
		}
		compatibleServices++
		host := "localhost"
		for _, dev := range a.cfg.Devices {
			if dev.ID == svc.DeviceID {
				host = dev.Host
				break
			}
		}
		url := fmt.Sprintf("http://%s:%d/v1", host, svc.Port)
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

	// Show target tools
	body += "\n\n" + lipgloss.NewStyle().Foreground(theme.Warn).Bold(true).Render("Target tools:") + "\n"
	tools := []string{"VS Code Copilot", "OpenCode", "OpenClaw"}
	for _, tool := range tools {
		body += fmt.Sprintf("  %s %s\n",
			theme.MutedStyle.Render("-"),
			theme.PrimaryStyle.Render(tool),
		)
	}

	// State-specific additions
	switch a.state {
	case atsConfiguring:
		body += "\n" + theme.StatusLoading() + " " + theme.PrimaryStyle.Render("Configuring AI tools...")
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
	case atsError:
		body += "\n" + theme.CritStyle.Render("x "+a.err)
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
		{Key: "a", Help: "auto-configure all"},
		{Key: "Esc", Help: "back"},
	}
}
