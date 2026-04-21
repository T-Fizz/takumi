package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	gomcp "github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	"github.com/tfitz/takumi/src/config"
	"github.com/tfitz/takumi/src/executor"
	"github.com/tfitz/takumi/src/graph"
	"github.com/tfitz/takumi/src/workspace"
)

func registerTools(s *server.MCPServer) {
	s.AddTool(statusTool, handleStatus)
	s.AddTool(buildTool, handleBuild)
	s.AddTool(testTool, handleTest)
	s.AddTool(diagnoseTool, handleDiagnose)
	s.AddTool(affectedTool, handleAffected)
	s.AddTool(validateTool, handleValidate)
	s.AddTool(graphTool, handleGraph)
}

// ---------------------------------------------------------------------------
// Tool definitions
// ---------------------------------------------------------------------------

var statusTool = gomcp.NewTool("takumi_status",
	gomcp.WithDescription("Get workspace health dashboard: packages, sources, environments, recent builds, and AI agent configuration."),
)

var buildTool = gomcp.NewTool("takumi_build",
	gomcp.WithDescription("Run build phase for packages in dependency order. Returns a summary with log file paths for detailed output."),
	gomcp.WithString("packages", gomcp.Description("Comma-separated package names to build. Omit to build all packages.")),
	gomcp.WithBoolean("affected", gomcp.Description("Only build packages affected by git changes since HEAD.")),
	gomcp.WithBoolean("no_cache", gomcp.Description("Skip cache and force rebuild.")),
)

var testTool = gomcp.NewTool("takumi_test",
	gomcp.WithDescription("Run test phase for packages in dependency order. Returns a summary with log file paths for detailed output."),
	gomcp.WithString("packages", gomcp.Description("Comma-separated package names to test. Omit to test all packages.")),
	gomcp.WithBoolean("affected", gomcp.Description("Only test packages affected by git changes since HEAD.")),
	gomcp.WithBoolean("no_cache", gomcp.Description("Skip cache and force re-run.")),
)

var diagnoseTool = gomcp.NewTool("takumi_diagnose",
	gomcp.WithDescription("Diagnose a build or test failure. Returns failure context: log file path, exit code, changed files, dependency chain, and environment status."),
	gomcp.WithString("package", gomcp.Required(), gomcp.Description("Package name to diagnose.")),
	gomcp.WithString("phase", gomcp.Description("Phase to diagnose (e.g. 'build', 'test'). If omitted, returns the most recent failing log.")),
)

var affectedTool = gomcp.NewTool("takumi_affected",
	gomcp.WithDescription("List packages affected by git changes, including transitive downstream dependents."),
	gomcp.WithString("since", gomcp.Description("Git ref to compare against. Defaults to HEAD.")),
)

var validateTool = gomcp.NewTool("takumi_validate",
	gomcp.WithDescription("Validate all workspace and package configurations. Checks syntax, dependency resolution, version sets, and cycle detection."),
)

var graphTool = gomcp.NewTool("takumi_graph",
	gomcp.WithDescription("Show the package dependency graph with topological level ordering."),
)

// ---------------------------------------------------------------------------
// Handlers
// ---------------------------------------------------------------------------

func handleStatus(ctx context.Context, request gomcp.CallToolRequest) (*gomcp.CallToolResult, error) {
	ws, err := loadWorkspace()
	if err != nil {
		// Outside a workspace — return a discovery message instead of a raw error.
		// This helps agents that have Takumi registered globally discover and init.
		return gomcp.NewToolResultText(
			"Takumi is available but this directory is not a Takumi workspace.\n\n" +
				"To set up Takumi here, run: takumi init\n" +
				"This will create takumi.yaml, a package config, and AI agent integration.\n\n" +
				"Takumi manages builds, tests, and custom phases (deploy, lint, dev) across\n" +
				"multi-package workspaces with dependency-aware execution and caching.\n\n" +
				"After init, use takumi_status to see the workspace dashboard.",
		), nil
	}

	var b strings.Builder
	fmt.Fprintf(&b, "Workspace: %s\n", ws.Config.Workspace.Name)
	fmt.Fprintf(&b, "Root: %s\n\n", ws.Root)

	names := sortedPackageNames(ws)
	fmt.Fprintf(&b, "Packages (%d):\n", len(names))
	for _, name := range names {
		pkg := ws.Packages[name]
		var parts []string
		if nd := len(pkg.Config.Dependencies); nd > 0 {
			parts = append(parts, fmt.Sprintf("%d deps", nd))
		}
		if np := len(pkg.Config.Phases); np > 0 {
			parts = append(parts, fmt.Sprintf("%d phases", np))
		}
		if pkg.Config.Runtime != nil {
			parts = append(parts, "runtime")
		}
		detail := ""
		if len(parts) > 0 {
			detail = " — " + strings.Join(parts, ", ")
		}
		fmt.Fprintf(&b, "  %s v%s%s\n", name, pkg.Config.Package.Version, detail)
	}

	if len(ws.Config.Workspace.Sources) > 0 {
		fmt.Fprintf(&b, "\nSources (%d):\n", len(ws.Config.Workspace.Sources))
		for name, src := range ws.Config.Workspace.Sources {
			srcPath := src.Path
			if srcPath == "" {
				srcPath = name
			}
			full := filepath.Join(ws.Root, srcPath)
			if _, serr := os.Stat(full); serr == nil {
				fmt.Fprintf(&b, "  ✓ %s — %s\n", name, srcPath)
			} else {
				fmt.Fprintf(&b, "  ✗ %s — missing\n", name)
			}
		}
	}

	metricsPath := filepath.Join(ws.Root, workspace.MarkerDir, "metrics.json")
	if data, rerr := os.ReadFile(metricsPath); rerr == nil {
		var metrics executor.MetricsFile
		if json.Unmarshal(data, &metrics) == nil && len(metrics.Runs) > 0 {
			fmt.Fprintf(&b, "\nRecent Builds:\n")
			start := 0
			if len(metrics.Runs) > 5 {
				start = len(metrics.Runs) - 5
			}
			for _, run := range metrics.Runs[start:] {
				status := "✓"
				if run.ExitCode != 0 {
					status = "✗"
				}
				fmt.Fprintf(&b, "  %s %s %s %dms\n", status, run.Package, run.Phase, run.DurationMs)
			}
		}
	}

	if ws.Config.Workspace.AI.Agent != "" {
		fmt.Fprintf(&b, "\nAI Agent: %s\n", ws.Config.Workspace.AI.Agent)
	}

	return gomcp.NewToolResultText(b.String()), nil
}

func handleBuild(ctx context.Context, request gomcp.CallToolRequest) (*gomcp.CallToolResult, error) {
	return handlePhase(ctx, request, "build")
}

func handleTest(ctx context.Context, request gomcp.CallToolRequest) (*gomcp.CallToolResult, error) {
	return handlePhase(ctx, request, "test")
}

func handlePhase(_ context.Context, request gomcp.CallToolRequest, phase string) (*gomcp.CallToolResult, error) {
	ws, err := loadWorkspace()
	if err != nil {
		return gomcp.NewToolResultError(err.Error()), nil
	}

	packagesStr := request.GetString("packages", "")
	affectedFlag := request.GetBool("affected", false)
	noCacheFlag := request.GetBool("no_cache", false)

	var packages []string
	if packagesStr != "" {
		for _, p := range strings.Split(packagesStr, ",") {
			p = strings.TrimSpace(p)
			if p != "" {
				packages = append(packages, p)
			}
		}
	}

	if affectedFlag {
		changedFiles, gerr := workspace.ChangedFiles(ws.Root, "HEAD")
		if gerr != nil {
			return gomcp.NewToolResultError("failed to determine affected packages: " + gerr.Error()), nil
		}
		affected := workspace.MapFilesToPackages(ws, changedFiles)
		g := newGraph(ws)
		allAffected := make(map[string]bool)
		for pkg := range affected {
			allAffected[pkg] = true
			for _, dep := range g.TransitiveDependents(pkg) {
				allAffected[dep] = true
			}
		}
		packages = nil
		for pkg := range allAffected {
			packages = append(packages, pkg)
		}
		if len(packages) == 0 {
			return gomcp.NewToolResultText("No affected packages to " + phase + "."), nil
		}
	}

	results, execErr := executor.Run(ws, executor.RunOptions{
		Phase:    phase,
		Packages: packages,
		Parallel: ws.Config.Workspace.Settings.Parallel,
		NoCache:  noCacheFlag,
		Quiet:    true,
	})

	// If the executor failed before producing any results (e.g. cycle detection),
	// return the error directly so the agent sees the root cause.
	if execErr != nil && len(results) == 0 {
		return gomcp.NewToolResultError(execErr.Error()), nil
	}

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

	var b strings.Builder
	var passed, failed, cached int
	var failedLogs []string

	for _, r := range results {
		if r.CacheHit {
			cached++
		} else if r.Error != nil || r.ExitCode != 0 {
			failed++
			if r.LogFile != "" {
				failedLogs = append(failedLogs, r.LogFile)
			}
		} else {
			passed++
		}
	}

	label := capitalize(phase)
	if failed > 0 {
		fmt.Fprintf(&b, "%s failed: %d passed, %d failed", label, passed, failed)
	} else {
		fmt.Fprintf(&b, "%s completed: %d passed", label, passed)
	}
	if cached > 0 {
		fmt.Fprintf(&b, ", %d cached", cached)
	}
	fmt.Fprintln(&b)
	fmt.Fprintln(&b)

	fmt.Fprintln(&b, "Results:")
	for _, r := range results {
		if r.CacheHit {
			fmt.Fprintf(&b, "  ✓ %s — cached\n", r.Package)
		} else if r.Error != nil || r.ExitCode != 0 {
			fmt.Fprintf(&b, "  ✗ %s — exit code %d (log: %s)\n", r.Package, r.ExitCode, r.LogFile)
		} else {
			fmt.Fprintf(&b, "  ✓ %s — %s", r.Package, r.Duration.Round(time.Millisecond))
			if r.LogFile != "" {
				fmt.Fprintf(&b, " (log: %s)", r.LogFile)
			}
			fmt.Fprintln(&b)
		}
	}

	if len(failedLogs) > 0 {
		fmt.Fprintln(&b)
		fmt.Fprintln(&b, "Failed package logs:")
		for _, logPath := range failedLogs {
			fmt.Fprintf(&b, "  %s\n", logPath)
		}
	}

	if execErr != nil {
		return gomcp.NewToolResultError(b.String()), nil
	}
	return gomcp.NewToolResultText(b.String()), nil
}

func handleDiagnose(_ context.Context, request gomcp.CallToolRequest) (*gomcp.CallToolResult, error) {
	ws, err := loadWorkspace()
	if err != nil {
		return gomcp.NewToolResultError(err.Error()), nil
	}

	pkgName, err := request.RequireString("package")
	if err != nil {
		return gomcp.NewToolResultError("package parameter is required"), nil
	}

	pkg, exists := ws.Packages[pkgName]
	if !exists {
		return gomcp.NewToolResultError("package not found: " + pkgName), nil
	}

	logDir := filepath.Join(ws.Root, workspace.MarkerDir, "logs")
	requestedPhase := request.GetString("phase", "")

	// Find the right log to diagnose. Strategy:
	// 1. If a phase was explicitly requested, use that log.
	// 2. Otherwise, prefer the most recent failing log (non-zero exit code).
	// 3. Fall back to the most recent log of any kind.
	var logFile, phase string
	if requestedPhase != "" {
		path := filepath.Join(logDir, fmt.Sprintf("%s.%s.log", pkgName, requestedPhase))
		if info, serr := os.Stat(path); serr == nil && info.Size() > 0 {
			logFile = path
			phase = requestedPhase
		}
	} else {
		var fallbackFile, fallbackPhase string
		var fallbackTime time.Time
		for _, p := range []string{"build", "test"} {
			path := filepath.Join(logDir, fmt.Sprintf("%s.%s.log", pkgName, p))
			info, serr := os.Stat(path)
			if serr != nil || info.Size() == 0 {
				continue
			}
			// Track as fallback if it's the newest log
			if fallbackFile == "" || info.ModTime().After(fallbackTime) {
				fallbackFile = path
				fallbackPhase = p
				fallbackTime = info.ModTime()
			}
			// Prefer failing logs (non-zero exit code)
			if exitCode := logExitCode(path); exitCode != 0 {
				if logFile == "" || info.ModTime().After(fallbackTime) {
					logFile = path
					phase = p
				}
			}
		}
		if logFile == "" {
			logFile = fallbackFile
			phase = fallbackPhase
		}
	}

	var b strings.Builder
	fmt.Fprintf(&b, "Diagnosis for %s:\n\n", pkgName)

	if logFile == "" {
		fmt.Fprintln(&b, "No log files found. Run build or test first.")
		return gomcp.NewToolResultText(b.String()), nil
	}

	fmt.Fprintf(&b, "Phase: %s\n", phase)
	fmt.Fprintf(&b, "Log file: %s\n", logFile)

	if data, rerr := os.ReadFile(logFile); rerr == nil {
		lines := strings.Split(string(data), "\n")
		for i := len(lines) - 1; i >= 0; i-- {
			line := strings.TrimSpace(lines[i])
			if strings.HasPrefix(line, "# exit code:") {
				fmt.Fprintf(&b, "Exit code: %s\n", strings.TrimPrefix(line, "# exit code: "))
				break
			}
			if strings.HasPrefix(line, "# duration:") {
				fmt.Fprintf(&b, "Duration: %s\n", strings.TrimPrefix(line, "# duration: "))
			}
		}
	}

	if files, gerr := workspace.ChangedFiles(ws.Root, "HEAD"); gerr == nil && len(files) > 0 {
		fmt.Fprintf(&b, "\nChanged files:\n")
		for _, f := range files {
			fmt.Fprintf(&b, "  %s\n", f)
		}
	}

	if len(pkg.Config.Dependencies) > 0 {
		fmt.Fprintf(&b, "\nDependencies: %s\n", strings.Join(pkg.Config.Dependencies, ", "))
	}

	if pkg.Config.Runtime != nil {
		envDir := filepath.Join(ws.Root, workspace.MarkerDir, "envs", pkgName)
		if _, serr := os.Stat(envDir); serr == nil {
			fmt.Fprintf(&b, "\nRuntime: configured (env dir: %s)\n", envDir)
		} else {
			fmt.Fprintln(&b, "\nRuntime: configured but not set up (run takumi env setup)")
		}
	}

	fmt.Fprintf(&b, "\nTo inspect the full log, read: %s\n", logFile)

	return gomcp.NewToolResultText(b.String()), nil
}

func handleAffected(_ context.Context, request gomcp.CallToolRequest) (*gomcp.CallToolResult, error) {
	ws, err := loadWorkspace()
	if err != nil {
		return gomcp.NewToolResultError(err.Error()), nil
	}

	since := request.GetString("since", "HEAD")

	changedFiles, err := workspace.ChangedFiles(ws.Root, since)
	if err != nil {
		return gomcp.NewToolResultError("failed to get changed files: " + err.Error()), nil
	}

	if len(changedFiles) == 0 {
		return gomcp.NewToolResultText(fmt.Sprintf("No changed files since %s.", since)), nil
	}

	direct := workspace.MapFilesToPackages(ws, changedFiles)
	g := newGraph(ws)
	transitive := make(map[string]bool)
	for pkg := range direct {
		for _, dep := range g.TransitiveDependents(pkg) {
			if !direct[dep] {
				transitive[dep] = true
			}
		}
	}

	var b strings.Builder
	fmt.Fprintf(&b, "Since: %s\n", since)
	fmt.Fprintf(&b, "Changed files: %d\n\n", len(changedFiles))

	if len(direct) > 0 {
		fmt.Fprintln(&b, "Directly affected:")
		for _, name := range sortedKeys(direct) {
			fmt.Fprintf(&b, "  %s\n", name)
		}
	}

	if len(transitive) > 0 {
		fmt.Fprintln(&b, "\nTransitively affected:")
		for _, name := range sortedKeys(transitive) {
			fmt.Fprintf(&b, "  %s\n", name)
		}
	}

	total := len(direct) + len(transitive)
	fmt.Fprintf(&b, "\nTotal affected: %d packages\n", total)

	return gomcp.NewToolResultText(b.String()), nil
}

func handleValidate(_ context.Context, _ gomcp.CallToolRequest) (*gomcp.CallToolResult, error) {
	ws, err := loadWorkspace()
	if err != nil {
		return gomcp.NewToolResultError(err.Error()), nil
	}

	var b strings.Builder
	var errCount, warnCount int

	for _, f := range config.ValidateWorkspace(ws.Config) {
		if f.Severity == config.SeverityError {
			errCount++
			fmt.Fprintf(&b, "✗ %s — %s\n", f.Field, f.Message)
		} else {
			warnCount++
			fmt.Fprintf(&b, "! %s — %s\n", f.Field, f.Message)
		}
	}

	for _, pe := range ws.ParseErrors {
		errCount++
		fmt.Fprintf(&b, "✗ %s — parse error: %s\n", pe.Path, pe.Err)
	}

	for _, name := range sortedPackageNames(ws) {
		pkg := ws.Packages[name]
		for _, f := range config.ValidatePackage(pkg.Config) {
			if f.Severity == config.SeverityError {
				errCount++
				fmt.Fprintf(&b, "✗ %s: %s — %s\n", name, f.Field, f.Message)
			} else {
				warnCount++
				fmt.Fprintf(&b, "! %s: %s — %s\n", name, f.Field, f.Message)
			}
		}
	}

	if ws.Config.Workspace.VersionSet.File != "" {
		vsPath := filepath.Join(ws.Root, ws.Config.Workspace.VersionSet.File)
		if _, serr := os.Stat(vsPath); serr == nil {
			vsCfg, loadErr := config.LoadVersionSetConfig(vsPath)
			if loadErr != nil {
				errCount++
				fmt.Fprintf(&b, "✗ version-set — failed to load: %s\n", loadErr)
			} else {
				for _, f := range config.ValidateVersionSet(vsCfg) {
					if f.Severity == config.SeverityError {
						errCount++
						fmt.Fprintf(&b, "✗ version-set: %s — %s\n", f.Field, f.Message)
					} else {
						warnCount++
						fmt.Fprintf(&b, "! version-set: %s — %s\n", f.Field, f.Message)
					}
				}
			}
		}
	}

	for _, name := range sortedPackageNames(ws) {
		pkg := ws.Packages[name]
		for _, dep := range pkg.Config.Dependencies {
			if _, exists := ws.Packages[dep]; !exists {
				errCount++
				fmt.Fprintf(&b, "✗ %s — depends on %q which is not in the workspace\n", name, dep)
			}
		}
	}

	g := newGraph(ws)
	if _, sortErr := g.Sort(); sortErr != nil {
		errCount++
		fmt.Fprintf(&b, "✗ dependency cycle detected: %s\n", sortErr)
	}

	if errCount == 0 && warnCount == 0 {
		return gomcp.NewToolResultText("All configurations valid."), nil
	}

	fmt.Fprintf(&b, "\nValidation: %d errors, %d warnings\n", errCount, warnCount)

	if errCount > 0 {
		return gomcp.NewToolResultError(b.String()), nil
	}
	return gomcp.NewToolResultText(b.String()), nil
}

func handleGraph(_ context.Context, _ gomcp.CallToolRequest) (*gomcp.CallToolResult, error) {
	ws, err := loadWorkspace()
	if err != nil {
		return gomcp.NewToolResultError(err.Error()), nil
	}

	g := newGraph(ws)
	levels, err := g.Sort()
	if err != nil {
		return gomcp.NewToolResultError("cycle detected: " + err.Error()), nil
	}

	var b strings.Builder
	fmt.Fprintf(&b, "Dependency Graph (%d packages, %d levels):\n\n", len(ws.Packages), len(levels))

	for _, level := range levels {
		sort.Strings(level.Packages)
		if level.Index == 0 {
			fmt.Fprintf(&b, "Level %d (no dependencies):\n", level.Index)
		} else {
			fmt.Fprintf(&b, "Level %d:\n", level.Index)
		}
		for _, name := range level.Packages {
			deps := g.DepsOf(name)
			if len(deps) > 0 {
				sorted := make([]string, len(deps))
				copy(sorted, deps)
				sort.Strings(sorted)
				fmt.Fprintf(&b, "  %s → %s\n", name, strings.Join(sorted, ", "))
			} else {
				fmt.Fprintf(&b, "  %s\n", name)
			}
		}
		fmt.Fprintln(&b)
	}

	return gomcp.NewToolResultText(b.String()), nil
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

func loadWorkspace() (*workspace.Info, error) {
	cwd, err := os.Getwd()
	if err != nil {
		return nil, fmt.Errorf("failed to get working directory: %w", err)
	}
	ws, err := workspace.Load(cwd)
	if err != nil {
		return nil, fmt.Errorf("failed to load workspace: %w", err)
	}
	if ws == nil {
		return nil, fmt.Errorf("no takumi workspace found in %s", cwd)
	}
	return ws, nil
}

func newGraph(ws *workspace.Info) *graph.Graph {
	g := graph.New()
	for name, pkg := range ws.Packages {
		g.AddNode(name, pkg.Config.Dependencies)
	}
	return g
}

func sortedPackageNames(ws *workspace.Info) []string {
	names := make([]string, 0, len(ws.Packages))
	for name := range ws.Packages {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

func sortedKeys(m map[string]bool) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

func capitalize(s string) string {
	if s == "" {
		return s
	}
	return strings.ToUpper(s[:1]) + s[1:]
}

// logExitCode reads the exit code trailer from a takumi log file.
// Returns -1 if the file can't be read or has no exit code line.
func logExitCode(path string) int {
	data, err := os.ReadFile(path)
	if err != nil {
		return -1
	}
	lines := strings.Split(string(data), "\n")
	for i := len(lines) - 1; i >= 0; i-- {
		line := strings.TrimSpace(lines[i])
		if strings.HasPrefix(line, "# exit code:") {
			code, cerr := strconv.Atoi(strings.TrimSpace(strings.TrimPrefix(line, "# exit code:")))
			if cerr == nil {
				return code
			}
		}
	}
	return -1
}
