package cli

import (
	"fmt"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"

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
	changedFiles, err := gitChangedFiles(ws.Root, since)
	if err != nil {
		return fmt.Errorf("git diff: %w", err)
	}

	if len(changedFiles) == 0 {
		fmt.Println(ui.Check("No changes detected since " + ui.Bold.Render(since)))
		return nil
	}

	// Map changed files to packages
	directlyAffected := mapFilesToPackages(ws, changedFiles)

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

// gitChangedFiles returns files changed since the given ref.
func gitChangedFiles(wsRoot, since string) ([]string, error) {
	// Try diff against ref first (for branch comparisons)
	cmd := exec.Command("git", "-C", wsRoot, "diff", "--name-only", since)
	out, err := cmd.Output()
	if err != nil {
		// Fall back to diff of working tree
		cmd = exec.Command("git", "-C", wsRoot, "diff", "--name-only")
		out, err = cmd.Output()
		if err != nil {
			return nil, err
		}
	}

	lines := strings.Split(strings.TrimSpace(string(out)), "\n")
	var files []string
	for _, line := range lines {
		if line != "" {
			files = append(files, line)
		}
	}
	return files, nil
}

// mapFilesToPackages determines which packages contain the changed files.
func mapFilesToPackages(ws *workspace.Info, files []string) map[string]bool {
	affected := make(map[string]bool)

	for _, file := range files {
		absFile := filepath.Join(ws.Root, file)

		for name, pkg := range ws.Packages {
			// Check if the changed file is under the package's directory
			rel, err := filepath.Rel(pkg.Dir, absFile)
			if err != nil {
				continue
			}
			// If rel doesn't start with "..", the file is inside this package
			if !strings.HasPrefix(rel, "..") {
				affected[name] = true
			}
		}
	}

	return affected
}
