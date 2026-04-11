package cli

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
	"github.com/tfitz/takumi/src/ui"
	"github.com/tfitz/takumi/src/workspace"
)

func init() {
	envCmd.AddCommand(envSetupCmd, envCleanCmd, envListCmd)
	rootCmd.AddCommand(envCmd)
}

var envCmd = &cobra.Command{
	Use:   "env",
	Short: "Manage per-package runtime environments",
}

var envSetupCmd = &cobra.Command{
	Use:   "setup [packages...]",
	Short: "Create runtime environments for packages",
	Long: `Run user-defined runtime.setup commands to create isolated environments.
If no packages are specified, sets up environments for all packages that
define a runtime section.`,
	RunE: runEnvSetup,
}

var envCleanCmd = &cobra.Command{
	Use:   "clean [packages...]",
	Short: "Tear down runtime environments",
	RunE:  runEnvClean,
}

var envListCmd = &cobra.Command{
	Use:   "list",
	Short: "Show runtime environment status for all packages",
	RunE:  runEnvList,
}

func runEnvSetup(cmd *cobra.Command, args []string) error {
	ws := requireWorkspace()

	fmt.Println()
	fmt.Println(ui.SectionHeader.Render("Setting up environments"))
	fmt.Println()

	var setupCount int
	for name, pkg := range ws.Packages {
		if len(args) > 0 && !contains(args, name) {
			continue
		}
		if pkg.Config.Runtime == nil || len(pkg.Config.Runtime.Setup) == 0 {
			continue
		}

		envDir := filepath.Join(ws.Root, workspace.MarkerDir, "envs", name)

		fmt.Println(ui.StepInfo("Setting up " + ui.Bold.Render(name)))

		for _, cmdStr := range pkg.Config.Runtime.Setup {
			// Substitute {{env_dir}} with absolute path
			expanded := strings.ReplaceAll(cmdStr, "{{env_dir}}", envDir)

			fmt.Println("    " + ui.Muted.Render("$ "+expanded))

			c := exec.Command("sh", "-c", expanded)
			c.Dir = pkg.Dir
			c.Stdout = os.Stdout
			c.Stderr = os.Stderr
			if err := c.Run(); err != nil {
				return fmt.Errorf("env setup for %s failed: %w", name, err)
			}
		}
		fmt.Println(ui.StepDone("Environment ready for " + ui.Bold.Render(name)))
		setupCount++
	}

	if setupCount == 0 {
		fmt.Println(ui.Muted.Render("  No packages define a runtime section"))
	}

	fmt.Println()
	return nil
}

func runEnvClean(cmd *cobra.Command, args []string) error {
	ws := requireWorkspace()

	fmt.Println()
	fmt.Println(ui.SectionHeader.Render("Cleaning environments"))
	fmt.Println()

	envsDir := filepath.Join(ws.Root, workspace.MarkerDir, "envs")
	var cleanCount int

	for name := range ws.Packages {
		if len(args) > 0 && !contains(args, name) {
			continue
		}

		envDir := filepath.Join(envsDir, name)
		if _, err := os.Stat(envDir); os.IsNotExist(err) {
			continue
		}

		if err := os.RemoveAll(envDir); err != nil {
			fmt.Fprintln(os.Stderr, ui.Cross("Failed to clean "+name+": "+err.Error()))
			continue
		}
		fmt.Println(ui.StepDone("Cleaned " + ui.Bold.Render(name)))
		cleanCount++
	}

	if cleanCount == 0 {
		fmt.Println(ui.Muted.Render("  No environments to clean"))
	}

	fmt.Println()
	return nil
}

func runEnvList(cmd *cobra.Command, args []string) error {
	ws := requireWorkspace()

	fmt.Println()
	fmt.Println(ui.SectionHeader.Render("Environment Status"))
	fmt.Println()

	envsDir := filepath.Join(ws.Root, workspace.MarkerDir, "envs")
	var hasRuntime bool

	for name, pkg := range ws.Packages {
		if pkg.Config.Runtime == nil {
			continue
		}
		hasRuntime = true

		envDir := filepath.Join(envsDir, name)
		if _, err := os.Stat(envDir); os.IsNotExist(err) {
			fmt.Println("  " + ui.Cross(ui.Bold.Render(name)+" "+ui.Error.Render("not set up")))
		} else {
			fmt.Println("  " + ui.Check(ui.Bold.Render(name)+" "+ui.Success.Render("ready")+" "+ui.Muted.Render(envDir)))
		}
	}

	if !hasRuntime {
		fmt.Println(ui.Muted.Render("  No packages define a runtime section"))
	}

	fmt.Println()
	return nil
}
