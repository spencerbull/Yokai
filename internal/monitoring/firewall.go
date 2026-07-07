package monitoring

import (
	"fmt"
	"strings"
)

// ConfigureHostScrapeFirewall allows Prometheus on the monitoring bridge to
// scrape host-bound exporters when UFW is active. Hosts without UFW no-op.
func ConfigureHostScrapeFirewall(client remoteClient, agentPort int) error {
	if agentPort <= 0 {
		agentPort = 7474
	}

	cmd := fmt.Sprintf(`sudo_cmd="sudo -n"
if [ "$(id -u 2>/dev/null)" = "0" ]; then sudo_cmd=""; fi
if command -v ufw >/dev/null 2>&1 && $sudo_cmd ufw status 2>/dev/null | grep -q '^Status: active'; then
  subnet=$(docker network inspect -f '{{(index .IPAM.Config 0).Subnet}}' monitoring_yokai-monitoring 2>/dev/null || docker network inspect -f '{{(index .IPAM.Config 0).Subnet}}' yokai-monitoring 2>/dev/null || true)
  if [ -n "$subnet" ]; then
    $sudo_cmd ufw allow in from "$subnet" to any port %d proto tcp >/dev/null
    $sudo_cmd ufw allow in from "$subnet" to any port 9100 proto tcp >/dev/null
  fi
fi`, agentPort)

	if out, err := client.Exec(cmd); err != nil {
		return fmt.Errorf("configuring monitoring firewall access: %w — stderr: %s", err, strings.TrimSpace(out))
	}
	return nil
}
