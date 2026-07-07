package monitoring

import (
	"strings"
	"testing"
)

type firewallFakeClient struct {
	cmds []string
}

func (f *firewallFakeClient) Exec(cmd string) (string, error) {
	f.cmds = append(f.cmds, cmd)
	return "", nil
}

func (f *firewallFakeClient) Upload(_, _ string) error { return nil }

func TestConfigureHostScrapeFirewallAllowsMonitoringSubnet(t *testing.T) {
	t.Parallel()

	fake := &firewallFakeClient{}
	if err := ConfigureHostScrapeFirewall(fake, 9191); err != nil {
		t.Fatalf("ConfigureHostScrapeFirewall returned error: %v", err)
	}

	if len(fake.cmds) != 1 {
		t.Fatalf("expected one command, got %d", len(fake.cmds))
	}
	cmd := fake.cmds[0]
	checks := []string{
		"sudo_cmd=\"sudo -n\"",
		"$sudo_cmd ufw status",
		"docker network inspect -f '{{(index .IPAM.Config 0).Subnet}}' monitoring_yokai-monitoring",
		"$sudo_cmd ufw allow in from \"$subnet\" to any port 9191 proto tcp",
		"$sudo_cmd ufw allow in from \"$subnet\" to any port 9100 proto tcp",
	}
	for _, check := range checks {
		if !strings.Contains(cmd, check) {
			t.Fatalf("expected command to contain %q, got %s", check, cmd)
		}
	}
}

func TestConfigureHostScrapeFirewallDefaultsAgentPort(t *testing.T) {
	t.Parallel()

	fake := &firewallFakeClient{}
	if err := ConfigureHostScrapeFirewall(fake, 0); err != nil {
		t.Fatalf("ConfigureHostScrapeFirewall returned error: %v", err)
	}
	if !strings.Contains(fake.cmds[0], "to any port 7474 proto tcp") {
		t.Fatalf("expected default agent port rule, got %s", fake.cmds[0])
	}
}
