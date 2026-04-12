// Package cli defines the Cobra command tree for Takumi.
package cli

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"github.com/tfitz/takumi/src/ui"
	"github.com/tfitz/takumi/src/workspace"
)

// version is set at build time via ldflags.
var version = "dev"

var rootCmd = &cobra.Command{
	Use:     "takumi",
	Version: version,
	Short:   "Takumi (匠) — AI-aware, language-agnostic package builder",
	Long: `Takumi is an AI-aware, language-agnostic package builder that works with any
project in any git repo. It runs user-defined shell commands, manages optional
per-package runtime environments, builds a dependency DAG for parallel execution,
and ships with an AI skills system that teaches AI assistants how to operate
the workspace.`,
}

// osExit is the exit function used by Execute and requireWorkspace.
// Replaced in tests to avoid killing the test process.
var osExit = os.Exit

// Execute runs the root command.
func Execute() {
	if err := rootCmd.Execute(); err != nil {
		osExit(1)
	}
}

// loadWorkspace detects and loads the workspace from cwd. Returns an error
// if not in a workspace or if the config is invalid.
func loadWorkspace() (*workspace.Info, error) {
	cwd, err := os.Getwd()
	if err != nil {
		return nil, fmt.Errorf("cannot determine working directory: %w", err)
	}

	ws, err := workspace.Load(cwd)
	if err != nil {
		return nil, fmt.Errorf("loading workspace: %w", err)
	}
	if ws == nil {
		return nil, fmt.Errorf("not in a Takumi workspace (no .takumi/ directory found)")
	}
	return ws, nil
}

// requireWorkspace loads the workspace from cwd and exits with a message if not found.
func requireWorkspace() *workspace.Info {
	ws, err := loadWorkspace()
	if err != nil {
		fmt.Fprintln(os.Stderr, ui.Cross(err.Error()))
		fmt.Fprintln(os.Stderr, ui.StepInfo("Run "+ui.Command("takumi init")+" to create a workspace"))
		osExit(1)
	}
	return ws
}
