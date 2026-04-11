package cli

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
	"github.com/tfitz/takumi/src/config"
	"github.com/tfitz/takumi/src/ui"
	"github.com/tfitz/takumi/src/workspace"
)

func init() {
	checkoutCmd.Flags().String("branch", "", "Branch to checkout")
	checkoutCmd.Flags().String("path", "", "Local path for the cloned repo")
	rootCmd.AddCommand(checkoutCmd)
}

var checkoutCmd = &cobra.Command{
	Use:   "checkout <clone-url>",
	Short: "Clone a repo and add it to the workspace",
	Long: `Clone a git repository into the workspace and register it as a tracked source.
Detects takumi-pkg.yaml in the cloned repo and sets up runtime environments.
If the repo contains sub-packages, all are registered.`,
	Args: cobra.ExactArgs(1),
	RunE: runCheckout,
}

func runCheckout(cmd *cobra.Command, args []string) error {
	ws := requireWorkspace()

	cloneURL := args[0]
	branch, _ := cmd.Flags().GetString("branch")
	pathFlag, _ := cmd.Flags().GetString("path")

	// Determine clone target directory
	var cloneDir string
	if pathFlag != "" {
		cloneDir = pathFlag
		if !filepath.IsAbs(cloneDir) {
			cloneDir = filepath.Join(ws.Root, cloneDir)
		}
	} else {
		repoName := repoNameFromURL(cloneURL)
		cloneDir = filepath.Join(ws.Root, repoName)
	}

	// Check if directory already exists
	if _, err := os.Stat(cloneDir); err == nil {
		return fmt.Errorf("directory already exists: %s", cloneDir)
	}

	// Git clone
	gitArgs := []string{"clone"}
	if branch != "" {
		gitArgs = append(gitArgs, "--branch", branch)
	}
	gitArgs = append(gitArgs, cloneURL, cloneDir)

	fmt.Println()
	fmt.Println(ui.SectionHeader.Render("Cloning repository"))
	fmt.Println(ui.StepInfo(ui.Muted.Render(cloneURL)))
	fmt.Println()

	gitCmd := exec.Command("git", gitArgs...)
	gitCmd.Stdout = os.Stdout
	gitCmd.Stderr = os.Stderr
	if err := gitCmd.Run(); err != nil {
		return fmt.Errorf("git clone failed: %w", err)
	}

	// Detect branch if not specified
	if branch == "" {
		branch = detectGitBranch(cloneDir)
	}

	// Scan for packages in cloned repo
	pkgs, _, err := workspace.ScanPackages(cloneDir, nil)
	if err != nil {
		return fmt.Errorf("scanning cloned repo: %w", err)
	}

	fmt.Println()
	if len(pkgs) == 0 {
		fmt.Println(ui.Warn("No " + ui.FilePath("takumi-pkg.yaml") + " found in cloned repo"))
		fmt.Println(ui.StepInfo("Run " + ui.Command("takumi init") + " inside the repo to create one"))
	} else {
		fmt.Println(ui.StepDone("Found " + ui.Bold.Render(ui.FormatCount(len(pkgs), "package", "packages")) + ":"))
		for name := range pkgs {
			deps := len(pkgs[name].Config.Dependencies)
			line := "  " + ui.Bullet(ui.Bold.Render(name))
			if deps > 0 {
				line += ui.Muted.Render(fmt.Sprintf(" (%d deps)", deps))
			}
			fmt.Println(line)
		}
	}

	// Register in workspace sources
	relPath, _ := filepath.Rel(ws.Root, cloneDir)
	sourceName := repoNameFromURL(cloneURL)

	if ws.Config.Workspace.Sources == nil {
		ws.Config.Workspace.Sources = make(map[string]config.Source)
	}
	ws.Config.Workspace.Sources[sourceName] = config.Source{
		URL:    cloneURL,
		Branch: branch,
		Path:   "./" + relPath,
	}

	// Save updated workspace config
	cfgPath := filepath.Join(ws.Root, workspace.WorkspaceFile)
	if err := config.SaveWorkspaceConfig(cfgPath, ws.Config); err != nil {
		return err
	}

	fmt.Println()
	fmt.Println(ui.StepDone("Registered " + ui.Bold.Render(sourceName) + " in " + ui.FilePath(workspace.WorkspaceFile)))
	fmt.Println()
	return nil
}

// repoNameFromURL extracts a short name from a git clone URL.
// https://github.com/org/my-repo.git → my-repo
// git@github.com:org/my-repo.git → my-repo
func repoNameFromURL(url string) string {
	// Strip trailing .git
	name := strings.TrimSuffix(url, ".git")
	// Take last path segment
	if i := strings.LastIndex(name, "/"); i >= 0 {
		name = name[i+1:]
	}
	// Handle SSH-style URLs (git@host:org/repo)
	if i := strings.LastIndex(name, ":"); i >= 0 {
		name = name[i+1:]
	}
	return name
}

// detectGitBranch returns the current branch name of a git repo.
func detectGitBranch(dir string) string {
	cmd := exec.Command("git", "-C", dir, "rev-parse", "--abbrev-ref", "HEAD")
	out, err := cmd.Output()
	if err != nil {
		return "main"
	}
	return strings.TrimSpace(string(out))
}
