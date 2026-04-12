package cli

import (
	"fmt"
	"path/filepath"
	"sort"

	"github.com/spf13/cobra"
	"github.com/tfitz/takumi/src/config"
	"github.com/tfitz/takumi/src/ui"
	"github.com/tfitz/takumi/src/workspace"
)

func init() {
	versionSetCmd.AddCommand(versionSetCheckCmd)
	rootCmd.AddCommand(versionSetCmd)
}

var versionSetCmd = &cobra.Command{
	Use:     "version-set",
	Aliases: []string{"vs"},
	Short:   "Manage version sets",
}

var versionSetCheckCmd = &cobra.Command{
	Use:   "check",
	Short: "Show declared versions across the workspace",
	Long:  `Parse the version-set file and report any version conflicts across packages.`,
	RunE:  runVersionSetCheck,
}

func runVersionSetCheck(cmd *cobra.Command, args []string) error {
	ws := requireWorkspace()

	vsFile := ws.Config.Workspace.VersionSet.File
	if vsFile == "" {
		vsFile = workspace.VersionsFile
	}

	vsPath := filepath.Join(ws.Root, vsFile)
	vsCfg, err := config.LoadVersionSetConfig(vsPath)
	if err != nil {
		fmt.Println(ui.Warn("No version-set file found (" + ui.FilePath(vsFile) + ")"))
		fmt.Println(ui.StepInfo("Create " + ui.FilePath(vsFile) + " to pin dependency versions"))
		return nil
	}

	fmt.Println()
	fmt.Println(ui.SectionHeader.Render("Version Set: " + vsCfg.VersionSet.Name))
	if vsCfg.VersionSet.Strategy != "" {
		fmt.Println(ui.Muted.Render("  Strategy: " + vsCfg.VersionSet.Strategy))
	}
	fmt.Println()

	// List all pinned versions
	var names []string
	for name := range vsCfg.VersionSet.Packages {
		names = append(names, name)
	}
	sort.Strings(names)

	for _, name := range names {
		version := vsCfg.VersionSet.Packages[name]
		fmt.Printf("  %s %s\n", ui.Bold.Render(name), ui.Muted.Render(version))
	}

	fmt.Println()
	fmt.Println(ui.Divider())
	fmt.Println(ui.FormatCount(len(names), "pinned dependency", "pinned dependencies"))
	fmt.Println()

	return nil
}
