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

	// Device tab selection
	activeDeviceTab int

	// Service selection
	selectedService int
	showDetail      bool

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

	case tea.MouseMsg:
		if msg.Action == tea.MouseActionRelease {
			// Check if a device tab was clicked
			for i := range d.devices {
				if zone.Get(fmt.Sprintf("device-tab-%d", i)).InBounds(msg) {
					if i != d.activeDeviceTab {
						d.activeDeviceTab = i
						d.selectedService = 0
						d.showDetail = false
						d.updateServiceContainers()
					}
					break
				}
			}
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
		case "esc":
			if d.showDetail {
				d.showDetail = false
				return d, nil
			}
		case "j", "down":
			d.moveServiceCursor(1)
			d.showDetail = false
		case "k", "up":
			d.moveServiceCursor(-1)
			d.showDetail = false
		case "enter":
			if len(d.serviceContainers) > 0 {
				d.showDetail = !d.showDetail
			}
		case "h", "left":
			d.moveDeviceTab(-1)
		case "l", "right":
			d.moveDeviceTab(1)
		case "L":
			if container := d.getSelectedContainer(); container != nil {
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
		case "x":
			if container := d.getSelectedContainer(); container != nil {
				containerID := container.ID
				serviceID := strings.TrimPrefix(container.Name, "yokai-")
				msg := fmt.Sprintf("Delete service %q? This stops and removes the container.", container.Name)
				return d, Navigate(NewConfirmView(msg, d.deleteService(containerID, serviceID), nil))
			}
		case "g":
			// Open Grafana - could implement later
			return d, nil
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

	// Device tabs / header
	if len(d.devices) > 0 {
		// Clamp active tab
		if d.activeDeviceTab >= len(d.devices) {
			d.activeDeviceTab = len(d.devices) - 1
		}
		sections = append(sections, d.renderDeviceTabBar())
	} else {
		sections = append(sections, d.renderHeader())
	}

	// Responsive layout based on terminal width
	// Wide (≥140): 3 cols; Medium (100-139): 2 cols; Narrow (<100): single stack
	switch {
	case d.width >= 140:
		sections = append(sections, d.renderWideLayout())
	case d.width >= 100:
		sections = append(sections, d.renderMediumLayout())
	default:
		sections = append(sections, d.renderNarrowLayout())
	}

	// Service list (full width, filtered to active device)
	sections = append(sections, d.renderServiceList())

	// Service detail panel (expandable)
	if d.showDetail {
		if detail := d.renderServiceDetail(); detail != "" {
			sections = append(sections, detail)
		}
	}

	content := lipgloss.JoinVertical(lipgloss.Left, sections...)
	return lipgloss.NewStyle().Width(d.width).Render(content)
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
		device := d.devices[d.activeDeviceTab]
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

		device := d.devices[d.activeDeviceTab]
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
		if d.activeDeviceTab >= len(d.devices) {
			d.activeDeviceTab = len(d.devices) - 1
		}
		device := d.devices[d.activeDeviceTab]

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
		focused := (i == d.activeDeviceTab)
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
	innerWidth := chartWidth - 4
	if innerWidth < 10 {
		innerWidth = 10
	}

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
		if i == d.activeDeviceTab {
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
	d.activeDeviceTab = (d.activeDeviceTab + delta + len(d.devices)) % len(d.devices)
	// Reset service selection when switching devices
	d.selectedService = 0
	d.showDetail = false
	d.updateServiceContainers()
}

// activeDeviceID returns the device ID for the currently selected tab.
func (d *Dashboard) activeDeviceID() string {
	if len(d.devices) == 0 {
		return ""
	}
	if d.activeDeviceTab >= len(d.devices) {
		d.activeDeviceTab = len(d.devices) - 1
	}
	return d.devices[d.activeDeviceTab].ID
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

	title := theme.TitleStyle.Render("Services")
	content := serviceList.Render()

	return theme.RenderPanel(title, content, d.width)
}

func (d *Dashboard) buildServiceRows() []components.ServiceRow {
	var rows []components.ServiceRow

	activeID := d.activeDeviceID()

	// Build from container data in metrics (filtered to active device)
	for deviceID, metrics := range d.metrics {
		if activeID != "" && deviceID != activeID {
			continue
		}
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
				Name:                displayName,
				Type:                serviceType,
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
			rows = append(rows, row)
		}
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
		return
	}
	if m, ok := d.metrics[activeID]; ok {
		d.serviceContainers = append(d.serviceContainers, m.Containers...)
	}
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
	return []KeyBind{
		{Key: "h/l", Help: "device"},
		{Key: "j/k", Help: "service"},
		{Key: "enter", Help: "detail"},
		{Key: "n", Help: "new"},
		{Key: "s", Help: "stop"},
		{Key: "r", Help: "restart"},
		{Key: "x", Help: "delete"},
		{Key: "L", Help: "logs"},
		{Key: "d", Help: "devices"},
		{Key: "c", Help: "ai tools"},
		{Key: "?", Help: "help"},
		{Key: "q", Help: "quit"},
	}
}
