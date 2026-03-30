package views

import (
	"testing"

	"github.com/spencerbull/yokai/internal/config"
)

func TestNewSSHCredsDoesNotDefaultUsername(t *testing.T) {
	v := NewSSHCreds(config.DefaultConfig(), "test", "100.126.187.30", "beskar", "tailscale", nil)
	if got := v.userInput.Value(); got != "" {
		t.Fatalf("expected blank username by default, got %q", got)
	}
}
