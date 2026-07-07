package ssh

import "testing"

func TestUsesTailscaleSSHProbe(t *testing.T) {
	tests := []struct {
		name string
		cfg  ClientConfig
		want bool
	}{
		{
			name: "tailscale connection type",
			cfg:  ClientConfig{Host: "gpu-box", ConnectionType: "tailscale"},
			want: true,
		},
		{
			name: "magicdns tailnet host",
			cfg:  ClientConfig{Host: "gpu-box.example.ts.net"},
			want: true,
		},
		{
			name: "tailscale ipv4",
			cfg:  ClientConfig{Host: "100.64.0.12"},
			want: true,
		},
		{
			name: "non tailscale host",
			cfg:  ClientConfig{Host: "192.168.1.10"},
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := usesTailscaleSSHProbe(tt.cfg); got != tt.want {
				t.Fatalf("usesTailscaleSSHProbe() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestExtractTailscaleAuthURL(t *testing.T) {
	output := "To continue, visit: https://login.tailscale.com/a/123456\nThen retry your SSH command."
	if got := extractTailscaleAuthURL(output); got != "https://login.tailscale.com/a/123456" {
		t.Fatalf("unexpected auth URL: %q", got)
	}
}
