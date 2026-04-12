package cli

import (
	"github.com/mark3labs/mcp-go/server"
	"github.com/spf13/cobra"
	takumimcp "github.com/tfitz/takumi/src/mcp"
)

func init() {
	mcpCmd.AddCommand(mcpServeCmd)
	rootCmd.AddCommand(mcpCmd)
}

var mcpCmd = &cobra.Command{
	Use:   "mcp",
	Short: "Model Context Protocol integration",
	Long:  `Commands for MCP server integration with AI agents.`,
}

var mcpServeCmd = &cobra.Command{
	Use:   "serve",
	Short: "Start MCP server over stdio",
	Long: `Start a Model Context Protocol server that exposes takumi workspace
tools over stdio. Configure in your AI agent's MCP settings to give
the agent direct access to build, test, diagnose, and other operations.`,
	RunE: runMCPServe,
}

func runMCPServe(cmd *cobra.Command, args []string) error {
	s := takumimcp.NewServer()
	return server.ServeStdio(s, server.WithWorkerPoolSize(1))
}
