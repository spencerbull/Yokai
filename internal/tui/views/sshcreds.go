package views

import (
	"fmt"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/spencerbull/yokai/internal/config"
	"github.com/spencerbull/yokai/internal/tui/theme"
)

type sshField int

const (
	fieldUser sshField = iota
	fieldAuthMethod
	fieldKeyPath
	fieldPassword
)

type authMethod int

const (
	authSSHAgent authMethod = iota
	authKeyFile
	authPassword
)

var authMethodLabels = []string{"SSH Agent", "Key File", "Password"}

// SSHCreds collects SSH credentials for connecting to a device.
type SSHCreds struct {
	cfg            *config.Config
	version        string
	host           string
	label          string // human-friendly name (e.g. Tailscale hostname)
	connectionType string
	user           string
	auth           authMethod
	keyPath        string
	password       string
	activeField    sshField
	err            string
	width          int
	height         int
}

// NewSSHCreds creates the SSH credentials view.
// label is a human-friendly display name for the device (e.g. a Tailscale
// hostname). When empty, the host address is used as the label.
func NewSSHCreds(cfg *config.Config, version string, host, label, connType string) *SSHCreds {
	if label == "" {
		label = host
	}
	return &SSHCreds{
		cfg:            cfg,
		version:        version,
		host:           host,
		label:          label,
		connectionType: connType,
		user:           "root",
		keyPath:        "~/.ssh/id_ed25519",
		activeField:    fieldUser,
	}
}

func (s *SSHCreds) Init() tea.Cmd {
	return nil
}

func (s *SSHCreds) Update(msg tea.Msg) (View, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		s.width = msg.Width
		if s.width > theme.MaxContentWidth-2*theme.ContentPadding {
			s.width = theme.MaxContentWidth - 2*theme.ContentPadding
		}
		s.height = msg.Height

	case tea.KeyMsg:
		switch msg.String() {
		case "tab":
			s.nextField()
		case "shift+tab":
			s.prevField()
		case "enter":
			if s.activeField == fieldAuthMethod {
				s.nextField()
				return s, nil
			}
			return s, s.submit()
		case "esc":
			return s, PopView()
		case "left":
			if s.activeField == fieldAuthMethod && s.auth > 0 {
				s.auth--
			}
		case "right":
			if s.activeField == fieldAuthMethod && s.auth < authPassword {
				s.auth++
			}
		case "backspace":
			s.handleBackspace()
		default:
			s.handleChar(msg.String())
		}
	}
	return s, nil
}

func (s *SSHCreds) nextField() {
	switch s.activeField {
	case fieldUser:
		s.activeField = fieldAuthMethod
	case fieldAuthMethod:
		switch s.auth {
		case authKeyFile:
			s.activeField = fieldKeyPath
		case authPassword:
			s.activeField = fieldPassword
		}
	case fieldKeyPath, fieldPassword:
		// At the end
	}
}

func (s *SSHCreds) prevField() {
	switch s.activeField {
	case fieldAuthMethod:
		s.activeField = fieldUser
	case fieldKeyPath, fieldPassword:
		s.activeField = fieldAuthMethod
	}
}

func (s *SSHCreds) handleBackspace() {
	switch s.activeField {
	case fieldUser:
		if len(s.user) > 0 {
			s.user = s.user[:len(s.user)-1]
		}
	case fieldKeyPath:
		if len(s.keyPath) > 0 {
			s.keyPath = s.keyPath[:len(s.keyPath)-1]
		}
	case fieldPassword:
		if len(s.password) > 0 {
			s.password = s.password[:len(s.password)-1]
		}
	}
}

func (s *SSHCreds) handleChar(ch string) {
	if len(ch) != 1 {
		return
	}
	switch s.activeField {
	case fieldUser:
		s.user += ch
	case fieldKeyPath:
		s.keyPath += ch
	case fieldPassword:
		s.password += ch
	}
}

func (s *SSHCreds) submit() tea.Cmd {
	if s.user == "" {
		s.err = "Username is required"
		return nil
	}
	s.err = ""

	sshKey := ""
	password := ""
	switch s.auth {
	case authKeyFile:
		sshKey = s.keyPath
	case authPassword:
		password = s.password
	}

	return Navigate(NewBootstrap(s.cfg, s.version, s.host, s.label, s.connectionType, s.user, sshKey, password))
}

func (s *SSHCreds) View() string {
	displayName := s.label
	if s.label != s.host {
		displayName = fmt.Sprintf("%s (%s)", s.label, s.host)
	}
	title := lipgloss.NewStyle().Foreground(theme.Accent).Bold(true).
		Render(fmt.Sprintf("SSH Credentials — %s", displayName))

	// User field
	userLabel := s.fieldLabel("Username:", fieldUser)
	userInput := s.fieldInput(s.user, fieldUser)

	// Auth method selector
	authLabel := s.fieldLabel("Auth Method:", fieldAuthMethod)
	var authChoices string
	for i, label := range authMethodLabels {
		if authMethod(i) == s.auth {
			authChoices += lipgloss.NewStyle().Foreground(theme.Accent).Bold(true).Render("[" + label + "]")
		} else {
			authChoices += theme.MutedStyle.Render(" " + label + " ")
		}
		if i < len(authMethodLabels)-1 {
			authChoices += "  "
		}
	}

	// Conditional field based on auth method
	var extraField string
	switch s.auth {
	case authKeyFile:
		keyLabel := s.fieldLabel("Key Path:", fieldKeyPath)
		keyInput := s.fieldInput(s.keyPath, fieldKeyPath)
		extraField = keyLabel + "\n" + keyInput
	case authPassword:
		pwLabel := s.fieldLabel("Password:", fieldPassword)
		masked := ""
		for range s.password {
			masked += "•"
		}
		pwInput := s.fieldInput(masked, fieldPassword)
		extraField = pwLabel + "\n" + pwInput
	case authSSHAgent:
		extraField = theme.MutedStyle.Render("  Will use SSH_AUTH_SOCK agent")
	}

	var errLine string
	if s.err != "" {
		errLine = "\n" + theme.CritStyle.Render(s.err)
	}

	body := fmt.Sprintf("%s\n%s\n\n%s\n%s\n\n%s%s",
		userLabel, userInput,
		authLabel, authChoices,
		extraField, errLine)

	// Responsive width
	cardWidth := 55
	if s.width > 0 && s.width < 65 {
		cardWidth = s.width - 10
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

func (s *SSHCreds) fieldLabel(label string, field sshField) string {
	style := theme.MutedStyle
	if s.activeField == field {
		style = lipgloss.NewStyle().Foreground(theme.Accent)
	}
	return style.Render(label)
}

func (s *SSHCreds) fieldInput(value string, field sshField) string {
	borderColor := theme.Border
	if s.activeField == field {
		borderColor = theme.Accent
	}
	display := value
	if s.activeField == field {
		display += "█"
	}

	// Responsive width
	inputWidth := 40
	if s.width > 0 && s.width < 65 {
		inputWidth = s.width - 25 // Account for padding and borders
		if inputWidth < 25 {
			inputWidth = 25
		}
	}

	return lipgloss.NewStyle().
		Border(lipgloss.NormalBorder()).
		BorderForeground(borderColor).
		Padding(0, 1).
		Width(inputWidth).
		Render(display)
}

func (s *SSHCreds) KeyBinds() []KeyBind {
	return []KeyBind{
		{Key: "Tab", Help: "next field"},
		{Key: "←/→", Help: "auth method"},
		{Key: "Enter", Help: "connect"},
		{Key: "Esc", Help: "back"},
	}
}
