package views

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/spencerbull/yokai/internal/config"
	"github.com/spencerbull/yokai/internal/tui/theme"
	"github.com/spencerbull/yokai/internal/vscode"
)

type copilotState int

const (
	csListing copilotState = iota
	csConfiguring
	csDone
	csError
)

// Copilot shows OpenAI-compatible endpoints and auto-configures VS Code.
type Copilot struct {
	cfg     *config.Config
	version string
	state   copilotState
	err     string
	width   int
	height  int
}

type copilotConfigMsg struct {
	err error
}

// NewCopilot creates the copilot endpoints view.
func NewCopilot(cfg *config.Config, version string) *Copilot {
	return &Copilot{
		cfg:     cfg,
		version: version,
		state:   csListing,
	}
}

func (c *Copilot) Init() tea.Cmd {
	return nil
}

func (c *Copilot) Update(msg tea.Msg) (View, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		c.width = msg.Width
		c.height = msg.Height

	case copilotConfigMsg:
		if msg.err != nil {
			c.state = csError
			c.err = msg.err.Error()
		} else {
			c.state = csDone
		}

	case tea.KeyMsg:
		switch msg.String() {
		case "a":
			if c.state == csListing {
				c.state = csConfiguring
				return c, c.autoConfigure()
			}
		case "esc":
			return c, PopView()
		case "enter":
			if c.state == csDone || c.state == csError {
				c.state = csListing
			}
		}
	}
	return c, nil
}

func (c *Copilot) autoConfigure() tea.Cmd {
	services := c.cfg.Services
	devices := c.cfg.Devices
	return func() tea.Msg {
		var endpoints []vscode.Endpoint
		for _, svc := range services {
			if svc.Type == "comfyui" {
				continue // ComfyUI isn't OpenAI-compatible
			}
			// Find device host
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

			endpoints = append(endpoints, vscode.Endpoint{
				Family: "openai",
				ID:     svc.ID,
				Name:   fmt.Sprintf("%s (yokai)", modelName),
				URL:    fmt.Sprintf("http://%s:%d/v1", host, svc.Port),
				APIKey: "none",
			})
		}

		if len(endpoints) == 0 {
			return copilotConfigMsg{err: fmt.Errorf("no OpenAI-compatible services running")}
		}

		err := vscode.AddEndpoints(endpoints)
		return copilotConfigMsg{err: err}
	}
}

func (c *Copilot) View() string {
	title := lipgloss.NewStyle().Foreground(theme.Accent).Bold(true).Render("VS Code Copilot Endpoints")

	var body string

	// List current endpoints
	compatibleServices := 0
	var serviceLines []string
	for _, svc := range c.cfg.Services {
		if svc.Type == "comfyui" {
			continue
		}
		compatibleServices++
		host := "localhost"
		for _, dev := range c.cfg.Devices {
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

	// State-specific additions
	switch c.state {
	case csConfiguring:
		body += "\n\n" + theme.StatusLoading() + " " + theme.PrimaryStyle.Render("Configuring VS Code settings.json...")
	case csDone:
		body += "\n\n" + theme.SuccessStyle.Render("✓ VS Code settings.json updated!") + "\n" +
			theme.MutedStyle.Render("  Backup saved as settings.json.yokai.bak")
	case csError:
		body += "\n\n" + theme.CritStyle.Render("✗ "+c.err)
	}

	// Responsive width
	cardWidth := 60
	if c.width > 0 && c.width < 70 {
		cardWidth = c.width - 10
		if cardWidth < 45 {
			cardWidth = 45
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

func (c *Copilot) KeyBinds() []KeyBind {
	return []KeyBind{
		{Key: "a", Help: "auto-configure"},
		{Key: "Esc", Help: "back"},
	}
}
