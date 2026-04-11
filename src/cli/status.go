package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"github.com/spf13/cobra"
	"github.com/tfitz/takumi/src/executor"
	"github.com/tfitz/takumi/src/ui"
	"github.com/tfitz/takumi/src/workspace"
)

func init() {
	rootCmd.AddCommand(statusCmd)
}

var statusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show workspace health dashboard",
	Long: `Display a full workspace health overview including package count, environment
status, version-set health, and recent build results.`,
	RunE: runStatus,
}

func runStatus(cmd *cobra.Command, args []string) error {
	ws := requireWorkspace()

	fmt.Println()
	fmt.Println(ui.Header())
	fmt.Println(ui.SectionHeader.Render("Workspace: " + ws.Config.Workspace.Name))
	fmt.Println(ui.Muted.Render("  " + ws.Root))
	fmt.Println()

	// Packages
	fmt.Println(ui.Bold.Render("  Packages"))
	if len(ws.Packages) == 0 {
		fmt.Println(ui.Muted.Render("    No packages found"))
	} else {
		var names []string
		for name := range ws.Packages {
			names = append(names, name)
		}
		sort.Strings(names)

		for _, name := range names {
			pkg := ws.Packages[name]
			info := ui.Bold.Render(name) + " " + ui.Muted.Render("v"+pkg.Config.Package.Version)
			deps := len(pkg.Config.Dependencies)
			phases := len(pkg.Config.Phases)
			var details []string
			if deps > 0 {
				details = append(details, ui.FormatCount(deps, "dep", "deps"))
			}
			if phases > 0 {
				details = append(details, ui.FormatCount(phases, "phase", "phases"))
			}
			if pkg.Config.Runtime != nil {
				details = append(details, "runtime")
			}
			if len(details) > 0 {
				info += " " + ui.Muted.Render("("+joinParts(details)+")")
			}
			fmt.Println("    " + ui.Bullet(info))
		}
	}
	fmt.Println()

	// Sources
	if len(ws.Config.Workspace.Sources) > 0 {
		fmt.Println(ui.Bold.Render("  Tracked Sources"))
		for name, source := range ws.Config.Workspace.Sources {
			status := ui.Success.Render("present")
			dir := source.Path
			if !filepath.IsAbs(dir) {
				dir = filepath.Join(ws.Root, dir)
			}
			if _, err := os.Stat(dir); os.IsNotExist(err) {
				status = ui.Error.Render("missing")
			}
			fmt.Printf("    %s %s %s %s\n",
				ui.Bullet(ui.Bold.Render(name)),
				ui.Muted.Render("("+source.Branch+")"),
				status,
				"",
			)
		}
		fmt.Println()
	}

	// Environments
	envsDir := filepath.Join(ws.Root, workspace.MarkerDir, "envs")
	hasRuntime := false
	for _, pkg := range ws.Packages {
		if pkg.Config.Runtime != nil {
			hasRuntime = true
			break
		}
	}
	if hasRuntime {
		fmt.Println(ui.Bold.Render("  Environments"))
		for name, pkg := range ws.Packages {
			if pkg.Config.Runtime == nil {
				continue
			}
			envDir := filepath.Join(envsDir, name)
			if _, err := os.Stat(envDir); os.IsNotExist(err) {
				fmt.Println("    " + ui.Cross(name+" "+ui.Error.Render("not set up")))
			} else {
				fmt.Println("    " + ui.Check(name+" "+ui.Success.Render("ready")))
			}
		}
		fmt.Println()
	}

	// Recent build metrics
	metricsPath := filepath.Join(ws.Root, workspace.MarkerDir, "metrics.json")
	if data, err := os.ReadFile(metricsPath); err == nil {
		var metrics executor.MetricsFile
		if json.Unmarshal(data, &metrics) == nil && len(metrics.Runs) > 0 {
			fmt.Println(ui.Bold.Render("  Recent Builds"))
			// Show last 5
			start := 0
			if len(metrics.Runs) > 5 {
				start = len(metrics.Runs) - 5
			}
			for _, run := range metrics.Runs[start:] {
				status := ui.Success.Render("pass")
				if run.ExitCode != 0 {
					status = ui.Error.Render("fail")
				}
				fmt.Printf("    %s %s %s %s\n",
					status,
					ui.Bold.Render(run.Package),
					ui.Muted.Render(run.Phase),
					ui.Muted.Render(fmt.Sprintf("%dms", run.DurationMs)),
				)
			}
			fmt.Println()
		}
	}

	// AI agent
	if ws.Config.Workspace.AI.Agent != "" {
		agent := ws.Config.Workspace.AI.Agent
		fmt.Println(ui.Bold.Render("  AI Agent"))
		fmt.Println("    " + ui.Check(agent))
		fmt.Println()
	}

	fmt.Println(ui.Divider())
	fmt.Println(ui.FormatCount(len(ws.Packages), "package", "packages") +
		", " + ui.FormatCount(len(ws.Config.Workspace.Sources), "source", "sources"))
	fmt.Println()

	return nil
}
