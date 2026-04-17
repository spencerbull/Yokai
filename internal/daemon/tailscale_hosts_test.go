package daemon

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/spencerbull/yokai/internal/config"
)

func TestApplyCurrentTailscaleHostAliases(t *testing.T) {
	stubTailscaleCLI(t, `{
	  "BackendState": "Running",
	  "Peer": {
	    "peer-1": {
	      "HostName": "gpu-1",
	      "DNSName": "gpu-1.tailnet.ts.net.",
	      "TailscaleIPs": ["100.64.0.2"],
	      "Online": true
	    },
	    "peer-2": {
	      "HostName": "gpu-2",
	      "DNSName": "gpu-2.tailnet.ts.net.",
	      "TailscaleIPs": ["100.64.0.3"],
	      "Online": true
	    }
	  }
	}`)

	cfg := &config.Config{
		Devices: []config.Device{
			{ID: "gpu-1", Host: "100.64.0.2", ConnectionType: "tailscale"},
			{ID: "gpu-2", Host: "gpu-2", ConnectionType: "tailscale"},
			{ID: "manual-1", Host: "100.64.0.9", ConnectionType: "manual"},
		},
	}

	if changed := applyCurrentTailscaleHostAliases(cfg); !changed {
		t.Fatal("expected tailscale hosts to be updated")
	}
	if cfg.Devices[0].Host != "gpu-1.tailnet.ts.net" {
		t.Fatalf("expected first tailscale host to be normalized, got %q", cfg.Devices[0].Host)
	}
	if cfg.Devices[1].Host != "gpu-2.tailnet.ts.net" {
		t.Fatalf("expected second tailscale host to be normalized, got %q", cfg.Devices[1].Host)
	}
	if cfg.Devices[2].Host != "100.64.0.9" {
		t.Fatalf("expected non-tailscale host to remain unchanged, got %q", cfg.Devices[2].Host)
	}
	if cfg.Devices[0].ID != "gpu-1" || cfg.Devices[1].ID != "gpu-2" {
		t.Fatalf("expected device IDs to remain stable, got %#v", cfg.Devices)
	}
}

func TestPreferTailscaleHostInRequest(t *testing.T) {
	stubTailscaleCLI(t, `{
	  "BackendState": "Running",
	  "Peer": {
	    "peer-1": {
	      "HostName": "gpu-1",
	      "DNSName": "gpu-1.tailnet.ts.net.",
	      "TailscaleIPs": ["100.64.0.2"],
	      "Online": true
	    }
	  }
	}`)

	tailscaleReq := preferTailscaleHostInRequest(deviceUpsertRequest{
		Host:           "100.64.0.2",
		ConnectionType: "tailscale",
	})
	if tailscaleReq.Host != "gpu-1.tailnet.ts.net" {
		t.Fatalf("expected tailscale host to be normalized, got %q", tailscaleReq.Host)
	}

	manualReq := preferTailscaleHostInRequest(deviceUpsertRequest{
		Host:           "100.64.0.2",
		ConnectionType: "manual",
	})
	if manualReq.Host != "100.64.0.2" {
		t.Fatalf("expected manual host to remain unchanged, got %q", manualReq.Host)
	}
}

func stubTailscaleCLI(t *testing.T, statusJSON string) {
	t.Helper()

	binDir := t.TempDir()
	scriptPath := filepath.Join(binDir, "tailscale")
	script := "#!/bin/sh\n" +
		"if [ \"$1\" = \"status\" ] && [ \"$2\" = \"--json\" ]; then\n" +
		"  cat <<'EOF'\n" + statusJSON + "\nEOF\n" +
		"  exit 0\n" +
		"fi\n" +
		"exit 1\n"
	if err := os.WriteFile(scriptPath, []byte(script), 0755); err != nil {
		t.Fatalf("write tailscale stub: %v", err)
	}

	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))
}
