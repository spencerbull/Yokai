package daemon

import (
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/spencerbull/yokai/internal/config"
)

func TestLifecycleMutationsOutliveMetricsClientTimeout(t *testing.T) {
	const token = "test-token"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		if request.Header.Get("Authorization") != "Bearer "+token {
			t.Errorf("missing agent authorization")
		}
		time.Sleep(25 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	port := server.Listener.Addr().(*net.TCPAddr).Port
	cfg := config.DefaultConfig()
	cfg.Devices = []config.Device{{ID: "spark", AgentToken: token}}
	tunnels := NewTunnelPool(cfg)
	tunnels.tunnels["spark"] = &tunnel{deviceID: "spark", localPort: port, connected: true}
	aggregator := NewAggregator(cfg, tunnels)
	aggregator.client = &http.Client{Timeout: 5 * time.Millisecond}
	if aggregator.mutationClient.Timeout < 2*time.Minute {
		t.Fatalf("lifecycle timeout is below the two-minute safety floor: %s", aggregator.mutationClient.Timeout)
	}

	for name, mutate := range map[string]func() error{
		"stop":    func() error { return aggregator.StopContainer("spark", "container") },
		"remove":  func() error { return aggregator.RemoveContainer("spark", "container") },
		"restart": func() error { return aggregator.RestartContainer("spark", "container") },
	} {
		t.Run(name, func(t *testing.T) {
			if err := mutate(); err != nil {
				t.Fatalf("lifecycle mutation inherited short metrics timeout: %v", err)
			}
		})
	}
}

func TestServiceProbeOutlivesMetricsClientTimeout(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		time.Sleep(25 * time.Millisecond)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ok":true,"metrics_ready":true,"message":"ready"}`))
	}))
	defer server.Close()

	port := server.Listener.Addr().(*net.TCPAddr).Port
	cfg := config.DefaultConfig()
	cfg.Devices = []config.Device{{ID: "spark"}}
	tunnels := NewTunnelPool(cfg)
	tunnels.tunnels["spark"] = &tunnel{deviceID: "spark", localPort: port, connected: true}
	aggregator := NewAggregator(cfg, tunnels)
	aggregator.client = &http.Client{Timeout: 5 * time.Millisecond}
	if aggregator.probeClient.Timeout < 2*time.Minute {
		t.Fatalf("service-probe timeout is below the two-minute safety floor: %s", aggregator.probeClient.Timeout)
	}

	if _, err := aggregator.TestContainerWithMetrics("spark", "container", "request-key"); err != nil {
		t.Fatalf("service probe inherited short metrics timeout: %v", err)
	}
}

func TestAggregatorConfigReloadConcurrentReads(t *testing.T) {
	first := config.DefaultConfig()
	first.Devices = []config.Device{{ID: "first", AgentToken: "first-token"}}
	second := config.DefaultConfig()
	second.Daemon.MetricsPollInterval = first.Daemon.MetricsPollInterval + 1
	second.Devices = []config.Device{{ID: "second", AgentToken: "second-token"}}
	aggregator := NewAggregator(first, NewTunnelPool(first))

	reloadsDone := make(chan struct{})
	go func() {
		defer close(reloadsDone)
		for index := 0; index < 1000; index++ {
			if index%2 == 0 {
				aggregator.UpdateConfig(second)
			} else {
				aggregator.UpdateConfig(first)
			}
		}
	}()
	for index := 0; index < 1000; index++ {
		if _, err := aggregator.agentRequest(http.MethodGet, "http://127.0.0.1/health", "first", nil); err != nil {
			t.Fatal(err)
		}
		if aggregator.metricsPollInterval() <= 0 {
			t.Fatal("config reload produced a non-positive poll interval")
		}
		_ = aggregator.configuredDeviceIDs()
	}
	<-reloadsDone
}
