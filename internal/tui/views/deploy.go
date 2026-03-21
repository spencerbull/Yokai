package views

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/spencerbull/yokai/internal/config"
	"github.com/spencerbull/yokai/internal/hf"
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

type hfSearchResultMsg struct {
	requestID int
	query     string
	results   []hf.Model
	err       error
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
	imageInput  textinput.Model
	imageTyping bool // true = free-text input mode, false = picking from history

	// Step 4: model
	modelInput  textinput.Model
	modelTyping bool // true = free-text input mode, false = picking from history

	// Step 5: config
	portInput          textinput.Model
	extraArgsInput     textinput.Model
	activeConfigField  int
	showArgsHelp       bool
	modelSearchResults []hf.Model
	modelSearchCursor  int
	modelSearchLoading bool
	modelSearchErr     string
	modelSearchQuery   string
	modelSearchRequest int

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

	// Initialize text inputs
	imageInput := components.NewTextField("Docker image")
	modelInput := components.NewTextField("e.g. meta-llama/Llama-3.1-8B-Instruct")
	portInput := components.NewPortField("8000")
	extraArgsInput := components.NewTextField("Extra arguments")

	// Set initial defaults but blur the inputs
	imageInput.Blur()
	modelInput.Blur()
	portInput.Blur()
	extraArgsInput.Blur()

	return &Deploy{
		cfg:            cfg,
		version:        version,
		history:        h,
		imageInput:     imageInput,
		modelInput:     modelInput,
		portInput:      portInput,
		extraArgsInput: extraArgsInput,
	}
}

func (d *Deploy) Init() tea.Cmd {
	return nil
}

// InputActive returns true when the view has active text inputs
func (d *Deploy) InputActive() bool {
	switch d.currentStep {
	case stepImage:
		return d.imageTyping
	case stepModel:
		return d.modelTyping
	case stepConfig:
		return true // Always has text inputs
	default:
		return false
	}
}

func (d *Deploy) Update(msg tea.Msg) (View, tea.Cmd) {
	var cmds []tea.Cmd

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

		// Update text input widths
		inputWidth := 50
		if d.width > 0 && d.width < 70 {
			inputWidth = d.width - 20
			if inputWidth < 30 {
				inputWidth = 30
			}
		}
		d.imageInput.Width = inputWidth
		d.modelInput.Width = inputWidth

		configWidth := 30
		if d.width > 0 && d.width < 70 {
			configWidth = d.width - 30
			if configWidth < 20 {
				configWidth = 20
			}
		}
		d.portInput.Width = configWidth
		d.extraArgsInput.Width = configWidth

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
						svc.Image == d.imageInput.Value() &&
						svc.Model == d.modelInput.Value() &&
						svc.ContainerID == "" {
						svc.ContainerID = msg.ContainerID
						_ = config.Save(d.cfg)
						break
					}
				}
			}
			// Save image and model to history for future deploys
			d.history.AddImage(d.imageInput.Value())
			d.history.AddModel(d.modelInput.Value())
			_ = config.SaveHistory(d.history)

			// Success - pop back to dashboard with toast
			return d, tea.Batch(
				PopView(),
				ShowToast("Service deployed successfully", ToastSuccess),
			)
		}
		return d, nil

	case hfSearchResultMsg:
		if msg.requestID != d.modelSearchRequest {
			return d, nil
		}
		d.modelSearchLoading = false
		if strings.TrimSpace(d.modelInput.Value()) != msg.query {
			return d, nil
		}
		if msg.err != nil {
			d.modelSearchErr = msg.err.Error()
			d.modelSearchResults = nil
			d.modelSearchCursor = 0
			return d, nil
		}
		d.modelSearchErr = ""
		d.modelSearchQuery = msg.query
		d.modelSearchResults = d.rankModelSearchResults(msg.results, msg.query)
		if d.modelSearchCursor >= len(d.modelSearchResults) {
			d.modelSearchCursor = 0
		}
		return d, nil

	case tea.KeyMsg:
		switch msg.String() {
		case "esc":
			if d.currentStep == stepType {
				return d, PopView()
			}
			// When leaving typing mode, blur the inputs
			if d.currentStep == stepImage && d.imageTyping {
				d.imageTyping = false
				d.imageInput.Blur()
			} else if d.currentStep == stepModel && d.modelTyping {
				d.modelTyping = false
				d.clearModelSearch()
				d.modelInput.Blur()
			} else {
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
			var cmd tea.Cmd
			d, cmd = d.updateImage(msg)
			if cmd != nil {
				cmds = append(cmds, cmd)
			}
		case stepModel:
			var cmd tea.Cmd
			d, cmd = d.updateModel(msg)
			if cmd != nil {
				cmds = append(cmds, cmd)
			}
		case stepConfig:
			var cmd tea.Cmd
			d, cmd = d.updateConfig(msg)
			if cmd != nil {
				cmds = append(cmds, cmd)
			}
		case stepDeploying:
			return d.updateDeploying(msg)
		}
	}

	if len(cmds) > 0 {
		return d, tea.Batch(cmds...)
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
			d.portInput.SetValue("8000")
		case wtLlamaCpp:
			d.portInput.SetValue("8080")
		case wtComfyUI:
			d.portInput.SetValue("8188")
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

func (d *Deploy) updateImage(msg tea.KeyMsg) (*Deploy, tea.Cmd) {
	options := d.imageOptions()

	// If no history and no typing, start in typing mode
	if len(options) == 0 {
		d.imageTyping = true
		d.imageInput.Focus()
	}

	if d.imageTyping {
		switch msg.String() {
		case "enter":
			if d.imageInput.Value() == "" {
				// Use default
				switch d.workload {
				case wtVLLM:
					d.imageInput.SetValue(d.cfg.Preferences.DefaultVLLMImage)
				case wtLlamaCpp:
					d.imageInput.SetValue(d.cfg.Preferences.DefaultLlamaImage)
				case wtComfyUI:
					d.imageInput.SetValue(d.cfg.Preferences.DefaultComfyImage)
				}
			}
			d.currentStep = stepModel
			d.cursor = 0
			d.imageTyping = false
			d.imageInput.Blur()
			return d, nil
		case "backspace":
			// Check if input is empty before forwarding to textinput
			if d.imageInput.Value() == "" && len(options) > 0 {
				d.imageTyping = false
				d.imageInput.Blur()
				return d, nil
			}
			// Forward to textinput
			var cmd tea.Cmd
			d.imageInput, cmd = d.imageInput.Update(msg)
			return d, cmd
		default:
			// Forward to textinput
			var cmd tea.Cmd
			d.imageInput, cmd = d.imageInput.Update(msg)
			return d, cmd
		}
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
			d.imageInput.SetValue("")
			d.imageInput.Focus()
			return d, nil
		}
		d.imageInput.SetValue(options[d.cursor])
		d.currentStep = stepModel
		d.cursor = 0
	case "/":
		// Switch to free-text typing
		d.imageTyping = true
		d.imageInput.SetValue("")
		d.imageInput.Focus()
	}
	return d, nil
}

func (d *Deploy) updateModel(msg tea.KeyMsg) (*Deploy, tea.Cmd) {
	if d.workload == wtComfyUI {
		if msg.String() == "enter" {
			d.modelInput.SetValue("")
			d.clearModelSearch()
			d.currentStep = stepConfig
			d.activeConfigField = 0
			d.portInput.Focus()
		}
		return d, nil
	}

	models := d.history.Models

	// If no history, start in typing mode
	if len(models) == 0 {
		d.modelTyping = true
		d.modelInput.Focus()
	}

	if d.modelTyping {
		switch msg.String() {
		case "up", "k":
			if len(d.modelSearchResults) > 0 && d.modelSearchCursor > 0 {
				d.modelSearchCursor--
				return d, nil
			}
		case "down", "j":
			if len(d.modelSearchResults) > 0 && d.modelSearchCursor < len(d.modelSearchResults)-1 {
				d.modelSearchCursor++
				return d, nil
			}
		case "pgdown":
			if len(d.modelSearchResults) > 0 {
				d.modelSearchCursor += 6
				if d.modelSearchCursor >= len(d.modelSearchResults) {
					d.modelSearchCursor = len(d.modelSearchResults) - 1
				}
				return d, nil
			}
		case "pgup":
			if len(d.modelSearchResults) > 0 {
				d.modelSearchCursor -= 6
				if d.modelSearchCursor < 0 {
					d.modelSearchCursor = 0
				}
				return d, nil
			}
		case "enter":
			if len(d.modelSearchResults) > 0 && d.modelSearchCursor < len(d.modelSearchResults) {
				d.modelInput.SetValue(d.modelSearchResults[d.modelSearchCursor].ID)
			}
			d.clearModelSearch()
			d.currentStep = stepConfig
			d.cursor = 0
			d.modelTyping = false
			d.modelInput.Blur()
			d.activeConfigField = 0
			d.portInput.Focus()
			return d, nil
		case "backspace":
			// Check if input is empty before forwarding to textinput
			if d.modelInput.Value() == "" && len(models) > 0 {
				d.modelTyping = false
				d.clearModelSearch()
				d.modelInput.Blur()
				return d, nil
			}
			// Forward to textinput
			var cmd tea.Cmd
			d.modelInput, cmd = d.modelInput.Update(msg)
			return d, tea.Batch(cmd, d.triggerModelSearch())
		default:
			// Forward to textinput
			var cmd tea.Cmd
			d.modelInput, cmd = d.modelInput.Update(msg)
			return d, tea.Batch(cmd, d.triggerModelSearch())
		}
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
			d.modelInput.SetValue("")
			d.clearModelSearch()
			d.modelInput.Focus()
			return d, nil
		}
		d.modelInput.SetValue(models[d.cursor])
		d.clearModelSearch()
		d.currentStep = stepConfig
		d.cursor = 0
		d.activeConfigField = 0
		d.portInput.Focus()
	case "/":
		d.modelTyping = true
		d.modelInput.SetValue("")
		d.clearModelSearch()
		d.modelInput.Focus()
	}
	return d, nil
}

func (d *Deploy) clearModelSearch() {
	d.modelSearchResults = nil
	d.modelSearchCursor = 0
	d.modelSearchLoading = false
	d.modelSearchErr = ""
	d.modelSearchQuery = ""
}

func (d *Deploy) triggerModelSearch() tea.Cmd {
	query := strings.TrimSpace(d.modelInput.Value())
	d.modelSearchErr = ""

	if d.cfg.HFToken == "" || query == "" || len(query) < 2 {
		d.modelSearchResults = nil
		d.modelSearchCursor = 0
		d.modelSearchLoading = false
		d.modelSearchQuery = query
		return nil
	}

	d.modelSearchRequest++
	requestID := d.modelSearchRequest
	d.modelSearchLoading = true
	d.modelSearchQuery = query
	workload := d.workload
	token := d.cfg.HFToken

	return func() tea.Msg {
		client := hf.NewClient(token)
		opts := hf.SearchOptions{Limit: 30}
		if workload == wtVLLM {
			opts.Filter = "text-generation"
		}
		results, err := client.SearchModelsWithOptions(query, opts)
		return hfSearchResultMsg{
			requestID: requestID,
			query:     query,
			results:   results,
			err:       err,
		}
	}
}

func (d *Deploy) rankModelSearchResults(results []hf.Model, query string) []hf.Model {
	ranked := append([]hf.Model(nil), results...)
	query = strings.ToLower(strings.TrimSpace(query))

	sort.SliceStable(ranked, func(i, j int) bool {
		left := d.modelSearchScore(ranked[i], query)
		right := d.modelSearchScore(ranked[j], query)
		if left != right {
			return left > right
		}
		if ranked[i].Downloads != ranked[j].Downloads {
			return ranked[i].Downloads > ranked[j].Downloads
		}
		return ranked[i].Likes > ranked[j].Likes
	})

	return ranked
}

func (d *Deploy) modelSearchScore(model hf.Model, query string) int {
	id := strings.ToLower(model.ID)
	score := 0

	if strings.HasPrefix(id, query) {
		score += 100
	}
	if strings.Contains(id, query) {
		score += 40
	}
	if strings.Contains(strings.ToLower(model.Author), query) {
		score += 15
	}

	hasGGUF := false
	for _, tag := range model.Tags {
		if strings.Contains(strings.ToLower(tag), "gguf") {
			hasGGUF = true
			break
		}
	}

	switch d.workload {
	case wtLlamaCpp:
		if hasGGUF || strings.Contains(id, "gguf") {
			score += 80
		}
		if model.Pipeline == "text-generation" {
			score += 15
		}
	case wtVLLM:
		if model.Pipeline == "text-generation" {
			score += 50
		}
		if hasGGUF || strings.Contains(id, "gguf") {
			score -= 20
		}
	}

	return score
}

func (d *Deploy) renderModelSearchResults() string {
	if d.cfg.HFToken == "" {
		return theme.MutedStyle.Render("Add your HuggingFace token to enable live model search.")
	}

	query := strings.TrimSpace(d.modelInput.Value())
	if query == "" {
		return theme.MutedStyle.Render("Type to search HuggingFace repos.")
	}
	if len(query) < 2 {
		return theme.MutedStyle.Render("Type at least 2 characters to search HuggingFace.")
	}
	if d.modelSearchLoading {
		return theme.MutedStyle.Render("Searching HuggingFace...")
	}
	if d.modelSearchErr != "" {
		return theme.WarnStyle.Render("HF search failed: " + d.modelSearchErr)
	}
	if len(d.modelSearchResults) == 0 {
		return theme.MutedStyle.Render("No HuggingFace matches yet. Press Enter to use the typed model ID.")
	}

	start := 0
	if d.modelSearchCursor > 3 {
		start = d.modelSearchCursor - 3
	}
	end := start + 6
	if end > len(d.modelSearchResults) {
		end = len(d.modelSearchResults)
	}
	if end-start < 6 && start > 0 {
		start = end - 6
		if start < 0 {
			start = 0
		}
	}

	body := theme.MutedStyle.Render(fmt.Sprintf("HuggingFace results %d-%d of %d", start+1, end, len(d.modelSearchResults))) + "\n\n"
	for i := start; i < end; i++ {
		model := d.modelSearchResults[i]
		cursor := "  "
		style := theme.PrimaryStyle
		metaStyle := theme.MutedStyle
		if i == d.modelSearchCursor {
			cursor = "> "
			style = lipgloss.NewStyle().Foreground(theme.Accent).Bold(true)
			metaStyle = lipgloss.NewStyle().Foreground(theme.Accent)
		}

		meta := []string{}
		if model.Pipeline != "" {
			meta = append(meta, model.Pipeline)
		}
		if model.Downloads > 0 {
			meta = append(meta, fmt.Sprintf("%d dl", model.Downloads))
		}
		if model.Likes > 0 {
			meta = append(meta, fmt.Sprintf("%d likes", model.Likes))
		}

		body += cursor + style.Render(model.ID) + "\n"
		if len(meta) > 0 {
			body += "  " + metaStyle.Render(strings.Join(meta, " • ")) + "\n"
		}
	}

	body += "\n" + theme.MutedStyle.Render("Type to refine • ↑/↓ to browse • Enter to select")
	return body
}

func (d *Deploy) updateConfig(msg tea.KeyMsg) (*Deploy, tea.Cmd) {
	switch msg.String() {
	case "tab":
		// Move focus to next field
		d.activeConfigField = (d.activeConfigField + 1) % 2
		if d.activeConfigField == 0 {
			d.portInput.Focus()
			d.extraArgsInput.Blur()
		} else {
			d.extraArgsInput.Focus()
			d.portInput.Blur()
		}
		return d, nil
	case "shift+tab":
		// Move focus to previous field
		d.activeConfigField = (d.activeConfigField - 1 + 2) % 2
		if d.activeConfigField == 0 {
			d.portInput.Focus()
			d.extraArgsInput.Blur()
		} else {
			d.extraArgsInput.Focus()
			d.portInput.Blur()
		}
		return d, nil
	case "up", "k":
		// Move focus up (to port field)
		d.activeConfigField = 0
		d.portInput.Focus()
		d.extraArgsInput.Blur()
		return d, nil
	case "down", "j":
		// Move focus down (to extra args field)
		d.activeConfigField = 1
		d.extraArgsInput.Focus()
		d.portInput.Blur()
		return d, nil
	case "enter":
		// Save to config first
		svc := config.Service{
			ID:       fmt.Sprintf("%s-%s", strings.ToLower(workloadLabels[d.workload]), sanitize(d.modelInput.Value())),
			DeviceID: d.cfg.Devices[d.deviceIdx].ID,
			Type:     strings.ToLower(workloadLabels[d.workload]),
			Image:    d.imageInput.Value(),
			Model:    d.modelInput.Value(),
			Port:     atoi(d.portInput.Value()),
		}
		d.cfg.Services = append(d.cfg.Services, svc)
		_ = config.Save(d.cfg)

		// Switch to deploying step and make API call
		d.currentStep = stepDeploying
		d.deployError = ""
		d.spinner = components.NewLoadingSpinner("Deploying container...")

		// Blur inputs
		d.portInput.Blur()
		d.extraArgsInput.Blur()

		return d, tea.Batch(d.deployToAPI(svc), d.spinner.Init())
	case "?":
		d.showArgsHelp = !d.showArgsHelp
		return d, nil
	default:
		// Forward to the active textinput
		var cmd tea.Cmd
		if d.activeConfigField == 0 {
			d.portInput, cmd = d.portInput.Update(msg)
		} else {
			d.extraArgsInput, cmd = d.extraArgsInput.Update(msg)
		}
		return d, cmd
	}
}

func (d *Deploy) updateDeploying(msg tea.KeyMsg) (View, tea.Cmd) {
	switch msg.String() {
	case "esc":
		// Allow cancelling during deployment
		return d, PopView()
	}
	return d, nil
}

func (d *Deploy) renderArgsHelp() string {
	label := "[+] Show extra arg help"
	if d.showArgsHelp {
		label = "[-] Hide extra arg help"
	}

	body := theme.MutedStyle.Render(label) + "\n\n"
	if !d.showArgsHelp {
		return body
	}

	switch d.workload {
	case wtVLLM:
		body += theme.MutedStyle.Render("How it works:") + "\n"
		body += "  Extra args are appended to the vLLM command after the image.\n"
		body += "  Use them for runtime flags like sequence length or tensor parallelism.\n\n"
		body += theme.MutedStyle.Render("Defaults added automatically:") + "\n"
		body += "  --model <selected model>\n"
		body += "  --host 0.0.0.0\n"
		body += "  --enable-auto-tool-choice\n"
		body += "  --tool-call-parser <inferred from model>\n\n"
		body += theme.MutedStyle.Render("Example:") + "\n"
		body += "  --tensor-parallel-size 2 --max-model-len 32768 --gpu-memory-utilization 0.95\n\n"
		body += theme.MutedStyle.Render("Note:") + "\n"
		body += "  Quote handling is limited, so enter plain space-separated flags.\n\n"
	default:
		body += theme.MutedStyle.Render("How it works:") + "\n"
		body += "  Extra args are appended to the container command after the image.\n"
		body += "  Use plain space-separated flags that your service understands.\n\n"
	}

	return body
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
			Ports:     map[string]string{d.portInput.Value(): d.portInput.Value()},
			Env:       map[string]string{},
			GPUIDs:    "all",
			ExtraArgs: d.extraArgsInput.Value(),
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
			if d.imageInput.Value() == "" {
				body += theme.MutedStyle.Render("Default: "+defaultImg) + "\n"
				body += theme.MutedStyle.Render("Press Enter to use default, or type a custom image") + "\n\n"
			}
			body += d.imageInput.View()
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
				body += d.modelInput.View()
				body += "\n\n" + d.renderModelSearchResults()
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
		body += theme.MutedStyle.Render("Port:") + "\n"
		body += d.portInput.View() + "\n\n"
		body += theme.MutedStyle.Render("Extra args:") + "\n"
		body += d.extraArgsInput.View() + "\n\n"
		body += theme.MutedStyle.Render("Tab to switch fields • ? to toggle arg help") + "\n\n"
		body += d.renderArgsHelp()

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
		body += fmt.Sprintf("  Image:  %s\n", d.imageInput.Value())
		if d.modelInput.Value() != "" {
			body += fmt.Sprintf("  Model:  %s\n", d.modelInput.Value())
		}
		body += fmt.Sprintf("  Port:   %s\n", d.portInput.Value())
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

func (d *Deploy) Name() string { return "Deploy" }

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
