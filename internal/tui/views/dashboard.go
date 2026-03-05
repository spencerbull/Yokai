package views

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	zone "github.com/lrstanley/bubblezone"
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
	gpuHistory map[string][]float64 // keyed by "deviceID-gpuIndex"
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
	TempC       int    `json:"temperature_c"`
	UtilPercent int    `json:"utilization_percent"`
	VRAMUsedMB  int64  `json:"vram_used_mb"`
	VRAMTotalMB int64  `json:"vram_total_mb"`
	PowerDrawW  int    `json:"power_draw_w"`
	PowerLimitW int    `json:"power_limit_w"`
	FanPercent  int    `json:"fan_percent"`
}

type ContainerData struct {
	ID            string            `json:"id"`
	Name          string            `json:"name"`
	Image         string            `json:"image"`
	Status        string            `json:"status"`
	CPUPercent    float64           `json:"cpu_percent"`
	MemUsedMB     int64             `json:"memory_used_mb"`
	GPUMemMB      int64             `json:"gpu_memory_mb"`
	UptimeSeconds int64             `json:"uptime_seconds"`
	Ports         map[string]string `json:"ports"`
	Health        string            `json:"health"`
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
		gpuHistory: make(map[string][]float64),
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
		if d.width > theme.MaxContentWidth-2*theme.ContentPadding {
			d.width = theme.MaxContentWidth - 2*theme.ContentPadding
		}
		d.height = msg.Height

	case MetricsMsg:
		if msg.Error != nil {
			d.lastError = msg.Error
		} else {
			d.metrics = msg.Metrics
			d.updateSparklineData()
			d.updateServiceContainers()
			d.enrichDevices()
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
			d.enrichDevices()
			d.lastError = nil
		}

	case tickMsg:
		return d, tea.Batch(d.pollMetrics(), d.pollDevices())

	case tea.MouseMsg:
		if msg.Action == tea.MouseActionRelease {
			// Check if a service row was clicked
			for i := range d.serviceContainers {
				if zone.Get(components.ServiceRowZoneID(i)).InBounds(msg) {
					d.selectedService = i
					break
				}
			}
		}

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
				containerID := container.ID
				name := container.Name
				msg := fmt.Sprintf("Stop service %q?", name)
				return d, Navigate(NewConfirmView(msg, d.stopService(containerID), nil))
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

	// Guard: if we haven't received a WindowSizeMsg yet, use a sensible default.
	if d.width == 0 {
		d.width = theme.MaxContentWidth - 2*theme.ContentPadding
	}

	var sections []string

	// Header
	header := d.renderHeader()
	sections = append(sections, header)

	// Wide layout (>= 100 chars): btop-inspired grid
	// Narrow layout (< 100 chars): single column stack
	if d.width >= 100 {
		sections = append(sections, d.renderGridLayout()...)
	} else {
		sections = append(sections, d.renderStackedLayout()...)
	}

	// Service list (always full width, bottom)
	serviceList := d.renderServiceList()
	sections = append(sections, serviceList)

	// Use Left alignment so the outer lipgloss.Place() in app.go handles centering.
	// Wrap at d.width so the assembled block has the correct width for centering.
	content := lipgloss.JoinVertical(lipgloss.Left, sections...)
	return lipgloss.NewStyle().Width(d.width).Render(content)
}

// renderGridLayout renders btop-inspired side-by-side panels.
func (d *Dashboard) renderGridLayout() []string {
	var sections []string

	// Top row: Device cards side-by-side
	if len(d.devices) > 0 {
		deviceCards := d.renderDeviceCards()
		sections = append(sections, deviceCards)
	}

	// Middle row: CPU/RAM charts (left) | GPU panels (right)
	leftWidth := (d.width - 3) / 2
	rightWidth := d.width - leftWidth - 3

	leftCol := d.renderSparklines2(leftWidth)
	rightCol := d.renderGPUPanels2(rightWidth)

	if leftCol != "" && rightCol != "" {
		leftStyled := lipgloss.NewStyle().Width(leftWidth).Render(leftCol)
		rightStyled := lipgloss.NewStyle().Width(rightWidth).Render(rightCol)

		middleRow := lipgloss.JoinHorizontal(lipgloss.Top, leftStyled, " ", rightStyled)
		sections = append(sections, middleRow)
	} else if leftCol != "" {
		sections = append(sections, leftCol)
	} else if rightCol != "" {
		sections = append(sections, rightCol)
	}

	return sections
}

// renderStackedLayout renders single-column layout for narrow terminals.
func (d *Dashboard) renderStackedLayout() []string {
	var sections []string

	if len(d.devices) > 0 {
		deviceCards := d.renderDeviceCards()
		sections = append(sections, deviceCards)
	}

	gpuPanels := d.renderGPUPanels()
	if gpuPanels != "" {
		sections = append(sections, gpuPanels)
	}

	sparklines := d.renderSparklines()
	if sparklines != "" {
		sections = append(sections, sparklines)
	}

	return sections
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
	if cardWidth < 20 {
		cardWidth = 20
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
	return d.renderGPUPanels2(d.width)
}

func (d *Dashboard) renderGPUPanels2(availableWidth int) string {
	if len(d.metrics) == 0 {
		return ""
	}

	var panels []string
	panelWidth := availableWidth - 2
	if panelWidth < 40 {
		panelWidth = 40
	}

	chartWidth := panelWidth - 4
	if chartWidth < 20 {
		chartWidth = 20
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

			// GPU utilization history chart
			historyKey := fmt.Sprintf("%s-%d", deviceID, gpu.Index)
			if gpuVals, ok := d.gpuHistory[historyKey]; ok && len(gpuVals) > 0 {
				label := fmt.Sprintf(" GPU %d Util %d%%", gpu.Index, gpu.UtilPercent)
				gpuTitle := lipgloss.NewStyle().Foreground(theme.Accent).Render(label)
				gpuChart := components.NewStreamChart("GPU Util", gpuVals, chartWidth, 6, theme.Accent)
				chartPanel := theme.RenderPanel(gpuTitle, gpuChart.Render(), panelWidth)
				panels = append(panels, chartPanel)
			}
		}
	}

	if len(panels) == 0 {
		return ""
	}

	return lipgloss.JoinVertical(lipgloss.Left, panels...)
}

func (d *Dashboard) renderSparklines() string {
	return d.renderSparklines2(d.width)
}

func (d *Dashboard) renderSparklines2(availableWidth int) string {
	if len(d.cpuHistory) == 0 && len(d.ramHistory) == 0 {
		return ""
	}

	// Panel border + padding takes 4 chars (2 border + 2 padding),
	// then the chart Y-axis labels take ~6 chars
	panelWidth := availableWidth - 2
	chartWidth := panelWidth - 4 // inner content after border+padding
	if chartWidth < 20 {
		chartWidth = 20
	}
	chartHeight := 6

	var charts []string

	for deviceID := range d.cpuHistory {
		device := d.findDevice(deviceID)
		if device == nil {
			continue
		}

		// CPU streamline chart
		if cpuVals, ok := d.cpuHistory[deviceID]; ok && len(cpuVals) > 0 {
			label := fmt.Sprintf(" %s CPU %.0f%%", device.Label, cpuVals[len(cpuVals)-1])
			cpuTitle := theme.GoodStyle.Render(label)
			cpu := components.NewStreamChart("CPU", cpuVals, chartWidth, chartHeight, theme.Good)
			cpuPanel := theme.RenderPanel(cpuTitle, cpu.Render(), panelWidth)
			charts = append(charts, cpuPanel)
		}

		// RAM streamline chart
		if ramVals, ok := d.ramHistory[deviceID]; ok && len(ramVals) > 0 {
			label := fmt.Sprintf(" %s RAM %.0f%%", device.Label, ramVals[len(ramVals)-1])
			ramTitle := theme.WarnStyle.Render(label)
			ram := components.NewStreamChart("RAM", ramVals, chartWidth, chartHeight, theme.Accent)
			ramPanel := theme.RenderPanel(ramTitle, ram.Render(), panelWidth)
			charts = append(charts, ramPanel)
		}
	}

	if len(charts) == 0 {
		return ""
	}

	return lipgloss.JoinVertical(lipgloss.Left, charts...)
}

func (d *Dashboard) renderServiceList() string {
	serviceRows := d.buildServiceRows()

	serviceList := components.NewServiceList(serviceRows, d.width-4)
	serviceList.Cursor = d.selectedService

	title := theme.TitleStyle.Render("Services")
	content := serviceList.Render()

	return theme.RenderPanel(title, content, d.width)
}

func (d *Dashboard) buildServiceRows() []components.ServiceRow {
	var rows []components.ServiceRow

	// Build from container data in metrics
	for deviceID, metrics := range d.metrics {
		device := d.findDevice(deviceID)
		if device == nil {
			continue
		}

		deviceLabel := device.Label
		if deviceLabel == "" {
			deviceLabel = device.ID
		}

		for _, container := range metrics.Containers {
			serviceType := d.inferServiceType(container.Name)

			// Strip the "yokai-" prefix for cleaner display
			displayName := strings.TrimPrefix(container.Name, "yokai-")

			row := components.ServiceRow{
				Name:       displayName,
				Type:       serviceType,
				Model:      d.getServiceModel(container),
				Status:     container.Status,
				Health:     container.Health,
				Device:     deviceLabel,
				Port:       extractExternalPort(container.Ports),
				CPUPercent: container.CPUPercent,
				MemUsedMB:  container.MemUsedMB,
				GPUMemMB:   container.GPUMemMB,
				Uptime:     formatUptime(container.UptimeSeconds),
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

		var envelope struct {
			Devices []DashboardDevice `json:"devices"`
		}
		if err := json.NewDecoder(resp.Body).Decode(&envelope); err != nil {
			return DevicesMsg{Error: err}
		}

		return DevicesMsg{Devices: envelope.Devices}
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

		// Update GPU history
		for _, gpu := range metrics.GPUs {
			key := fmt.Sprintf("%s-%d", deviceID, gpu.Index)
			if _, ok := d.gpuHistory[key]; !ok {
				d.gpuHistory[key] = make([]float64, 0, d.maxSamples)
			}
			d.gpuHistory[key] = append(d.gpuHistory[key], float64(gpu.UtilPercent))
			if len(d.gpuHistory[key]) > d.maxSamples {
				d.gpuHistory[key] = d.gpuHistory[key][1:]
			}
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

func (d *Dashboard) enrichDevices() {
	for i := range d.devices {
		dev := &d.devices[i]
		m, ok := d.metrics[dev.ID]
		if !ok {
			continue
		}
		dev.GPUCount = len(m.GPUs)
		dev.CPUPercent = m.CPU.Percent
		dev.RAMPercent = m.RAM.Percent
		dev.ServiceCount = len(m.Containers)
	}
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

func (d *Dashboard) getServiceModel(container ContainerData) string {
	// Try matching by container ID first
	for _, service := range d.cfg.Services {
		if service.ContainerID != "" && service.ContainerID == container.ID {
			if service.Model != "" {
				return service.Model
			}
		}
	}

	// Fall back to matching by name. The container name is "yokai-<service-id>"
	// and the config service ID matches the suffix.
	containerName := strings.TrimPrefix(container.Name, "yokai-")
	for _, service := range d.cfg.Services {
		if service.ID != "" && service.ID == containerName {
			if service.Model != "" {
				return service.Model
			}
		}
	}

	// Try to extract model from image name as last resort
	if container.Image != "" {
		// e.g. "vllm/vllm-openai:latest" → not useful, but a custom image might encode the model
		return ""
	}

	return ""
}

// extractExternalPort returns the first external (host) port from the port map,
// or 0 if none are mapped.
func extractExternalPort(ports map[string]string) int {
	for _, external := range ports {
		return atoi(external)
	}
	return 0
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

func formatUptime(seconds int64) string {
	if seconds <= 0 {
		return "-"
	}
	d := time.Duration(seconds) * time.Second
	hours := int(d.Hours())
	mins := int(d.Minutes()) % 60

	if hours >= 24 {
		days := hours / 24
		return fmt.Sprintf("%dd%dh", days, hours%24)
	}
	if hours > 0 {
		return fmt.Sprintf("%dh%dm", hours, mins)
	}
	return fmt.Sprintf("%dm", mins)
}

func (d *Dashboard) InputActive() bool { return false }

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
