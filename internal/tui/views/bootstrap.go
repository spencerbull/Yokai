package views

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/spencerbull/yokai/internal/config"
	"github.com/spencerbull/yokai/internal/docker"
	sshpkg "github.com/spencerbull/yokai/internal/ssh"
	"github.com/spencerbull/yokai/internal/tui/theme"
)

type bootstrapStep int

const (
	bsConnecting bootstrapStep = iota
	bsPreflight
	bsDeployingAgent
	bsMonitoringPrompt
	bsDeployingMonitoring
	bsDone
	bsFailed
)

// Bootstrap handles device bootstrapping (SSH connect, pre-flight, agent deploy).
type Bootstrap struct {
	cfg            *config.Config
	version        string
	host           string
	connectionType string
	sshUser        string
	sshKey         string
	sshPassword    string

	step       bootstrapStep
	err        string
	preflight  *sshpkg.PreflightResult
	agentToken string
	width      int
	height     int
}

type bootstrapProgressMsg struct {
	step       bootstrapStep
	err        error
	preflight  *sshpkg.PreflightResult
	agentToken string
}

// NewBootstrap creates the bootstrap view.
func NewBootstrap(cfg *config.Config, version string, host, connType, user, keyPath, password string) *Bootstrap {
	return &Bootstrap{
		cfg:            cfg,
		version:        version,
		host:           host,
		connectionType: connType,
		sshUser:        user,
		sshKey:         keyPath,
		sshPassword:    password,
		step:           bsConnecting,
	}
}

func (b *Bootstrap) Init() tea.Cmd {
	return b.runBootstrap()
}

func (b *Bootstrap) runBootstrap() tea.Cmd {
	return func() tea.Msg {
		// Step 1: Connect via SSH
		client, err := sshpkg.Connect(sshpkg.ClientConfig{
			Host:     b.host,
			User:     b.sshUser,
			KeyPath:  b.sshKey,
			Password: b.sshPassword,
		})
		if err != nil {
			return bootstrapProgressMsg{step: bsFailed, err: fmt.Errorf("SSH connect: %w", err)}
		}
		defer func() {
			_ = client.Close() // Best-effort SSH client close after bootstrap.
		}()

		// Step 2: Pre-flight checks
		pf, err := sshpkg.Preflight(client)
		if err != nil {
			return bootstrapProgressMsg{step: bsFailed, err: fmt.Errorf("pre-flight: %w", err)}
		}

		if !pf.DockerInstalled {
			return bootstrapProgressMsg{
				step:      bsFailed,
				err:       fmt.Errorf("docker not installed on %s", b.host),
				preflight: pf,
			}
		}

		// Step 3: Generate agent token and deploy agent
		tokenBytes := make([]byte, 32)
		if _, err := rand.Read(tokenBytes); err != nil {
			return bootstrapProgressMsg{step: bsFailed, err: fmt.Errorf("generating token: %w", err)}
		}
		agentToken := hex.EncodeToString(tokenBytes)

		// Get current binary path for deployment
		binaryPath, err := os.Executable()
		if err != nil {
			return bootstrapProgressMsg{step: bsFailed, err: fmt.Errorf("getting binary path: %w", err)}
		}

		// Deploy the agent
		if err := sshpkg.DeployAgent(client, binaryPath, agentToken); err != nil {
			return bootstrapProgressMsg{step: bsFailed, err: fmt.Errorf("deploying agent: %w", err)}
		}

		return bootstrapProgressMsg{
			step:       bsMonitoringPrompt,
			preflight:  pf,
			agentToken: agentToken,
		}
	}
}

func (b *Bootstrap) runMonitoringDeploy() tea.Cmd {
	return func() tea.Msg {
		// Connect to SSH for monitoring deployment
		client, err := sshpkg.Connect(sshpkg.ClientConfig{
			Host:     b.host,
			User:     b.sshUser,
			KeyPath:  b.sshKey,
			Password: b.sshPassword,
		})
		if err != nil {
			return bootstrapProgressMsg{step: bsFailed, err: fmt.Errorf("SSH connect for monitoring: %w", err)}
		}
		defer func() {
			_ = client.Close() // Best-effort SSH client close after monitoring deploy.
		}()

		// Generate monitoring configuration
		monitoringCfg := docker.MonitoringConfig{
			AgentHost:      b.host,
			AgentPort:      7474,
			PrometheusPort: 9090,
			GrafanaPort:    3000,
			HasNvidiaGPU:   b.preflight != nil && b.preflight.GPUDetected,
		}

		composeYAML := docker.GenerateMonitoringCompose(monitoringCfg)
		prometheusYAML := docker.GeneratePrometheusConfig(monitoringCfg)

		// Upload compose file
		tmpDir := "/tmp/yokai-monitoring"
		createDirCmd := fmt.Sprintf("mkdir -p %s", tmpDir)
		if _, err := client.Exec(createDirCmd); err != nil {
			return bootstrapProgressMsg{step: bsFailed, err: fmt.Errorf("creating monitoring dir: %w", err)}
		}

		// Write compose file
		writeComposeCmd := fmt.Sprintf(`cat > %s/docker-compose.yml << 'EOF'
%s
EOF`, tmpDir, composeYAML)
		if _, err := client.Exec(writeComposeCmd); err != nil {
			return bootstrapProgressMsg{step: bsFailed, err: fmt.Errorf("writing compose file: %w", err)}
		}

		// Write prometheus config
		writePrometheusCmd := fmt.Sprintf(`cat > %s/prometheus.yml << 'EOF'
%s
EOF`, tmpDir, prometheusYAML)
		if _, err := client.Exec(writePrometheusCmd); err != nil {
			return bootstrapProgressMsg{step: bsFailed, err: fmt.Errorf("writing prometheus config: %w", err)}
		}

		// Start monitoring stack
		deployCmd := fmt.Sprintf("cd %s && docker compose up -d", tmpDir)
		if _, err := client.Exec(deployCmd); err != nil {
			return bootstrapProgressMsg{step: bsFailed, err: fmt.Errorf("starting monitoring stack: %w", err)}
		}

		return bootstrapProgressMsg{step: bsDone}
	}
}

func (b *Bootstrap) Update(msg tea.Msg) (View, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		b.width = msg.Width
		b.height = msg.Height

	case bootstrapProgressMsg:
		if msg.err != nil {
			b.step = bsFailed
			b.err = msg.err.Error()
			if msg.preflight != nil {
				b.preflight = msg.preflight
			}
			return b, nil
		}

		b.step = msg.step
		if msg.preflight != nil {
			b.preflight = msg.preflight
		}
		if msg.agentToken != "" {
			b.agentToken = msg.agentToken
		}

		if msg.step == bsDone {
			// Add device to config
			device := config.Device{
				ID:             b.host,
				Label:          b.host,
				Host:           b.host,
				SSHUser:        b.sshUser,
				SSHKey:         b.sshKey,
				ConnectionType: b.connectionType,
				AgentPort:      7474,
				AgentToken:     b.agentToken,
			}
			if b.preflight != nil && b.preflight.GPUDetected {
				device.GPUType = "nvidia"
			}
			b.cfg.AddDevice(device)
			_ = config.Save(b.cfg)
		}

	case tea.KeyMsg:
		switch msg.String() {
		case "y":
			if b.step == bsMonitoringPrompt {
				b.step = bsDeployingMonitoring
				return b, b.runMonitoringDeploy()
			}
		case "n":
			if b.step == bsMonitoringPrompt {
				return b, func() tea.Msg {
					return bootstrapProgressMsg{step: bsDone}
				}
			}
		case "enter":
			if b.step == bsDone {
				return b, Navigate(NewHFToken(b.cfg, b.version))
			}
		case "esc":
			if b.step == bsFailed || b.step == bsDone {
				return b, PopView()
			}
		case "r":
			if b.step == bsFailed {
				b.step = bsConnecting
				b.err = ""
				return b, b.runBootstrap()
			}
		}
	}
	return b, nil
}

func (b *Bootstrap) View() string {
	title := lipgloss.NewStyle().Foreground(theme.Accent).Bold(true).
		Render(fmt.Sprintf("Bootstrap — %s", b.host))

	// Step indicators
	var steps string
	stepNames := []string{"SSH Connect", "Pre-flight", "Deploy Agent", "Deploy Monitoring"}
	for i, name := range stepNames {
		var icon string
		currentStep := bootstrapStep(i)

		// Skip monitoring step if we're not at that point yet
		if currentStep >= bsMonitoringPrompt && b.step < bsMonitoringPrompt {
			continue
		}

		switch {
		case currentStep < b.step || (currentStep == bsDeployingMonitoring && b.step == bsDone):
			icon = theme.SuccessStyle.Render("✓")
		case currentStep == b.step && b.step != bsFailed:
			if b.step == bsMonitoringPrompt {
				icon = theme.StatusLoading()
			} else {
				icon = theme.StatusLoading()
			}
		case b.step == bsFailed:
			icon = theme.CritStyle.Render("✗")
		default:
			icon = theme.MutedStyle.Render("○")
		}

		label := theme.MutedStyle.Render(name)
		if currentStep == b.step || (b.step == bsFailed && currentStep <= b.step) {
			label = theme.PrimaryStyle.Render(name)
		}
		steps += fmt.Sprintf("  %s %s\n", icon, label)
	}

	// Preflight results
	var preflightInfo string
	if b.preflight != nil {
		pf := b.preflight
		preflightInfo = "\n" + theme.MutedStyle.Render("── Pre-flight ──") + "\n"
		preflightInfo += fmt.Sprintf("  OS:     %s (%s)\n", pf.OS, pf.Arch)
		preflightInfo += fmt.Sprintf("  Docker: %s\n", boolStatus(pf.DockerInstalled, pf.DockerVersion))
		preflightInfo += fmt.Sprintf("  GPU:    %s\n", boolStatus(pf.GPUDetected, pf.GPUName))
		if pf.GPUDetected {
			preflightInfo += fmt.Sprintf("  VRAM:   %d MB\n", pf.GPUVRAMMb)
			preflightInfo += fmt.Sprintf("  Toolkit:%s\n", boolStatus(pf.NvidiaToolkitInstalled, ""))
			preflightInfo += fmt.Sprintf("  Runtime:%s\n", boolStatus(pf.NvidiaRuntimeAvailable, ""))
		}
		preflightInfo += fmt.Sprintf("  Disk:   %d GB free\n", pf.DiskFreeGB)
	}

	// Error or success
	var statusLine string
	if b.err != "" {
		statusLine = "\n" + theme.CritStyle.Render("Error: "+b.err)
		if b.step == bsFailed {
			statusLine += "\n\n" + theme.MutedStyle.Render("Press 'r' to retry, Esc to go back")
		}
	} else if b.step == bsMonitoringPrompt {
		statusLine = "\n" + theme.PrimaryStyle.Render("Deploy monitoring stack?")
		statusLine += "\n" + theme.MutedStyle.Render("Includes Prometheus, Grafana, and Node Exporter")
		if b.preflight != nil && b.preflight.GPUDetected {
			statusLine += "\n" + theme.MutedStyle.Render("GPU monitoring (dcgm-exporter) will be included")
		}
		statusLine += "\n\n" + theme.MutedStyle.Render("Press 'y' for yes, 'n' for no")
	} else if b.step == bsDone {
		statusLine = "\n" + theme.SuccessStyle.Render("Device bootstrapped successfully!")
		statusLine += "\n" + theme.MutedStyle.Render("Press Enter to continue")
	}

	// Responsive width
	cardWidth := 55
	if b.width > 0 && b.width < 65 {
		cardWidth = b.width - 10
		if cardWidth < 40 {
			cardWidth = 40
		}
	}

	card := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(theme.Border).
		Padding(1, 2).
		Width(cardWidth).
		Render(title + "\n\n" + steps + preflightInfo + statusLine)

	return lipgloss.NewStyle().Padding(2, 0).Render(card)
}

func boolStatus(ok bool, detail string) string {
	if ok {
		status := theme.GoodStyle.Render("✓")
		if detail != "" {
			return status + " " + detail
		}
		return status + " yes"
	}
	return theme.CritStyle.Render("✗") + " not found"
}

func (b *Bootstrap) KeyBinds() []KeyBind {
	switch b.step {
	case bsFailed:
		return []KeyBind{
			{Key: "r", Help: "retry"},
			{Key: "Esc", Help: "back"},
		}
	case bsMonitoringPrompt:
		return []KeyBind{
			{Key: "y", Help: "deploy monitoring"},
			{Key: "n", Help: "skip monitoring"},
		}
	case bsDone:
		return []KeyBind{
			{Key: "Enter", Help: "continue"},
			{Key: "Esc", Help: "back"},
		}
	default:
		return []KeyBind{
			{Key: "", Help: "bootstrapping..."},
		}
	}
}
