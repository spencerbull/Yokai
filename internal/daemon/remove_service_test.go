package daemon

import (
	"testing"

	"github.com/spencerbull/yokai/internal/config"
)

func TestRemoveServiceByContainerID(t *testing.T) {
	configureTestConfigHome(t)

	cfg := config.DefaultConfig()
	cfg.Services = []config.Service{
		{ID: "svc-a", DeviceID: "dev-1", ContainerID: "cont-a"},
		{ID: "svc-b", DeviceID: "dev-1", ContainerID: "cont-b"},
	}
	if err := config.Save(cfg); err != nil {
		t.Fatalf("save config: %v", err)
	}

	d := &Daemon{cfg: cfg}

	removed, err := d.removeServiceByContainerID("cont-a")
	if err != nil {
		t.Fatalf("remove service: %v", err)
	}
	if removed != 1 {
		t.Fatalf("expected 1 removed service, got %d", removed)
	}
	if len(d.cfg.Services) != 1 || d.cfg.Services[0].ID != "svc-b" {
		t.Fatalf("unexpected in-memory services: %#v", d.cfg.Services)
	}

	loaded, err := config.Load()
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	if len(loaded.Services) != 1 || loaded.Services[0].ID != "svc-b" {
		t.Fatalf("unexpected saved services: %#v", loaded.Services)
	}
}
