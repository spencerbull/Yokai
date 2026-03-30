package tailscale

import (
	"reflect"
	"strings"
	"testing"
)

func TestStatusListPeersPreservesTags(t *testing.T) {
	t.Parallel()

	status := &Status{
		Peers: map[string]*statusPeer{
			"node-1": {
				HostName:     "gpu-box",
				DNSName:      "gpu-box.tailnet.ts.net.",
				TailscaleIPs: []string{"100.64.0.2"},
				Online:       true,
				OS:           "linux",
				Tags:         []string{AIGPUTag, "tag:nvidia"},
			},
		},
	}

	peers := status.ListPeers()
	if len(peers) != 1 {
		t.Fatalf("expected 1 peer, got %d", len(peers))
	}
	peer := peers[0]
	if peer.DNSName != "gpu-box.tailnet.ts.net" {
		t.Fatalf("expected trimmed DNS name, got %q", peer.DNSName)
	}
	if peer.TailAddr != "100.64.0.2" {
		t.Fatalf("expected primary Tailscale IP, got %q", peer.TailAddr)
	}
	if !reflect.DeepEqual(peer.Tags, []string{AIGPUTag, "tag:nvidia"}) {
		t.Fatalf("expected tags to be preserved, got %#v", peer.Tags)
	}
}

func TestPeerTagHelpers(t *testing.T) {
	t.Parallel()

	peer := Peer{Tags: []string{"TAG:AI-GPU", "tag:zeta", "tag:alpha"}}

	if !peer.HasTag(AIGPUTag) {
		t.Fatal("expected ai-gpu tag to match case-insensitively")
	}
	if got := peer.HighlightedTags(); !reflect.DeepEqual(got, []string{"AI GPU"}) {
		t.Fatalf("unexpected highlighted tags: %#v", got)
	}
	if got := peer.OtherTags(); !reflect.DeepEqual(got, []string{"tag:alpha", "tag:zeta"}) {
		t.Fatalf("unexpected other tags: %#v", got)
	}
}

func TestEnrollmentTagHelpMentionsAIGPUTag(t *testing.T) {
	t.Parallel()

	help := EnrollmentTagHelp()
	for _, snippet := range []string{AIGPUTag, "tagOwners", "--advertise-tags=tag:ai-gpu"} {
		if !strings.Contains(help, snippet) {
			t.Fatalf("expected help to contain %q", snippet)
		}
	}
}
