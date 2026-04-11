package cli

import (
	"github.com/spf13/cobra"
)

func init() {
	runCmd.Flags().Bool("affected", false, "Only run for packages affected by changes")
	runCmd.Flags().Bool("no-cache", false, "Skip cache and force execution")
	runCmd.Flags().Bool("dry-run", false, "Show execution plan without running anything")
	rootCmd.AddCommand(runCmd)
}

var runCmd = &cobra.Command{
	Use:   "run <phase> [packages...]",
	Short: "Run any named phase for packages",
	Long: `Run an arbitrary phase (e.g., lint, deploy) for one or more packages.
If no packages are specified, runs the phase for all packages that define it.`,
	Args: cobra.MinimumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		phase := args[0]
		packages := args[1:]
		return runPhaseCommand(cmd, packages, phase)
	},
}
