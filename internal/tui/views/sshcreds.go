package views

import (
	"fmt"
	"strconv"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/spencerbull/yokai/internal/config"
	"github.com/spencerbull/yokai/internal/tui/components"
	"github.com/spencerbull/yokai/internal/tui/theme"
)

type sshField int

const (
	fieldUser sshField = iota
	fieldSSHPort
	fieldAuthMethod
	fieldKeyPath
	fieldKeyPassphrase
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
	cfg                *config.Config
	version            string
	host               string
	label              string // human-friendly name (e.g. Tailscale hostname)
	peerTags           []string
	connectionType     string
	userInput          textinput.Model
	sshPortInput       textinput.Model
	keyPathInput       textinput.Model
	keyPassphraseInput textinput.Model
	passwordInput      textinput.Model
	auth               authMethod
	activeField        sshField
	err                string
	width              int
	height             int
}

// NewSSHCreds creates the SSH credentials view.
// label is a human-friendly display name for the device (e.g. a Tailscale
// hostname). When empty, the host address is used as the label.
func NewSSHCreds(cfg *config.Config, version string, host, label, connType string, peerTags []string) *SSHCreds {
	if label == "" {
		label = host
	}

	s := &SSHCreds{
		cfg:            cfg,
		version:        version,
		host:           host,
		label:          label,
		peerTags:       append([]string(nil), peerTags...),
		connectionType: connType,
		auth:           authSSHAgent,
		activeField:    fieldUser,
	}

	// Initialize text inputs using components helpers
	s.userInput = components.NewTextField("username")
	s.userInput.SetValue("root")
	s.userInput.Focus()

	s.sshPortInput = components.NewPortField("22")

	s.keyPathInput = components.NewTextField("~/.ssh/id_ed25519")
	s.keyPathInput.SetValue("~/.ssh/id_ed25519")

	s.keyPassphraseInput = components.NewPasswordField("key passphrase (optional)")
	s.passwordInput = components.NewPasswordField("password")

	return s
}

// NewSSHCredsFromConfig creates the SSH credentials view pre-filled with values
// discovered from ~/.ssh/config. It pre-selects "Key File" auth and focuses the
// passphrase field since the key is known to be encrypted.
func NewSSHCredsFromConfig(cfg *config.Config, version string, host, label, connType, user, keyPath string, sshPort int) *SSHCreds {
	if label == "" {
		label = host
	}

	s := &SSHCreds{
		cfg:            cfg,
		version:        version,
		host:           host,
		label:          label,
		connectionType: connType,
		auth:           authKeyFile,
		activeField:    fieldKeyPassphrase,
	}

	s.userInput = components.NewTextField("username")
	s.userInput.SetValue(user)

	s.sshPortInput = components.NewPortField(fmt.Sprintf("%d", sshPort))

	s.keyPathInput = components.NewTextField("~/.ssh/id_ed25519")
	s.keyPathInput.SetValue(keyPath)

	s.keyPassphraseInput = components.NewPasswordField("key passphrase (required)")
	s.keyPassphraseInput.Focus()
	s.passwordInput = components.NewPasswordField("password")

	return s
}

func (s *SSHCreds) Init() tea.Cmd {
	return textinput.Blink
}

func (s *SSHCreds) Update(msg tea.Msg) (View, tea.Cmd) {
	var cmds []tea.Cmd

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		s.width = msg.Width
		if s.width > theme.MaxContentWidth-2*theme.ContentPadding {
			s.width = theme.MaxContentWidth - 2*theme.ContentPadding
		}
		s.height = msg.Height

	case tea.KeyMsg:
		switch msg.String() {
		case "tab", "down":
			s.nextField()
			return s, nil
		case "shift+tab", "up":
			s.prevField()
			return s, nil
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
				return s, nil
			}
		case "right":
			if s.activeField == fieldAuthMethod && s.auth < authPassword {
				s.auth++
				return s, nil
			}
		}

		// Forward key message to focused text input
		var cmd tea.Cmd
		switch s.activeField {
		case fieldUser:
			s.userInput, cmd = s.userInput.Update(msg)
		case fieldSSHPort:
			s.sshPortInput, cmd = s.sshPortInput.Update(msg)
		case fieldKeyPath:
			s.keyPathInput, cmd = s.keyPathInput.Update(msg)
		case fieldKeyPassphrase:
			s.keyPassphraseInput, cmd = s.keyPassphraseInput.Update(msg)
		case fieldPassword:
			s.passwordInput, cmd = s.passwordInput.Update(msg)
		}
		if cmd != nil {
			cmds = append(cmds, cmd)
		}
	}

	// Update all text inputs with non-key messages
	if _, isKey := msg.(tea.KeyMsg); !isKey {
		var cmd tea.Cmd
		s.userInput, cmd = s.userInput.Update(msg)
		if cmd != nil {
			cmds = append(cmds, cmd)
		}
		s.sshPortInput, cmd = s.sshPortInput.Update(msg)
		if cmd != nil {
			cmds = append(cmds, cmd)
		}
		s.keyPathInput, cmd = s.keyPathInput.Update(msg)
		if cmd != nil {
			cmds = append(cmds, cmd)
		}
		s.keyPassphraseInput, cmd = s.keyPassphraseInput.Update(msg)
		if cmd != nil {
			cmds = append(cmds, cmd)
		}
		s.passwordInput, cmd = s.passwordInput.Update(msg)
		if cmd != nil {
			cmds = append(cmds, cmd)
		}
	}

	return s, tea.Batch(cmds...)
}

func (s *SSHCreds) nextField() {
	s.blurCurrentField()
	switch s.activeField {
	case fieldUser:
		s.activeField = fieldSSHPort
	case fieldSSHPort:
		s.activeField = fieldAuthMethod
	case fieldAuthMethod:
		switch s.auth {
		case authKeyFile:
			s.activeField = fieldKeyPath
		case authPassword:
			s.activeField = fieldPassword
		default:
			// SSH Agent — no extra fields, stay on auth method
		}
	case fieldKeyPath:
		s.activeField = fieldKeyPassphrase
	case fieldKeyPassphrase, fieldPassword:
		// At the end
	}
	s.focusCurrentField()
}

func (s *SSHCreds) prevField() {
	s.blurCurrentField()
	switch s.activeField {
	case fieldSSHPort:
		s.activeField = fieldUser
	case fieldAuthMethod:
		s.activeField = fieldSSHPort
	case fieldKeyPath, fieldPassword:
		s.activeField = fieldAuthMethod
	case fieldKeyPassphrase:
		s.activeField = fieldKeyPath
	}
	s.focusCurrentField()
}

func (s *SSHCreds) focusCurrentField() {
	switch s.activeField {
	case fieldUser:
		s.userInput.Focus()
	case fieldSSHPort:
		s.sshPortInput.Focus()
	case fieldKeyPath:
		s.keyPathInput.Focus()
	case fieldKeyPassphrase:
		s.keyPassphraseInput.Focus()
	case fieldPassword:
		s.passwordInput.Focus()
	}
}

func (s *SSHCreds) blurCurrentField() {
	s.userInput.Blur()
	s.sshPortInput.Blur()
	s.keyPathInput.Blur()
	s.keyPassphraseInput.Blur()
	s.passwordInput.Blur()
}

func (s *SSHCreds) submit() tea.Cmd {
	if s.userInput.Value() == "" {
		s.err = "Username is required"
		return nil
	}

	// Validate SSH port
	sshPort := 22
	if s.sshPortInput.Value() != "" {
		p, err := strconv.Atoi(s.sshPortInput.Value())
		if err != nil || p < 1 || p > 65535 {
			s.err = "SSH port must be between 1-65535"
			return nil
		}
		sshPort = p
	}

	s.err = ""

	sshKey := ""
	passphrase := ""
	password := ""
	switch s.auth {
	case authKeyFile:
		sshKey = s.keyPathInput.Value()
		passphrase = s.keyPassphraseInput.Value()
	case authPassword:
		password = s.passwordInput.Value()
	}

	return Navigate(NewBootstrap(s.cfg, s.version, s.host, s.label, s.connectionType, s.userInput.Value(), sshKey, passphrase, password, sshPort, s.peerTags))
}

func (s *SSHCreds) View() string {
	displayName := s.label
	if s.label != s.host {
		displayName = fmt.Sprintf("%s (%s)", s.label, s.host)
	}
	title := lipgloss.NewStyle().Foreground(theme.Accent).Bold(true).
		Render(fmt.Sprintf("SSH Credentials — %s", displayName))

	// Responsive width for text inputs
	inputWidth := 40
	if s.width > 0 && s.width < 65 {
		inputWidth = s.width - 25 // Account for padding and borders
		if inputWidth < 25 {
			inputWidth = 25
		}
	}

	// Set width for all text inputs
	s.userInput.Width = inputWidth
	s.sshPortInput.Width = inputWidth
	s.keyPathInput.Width = inputWidth
	s.keyPassphraseInput.Width = inputWidth
	s.passwordInput.Width = inputWidth

	// User field
	userLabel := s.fieldLabel("Username:", fieldUser)
	userInput := s.userInput.View()

	// SSH Port field
	portLabel := s.fieldLabel("SSH Port:", fieldSSHPort)
	portInput := s.sshPortInput.View()

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
		keyInput := s.keyPathInput.View()
		extraField = keyLabel + "\n" + keyInput
		// Passphrase field (optional)
		ppLabel := s.fieldLabel("Key Passphrase:", fieldKeyPassphrase)
		ppInput := s.keyPassphraseInput.View()
		ppHint := theme.MutedStyle.Render("  Leave empty if key is not encrypted")
		extraField += "\n" + ppLabel + "\n" + ppInput + "\n" + ppHint
	case authPassword:
		pwLabel := s.fieldLabel("Password:", fieldPassword)
		pwInput := s.passwordInput.View()
		extraField = pwLabel + "\n" + pwInput
	case authSSHAgent:
		extraField = theme.MutedStyle.Render("  Will use SSH_AUTH_SOCK agent")
	}

	var errLine string
	if s.err != "" {
		errLine = "\n" + theme.CritStyle.Render(s.err)
	}

	body := fmt.Sprintf("%s\n%s\n%s\n%s\n\n%s\n%s\n\n%s%s",
		userLabel, userInput,
		portLabel, portInput,
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

func (s *SSHCreds) Name() string { return "SSH Credentials" }

func (s *SSHCreds) KeyBinds() []KeyBind {
	return []KeyBind{
		{Key: "Tab/↕", Help: "next field"},
		{Key: "←/→", Help: "auth method"},
		{Key: "Enter", Help: "connect"},
		{Key: "Esc", Help: "back"},
	}
}

func (s *SSHCreds) InputActive() bool {
	return true
}
