package main

import (
	"fmt"
	"os"

	"github.com/spencerbull/yokai/internal/agent"
	"github.com/spencerbull/yokai/internal/daemon"
	"github.com/spencerbull/yokai/internal/tui"
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
		case "--help", "-h":
			printUsage()
			return
		}
	}

	// Default: launch TUI
	if err := tui.Run(version); err != nil {
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
  yokai              Launch the TUI (default)
  yokai agent [port] Run the agent on a target device (default port: 7474)
  yokai daemon       Run the local background daemon
  yokai upgrade      Update to the latest version
  yokai version      Print version
`, version)
}
