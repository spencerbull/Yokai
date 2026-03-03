package views

import (
	"fmt"
	"strconv"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/spencerbull/yokai/internal/config"
	"github.com/spencerbull/yokai/internal/tui/theme"
)

type deviceEditField int

const (
	fieldLabel deviceEditField = iota
	fieldHost
	fieldSSHUser
	fieldSSHKey
	fieldAgentPort
	fieldConnectionType
)

var connectionTypes = []string{"local", "tailscale", "manual"}

// DeviceEdit allows editing device details.
type DeviceEdit struct {
	cfg         *config.Config
	version     string
	device      config.Device
	activeField deviceEditField
	connTypeIdx int
	err         string
}

// NewDeviceEdit creates the device edit view.
func NewDeviceEdit(cfg *config.Config, version string, device config.Device) *DeviceEdit {
	// Find current connection type index
	connTypeIdx := 0
	for i, ct := range connectionTypes {
		if ct == device.ConnectionType {
			connTypeIdx = i
			break
		}
	}

	return &DeviceEdit{
		cfg:         cfg,
		version:     version,
		device:      device,
		activeField: fieldLabel,
		connTypeIdx: connTypeIdx,
	}
}

func (de *DeviceEdit) Init() tea.Cmd {
	return nil
}

func (de *DeviceEdit) Update(msg tea.Msg) (View, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "tab":
			de.nextField()
		case "shift+tab":
			de.prevField()
		case "enter":
			if de.activeField == fieldConnectionType {
				de.nextField()
				return de, nil
			}
			return de, de.submit()
		case "esc":
			return de, PopView()
		case "left":
			if de.activeField == fieldConnectionType && de.connTypeIdx > 0 {
				de.connTypeIdx--
				de.device.ConnectionType = connectionTypes[de.connTypeIdx]
			}
		case "right":
			if de.activeField == fieldConnectionType && de.connTypeIdx < len(connectionTypes)-1 {
				de.connTypeIdx++
				de.device.ConnectionType = connectionTypes[de.connTypeIdx]
			}
		case "backspace":
			de.handleBackspace()
		default:
			de.handleChar(msg.String())
		}
	}
	return de, nil
}

func (de *DeviceEdit) nextField() {
	if de.activeField < fieldConnectionType {
		de.activeField++
	}
}

func (de *DeviceEdit) prevField() {
	if de.activeField > fieldLabel {
		de.activeField--
	}
}

func (de *DeviceEdit) handleBackspace() {
	switch de.activeField {
	case fieldLabel:
		if len(de.device.Label) > 0 {
			de.device.Label = de.device.Label[:len(de.device.Label)-1]
		}
	case fieldHost:
		if len(de.device.Host) > 0 {
			de.device.Host = de.device.Host[:len(de.device.Host)-1]
		}
	case fieldSSHUser:
		if len(de.device.SSHUser) > 0 {
			de.device.SSHUser = de.device.SSHUser[:len(de.device.SSHUser)-1]
		}
	case fieldSSHKey:
		if len(de.device.SSHKey) > 0 {
			de.device.SSHKey = de.device.SSHKey[:len(de.device.SSHKey)-1]
		}
	case fieldAgentPort:
		portStr := strconv.Itoa(de.device.AgentPort)
		if len(portStr) > 0 {
			portStr = portStr[:len(portStr)-1]
			if portStr == "" {
				de.device.AgentPort = 0
			} else if port, err := strconv.Atoi(portStr); err == nil {
				de.device.AgentPort = port
			}
		}
	}
}

func (de *DeviceEdit) handleChar(ch string) {
	if len(ch) != 1 {
		return
	}
	switch de.activeField {
	case fieldLabel:
		de.device.Label += ch
	case fieldHost:
		de.device.Host += ch
	case fieldSSHUser:
		de.device.SSHUser += ch
	case fieldSSHKey:
		de.device.SSHKey += ch
	case fieldAgentPort:
		// Only allow digits for port
		if ch >= "0" && ch <= "9" {
			portStr := strconv.Itoa(de.device.AgentPort) + ch
			if port, err := strconv.Atoi(portStr); err == nil && port <= 65535 {
				de.device.AgentPort = port
			}
		}
	}
}

func (de *DeviceEdit) submit() tea.Cmd {
	if de.device.Label == "" {
		de.err = "Label is required"
		return nil
	}
	if de.device.Host == "" {
		de.err = "Host is required"
		return nil
	}
	if de.device.AgentPort <= 0 || de.device.AgentPort > 65535 {
		de.err = "Agent port must be between 1-65535"
		return nil
	}

	de.err = ""

	// Update the device in config
	for i := range de.cfg.Devices {
		if de.cfg.Devices[i].ID == de.device.ID {
			de.cfg.Devices[i] = de.device
			break
		}
	}

	_ = config.Save(de.cfg)
	return PopView()
}

func (de *DeviceEdit) View() string {
	title := lipgloss.NewStyle().Foreground(theme.Accent).Bold(true).
		Render(fmt.Sprintf("Edit Device — %s", de.device.ID))

	// Form fields
	labelField := de.renderField("Label:", de.device.Label, fieldLabel)
	hostField := de.renderField("Host/IP:", de.device.Host, fieldHost)
	userField := de.renderField("SSH User:", de.device.SSHUser, fieldSSHUser)
	keyField := de.renderField("SSH Key Path:", de.device.SSHKey, fieldSSHKey)
	portField := de.renderField("Agent Port:", strconv.Itoa(de.device.AgentPort), fieldAgentPort)

	// Connection type selector
	connLabel := de.fieldLabel("Connection Type:", fieldConnectionType)
	var connChoices string
	for i, ct := range connectionTypes {
		if i == de.connTypeIdx {
			connChoices += lipgloss.NewStyle().Foreground(theme.Accent).Bold(true).Render("[" + ct + "]")
		} else {
			connChoices += theme.MutedStyle.Render(" " + ct + " ")
		}
		if i < len(connectionTypes)-1 {
			connChoices += "  "
		}
	}

	var errLine string
	if de.err != "" {
		errLine = "\n" + theme.CritStyle.Render(de.err)
	}

	body := fmt.Sprintf("%s\n%s\n%s\n%s\n%s\n%s%s",
		labelField, hostField, userField, keyField, portField,
		connLabel+"\n"+connChoices, errLine)

	card := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(theme.Border).
		Padding(1, 2).
		Width(60).
		Render(title + "\n\n" + body)

	return lipgloss.NewStyle().Padding(2, 0).Render(card)
}

func (de *DeviceEdit) renderField(label, value string, field deviceEditField) string {
	fieldLabel := de.fieldLabel(label, field)
	fieldInput := de.fieldInput(value, field)
	return fieldLabel + "\n" + fieldInput
}

func (de *DeviceEdit) fieldLabel(label string, field deviceEditField) string {
	style := theme.MutedStyle
	if de.activeField == field {
		style = lipgloss.NewStyle().Foreground(theme.Accent)
	}
	return style.Render(label)
}

func (de *DeviceEdit) fieldInput(value string, field deviceEditField) string {
	borderColor := theme.Border
	if de.activeField == field {
		borderColor = theme.Accent
	}
	display := value
	if de.activeField == field {
		display += "█"
	}
	return lipgloss.NewStyle().
		Border(lipgloss.NormalBorder()).
		BorderForeground(borderColor).
		Padding(0, 1).
		Width(40).
		Render(display)
}

func (de *DeviceEdit) KeyBinds() []KeyBind {
	return []KeyBind{
		{Key: "Tab", Help: "next field"},
		{Key: "←/→", Help: "connection type"},
		{Key: "Enter", Help: "save"},
		{Key: "Esc", Help: "cancel"},
	}
}
