package views

import (
	"fmt"
	"os"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/spencerbull/yokai/internal/config"
	"github.com/spencerbull/yokai/internal/hf"
	"github.com/spencerbull/yokai/internal/tui/theme"
)

type hfState int

const (
	hfChecking hfState = iota
	hfFound
	hfInput
	hfValidating
	hfValid
	hfInvalid
)

// HFToken handles HuggingFace token detection and input.
type HFToken struct {
	cfg      *config.Config
	version  string
	state    hfState
	token    string
	envToken string
	username string
	err      string
	width    int
	height   int
}

type hfCheckMsg struct {
	envToken string
}

type hfValidateMsg struct {
	username string
	err      error
}

// NewHFToken creates the HuggingFace token view.
func NewHFToken(cfg *config.Config, version string) *HFToken {
	return &HFToken{
		cfg:     cfg,
		version: version,
		state:   hfChecking,
	}
}

func (h *HFToken) Init() tea.Cmd {
	return func() tea.Msg {
		envToken := os.Getenv("HF_TOKEN")
		if envToken == "" {
			envToken = os.Getenv("HUGGING_FACE_HUB_TOKEN")
		}
		return hfCheckMsg{envToken: envToken}
	}
}

func (h *HFToken) Update(msg tea.Msg) (View, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		h.width = msg.Width
		if h.width > theme.MaxContentWidth-2*theme.ContentPadding {
			h.width = theme.MaxContentWidth - 2*theme.ContentPadding
		}
		h.height = msg.Height

	case hfCheckMsg:
		if msg.envToken != "" {
			h.envToken = msg.envToken
			h.token = msg.envToken
			h.state = hfFound
		} else if h.cfg.HFToken != "" {
			h.token = h.cfg.HFToken
			h.state = hfFound
		} else {
			h.state = hfInput
		}

	case hfValidateMsg:
		if msg.err != nil {
			h.state = hfInvalid
			h.err = msg.err.Error()
		} else {
			h.state = hfValid
			h.username = msg.username
			h.cfg.HFToken = h.token
			_ = config.Save(h.cfg)
		}

	case tea.KeyMsg:
		switch msg.String() {
		case "enter":
			switch h.state {
			case hfFound:
				// Use detected token, validate it
				h.state = hfValidating
				return h, h.validateToken()
			case hfInput:
				if h.token == "" {
					h.err = "Token is required for model downloads"
					return h, nil
				}
				h.state = hfValidating
				return h, h.validateToken()
			case hfValid:
				return h, NavigateReplace(NewDashboard(h.cfg, h.version))
			case hfInvalid:
				h.state = hfInput
				h.err = ""
			}
		case "s":
			if h.state == hfInput || h.state == hfFound {
				// Skip token setup
				return h, NavigateReplace(NewDashboard(h.cfg, h.version))
			}
		case "esc":
			return h, PopView()
		case "backspace":
			if h.state == hfInput && len(h.token) > 0 {
				h.token = h.token[:len(h.token)-1]
			}
		case "ctrl+u":
			if h.state == hfInput {
				h.token = ""
			}
		default:
			if h.state == hfInput && len(msg.String()) == 1 {
				h.token += msg.String()
			}
		}
	}
	return h, nil
}

func (h *HFToken) validateToken() tea.Cmd {
	token := h.token
	return func() tea.Msg {
		client := hf.NewClient(token)
		username, err := client.ValidateToken()
		return hfValidateMsg{username: username, err: err}
	}
}

func (h *HFToken) View() string {
	title := lipgloss.NewStyle().Foreground(theme.Accent).Bold(true).Render("HuggingFace Token")

	var body string
	switch h.state {
	case hfChecking:
		body = theme.MutedStyle.Render("Checking for HuggingFace token...")

	case hfFound:
		masked := h.token[:4] + "..." + h.token[len(h.token)-4:]
		source := "config"
		if h.envToken != "" {
			source = "HF_TOKEN env"
		}
		body = fmt.Sprintf("%s %s\n%s\n\n%s",
			theme.GoodStyle.Render("✓ Token detected"),
			theme.MutedStyle.Render("("+source+")"),
			theme.MutedStyle.Render("  "+masked),
			theme.PrimaryStyle.Render("Press Enter to validate, 's' to skip"))

	case hfInput:
		body = theme.PrimaryStyle.Render("Enter your HuggingFace token:") + "\n" +
			theme.MutedStyle.Render("(needed for downloading gated models)") + "\n\n"

		// Responsive width
		inputWidth := 45
		if h.width > 0 && h.width < 65 {
			inputWidth = h.width - 20
			if inputWidth < 30 {
				inputWidth = 30
			}
		}

		inputBox := lipgloss.NewStyle().
			Border(lipgloss.NormalBorder()).
			BorderForeground(theme.Accent).
			Padding(0, 1).
			Width(inputWidth).
			Render(h.token + "█")
		body += inputBox
		if h.err != "" {
			body += "\n" + theme.CritStyle.Render(h.err)
		}
		body += "\n\n" + theme.MutedStyle.Render("Get a token at: https://huggingface.co/settings/tokens\nPress 's' to skip")

	case hfValidating:
		body = theme.StatusLoading() + " " + theme.PrimaryStyle.Render("Validating token...")

	case hfValid:
		body = fmt.Sprintf("%s Logged in as %s\n\n%s",
			theme.GoodStyle.Render("✓"),
			theme.SuccessStyle.Render(h.username),
			theme.MutedStyle.Render("Press Enter to continue to dashboard"))

	case hfInvalid:
		body = fmt.Sprintf("%s %s\n\n%s",
			theme.CritStyle.Render("✗ Invalid token"),
			theme.CritStyle.Render(h.err),
			theme.MutedStyle.Render("Press Enter to re-enter token"))
	}

	// Responsive width
	cardWidth := 55
	if h.width > 0 && h.width < 65 {
		cardWidth = h.width - 10
		if cardWidth < 40 {
			cardWidth = 40
		}
	}

	card := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(theme.Border).
		Padding(1, 2).
		Width(cardWidth).
		Render(title + "\n\n" + body)

	return lipgloss.NewStyle().Padding(2, 0).Render(card)
}

func (h *HFToken) KeyBinds() []KeyBind {
	switch h.state {
	case hfInput:
		return []KeyBind{
			{Key: "Enter", Help: "validate"},
			{Key: "s", Help: "skip"},
			{Key: "Esc", Help: "back"},
		}
	case hfValid:
		return []KeyBind{
			{Key: "Enter", Help: "continue"},
		}
	default:
		return []KeyBind{
			{Key: "Enter", Help: "continue"},
			{Key: "s", Help: "skip"},
			{Key: "Esc", Help: "back"},
		}
	}
}
