package cli

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"
	"github.com/tfitz/takumi/src/config"
	"github.com/tfitz/takumi/src/ui"
	"github.com/tfitz/takumi/src/workspace"
)

func init() {
	removeCmd.Flags().Bool("delete", false, "Also delete the cloned directory from disk")
	rootCmd.AddCommand(removeCmd)
}

var removeCmd = &cobra.Command{
	Use:   "remove <package>",
	Short: "Remove a package from the workspace",
	Long: `Remove a tracked package from the workspace. Cleans up runtime environments.
Use --delete to also remove the directory from disk.`,
	Args: cobra.ExactArgs(1),
	RunE: runRemove,
}

func runRemove(cmd *cobra.Command, args []string) error {
	ws := requireWorkspace()

	pkgName := args[0]
	deleteDisk, _ := cmd.Flags().GetBool("delete")

	// Check if the package is a tracked source
	source, tracked := ws.Config.Workspace.Sources[pkgName]
	if !tracked {
		return fmt.Errorf("'%s' is not a tracked source in %s", pkgName, workspace.WorkspaceFile)
	}

	fmt.Println()
	fmt.Println(ui.SectionHeader.Render("Removing " + pkgName))
	fmt.Println()

	// Remove from sources
	delete(ws.Config.Workspace.Sources, pkgName)

	// Clean up runtime environment
	envDir := filepath.Join(ws.Root, workspace.MarkerDir, "envs", pkgName)
	if _, err := os.Stat(envDir); err == nil {
		if err := os.RemoveAll(envDir); err != nil {
			fmt.Fprintln(os.Stderr, ui.Warn("Could not clean up env dir: "+err.Error()))
		} else {
			fmt.Println(ui.StepDone("Cleaned up " + ui.FilePath(workspace.MarkerDir+"/envs/"+pkgName)))
		}
	}

	// Optionally delete from disk
	if deleteDisk {
		diskPath := source.Path
		if !filepath.IsAbs(diskPath) {
			diskPath = filepath.Join(ws.Root, diskPath)
		}
		if _, err := os.Stat(diskPath); err == nil {
			if err := os.RemoveAll(diskPath); err != nil {
				return fmt.Errorf("removing directory %s: %w", diskPath, err)
			}
			fmt.Println(ui.StepDone("Deleted " + ui.FilePath(source.Path) + " from disk"))
		}
	}

	// Save updated workspace config
	cfgPath := filepath.Join(ws.Root, workspace.WorkspaceFile)
	if err := config.SaveWorkspaceConfig(cfgPath, ws.Config); err != nil {
		return err
	}

	fmt.Println(ui.StepDone("Removed " + ui.Bold.Render(pkgName) + " from workspace"))
	fmt.Println()
	return nil
}
