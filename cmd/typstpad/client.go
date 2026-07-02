package main

import (
	"github.com/spf13/cobra"
)

// addClientCommands registers the CLI/MCP client subcommands (login, pull,
// push, compile, watch, mcp). Implemented in cli.go / mcp.go.
func addClientCommands(root *cobra.Command) {
	root.AddCommand(cliCommands()...)
}
