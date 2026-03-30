package views

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	zone "github.com/lrstanley/bubblezone"
	"github.com/spencerbull/yokai/internal/config"
	"github.com/spencerbull/yokai/internal/tui/components"
	"github.com/spencerbull/yokai/internal/tui/theme"
)

type dashboardMode int

const (
	dashboardOverview dashboardMode = iota
	dashboardDeviceDetail
)

type overviewFocus int

const (
	overviewFocusDevices overviewFocus = iota
	overviewFocusAIServices
	overviewFocusMonitoringServices
)

const (
	overviewAIServiceZonePrefix         = "overview-ai"
	overviewMonitoringServiceZonePrefix = "overview-monitoring"
)

// Dashboard is the main btop-style monitoring view.
type Dashboard struct {
	cfg     *config.Config
	version string
	width   int
	height  int

	mode           dashboardMode
	selectedDevice int
	overviewFocus  overviewFocus

	selectedAIServiceID         string
	selectedMonitoringServiceID string

	// Service selection
	selectedService   int
	showServiceDetail bool

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

type serviceStopMsg struct {
	err error
}

type serviceRestartMsg struct {
	err error
}

type serviceDeleteMsg struct {
	containerID string
	serviceID   string
	err         error
}

type serviceTestMsg struct {
	resultMessage string
	err           error
}

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
	ID                  string            `json:"id"`
	Name                string            `json:"name"`
	Image               string            `json:"image"`
	Status              string            `json:"status"`
	CPUPercent          float64           `json:"cpu_percent"`
	MemUsedMB           int64             `json:"memory_used_mb"`
	GPUMemMB            int64             `json:"gpu_memory_mb"`
	UptimeSeconds       int64             `json:"uptime_seconds"`
	Ports               map[string]string `json:"ports"`
	Health              string            `json:"health"`
	GenerationTokPerSec float64           `json:"generation_tok_per_s"`
	PromptTokPerSec     float64           `json:"prompt_tok_per_s"`
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
	zone.NewGlobal()
	return &Dashboard{
		cfg:             cfg,
		version:         version,
		mode:            dashboardOverview,
		overviewFocus:   overviewFocusDevices,
		metrics:         make(map[string]*DashboardMetrics),
		devices:         []DashboardDevice{},
		cpuHistory:      make(map[string][]float64),
		ramHistory:      make(map[string][]float64),
		gpuHistory:      make(map[string][]float64),
		maxSamples:      60, // 60 samples * 2s = 2 minutes of history
		selectedService: 0,
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
			d.enrichDevices()
			d.clampSelectedDevice()
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
			d.enrichDevices()
			d.clampSelectedDevice()
			d.updateServiceContainers()
			d.lastError = nil
		}

	case tickMsg:
		return d, tea.Batch(d.pollMetrics(), d.pollDevices())

	case serviceStopMsg:
		if msg.err != nil {
			return d, ShowToast("Stop failed: "+msg.err.Error(), ToastError)
		}
		return d, ShowToast("Service stopped", ToastSuccess)

	case serviceRestartMsg:
		if msg.err != nil {
			return d, ShowToast("Restart failed: "+msg.err.Error(), ToastError)
		}
		return d, ShowToast("Service restarting...", ToastInfo)

	case serviceDeleteMsg:
		if msg.err != nil {
			return d, ShowToast("Delete failed: "+msg.err.Error(), ToastError)
		}

		removed := d.cfg.RemoveServiceByContainerID(msg.containerID)
		if removed == 0 && msg.serviceID != "" {
			removed = d.cfg.RemoveServiceByID(msg.serviceID)
		}
		if removed > 0 {
			if err := config.Save(d.cfg); err != nil {
				return d, ShowToast("Deleted container, but failed to save config: "+err.Error(), ToastError)
			}
		}

		return d, ShowToast("Service deleted", ToastSuccess)

	case serviceTestMsg:
		if msg.err != nil {
			return d, ShowToast("Test failed: "+compactServiceTestError(msg.err), ToastError)
		}
		return d, ShowToast(msg.resultMessage, ToastSuccess)

	case tea.MouseMsg:
		if msg.Action == tea.MouseActionRelease {
			if d.mode == dashboardOverview {
				for i := range d.devices {
					if zone.Get(overviewPickerZoneID(i)).InBounds(msg) {
						d.selectedDevice = i
						d.overviewFocus = overviewFocusDevices
						d.showServiceDetail = false
						break
					}
				}
				for i := range d.devices {
					if zone.Get(dashboardDeviceZoneID(i)).InBounds(msg) {
						d.selectedDevice = i
						d.overviewFocus = overviewFocusDevices
						d.showServiceDetail = false
						break
					}
				}
				for i, container := range d.visibleOverviewServiceContainersForFocus(overviewFocusAIServices) {
					if zone.Get(components.ServiceRowZoneIDWithPrefix(overviewAIServiceZonePrefix, i)).InBounds(msg) {
						d.overviewFocus = overviewFocusAIServices
						d.selectedAIServiceID = container.ID
						break
					}
				}
				for i, container := range d.visibleOverviewServiceContainersForFocus(overviewFocusMonitoringServices) {
					if zone.Get(components.ServiceRowZoneIDWithPrefix(overviewMonitoringServiceZonePrefix, i)).InBounds(msg) {
						d.overviewFocus = overviewFocusMonitoringServices
						d.selectedMonitoringServiceID = container.ID
						break
					}
				}
			} else {
				for i := range d.serviceContainers {
					if zone.Get(components.ServiceRowZoneID(i)).InBounds(msg) {
						d.selectedService = i
						break
					}
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
		case "esc":
			if d.showServiceDetail {
				d.showServiceDetail = false
				return d, nil
			}
			if d.mode == dashboardDeviceDetail {
				d.mode = dashboardOverview
				d.selectedService = 0
				d.updateServiceContainers()
				return d, nil
			}
			if d.mode == dashboardOverview && d.overviewFocus != overviewFocusDevices {
				d.overviewFocus = overviewFocusDevices
				return d, nil
			}
		case "g":
			// Open Grafana - could implement later
			return d, nil
		}

		if container := d.getSelectedContainer(); container != nil {
			switch msg.String() {
			case "L":
				deviceID := d.findDeviceIDForContainer(container.ID)
				return d, Navigate(NewLogViewer(d.cfg, d.version, container.Name, deviceID, container.ID))
			case "s":
				containerID := container.ID
				name := container.Name
				confirmMsg := fmt.Sprintf("Stop service %q?", name)
				return d, Navigate(NewConfirmView(confirmMsg, d.stopService(containerID), nil))
			case "r":
				return d, d.restartService(container.ID)
			case "x":
				containerID := container.ID
				serviceID := strings.TrimPrefix(container.Name, "yokai-")
				confirmMsg := fmt.Sprintf("Delete service %q? This stops and removes the container.", container.Name)
				return d, Navigate(NewConfirmView(confirmMsg, d.deleteService(containerID, serviceID), nil))
			case "t":
				return d, d.testService(container.ID)
			}
		}

		switch d.mode {
		case dashboardOverview:
			switch msg.String() {
			case "tab", "l", "right":
				d.moveOverviewFocus(1)
			case "shift+tab", "h", "left":
				d.moveOverviewFocus(-1)
			case "j", "down":
				d.moveOverviewSelection(1)
			case "k", "up":
				d.moveOverviewSelection(-1)
			case "enter":
				if d.overviewFocus == overviewFocusDevices {
					d.enterDeviceDetail()
				} else {
					d.openOverviewSelectedService()
				}
			}
		case dashboardDeviceDetail:
			switch msg.String() {
			case "j", "down":
				d.moveServiceCursor(1)
				d.showServiceDetail = false
			case "k", "up":
				d.moveServiceCursor(-1)
				d.showServiceDetail = false
			case "enter":
				if len(d.serviceContainers) > 0 {
					d.showServiceDetail = !d.showServiceDetail
				}
			case "h", "left":
				d.moveDeviceTab(-1)
			case "l", "right":
				d.moveDeviceTab(1)
			}
		}
	}
	return d, nil
}

func (d *Dashboard) View() string {
	// Guard: if we haven't received a WindowSizeMsg yet, use a sensible default.
	if d.width == 0 {
		d.width = theme.MaxContentWidth - 2*theme.ContentPadding
	}

	var sections []string

	// Error banner when daemon is offline
	if d.lastError != nil {
		sections = append(sections, d.renderErrorBanner())
	}

	switch d.mode {
	case dashboardOverview:
		sections = append(sections, d.renderOverviewHeader())
		if summary := d.renderOverviewSummaryPanels(); summary != "" {
			sections = append(sections, summary)
		}
		sections = append(sections, d.renderOverviewDeviceList())
		if preview := d.renderOverviewServicePreview(); preview != "" {
			sections = append(sections, preview)
		}
	case dashboardDeviceDetail:
		sections = append(sections, d.renderDeviceDetailHeader())
		if detail := d.renderDeviceDetailLayout(); detail != "" {
			sections = append(sections, detail)
		}
		sections = append(sections, d.renderServiceList())
		if d.showServiceDetail {
			if detail := d.renderServiceDetail(); detail != "" {
				sections = append(sections, detail)
			}
		}
	}

	content := lipgloss.JoinVertical(lipgloss.Left, sections...)
	return lipgloss.NewStyle().Width(d.width).Render(content)
}

func (d *Dashboard) renderOverviewHeader() string {
	online, services, gpuNodes, alerts := d.fleetStats()
	summary := fmt.Sprintf("%d device(s) · %d online · %d service(s) · %d GPU node(s)", len(d.devices), online, services, gpuNodes)
	if alerts > 0 {
		summary += fmt.Sprintf(" · %d alert(s)", alerts)
	}

	lines := []string{
		theme.TitleStyle.Render("Fleet Overview"),
		theme.MutedStyle.Render(summary),
	}
	lines = append(lines, theme.MutedStyle.Render("Tab between panels · j/k navigate · Enter inspect · L/s/r/x/t act on selected service"))
	return strings.Join(lines, "\n")
}

func (d *Dashboard) renderOverviewSummaryPanels() string {
	if len(d.devices) == 0 {
		return ""
	}

	switch {
	case d.width >= 120:
		widths := dashboardPanelWidths(d.width, 2)
		firstRow := lipgloss.JoinHorizontal(lipgloss.Top,
			d.renderOverviewFleetGPUPanel(widths[0]),
			d.renderOverviewFleetRuntimePanel(widths[1]),
		)
		return lipgloss.JoinVertical(lipgloss.Left, firstRow, d.renderOverviewSelectedDevicePanel(d.width))
	case d.width >= 84:
		widths := dashboardPanelWidths(d.width, 2)
		firstRow := lipgloss.JoinHorizontal(lipgloss.Top,
			d.renderOverviewFleetGPUPanel(widths[0]),
			d.renderOverviewFleetRuntimePanel(widths[1]),
		)
		return lipgloss.JoinVertical(lipgloss.Left, firstRow, d.renderOverviewSelectedDevicePanel(d.width))
	default:
		return lipgloss.JoinVertical(lipgloss.Left,
			d.renderOverviewFleetGPUPanel(d.width),
			d.renderOverviewFleetRuntimePanel(d.width),
			d.renderOverviewSelectedDevicePanel(d.width),
		)
	}
}

func (d *Dashboard) renderOverviewFleetGPUPanel(width int) string {
	avgUtil, usedMB, totalMB, activeGPUs, gpuCount := d.fleetGPUStats()
	title := theme.TitleStyle.Render("AI Fleet")
	innerWidth := maxDashboardInt(24, width-6)
	vramPercent := 0.0
	if totalMB > 0 {
		vramPercent = float64(usedMB) / float64(totalMB) * 100
	}

	lines := []string{
		renderOverviewMetricLine("VRAM", vramPercent, formatOverviewMemoryPair(usedMB, totalMB), innerWidth),
		renderOverviewMetricLine("GPU", avgUtil, fmt.Sprintf("%.0f%% avg", avgUtil), innerWidth),
		theme.MutedStyle.Render(fmt.Sprintf("GPUs   %d active / %d total", activeGPUs, gpuCount)),
		theme.MutedStyle.Render(fmt.Sprintf("Free   %s available", formatOverviewMem(maxDashboardInt64(totalMB-usedMB, 0)))),
	}

	return theme.RenderPanel(title, strings.Join(lines, "\n"), width)
}

func (d *Dashboard) renderOverviewFleetRuntimePanel(width int) string {
	avgCPU, avgRAM := d.fleetCPUAndRAMStats()
	online, services, _, _ := d.fleetStats()
	serviceAlerts := d.fleetServiceAlerts()
	innerWidth := maxDashboardInt(24, width-6)
	deviceCount := len(d.devices)
	offline := deviceCount - online
	title := theme.TitleStyle.Render("Fleet Runtime")

	lines := []string{
		renderOverviewMetricLine("CPU", avgCPU, fmt.Sprintf("%.0f%% avg", avgCPU), innerWidth),
		renderOverviewMetricLine("RAM", avgRAM, fmt.Sprintf("%.0f%% avg", avgRAM), innerWidth),
		theme.MutedStyle.Render(fmt.Sprintf("Nodes  %d online · %d offline", online, maxDashboardInt(offline, 0))),
		theme.MutedStyle.Render(fmt.Sprintf("Svc    %d total · %d alert(s)", services, serviceAlerts)),
	}

	return theme.RenderPanel(title, strings.Join(lines, "\n"), width)
}

func (d *Dashboard) renderOverviewSelectedDevicePanel(width int) string {
	device := d.selectedDashboardDevice()
	title := theme.TitleStyle.Render("Per Device")
	if device == nil {
		return theme.RenderPanel(title, theme.MutedStyle.Render("No device selected"), width)
	}

	gpuName, gpuUtil, usedMB, totalMB, gpuCount := d.deviceGPUStats(*device)
	vramPercent := 0.0
	if totalMB > 0 {
		vramPercent = float64(usedMB) / float64(totalMB) * 100
	}
	innerWidth := maxDashboardInt(24, width-6)
	nameLine := theme.PrimaryStyle.Bold(true).Render(d.deviceDisplayLabel(*device))
	if device.Host != "" {
		nameLine += theme.MutedStyle.Render(" · " + device.Host)
	}

	lines := []string{d.renderOverviewDevicePicker(), nameLine}
	if gpuName != "" && gpuName != "none" {
		if gpuCount > 1 {
			gpuName = fmt.Sprintf("%dx %s", gpuCount, gpuName)
		}
		lines = append(lines, theme.MutedStyle.Render("GPU    "+gpuName))
		lines = append(lines,
			renderOverviewMetricLine("GUtil", gpuUtil, fmt.Sprintf("%.0f%%", gpuUtil), innerWidth),
			renderOverviewMetricLine("VRAM", vramPercent, formatOverviewMemoryPair(usedMB, totalMB), innerWidth),
		)
	} else {
		lines = append(lines, theme.MutedStyle.Render("GPU    none"))
	}
	lines = append(lines, theme.MutedStyle.Render(fmt.Sprintf("CPU %.0f%% · RAM %.0f%% · %d svc", device.CPUPercent, device.RAMPercent, device.ServiceCount)))

	return theme.RenderPanel(title, strings.Join(lines, "\n"), width)
}

func (d *Dashboard) renderOverviewDevicePicker() string {
	if len(d.devices) == 0 {
		return ""
	}

	activeStyle := lipgloss.NewStyle().Foreground(theme.Background).Background(theme.Accent).Bold(true).Padding(0, 1)
	inactiveStyle := lipgloss.NewStyle().Foreground(theme.TextPrimary).Background(theme.Highlight).Padding(0, 1)

	chips := make([]string, 0, len(d.devices))
	for i, device := range d.devices {
		label := d.deviceDisplayLabel(device)
		if device.Online {
			label = theme.StatusOnline() + " " + label
		} else {
			label = theme.StatusOffline() + " " + label
		}
		zoneID := overviewPickerZoneID(i)
		if i == d.selectedDevice {
			chips = append(chips, zone.Mark(zoneID, activeStyle.Render(label)))
		} else {
			chips = append(chips, zone.Mark(zoneID, inactiveStyle.Render(label)))
		}
	}
	return strings.Join(chips, "  ")
}

func renderOverviewMetricLine(label string, percent float64, suffix string, width int) string {
	labelText := theme.MutedStyle.Render(label)
	barWidth := width - ansi.StringWidth(label) - ansi.StringWidth(suffix) - 2
	if barWidth < 10 {
		return labelText + " " + suffix
	}
	return labelText + " " + components.ThresholdProgressBar(percent, barWidth) + " " + suffix
}

func renderOverviewPanel(title, content string, width int, focused bool) string {
	if focused {
		return theme.RenderFocusedPanel(title, content, width)
	}
	return theme.RenderPanel(title, content, width)
}

func (d *Dashboard) moveOverviewFocus(delta int) {
	panels := d.overviewFocusablePanels()
	if len(panels) == 0 {
		d.overviewFocus = overviewFocusDevices
		return
	}
	idx := 0
	for i, panel := range panels {
		if panel == d.overviewFocus {
			idx = i
			break
		}
	}
	idx = (idx + delta + len(panels)) % len(panels)
	d.overviewFocus = panels[idx]
}

func (d *Dashboard) moveOverviewSelection(delta int) {
	switch d.overviewFocus {
	case overviewFocusDevices:
		d.moveDeviceSelection(delta)
	case overviewFocusAIServices:
		d.moveOverviewServiceSelection(delta, overviewFocusAIServices)
	case overviewFocusMonitoringServices:
		d.moveOverviewServiceSelection(delta, overviewFocusMonitoringServices)
	}
}

func (d *Dashboard) moveOverviewServiceSelection(delta int, focus overviewFocus) {
	containers := d.visibleOverviewServiceContainersForFocus(focus)
	if len(containers) == 0 {
		return
	}
	idx := d.overviewServiceCursor(focus, containers)
	idx += delta
	if idx < 0 {
		idx = 0
	}
	if idx >= len(containers) {
		idx = len(containers) - 1
	}
	d.setOverviewSelectedServiceID(focus, containers[idx].ID)
}

func (d *Dashboard) openOverviewSelectedService() {
	container := d.getSelectedContainer()
	if container == nil {
		return
	}
	deviceID := d.findDeviceIDForContainer(container.ID)
	for i, device := range d.devices {
		if device.ID == deviceID {
			d.selectedDevice = i
			break
		}
	}
	d.mode = dashboardDeviceDetail
	d.showServiceDetail = true
	d.updateServiceContainers()
	for i, candidate := range d.serviceContainers {
		if candidate.ID == container.ID {
			d.selectedService = i
			return
		}
	}
}

func (d *Dashboard) overviewFocusablePanels() []overviewFocus {
	panels := []overviewFocus{overviewFocusDevices}
	if len(d.overviewAIServiceContainers()) > 0 {
		panels = append(panels, overviewFocusAIServices)
	}
	if len(d.overviewMonitoringServiceContainers()) > 0 {
		panels = append(panels, overviewFocusMonitoringServices)
	}
	return panels
}

func (d *Dashboard) visibleOverviewServiceContainersForFocus(focus overviewFocus) []ContainerData {
	containers := d.overviewServiceContainersForFocus(focus)
	limit := d.overviewServicePreviewLimit()
	if limit > 0 && len(containers) > limit {
		return containers[:limit]
	}
	return containers
}

func (d *Dashboard) overviewAIServiceContainers() []ContainerData {
	var containers []ContainerData
	for _, container := range d.serviceContainers {
		if isMonitoringServiceContainer(container) {
			continue
		}
		containers = append(containers, container)
	}
	return containers
}

func (d *Dashboard) overviewMonitoringServiceContainers() []ContainerData {
	var containers []ContainerData
	for _, container := range d.serviceContainers {
		if isMonitoringServiceContainer(container) {
			containers = append(containers, container)
		}
	}
	return containers
}

func (d *Dashboard) overviewServiceContainersForFocus(focus overviewFocus) []ContainerData {
	switch focus {
	case overviewFocusAIServices:
		return d.overviewAIServiceContainers()
	case overviewFocusMonitoringServices:
		return d.overviewMonitoringServiceContainers()
	default:
		return nil
	}
}

func (d *Dashboard) overviewServiceCursor(focus overviewFocus, containers []ContainerData) int {
	selectedID := d.overviewSelectedServiceID(focus)
	for i, container := range containers {
		if container.ID == selectedID {
			return i
		}
	}
	return 0
}

func (d *Dashboard) overviewSelectedServiceID(focus overviewFocus) string {
	switch focus {
	case overviewFocusAIServices:
		return d.selectedAIServiceID
	case overviewFocusMonitoringServices:
		return d.selectedMonitoringServiceID
	default:
		return ""
	}
}

func (d *Dashboard) setOverviewSelectedServiceID(focus overviewFocus, serviceID string) {
	switch focus {
	case overviewFocusAIServices:
		d.selectedAIServiceID = serviceID
	case overviewFocusMonitoringServices:
		d.selectedMonitoringServiceID = serviceID
	}
}

func overviewServiceZonePrefix(focus overviewFocus) string {
	switch focus {
	case overviewFocusAIServices:
		return overviewAIServiceZonePrefix
	case overviewFocusMonitoringServices:
		return overviewMonitoringServiceZonePrefix
	default:
		return ""
	}
}

func (d *Dashboard) serviceRowForContainer(container ContainerData) components.ServiceRow {
	deviceLabel := d.serviceContainerDeviceLabel(container.ID)
	return components.ServiceRow{
		Name:                strings.TrimPrefix(container.Name, "yokai-"),
		Type:                d.inferServiceType(container.Name),
		Model:               d.getServiceModel(container),
		Status:              container.Status,
		Health:              container.Health,
		Device:              deviceLabel,
		Port:                extractExternalPort(container.Ports),
		CPUPercent:          container.CPUPercent,
		MemUsedMB:           container.MemUsedMB,
		GPUMemMB:            container.GPUMemMB,
		Uptime:              formatUptime(container.UptimeSeconds),
		GenerationTokPerSec: container.GenerationTokPerSec,
		PromptTokPerSec:     container.PromptTokPerSec,
	}
}

func dashboardPanelWidths(totalWidth, columns int) []int {
	if columns <= 0 {
		return nil
	}
	widths := make([]int, columns)
	gapTotal := columns - 1
	base := (totalWidth - gapTotal) / columns
	for i := range widths {
		widths[i] = base
	}
	used := base*columns + gapTotal
	if used < totalWidth {
		widths[len(widths)-1] += totalWidth - used
	}
	return widths
}

func (d *Dashboard) renderOverviewDeviceList() string {
	title := theme.TitleStyle.Render("Devices")
	if len(d.devices) == 0 {
		return theme.RenderPanel(title, theme.MutedStyle.Render("No devices connected yet. Press 'd' for Devices to add one."), d.width)
	}

	innerWidth := d.width - 6
	if innerWidth < 40 {
		innerWidth = 40
	}
	start, end := d.overviewWindow()

	lines := []string{
		theme.MutedStyle.Render(d.renderOverviewDeviceHeader(innerWidth)),
		theme.MutedStyle.Render(strings.Repeat("─", innerWidth+1)),
	}
	if start > 0 {
		lines = append(lines, theme.MutedStyle.Render(fmt.Sprintf("  ↑ %d more device(s)", start)))
	}
	for i := start; i < end; i++ {
		lines = append(lines, d.renderOverviewDeviceRow(d.devices[i], i == d.selectedDevice, i, innerWidth))
	}
	if end < len(d.devices) {
		lines = append(lines, theme.MutedStyle.Render(fmt.Sprintf("  ↓ %d more device(s)", len(d.devices)-end)))
	}

	return renderOverviewPanel(title, strings.Join(lines, "\n"), d.width, d.overviewFocus == overviewFocusDevices)
}

func (d *Dashboard) renderDeviceDetailHeader() string {
	device := d.selectedDashboardDevice()
	if device == nil {
		return theme.TitleStyle.Render("Device Detail")
	}

	status := theme.StatusOffline() + " offline"
	if device.Online {
		status = theme.StatusOnline() + " online"
	}
	meta := fmt.Sprintf("%s · %s · %s · %d/%d", d.deviceDisplayLabel(*device), device.Host, status, d.selectedDevice+1, len(d.devices))

	return strings.Join([]string{
		theme.TitleStyle.Render("Device Detail"),
		theme.PrimaryStyle.Render(meta),
		theme.MutedStyle.Render("Esc to overview · h/l switch device · j/k select service"),
	}, "\n")
}

func (d *Dashboard) renderDeviceDetailLayout() string {
	device := d.selectedDashboardDevice()
	if device == nil {
		return ""
	}

	sections := []string{d.renderDeviceCardWithGPU(*device, d.width)}
	if sparklines := d.renderDeviceSparklines(device.ID, d.width); sparklines != "" {
		sections = append(sections, sparklines)
	}
	return lipgloss.JoinVertical(lipgloss.Left, sections...)
}

func (d *Dashboard) enterDeviceDetail() {
	if len(d.devices) == 0 {
		return
	}
	d.mode = dashboardDeviceDetail
	d.selectedService = 0
	d.showServiceDetail = false
	d.updateServiceContainers()
}

func (d *Dashboard) moveDeviceSelection(delta int) {
	if len(d.devices) == 0 {
		return
	}
	d.selectedDevice += delta
	if d.selectedDevice < 0 {
		d.selectedDevice = 0
	}
	if d.selectedDevice >= len(d.devices) {
		d.selectedDevice = len(d.devices) - 1
	}
	d.showServiceDetail = false
}

func (d *Dashboard) clampSelectedDevice() {
	if len(d.devices) == 0 {
		d.selectedDevice = 0
		d.mode = dashboardOverview
		d.selectedService = 0
		d.showServiceDetail = false
		return
	}
	if d.selectedDevice < 0 {
		d.selectedDevice = 0
	}
	if d.selectedDevice >= len(d.devices) {
		d.selectedDevice = len(d.devices) - 1
	}
}

func (d *Dashboard) selectedDashboardDevice() *DashboardDevice {
	if len(d.devices) == 0 {
		return nil
	}
	d.clampSelectedDevice()
	return &d.devices[d.selectedDevice]
}

func (d *Dashboard) renderOverviewDeviceHeader(innerWidth int) string {
	switch {
	case innerWidth >= 118:
		labelW, hostW, gpuW := 14, 18, 20
		return strings.Join([]string{
			fitDashboardCell("", 2),
			fitDashboardCell("Device", labelW),
			fitDashboardCell("Host", hostW),
			fitDashboardCell("GPU", gpuW),
			fitDashboardCell("GUtil", 7),
			fitDashboardCell("VRAM", 10),
			fitDashboardCell("CPU", 6),
			fitDashboardCell("RAM", 6),
			fitDashboardCell("Svc", 5),
		}, " ")
	case innerWidth >= 96:
		labelW, gpuW := 16, 18
		return strings.Join([]string{
			fitDashboardCell("", 2),
			fitDashboardCell("Device", labelW),
			fitDashboardCell("GPU", gpuW),
			fitDashboardCell("GUtil", 7),
			fitDashboardCell("VRAM", 10),
			fitDashboardCell("CPU", 6),
			fitDashboardCell("RAM", 6),
			fitDashboardCell("Svc", 5),
		}, " ")
	case innerWidth >= 82:
		labelW, gpuW := 18, 14
		return strings.Join([]string{
			fitDashboardCell("", 2),
			fitDashboardCell("Device", labelW),
			fitDashboardCell("GPU", gpuW),
			fitDashboardCell("VRAM", 10),
			fitDashboardCell("CPU", 6),
			fitDashboardCell("RAM", 6),
			fitDashboardCell("Svc", 5),
		}, " ")
	default:
		nameW := maxDashboardInt(16, innerWidth-(2+1+10+1+6+1+5+4))
		return strings.Join([]string{
			fitDashboardCell("", 2),
			fitDashboardCell("Device", nameW),
			fitDashboardCell("VRAM", 10),
			fitDashboardCell("CPU", 6),
			fitDashboardCell("Svc", 5),
		}, " ")
	}
}

func (d *Dashboard) renderOverviewDeviceRow(device DashboardDevice, selected bool, index, innerWidth int) string {
	label := d.deviceDisplayLabel(device)
	host := device.Host
	if host == "" {
		host = "-"
	}
	status := theme.StatusOffline()
	if device.Online {
		status = theme.StatusOnline()
	}
	gpuName, gpuUtil, vramUsedMB, vramTotalMB, _ := d.deviceGPUStats(device)
	cpu := dashboardPercent(device.Online, device.CPUPercent)
	ram := dashboardPercent(device.Online, device.RAMPercent)
	gutil := dashboardPercent(device.Online && vramTotalMB > 0, gpuUtil)
	vram := formatOverviewMemoryPair(vramUsedMB, vramTotalMB)
	svc := fmt.Sprintf("%d", device.ServiceCount)

	var row string
	switch {
	case innerWidth >= 118:
		labelW, hostW, gpuW := 14, 18, 20
		row = strings.Join([]string{
			fitDashboardCell(status, 2),
			fitDashboardCell(label, labelW),
			fitDashboardCell(host, hostW),
			fitDashboardCell(gpuName, gpuW),
			fitDashboardCell(gutil, 7),
			fitDashboardCell(vram, 10),
			fitDashboardCell(cpu, 6),
			fitDashboardCell(ram, 6),
			fitDashboardCell(svc, 5),
		}, " ")
	case innerWidth >= 96:
		labelW, gpuW := 16, 18
		row = strings.Join([]string{
			fitDashboardCell(status, 2),
			fitDashboardCell(label, labelW),
			fitDashboardCell(gpuName, gpuW),
			fitDashboardCell(gutil, 7),
			fitDashboardCell(vram, 10),
			fitDashboardCell(cpu, 6),
			fitDashboardCell(ram, 6),
			fitDashboardCell(svc, 5),
		}, " ")
	case innerWidth >= 82:
		labelW, gpuW := 18, 14
		row = strings.Join([]string{
			fitDashboardCell(status, 2),
			fitDashboardCell(label+" · "+host, labelW),
			fitDashboardCell(gpuName, gpuW),
			fitDashboardCell(vram, 10),
			fitDashboardCell(cpu, 6),
			fitDashboardCell(ram, 6),
			fitDashboardCell(svc, 5),
		}, " ")
	default:
		nameW := maxDashboardInt(16, innerWidth-(2+1+10+1+6+1+5+4))
		row = strings.Join([]string{
			fitDashboardCell(status, 2),
			fitDashboardCell(label+" · "+host, nameW),
			fitDashboardCell(vram, 10),
			fitDashboardCell(cpu, 6),
			fitDashboardCell(svc, 5),
		}, " ")
	}

	zoneID := dashboardDeviceZoneID(index)
	if selected {
		bar := lipgloss.NewStyle().Foreground(theme.Accent).Bold(true).Render("▌")
		content := lipgloss.NewStyle().Background(theme.Highlight).Foreground(theme.TextPrimary).Width(innerWidth + 1).Render(row)
		return zone.Mark(zoneID, bar+content)
	}

	content := lipgloss.NewStyle().Foreground(theme.TextPrimary).Width(innerWidth + 1).Render(" " + row)
	return zone.Mark(zoneID, content)
}

func (d *Dashboard) renderOverviewServicePreview() string {
	aiContainers := d.overviewAIServiceContainers()
	monitoringContainers := d.overviewMonitoringServiceContainers()
	if len(aiContainers) == 0 && len(monitoringContainers) == 0 {
		return ""
	}
	limit := d.overviewServicePreviewLimit()
	if limit <= 0 {
		return ""
	}

	if d.width >= 110 {
		widths := dashboardPanelWidths(d.width, 2)
		return lipgloss.JoinHorizontal(lipgloss.Top,
			d.renderOverviewServicePanel("AI Services", aiContainers, widths[0], limit, overviewFocusAIServices),
			d.renderOverviewServicePanel("Monitoring Services", monitoringContainers, widths[1], limit, overviewFocusMonitoringServices),
		)
	}

	sections := []string{d.renderOverviewServicePanel("AI Services", aiContainers, d.width, limit, overviewFocusAIServices)}
	if len(monitoringContainers) > 0 {
		sections = append(sections, d.renderOverviewServicePanel("Monitoring Services", monitoringContainers, d.width, limit, overviewFocusMonitoringServices))
	}
	return lipgloss.JoinVertical(lipgloss.Left, sections...)
}

func (d *Dashboard) renderOverviewServicePanel(title string, containers []ContainerData, width, limit int, focus overviewFocus) string {
	titleText := theme.TitleStyle.Render(title)
	if len(containers) == 0 {
		return renderOverviewPanel(titleText, theme.MutedStyle.Render("No services in this category."), width, d.overviewFocus == focus)
	}
	truncated := containers
	if len(truncated) > limit {
		truncated = truncated[:limit]
	}
	rows := make([]components.ServiceRow, 0, len(truncated))
	for _, container := range truncated {
		rows = append(rows, d.serviceRowForContainer(container))
	}
	serviceList := components.NewServiceList(rows, width-4)
	serviceList.Cursor = d.overviewServiceCursor(focus, truncated)
	serviceList.ForceDeviceColumn = true
	serviceList.ZonePrefix = overviewServiceZonePrefix(focus)
	content := serviceList.Render()
	if len(containers) > len(truncated) {
		content += "\n" + theme.MutedStyle.Render(fmt.Sprintf("Showing %d of %d services", len(truncated), len(containers)))
	}
	return renderOverviewPanel(titleText, content, width, d.overviewFocus == focus)
}

func (d *Dashboard) overviewWindow() (int, int) {
	if len(d.devices) == 0 {
		return 0, 0
	}
	maxRows := d.height - 30
	if maxRows < 4 {
		maxRows = 4
	}
	if maxRows > 12 {
		maxRows = 12
	}
	if maxRows >= len(d.devices) {
		return 0, len(d.devices)
	}

	start := d.selectedDevice - maxRows/2
	if start < 0 {
		start = 0
	}
	end := start + maxRows
	if end > len(d.devices) {
		end = len(d.devices)
		start = end - maxRows
	}
	return start, end
}

func (d *Dashboard) overviewServicePreviewLimit() int {
	start, end := d.overviewWindow()
	visibleDeviceRows := end - start
	limit := d.height - 22 - visibleDeviceRows
	if limit < 4 {
		limit = 4
	}
	if limit > 10 {
		limit = 10
	}
	return limit
}

func (d *Dashboard) fleetStats() (online, services, gpuNodes, alerts int) {
	for _, device := range d.devices {
		if device.Online {
			online++
		} else {
			alerts++
		}
		services += device.ServiceCount
		if device.GPUCount > 0 {
			gpuNodes++
		}
	}
	for _, metrics := range d.metrics {
		for _, container := range metrics.Containers {
			if dashboardServiceAlert(container) {
				alerts++
			}
		}
	}
	return online, services, gpuNodes, alerts
}

func dashboardServiceAlert(container ContainerData) bool {
	status := container.Health
	if status == "" {
		status = container.Status
	}
	switch status {
	case "healthy", "running", "starting", "created", "restarting", "":
		return false
	default:
		return true
	}
}

func (d *Dashboard) overviewGPUText(device DashboardDevice) string {
	name, _, _, _, count := d.deviceGPUStats(device)
	if count > 1 && name != "" && name != "none" {
		return fmt.Sprintf("%dx %s", count, name)
	}
	return name
}

func (d *Dashboard) deviceDisplayLabel(device DashboardDevice) string {
	if device.Label != "" {
		return device.Label
	}
	return device.ID
}

func dashboardDeviceZoneID(index int) string {
	return fmt.Sprintf("dashboard-device-%d", index)
}

func overviewPickerZoneID(index int) string {
	return fmt.Sprintf("overview-picker-%d", index)
}

func dashboardPercent(online bool, value float64) string {
	if !online {
		return "--"
	}
	return fmt.Sprintf("%.0f%%", value)
}

func fitDashboardCell(value string, width int) string {
	if width <= 0 {
		return ""
	}
	if ansi.StringWidth(value) > width {
		if width == 1 {
			value = "…"
		} else {
			value = ansi.Truncate(value, width-1, "") + "…"
		}
	}
	if pad := width - ansi.StringWidth(value); pad > 0 {
		value += strings.Repeat(" ", pad)
	}
	return value
}

func (d *Dashboard) deviceGPUStats(device DashboardDevice) (name string, util float64, usedMB, totalMB int64, count int) {
	name = "none"
	if metrics, ok := d.metrics[device.ID]; ok && len(metrics.GPUs) > 0 {
		count = len(metrics.GPUs)
		name = strings.TrimPrefix(metrics.GPUs[0].Name, "NVIDIA ")
		var totalUtil float64
		for _, gpu := range metrics.GPUs {
			totalUtil += float64(gpu.UtilPercent)
			usedMB += gpu.VRAMUsedMB
			totalMB += gpu.VRAMTotalMB
		}
		util = totalUtil / float64(len(metrics.GPUs))
		return name, util, usedMB, totalMB, count
	}

	if device.GPUCount > 0 {
		count = device.GPUCount
		name = strings.TrimPrefix(device.GPUType, "NVIDIA ")
		if name == "" {
			name = "GPU"
		}
	}
	return name, 0, 0, 0, count
}

func (d *Dashboard) fleetGPUStats() (avgUtil float64, usedMB, totalMB int64, activeGPUs, gpuCount int) {
	var utilSum float64
	for _, metrics := range d.metrics {
		if len(metrics.GPUs) == 0 {
			continue
		}
		for _, gpu := range metrics.GPUs {
			gpuCount++
			utilSum += float64(gpu.UtilPercent)
			usedMB += gpu.VRAMUsedMB
			totalMB += gpu.VRAMTotalMB
			if gpu.UtilPercent > 0 || gpu.VRAMUsedMB > 0 {
				activeGPUs++
			}
		}
	}
	if gpuCount > 0 {
		avgUtil = utilSum / float64(gpuCount)
	}
	return avgUtil, usedMB, totalMB, activeGPUs, gpuCount
}

func (d *Dashboard) fleetCPUAndRAMStats() (avgCPU, avgRAM float64) {
	count := 0
	for _, device := range d.devices {
		if !device.Online {
			continue
		}
		avgCPU += device.CPUPercent
		avgRAM += device.RAMPercent
		count++
	}
	if count == 0 {
		return 0, 0
	}
	return avgCPU / float64(count), avgRAM / float64(count)
}

func formatOverviewMem(mb int64) string {
	if mb <= 0 {
		return "-"
	}
	if mb < 1024 {
		return fmt.Sprintf("%dM", mb)
	}
	return fmt.Sprintf("%.1fG", float64(mb)/1024)
}

func formatOverviewMemoryPair(usedMB, totalMB int64) string {
	if totalMB <= 0 {
		return "-"
	}
	return formatOverviewMem(usedMB) + "/" + formatOverviewMem(totalMB)
}

func dashboardServiceRowAlert(row components.ServiceRow) bool {
	switch row.Health {
	case "unhealthy", "error":
		return true
	}
	switch row.Status {
	case "exited", "dead", "error", "unhealthy":
		return true
	}
	return false
}

func (d *Dashboard) fleetServiceAlerts() int {
	alerts := 0
	for _, metrics := range d.metrics {
		for _, container := range metrics.Containers {
			if dashboardServiceAlert(container) {
				alerts++
			}
		}
	}
	return alerts
}

func isMonitoringServiceRow(row components.ServiceRow) bool {
	return strings.HasPrefix(strings.ToLower(row.Name), "mon-")
}

func isMonitoringServiceContainer(container ContainerData) bool {
	return isMonitoringServiceRow(components.ServiceRow{Name: strings.TrimPrefix(container.Name, "yokai-")})
}

func maxDashboardInt64(a, b int64) int64 {
	if a > b {
		return a
	}
	return b
}

func maxDashboardInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

// renderWideLayout renders the 3-column btop-style grid (≥140 cols).
// Row 1: Device cards side-by-side (up to 3)
// Row 2: CPU | RAM | GPU charts side-by-side
func (d *Dashboard) renderWideLayout() string {
	var rows []string

	// Row 1: Device cards
	if len(d.devices) > 0 {
		row1 := d.renderDeviceCardsRow(3)
		if row1 != "" {
			rows = append(rows, row1)
		}
	}

	// Row 2: Sparkline charts (CPU, RAM, GPU) side-by-side
	if len(d.devices) > 0 {
		device := d.devices[d.selectedDevice]
		row2 := d.renderChartsRow(device.ID, 3)
		if row2 != "" {
			rows = append(rows, row2)
		}
	}

	if len(rows) == 0 {
		return ""
	}
	return lipgloss.JoinVertical(lipgloss.Left, rows...)
}

// renderMediumLayout renders the 2-column grid (100–139 cols).
// Row 1: 2 device cards per row
// Row 2: CPU + RAM charts side-by-side; GPU chart below if present
func (d *Dashboard) renderMediumLayout() string {
	var rows []string

	if len(d.devices) > 0 {
		row1 := d.renderDeviceCardsRow(2)
		if row1 != "" {
			rows = append(rows, row1)
		}

		device := d.devices[d.selectedDevice]
		row2 := d.renderChartsRow(device.ID, 2)
		if row2 != "" {
			rows = append(rows, row2)
		}
	}

	if len(rows) == 0 {
		return ""
	}
	return lipgloss.JoinVertical(lipgloss.Left, rows...)
}

// renderNarrowLayout renders the single-column stack (<100 cols).
func (d *Dashboard) renderNarrowLayout() string {
	var rows []string

	if len(d.devices) > 0 {
		if d.selectedDevice >= len(d.devices) {
			d.selectedDevice = len(d.devices) - 1
		}
		device := d.devices[d.selectedDevice]

		card := d.renderDeviceCardWithGPU(device, d.width)
		rows = append(rows, card)

		sparklines := d.renderDeviceSparklines(device.ID, d.width)
		if sparklines != "" {
			rows = append(rows, sparklines)
		}
	}

	if len(rows) == 0 {
		return ""
	}
	return lipgloss.JoinVertical(lipgloss.Left, rows...)
}

// renderDeviceCardsRow renders device cards side-by-side (up to maxCols).
// All visible devices are shown; the active device tab is highlighted.
func (d *Dashboard) renderDeviceCardsRow(maxCols int) string {
	if len(d.devices) == 0 {
		return ""
	}

	// Show all devices (wrap to next row if more than maxCols)
	numCols := len(d.devices)
	if numCols > maxCols {
		numCols = maxCols
	}

	gap := numCols - 1
	if gap < 0 {
		gap = 0
	}
	cardWidth := (d.width - gap) / numCols
	if cardWidth < 24 {
		cardWidth = 24
	}

	var cards []string
	for i, device := range d.devices {
		if i >= maxCols {
			break
		}
		w := cardWidth
		// Last card may be slightly wider to fill remaining space
		if i == numCols-1 {
			w = d.width - (cardWidth+1)*(numCols-1)
			if w < cardWidth {
				w = cardWidth
			}
		}

		// Highlight active device with focused border
		focused := (i == d.selectedDevice)
		card := d.renderDeviceCardCompact(device, w, focused)
		cards = append(cards, card)
	}

	if len(cards) == 0 {
		return ""
	}
	return lipgloss.JoinHorizontal(lipgloss.Top, cards...)
}

// renderDeviceCardCompact renders a compact device card, optionally with a focus border.
func (d *Dashboard) renderDeviceCardCompact(device DashboardDevice, width int, focused bool) string {
	var gpus []components.GPUMetrics
	if m, ok := d.metrics[device.ID]; ok {
		for _, gpu := range m.GPUs {
			gpus = append(gpus, components.GPUMetrics{
				Name:        gpu.Name,
				UtilPercent: gpu.UtilPercent,
				VRAMUsedMB:  gpu.VRAMUsedMB,
				VRAMTotalMB: gpu.VRAMTotalMB,
				TempC:       gpu.TempC,
				PowerDrawW:  float64(gpu.PowerDrawW),
				PowerLimitW: float64(gpu.PowerLimitW),
				FanPercent:  gpu.FanPercent,
			})
		}
	}

	card := components.NewDeviceCardFull(
		device.Label,
		device.Host,
		device.Online,
		device.CPUPercent,
		device.RAMPercent,
		device.ServiceCount,
		gpus,
		width,
	)
	card.Focused = focused
	return card.Render()
}

// renderChartsRow renders CPU/RAM/GPU stream charts side-by-side.
// numCols determines how many charts to show per row.
func (d *Dashboard) renderChartsRow(deviceID string, numCols int) string {
	cpuVals := d.cpuHistory[deviceID]
	ramVals := d.ramHistory[deviceID]

	// Get GPU history
	var gpuVals []float64
	var gpuLabel string
	if m, ok := d.metrics[deviceID]; ok {
		for _, gpu := range m.GPUs {
			historyKey := fmt.Sprintf("%s-%d", deviceID, gpu.Index)
			if vals, ok := d.gpuHistory[historyKey]; ok && len(vals) > 0 {
				gpuVals = vals
				gpuLabel = fmt.Sprintf("GPU %d%%", gpu.UtilPercent)
				break
			}
		}
	}

	// Collect charts that have data
	type chartSpec struct {
		label  string
		values []float64
		color  lipgloss.Color
	}
	var chartSpecs []chartSpec

	if len(cpuVals) > 0 {
		label := fmt.Sprintf(" CPU %.0f%%", cpuVals[len(cpuVals)-1])
		chartSpecs = append(chartSpecs, chartSpec{label, cpuVals, theme.Good})
	}
	if len(ramVals) > 0 {
		label := fmt.Sprintf(" RAM %.0f%%", ramVals[len(ramVals)-1])
		chartSpecs = append(chartSpecs, chartSpec{label, ramVals, theme.Accent})
	}
	if len(gpuVals) > 0 {
		chartSpecs = append(chartSpecs, chartSpec{" " + gpuLabel, gpuVals, theme.Warn})
	}

	if len(chartSpecs) == 0 {
		return ""
	}

	// If more charts than columns, truncate to numCols (GPU may wrap in medium layout)
	if len(chartSpecs) > numCols {
		chartSpecs = chartSpecs[:numCols]
	}

	n := len(chartSpecs)
	gap := n - 1
	if gap < 0 {
		gap = 0
	}
	chartWidth := (d.width - gap) / n
	if chartWidth < 20 {
		chartWidth = 20
	}
	chartHeight := 6

	var charts []string
	for i, spec := range chartSpecs {
		val := spec.values[len(spec.values)-1]
		titleStyle := utilColorStyle(val)
		chartTitle := titleStyle.Render(spec.label)
		w := chartWidth
		if i == n-1 {
			w = d.width - (chartWidth+1)*(n-1)
			if w < chartWidth {
				w = chartWidth
			}
		}
		iw := w - 4
		if iw < 10 {
			iw = 10
		}
		sc := components.NewStreamChartGradient(spec.label, spec.values, iw, chartHeight, spec.color)
		charts = append(charts, theme.RenderPanel(chartTitle, sc.Render(), w))
	}

	return lipgloss.JoinHorizontal(lipgloss.Top, charts...)
}

// renderDeviceTabBar renders the device tab selector in btop style.
func (d *Dashboard) renderDeviceTabBar() string {
	var tabs []string

	activeLabelStyle := lipgloss.NewStyle().
		Foreground(theme.Background).
		Background(theme.Accent).
		Bold(true).
		Padding(0, 1)

	inactiveStyle := lipgloss.NewStyle().
		Foreground(theme.TextMuted).
		Padding(0, 1)

	for i, device := range d.devices {
		label := device.Label
		if label == "" {
			label = device.ID
		}
		// Add status dot
		dot := theme.StatusOffline()
		if device.Online {
			dot = theme.StatusOnline()
		}
		tabLabel := dot + " " + label

		zoneID := fmt.Sprintf("device-tab-%d", i)
		if i == d.selectedDevice {
			rendered := activeLabelStyle.Render(tabLabel)
			tabs = append(tabs, zone.Mark(zoneID, rendered))
		} else {
			tabs = append(tabs, zone.Mark(zoneID, inactiveStyle.Render(tabLabel)))
		}
	}

	tabRow := lipgloss.JoinHorizontal(lipgloss.Center, tabs...)
	hint := theme.MutedStyle.Render("  h/l: switch")

	// Separator line below tabs
	sepLine := lipgloss.NewStyle().
		Foreground(theme.Border).
		Render(strings.Repeat("─", d.width))

	return tabRow + hint + "\n" + sepLine
}

// utilColorStyle returns a lipgloss style colored by utilization threshold.
func utilColorStyle(val float64) lipgloss.Style {
	switch {
	case val >= 80:
		return lipgloss.NewStyle().Foreground(theme.Crit).Bold(true)
	case val >= 50:
		return lipgloss.NewStyle().Foreground(theme.Warn).Bold(true)
	default:
		return lipgloss.NewStyle().Foreground(theme.Good).Bold(true)
	}
}

// moveDeviceTab changes the active device tab by delta, wrapping around.
func (d *Dashboard) moveDeviceTab(delta int) {
	if len(d.devices) == 0 {
		return
	}
	d.selectedDevice = (d.selectedDevice + delta + len(d.devices)) % len(d.devices)
	// Reset service selection when switching devices
	d.selectedService = 0
	d.showServiceDetail = false
	d.updateServiceContainers()
}

// activeDeviceID returns the device ID for the currently selected tab.
func (d *Dashboard) activeDeviceID() string {
	if d.mode == dashboardOverview {
		return ""
	}
	if len(d.devices) == 0 {
		return ""
	}
	if d.selectedDevice >= len(d.devices) {
		d.selectedDevice = len(d.devices) - 1
	}
	return d.devices[d.selectedDevice].ID
}

// renderDeviceCardWithGPU renders a full-width device card with GPU metrics inline.
func (d *Dashboard) renderDeviceCardWithGPU(device DashboardDevice, width int) string {
	var gpus []components.GPUMetrics
	if m, ok := d.metrics[device.ID]; ok {
		for _, gpu := range m.GPUs {
			gpus = append(gpus, components.GPUMetrics{
				Name:        gpu.Name,
				UtilPercent: gpu.UtilPercent,
				VRAMUsedMB:  gpu.VRAMUsedMB,
				VRAMTotalMB: gpu.VRAMTotalMB,
				TempC:       gpu.TempC,
				PowerDrawW:  float64(gpu.PowerDrawW),
				PowerLimitW: float64(gpu.PowerLimitW),
				FanPercent:  gpu.FanPercent,
			})
		}
	}

	card := components.NewDeviceCardFull(
		device.Label,
		device.Host,
		device.Online,
		device.CPUPercent,
		device.RAMPercent,
		device.ServiceCount,
		gpus,
		width,
	)
	return card.Render()
}

// renderDeviceSparklines renders CPU + RAM + optionally GPU sparklines side by side for one device.
func (d *Dashboard) renderDeviceSparklines(deviceID string, availableWidth int) string {
	cpuVals := d.cpuHistory[deviceID]
	ramVals := d.ramHistory[deviceID]
	if len(cpuVals) == 0 && len(ramVals) == 0 {
		return ""
	}

	// Determine if this device has GPU history
	var gpuVals []float64
	var gpuLabel string
	if m, ok := d.metrics[deviceID]; ok {
		for _, gpu := range m.GPUs {
			historyKey := fmt.Sprintf("%s-%d", deviceID, gpu.Index)
			if vals, ok := d.gpuHistory[historyKey]; ok && len(vals) > 0 {
				gpuVals = vals
				gpuLabel = fmt.Sprintf("GPU %d%%", gpu.UtilPercent)
				break
			}
		}
	}

	numCharts := 2
	if len(gpuVals) > 0 {
		numCharts = 3
	}

	chartWidth := (availableWidth - numCharts + 1) / numCharts
	if chartWidth < 20 {
		chartWidth = 20
	}
	chartHeight := 5
	innerWidth := chartWidth - 4
	if innerWidth < 10 {
		innerWidth = 10
	}

	var charts []string

	if len(cpuVals) > 0 {
		label := fmt.Sprintf(" CPU %.0f%%", cpuVals[len(cpuVals)-1])
		cpuTitle := theme.GoodStyle.Render(label)
		cpu := components.NewStreamChart("CPU", cpuVals, innerWidth, chartHeight, theme.Good)
		charts = append(charts, theme.RenderPanel(cpuTitle, cpu.Render(), chartWidth))
	}

	if len(ramVals) > 0 {
		label := fmt.Sprintf(" RAM %.0f%%", ramVals[len(ramVals)-1])
		ramTitle := theme.WarnStyle.Render(label)
		ram := components.NewStreamChart("RAM", ramVals, innerWidth, chartHeight, theme.Accent)
		charts = append(charts, theme.RenderPanel(ramTitle, ram.Render(), chartWidth))
	}

	if len(gpuVals) > 0 {
		gpuTitle := lipgloss.NewStyle().Foreground(theme.Accent).Render(" " + gpuLabel)
		gpuChart := components.NewStreamChart("GPU", gpuVals, innerWidth, chartHeight, theme.Accent)
		charts = append(charts, theme.RenderPanel(gpuTitle, gpuChart.Render(), chartWidth))
	}

	if len(charts) == 0 {
		return ""
	}

	return lipgloss.JoinHorizontal(lipgloss.Top, charts...)
}

func (d *Dashboard) renderErrorBanner() string {
	bannerWidth := d.width - 4
	if bannerWidth < 30 {
		bannerWidth = 30
	}

	msg := fmt.Sprintf("  Daemon offline: %v", d.lastError)
	hint := "  Retrying automatically..."

	content := theme.WarnStyle.Render(msg) + "\n" + theme.MutedStyle.Render(hint)

	return lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(theme.Warn).
		Padding(0, 1).
		Width(bannerWidth).
		Render(content)
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

func (d *Dashboard) renderServiceList() string {
	serviceRows := d.buildServiceRows()

	serviceList := components.NewServiceList(serviceRows, d.width-4)
	serviceList.Cursor = d.selectedService

	titleText := "Services"
	if d.mode == dashboardDeviceDetail {
		if device := d.selectedDashboardDevice(); device != nil {
			titleText = "Services — " + d.deviceDisplayLabel(*device)
		}
	}
	title := theme.TitleStyle.Render(titleText)
	content := serviceList.Render()

	return theme.RenderPanel(title, content, d.width)
}

func (d *Dashboard) buildServiceRows() []components.ServiceRow {
	rows := make([]components.ServiceRow, 0, len(d.serviceContainers))
	for _, container := range d.serviceContainers {
		rows = append(rows, d.serviceRowForContainer(container))
	}
	return rows
}

func (d *Dashboard) renderServiceDetail() string {
	container := d.getSelectedContainer()
	if container == nil {
		return ""
	}

	displayName := strings.TrimPrefix(container.Name, "yokai-")
	deviceID := d.findDeviceIDForContainer(container.ID)
	device := d.findDevice(deviceID)
	deviceLabel := ""
	if device != nil {
		deviceLabel = device.Label
	}

	// Gather CPU history for this container's device
	var cpuHistory []float64
	if vals, ok := d.cpuHistory[deviceID]; ok {
		cpuHistory = vals
	}

	data := components.ServiceDetailData{
		Name:        displayName,
		Image:       container.Image,
		Model:       d.getServiceModel(*container),
		ContainerID: container.ID,
		Status:      container.Status,
		Health:      container.Health,
		Port:        extractExternalPort(container.Ports),
		Device:      deviceLabel,
		CPUPercent:  container.CPUPercent,
		MemUsedMB:   container.MemUsedMB,
		GPUMemMB:    container.GPUMemMB,
		Uptime:      formatUptime(container.UptimeSeconds),
		CPUHistory:  cpuHistory,
	}

	detail := components.NewServiceDetail(data, d.width)
	return detail.Render()
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
	if d.mode == dashboardOverview {
		containers := d.visibleOverviewServiceContainersForFocus(d.overviewFocus)
		if len(containers) == 0 {
			return nil
		}
		idx := d.overviewServiceCursor(d.overviewFocus, containers)
		if idx >= 0 && idx < len(containers) {
			return &containers[idx]
		}
		return nil
	}
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
	activeID := d.activeDeviceID()
	if activeID == "" {
		// No active device — show all containers
		for _, metrics := range d.metrics {
			d.serviceContainers = append(d.serviceContainers, metrics.Containers...)
		}
	} else if m, ok := d.metrics[activeID]; ok {
		d.serviceContainers = append(d.serviceContainers, m.Containers...)
	}
	d.sortServiceContainers()
	if d.mode == dashboardOverview {
		d.syncOverviewSelections()
	}

	if len(d.serviceContainers) == 0 {
		d.selectedService = 0
		d.showServiceDetail = false
		return
	}
	if d.selectedService < 0 {
		d.selectedService = 0
	}
	if d.selectedService >= len(d.serviceContainers) {
		d.selectedService = len(d.serviceContainers) - 1
	}
	if d.mode == dashboardOverview {
		d.showServiceDetail = false
	}
}

func (d *Dashboard) sortServiceContainers() {
	sort.SliceStable(d.serviceContainers, func(i, j int) bool {
		return d.lessServiceContainer(d.serviceContainers[i], d.serviceContainers[j])
	})
}

func (d *Dashboard) lessServiceContainer(left, right ContainerData) bool {
	leftAlert := dashboardServiceAlert(left)
	rightAlert := dashboardServiceAlert(right)
	if leftAlert != rightAlert {
		return leftAlert
	}
	leftDevice := d.serviceContainerDeviceLabel(left.ID)
	rightDevice := d.serviceContainerDeviceLabel(right.ID)
	if leftDevice != rightDevice {
		return leftDevice < rightDevice
	}
	leftName := strings.TrimPrefix(left.Name, "yokai-")
	rightName := strings.TrimPrefix(right.Name, "yokai-")
	return leftName < rightName
}

func (d *Dashboard) serviceContainerDeviceLabel(containerID string) string {
	deviceID := d.findDeviceIDForContainer(containerID)
	if device := d.findDevice(deviceID); device != nil {
		return d.deviceDisplayLabel(*device)
	}
	return deviceID
}

func (d *Dashboard) syncOverviewSelections() {
	ai := d.visibleOverviewServiceContainersForFocus(overviewFocusAIServices)
	monitoring := d.visibleOverviewServiceContainersForFocus(overviewFocusMonitoringServices)
	d.selectedAIServiceID = syncOverviewSelectionID(ai, d.selectedAIServiceID)
	d.selectedMonitoringServiceID = syncOverviewSelectionID(monitoring, d.selectedMonitoringServiceID)

	validFocus := false
	for _, panel := range d.overviewFocusablePanels() {
		if panel == d.overviewFocus {
			validFocus = true
			break
		}
	}
	if !validFocus {
		d.overviewFocus = overviewFocusDevices
	}
}

func syncOverviewSelectionID(containers []ContainerData, current string) string {
	if len(containers) == 0 {
		return ""
	}
	for _, container := range containers {
		if container.ID == current {
			return current
		}
	}
	return containers[0].ID
}

func (d *Dashboard) stopService(containerID string) tea.Cmd {
	return func() tea.Msg {
		deviceID := d.findDeviceIDForContainer(containerID)
		if deviceID == "" {
			return serviceStopMsg{err: fmt.Errorf("device not found")}
		}
		url := fmt.Sprintf("http://%s/containers/%s/%s/stop", d.cfg.Daemon.Listen, deviceID, containerID)
		resp, err := http.Post(url, "application/json", nil)
		if err != nil {
			return serviceStopMsg{err: err}
		}
		_ = resp.Body.Close()
		return serviceStopMsg{}
	}
}

func (d *Dashboard) restartService(containerID string) tea.Cmd {
	return func() tea.Msg {
		deviceID := d.findDeviceIDForContainer(containerID)
		if deviceID == "" {
			return serviceRestartMsg{err: fmt.Errorf("device not found")}
		}
		url := fmt.Sprintf("http://%s/containers/%s/%s/restart", d.cfg.Daemon.Listen, deviceID, containerID)
		resp, err := http.Post(url, "application/json", nil)
		if err != nil {
			return serviceRestartMsg{err: err}
		}
		_ = resp.Body.Close()
		return serviceRestartMsg{}
	}
}

func (d *Dashboard) deleteService(containerID, serviceID string) tea.Cmd {
	return func() tea.Msg {
		deviceID := d.findDeviceIDForContainer(containerID)
		if deviceID == "" {
			return serviceDeleteMsg{containerID: containerID, serviceID: serviceID, err: fmt.Errorf("device not found")}
		}

		url := fmt.Sprintf("http://%s/containers/%s/%s/remove", d.cfg.Daemon.Listen, deviceID, containerID)
		req, err := http.NewRequest(http.MethodDelete, url, nil)
		if err != nil {
			return serviceDeleteMsg{containerID: containerID, serviceID: serviceID, err: err}
		}

		httpClient := &http.Client{Timeout: 30 * time.Second}
		resp, err := httpClient.Do(req)
		if err != nil {
			return serviceDeleteMsg{containerID: containerID, serviceID: serviceID, err: err}
		}
		defer func() {
			_ = resp.Body.Close()
		}()

		if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNoContent {
			return serviceDeleteMsg{containerID: containerID, serviceID: serviceID, err: fmt.Errorf("daemon returned status %d", resp.StatusCode)}
		}

		return serviceDeleteMsg{containerID: containerID, serviceID: serviceID}
	}
}

func (d *Dashboard) testService(containerID string) tea.Cmd {
	return func() tea.Msg {
		deviceID := d.findDeviceIDForContainer(containerID)
		if deviceID == "" {
			return serviceTestMsg{err: fmt.Errorf("device not found")}
		}

		url := fmt.Sprintf("http://%s/containers/%s/%s/test", d.cfg.Daemon.Listen, deviceID, containerID)
		resp, err := http.Post(url, "application/json", strings.NewReader("{}"))
		if err != nil {
			return serviceTestMsg{err: err}
		}
		defer func() {
			_ = resp.Body.Close()
		}()

		if resp.StatusCode != http.StatusOK {
			var errResp struct {
				Message string `json:"message"`
			}
			if json.NewDecoder(resp.Body).Decode(&errResp) == nil && errResp.Message != "" {
				return serviceTestMsg{err: fmt.Errorf("%s", errResp.Message)}
			}
			return serviceTestMsg{err: fmt.Errorf("daemon returned status %d", resp.StatusCode)}
		}

		var result struct {
			Message string `json:"message"`
		}
		if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
			return serviceTestMsg{err: err}
		}
		if strings.TrimSpace(result.Message) == "" {
			result.Message = "Service test passed"
		}
		return serviceTestMsg{resultMessage: result.Message}
	}
}

func compactServiceTestError(err error) string {
	if err == nil {
		return ""
	}
	msg := strings.TrimSpace(err.Error())
	if strings.Contains(msg, "status 404") || strings.Contains(strings.ToLower(msg), "page not found") {
		return "service test route unavailable; restart local daemon and re-bootstrap the device agent"
	}
	msg = strings.Join(strings.Fields(msg), " ")
	if len(msg) > 110 {
		return msg[:107] + "..."
	}
	return msg
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

func (d *Dashboard) Name() string { return "Dashboard" }

func (d *Dashboard) KeyBinds() []KeyBind {
	if d.mode == dashboardOverview {
		return []KeyBind{
			{Key: "tab", Help: "panel"},
			{Key: "j/k", Help: "move"},
			{Key: "enter", Help: "inspect"},
			{Key: "L", Help: "logs"},
			{Key: "s/r/x/t", Help: "service action"},
			{Key: "n", Help: "new"},
			{Key: "d", Help: "devices"},
			{Key: "c", Help: "ai tools"},
			{Key: "?", Help: "help"},
			{Key: "q", Help: "quit"},
		}
	}
	return []KeyBind{
		{Key: "h/l", Help: "device"},
		{Key: "j/k", Help: "service"},
		{Key: "enter", Help: "service detail"},
		{Key: "esc", Help: "overview"},
		{Key: "n", Help: "new"},
		{Key: "s", Help: "stop"},
		{Key: "r", Help: "restart"},
		{Key: "t", Help: "test"},
		{Key: "x", Help: "delete"},
		{Key: "L", Help: "logs"},
		{Key: "d", Help: "devices"},
		{Key: "c", Help: "ai tools"},
		{Key: "?", Help: "help"},
		{Key: "q", Help: "quit"},
	}
}
