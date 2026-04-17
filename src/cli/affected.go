package cli

import (
	"fmt"
	"sort"

	"github.com/spf13/cobra"
	"github.com/tfitz/takumi/src/ui"
	"github.com/tfitz/takumi/src/workspace"
)

func init() {
	affectedCmd.Flags().String("since", "", "Git ref to compare against (e.g., main, HEAD~3)")
	rootCmd.AddCommand(affectedCmd)
}

var affectedCmd = &cobra.Command{
	Use:   "affected",
	Short: "List packages affected by recent changes",
	Long: `Determine which packages have been affected by file changes since a given
git ref. Includes downstream dependents.`,
	RunE: runAffected,
}

func runAffected(cmd *cobra.Command, args []string) error {
	ws := requireWorkspace()
	since, _ := cmd.Flags().GetString("since")

	if since == "" {
		since = "HEAD"
	}

	// Get changed files from git
	changedFiles, err := workspace.ChangedFiles(ws.Root, since)
	if err != nil {
		return fmt.Errorf("git diff: %w", err)
	}

	if len(changedFiles) == 0 {
		fmt.Println(ui.Check("No changes detected since " + ui.Bold.Render(since)))
		return nil
	}

	// Map changed files to packages
	directlyAffected := workspace.MapFilesToPackages(ws, changedFiles)

	if len(directlyAffected) == 0 {
		fmt.Println(ui.Check("Changed files don't belong to any tracked package"))
		return nil
	}

	// Build graph and find transitive dependents
	g := buildGraph(ws)
	allAffected := make(map[string]bool)
	for pkg := range directlyAffected {
		allAffected[pkg] = true
		for _, dep := range g.TransitiveDependents(pkg) {
			allAffected[dep] = true
		}
	}

	// Sort for stable output
	var direct []string
	for pkg := range directlyAffected {
		direct = append(direct, pkg)
	}
	sort.Strings(direct)

	var downstream []string
	for pkg := range allAffected {
		if !directlyAffected[pkg] {
			downstream = append(downstream, pkg)
		}
	}
	sort.Strings(downstream)

	fmt.Println()
	fmt.Println(ui.SectionHeader.Render("Affected Packages"))
	fmt.Println()

	fmt.Println("  " + ui.Bold.Render("Directly changed:"))
	for _, pkg := range direct {
		fmt.Println("    " + ui.Bullet(ui.Bold.Render(pkg)))
	}

	if len(downstream) > 0 {
		fmt.Println()
		fmt.Println("  " + ui.Bold.Render("Downstream dependents:"))
		for _, pkg := range downstream {
			fmt.Println("    " + ui.Bullet(ui.Accent.Render(pkg)))
		}
	}

	fmt.Println()
	fmt.Println(ui.Divider())
	fmt.Printf("%s affected (%s direct, %s downstream)\n",
		ui.FormatCount(len(allAffected), "package", "packages"),
		ui.FormatCount(len(direct), "package", "packages"),
		ui.FormatCount(len(downstream), "package", "packages"),
	)
	fmt.Println()

	return nil
}
