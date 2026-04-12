package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"github.com/tfitz/takumi/src/graph"
	"github.com/tfitz/takumi/src/ui"
)

var benchPublish bool
var benchModel string

func init() {
	benchmarkCmd.Flags().BoolVar(&benchPublish, "publish", false, "publish results to a GitHub Gist dashboard")
	benchmarkCmd.Flags().StringVar(&benchModel, "model", "", "override the LLM model (default: claude-haiku-4-5-20251001)")
	rootCmd.AddCommand(benchmarkCmd)
}

var benchmarkCmd = &cobra.Command{
	Use:   "benchmark [scenarios...]",
	Short: "Run performance benchmarks comparing agent work with and without Takumi",
	Long: `Run identical tasks with and without Takumi operator instructions, measuring
token usage, tool calls, turns, errors, and wall-clock time. Optionally publish
results to a shareable GitHub Gist dashboard.

Available scenarios: fix-build-error, scoped-rebuild, understand-structure
If no scenarios specified, all are run.`,
	RunE: runBenchmark,
}

// benchResult mirrors the JSON structure from benchmark.py
type benchResult struct {
	Model     string                                  `json:"model"`
	MaxTurns  int                                     `json:"max_turns"`
	Timestamp string                                  `json:"timestamp"`
	Scenarios map[string]map[string]benchScenarioRun  `json:"scenarios"`
	Totals    map[string]benchTotals                  `json:"totals"`
}

type benchScenarioRun struct {
	InputTokens   int     `json:"input_tokens"`
	OutputTokens  int     `json:"output_tokens"`
	ToolCalls     int     `json:"tool_calls"`
	Turns         int     `json:"turns"`
	Errors        int     `json:"errors"`
	WallTimeS     float64 `json:"wall_time_s"`
	TaskCompleted bool    `json:"task_completed"`
	Success       bool    `json:"success"`
	Error         string  `json:"error,omitempty"`
}

type benchTotals struct {
	Tokens int `json:"tokens"`
	Turns  int `json:"turns"`
	Calls  int `json:"calls"`
	Errors int `json:"errors"`
}

func runBenchmark(cmd *cobra.Command, args []string) error {
	// Load .env if present (for ANTHROPIC_API_KEY)
	loadDotEnv()

	// Find the benchmark script
	self, err := os.Executable()
	if err != nil {
		return fmt.Errorf("cannot find executable path: %w", err)
	}
	selfDir := filepath.Dir(self)

	// Look for benchmark.py relative to binary or in known locations
	scriptPath := findBenchmarkScript(selfDir)
	if scriptPath == "" {
		return fmt.Errorf("benchmark script not found. Run from the Takumi repo or set BENCH_SCRIPT")
	}

	// Find a python with anthropic
	python := findPython()
	if python == "" {
		return fmt.Errorf("python3 with anthropic package not found. Run: pip install anthropic")
	}

	// Check API key
	if os.Getenv("ANTHROPIC_API_KEY") == "" {
		return fmt.Errorf("ANTHROPIC_API_KEY not set. Add it to .env or export it")
	}

	// Build command args
	pyArgs := []string{scriptPath}
	pyArgs = append(pyArgs, args...)

	// Set environment
	env := os.Environ()
	env = append(env, "TAKUMI_BIN="+self)
	if benchModel != "" {
		env = append(env, "BENCH_MODEL="+benchModel)
	}

	// Run benchmark — stream output directly
	c := exec.Command(python, pyArgs...)
	c.Env = env
	c.Stdout = os.Stdout
	c.Stderr = os.Stderr
	c.Stdin = os.Stdin

	if err := c.Run(); err != nil {
		return fmt.Errorf("benchmark failed: %w", err)
	}

	if !benchPublish {
		return nil
	}

	// ── Publish to Gist ─────────────────────────────────────────────────
	fmt.Println()
	fmt.Println(ui.SectionHeader.Render("Publishing results"))

	// Read results JSON
	resultsPath := filepath.Join(filepath.Dir(scriptPath), "results.json")
	data, err := os.ReadFile(resultsPath)
	if err != nil {
		return fmt.Errorf("cannot read results: %w", err)
	}

	var results benchResult
	if err := json.Unmarshal(data, &results); err != nil {
		return fmt.Errorf("cannot parse results: %w", err)
	}

	// Collect workspace context (if in a workspace)
	wsContext := collectWorkspaceContext()

	// Collect log files
	logsDir := filepath.Join(filepath.Dir(scriptPath), "logs")
	logs := collectLogs(logsDir)

	// Generate markdown report
	report := generateReport(results, wsContext, logs)

	// Write report to temp file
	tmpFile, err := os.CreateTemp("", "takumi-bench-*.md")
	if err != nil {
		return fmt.Errorf("cannot create temp file: %w", err)
	}
	defer os.Remove(tmpFile.Name())

	if _, err := tmpFile.WriteString(report); err != nil {
		return fmt.Errorf("writing report: %w", err)
	}
	tmpFile.Close()

	// Create Gist
	ghArgs := []string{
		"gist", "create", tmpFile.Name(),
		"--desc", fmt.Sprintf("Takumi Performance Benchmark — %s", results.Timestamp),
		"--public",
	}

	ghCmd := exec.Command("gh", ghArgs...)
	ghOut, err := ghCmd.CombinedOutput()
	if err != nil {
		fmt.Fprintln(os.Stderr, ui.Cross("Failed to create Gist. Is `gh` installed and authenticated?"))
		fmt.Fprintln(os.Stderr, string(ghOut))

		// Fall back: save report locally
		localPath := filepath.Join(filepath.Dir(scriptPath), "report.md")
		os.WriteFile(localPath, []byte(report), 0644)
		fmt.Println(ui.StepInfo("Report saved locally: " + localPath))
		return nil
	}

	gistURL := strings.TrimSpace(string(ghOut))
	fmt.Println(ui.StepDone("Published: " + gistURL))

	return nil
}

// workspaceContext holds info about the current workspace for the report.
type workspaceContext struct {
	Name     string
	Packages []pkgContext
	Graph    string
}

type pkgContext struct {
	Name         string
	Version      string
	Dependencies []string
	Phases       []string
	HasRuntime   bool
	AIDesc       string
}

func collectWorkspaceContext() *workspaceContext {
	ws, err := loadWorkspace()
	if err != nil {
		return nil
	}

	ctx := &workspaceContext{Name: ws.Config.Workspace.Name}

	var names []string
	for name := range ws.Packages {
		names = append(names, name)
	}
	sort.Strings(names)

	for _, name := range names {
		pkg := ws.Packages[name]
		pc := pkgContext{
			Name:         name,
			Version:      pkg.Config.Package.Version,
			Dependencies: pkg.Config.Dependencies,
			HasRuntime:   pkg.Config.Runtime != nil,
		}
		for phase := range pkg.Config.Phases {
			pc.Phases = append(pc.Phases, phase)
		}
		sort.Strings(pc.Phases)
		if pkg.Config.AI != nil {
			pc.AIDesc = pkg.Config.AI.Description
		}
		ctx.Packages = append(ctx.Packages, pc)
	}

	// Build graph string
	g := graph.New()
	for name, pkg := range ws.Packages {
		g.AddNode(name, pkg.Config.Dependencies)
	}
	if levels, err := g.Sort(); err == nil {
		var sb strings.Builder
		for _, level := range levels {
			pkgs := level.Packages
			sort.Strings(pkgs)
			sb.WriteString(fmt.Sprintf("Level %d: %s\n", level.Index, strings.Join(pkgs, ", ")))
		}
		ctx.Graph = sb.String()
	}

	return ctx
}

func collectLogs(logsDir string) map[string]string {
	logs := make(map[string]string)
	entries, err := os.ReadDir(logsDir)
	if err != nil {
		return logs
	}
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".log") {
			data, err := os.ReadFile(filepath.Join(logsDir, e.Name()))
			if err == nil {
				logs[e.Name()] = string(data)
			}
		}
	}
	return logs
}

func generateReport(results benchResult, ws *workspaceContext, logs map[string]string) string {
	var sb strings.Builder

	sb.WriteString("# Takumi Performance Benchmark\n\n")
	sb.WriteString(fmt.Sprintf("> Generated: %s\n\n", time.Now().Format("2006-01-02 15:04:05 MST")))

	// Config
	sb.WriteString("## Configuration\n\n")
	sb.WriteString(fmt.Sprintf("| Setting | Value |\n"))
	sb.WriteString(fmt.Sprintf("|---------|-------|\n"))
	sb.WriteString(fmt.Sprintf("| Model | `%s` |\n", results.Model))
	sb.WriteString(fmt.Sprintf("| Max turns | %d |\n", results.MaxTurns))
	sb.WriteString(fmt.Sprintf("| Takumi version | `%s` |\n", version))
	sb.WriteString(fmt.Sprintf("| Timestamp | %s |\n", results.Timestamp))
	sb.WriteString("\n")

	// Workspace context
	if ws != nil {
		sb.WriteString("## Workspace Context\n\n")
		sb.WriteString(fmt.Sprintf("**Workspace:** %s\n\n", ws.Name))

		if len(ws.Packages) > 0 {
			sb.WriteString("### Packages\n\n")
			sb.WriteString("| Package | Version | Dependencies | Phases | Runtime | AI Description |\n")
			sb.WriteString("|---------|---------|-------------|--------|---------|----------------|\n")
			for _, pkg := range ws.Packages {
				deps := "-"
				if len(pkg.Dependencies) > 0 {
					deps = strings.Join(pkg.Dependencies, ", ")
				}
				phases := strings.Join(pkg.Phases, ", ")
				runtime := "-"
				if pkg.HasRuntime {
					runtime = "yes"
				}
				aiDesc := "-"
				if pkg.AIDesc != "" {
					aiDesc = pkg.AIDesc
				}
				sb.WriteString(fmt.Sprintf("| %s | %s | %s | %s | %s | %s |\n",
					pkg.Name, pkg.Version, deps, phases, runtime, aiDesc))
			}
			sb.WriteString("\n")
		}

		if ws.Graph != "" {
			sb.WriteString("### Dependency Graph\n\n")
			sb.WriteString("```\n")
			sb.WriteString(ws.Graph)
			sb.WriteString("```\n\n")
		}
	}

	// Overall summary
	w := results.Totals["without"]
	t := results.Totals["with"]
	if w.Tokens > 0 {
		tokPct := float64(w.Tokens-t.Tokens) / float64(w.Tokens) * 100

		sb.WriteString("## Overall Results\n\n")
		sb.WriteString("| Metric | Without Takumi | With Takumi | Saved | % |\n")
		sb.WriteString("|--------|---------------|-------------|-------|---|\n")
		sb.WriteString(fmt.Sprintf("| **Tokens** | %s | %s | %s | **%+.1f%%** |\n",
			fmtInt(w.Tokens), fmtInt(t.Tokens), fmtInt(w.Tokens-t.Tokens), tokPct))
		sb.WriteString(fmt.Sprintf("| Turns | %d | %d | %d | |\n", w.Turns, t.Turns, w.Turns-t.Turns))
		sb.WriteString(fmt.Sprintf("| Tool calls | %d | %d | %d | |\n", w.Calls, t.Calls, w.Calls-t.Calls))
		sb.WriteString(fmt.Sprintf("| Errors | %d | %d | %d | |\n", w.Errors, t.Errors, w.Errors-t.Errors))
		sb.WriteString("\n")
	}

	// Per-scenario details
	scenarioNames := map[string]string{
		"fix-build-error":     "Fix Build Error",
		"scoped-rebuild":      "Scoped Rebuild",
		"understand-structure": "Understand Structure",
	}
	scenarioDescs := map[string]string{
		"fix-build-error":     "Find and fix a type error in a Go HTTP handler",
		"scoped-rebuild":      "After changing shared lib, build only affected packages",
		"understand-structure": "Explain dependency graph and build order of a 4-package monorepo",
	}

	for sid, modes := range results.Scenarios {
		name := scenarioNames[sid]
		if name == "" {
			name = sid
		}
		desc := scenarioDescs[sid]

		wo := modes["without_takumi"]
		wi := modes["with_takumi"]
		if wo.Error != "" || wi.Error != "" {
			continue
		}

		wTok := wo.InputTokens + wo.OutputTokens
		tTok := wi.InputTokens + wi.OutputTokens
		tokPct := float64(0)
		if wTok > 0 {
			tokPct = float64(wTok-tTok) / float64(wTok) * 100
		}
		timePct := float64(0)
		if wo.WallTimeS > 0 {
			timePct = (wo.WallTimeS - wi.WallTimeS) / wo.WallTimeS * 100
		}

		sb.WriteString(fmt.Sprintf("## %s\n\n", name))
		if desc != "" {
			sb.WriteString(fmt.Sprintf("> %s\n\n", desc))
		}

		sb.WriteString("| Metric | Without | With Takumi | Saved |\n")
		sb.WriteString("|--------|---------|-------------|-------|\n")
		sb.WriteString(fmt.Sprintf("| Tokens | %s | %s | **%+.1f%%** |\n", fmtInt(wTok), fmtInt(tTok), tokPct))
		sb.WriteString(fmt.Sprintf("| Input tokens | %s | %s | |\n", fmtInt(wo.InputTokens), fmtInt(wi.InputTokens)))
		sb.WriteString(fmt.Sprintf("| Output tokens | %s | %s | |\n", fmtInt(wo.OutputTokens), fmtInt(wi.OutputTokens)))
		sb.WriteString(fmt.Sprintf("| Time | %.1fs | %.1fs | **%+.1f%%** |\n", wo.WallTimeS, wi.WallTimeS, timePct))
		sb.WriteString(fmt.Sprintf("| Turns | %d | %d | %d |\n", wo.Turns, wi.Turns, wo.Turns-wi.Turns))
		sb.WriteString(fmt.Sprintf("| Tool calls | %d | %d | %d |\n", wo.ToolCalls, wi.ToolCalls, wo.ToolCalls-wi.ToolCalls))
		sb.WriteString(fmt.Sprintf("| Errors | %d | %d | %d |\n", wo.Errors, wi.Errors, wo.Errors-wi.Errors))
		sb.WriteString(fmt.Sprintf("| Completed | %s | %s | |\n", fmtBool(wo.TaskCompleted), fmtBool(wi.TaskCompleted)))
		sb.WriteString(fmt.Sprintf("| Verified | %s | %s | |\n", fmtBool(wo.Success), fmtBool(wi.Success)))
		sb.WriteString("\n")

		// Include logs for this scenario
		for _, tag := range []string{"without", "with-takumi"} {
			logName := fmt.Sprintf("%s.%s.log", sid, tag)
			if content, ok := logs[logName]; ok {
				label := "Without Takumi"
				if tag == "with-takumi" {
					label = "With Takumi"
				}
				sb.WriteString(fmt.Sprintf("<details>\n<summary>%s — full transcript</summary>\n\n", label))
				sb.WriteString("```\n")
				sb.WriteString(content)
				sb.WriteString("```\n\n")
				sb.WriteString("</details>\n\n")
			}
		}
	}

	sb.WriteString("---\n\n")
	sb.WriteString("*Generated by `takumi benchmark --publish`*\n")

	return sb.String()
}

func fmtInt(n int) string {
	if n < 0 {
		return fmt.Sprintf("-%s", fmtInt(-n))
	}
	s := fmt.Sprintf("%d", n)
	if len(s) <= 3 {
		return s
	}
	// Insert commas
	var result strings.Builder
	for i, c := range s {
		if i > 0 && (len(s)-i)%3 == 0 {
			result.WriteByte(',')
		}
		result.WriteRune(c)
	}
	return result.String()
}

func fmtBool(b bool) string {
	if b {
		return "yes"
	}
	return "no"
}

func findBenchmarkScript(binaryDir string) string {
	// Check env override
	if p := os.Getenv("BENCH_SCRIPT"); p != "" {
		return p
	}

	// Check relative to binary
	candidates := []string{
		filepath.Join(binaryDir, "..", "tests", "benchmark", "perf", "benchmark.py"),
		filepath.Join(binaryDir, "..", "..", "tests", "benchmark", "perf", "benchmark.py"),
	}

	// Check relative to cwd
	cwd, _ := os.Getwd()
	if cwd != "" {
		candidates = append(candidates,
			filepath.Join(cwd, "tests", "benchmark", "perf", "benchmark.py"),
		)
	}

	for _, p := range candidates {
		abs, err := filepath.Abs(p)
		if err != nil {
			continue
		}
		if _, err := os.Stat(abs); err == nil {
			return abs
		}
	}
	return ""
}

func loadDotEnv() {
	// Look for .env in cwd and up to workspace root
	candidates := []string{".env"}
	if ws, _ := loadWorkspace(); ws != nil {
		candidates = append(candidates, filepath.Join(ws.Root, ".env"))
	}

	for _, path := range candidates {
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		for _, line := range strings.Split(string(data), "\n") {
			line = strings.TrimSpace(line)
			if line == "" || strings.HasPrefix(line, "#") {
				continue
			}
			if k, v, ok := strings.Cut(line, "="); ok {
				k = strings.TrimSpace(k)
				v = strings.TrimSpace(v)
				if os.Getenv(k) == "" { // don't override existing env
					os.Setenv(k, v)
				}
			}
		}
		break // only load first found
	}
}

func findPython() string {
	// Check if python3 has anthropic
	for _, py := range []string{"python3", "python3.12", "python3.11", "python3.10"} {
		cmd := exec.Command(py, "-c", "import anthropic")
		if cmd.Run() == nil {
			return py
		}
	}
	return ""
}
