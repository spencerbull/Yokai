package views

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/spencerbull/yokai/internal/config"
	"github.com/spencerbull/yokai/internal/tui/components"
)

func TestDeleteServiceUsesDeleteEndpoint(t *testing.T) {
	t.Parallel()

	var gotMethod, gotPath string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		gotPath = r.URL.Path
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	cfg := &config.Config{
		Daemon: config.DaemonConfig{Listen: strings.TrimPrefix(server.URL, "http://")},
	}
	d := NewDashboard(cfg, "test")
	d.metrics = map[string]*DashboardMetrics{
		"dev-1": {
			Containers: []ContainerData{{ID: "cont-1", Name: "yokai-svc-a"}},
		},
	}

	msg := d.deleteService("cont-1", "svc-a")()
	deleteMsg, ok := msg.(serviceDeleteMsg)
	if !ok {
		t.Fatalf("expected serviceDeleteMsg, got %T", msg)
	}
	if deleteMsg.err != nil {
		t.Fatalf("expected nil error, got %v", deleteMsg.err)
	}
	if gotMethod != http.MethodDelete {
		t.Fatalf("expected method DELETE, got %s", gotMethod)
	}
	if gotPath != "/containers/dev-1/cont-1/remove" {
		t.Fatalf("unexpected path: %s", gotPath)
	}
}

func TestDeleteServiceReturnsErrorOnFailureStatus(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	cfg := &config.Config{
		Daemon: config.DaemonConfig{Listen: strings.TrimPrefix(server.URL, "http://")},
	}
	d := NewDashboard(cfg, "test")
	d.metrics = map[string]*DashboardMetrics{
		"dev-1": {
			Containers: []ContainerData{{ID: "cont-1", Name: "yokai-svc-a"}},
		},
	}

	msg := d.deleteService("cont-1", "svc-a")()
	deleteMsg, ok := msg.(serviceDeleteMsg)
	if !ok {
		t.Fatalf("expected serviceDeleteMsg, got %T", msg)
	}
	if deleteMsg.err == nil {
		t.Fatal("expected error for non-200 status")
	}
}

func TestTestServiceUsesTestEndpoint(t *testing.T) {
	t.Parallel()

	var gotMethod, gotPath string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		gotPath = r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"message":"service test passed"}`))
	}))
	defer server.Close()

	cfg := &config.Config{
		Daemon: config.DaemonConfig{Listen: strings.TrimPrefix(server.URL, "http://")},
	}
	d := NewDashboard(cfg, "test")
	d.metrics = map[string]*DashboardMetrics{
		"dev-1": {
			Containers: []ContainerData{{ID: "cont-1", Name: "yokai-svc-a"}},
		},
	}

	msg := d.testService("cont-1")()
	testMsg, ok := msg.(serviceTestMsg)
	if !ok {
		t.Fatalf("expected serviceTestMsg, got %T", msg)
	}
	if testMsg.err != nil {
		t.Fatalf("expected nil error, got %v", testMsg.err)
	}
	if gotMethod != http.MethodPost {
		t.Fatalf("expected method POST, got %s", gotMethod)
	}
	if gotPath != "/containers/dev-1/cont-1/test" {
		t.Fatalf("unexpected path: %s", gotPath)
	}
}

func TestDashboardUpdatePrunesServiceAfterDelete(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	cfg := &config.Config{
		Daemon: config.DaemonConfig{Listen: "127.0.0.1:7473"},
		Services: []config.Service{
			{ID: "svc-a", DeviceID: "dev-1", ContainerID: "cont-1"},
		},
	}
	d := NewDashboard(cfg, "test")

	_, _ = d.Update(serviceDeleteMsg{containerID: "cont-1", serviceID: "svc-a"})

	if len(cfg.Services) != 0 {
		t.Fatalf("expected service to be removed from config, got %d entries", len(cfg.Services))
	}
}

func TestDashboardOverviewShowsAllDevicesWhenHeightAllows(t *testing.T) {
	d := NewDashboard(config.DefaultConfig(), "test")
	d.width = 140
	d.height = 40
	d.devices = []DashboardDevice{
		{ID: "dev-1", Label: "alpha", Host: "10.0.0.1", Online: true},
		{ID: "dev-2", Label: "beta", Host: "10.0.0.2", Online: true},
		{ID: "dev-3", Label: "gamma", Host: "10.0.0.3", Online: true},
		{ID: "dev-4", Label: "delta", Host: "10.0.0.4", Online: true},
	}

	view := d.View()
	for _, label := range []string{"alpha", "beta", "gamma", "delta"} {
		if !strings.Contains(view, label) {
			t.Fatalf("expected overview to contain %q, got:\n%s", label, view)
		}
	}
}

func TestDashboardEnterDrillsIntoDeviceDetailAndEscReturns(t *testing.T) {
	d := NewDashboard(config.DefaultConfig(), "test")
	d.devices = []DashboardDevice{{ID: "dev-1", Label: "alpha", Host: "10.0.0.1", Online: true}}
	d.metrics = map[string]*DashboardMetrics{
		"dev-1": {Containers: []ContainerData{{ID: "cont-1", Name: "yokai-svc-a"}}},
	}
	d.updateServiceContainers()

	updated, _ := d.Update(tea.KeyMsg{Type: tea.KeyEnter})
	tv := updated.(*Dashboard)
	if tv.mode != dashboardDeviceDetail {
		t.Fatalf("expected dashboard detail mode, got %v", tv.mode)
	}
	if tv.activeDeviceID() != "dev-1" {
		t.Fatalf("expected active device dev-1, got %q", tv.activeDeviceID())
	}

	updated, _ = tv.Update(tea.KeyMsg{Type: tea.KeyEsc})
	tv = updated.(*Dashboard)
	if tv.mode != dashboardOverview {
		t.Fatalf("expected overview mode after esc, got %v", tv.mode)
	}
}

func TestDashboardServiceFilteringDiffersByMode(t *testing.T) {
	d := NewDashboard(config.DefaultConfig(), "test")
	d.devices = []DashboardDevice{
		{ID: "dev-1", Label: "alpha", Host: "10.0.0.1", Online: true},
		{ID: "dev-2", Label: "beta", Host: "10.0.0.2", Online: true},
	}
	d.metrics = map[string]*DashboardMetrics{
		"dev-1": {Containers: []ContainerData{{ID: "cont-1", Name: "yokai-svc-a"}}},
		"dev-2": {Containers: []ContainerData{{ID: "cont-2", Name: "yokai-svc-b"}}},
	}

	d.updateServiceContainers()
	if got := len(d.buildServiceRows()); got != 2 {
		t.Fatalf("expected overview to show 2 services, got %d", got)
	}

	d.selectedDevice = 1
	d.enterDeviceDetail()
	rows := d.buildServiceRows()
	if len(rows) != 1 {
		t.Fatalf("expected detail mode to show 1 service, got %d", len(rows))
	}
	if rows[0].Device != "beta" {
		t.Fatalf("expected detail service row for beta, got %q", rows[0].Device)
	}
}

func TestDashboardOverviewIncludesFleetPanelsAndServicePreview(t *testing.T) {
	d := NewDashboard(config.DefaultConfig(), "test")
	d.width = 140
	d.height = 42
	d.devices = []DashboardDevice{
		{ID: "dev-1", Label: "alpha", Host: "10.0.0.1", Online: true, CPUPercent: 12, RAMPercent: 30, ServiceCount: 2},
		{ID: "dev-2", Label: "beta", Host: "10.0.0.2", Online: true, CPUPercent: 22, RAMPercent: 40, ServiceCount: 1},
	}
	d.metrics = map[string]*DashboardMetrics{
		"dev-1": {GPUs: []GPUData{{Name: "RTX 4090", UtilPercent: 80, VRAMUsedMB: 10240, VRAMTotalMB: 24576}}, Containers: []ContainerData{{ID: "cont-1", Name: "yokai-svc-a", Status: "running"}}},
		"dev-2": {GPUs: []GPUData{{Name: "RTX 3090", UtilPercent: 20, VRAMUsedMB: 4096, VRAMTotalMB: 24576}}, Containers: []ContainerData{{ID: "cont-2", Name: "yokai-svc-b", Status: "running"}}},
	}
	d.updateServiceContainers()

	view := d.View()
	for _, snippet := range []string{"AI Fleet", "Fleet Runtime", "Per Device", "AI Services", "Monitoring Services", "GUtil", "VRAM"} {
		if !strings.Contains(view, snippet) {
			t.Fatalf("expected overview to contain %q, got:\n%s", snippet, view)
		}
	}
}

func TestDashboardBuildServiceRowsSortsAlertsFirst(t *testing.T) {
	d := NewDashboard(config.DefaultConfig(), "test")
	d.devices = []DashboardDevice{
		{ID: "dev-1", Label: "alpha", Host: "10.0.0.1", Online: true},
		{ID: "dev-2", Label: "beta", Host: "10.0.0.2", Online: true},
	}
	d.metrics = map[string]*DashboardMetrics{
		"dev-1": {Containers: []ContainerData{{ID: "cont-1", Name: "yokai-svc-healthy", Status: "running", Health: "healthy"}}},
		"dev-2": {Containers: []ContainerData{{ID: "cont-2", Name: "yokai-svc-error", Status: "exited", Health: "unhealthy"}}},
	}

	d.updateServiceContainers()
	rows := d.buildServiceRows()
	if len(rows) != 2 {
		t.Fatalf("expected 2 rows, got %d", len(rows))
	}
	if rows[0].Name != "svc-error" {
		t.Fatalf("expected alerting service first, got %q", rows[0].Name)
	}
}

func TestDashboardDetailSelectionMatchesSortedServiceRows(t *testing.T) {
	d := NewDashboard(config.DefaultConfig(), "test")
	d.devices = []DashboardDevice{{ID: "dev-1", Label: "alpha", Host: "10.0.0.1", Online: true}}
	d.metrics = map[string]*DashboardMetrics{
		"dev-1": {Containers: []ContainerData{
			{ID: "cont-healthy", Name: "yokai-svc-healthy", Status: "running", Health: "healthy"},
			{ID: "cont-error", Name: "yokai-svc-error", Status: "exited", Health: "unhealthy"},
		}},
	}

	d.enterDeviceDetail()
	rows := d.buildServiceRows()
	selected := d.getSelectedContainer()
	if len(rows) == 0 || selected == nil {
		t.Fatal("expected sorted rows and selected container to exist")
	}
	if rows[0].Name != strings.TrimPrefix(selected.Name, "yokai-") {
		t.Fatalf("expected selected container %q to match first rendered row %q", selected.Name, rows[0].Name)
	}
}

func TestFleetServiceAlertsIgnoreOfflineDevices(t *testing.T) {
	d := NewDashboard(config.DefaultConfig(), "test")
	d.devices = []DashboardDevice{
		{ID: "dev-1", Label: "alpha", Online: true},
		{ID: "dev-2", Label: "beta", Online: false},
	}
	d.metrics = map[string]*DashboardMetrics{
		"dev-1": {Containers: []ContainerData{{ID: "cont-1", Name: "yokai-svc-a", Status: "running", Health: "healthy"}}},
	}

	if got := d.fleetServiceAlerts(); got != 0 {
		t.Fatalf("expected 0 service alerts, got %d", got)
	}
}

func TestIsMonitoringServiceRowUsesMonitoringPrefix(t *testing.T) {
	if !isMonitoringServiceRow(components.ServiceRow{Name: "mon-grafana"}) {
		t.Fatal("expected mon-grafana to be treated as monitoring")
	}
	if isMonitoringServiceRow(components.ServiceRow{Name: "my-exporter-model"}) {
		t.Fatal("expected non-monitoring AI service names to stay out of monitoring panel")
	}
}

func TestDashboardOverviewGetSelectedContainerUsesFocusedPanel(t *testing.T) {
	d := NewDashboard(config.DefaultConfig(), "test")
	d.devices = []DashboardDevice{{ID: "dev-1", Label: "alpha", Online: true}}
	d.metrics = map[string]*DashboardMetrics{
		"dev-1": {Containers: []ContainerData{
			{ID: "cont-ai", Name: "yokai-vllm-model", Status: "running", Health: "healthy"},
			{ID: "cont-mon", Name: "yokai-mon-grafana", Status: "running", Health: "healthy"},
		}},
	}
	d.updateServiceContainers()

	d.overviewFocus = overviewFocusAIServices
	d.selectedAIServiceID = "cont-ai"
	selected := d.getSelectedContainer()
	if selected == nil || selected.ID != "cont-ai" {
		t.Fatalf("expected ai service selected, got %#v", selected)
	}

	d.overviewFocus = overviewFocusMonitoringServices
	d.selectedMonitoringServiceID = "cont-mon"
	selected = d.getSelectedContainer()
	if selected == nil || selected.ID != "cont-mon" {
		t.Fatalf("expected monitoring service selected, got %#v", selected)
	}
}

func TestDashboardOverviewEnterOnServiceOpensDeviceDetailForThatService(t *testing.T) {
	d := NewDashboard(config.DefaultConfig(), "test")
	d.devices = []DashboardDevice{{ID: "dev-1", Label: "alpha", Online: true}}
	d.metrics = map[string]*DashboardMetrics{
		"dev-1": {Containers: []ContainerData{{ID: "cont-ai", Name: "yokai-vllm-model", Status: "running", Health: "healthy"}}},
	}
	d.updateServiceContainers()
	d.overviewFocus = overviewFocusAIServices
	d.selectedAIServiceID = "cont-ai"

	updated, _ := d.Update(tea.KeyMsg{Type: tea.KeyEnter})
	tv := updated.(*Dashboard)
	if tv.mode != dashboardDeviceDetail {
		t.Fatalf("expected device detail mode, got %v", tv.mode)
	}
	selected := tv.getSelectedContainer()
	if selected == nil || selected.ID != "cont-ai" {
		t.Fatalf("expected selected service cont-ai in detail mode, got %#v", selected)
	}
	if !tv.showServiceDetail {
		t.Fatal("expected enter from overview service panel to open service detail")
	}
}

func TestDashboardOverviewSelectionStaysWithinVisibleServicePreview(t *testing.T) {
	d := NewDashboard(config.DefaultConfig(), "test")
	d.width = 140
	d.height = 26
	d.devices = []DashboardDevice{{ID: "dev-1", Label: "alpha", Online: true}}
	d.metrics = map[string]*DashboardMetrics{
		"dev-1": {Containers: []ContainerData{
			{ID: "cont-1", Name: "yokai-vllm-1", Status: "running", Health: "healthy"},
			{ID: "cont-2", Name: "yokai-vllm-2", Status: "running", Health: "healthy"},
			{ID: "cont-3", Name: "yokai-vllm-3", Status: "running", Health: "healthy"},
			{ID: "cont-4", Name: "yokai-vllm-4", Status: "running", Health: "healthy"},
			{ID: "cont-5", Name: "yokai-vllm-5", Status: "running", Health: "healthy"},
		}},
	}
	d.selectedAIServiceID = "cont-5"
	d.updateServiceContainers()
	d.overviewFocus = overviewFocusAIServices

	selected := d.getSelectedContainer()
	visible := d.visibleOverviewServiceContainersForFocus(overviewFocusAIServices)
	if len(visible) == 0 || selected == nil {
		t.Fatal("expected visible overview services and a selected container")
	}
	if selected.ID != visible[0].ID {
		t.Fatalf("expected selection to clamp to first visible preview service %q, got %q", visible[0].ID, selected.ID)
	}
}
