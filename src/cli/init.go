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
	initCmd.Flags().String("root", "", "Create a project directory and initialize workspace inside it")
	initCmd.Flags().String("agent", "", "AI agent to configure ("+agentNames()+")")
	rootCmd.AddCommand(initCmd)
}

var initCmd = &cobra.Command{
	Use:   "init [name]",
	Short: "Initialize a new package or workspace",
	Long: `Initialize a Takumi package in the current directory, or scaffold a new package
in a subdirectory if a name is provided.

If no .takumi/ workspace marker exists above the target directory, a new workspace
is also created (takumi.yaml + .takumi/ directory).

Use --root <project-name> to create a project directory, initialize a workspace
inside it, and optionally scaffold a package with [name].`,
	Args: cobra.MaximumNArgs(1),
	RunE: runInit,
}

func runInit(cmd *cobra.Command, args []string) error {
	rootFlag, _ := cmd.Flags().GetString("root")

	cwd, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("cannot determine working directory: %w", err)
	}

	// --root: create a project directory and work inside it
	if rootFlag != "" {
		projectDir := filepath.Join(cwd, rootFlag)
		if err := os.MkdirAll(projectDir, 0755); err != nil {
			return fmt.Errorf("creating project directory %s: %w", rootFlag, err)
		}
		cwd = projectDir

		agent, err := resolveAgent(cmd)
		if err != nil {
			return err
		}

		if err := initWorkspace(cwd, rootFlag, agent); err != nil {
			return err
		}

		// If a package name is also given, scaffold it as a subdir
		if len(args) == 1 {
			return initPackageInDir(filepath.Join(cwd, args[0]), args[0], cwd, true)
		}

		// Otherwise scaffold a root package using the project name
		return initPackageInDir(cwd, rootFlag, cwd, false)
	}

	// Standard init (no --root)
	targetDir := cwd
	var pkgName string

	if len(args) == 1 {
		pkgName = args[0]
		targetDir = filepath.Join(cwd, pkgName)
	} else {
		pkgName = filepath.Base(cwd)
	}

	// If creating a named subdir, make it
	if len(args) == 1 {
		if err := os.MkdirAll(targetDir, 0755); err != nil {
			return fmt.Errorf("creating directory %s: %w", targetDir, err)
		}
	}

	// Determine workspace root: walk up from target looking for .takumi/
	wsRoot := workspace.Detect(targetDir)

	if wsRoot == "" {
		// No workspace found — create one at cwd
		wsRoot = cwd

		agent, err := resolveAgent(cmd)
		if err != nil {
			return err
		}

		if err := initWorkspace(wsRoot, filepath.Base(wsRoot), agent); err != nil {
			return err
		}
	}

	return initPackageInDir(targetDir, pkgName, wsRoot, len(args) == 1)
}

// resolveAgent determines the AI agent from the --agent flag or interactive menu.
func resolveAgent(cmd *cobra.Command) (*AgentType, error) {
	agentFlag, _ := cmd.Flags().GetString("agent")

	if agentFlag != "" {
		agent := AgentByName(agentFlag)
		if agent == nil {
			return nil, fmt.Errorf("unknown agent %q (valid: %s)", agentFlag, agentNames())
		}
		return agent, nil
	}

	// Interactive menu
	return promptAgentSelection()
}

// initPackageInDir creates a takumi-pkg.yaml in targetDir.
func initPackageInDir(targetDir, pkgName, wsRoot string, isSubdir bool) error {
	if isSubdir {
		if err := os.MkdirAll(targetDir, 0755); err != nil {
			return fmt.Errorf("creating directory %s: %w", targetDir, err)
		}
	}

	pkgFile := filepath.Join(targetDir, workspace.PackageFile)
	if _, err := os.Stat(pkgFile); err == nil {
		return fmt.Errorf("%s already exists in %s", workspace.PackageFile, targetDir)
	}

	pkgCfg := config.DefaultPackageConfig(pkgName)
	data, err := pkgCfg.Marshal()
	if err != nil {
		return fmt.Errorf("marshaling package config: %w", err)
	}

	if err := os.WriteFile(pkgFile, data, 0644); err != nil {
		return fmt.Errorf("writing %s: %w", workspace.PackageFile, err)
	}

	rel, _ := filepath.Rel(wsRoot, targetDir)
	if rel == "." {
		fmt.Println(ui.StepDone("Initialized package " + ui.Bold.Render(pkgName)))
	} else {
		fmt.Println(ui.StepDone("Initialized package " + ui.Bold.Render(pkgName) + " in " + ui.FilePath(rel+"/")))
	}
	fmt.Println(ui.StepInfo("Created " + ui.FilePath(workspace.PackageFile)))

	return nil
}

// initWorkspace creates the .takumi/ marker directory, takumi.yaml, TAKUMI.md,
// and the agent config file at root.
func initWorkspace(root, name string, agent *AgentType) error {
	fmt.Println()
	fmt.Println(ui.Header())
	fmt.Println(ui.SectionHeader.Render("Initializing workspace: " + name))
	fmt.Println()

	markerDir := filepath.Join(root, workspace.MarkerDir)
	if err := os.MkdirAll(markerDir, 0755); err != nil {
		return fmt.Errorf("creating %s: %w", workspace.MarkerDir, err)
	}

	// Create subdirectories
	for _, sub := range []string{"envs", "logs", "skills", "skills/_builtin"} {
		if err := os.MkdirAll(filepath.Join(markerDir, sub), 0755); err != nil {
			return fmt.Errorf("creating %s/%s: %w", workspace.MarkerDir, sub, err)
		}
	}
	fmt.Println(ui.StepDone("Created " + ui.FilePath(workspace.MarkerDir+"/")))

	// Write takumi.yaml
	wsCfg := config.DefaultWorkspaceConfig(name)
	if agent != nil && agent.Name != "none" {
		wsCfg.Workspace.AI.Agent = agent.Name
	}
	wsFile := filepath.Join(root, workspace.WorkspaceFile)
	if err := config.SaveWorkspaceConfig(wsFile, wsCfg); err != nil {
		return err
	}
	fmt.Println(ui.StepDone("Created " + ui.FilePath(workspace.WorkspaceFile)))

	// Write .takumi/TAKUMI.md
	if err := writeTakumiMD(root, name); err != nil {
		return fmt.Errorf("writing .takumi/TAKUMI.md: %w", err)
	}

	// Set up agent config file
	if agent != nil && agent.Name != "none" {
		if err := setupAgentConfig(root, agent); err != nil {
			return fmt.Errorf("setting up %s config: %w", agent.Label, err)
		}
		fmt.Println(ui.StepDone("Created " + ui.FilePath(agent.FilePath) + " for " + ui.Bold.Render(agent.Label)))
	}

	fmt.Println()
	return nil
}
