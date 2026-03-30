package views

import (
	"strings"
	"testing"

	"github.com/spencerbull/yokai/internal/config"
)

func TestBootstrapFailedStateUsesCopyableErrorBox(t *testing.T) {
	b := NewBootstrap(config.DefaultConfig(), "test", "100.64.0.2", "beskar", "tailscale", "root", "", "", "", 22, nil)
	b.step = bsFailed
	b.err = "tailscale: tailnet policy does not permit you to SSH to this node"
	b.authURL = "https://login.tailscale.com/a/test"
	b.setErrorBoxContent()

	if !b.InputActive() {
		t.Fatal("expected failed bootstrap state to activate error input")
	}

	content := b.errorBox.Value()
	for _, snippet := range []string{"tailnet policy does not permit", "https://login.tailscale.com/a/test"} {
		if !strings.Contains(content, snippet) {
			t.Fatalf("expected error box to contain %q, got %q", snippet, content)
		}
	}
}
