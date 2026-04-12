package cli

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/spf13/cobra"
	"github.com/tfitz/takumi/src/ui"
)

func init() {
	rootCmd.AddCommand(syncCmd)
}

var syncCmd = &cobra.Command{
	Use:   "sync",
	Short: "Pull or clone all tracked sources",
	Long: `Synchronize the workspace by pulling updates for existing tracked sources
and cloning any that are missing.`,
	RunE: runSync,
}

func runSync(cmd *cobra.Command, args []string) error {
	ws := requireWorkspace()

	if len(ws.Config.Workspace.Sources) == 0 {
		fmt.Println("No tracked sources in workspace.")
		return nil
	}

	fmt.Println()
	fmt.Println(ui.SectionHeader.Render("Syncing " + ui.FormatCount(len(ws.Config.Workspace.Sources), "source", "sources")))
	fmt.Println()

	var cloned, pulled, failed int

	for name, source := range ws.Config.Workspace.Sources {
		dir := source.Path
		if !filepath.IsAbs(dir) {
			dir = filepath.Join(ws.Root, dir)
		}

		if _, err := os.Stat(dir); os.IsNotExist(err) {
			// Clone
			fmt.Println(ui.StepInfo("Cloning " + ui.Bold.Render(name) + " " + ui.Muted.Render("("+source.URL+")")))

			gitArgs := []string{"clone"}
			if source.Branch != "" {
				gitArgs = append(gitArgs, "--branch", source.Branch)
			}
			gitArgs = append(gitArgs, source.URL, dir)

			gitCmd := exec.Command("git", gitArgs...)
			gitCmd.Stdout = os.Stdout
			gitCmd.Stderr = os.Stderr
			if err := gitCmd.Run(); err != nil {
				fmt.Fprintln(os.Stderr, ui.Cross("Failed to clone "+name+": "+err.Error()))
				failed++
				continue
			}
			fmt.Println(ui.StepDone("Cloned " + ui.Bold.Render(name)))
			cloned++
		} else {
			// Pull
			branchInfo := ""
			if source.Branch != "" {
				branchInfo = " " + ui.Muted.Render("("+source.Branch+")")
			}
			fmt.Println(ui.StepInfo("Pulling " + ui.Bold.Render(name) + branchInfo))

			gitArgs := []string{"-C", dir, "pull"}
			if source.Branch != "" {
				gitArgs = append(gitArgs, "origin", source.Branch)
			}

			gitCmd := exec.Command("git", gitArgs...)
			gitCmd.Stdout = os.Stdout
			gitCmd.Stderr = os.Stderr
			if err := gitCmd.Run(); err != nil {
				fmt.Fprintln(os.Stderr, ui.Cross("Failed to pull "+name+": "+err.Error()))
				failed++
				continue
			}
			fmt.Println(ui.StepDone("Pulled " + ui.Bold.Render(name)))
			pulled++
		}
	}

	// Summary
	fmt.Println()
	fmt.Println(ui.Divider())
	parts := []string{}
	if cloned > 0 {
		parts = append(parts, ui.Success.Render(ui.FormatCount(cloned, "cloned", "cloned")))
	}
	if pulled > 0 {
		parts = append(parts, ui.Success.Render(ui.FormatCount(pulled, "pulled", "pulled")))
	}
	if failed > 0 {
		parts = append(parts, ui.Error.Render(ui.FormatCount(failed, "failed", "failed")))
	}
	fmt.Println("Sync complete: " + joinParts(parts))
	fmt.Println()

	return nil
}

func joinParts(parts []string) string {
	if len(parts) == 0 {
		return "nothing to do"
	}
	result := parts[0]
	for i := 1; i < len(parts); i++ {
		result += ", " + parts[i]
	}
	return result
}
