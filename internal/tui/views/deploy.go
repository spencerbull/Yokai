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
	"github.com/spencerbull/yokai/internal/tui/components"
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
	Model     string            `json:"model"`
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
	imageTag    string
	imageTyping bool // true = free-text input mode, false = picking from history

	// Step 4: model
	modelID     string
	modelTyping bool // true = free-text input mode, false = picking from history

	// Step 5: config
	port              string
	extraArgs         string
	activeConfigField int

	// History (loaded from ~/.config/yokai/history.json)
	history *config.History

	// Deployment state
	deployError string
	spinner     components.LoadingSpinner
	width       int
	height      int
}

// NewDeploy creates the deploy wizard view.
func NewDeploy(cfg *config.Config, version string) *Deploy {
	h, err := config.LoadHistory()
	if err != nil {
		h = &config.History{}
	}

	return &Deploy{
		cfg:     cfg,
		version: version,
		port:    "8000",
		history: h,
	}
}

func (d *Deploy) Init() tea.Cmd {
	return nil
}

func (d *Deploy) Update(msg tea.Msg) (View, tea.Cmd) {
	// Forward spinner ticks when deploying
	if d.currentStep == stepDeploying {
		var spinnerCmd tea.Cmd
		d.spinner, spinnerCmd = d.spinner.Update(msg)
		if spinnerCmd != nil {
			// Check for non-key messages (spinner ticks etc)
			if _, ok := msg.(tea.KeyMsg); !ok {
				return d, spinnerCmd
			}
		}
	}

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		d.width = msg.Width
		if d.width > theme.MaxContentWidth-2*theme.ContentPadding {
			d.width = theme.MaxContentWidth - 2*theme.ContentPadding
		}
		d.height = msg.Height

	case deployResultMsg:
		if msg.Error != nil {
			d.deployError = msg.Error.Error()
			d.currentStep = stepConfig
		} else {
			// Save container ID back to config so dashboard can match model
			if msg.ContainerID != "" {
				for i := range d.cfg.Services {
					svc := &d.cfg.Services[i]
					if svc.DeviceID == d.cfg.Devices[d.deviceIdx].ID &&
						svc.Image == d.imageTag &&
						svc.Model == d.modelID &&
						svc.ContainerID == "" {
						svc.ContainerID = msg.ContainerID
						_ = config.Save(d.cfg)
						break
					}
				}
			}
			// Save image and model to history for future deploys
			d.history.AddImage(d.imageTag)
			d.history.AddModel(d.modelID)
			_ = config.SaveHistory(d.history)

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
	case "backspace":
		return d, PopView()
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
	case "backspace":
		d.currentStep--
		return d, nil
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

// imageOptions returns the list of images to pick from: history items + default.
func (d *Deploy) imageOptions() []string {
	defaultImg := ""
	switch d.workload {
	case wtVLLM:
		defaultImg = d.cfg.Preferences.DefaultVLLMImage
	case wtLlamaCpp:
		defaultImg = d.cfg.Preferences.DefaultLlamaImage
	case wtComfyUI:
		defaultImg = d.cfg.Preferences.DefaultComfyImage
	}

	// Start with history, then append default if not already present
	seen := make(map[string]bool)
	var options []string
	for _, img := range d.history.Images {
		if !seen[img] {
			options = append(options, img)
			seen[img] = true
		}
	}
	if defaultImg != "" && !seen[defaultImg] {
		options = append(options, defaultImg)
	}
	return options
}

func (d *Deploy) updateImage(msg tea.KeyMsg) (View, tea.Cmd) {
	options := d.imageOptions()

	// If no history and no typing, start in typing mode
	if len(options) == 0 {
		d.imageTyping = true
	}

	if d.imageTyping {
		switch msg.String() {
		case "enter":
			if d.imageTag == "" {
				// Use default
				switch d.workload {
				case wtVLLM:
					d.imageTag = d.cfg.Preferences.DefaultVLLMImage
				case wtLlamaCpp:
					d.imageTag = d.cfg.Preferences.DefaultLlamaImage
				case wtComfyUI:
					d.imageTag = d.cfg.Preferences.DefaultComfyImage
				}
			}
			d.currentStep = stepModel
			d.cursor = 0
			d.imageTyping = false
		case "backspace":
			if len(d.imageTag) > 0 {
				d.imageTag = d.imageTag[:len(d.imageTag)-1]
			} else {
				// Switch back to picker if there are options
				if len(options) > 0 {
					d.imageTyping = false
				}
			}
		default:
			s := msg.String()
			if len(s) == 1 || (len(s) > 1 && !strings.HasPrefix(s, "ctrl+") && !strings.HasPrefix(s, "alt+")) {
				d.imageTag += s
			}
		}
		return d, nil
	}

	// Picker mode
	switch msg.String() {
	case "up", "k":
		if d.cursor > 0 {
			d.cursor--
		}
	case "down", "j":
		if d.cursor < len(options) {
			d.cursor++
		}
	case "enter":
		if d.cursor >= len(options) {
			// "Type custom" option selected
			d.imageTyping = true
			d.imageTag = ""
			return d, nil
		}
		d.imageTag = options[d.cursor]
		d.currentStep = stepModel
		d.cursor = 0
	case "/":
		// Switch to free-text typing
		d.imageTyping = true
		d.imageTag = ""
	}
	return d, nil
}

func (d *Deploy) updateModel(msg tea.KeyMsg) (View, tea.Cmd) {
	if d.workload == wtComfyUI {
		if msg.String() == "enter" {
			d.modelID = ""
			d.currentStep = stepConfig
		}
		return d, nil
	}

	models := d.history.Models

	// If no history, start in typing mode
	if len(models) == 0 {
		d.modelTyping = true
	}

	if d.modelTyping {
		switch msg.String() {
		case "enter":
			d.currentStep = stepConfig
			d.cursor = 0
			d.modelTyping = false
		case "backspace":
			if len(d.modelID) > 0 {
				d.modelID = d.modelID[:len(d.modelID)-1]
			} else if len(models) > 0 {
				d.modelTyping = false
			}
		default:
			s := msg.String()
			if len(s) == 1 || (len(s) > 1 && !strings.HasPrefix(s, "ctrl+") && !strings.HasPrefix(s, "alt+")) {
				d.modelID += s
			}
		}
		return d, nil
	}

	// Picker mode
	switch msg.String() {
	case "up", "k":
		if d.cursor > 0 {
			d.cursor--
		}
	case "down", "j":
		if d.cursor < len(models) {
			d.cursor++
		}
	case "enter":
		if d.cursor >= len(models) {
			// "Type custom" option selected
			d.modelTyping = true
			d.modelID = ""
			return d, nil
		}
		d.modelID = models[d.cursor]
		d.currentStep = stepConfig
		d.cursor = 0
	case "/":
		d.modelTyping = true
		d.modelID = ""
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
			ID:       fmt.Sprintf("%s-%s", strings.ToLower(workloadLabels[d.workload]), sanitize(d.modelID)),
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
		d.spinner = components.NewLoadingSpinner("Deploying container...")
		return d, tea.Batch(d.deployToAPI(svc), d.spinner.Init())
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
		s := msg.String()
		if len(s) == 1 || (len(s) > 1 && !strings.HasPrefix(s, "ctrl+") && !strings.HasPrefix(s, "alt+")) {
			switch d.activeConfigField {
			case 0:
				d.port += s
			case 1:
				d.extraArgs += s
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
			Model:     svc.Model,
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

		client := &http.Client{Timeout: 10 * time.Minute}
		resp, err := client.Do(httpReq)
		if err != nil {
			return deployResultMsg{Error: fmt.Errorf("daemon request failed: %w", err)}
		}
		defer func() {
			_ = resp.Body.Close() // Best-effort close of deploy response body.
		}()

		if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
			// Read daemon error body for better diagnostics
			var errResp struct {
				Error   string `json:"error"`
				Message string `json:"message"`
			}
			if json.NewDecoder(resp.Body).Decode(&errResp) == nil && errResp.Message != "" {
				return deployResultMsg{Error: fmt.Errorf("%s", errResp.Message)}
			}
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
		options := d.imageOptions()

		if d.imageTyping || len(options) == 0 {
			// Free-text input mode
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
			if len(options) > 0 {
				body += "\n\n" + theme.MutedStyle.Render("Backspace to return to history")
			}
		} else {
			// Picker mode — show history + default
			body += theme.MutedStyle.Render("Recent images:") + "\n\n"
			for i, opt := range options {
				cursor := "  "
				style := theme.PrimaryStyle
				if i == d.cursor {
					cursor = "> "
					style = lipgloss.NewStyle().Foreground(theme.Accent).Bold(true)
				}
				body += cursor + style.Render(opt) + "\n"
			}
			// "Type custom" option at the end
			cursor := "  "
			style := theme.MutedStyle
			if d.cursor == len(options) {
				cursor = "> "
				style = lipgloss.NewStyle().Foreground(theme.Accent).Bold(true)
			}
			body += cursor + style.Render("Type custom image...") + "\n"
			body += "\n" + theme.MutedStyle.Render("/ to type custom")
		}

	case stepModel:
		if d.workload == wtComfyUI {
			body = theme.PrimaryStyle.Render("ComfyUI does not require a model selection.") + "\n\n" +
				theme.MutedStyle.Render("Press Enter to continue.")
		} else {
			body = theme.PrimaryStyle.Render("Model ID (HuggingFace):") + "\n\n"
			models := d.history.Models

			if d.modelTyping || len(models) == 0 {
				// Free-text input mode
				body += theme.MutedStyle.Render("e.g. meta-llama/Llama-3.1-8B-Instruct") + "\n\n"
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
				if len(models) > 0 {
					body += "\n\n" + theme.MutedStyle.Render("Backspace to return to history")
				}
			} else {
				// Picker mode — show history
				body += theme.MutedStyle.Render("Recent models:") + "\n\n"
				for i, m := range models {
					cursor := "  "
					style := theme.PrimaryStyle
					if i == d.cursor {
						cursor = "> "
						style = lipgloss.NewStyle().Foreground(theme.Accent).Bold(true)
					}
					body += cursor + style.Render(m) + "\n"
				}
				// "Type custom" option at the end
				cursor := "  "
				style := theme.MutedStyle
				if d.cursor == len(models) {
					cursor = "> "
					style = lipgloss.NewStyle().Foreground(theme.Accent).Bold(true)
				}
				body += cursor + style.Render("Type custom model...") + "\n"
				body += "\n" + theme.MutedStyle.Render("/ to type custom")
			}
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
		body += d.spinner.View() + "\n\n"
		body += theme.MutedStyle.Render("This may take several minutes for large images.") + "\n"
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
		{Key: "Esc", Help: "prev step"},
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
