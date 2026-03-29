package views

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/spencerbull/yokai/internal/config"
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
