package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/mark3labs/mcp-go/server"
	"github.com/spf13/cobra"
	takumimcp "github.com/tfitz/takumi/src/mcp"
	"github.com/tfitz/takumi/src/ui"
)

func init() {
	mcpCmd.AddCommand(mcpServeCmd, mcpInstallCmd)
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

var mcpInstallCmd = &cobra.Command{
	Use:   "install",
	Short: "Register Takumi as a global MCP server for AI agents",
	Long: `Register the Takumi MCP server in your AI agent's global configuration
so that Takumi tools (status, build, test, diagnose, etc.) are available
in every project — even before running takumi init.

Supports: Claude Code (claude_desktop_config.json)

The agent will automatically discover Takumi's tools. If the current
directory is not a Takumi workspace, takumi_status will guide the agent
to run takumi init.`,
	RunE: runMCPInstall,
}

func runMCPInstall(cmd *cobra.Command, args []string) error {
	// Find the takumi binary path
	binPath, err := exec.LookPath("takumi")
	if err != nil {
		// Fall back to the currently running binary
		binPath, err = os.Executable()
		if err != nil {
			return fmt.Errorf("cannot determine takumi binary path: %w", err)
		}
	}
	binPath, _ = filepath.Abs(binPath)

	// Claude Code global config
	home, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("cannot determine home directory: %w", err)
	}

	configPath := filepath.Join(home, ".claude", "claude_desktop_config.json")
	if err := installClaudeMCP(configPath, binPath); err != nil {
		return err
	}

	fmt.Println(ui.StepDone("Registered Takumi MCP server globally"))
	fmt.Println(ui.StepInfo("Takumi tools will be available in all Claude Code sessions"))
	fmt.Println(ui.StepInfo("Agents can call takumi_status to discover your workspace"))
	return nil
}

func installClaudeMCP(configPath, binPath string) error {
	// Read existing config or start fresh
	var config map[string]any

	if data, err := os.ReadFile(configPath); err == nil {
		if err := json.Unmarshal(data, &config); err != nil {
			return fmt.Errorf("invalid JSON in %s: %w", configPath, err)
		}
	} else {
		config = make(map[string]any)
	}

	// Ensure mcpServers key exists
	servers, ok := config["mcpServers"].(map[string]any)
	if !ok {
		servers = make(map[string]any)
	}

	// Add or update takumi entry
	servers["takumi"] = map[string]any{
		"command": binPath,
		"args":    []string{"mcp", "serve"},
	}
	config["mcpServers"] = servers

	// Write back
	if err := os.MkdirAll(filepath.Dir(configPath), 0755); err != nil {
		return fmt.Errorf("creating config directory: %w", err)
	}

	data, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		return fmt.Errorf("marshaling config: %w", err)
	}

	if err := os.WriteFile(configPath, data, 0644); err != nil {
		return fmt.Errorf("writing %s: %w", configPath, err)
	}

	return nil
}
