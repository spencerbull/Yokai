package main

import (
	"fmt"
	"os"

	"github.com/spencerbull/yokai/internal/agent"
	"github.com/spencerbull/yokai/internal/cli"
	"github.com/spencerbull/yokai/internal/daemon"
	"github.com/spencerbull/yokai/internal/opentui"
	"github.com/spencerbull/yokai/internal/upgrade"
)

var (
	version   = "dev"
	buildTime = "unknown"
)

func main() {
	if len(os.Args) > 1 {
		switch os.Args[1] {
		case "agent":
			runAgent()
			return
		case "daemon":
			runDaemon()
			return
		case "version":
			fmt.Printf("yokai %s (built %s)\n", version, buildTime)
			return
		case "upgrade":
			if err := upgrade.Run(version); err != nil {
				fmt.Fprintf(os.Stderr, "Upgrade error: %v\n", err)
				os.Exit(1)
			}
			return

		// CLI commands (non-TUI)
		case "devices":
			cli.RunDevices(os.Args[2:])
			return
		case "services":
			cli.RunServices(os.Args[2:])
			return
		case "status":
			cli.RunStatus(os.Args[2:])
			return
		case "metrics":
			cli.RunMetrics(os.Args[2:])
			return
		case "config":
			cli.RunConfig(os.Args[2:])
			return

		case "--help", "-h":
			printUsage()
			return
		}
	}

	// Default: launch OpenTUI
	if err := opentui.Run(version); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

func runAgent() {
	port := "7474"
	if len(os.Args) > 2 {
		port = os.Args[2]
	}
	if err := agent.Run(port, version); err != nil {
		fmt.Fprintf(os.Stderr, "Agent error: %v\n", err)
		os.Exit(1)
	}
}

func runDaemon() {
	if err := daemon.Run(version); err != nil {
		fmt.Fprintf(os.Stderr, "Daemon error: %v\n", err)
		os.Exit(1)
	}
}

func printUsage() {
	fmt.Printf(`yokai %s — GPU Fleet Manager

Usage:
  yokai                    Launch OpenTUI (default; auto-starts daemon)
  yokai agent [port]       Run the agent on a target device (default port: 7474)
  yokai daemon             Run the local background daemon
  yokai upgrade            Update to the latest version
  yokai version            Print version

Device Management:
  yokai devices list                         List all configured devices
  yokai devices add --host <host> [flags]    Add a device
  yokai devices remove <device-id>           Remove a device
  yokai devices test <device-id>             Test SSH + agent connectivity
  yokai devices bootstrap <device-id>        Install/upgrade agent on device

Service Management:
  yokai services list [--device <id>]        List containers across fleet
  yokai services deploy --device <id> [flags] Deploy a service
  yokai services stop <device-id> <cid>      Stop a container
  yokai services restart <device-id> <cid>   Restart a container
  yokai services logs [--follow] <did> <cid> Stream container logs

Fleet Status:
  yokai status                               Fleet overview (JSON)
  yokai metrics [--device <id>]              Detailed device metrics (JSON)

Configuration:
  yokai config show                          Dump config (tokens redacted)
  yokai config set <key> <value>             Set a config value
  yokai config path                          Print config file path

All CLI commands output JSON to stdout. Errors go to stderr as JSON.
`, version)
}
