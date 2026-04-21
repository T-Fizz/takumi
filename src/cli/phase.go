package cli

import (
	"sort"

	"github.com/spf13/cobra"
)

// builtinPhases are phases that have their own static command definitions
// (e.g., build has a "clean" subcommand). These are skipped during dynamic
// registration to avoid conflicts.
var builtinPhases = map[string]bool{
	"build": true,
	"test":  true,
}

// registerPhaseCommands discovers all unique phase names across workspace
// packages and registers a top-level command for each one that isn't already
// a built-in command. This allows `takumi deploy`, `takumi lint`, etc. to
// work without needing `takumi run <phase>`.
func registerPhaseCommands() {
	ws, err := loadWorkspace()
	if err != nil {
		return // not in a workspace — nothing to register
	}

	// Collect all existing command names so we don't shadow them.
	existing := make(map[string]bool)
	for _, cmd := range rootCmd.Commands() {
		existing[cmd.Name()] = true
		for _, alias := range cmd.Aliases {
			existing[alias] = true
		}
	}

	// Discover unique phase names.
	phases := make(map[string]bool)
	for _, pkg := range ws.Packages {
		for phaseName := range pkg.Config.Phases {
			phases[phaseName] = true
		}
	}

	// Sort for deterministic registration order.
	var names []string
	for name := range phases {
		if existing[name] || builtinPhases[name] {
			continue
		}
		names = append(names, name)
	}
	sort.Strings(names)

	for _, name := range names {
		registerPhaseCmd(name)
	}
}

// registerPhaseCmd creates and registers a single dynamic phase command.
func registerPhaseCmd(phase string) {
	cmd := &cobra.Command{
		Use:   phase + " [packages...]",
		Short: "Run " + phase + " phase for packages",
		Long: "Run the " + phase + " phase for one or more packages.\n" +
			"If no packages are specified, runs for all packages that define it.",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runPhaseCommand(cmd, args, phase)
		},
	}
	cmd.Flags().Bool("affected", false, "Only run for packages affected by changes")
	cmd.Flags().Bool("no-cache", false, "Skip cache and force execution")
	cmd.Flags().Bool("dry-run", false, "Show execution plan without running anything")
	rootCmd.AddCommand(cmd)
}
