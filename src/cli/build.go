package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"github.com/tfitz/takumi/src/cache"
	"github.com/tfitz/takumi/src/executor"
	"github.com/tfitz/takumi/src/ui"
	"github.com/tfitz/takumi/src/workspace"
)

const buildDir = "build"

func init() {
	buildCmd.Flags().Bool("affected", false, "Only build packages affected by changes")
	buildCmd.Flags().Bool("no-cache", false, "Skip cache and force execution")
	buildCmd.Flags().Bool("dry-run", false, "Show execution plan without running anything")
	buildCmd.AddCommand(buildCleanCmd)
	rootCmd.AddCommand(buildCmd)
}

var buildCmd = &cobra.Command{
	Use:   "build [packages...]",
	Short: "Run build phase for packages",
	Long: `Build packages in dependency order. If no packages are specified, builds all
packages in the workspace. Use --affected to build only changed packages
and their downstream dependents.

Subcommands:
  clean    Remove build artifacts`,
	RunE: runBuild,
}

var buildCleanCmd = &cobra.Command{
	Use:   "clean",
	Short: "Remove build artifacts",
	Long:  `Remove the build/ directory and all compiled artifacts.`,
	RunE:  runBuildClean,
}

func runBuild(cmd *cobra.Command, args []string) error {
	return runPhaseCommand(cmd, args, "build")
}

func runBuildClean(cmd *cobra.Command, args []string) error {
	ws := requireWorkspace()

	dir := filepath.Join(ws.Root, buildDir)
	if err := os.RemoveAll(dir); err != nil {
		return fmt.Errorf("removing %s: %w", buildDir, err)
	}

	cacheStore := cache.NewStore(ws.Root)
	cacheStore.Clean()

	fmt.Println(ui.StepDone("Cleaned " + ui.FilePath(buildDir+"/") + " and cache"))
	return nil
}

// runPhaseCommand is shared logic for build/test/run commands.
func runPhaseCommand(cmd *cobra.Command, args []string, phase string) error {
	ws := requireWorkspace()
	affectedFlag, _ := cmd.Flags().GetBool("affected")
	noCacheFlag, _ := cmd.Flags().GetBool("no-cache")
	dryRunFlag, _ := cmd.Flags().GetBool("dry-run")

	var packages []string

	if affectedFlag {
		changedFiles, err := workspace.ChangedFiles(ws.Root, "HEAD")
		if err != nil {
			return fmt.Errorf("determining affected packages: %w", err)
		}
		affected := workspace.MapFilesToPackages(ws, changedFiles)
		g := buildGraph(ws)
		allAffected := make(map[string]bool)
		for pkg := range affected {
			allAffected[pkg] = true
			for _, dep := range g.TransitiveDependents(pkg) {
				allAffected[dep] = true
			}
		}
		for pkg := range allAffected {
			packages = append(packages, pkg)
		}

		if len(packages) == 0 {
			fmt.Println(ui.Check("No affected packages to " + phase))
			return nil
		}
	} else if len(args) > 0 {
		packages = args
	}

	if dryRunFlag {
		return printDryRun(ws, packages, phase, noCacheFlag)
	}

	fmt.Println()
	label := phase
	if len(packages) > 0 {
		label += fmt.Sprintf(" (%s)", ui.FormatCount(len(packages), "package", "packages"))
	}
	fmt.Println(ui.SectionHeader.Render("Running " + label))
	fmt.Println()

	results, err := executor.Run(ws, executor.RunOptions{
		Phase:    phase,
		Packages: packages,
		Parallel: ws.Config.Workspace.Settings.Parallel,
		NoCache:  noCacheFlag,
	})

	// Record metrics for non-cached results
	if len(results) > 0 {
		var executed []executor.Result
		for _, r := range results {
			if !r.CacheHit {
				executed = append(executed, r)
			}
		}
		if len(executed) > 0 {
			executor.RecordMetrics(ws.Root, executed)
		}
	}

	// Print summary
	fmt.Println()
	fmt.Println(ui.Divider())

	var passed, failed, skipped, cached int
	for _, r := range results {
		if r.CacheHit {
			cached++
			fmt.Println(ui.Check(r.Package + " " + ui.Muted.Render("cached")))
		} else if r.Error != nil || r.ExitCode != 0 {
			failed++
			fmt.Println(ui.Cross(r.Package + " " + ui.Muted.Render(r.Duration.Round(time.Millisecond).String())))
		} else if r.Duration == 0 && r.LogFile == "" {
			skipped++
		} else {
			passed++
			fmt.Println(ui.StepDone(r.Package + " " + ui.Muted.Render(r.Duration.Round(time.Millisecond).String())))
		}
	}

	parts := []string{}
	if passed > 0 {
		parts = append(parts, ui.Success.Render(ui.FormatCount(passed, "passed", "passed")))
	}
	if failed > 0 {
		parts = append(parts, ui.Error.Render(ui.FormatCount(failed, "failed", "failed")))
	}
	if cached > 0 {
		parts = append(parts, ui.Muted.Render(ui.FormatCount(cached, "cached", "cached")))
	}
	if skipped > 0 {
		parts = append(parts, ui.Muted.Render(ui.FormatCount(skipped, "skipped", "skipped")))
	}
	if len(parts) > 0 {
		fmt.Println(joinParts(parts))
	}
	fmt.Println()

	return err
}

// printDryRun shows the execution plan without running anything.
func printDryRun(ws *workspace.Info, packages []string, phase string, noCache bool) error {
	g := buildGraph(ws)
	levels, err := g.Sort()
	if err != nil {
		return err
	}

	targetSet := make(map[string]bool)
	if len(packages) > 0 {
		for _, p := range packages {
			targetSet[p] = true
		}
	}

	// Compute cache state
	store := cache.NewStore(ws.Root)
	cacheKeys := make(map[string]string)

	fmt.Println()
	fmt.Println(ui.SectionHeader.Render("Dry Run: " + phase))
	fmt.Println()

	var totalPkgs, totalCmds, totalCached int
	parallel := ws.Config.Workspace.Settings.Parallel
	renderedLevels := 0

	for _, level := range levels {
		var levelPkgs []string
		for _, name := range level.Packages {
			if len(targetSet) > 0 && !targetSet[name] {
				continue
			}
			pkg, exists := ws.Packages[name]
			if !exists {
				continue
			}
			if pkg.Config.Phases[phase] == nil {
				continue
			}
			levelPkgs = append(levelPkgs, name)
		}
		if len(levelPkgs) == 0 {
			continue
		}

		sort.Strings(levelPkgs)
		renderedLevels++

		mode := "sequential"
		if parallel && len(levelPkgs) > 1 {
			mode = "parallel"
		}
		fmt.Printf("  %s\n", ui.Muted.Render(fmt.Sprintf("Level %d — %s", level.Index, mode)))

		for _, name := range levelPkgs {
			pkg := ws.Packages[name]
			totalPkgs++

			// Compute cache key for this package
			depKeys := make(map[string]string)
			for _, dep := range pkg.Config.Dependencies {
				if key, ok := cacheKeys[dep]; ok {
					depKeys[dep] = key
				}
			}
			configPath := filepath.Join(pkg.Dir, workspace.PackageFile)
			key, _, _ := cache.ComputeKey(pkg.Dir, configPath, phase, depKeys, ws.Config.Workspace.Ignore)
			if key != "" {
				cacheKeys[name] = key
			}

			// Check cache status
			isCached := false
			if !noCache && key != "" {
				if entry := store.Lookup(name, phase); entry != nil && entry.Key == key {
					isCached = true
				}
			}

			if isCached {
				totalCached++
				fmt.Printf("    %s  %s\n", ui.Bold.Render(name), ui.Check("cached"))
			} else {
				phaseConfig := pkg.Config.Phases[phase]
				cmdCount := len(phaseConfig.Pre) + len(phaseConfig.Commands) + len(phaseConfig.Post)
				totalCmds += cmdCount

				reason := ""
				if !noCache && key != "" {
					if store.Lookup(name, phase) == nil {
						reason = "no cache entry"
					} else {
						reason = "content changed"
					}
				}

				label := fmt.Sprintf("will run (%s)", ui.FormatCount(cmdCount, "command", "commands"))
				if reason != "" {
					label += " — " + reason
				}
				fmt.Printf("    %s  %s\n", ui.Bold.Render(name), ui.Muted.Render(label))

				printCmdGroup("pre", phaseConfig.Pre)
				printCmdGroup("cmd", phaseConfig.Commands)
				printCmdGroup("post", phaseConfig.Post)

				if pkg.Config.Runtime != nil && len(pkg.Config.Runtime.Env) > 0 {
					var envKeys []string
					for k := range pkg.Config.Runtime.Env {
						envKeys = append(envKeys, k)
					}
					sort.Strings(envKeys)
					for _, k := range envKeys {
						fmt.Printf("      %s  %s=%s\n",
							ui.Muted.Render("env:"), k, ui.Muted.Render(pkg.Config.Runtime.Env[k]))
					}
				}
			}
			fmt.Println()
		}
	}

	fmt.Println(ui.Divider())
	parts := []string{
		ui.FormatCount(totalPkgs, "package", "packages"),
		ui.FormatCount(renderedLevels, "level", "levels"),
	}
	if totalCached > 0 {
		parts = append(parts, ui.FormatCount(totalCached, "cached", "cached"))
	}
	if totalCmds > 0 {
		parts = append(parts, ui.FormatCount(totalCmds, "command", "commands"))
	}
	fmt.Println(strings.Join(parts, ", "))
	fmt.Println()
	return nil
}

func printCmdGroup(label string, cmds []string) {
	for _, c := range cmds {
		fmt.Printf("      %s  %s\n", ui.Muted.Render(label+":"), c)
	}
}
