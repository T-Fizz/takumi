package cli

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"github.com/tfitz/takumi/src/agent"
	"github.com/tfitz/takumi/src/ui"
	"github.com/tfitz/takumi/src/workspace"
)

var reviewOutput string
var reviewModel string
var reviewBase string
var reviewProvider string
var reviewMaxTurns int

func init() {
	reviewCmd.Flags().StringVarP(&reviewOutput, "output", "o", "", "write review to file (default: .takumi/reviews/<timestamp>.md)")
	reviewCmd.Flags().StringVar(&reviewModel, "model", "", "LLM model (default: auto-detected from provider)")
	reviewCmd.Flags().StringVar(&reviewBase, "base", "HEAD", "base ref for diff (e.g. main, HEAD~3)")
	reviewCmd.Flags().StringVar(&reviewProvider, "provider", "", "LLM provider: anthropic, openai (default: auto-detected from env)")
	reviewCmd.Flags().IntVar(&reviewMaxTurns, "max-turns", 20, "maximum agent turns")
	rootCmd.AddCommand(reviewCmd)
}

var reviewCmd = &cobra.Command{
	Use:   "review",
	Short: "Run a thorough code review of all workspace changes",
	Long: `Analyze all uncommitted or branched changes in the workspace using an AI
sub-agent. The agent can read files, run commands, and explore the codebase
to produce a thorough code review document.

Supports multiple LLM providers. Set one of:
  ANTHROPIC_API_KEY  — uses Anthropic (Claude models)
  OPENAI_API_KEY     — uses OpenAI (GPT models)

Or specify explicitly with --provider and --model.`,
	RunE: runReview,
}

// ---------------------------------------------------------------------------
// Review tools
// ---------------------------------------------------------------------------

func reviewTools(wsRoot string) []agent.Tool {
	return []agent.Tool{
		{
			Name:        "read_file",
			Description: "Read the full contents of a file in the workspace.",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"path": map[string]any{"type": "string", "description": "Path relative to workspace root"},
				},
				"required": []string{"path"},
			},
			Execute: func(input map[string]any) (string, bool) {
				path := fmt.Sprintf("%v", input["path"])
				data, err := os.ReadFile(filepath.Join(wsRoot, path))
				if err != nil {
					return fmt.Sprintf("Error: %v", err), true
				}
				if len(data) > 50_000 {
					return string(data[:50_000]) + "\n... (truncated at 50KB)", false
				}
				return string(data), false
			},
		},
		{
			Name:        "list_files",
			Description: "List files and directories at a path in the workspace.",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"path": map[string]any{"type": "string", "description": "Directory path relative to workspace root (default: .)"},
				},
			},
			Execute: func(input map[string]any) (string, bool) {
				path := "."
				if p, ok := input["path"]; ok && p != nil {
					path = fmt.Sprintf("%v", p)
				}
				entries, err := os.ReadDir(filepath.Join(wsRoot, path))
				if err != nil {
					return fmt.Sprintf("Error: %v", err), true
				}
				var lines []string
				for _, e := range entries {
					name := e.Name()
					if e.IsDir() {
						name += "/"
					}
					lines = append(lines, name)
				}
				return strings.Join(lines, "\n"), false
			},
		},
		{
			Name:        "run_command",
			Description: "Run a shell command in the workspace directory. Use for: running tests, checking build output, grepping for patterns, git log, etc.",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"command": map[string]any{"type": "string", "description": "Shell command to execute"},
				},
				"required": []string{"command"},
			},
			Execute: func(input map[string]any) (string, bool) {
				cmd := fmt.Sprintf("%v", input["command"])
				c := exec.Command("sh", "-c", cmd)
				c.Dir = wsRoot
				out, err := c.CombinedOutput()
				result := string(out)
				if err != nil {
					if result == "" {
						result = err.Error()
					}
					return result, true
				}
				if len(result) > 20_000 {
					result = result[:20_000] + "\n... (truncated)"
				}
				return result, false
			},
		},
		{
			Name:        "review_complete",
			Description: "Submit the final code review document. Call this when you have finished reviewing all changes.",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"review": map[string]any{"type": "string", "description": "The complete code review in markdown format"},
				},
				"required": []string{"review"},
			},
			Execute: func(input map[string]any) (string, bool) {
				return "Review submitted.", false
			},
		},
	}
}

// ---------------------------------------------------------------------------
// Review command
// ---------------------------------------------------------------------------

func runReview(cmd *cobra.Command, args []string) error {
	loadDotEnv()

	provider, err := agent.DetectProvider(reviewProvider, reviewModel)
	if err != nil {
		return err
	}

	ws := requireWorkspace()

	fmt.Println()
	fmt.Println(ui.SectionHeader.Render("Code Review"))
	fmt.Println()

	// Gather initial context for the agent
	fmt.Println(ui.StepInfo("Gathering changes..."))

	diff := reviewDiff(ws.Root, reviewBase)
	if strings.TrimSpace(diff) == "" {
		fmt.Println(ui.StepDone("No changes to review"))
		return nil
	}

	diffStat := reviewDiffStat(ws.Root, reviewBase)
	changedFiles, _ := workspace.ChangedFiles(ws.Root, reviewBase)
	affected := workspace.MapFilesToPackages(ws, changedFiles)

	var pkgNames []string
	for name := range affected {
		pkgNames = append(pkgNames, name)
	}
	sort.Strings(pkgNames)

	var pkgContext strings.Builder
	for _, name := range pkgNames {
		pkg := ws.Packages[name]
		pkgContext.WriteString(fmt.Sprintf("- %s v%s", name, pkg.Config.Package.Version))
		if len(pkg.Config.Dependencies) > 0 {
			pkgContext.WriteString(fmt.Sprintf(" (deps: %s)", strings.Join(pkg.Config.Dependencies, ", ")))
		}
		if pkg.Config.AI != nil && pkg.Config.AI.Description != "" {
			pkgContext.WriteString(fmt.Sprintf(" — %s", pkg.Config.AI.Description))
		}
		pkgContext.WriteString("\n")
	}

	// Build initial message with diff context
	var userMsg strings.Builder
	userMsg.WriteString("Review the following changes thoroughly. You have tools to read files, list directories, and run commands to investigate further.\n\n")
	userMsg.WriteString("## Changed Files\n\n```\n")
	userMsg.WriteString(diffStat)
	userMsg.WriteString("```\n\n")
	if pkgContext.Len() > 0 {
		userMsg.WriteString("## Affected Packages\n\n")
		userMsg.WriteString(pkgContext.String())
		userMsg.WriteString("\n")
	}
	userMsg.WriteString("## Diff\n\n```diff\n")
	if len(diff) > 80_000 {
		userMsg.WriteString(diff[:80_000])
		userMsg.WriteString("\n... (diff truncated, use read_file to see full files)\n")
	} else {
		userMsg.WriteString(diff)
	}
	userMsg.WriteString("```\n\n")
	userMsg.WriteString("Use read_file to examine the full contents of changed files. Use run_command to check tests, grep for patterns, or investigate anything suspicious. When you have completed your review, call review_complete with the full review document in markdown.\n")

	systemPrompt := `You are a senior software engineer performing a thorough code review. You have tools to explore the codebase.

Your workflow:
1. Read the diff to understand what changed
2. Use read_file to examine the full context of changed files
3. Use run_command to check for test coverage, grep for related patterns, etc.
4. Investigate anything suspicious — don't just review the diff in isolation

Your review MUST cover ALL of these categories:
1. Critical Issues (bugs, logic errors, data loss risks, security vulnerabilities)
2. Design & Architecture (coupling, separation of concerns, API design, naming)
3. Error Handling (missing checks, swallowed errors, unhelpful messages)
4. Testing (missing test coverage, edge cases not covered, brittle tests)
5. Performance (unnecessary allocations, N+1 queries, missing caching)
6. Style & Consistency (naming conventions, formatting, idioms for this language)
7. Documentation (missing/outdated comments, unclear function signatures)
8. Nits (typos, dead code, unnecessary imports, minor cleanup opportunities)

For each finding: reference the exact file and line, explain why it matters, suggest a fix.
If a category has no findings, say "No issues found."

When done, call review_complete with the full markdown review document.`

	fmt.Println(ui.StepInfo(fmt.Sprintf("Agent reviewing %d files across %d packages (%s)...",
		len(changedFiles), len(pkgNames), provider.Model)))
	fmt.Println()

	cfg := &agent.Config{
		SystemPrompt:   systemPrompt,
		Tools:          reviewTools(ws.Root),
		CompletionTool: "review_complete",
		MaxTurns:       reviewMaxTurns,
		MaxTokens:      8192,
		OnToolCall: func(name string, input map[string]any) {
			fmt.Printf("  [%s] %s\n", name, toolCallSummary(name, input))
		},
	}

	result, err := agent.Run(context.Background(), provider, cfg, userMsg.String())
	if err != nil {
		return fmt.Errorf("review failed: %w", err)
	}

	if result.Output == "" {
		return fmt.Errorf("agent did not produce a review (ran out of turns?)")
	}

	// Write review
	outPath := reviewOutput
	if outPath == "" {
		reviewDir := filepath.Join(ws.Root, ".takumi", "reviews")
		os.MkdirAll(reviewDir, 0o755)
		ts := time.Now().Format("2006-01-02-150405")
		outPath = filepath.Join(reviewDir, ts+".md")
	}

	header := fmt.Sprintf("# Code Review\n\n"+
		"> Generated: %s\n"+
		"> Base: `%s`\n"+
		"> Provider: `%s`\n"+
		"> Model: `%s`\n"+
		"> Files: %d | Packages: %s\n\n---\n\n",
		time.Now().Format("2006-01-02 15:04:05"),
		reviewBase,
		provider.Name,
		provider.Model,
		len(changedFiles),
		strings.Join(pkgNames, ", "),
	)

	if err := os.WriteFile(outPath, []byte(header+result.Output), 0o644); err != nil {
		return fmt.Errorf("writing review: %w", err)
	}

	fmt.Println()
	fmt.Println(ui.StepDone("Review written to " + outPath))

	// Print summary (first ~15 lines)
	lines := strings.Split(result.Output, "\n")
	summaryEnd := len(lines)
	for i, line := range lines {
		if i > 0 && strings.HasPrefix(line, "## ") {
			summaryEnd = i
			break
		}
	}
	if summaryEnd > 15 {
		summaryEnd = 15
	}
	fmt.Println()
	for _, line := range lines[:summaryEnd] {
		fmt.Println("  " + line)
	}
	if summaryEnd < len(lines) {
		fmt.Println()
		fmt.Println(ui.StepInfo(fmt.Sprintf("Full review: %s", outPath)))
	}

	return nil
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

func toolCallSummary(name string, input map[string]any) string {
	switch name {
	case "read_file":
		return fmt.Sprintf("%v", input["path"])
	case "list_files":
		if p, ok := input["path"]; ok {
			return fmt.Sprintf("%v", p)
		}
		return "."
	case "run_command":
		cmd := fmt.Sprintf("%v", input["command"])
		if len(cmd) > 60 {
			cmd = cmd[:57] + "..."
		}
		return cmd
	case "review_complete":
		return "(submitting review)"
	}
	return ""
}

func reviewDiff(wsRoot, base string) string {
	cmd := exec.Command("git", "-C", wsRoot, "diff", base)
	out, err := cmd.Output()
	if err != nil {
		cmd = exec.Command("git", "-C", wsRoot, "diff")
		out, _ = cmd.Output()
	}

	staged := exec.Command("git", "-C", wsRoot, "diff", "--cached")
	stagedOut, _ := staged.Output()

	return string(out) + string(stagedOut)
}

func reviewDiffStat(wsRoot, base string) string {
	cmd := exec.Command("git", "-C", wsRoot, "diff", "--stat", base)
	out, err := cmd.Output()
	if err != nil {
		cmd = exec.Command("git", "-C", wsRoot, "diff", "--stat")
		out, _ = cmd.Output()
	}
	return string(out)
}
