package views

import (
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/spencerbull/yokai/internal/config"
	sshpkg "github.com/spencerbull/yokai/internal/ssh"
	"github.com/spencerbull/yokai/internal/tui/components"
	"github.com/spencerbull/yokai/internal/tui/theme"
)

// hasSSHConfig returns true if ~/.ssh/config exists and has non-wildcard hosts.
func hasSSHConfig() bool {
	return len(sshpkg.DiscoverSSHHosts()) > 0
}

type connectionTestResult struct {
	deviceID string
	online   bool
	version  string
	err      error
}

type upgradeResultMsg struct {
	deviceID string
	err      error
}

// DeviceManager shows all registered devices with management options.
type DeviceManager struct {
	cfg            *config.Config
	version        string
	cursor         int
	testResults    map[string]connectionTestResult
	testing        map[string]bool
	upgrading      map[string]bool
	upgradeResults map[string]*upgradeResultMsg
	width          int
	height         int
}

// NewDeviceManager creates the device manager view.
func NewDeviceManager(cfg *config.Config, version string) *DeviceManager {
	return &DeviceManager{
		cfg:            cfg,
		version:        version,
		testResults:    make(map[string]connectionTestResult),
		testing:        make(map[string]bool),
		upgrading:      make(map[string]bool),
		upgradeResults: make(map[string]*upgradeResultMsg),
	}
}

func (dm *DeviceManager) Init() tea.Cmd {
	return nil
}

func (dm *DeviceManager) Update(msg tea.Msg) (View, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		dm.width = msg.Width
		if dm.width > theme.MaxContentWidth-2*theme.ContentPadding {
			dm.width = theme.MaxContentWidth - 2*theme.ContentPadding
		}
		dm.height = msg.Height

	case connectionTestResult:
		dm.testResults[msg.deviceID] = msg
		delete(dm.testing, msg.deviceID)
		if msg.err != nil {
			return dm, ShowToast("Connection failed: "+msg.err.Error(), ToastError)
		}
		if msg.online {
			return dm, ShowToast("Device online", ToastSuccess)
		}
		return dm, nil

	case upgradeResultMsg:
		delete(dm.upgrading, msg.deviceID)
		dm.upgradeResults[msg.deviceID] = &msg
		if msg.err != nil {
			return dm, ShowToast("Upgrade failed: "+msg.err.Error(), ToastError)
		}
		return dm, ShowToast("Agent upgraded successfully", ToastSuccess)

	case tea.KeyMsg:
		switch msg.String() {
		case "up", "k":
			if dm.cursor > 0 {
				dm.cursor--
			}
		case "down", "j":
			if dm.cursor < len(dm.cfg.Devices)-1 {
				dm.cursor++
			}
		case "a":
			if hasSSHConfig() {
				return dm, Navigate(NewSSHConfigPicker(dm.cfg, dm.version))
			}
			return dm, Navigate(NewWelcome(dm.cfg, dm.version))
		case "e":
			if len(dm.cfg.Devices) > 0 {
				device := dm.cfg.Devices[dm.cursor]
				return dm, Navigate(NewDeviceEdit(dm.cfg, dm.version, device))
			}
		case "t":
			if len(dm.cfg.Devices) > 0 && dm.cursor < len(dm.cfg.Devices) {
				device := dm.cfg.Devices[dm.cursor]
				if !dm.testing[device.ID] {
					dm.testing[device.ID] = true
					return dm, dm.testConnection(device)
				}
			}
		case "u":
			// Upgrade selected device agent
			if len(dm.cfg.Devices) > 0 && dm.cursor < len(dm.cfg.Devices) {
				device := dm.cfg.Devices[dm.cursor]
				if !dm.upgrading[device.ID] {
					dm.upgrading[device.ID] = true
					delete(dm.upgradeResults, device.ID)
					return dm, dm.upgradeDevice(device)
				}
			}
		case "T":
			// Test ALL device connections
			var cmds []tea.Cmd
			for _, device := range dm.cfg.Devices {
				if !dm.testing[device.ID] {
					dm.testing[device.ID] = true
					cmds = append(cmds, dm.testConnection(device))
				}
			}
			if len(cmds) > 0 {
				return dm, tea.Batch(cmds...)
			}
		case "U":
			// Upgrade ALL device agents
			var cmds []tea.Cmd
			for _, device := range dm.cfg.Devices {
				if !dm.upgrading[device.ID] {
					dm.upgrading[device.ID] = true
					delete(dm.upgradeResults, device.ID)
					cmds = append(cmds, dm.upgradeDevice(device))
				}
			}
			if len(cmds) > 0 {
				return dm, tea.Batch(cmds...)
			}
		case "x":
			if len(dm.cfg.Devices) > 0 {
				device := dm.cfg.Devices[dm.cursor]
				onConfirm := func() tea.Msg {
					dm.cfg.RemoveDevice(device.ID)
					_ = config.Save(dm.cfg)
					if dm.cursor >= len(dm.cfg.Devices) && dm.cursor > 0 {
						dm.cursor--
					}
					return nil
				}
				msg := fmt.Sprintf("Remove device %q? This cannot be undone.", device.Label)
				onConfirmWithToast := func() tea.Msg {
					onConfirm()
					return components.ShowToastMsg{Message: "Device removed", Level: ToastSuccess}
				}
				return dm, Navigate(NewConfirmView(msg, onConfirmWithToast, nil))
			}
		case "esc":
			return dm, PopView()
		}
	}
	return dm, nil
}

func (dm *DeviceManager) testConnection(device config.Device) tea.Cmd {
	return func() tea.Msg {
		// First test SSH connectivity
		client, err := sshpkg.Connect(sshpkg.ClientConfig{
			Host:    device.Host,
			Port:    fmt.Sprintf("%d", device.SSHPortOrDefault()),
			User:    device.SSHUser,
			KeyPath: device.SSHKey,
		})
		if err != nil {
			return connectionTestResult{
				deviceID: device.ID,
				online:   false,
				err:      fmt.Errorf("SSH connection failed: %w", err),
			}
		}
		defer func() {
			_ = client.Close() // Best-effort SSH client close after test.
		}()

		// Test agent health endpoint
		healthURL := fmt.Sprintf("http://%s:%d/health", device.Host, device.AgentPort)
		httpClient := &http.Client{Timeout: 5 * time.Second}

		resp, err := httpClient.Get(healthURL)
		if err != nil {
			return connectionTestResult{
				deviceID: device.ID,
				online:   true, // SSH worked but agent is down
				err:      fmt.Errorf("agent not responding: %w", err),
			}
		}
		defer func() {
			_ = resp.Body.Close() // Best-effort close of health response body.
		}()

		if resp.StatusCode == http.StatusOK {
			return connectionTestResult{
				deviceID: device.ID,
				online:   true,
				version:  "unknown", // Would need to parse response for actual version
			}
		}

		return connectionTestResult{
			deviceID: device.ID,
			online:   true,
			err:      fmt.Errorf("agent returned status %d", resp.StatusCode),
		}
	}
}

func (dm *DeviceManager) upgradeDevice(device config.Device) tea.Cmd {
	return func() tea.Msg {
		// Resolve the local binary path
		localBinary, err := os.Executable()
		if err != nil {
			return upgradeResultMsg{deviceID: device.ID, err: fmt.Errorf("cannot find local binary: %w", err)}
		}
		localBinary, err = filepath.EvalSymlinks(localBinary)
		if err != nil {
			return upgradeResultMsg{deviceID: device.ID, err: fmt.Errorf("resolving binary path: %w", err)}
		}

		// Connect via SSH
		client, err := sshpkg.Connect(sshpkg.ClientConfig{
			Host:    device.Host,
			Port:    fmt.Sprintf("%d", device.SSHPortOrDefault()),
			User:    device.SSHUser,
			KeyPath: device.SSHKey,
		})
		if err != nil {
			return upgradeResultMsg{deviceID: device.ID, err: fmt.Errorf("SSH: %w", err)}
		}
		defer func() {
			_ = client.Close()
		}()

		// Run the upgrade
		if err := sshpkg.UpgradeAgent(client, localBinary, device.AgentPort); err != nil {
			return upgradeResultMsg{deviceID: device.ID, err: err}
		}

		return upgradeResultMsg{deviceID: device.ID}
	}
}

func (dm *DeviceManager) View() string {
	title := lipgloss.NewStyle().Foreground(theme.Accent).Bold(true).Render("Device Manager")

	if len(dm.cfg.Devices) == 0 {
		body := theme.MutedStyle.Render("No devices registered.\nPress 'a' to add a device.")
		// Responsive width for empty state
		cardWidth := 55
		if dm.width > 0 && dm.width < 65 {
			cardWidth = dm.width - 10
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
		return lipgloss.NewStyle().Padding(1, 0).Render(card)
	}

	// Device list
	var list string
	for i, dev := range dm.cfg.Devices {
		cursor := "  "
		style := theme.PrimaryStyle
		if i == dm.cursor {
			cursor = "> "
			style = lipgloss.NewStyle().Foreground(theme.Accent).Bold(true)
		}

		// Determine status based on test/upgrade results
		var status string
		if dm.upgrading[dev.ID] {
			status = theme.StatusLoading()
		} else if dm.testing[dev.ID] {
			status = theme.StatusLoading()
		} else if ur, exists := dm.upgradeResults[dev.ID]; exists && ur.err == nil {
			status = theme.GoodStyle.Render("↑") // upgraded
		} else if ur, exists := dm.upgradeResults[dev.ID]; exists && ur.err != nil {
			status = theme.CritStyle.Render("!")
		} else if result, exists := dm.testResults[dev.ID]; exists {
			if result.err != nil {
				status = theme.StatusOffline()
			} else if result.online {
				status = theme.StatusOnline()
			} else {
				status = theme.StatusOffline()
			}
		} else {
			status = theme.MutedStyle.Render("○") // untested
		}

		gpu := dev.GPUType
		if gpu == "" {
			gpu = "unknown"
		}
		list += fmt.Sprintf("%s%s %s  %s  %s  %s\n",
			cursor, status,
			style.Render(dev.Label),
			theme.MutedStyle.Render(dev.Host),
			theme.MutedStyle.Render(dev.ConnectionType),
			theme.MutedStyle.Render(gpu),
		)
	}

	// Detail card for selected device
	var detail string
	if dm.cursor < len(dm.cfg.Devices) {
		dev := dm.cfg.Devices[dm.cursor]
		detail = "\n" + theme.MutedStyle.Render("── Details ──") + "\n"
		detail += fmt.Sprintf("  ID:         %s\n", dev.ID)
		detail += fmt.Sprintf("  Host:       %s\n", dev.Host)
		detail += fmt.Sprintf("  SSH User:   %s\n", dev.SSHUser)
		detail += fmt.Sprintf("  Connection: %s\n", dev.ConnectionType)
		detail += fmt.Sprintf("  Agent Port: %d\n", dev.AgentPort)
		detail += fmt.Sprintf("  GPU Type:   %s\n", dev.GPUType)

		// Show connection test result if available
		if dm.testing[dev.ID] {
			detail += "\n" + theme.MutedStyle.Render("  Status: Testing...")
		} else if result, exists := dm.testResults[dev.ID]; exists {
			if result.err != nil {
				detail += "\n" + theme.CritStyle.Render("  Status: "+result.err.Error())
			} else if result.online {
				statusStr := "Online"
				if result.version != "" {
					statusStr += " (v" + result.version + ")"
				}
				detail += "\n" + theme.GoodStyle.Render("  Status: "+statusStr)
			}
		} else {
			detail += "\n" + theme.MutedStyle.Render("  Status: Not tested (press 't')")
		}

		// Show upgrade status
		if dm.upgrading[dev.ID] {
			detail += "\n" + theme.WarnStyle.Render("  Upgrading agent...")
		} else if ur, exists := dm.upgradeResults[dev.ID]; exists {
			if ur.err != nil {
				detail += "\n" + theme.CritStyle.Render("  Upgrade failed: "+ur.err.Error())
			} else {
				detail += "\n" + theme.GoodStyle.Render("  Agent upgraded successfully")
			}
		}
	}

	// Responsive width for main view
	cardWidth := 65
	if dm.width > 0 && dm.width < 75 {
		cardWidth = dm.width - 10
		if cardWidth < 45 {
			cardWidth = 45
		}
	}

	card := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(theme.Border).
		Padding(1, 2).
		Width(cardWidth).
		Render(title + "\n\n" + list + detail)

	return lipgloss.NewStyle().Padding(1, 0).Render(card)
}

func (dm *DeviceManager) InputActive() bool { return false }

func (dm *DeviceManager) Name() string { return "Devices" }

func (dm *DeviceManager) KeyBinds() []KeyBind {
	return []KeyBind{
		{Key: "↑/↓", Help: "navigate"},
		{Key: "a", Help: "add device"},
		{Key: "e", Help: "edit"},
		{Key: "t", Help: "test connection"},
		{Key: "T", Help: "test all"},
		{Key: "u", Help: "upgrade agent"},
		{Key: "U", Help: "upgrade all"},
		{Key: "x", Help: "remove"},
		{Key: "Esc", Help: "back"},
	}
}
