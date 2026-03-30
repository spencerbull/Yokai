package views

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/spencerbull/yokai/internal/config"
	"github.com/spencerbull/yokai/internal/tailscale"
)

func TestTailscaleViewHelpRowSelectable(t *testing.T) {
	v := NewTailscaleView(config.DefaultConfig(), "test")
	v.state = tsPeerList
	v.peers = []tailscale.Peer{{HostName: "beskar", TailAddr: "100.64.0.2", Online: true, OS: "linux"}}

	updated, _ := v.Update(tea.KeyMsg{Type: tea.KeyDown})
	tv := updated.(*TailscaleView)
	if tv.cursor != 1 {
		t.Fatalf("expected cursor to move to help row, got %d", tv.cursor)
	}

	updated, _ = tv.Update(tea.KeyMsg{Type: tea.KeyEnter})
	tv = updated.(*TailscaleView)
	if !tv.showTagHelp {
		t.Fatal("expected enter on help row to open tag help")
	}
}

func TestTailscaleViewHelpRowSelectableWithoutPeers(t *testing.T) {
	v := NewTailscaleView(config.DefaultConfig(), "test")
	v.state = tsPeerList

	updated, _ := v.Update(tea.KeyMsg{Type: tea.KeyEnter})
	tv := updated.(*TailscaleView)
	if !tv.showTagHelp {
		t.Fatal("expected help row to be selectable even when no peers are visible")
	}
}
