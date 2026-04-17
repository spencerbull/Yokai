package daemon

import (
	"strings"

	"github.com/spencerbull/yokai/internal/config"
	"github.com/spencerbull/yokai/internal/tailscale"
)

func currentTailscaleStatus() *tailscale.Status {
	if !tailscale.IsInstalled() {
		return nil
	}

	status, err := tailscale.GetStatus()
	if err != nil || !status.IsRunning() {
		return nil
	}

	return status
}

func preferredTailscaleHost(host string, connectionType string, status *tailscale.Status) (string, bool) {
	if status == nil || !strings.EqualFold(strings.TrimSpace(connectionType), "tailscale") {
		return "", false
	}

	return status.PreferredDNSName(host)
}

func applyPreferredTailscaleHosts(cfg *config.Config, status *tailscale.Status) bool {
	if cfg == nil || status == nil {
		return false
	}

	changed := false
	for i := range cfg.Devices {
		preferred, ok := preferredTailscaleHost(cfg.Devices[i].Host, cfg.Devices[i].ConnectionType, status)
		if !ok || preferred == cfg.Devices[i].Host {
			continue
		}
		cfg.Devices[i].Host = preferred
		changed = true
	}

	return changed
}

func applyCurrentTailscaleHostAliases(cfg *config.Config) bool {
	return applyPreferredTailscaleHosts(cfg, currentTailscaleStatus())
}

func preferTailscaleHostInRequest(req deviceUpsertRequest) deviceUpsertRequest {
	preferred, ok := preferredTailscaleHost(req.Host, req.ConnectionType, currentTailscaleStatus())
	if ok {
		req.Host = preferred
	}
	return req
}
