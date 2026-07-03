package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/spf13/cobra"

	"github.com/ladderlogix/typstpad/internal/mcp"
)

func mcpCmd() *cobra.Command {
	var url, token string
	cmd := &cobra.Command{
		Use:   "mcp",
		Short: "Run an MCP stdio server proxying to a TypstPad instance (for AI agents)",
		Long: `Run a Model Context Protocol server over stdio. Point your AI agent at:
  typstpad mcp --url http://server:8080 --token tfp_...
Falls back to the credentials saved by "typstpad login".`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if url == "" || token == "" {
				cfg, err := loadConfig()
				if err != nil {
					return fmt.Errorf("--url/--token not given and %w", err)
				}
				if url == "" {
					url = cfg.URL
				}
				if token == "" {
					token = cfg.Token
				}
			}
			ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
			defer stop()
			server := mcp.NewServer(mcp.NewRESTClient(url, token))
			return server.Run(ctx, &sdk.StdioTransport{})
		},
	}
	cmd.Flags().StringVar(&url, "url", "", "TypstPad server URL")
	cmd.Flags().StringVar(&token, "token", "", "API token (tfp_...)")
	return cmd
}
