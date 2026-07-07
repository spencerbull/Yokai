package daemon

import (
	"net/http"
	"sort"

	"github.com/spencerbull/yokai/internal/tailscale"
)

type tailscaleSelfRecord struct {
	Hostname string `json:"hostname,omitempty"`
	DNSName  string `json:"dns_name,omitempty"`
	IP       string `json:"ip,omitempty"`
}

type tailscaleStatusResponse struct {
	Installed           bool                 `json:"installed"`
	Running             bool                 `json:"running"`
	NeedsLogin          bool                 `json:"needs_login"`
	BackendState        string               `json:"backend_state,omitempty"`
	Self                *tailscaleSelfRecord `json:"self,omitempty"`
	Error               string               `json:"error,omitempty"`
	InstallInstructions string               `json:"install_instructions,omitempty"`
	TagHelp             string               `json:"tag_help,omitempty"`
}

type tailscalePeerRecord struct {
	Hostname        string   `json:"hostname"`
	DNSName         string   `json:"dns_name,omitempty"`
	IP              string   `json:"ip"`
	IPs             []string `json:"ips,omitempty"`
	OS              string   `json:"os,omitempty"`
	Online          bool     `json:"online"`
	Tags            []string `json:"tags,omitempty"`
	HighlightedTags []string `json:"highlighted_tags,omitempty"`
	OtherTags       []string `json:"other_tags,omitempty"`
	Recommended     bool     `json:"recommended"`
}

func (d *Daemon) handleTailscaleStatus(w http.ResponseWriter, r *http.Request) {
	if !tailscale.IsInstalled() {
		writeJSON(w, http.StatusOK, tailscaleStatusResponse{
			Installed:           false,
			Running:             false,
			NeedsLogin:          false,
			InstallInstructions: tailscale.InstallInstructions(),
			TagHelp:             tailscale.EnrollmentTagHelp(),
		})
		return
	}

	status, err := tailscale.GetStatus()
	if err != nil {
		writeJSON(w, http.StatusOK, tailscaleStatusResponse{
			Installed:           true,
			Running:             false,
			NeedsLogin:          false,
			Error:               err.Error(),
			InstallInstructions: tailscale.InstallInstructions(),
			TagHelp:             tailscale.EnrollmentTagHelp(),
		})
		return
	}

	response := tailscaleStatusResponse{
		Installed:           true,
		Running:             status.IsRunning(),
		NeedsLogin:          status.NeedsLogin(),
		BackendState:        status.BackendState,
		InstallInstructions: tailscale.InstallInstructions(),
		TagHelp:             tailscale.EnrollmentTagHelp(),
	}
	if status.Self != nil {
		response.Self = &tailscaleSelfRecord{
			Hostname: status.Self.HostName,
			DNSName:  status.Self.DNSName,
		}
		if len(status.Self.TailscaleIPs) > 0 {
			response.Self.IP = status.Self.TailscaleIPs[0]
		}
	}

	writeJSON(w, http.StatusOK, response)
}

func (d *Daemon) handleTailscalePeers(w http.ResponseWriter, r *http.Request) {
	if !tailscale.IsInstalled() {
		writeJSON(w, http.StatusOK, map[string]interface{}{"peers": []tailscalePeerRecord{}})
		return
	}

	status, err := tailscale.GetStatus()
	if err != nil || !status.IsRunning() {
		writeJSON(w, http.StatusOK, map[string]interface{}{"peers": []tailscalePeerRecord{}})
		return
	}

	peers := status.OnlinePeers()
	records := make([]tailscalePeerRecord, 0, len(peers))
	for _, peer := range peers {
		records = append(records, tailscalePeerRecord{
			Hostname:        peer.HostName,
			DNSName:         peer.DNSName,
			IP:              peer.TailAddr,
			IPs:             append([]string(nil), peer.IPs...),
			OS:              peer.OS,
			Online:          peer.Online,
			Tags:            append([]string(nil), peer.Tags...),
			HighlightedTags: peer.HighlightedTags(),
			OtherTags:       peer.OtherTags(),
			Recommended:     peer.HasTag(tailscale.AIGPUTag),
		})
	}

	sort.SliceStable(records, func(i, j int) bool {
		if records[i].Recommended != records[j].Recommended {
			return records[i].Recommended
		}
		return records[i].Hostname < records[j].Hostname
	})

	writeJSON(w, http.StatusOK, map[string]interface{}{"peers": records})
}
