package cmd

import (
	"github.com/spf13/cobra"
	"github.com/steamfoundry/agent-memory/internal/mcp"
)

var mcpCmd = &cobra.Command{
	Use:   "mcp",
	Short: "Start MCP server (stdio)",
	Long:  "Start a Model Context Protocol server over stdin/stdout for AI agent integration.",
	RunE:  runMCP,
}

func init() {
	rootCmd.AddCommand(mcpCmd)
}

func runMCP(cmd *cobra.Command, args []string) error {
	s, err := openStore()
	if err != nil {
		return err
	}
	defer s.Close()

	server := mcp.NewServer(s, workspace)
	return server.Run()
}
