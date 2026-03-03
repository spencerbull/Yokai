package views

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/spencerbull/yokai/internal/config"
	"github.com/spencerbull/yokai/internal/tui/theme"
)

type deployStep int

const (
	stepType deployStep = iota
	stepDevice
	stepImage
	stepModel
	stepConfig
	stepDeploying
)

var stepLabels = []string{"Type", "Device", "Image", "Model", "Config", "Deploy"}

type workloadType int

const (
	wtVLLM workloadType = iota
	wtLlamaCpp
	wtComfyUI
)

var workloadLabels = []string{"vLLM", "llama.cpp", "ComfyUI"}

// deployResultMsg is sent when the deploy API call completes.
type deployResultMsg struct {
	ContainerID string
	Error       error
}

// deployRequest represents the API request payload.
type deployRequest struct {
	DeviceID  string            `json:"device_id"`
	Image     string            `json:"image"`
	Name      string            `json:"name"`
	Ports     map[string]string `json:"ports"`
	Env       map[string]string `json:"env"`
	GPUIDs    string            `json:"gpu_ids"`
	ExtraArgs string            `json:"extra_args"`
	Volumes   map[string]string `json:"volumes"`
}

// Deploy is the multi-step deploy wizard.
type Deploy struct {
	cfg     *config.Config
	version string

	currentStep deployStep
	cursor      int

	// Step 1: workload type
	workload workloadType

	// Step 2: device
	deviceIdx int

	// Step 3: image
	imageTag string

	// Step 4: model
	modelID string

	// Step 5: config
	port              string
	extraArgs         string
	activeConfigField int

	// Deployment state
	deployError string
	width       int
	height      int
}

// NewDeploy creates the deploy wizard view.
func NewDeploy(cfg *config.Config, version string) *Deploy {
	return &Deploy{
		cfg:     cfg,
		version: version,
		port:    "8000",
	}
}

func (d *Deploy) Init() tea.Cmd {
	return nil
}

func (d *Deploy) Update(msg tea.Msg) (View, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		d.width = msg.Width
		d.height = msg.Height

	case deployResultMsg:
		if msg.Error != nil {
			d.deployError = msg.Error.Error()
			d.currentStep = stepConfig
		} else {
			// Success - pop back to dashboard
			return d, PopView()
		}
		return d, nil

	case tea.KeyMsg:
		switch msg.String() {
		case "esc":
			if d.currentStep == stepType {
				return d, PopView()
			}
			d.currentStep--
			return d, nil
		case "backspace":
			if d.currentStep > stepType {
				d.currentStep--
			}
			return d, nil
		}

		switch d.currentStep {
		case stepType:
			return d.updateType(msg)
		case stepDevice:
			return d.updateDevice(msg)
		case stepImage:
			return d.updateImage(msg)
		case stepModel:
			return d.updateModel(msg)
		case stepConfig:
			return d.updateConfig(msg)
		case stepDeploying:
			return d.updateDeploying(msg)
		}
	}
	return d, nil
}

func (d *Deploy) updateType(msg tea.KeyMsg) (View, tea.Cmd) {
	switch msg.String() {
	case "up", "k":
		if d.cursor > 0 {
			d.cursor--
		}
	case "down", "j":
		if d.cursor < len(workloadLabels)-1 {
			d.cursor++
		}
	case "enter":
		d.workload = workloadType(d.cursor)
		d.currentStep = stepDevice
		d.cursor = 0
		// Set default port based on type
		switch d.workload {
		case wtVLLM:
			d.port = "8000"
		case wtLlamaCpp:
			d.port = "8080"
		case wtComfyUI:
			d.port = "8188"
		}
	}
	return d, nil
}

func (d *Deploy) updateDevice(msg tea.KeyMsg) (View, tea.Cmd) {
	switch msg.String() {
	case "up", "k":
		if d.cursor > 0 {
			d.cursor--
		}
	case "down", "j":
		if d.cursor < len(d.cfg.Devices)-1 {
			d.cursor++
		}
	case "enter":
		if len(d.cfg.Devices) > 0 {
			d.deviceIdx = d.cursor
			d.currentStep = stepImage
			d.cursor = 0
		}
	}
	return d, nil
}

func (d *Deploy) updateImage(msg tea.KeyMsg) (View, tea.Cmd) {
	switch msg.String() {
	case "enter":
		// Use default image for now
		switch d.workload {
		case wtVLLM:
			d.imageTag = d.cfg.Preferences.DefaultVLLMImage
		case wtLlamaCpp:
			d.imageTag = d.cfg.Preferences.DefaultLlamaImage
		case wtComfyUI:
			d.imageTag = d.cfg.Preferences.DefaultComfyImage
		}
		d.currentStep = stepModel
		d.cursor = 0
	case "backspace":
		if len(d.imageTag) > 0 {
			d.imageTag = d.imageTag[:len(d.imageTag)-1]
		}
	default:
		if len(msg.String()) == 1 {
			d.imageTag += msg.String()
		}
	}
	return d, nil
}

func (d *Deploy) updateModel(msg tea.KeyMsg) (View, tea.Cmd) {
	switch msg.String() {
	case "enter":
		if d.workload == wtComfyUI {
			d.modelID = "" // ComfyUI doesn't need a model
		}
		d.currentStep = stepConfig
	case "backspace":
		if len(d.modelID) > 0 {
			d.modelID = d.modelID[:len(d.modelID)-1]
		}
	default:
		if len(msg.String()) == 1 {
			d.modelID += msg.String()
		}
	}
	return d, nil
}

func (d *Deploy) updateConfig(msg tea.KeyMsg) (View, tea.Cmd) {
	switch msg.String() {
	case "tab":
		d.activeConfigField = (d.activeConfigField + 1) % 2
	case "enter":
		// Save to config first
		svc := config.Service{
			ID:       fmt.Sprintf("yokai-%s-%s", workloadLabels[d.workload], sanitize(d.modelID)),
			DeviceID: d.cfg.Devices[d.deviceIdx].ID,
			Type:     strings.ToLower(workloadLabels[d.workload]),
			Image:    d.imageTag,
			Model:    d.modelID,
			Port:     atoi(d.port),
		}
		d.cfg.Services = append(d.cfg.Services, svc)
		_ = config.Save(d.cfg)

		// Switch to deploying step and make API call
		d.currentStep = stepDeploying
		d.deployError = ""
		return d, d.deployToAPI(svc)
	case "backspace":
		switch d.activeConfigField {
		case 0:
			if len(d.port) > 0 {
				d.port = d.port[:len(d.port)-1]
			}
		case 1:
			if len(d.extraArgs) > 0 {
				d.extraArgs = d.extraArgs[:len(d.extraArgs)-1]
			}
		}
	default:
		if len(msg.String()) == 1 {
			switch d.activeConfigField {
			case 0:
				d.port += msg.String()
			case 1:
				d.extraArgs += msg.String()
			}
		}
	}
	return d, nil
}

func (d *Deploy) updateDeploying(msg tea.KeyMsg) (View, tea.Cmd) {
	switch msg.String() {
	case "esc":
		// Allow cancelling during deployment
		return d, PopView()
	}
	return d, nil
}

// deployToAPI makes an HTTP POST to the daemon to deploy the service.
func (d *Deploy) deployToAPI(svc config.Service) tea.Cmd {
	return func() tea.Msg {
		daemonAddr := d.cfg.Daemon.Listen
		if daemonAddr == "" {
			daemonAddr = "127.0.0.1:7473"
		}

		// Build deployment request
		req := deployRequest{
			DeviceID:  svc.DeviceID,
			Image:     svc.Image,
			Name:      svc.ID,
			Ports:     map[string]string{d.port: d.port},
			Env:       map[string]string{},
			GPUIDs:    "all",
			ExtraArgs: d.extraArgs,
			Volumes:   map[string]string{},
		}

		// Add model to environment if provided
		if svc.Model != "" {
			req.Env["MODEL"] = svc.Model
		}

		// Encode request
		reqBody, err := json.Marshal(req)
		if err != nil {
			return deployResultMsg{Error: fmt.Errorf("encoding request: %w", err)}
		}

		// Make HTTP request
		url := fmt.Sprintf("http://%s/deploy", daemonAddr)
		httpReq, err := http.NewRequest("POST", url, bytes.NewReader(reqBody))
		if err != nil {
			return deployResultMsg{Error: fmt.Errorf("creating request: %w", err)}
		}
		httpReq.Header.Set("Content-Type", "application/json")

		client := &http.Client{Timeout: 30 * time.Second}
		resp, err := client.Do(httpReq)
		if err != nil {
			return deployResultMsg{Error: fmt.Errorf("daemon request failed: %w", err)}
		}
		defer resp.Body.Close()

		if resp.StatusCode != 200 {
			return deployResultMsg{Error: fmt.Errorf("daemon returned status %d", resp.StatusCode)}
		}

		// Parse response to get container ID (if provided)
		var respData struct {
			ContainerID string `json:"container_id"`
		}
		if err := json.NewDecoder(resp.Body).Decode(&respData); err != nil {
			// Success but couldn't parse response - that's fine
			return deployResultMsg{ContainerID: ""}
		}

		return deployResultMsg{ContainerID: respData.ContainerID}
	}
}

func (d *Deploy) View() string {
	title := lipgloss.NewStyle().Foreground(theme.Accent).Bold(true).Render("Deploy Wizard")

	// Step indicator
	var stepBar string
	for i, label := range stepLabels {
		style := theme.MutedStyle
		if deployStep(i) == d.currentStep {
			style = lipgloss.NewStyle().Foreground(theme.Accent).Bold(true)
		} else if deployStep(i) < d.currentStep {
			style = theme.GoodStyle
		}
		stepBar += style.Render(fmt.Sprintf(" %d·%s ", i+1, label))
		if i < len(stepLabels)-1 {
			stepBar += theme.MutedStyle.Render("→")
		}
	}

	var body string
	switch d.currentStep {
	case stepType:
		body = theme.PrimaryStyle.Render("Select workload type:") + "\n\n"
		for i, label := range workloadLabels {
			cursor := "  "
			style := theme.PrimaryStyle
			if i == d.cursor {
				cursor = "> "
				style = lipgloss.NewStyle().Foreground(theme.Accent).Bold(true)
			}
			body += cursor + style.Render(label) + "\n"
		}

	case stepDevice:
		body = theme.PrimaryStyle.Render("Select target device:") + "\n\n"
		if len(d.cfg.Devices) == 0 {
			body += theme.WarnStyle.Render("No devices registered. Go back and add a device first.")
		} else {
			for i, dev := range d.cfg.Devices {
				cursor := "  "
				style := theme.PrimaryStyle
				if i == d.cursor {
					cursor = "> "
					style = lipgloss.NewStyle().Foreground(theme.Accent).Bold(true)
				}
				body += fmt.Sprintf("%s%s  %s\n", cursor,
					style.Render(dev.Label), theme.MutedStyle.Render(dev.Host))
			}
		}

	case stepImage:
		body = theme.PrimaryStyle.Render("Docker image:") + "\n\n"
		defaultImg := ""
		switch d.workload {
		case wtVLLM:
			defaultImg = d.cfg.Preferences.DefaultVLLMImage
		case wtLlamaCpp:
			defaultImg = d.cfg.Preferences.DefaultLlamaImage
		case wtComfyUI:
			defaultImg = d.cfg.Preferences.DefaultComfyImage
		}
		if d.imageTag == "" {
			body += theme.MutedStyle.Render("Default: "+defaultImg) + "\n"
			body += theme.MutedStyle.Render("Press Enter to use default, or type a custom image") + "\n\n"
		}
		// Responsive width for input
		inputWidth := 50
		if d.width > 0 && d.width < 70 {
			inputWidth = d.width - 20
			if inputWidth < 30 {
				inputWidth = 30
			}
		}

		inputBox := lipgloss.NewStyle().
			Border(lipgloss.NormalBorder()).
			BorderForeground(theme.Accent).
			Padding(0, 1).
			Width(inputWidth).
			Render(d.imageTag + "█")
		body += inputBox

	case stepModel:
		if d.workload == wtComfyUI {
			body = theme.PrimaryStyle.Render("ComfyUI does not require a model selection.") + "\n\n" +
				theme.MutedStyle.Render("Press Enter to continue.")
		} else {
			body = theme.PrimaryStyle.Render("Model ID (HuggingFace):") + "\n\n"
			body += theme.MutedStyle.Render("e.g. meta-llama/Llama-3.1-8B-Instruct") + "\n\n"
			// Responsive width for input
			inputWidth := 50
			if d.width > 0 && d.width < 70 {
				inputWidth = d.width - 20
				if inputWidth < 30 {
					inputWidth = 30
				}
			}

			inputBox := lipgloss.NewStyle().
				Border(lipgloss.NormalBorder()).
				BorderForeground(theme.Accent).
				Padding(0, 1).
				Width(inputWidth).
				Render(d.modelID + "█")
			body += inputBox
		}

	case stepConfig:
		body = theme.PrimaryStyle.Render("Configuration:") + "\n\n"
		body += d.configField("Port:", d.port, 0) + "\n"
		body += d.configField("Extra args:", d.extraArgs, 1) + "\n\n"

		// Show deployment error if any
		if d.deployError != "" {
			body += "\n" + theme.WarnStyle.Render("❌ Deployment failed: "+d.deployError) + "\n"
		}

		// Summary
		body += theme.MutedStyle.Render("── Summary ──") + "\n"
		body += fmt.Sprintf("  Type:   %s\n", workloadLabels[d.workload])
		if d.deviceIdx < len(d.cfg.Devices) {
			body += fmt.Sprintf("  Device: %s\n", d.cfg.Devices[d.deviceIdx].Label)
		}
		body += fmt.Sprintf("  Image:  %s\n", d.imageTag)
		if d.modelID != "" {
			body += fmt.Sprintf("  Model:  %s\n", d.modelID)
		}
		body += fmt.Sprintf("  Port:   %s\n", d.port)
		body += "\n" + theme.SuccessStyle.Render("Press Enter to deploy")

	case stepDeploying:
		body = theme.PrimaryStyle.Render("Deploying service...") + "\n\n"
		body += theme.MutedStyle.Render("●○○") + " Connecting to daemon\n"
		body += theme.MutedStyle.Render("○●○") + " Starting container\n"
		body += theme.MutedStyle.Render("○○●") + " Verifying deployment\n\n"
		body += theme.MutedStyle.Render("Press Esc to cancel")
	}

	// Responsive width for card
	cardWidth := 60
	if d.width > 0 && d.width < 70 {
		cardWidth = d.width - 10
		if cardWidth < 45 {
			cardWidth = 45
		}
	}

	card := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(theme.Border).
		Padding(1, 2).
		Width(cardWidth).
		Render(title + "\n" + stepBar + "\n\n" + body)

	return lipgloss.NewStyle().Padding(1, 0).Render(card)
}

func (d *Deploy) configField(label, value string, idx int) string {
	borderColor := theme.Border
	if d.activeConfigField == idx {
		borderColor = theme.Accent
	}
	display := value
	if d.activeConfigField == idx {
		display += "█"
	}

	// Responsive width for config fields
	fieldWidth := 30
	if d.width > 0 && d.width < 70 {
		fieldWidth = d.width - 30
		if fieldWidth < 20 {
			fieldWidth = 20
		}
	}

	return theme.MutedStyle.Render(label) + "\n" +
		lipgloss.NewStyle().
			Border(lipgloss.NormalBorder()).
			BorderForeground(borderColor).
			Padding(0, 1).
			Width(fieldWidth).
			Render(display)
}

func (d *Deploy) KeyBinds() []KeyBind {
	return []KeyBind{
		{Key: "↑/↓", Help: "navigate"},
		{Key: "Enter", Help: "next"},
		{Key: "Backspace", Help: "prev step"},
		{Key: "Esc", Help: "cancel"},
	}
}

func sanitize(s string) string {
	r := strings.NewReplacer("/", "-", " ", "-")
	s = r.Replace(s)
	if len(s) > 30 {
		s = s[:30]
	}
	return s
}

func atoi(s string) int {
	n := 0
	for _, c := range s {
		if c >= '0' && c <= '9' {
			n = n*10 + int(c-'0')
		}
	}
	return n
}
