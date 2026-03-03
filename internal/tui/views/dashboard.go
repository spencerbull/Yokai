package views

import (
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

// Dashboard is the main btop-style monitoring view.
type Dashboard struct {
	cfg     *config.Config
	version string
	width   int
	height  int

	// Service selection
	selectedService int

	// Live metrics data
	metrics   map[string]*DashboardMetrics
	devices   []DashboardDevice
	lastError error

	// Service tracking for operations
	serviceContainers []ContainerData // flattened list for operations

	// Sparkline data - maintain 60 samples for 2-minute history
	cpuHistory map[string][]float64 // keyed by device ID
	ramHistory map[string][]float64
	maxSamples int
}

// Custom messages for daemon communication
type MetricsMsg struct {
	Metrics map[string]*DashboardMetrics
	Error   error
}

type DevicesMsg struct {
	Devices []DashboardDevice
	Error   error
}

type tickMsg struct{}

// DashboardMetrics represents the metrics data from the daemon API
type DashboardMetrics struct {
	CPU        CPUData         `json:"cpu"`
	RAM        RAMData         `json:"ram"`
	GPUs       []GPUData       `json:"gpus"`
	Containers []ContainerData `json:"containers"`
}

type CPUData struct {
	Percent float64 `json:"percent"`
}

type RAMData struct {
	UsedMB  int64   `json:"used_mb"`
	TotalMB int64   `json:"total_mb"`
	Percent float64 `json:"percent"`
}

type GPUData struct {
	Index       int    `json:"index"`
	Name        string `json:"name"`
	TempC       int    `json:"temp_c"`
	UtilPercent int    `json:"util_percent"`
	VRAMUsedMB  int64  `json:"vram_used_mb"`
	VRAMTotalMB int64  `json:"vram_total_mb"`
	PowerDrawW  int    `json:"power_draw_w"`
	PowerLimitW int    `json:"power_limit_w"`
	FanPercent  int    `json:"fan_percent"`
}

type ContainerData struct {
	ID         string  `json:"id"`
	Name       string  `json:"name"`
	Status     string  `json:"status"`
	CPUPercent float64 `json:"cpu_percent"`
	MemUsedMB  int64   `json:"mem_used_mb"`
	GPUMemMB   int64   `json:"gpu_mem_mb"`
	Uptime     string  `json:"uptime"`
	Port       int     `json:"port"`
}

type DashboardDevice struct {
	ID           string  `json:"id"`
	Label        string  `json:"label"`
	Host         string  `json:"host"`
	Online       bool    `json:"online"`
	GPUType      string  `json:"gpu_type"`
	GPUCount     int     `json:"gpu_count"`
	CPUPercent   float64 `json:"cpu_percent"`
	RAMPercent   float64 `json:"ram_percent"`
	ServiceCount int     `json:"service_count"`
}

// NewDashboard creates the dashboard view.
func NewDashboard(cfg *config.Config, version string) *Dashboard {
	return &Dashboard{
		cfg:        cfg,
		version:    version,
		metrics:    make(map[string]*DashboardMetrics),
		devices:    []DashboardDevice{},
		cpuHistory: make(map[string][]float64),
		ramHistory: make(map[string][]float64),
		maxSamples: 60, // 60 samples * 2s = 2 minutes of history
	}
}

func (d *Dashboard) Init() tea.Cmd {
	return tea.Batch(d.pollMetrics(), d.pollDevices())
}

func (d *Dashboard) Update(msg tea.Msg) (View, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		d.width = msg.Width
		d.height = msg.Height

	case MetricsMsg:
		if msg.Error != nil {
			d.lastError = msg.Error
		} else {
			d.metrics = msg.Metrics
			d.updateSparklineData()
			d.updateServiceContainers()
			d.lastError = nil
		}
		return d, tea.Tick(2*time.Second, func(time.Time) tea.Msg {
			return tickMsg{}
		})

	case DevicesMsg:
		if msg.Error != nil {
			d.lastError = msg.Error
		} else {
			d.devices = msg.Devices
			d.lastError = nil
		}

	case tickMsg:
		return d, tea.Batch(d.pollMetrics(), d.pollDevices())

	case tea.KeyMsg:
		switch msg.String() {
		case "q":
			return d, tea.Quit
		case "n":
			return d, Navigate(NewDeploy(d.cfg, d.version))
		case "d":
			return d, Navigate(NewDeviceManager(d.cfg, d.version))
		case "c":
			return d, Navigate(NewAITools(d.cfg, d.version))
		case "?":
			return d, Navigate(NewHelp(d.version))
		case "j", "down":
			d.moveServiceCursor(1)
		case "k", "up":
			d.moveServiceCursor(-1)
		case "enter", "l":
			if container := d.getSelectedContainer(); container != nil {
				// Find device ID for this container
				deviceID := d.findDeviceIDForContainer(container.ID)
				return d, Navigate(NewLogViewer(d.cfg, d.version, container.Name, deviceID, container.ID))
			}
		case "s":
			if container := d.getSelectedContainer(); container != nil {
				return d, d.stopService(container.ID)
			}
		case "r":
			if container := d.getSelectedContainer(); container != nil {
				return d, d.restartService(container.ID)
			}
		case "g":
			// Open Grafana - could implement later
			return d, nil
		}
	}
	return d, nil
}

func (d *Dashboard) View() string {
	if d.lastError != nil {
		return d.renderError()
	}

	var sections []string

	// Header
	header := d.renderHeader()
	sections = append(sections, header)

	// Device cards
	if len(d.devices) > 0 {
		deviceCards := d.renderDeviceCards()
		sections = append(sections, deviceCards)
	}

	// GPU panels
	gpuPanels := d.renderGPUPanels()
	if gpuPanels != "" {
		sections = append(sections, gpuPanels)
	}

	// CPU/RAM sparklines
	sparklines := d.renderSparklines()
	if sparklines != "" {
		sections = append(sections, sparklines)
	}

	// Service list
	serviceList := d.renderServiceList()
	sections = append(sections, serviceList)

	return lipgloss.JoinVertical(lipgloss.Left, sections...)
}

func (d *Dashboard) renderError() string {
	errorMsg := "Daemon not running. Start with: yokai daemon"
	if d.lastError != nil {
		errorMsg = fmt.Sprintf("Error connecting to daemon: %v", d.lastError)
	}

	return lipgloss.NewStyle().
		Foreground(theme.Crit).
		Bold(true).
		Render(errorMsg)
}

func (d *Dashboard) renderHeader() string {
	deviceCount := len(d.devices)
	serviceCount := 0
	for _, device := range d.devices {
		serviceCount += device.ServiceCount
	}

	headerText := fmt.Sprintf("yokai — %d device(s), %d service(s)  v%s",
		deviceCount, serviceCount, d.version)

	return lipgloss.NewStyle().
		Foreground(theme.Accent).
		Bold(true).
		Render(headerText)
}

func (d *Dashboard) renderDeviceCards() string {
	if len(d.devices) == 0 {
		return ""
	}

	cards := make([]string, len(d.devices))
	var cardWidth int

	// If width > 120, show side-by-side; otherwise stacked
	if d.width > 120 {
		cardWidth = (d.width - 4) / 2 // 2 cards side by side with spacing
	} else {
		cardWidth = d.width - 4
	}

	for i, device := range d.devices {
		card := components.NewDeviceCard(
			device.Label,
			device.Host,
			device.Online,
			device.GPUType,
			device.GPUCount,
			device.CPUPercent,
			device.RAMPercent,
			device.ServiceCount,
			cardWidth,
		)
		cards[i] = card.Render()
	}

	if d.width > 120 && len(cards) >= 2 {
		// Arrange side by side
		var rows []string
		for i := 0; i < len(cards); i += 2 {
			if i+1 < len(cards) {
				row := lipgloss.JoinHorizontal(lipgloss.Top, cards[i], "  ", cards[i+1])
				rows = append(rows, row)
			} else {
				rows = append(rows, cards[i])
			}
		}
		return lipgloss.JoinVertical(lipgloss.Left, rows...)
	}

	// Stack vertically
	return lipgloss.JoinVertical(lipgloss.Left, cards...)
}

func (d *Dashboard) renderGPUPanels() string {
	if len(d.metrics) == 0 {
		return ""
	}

	var panels []string
	panelWidth := d.width - 4
	if panelWidth < 40 {
		panelWidth = 40
	}

	for deviceID, metrics := range d.metrics {
		device := d.findDevice(deviceID)
		if device == nil {
			continue
		}

		for _, gpu := range metrics.GPUs {
			panel := components.NewGPUPanel(
				gpu.Index,
				gpu.Name,
				gpu.TempC,
				gpu.UtilPercent,
				gpu.VRAMUsedMB,
				gpu.VRAMTotalMB,
				gpu.PowerDrawW,
				gpu.PowerLimitW,
				gpu.FanPercent,
				panelWidth,
			)
			panels = append(panels, panel.Render())
		}
	}

	if len(panels) == 0 {
		return ""
	}

	return lipgloss.JoinVertical(lipgloss.Left, panels...)
}

func (d *Dashboard) renderSparklines() string {
	if len(d.cpuHistory) == 0 && len(d.ramHistory) == 0 {
		return ""
	}

	sparklineWidth := d.width - 20 // leave space for labels
	if sparklineWidth < 20 {
		sparklineWidth = 20
	}

	var lines []string

	for deviceID := range d.cpuHistory {
		device := d.findDevice(deviceID)
		if device == nil {
			continue
		}

		// CPU sparkline
		if cpuVals, ok := d.cpuHistory[deviceID]; ok && len(cpuVals) > 0 {
			cpu := components.NewSparkline(cpuVals, sparklineWidth, theme.Good)
			cpuLine := fmt.Sprintf("%s CPU %s", device.Label, cpu.Render())
			lines = append(lines, cpuLine)
		}

		// RAM sparkline
		if ramVals, ok := d.ramHistory[deviceID]; ok && len(ramVals) > 0 {
			ram := components.NewSparkline(ramVals, sparklineWidth, theme.Warn)
			ramLine := fmt.Sprintf("%s RAM %s", device.Label, ram.Render())
			lines = append(lines, ramLine)
		}
	}

	if len(lines) == 0 {
		return ""
	}

	return lipgloss.JoinVertical(lipgloss.Left, lines...)
}

func (d *Dashboard) renderServiceList() string {
	serviceRows := d.buildServiceRows()

	serviceList := components.NewServiceList(serviceRows, d.width-4)
	serviceList.Cursor = d.selectedService

	title := theme.TitleStyle.Render("Services")
	content := serviceList.Render()

	return theme.Panel(title).Width(d.width - 2).Render(content)
}

func (d *Dashboard) buildServiceRows() []components.ServiceRow {
	var rows []components.ServiceRow

	// Build from container data in metrics
	for deviceID, metrics := range d.metrics {
		device := d.findDevice(deviceID)
		if device == nil {
			continue
		}

		for _, container := range metrics.Containers {
			// Map container to service type - this is a simple heuristic
			serviceType := d.inferServiceType(container.Name)

			row := components.ServiceRow{
				Name:       container.Name,
				Type:       serviceType,
				Model:      d.getServiceModel(container.ID),
				Status:     container.Status,
				Port:       container.Port,
				CPUPercent: container.CPUPercent,
				MemUsedMB:  container.MemUsedMB,
				GPUMemMB:   container.GPUMemMB,
				Uptime:     container.Uptime,
			}
			rows = append(rows, row)
		}
	}

	return rows
}

func (d *Dashboard) pollMetrics() tea.Cmd {
	return func() tea.Msg {
		daemonURL := fmt.Sprintf("http://%s/metrics", d.cfg.Daemon.Listen)
		resp, err := http.Get(daemonURL)
		if err != nil {
			return MetricsMsg{Error: err}
		}
		defer func() {
			_ = resp.Body.Close() // Best-effort close of metrics response body.
		}()

		var metrics map[string]*DashboardMetrics
		if err := json.NewDecoder(resp.Body).Decode(&metrics); err != nil {
			return MetricsMsg{Error: err}
		}

		return MetricsMsg{Metrics: metrics}
	}
}

func (d *Dashboard) pollDevices() tea.Cmd {
	return func() tea.Msg {
		daemonURL := fmt.Sprintf("http://%s/devices", d.cfg.Daemon.Listen)
		resp, err := http.Get(daemonURL)
		if err != nil {
			return DevicesMsg{Error: err}
		}
		defer func() {
			_ = resp.Body.Close() // Best-effort close of devices response body.
		}()

		var devices []DashboardDevice
		if err := json.NewDecoder(resp.Body).Decode(&devices); err != nil {
			return DevicesMsg{Error: err}
		}

		return DevicesMsg{Devices: devices}
	}
}

func (d *Dashboard) updateSparklineData() {
	for deviceID, metrics := range d.metrics {
		// Update CPU history
		if _, ok := d.cpuHistory[deviceID]; !ok {
			d.cpuHistory[deviceID] = make([]float64, 0, d.maxSamples)
		}
		d.cpuHistory[deviceID] = append(d.cpuHistory[deviceID], metrics.CPU.Percent)
		if len(d.cpuHistory[deviceID]) > d.maxSamples {
			d.cpuHistory[deviceID] = d.cpuHistory[deviceID][1:]
		}

		// Update RAM history
		if _, ok := d.ramHistory[deviceID]; !ok {
			d.ramHistory[deviceID] = make([]float64, 0, d.maxSamples)
		}
		d.ramHistory[deviceID] = append(d.ramHistory[deviceID], metrics.RAM.Percent)
		if len(d.ramHistory[deviceID]) > d.maxSamples {
			d.ramHistory[deviceID] = d.ramHistory[deviceID][1:]
		}
	}
}

func (d *Dashboard) moveServiceCursor(delta int) {
	serviceCount := len(d.serviceContainers)
	if serviceCount == 0 {
		return
	}

	d.selectedService += delta
	if d.selectedService < 0 {
		d.selectedService = 0
	}
	if d.selectedService >= serviceCount {
		d.selectedService = serviceCount - 1
	}
}

func (d *Dashboard) getSelectedContainer() *ContainerData {
	if d.selectedService >= 0 && d.selectedService < len(d.serviceContainers) {
		return &d.serviceContainers[d.selectedService]
	}
	return nil
}

func (d *Dashboard) updateServiceContainers() {
	d.serviceContainers = nil
	for _, metrics := range d.metrics {
		d.serviceContainers = append(d.serviceContainers, metrics.Containers...)
	}
}

func (d *Dashboard) stopService(containerID string) tea.Cmd {
	return func() tea.Msg {
		deviceID := d.findDeviceIDForContainer(containerID)
		if deviceID == "" {
			return nil
		}
		url := fmt.Sprintf("http://%s/containers/%s/%s/stop", d.cfg.Daemon.Listen, deviceID, containerID)
		resp, err := http.Post(url, "application/json", nil)
		if err != nil {
			return nil // Could return error message
		}
		_ = resp.Body.Close() // Best-effort close of stop response body.
		return nil
	}
}

func (d *Dashboard) restartService(containerID string) tea.Cmd {
	return func() tea.Msg {
		deviceID := d.findDeviceIDForContainer(containerID)
		if deviceID == "" {
			return nil
		}
		url := fmt.Sprintf("http://%s/containers/%s/%s/restart", d.cfg.Daemon.Listen, deviceID, containerID)
		resp, err := http.Post(url, "application/json", nil)
		if err != nil {
			return nil // Could return error message
		}
		_ = resp.Body.Close() // Best-effort close of restart response body.
		return nil
	}
}

func (d *Dashboard) findDevice(deviceID string) *DashboardDevice {
	for i := range d.devices {
		if d.devices[i].ID == deviceID {
			return &d.devices[i]
		}
	}
	return nil
}

func (d *Dashboard) inferServiceType(containerName string) string {
	containerName = strings.ToLower(containerName)
	if strings.Contains(containerName, "vllm") {
		return "vllm"
	}
	if strings.Contains(containerName, "llama") {
		return "llamacpp"
	}
	if strings.Contains(containerName, "comfyui") || strings.Contains(containerName, "comfy") {
		return "comfyui"
	}
	return "unknown"
}

func (d *Dashboard) getServiceModel(containerID string) string {
	// Try to find model from config by matching container ID
	for _, service := range d.cfg.Services {
		if service.ContainerID == containerID {
			return service.Model
		}
	}
	return "unknown"
}

func (d *Dashboard) findDeviceIDForContainer(containerID string) string {
	// Find which device hosts this container
	for deviceID, metrics := range d.metrics {
		for _, container := range metrics.Containers {
			if container.ID == containerID {
				return deviceID
			}
		}
	}
	return ""
}

func (d *Dashboard) KeyBinds() []KeyBind {
	return []KeyBind{
		{Key: "n", Help: "new"},
		{Key: "s", Help: "stop"},
		{Key: "r", Help: "restart"},
		{Key: "l", Help: "logs"},
		{Key: "d", Help: "devices"},
		{Key: "g", Help: "grafana"},
		{Key: "c", Help: "ai tools"},
		{Key: "j/k", Help: "navigate"},
		{Key: "?", Help: "help"},
		{Key: "q", Help: "quit"},
	}
}
