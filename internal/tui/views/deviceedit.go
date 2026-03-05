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

type deviceEditField int

const (
	fieldLabel deviceEditField = iota
	fieldHost
	fieldSSHUser
	fieldSSHKey
	fieldEditSSHPort
	fieldAgentPort
	fieldConnectionType
)

var connectionTypes = []string{"local", "tailscale", "manual"}

// DeviceEdit allows editing device details.
type DeviceEdit struct {
	cfg            *config.Config
	version        string
	device         config.Device
	activeField    deviceEditField
	connTypeIdx    int
	err            string
	width          int
	height         int
	labelInput     textinput.Model
	hostInput      textinput.Model
	sshUserInput   textinput.Model
	sshKeyInput    textinput.Model
	sshPortInput   textinput.Model
	agentPortInput textinput.Model
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

	// Ensure SSHPort has a display value
	if device.SSHPort <= 0 {
		device.SSHPort = 22
	}

	// Initialize text inputs
	labelInput := components.NewTextField("Device label")
	labelInput.SetValue(device.Label)
	labelInput.Focus()

	hostInput := components.NewTextField("hostname or IP address")
	hostInput.SetValue(device.Host)

	sshUserInput := components.NewTextField("SSH username")
	sshUserInput.SetValue(device.SSHUser)

	sshKeyInput := components.NewTextField("path to SSH private key")
	sshKeyInput.SetValue(device.SSHKey)

	sshPortInput := components.NewPortField(strconv.Itoa(device.SSHPort))

	agentPortInput := components.NewPortField(strconv.Itoa(device.AgentPort))

	return &DeviceEdit{
		cfg:            cfg,
		version:        version,
		device:         device,
		activeField:    fieldLabel,
		connTypeIdx:    connTypeIdx,
		width:          80, // default width
		labelInput:     labelInput,
		hostInput:      hostInput,
		sshUserInput:   sshUserInput,
		sshKeyInput:    sshKeyInput,
		sshPortInput:   sshPortInput,
		agentPortInput: agentPortInput,
	}
}

func (de *DeviceEdit) Init() tea.Cmd {
	return nil
}

func (de *DeviceEdit) Update(msg tea.Msg) (View, tea.Cmd) {
	var cmds []tea.Cmd

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		de.width = msg.Width
		if de.width > theme.MaxContentWidth-2*theme.ContentPadding {
			de.width = theme.MaxContentWidth - 2*theme.ContentPadding
		}
		de.height = msg.Height

		// Update textinput widths based on available space
		inputWidth := de.width - 20 // leave space for borders and padding
		if inputWidth > 40 {
			inputWidth = 40
		}
		if inputWidth < 20 {
			inputWidth = 20
		}

		de.labelInput.Width = inputWidth
		de.hostInput.Width = inputWidth
		de.sshUserInput.Width = inputWidth
		de.sshKeyInput.Width = inputWidth

	case tea.KeyMsg:
		switch msg.String() {
		case "tab", "down":
			de.nextField()
		case "shift+tab", "up":
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
			} else {
				// Forward to active textinput for cursor movement
				cmd := de.updateActiveTextInput(msg)
				cmds = append(cmds, cmd)
			}
		case "right":
			if de.activeField == fieldConnectionType && de.connTypeIdx < len(connectionTypes)-1 {
				de.connTypeIdx++
				de.device.ConnectionType = connectionTypes[de.connTypeIdx]
			} else {
				// Forward to active textinput for cursor movement
				cmd := de.updateActiveTextInput(msg)
				cmds = append(cmds, cmd)
			}
		default:
			// Forward all other key messages to the active textinput
			cmd := de.updateActiveTextInput(msg)
			cmds = append(cmds, cmd)
		}
	}

	return de, tea.Batch(cmds...)
}

func (de *DeviceEdit) updateActiveTextInput(msg tea.Msg) tea.Cmd {
	var cmd tea.Cmd
	switch de.activeField {
	case fieldLabel:
		de.labelInput, cmd = de.labelInput.Update(msg)
	case fieldHost:
		de.hostInput, cmd = de.hostInput.Update(msg)
	case fieldSSHUser:
		de.sshUserInput, cmd = de.sshUserInput.Update(msg)
	case fieldSSHKey:
		de.sshKeyInput, cmd = de.sshKeyInput.Update(msg)
	case fieldEditSSHPort:
		de.sshPortInput, cmd = de.sshPortInput.Update(msg)
	case fieldAgentPort:
		de.agentPortInput, cmd = de.agentPortInput.Update(msg)
	}
	return cmd
}

func (de *DeviceEdit) nextField() {
	// Blur current field
	de.blurActiveField()

	// Move to next field
	if de.activeField < fieldConnectionType {
		de.activeField++
	} else {
		de.activeField = fieldLabel
	}

	// Focus new field
	de.focusActiveField()
}

func (de *DeviceEdit) prevField() {
	// Blur current field
	de.blurActiveField()

	// Move to previous field
	if de.activeField > fieldLabel {
		de.activeField--
	} else {
		de.activeField = fieldConnectionType
	}

	// Focus new field
	de.focusActiveField()
}

func (de *DeviceEdit) focusActiveField() {
	switch de.activeField {
	case fieldLabel:
		de.labelInput.Focus()
	case fieldHost:
		de.hostInput.Focus()
	case fieldSSHUser:
		de.sshUserInput.Focus()
	case fieldSSHKey:
		de.sshKeyInput.Focus()
	case fieldEditSSHPort:
		de.sshPortInput.Focus()
	case fieldAgentPort:
		de.agentPortInput.Focus()
	case fieldConnectionType:
		// Connection type doesn't use textinput, no focus needed
	}
}

func (de *DeviceEdit) blurActiveField() {
	switch de.activeField {
	case fieldLabel:
		de.labelInput.Blur()
	case fieldHost:
		de.hostInput.Blur()
	case fieldSSHUser:
		de.sshUserInput.Blur()
	case fieldSSHKey:
		de.sshKeyInput.Blur()
	case fieldEditSSHPort:
		de.sshPortInput.Blur()
	case fieldAgentPort:
		de.agentPortInput.Blur()
	case fieldConnectionType:
		// Connection type doesn't use textinput, no blur needed
	}
}

func (de *DeviceEdit) submit() tea.Cmd {
	// Read values from textinputs
	label := de.labelInput.Value()
	host := de.hostInput.Value()
	sshUser := de.sshUserInput.Value()
	sshKey := de.sshKeyInput.Value()
	sshPortStr := de.sshPortInput.Value()
	agentPortStr := de.agentPortInput.Value()

	// Validation
	if label == "" {
		de.err = "Label is required"
		return nil
	}
	if host == "" {
		de.err = "Host is required"
		return nil
	}

	// Parse ports
	sshPort := 22
	if sshPortStr != "" {
		if port, err := strconv.Atoi(sshPortStr); err != nil {
			de.err = "SSH port must be a number"
			return nil
		} else if port <= 0 || port > 65535 {
			de.err = "SSH port must be between 1-65535"
			return nil
		} else {
			sshPort = port
		}
	}

	agentPort := 0
	if agentPortStr != "" {
		if port, err := strconv.Atoi(agentPortStr); err != nil {
			de.err = "Agent port must be a number"
			return nil
		} else if port <= 0 || port > 65535 {
			de.err = "Agent port must be between 1-65535"
			return nil
		} else {
			agentPort = port
		}
	}
	if agentPort == 0 {
		de.err = "Agent port is required"
		return nil
	}

	de.err = ""

	// Update device with new values
	de.device.Label = label
	de.device.Host = host
	de.device.SSHUser = sshUser
	de.device.SSHKey = sshKey
	de.device.SSHPort = sshPort
	de.device.AgentPort = agentPort

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

// InputActive returns true since this view has active text inputs
func (de *DeviceEdit) InputActive() bool {
	return true
}

func (de *DeviceEdit) View() string {
	title := lipgloss.NewStyle().Foreground(theme.Accent).Bold(true).
		Render(fmt.Sprintf("Edit Device — %s", de.device.ID))

	// Form fields
	labelField := de.renderField("Label:", de.labelInput.View(), fieldLabel)
	hostField := de.renderField("Host/IP:", de.hostInput.View(), fieldHost)
	userField := de.renderField("SSH User:", de.sshUserInput.View(), fieldSSHUser)
	keyField := de.renderField("SSH Key Path:", de.sshKeyInput.View(), fieldSSHKey)
	sshPortField := de.renderField("SSH Port:", de.sshPortInput.View(), fieldEditSSHPort)
	portField := de.renderField("Agent Port:", de.agentPortInput.View(), fieldAgentPort)

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

	body := fmt.Sprintf("%s\n%s\n%s\n%s\n%s\n%s\n%s%s",
		labelField, hostField, userField, keyField, sshPortField, portField,
		connLabel+"\n"+connChoices, errLine)

	// Calculate responsive card width
	cardWidth := de.width - 8 // leave some margin
	if cardWidth > 80 {
		cardWidth = 80
	}
	if cardWidth < 50 {
		cardWidth = 50
	}

	card := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(theme.Border).
		Padding(1, 2).
		Width(cardWidth).
		Render(title + "\n\n" + body)

	return lipgloss.NewStyle().Padding(2, 0).Render(card)
}

func (de *DeviceEdit) renderField(label, fieldView string, field deviceEditField) string {
	fieldLabel := de.fieldLabel(label, field)
	return fieldLabel + "\n" + fieldView
}

func (de *DeviceEdit) fieldLabel(label string, field deviceEditField) string {
	style := theme.MutedStyle
	if de.activeField == field {
		style = lipgloss.NewStyle().Foreground(theme.Accent)
	}
	return style.Render(label)
}

func (de *DeviceEdit) KeyBinds() []KeyBind {
	return []KeyBind{
		{Key: "Tab", Help: "next field"},
		{Key: "←/→", Help: "connection type"},
		{Key: "Enter", Help: "save"},
		{Key: "Esc", Help: "cancel"},
	}
}
