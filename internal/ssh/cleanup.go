package ssh

import (
	"fmt"
	"strings"
)

func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", "'\"'\"'") + "'"
}

// CleanupOptions controls optional remote cleanup behavior.
type CleanupOptions struct {
	RemoveDockerImages bool
}

func runCleanupCmd(client remoteRunner, cmd, action string) error {
	out, err := client.Exec(cmd)
	if err != nil {
		return fmt.Errorf("%s: %w — stderr: %s", action, err, strings.TrimSpace(out))
	}
	return nil
}

func cleanupDevice(client remoteRunner, opts CleanupOptions) error {
	hasSystem := hasSystemService(client)
	root := remoteIsRoot(client)

	canTouchSystem := root
	useSudo := false
	if hasSystem && !root {
		if err := ensureSudoNonInteractive(client); err != nil {
			return err
		}
		canTouchSystem = true
		useSudo = true
	}

	if err := runCleanupCmd(client,
		"systemctl --user disable --now yokai-agent 2>/dev/null || true; "+
			"systemctl --user daemon-reload 2>/dev/null || true; "+
			"rm -f ~/.config/systemd/user/yokai-agent.service 2>/dev/null || true; "+
			"rm -rf ~/.config/yokai 2>/dev/null || true; "+
			"rm -f ~/.local/bin/yokai 2>/dev/null || true; "+
			"pkill -f 'yokai agent' 2>/dev/null || true",
		"removing user-level yokai agent",
	); err != nil {
		return err
	}

	if canTouchSystem {
		binaryPath := systemAgentBinaryPath(client)
		if binaryPath == "" {
			binaryPath = "/usr/local/bin/yokai"
		}

		prefix := sudoPrefix(useSudo)
		systemCmd := fmt.Sprintf(
			"%ssystemctl disable --now yokai-agent 2>/dev/null || true; "+
				"%ssystemctl daemon-reload 2>/dev/null || true; "+
				"%srm -f /etc/systemd/system/yokai-agent.service /lib/systemd/system/yokai-agent.service 2>/dev/null || true; "+
				"%srm -rf /etc/yokai 2>/dev/null || true; "+
				"%srm -f %s /usr/local/bin/yokai 2>/dev/null || true",
			prefix, prefix, prefix, prefix, prefix, shellQuote(binaryPath),
		)
		if err := runCleanupCmd(client, systemCmd, "removing system-level yokai agent"); err != nil {
			return err
		}
	}

	var dockerCleanupCmd string
	if opts.RemoveDockerImages {
		dockerCleanupCmd = `sh -lc 'command -v docker >/dev/null 2>&1 || exit 0; tmpf=$(mktemp); docker ps -a --filter "name=yokai-" --format "{{.Image}}" 2>/dev/null | sort -u > "$tmpf"; docker ps -aq --filter "name=yokai-" | xargs -r docker rm -f >/dev/null 2>&1 || true; if [ -s "$tmpf" ]; then xargs -r -n1 docker rmi < "$tmpf" >/dev/null 2>&1 || true; fi; rm -f "$tmpf"'`
	} else {
		dockerCleanupCmd = `sh -lc 'command -v docker >/dev/null 2>&1 || exit 0; docker ps -aq --filter "name=yokai-" | xargs -r docker rm -f >/dev/null 2>&1 || true'`
	}
	if err := runCleanupCmd(client, dockerCleanupCmd, "removing yokai containers"); err != nil {
		return err
	}

	if err := runCleanupCmd(client,
		"rm -rf /tmp/yokai-monitoring 2>/dev/null || true; rm -f /tmp/yokai-agent.log 2>/dev/null || true",
		"removing yokai temporary files",
	); err != nil {
		return err
	}

	return nil
}

// CleanupDevice removes Yokai artifacts from a remote device.
// It supports both user-level and system-level installs.
func CleanupDevice(client *Client, opts CleanupOptions) error {
	return cleanupDevice(client, opts)
}
