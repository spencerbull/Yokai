package tailscale

import (
	"encoding/json"
	"fmt"
	"os/exec"
	"sort"
	"strings"
)

const AIGPUTag = "tag:ai-gpu"

// Peer represents a Tailscale peer device.
type Peer struct {
	HostName string   `json:"HostName"`
	DNSName  string   `json:"DNSName"`
	TailAddr string   `json:"TailscaleIPs"` // parsed from array
	IPs      []string // parsed from TailscaleIPs
	Online   bool     `json:"Online"`
	OS       string   `json:"OS"`
	Tags     []string `json:"Tags"`
}

// Status represents the result of `tailscale status --json`.
type Status struct {
	BackendState string                 `json:"BackendState"` // "Running", "Stopped", "NeedsLogin"
	Self         *statusPeer            `json:"Self"`
	Peers        map[string]*statusPeer `json:"Peer"`
}

type statusPeer struct {
	HostName     string   `json:"HostName"`
	DNSName      string   `json:"DNSName"`
	TailscaleIPs []string `json:"TailscaleIPs"`
	Online       bool     `json:"Online"`
	OS           string   `json:"OS"`
	Tags         []string `json:"Tags,omitempty"`
}

// IsInstalled checks if the tailscale CLI is available.
func IsInstalled() bool {
	_, err := exec.LookPath("tailscale")
	return err == nil
}

// GetStatus runs `tailscale status --json` and parses the result.
func GetStatus() (*Status, error) {
	out, err := exec.Command("tailscale", "status", "--json").Output()
	if err != nil {
		return nil, fmt.Errorf("running tailscale status: %w", err)
	}

	var status Status
	if err := json.Unmarshal(out, &status); err != nil {
		return nil, fmt.Errorf("parsing tailscale status: %w", err)
	}

	return &status, nil
}

// IsRunning returns true if Tailscale backend is in Running state.
func (s *Status) IsRunning() bool {
	return s.BackendState == "Running"
}

// NeedsLogin returns true if Tailscale needs authentication.
func (s *Status) NeedsLogin() bool {
	return s.BackendState == "NeedsLogin" || s.BackendState == "NeedsMachineAuth"
}

// ListPeers returns online peers as a simple slice.
func (s *Status) ListPeers() []Peer {
	var peers []Peer
	for _, p := range s.Peers {
		peer := Peer{
			HostName: p.HostName,
			DNSName:  strings.TrimSuffix(p.DNSName, "."),
			IPs:      p.TailscaleIPs,
			Online:   p.Online,
			OS:       p.OS,
			Tags:     p.Tags,
		}
		if len(p.TailscaleIPs) > 0 {
			peer.TailAddr = p.TailscaleIPs[0]
		}
		peers = append(peers, peer)
	}
	return peers
}

// OnlinePeers returns only peers that are currently online.
func (s *Status) OnlinePeers() []Peer {
	all := s.ListPeers()
	var online []Peer
	for _, p := range all {
		if p.Online {
			online = append(online, p)
		}
	}
	return online
}

// HasTag reports whether the peer has the given tag.
func (p Peer) HasTag(tag string) bool {
	tag = strings.ToLower(strings.TrimSpace(tag))
	if tag == "" {
		return false
	}
	for _, candidate := range p.Tags {
		if strings.ToLower(candidate) == tag {
			return true
		}
	}
	return false
}

// HighlightedTags returns Yokai-recognized tags in display order.
func (p Peer) HighlightedTags() []string {
	if p.HasTag(AIGPUTag) {
		return []string{"AI GPU"}
	}
	return nil
}

// OtherTags returns non-highlighted raw tags in sorted order.
func (p Peer) OtherTags() []string {
	if len(p.Tags) == 0 {
		return nil
	}
	other := make([]string, 0, len(p.Tags))
	for _, tag := range p.Tags {
		if strings.EqualFold(tag, AIGPUTag) {
			continue
		}
		other = append(other, tag)
	}
	sort.Strings(other)
	if len(other) == 0 {
		return nil
	}
	return other
}

// EnrollmentTagHelp explains how to mark AI GPU servers in Tailscale.
func EnrollmentTagHelp() string {
	return `Recommended Yokai tag: tag:ai-gpu

Define the tag in your tailnet policy:
  {
    "tagOwners": {
      "tag:ai-gpu": ["group:infra"]
    }
  }

Apply it to the server:
  Admin console: Machines -> device -> Edit tags -> tag:ai-gpu
  CLI: sudo tailscale set --advertise-tags=tag:ai-gpu

If the device was authenticated as a user, you may need:
  sudo tailscale up --advertise-tags=tag:ai-gpu --force-reauth

Use tags for non-human server nodes. In Tailscale, tags become the
device identity and replace user-based authentication on that machine.`
}

// InstallInstructions returns platform-specific install instructions.
func InstallInstructions() string {
	return `Install Tailscale:
  curl -fsSL https://tailscale.com/install.sh | sh

Then authenticate:
  sudo tailscale up

Visit the URL shown to complete login.`
}
