package views

import (
	"fmt"
	"os"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/spencerbull/yokai/internal/config"
	"github.com/spencerbull/yokai/internal/hf"
	"github.com/spencerbull/yokai/internal/tui/components"
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
	cfg        *config.Config
	version    string
	state      hfState
	token      string
	tokenInput textinput.Model
	envToken   string
	username   string
	err        string
	width      int
	height     int
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
	tokenInput := components.NewTextField("hf_...")
	tokenInput.Width = 45

	return &HFToken{
		cfg:        cfg,
		version:    version,
		state:      hfChecking,
		tokenInput: tokenInput,
	}
}

// InputActive returns true when the token input is active.
func (h *HFToken) InputActive() bool {
	return h.state == hfInput
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
	var cmd tea.Cmd

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		h.width = msg.Width
		if h.width > theme.MaxContentWidth-2*theme.ContentPadding {
			h.width = theme.MaxContentWidth - 2*theme.ContentPadding
		}
		h.height = msg.Height

		// Update textinput width responsively
		inputWidth := 45
		if h.width > 0 && h.width < 65 {
			inputWidth = h.width - 20
			if inputWidth < 30 {
				inputWidth = 30
			}
		}
		h.tokenInput.Width = inputWidth

	case hfCheckMsg:
		if msg.envToken != "" {
			h.envToken = msg.envToken
			h.token = msg.envToken
			h.tokenInput.SetValue(msg.envToken)
			h.state = hfFound
		} else if h.cfg.HFToken != "" {
			h.token = h.cfg.HFToken
			h.tokenInput.SetValue(h.cfg.HFToken)
			h.state = hfFound
		} else {
			h.state = hfInput
			h.tokenInput.Focus()
		}

	case hfValidateMsg:
		if msg.err != nil {
			h.state = hfInvalid
			h.err = msg.err.Error()
			h.tokenInput.Blur()
		} else {
			h.state = hfValid
			h.username = msg.username
			h.cfg.HFToken = h.tokenInput.Value()
			h.tokenInput.Blur()
			_ = config.Save(h.cfg)
		}

	case tea.KeyMsg:
		switch h.state {
		case hfInput:
			// In input state, only handle specific keys explicitly
			switch msg.String() {
			case "enter":
				if h.tokenInput.Value() == "" {
					h.err = "Token is required for model downloads"
					return h, nil
				}
				h.state = hfValidating
				h.tokenInput.Blur()
				return h, h.validateToken()
			case "esc":
				return h, PopView()
			default:
				// Forward ALL other keys to textinput (including 's')
				h.tokenInput, cmd = h.tokenInput.Update(msg)
				return h, cmd
			}

		case hfFound:
			switch msg.String() {
			case "enter":
				// Use detected token, validate it
				h.state = hfValidating
				return h, h.validateToken()
			case "s":
				// Skip token setup - only works in hfFound state
				return h, NavigateReplace(NewDashboard(h.cfg, h.version))
			case "esc":
				return h, PopView()
			}

		case hfValid:
			switch msg.String() {
			case "enter":
				return h, NavigateReplace(NewDashboard(h.cfg, h.version))
			case "esc":
				return h, PopView()
			}

		case hfInvalid:
			switch msg.String() {
			case "enter":
				h.state = hfInput
				h.err = ""
				h.tokenInput.Focus()
			case "esc":
				return h, PopView()
			}

		default:
			switch msg.String() {
			case "esc":
				return h, PopView()
			}
		}
	}

	// Update textinput if we're not in hfInput state (to handle unfocus, etc.)
	if h.state != hfInput {
		h.tokenInput, cmd = h.tokenInput.Update(msg)
	}

	return h, cmd
}

func (h *HFToken) validateToken() tea.Cmd {
	token := h.tokenInput.Value()
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

		// Render the textinput
		body += h.tokenInput.View()

		if h.err != "" {
			body += "\n" + theme.CritStyle.Render(h.err)
		}
		body += "\n\n" + theme.MutedStyle.Render("Get a token at: https://huggingface.co/settings/tokens")

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
			{Key: "Esc", Help: "back"},
		}
	case hfFound:
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
			{Key: "Esc", Help: "back"},
		}
	}
}
