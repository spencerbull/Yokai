package daemon

import (
	"testing"

	"github.com/spencerbull/yokai/internal/config"
)

func TestPersistDeployResult(t *testing.T) {
	configureTestConfigHome(t)

	cfg := config.DefaultConfig()
	cfg.Devices = []config.Device{{ID: "dev-1", Label: "alpha", Host: "10.0.0.1", AgentPort: 7474}}
	if err := config.Save(cfg); err != nil {
		t.Fatalf("save config: %v", err)
	}

	d := &Daemon{cfg: cfg}
	err := d.persistDeployResult(DeployRequest{
		DeviceID:    "dev-1",
		ServiceType: "vllm",
		Image:       "vllm/vllm-openai:latest",
		Name:        "vllm-meta-llama-3-1",
		Model:       "meta-llama/Llama-3.1-8B-Instruct",
		Ports:       map[string]string{"8000": "8000"},
		Env:         map[string]string{"MODEL": "meta-llama/Llama-3.1-8B-Instruct"},
		GPUIDs:      "all",
		ExtraArgs:   "--max-model-len 32768",
		Volumes:     map[string]string{"/data": "/models"},
	}, &DeployResult{ContainerID: "abc123", Status: "running", Ports: map[string]string{"8000": "8000"}})
	if err != nil {
		t.Fatalf("persist deploy result: %v", err)
	}

	if len(d.cfg.Services) != 1 {
		t.Fatalf("expected 1 service, got %d", len(d.cfg.Services))
	}
	svc := d.cfg.Services[0]
	if svc.ID != "vllm-meta-llama-3-1" || svc.Type != "vllm" || svc.ContainerID != "abc123" {
		t.Fatalf("unexpected persisted service: %#v", svc)
	}
	if svc.Port != 8000 {
		t.Fatalf("expected port 8000, got %d", svc.Port)
	}

	loaded, err := config.Load()
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	if len(loaded.Services) != 1 || loaded.Services[0].ContainerID != "abc123" {
		t.Fatalf("unexpected saved services: %#v", loaded.Services)
	}
}
