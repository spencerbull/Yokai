// Command yokai-mcp exposes the yokai daemon's /agent read endpoints to any
// MCP-speaking agent (pi, opencode, hermes, openclaw, Claude Desktop, ...).
//
// Run it as a stdio MCP server, e.g. in opencode:
//
//	"mcp": { "yokai": { "type": "stdio", "command": "yokai-mcp" } }
//
// It requires the yokai daemon to be running (the same one the TUI talks to).
package main

import (
	"fmt"
	"os"

	"github.com/spencerbull/yokai/internal/mcp"
)

func main() {
	srv, err := mcp.New()
	if err != nil {
		fmt.Fprintln(os.Stderr, "yokai-mcp:", err)
		os.Exit(1)
	}
	if err := srv.Serve(); err != nil {
		fmt.Fprintln(os.Stderr, "yokai-mcp:", err)
		os.Exit(1)
	}
}
