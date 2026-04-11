package cli

import (
	"github.com/spf13/cobra"
)

func init() {
	testCmd.Flags().Bool("affected", false, "Only test packages affected by changes")
	testCmd.Flags().Bool("no-cache", false, "Skip cache and force execution")
	testCmd.Flags().Bool("dry-run", false, "Show execution plan without running anything")
	rootCmd.AddCommand(testCmd)
}

var testCmd = &cobra.Command{
	Use:   "test [packages...]",
	Short: "Run test phase for packages",
	Long: `Run tests for packages in dependency order. If no packages are specified,
tests all packages in the workspace. Use --affected to test only changed
packages and their downstream dependents.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		return runPhaseCommand(cmd, args, "test")
	},
}
