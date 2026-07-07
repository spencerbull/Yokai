package daemon

import (
	"bytes"
	"encoding/json"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/spencerbull/yokai/internal/config"
)

func TestHandleCreateDevice(t *testing.T) {
	configureTestConfigHome(t)

	cfg := config.DefaultConfig()
	if err := config.Save(cfg); err != nil {
		t.Fatalf("save config: %v", err)
	}

	tunnels := NewTunnelPool(cfg)
	defer tunnels.CloseAll()
	d := &Daemon{cfg: cfg, tunnels: tunnels, aggregator: NewAggregator(cfg, tunnels)}
	body := bytes.NewBufferString(`{"label":"alpha","host":"10.0.0.1","ssh_user":"root","ssh_key":"~/.ssh/id_ed25519","agent_port":7474,"connection_type":"manual"}`)
	req := httptest.NewRequest("POST", "/devices", body)
	rr := httptest.NewRecorder()

	d.handleCreateDevice(rr, req)

	if rr.Code != 201 {
		t.Fatalf("expected status 201, got %d: %s", rr.Code, rr.Body.String())
	}
	if len(d.cfg.Devices) != 1 {
		t.Fatalf("expected 1 device in memory, got %d", len(d.cfg.Devices))
	}
	if d.cfg.Devices[0].ID != "10.0.0.1" {
		t.Fatalf("expected host-derived device id, got %q", d.cfg.Devices[0].ID)
	}

	loaded, err := config.Load()
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	if len(loaded.Devices) != 1 || loaded.Devices[0].Label != "alpha" {
		t.Fatalf("unexpected saved devices: %#v", loaded.Devices)
	}
}

func TestHandleUpdateDevice(t *testing.T) {
	configureTestConfigHome(t)

	cfg := config.DefaultConfig()
	cfg.Devices = []config.Device{{ID: "dev-1", Label: "alpha", Host: "10.0.0.1", AgentPort: 7474, ConnectionType: "manual"}}
	if err := config.Save(cfg); err != nil {
		t.Fatalf("save config: %v", err)
	}

	tunnels := NewTunnelPool(cfg)
	defer tunnels.CloseAll()
	d := &Daemon{cfg: cfg, tunnels: tunnels, aggregator: NewAggregator(cfg, tunnels)}
	body := bytes.NewBufferString(`{"label":"beta","host":"10.0.0.2","ssh_user":"sbull","ssh_port":2222,"agent_port":8484,"connection_type":"ssh-config"}`)
	req := httptest.NewRequest("PUT", "/devices/dev-1", body)
	req.SetPathValue("deviceID", "dev-1")
	rr := httptest.NewRecorder()

	d.handleUpdateDevice(rr, req)

	if rr.Code != 200 {
		t.Fatalf("expected status 200, got %d: %s", rr.Code, rr.Body.String())
	}
	updated := d.cfg.FindDevice("dev-1")
	if updated == nil || updated.Label != "beta" || updated.Host != "10.0.0.2" || updated.AgentPort != 8484 {
		t.Fatalf("unexpected updated device: %#v", updated)
	}
}

func TestHandleDeleteDevice(t *testing.T) {
	configureTestConfigHome(t)

	cfg := config.DefaultConfig()
	cfg.Devices = []config.Device{{ID: "dev-1", Label: "alpha", Host: "10.0.0.1", AgentPort: 7474, ConnectionType: "manual"}}
	cfg.Services = []config.Service{{ID: "svc-1", DeviceID: "dev-1", ContainerID: "cont-1"}}
	if err := config.Save(cfg); err != nil {
		t.Fatalf("save config: %v", err)
	}

	tunnels := NewTunnelPool(cfg)
	defer tunnels.CloseAll()
	aggregator := NewAggregator(cfg, tunnels)
	d := &Daemon{cfg: cfg, tunnels: tunnels, aggregator: aggregator}
	req := httptest.NewRequest("DELETE", "/devices/dev-1", nil)
	req.SetPathValue("deviceID", "dev-1")
	rr := httptest.NewRecorder()

	d.handleDeleteDevice(rr, req)

	if rr.Code != 200 {
		t.Fatalf("expected status 200, got %d: %s", rr.Code, rr.Body.String())
	}
	if len(d.cfg.Devices) != 0 || len(d.cfg.Services) != 0 {
		t.Fatalf("expected device and services removed, got devices=%#v services=%#v", d.cfg.Devices, d.cfg.Services)
	}

	var resp deviceDeleteResponse
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp.RemovedServices != 1 {
		t.Fatalf("expected 1 removed service, got %d", resp.RemovedServices)
	}
}

func TestHandleSSHConfigHosts(t *testing.T) {
	configureTestConfigHome(t)

	home := os.Getenv("HOME")
	sshDir := filepath.Join(home, ".ssh")
	if err := os.MkdirAll(sshDir, 0700); err != nil {
		t.Fatalf("mkdir ssh dir: %v", err)
	}

	sshConfigPath := filepath.Join(sshDir, "config")
	content := []byte("Host gpu-box\n  HostName 10.0.0.20\n  User root\n  Port 2222\n  IdentityFile ~/.ssh/id_ed25519\n")
	if err := os.WriteFile(sshConfigPath, content, 0600); err != nil {
		t.Fatalf("write ssh config: %v", err)
	}

	d := &Daemon{cfg: config.DefaultConfig(), tunnels: NewTunnelPool(config.DefaultConfig()), aggregator: NewAggregator(config.DefaultConfig(), NewTunnelPool(config.DefaultConfig()))}
	req := httptest.NewRequest("GET", "/discovery/ssh-config-hosts", nil)
	rr := httptest.NewRecorder()

	d.handleSSHConfigHosts(rr, req)

	if rr.Code != 200 {
		t.Fatalf("expected status 200, got %d: %s", rr.Code, rr.Body.String())
	}

	var payload struct {
		Hosts []sshConfigHostRecord `json:"hosts"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(payload.Hosts) != 1 {
		t.Fatalf("expected 1 host, got %d", len(payload.Hosts))
	}
	if payload.Hosts[0].Alias != "gpu-box" || payload.Hosts[0].Host != "10.0.0.20" || payload.Hosts[0].Port != 2222 {
		t.Fatalf("unexpected host payload: %#v", payload.Hosts[0])
	}
}
