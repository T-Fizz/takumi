package cli

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"github.com/tfitz/takumi/src/ui"
)

var reviewOutput string
var reviewModel string
var reviewBase string
var reviewProvider string

func init() {
	reviewCmd.Flags().StringVarP(&reviewOutput, "output", "o", "", "write review to file (default: .takumi/reviews/<timestamp>.md)")
	reviewCmd.Flags().StringVar(&reviewModel, "model", "", "LLM model (default: auto-detected from provider)")
	reviewCmd.Flags().StringVar(&reviewBase, "base", "HEAD", "base ref for diff (e.g. main, HEAD~3)")
	reviewCmd.Flags().StringVar(&reviewProvider, "provider", "", "LLM provider: anthropic, openai (default: auto-detected from env)")
	rootCmd.AddCommand(reviewCmd)
}

var reviewCmd = &cobra.Command{
	Use:   "review",
	Short: "Run a thorough code review of all workspace changes",
	Long: `Analyze all uncommitted or branched changes in the workspace and produce
a detailed code review document. Checks for bugs, logic errors, style issues,
missing tests, security concerns, and nits. Writes findings to a markdown file.

Supports multiple LLM providers. Set one of:
  ANTHROPIC_API_KEY  — uses Anthropic (Claude models)
  OPENAI_API_KEY     — uses OpenAI (GPT models)

Or specify explicitly with --provider and --model.`,
	RunE: runReview,
}

// llmProvider holds resolved provider config
type llmProvider struct {
	Name   string
	APIKey string
	Model  string
}

func detectProvider() (*llmProvider, error) {
	// Explicit flag takes priority
	if reviewProvider != "" {
		switch reviewProvider {
		case "anthropic":
			key := os.Getenv("ANTHROPIC_API_KEY")
			if key == "" {
				return nil, fmt.Errorf("ANTHROPIC_API_KEY not set")
			}
			model := reviewModel
			if model == "" {
				model = "claude-sonnet-4-5-20250514"
			}
			return &llmProvider{Name: "anthropic", APIKey: key, Model: model}, nil
		case "openai":
			key := os.Getenv("OPENAI_API_KEY")
			if key == "" {
				return nil, fmt.Errorf("OPENAI_API_KEY not set")
			}
			model := reviewModel
			if model == "" {
				model = "gpt-4o"
			}
			return &llmProvider{Name: "openai", APIKey: key, Model: model}, nil
		default:
			return nil, fmt.Errorf("unknown provider %q (supported: anthropic, openai)", reviewProvider)
		}
	}

	// Auto-detect from env
	if key := os.Getenv("ANTHROPIC_API_KEY"); key != "" {
		model := reviewModel
		if model == "" {
			model = "claude-sonnet-4-5-20250514"
		}
		return &llmProvider{Name: "anthropic", APIKey: key, Model: model}, nil
	}
	if key := os.Getenv("OPENAI_API_KEY"); key != "" {
		model := reviewModel
		if model == "" {
			model = "gpt-4o"
		}
		return &llmProvider{Name: "openai", APIKey: key, Model: model}, nil
	}

	return nil, fmt.Errorf("no LLM API key found. Set ANTHROPIC_API_KEY or OPENAI_API_KEY in env or .env")
}

func runReview(cmd *cobra.Command, args []string) error {
	loadDotEnv()

	provider, err := detectProvider()
	if err != nil {
		return err
	}

	ws := requireWorkspace()

	fmt.Println()
	fmt.Println(ui.SectionHeader.Render("Code Review"))
	fmt.Println()

	// Gather context
	fmt.Println(ui.StepInfo("Gathering changes..."))

	diff := reviewDiff(ws.Root, reviewBase)
	if strings.TrimSpace(diff) == "" {
		fmt.Println(ui.StepDone("No changes to review"))
		return nil
	}

	diffStat := reviewDiffStat(ws.Root, reviewBase)
	changedFiles, _ := gitChangedFiles(ws.Root, reviewBase)
	affected := mapFilesToPackages(ws, changedFiles)

	var pkgNames []string
	for name := range affected {
		pkgNames = append(pkgNames, name)
	}
	sort.Strings(pkgNames)

	// Build package context
	var pkgContext strings.Builder
	for _, name := range pkgNames {
		pkg := ws.Packages[name]
		pkgContext.WriteString(fmt.Sprintf("- **%s** v%s", name, pkg.Config.Package.Version))
		if len(pkg.Config.Dependencies) > 0 {
			pkgContext.WriteString(fmt.Sprintf(" (deps: %s)", strings.Join(pkg.Config.Dependencies, ", ")))
		}
		if pkg.Config.AI != nil && pkg.Config.AI.Description != "" {
			pkgContext.WriteString(fmt.Sprintf(" — %s", pkg.Config.AI.Description))
		}
		pkgContext.WriteString("\n")
	}

	// Read full file contents for changed files (up to reasonable size)
	var fileContents strings.Builder
	totalSize := 0
	maxSize := 100_000 // 100KB cap on file contents
	for _, f := range changedFiles {
		if totalSize >= maxSize {
			fileContents.WriteString(fmt.Sprintf("\n--- (remaining files truncated, %d files total) ---\n", len(changedFiles)))
			break
		}
		absPath := filepath.Join(ws.Root, f)
		data, err := os.ReadFile(absPath)
		if err != nil {
			continue
		}
		// Skip binary files
		if isBinary(data) {
			continue
		}
		chunk := string(data)
		if totalSize+len(chunk) > maxSize {
			chunk = chunk[:maxSize-totalSize] + "\n... (truncated)"
		}
		fileContents.WriteString(fmt.Sprintf("\n=== %s ===\n%s\n", f, chunk))
		totalSize += len(chunk)
	}

	prompt := buildReviewPrompt(diff, diffStat, pkgContext.String(), fileContents.String(), changedFiles)

	fmt.Println(ui.StepInfo(fmt.Sprintf("Reviewing %d files across %d packages...", len(changedFiles), len(pkgNames))))

	// Call LLM
	review, err := callLLM(provider, prompt)
	if err != nil {
		return fmt.Errorf("review failed: %w", err)
	}

	// Determine output path
	outPath := reviewOutput
	if outPath == "" {
		reviewDir := filepath.Join(ws.Root, ".takumi", "reviews")
		os.MkdirAll(reviewDir, 0o755)
		ts := time.Now().Format("2006-01-02-150405")
		outPath = filepath.Join(reviewDir, ts+".md")
	}

	// Write review
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

	if err := os.WriteFile(outPath, []byte(header+review), 0o644); err != nil {
		return fmt.Errorf("writing review: %w", err)
	}

	fmt.Println()
	fmt.Println(ui.StepDone("Review written to " + outPath))

	// Print summary (first section of the review)
	lines := strings.Split(review, "\n")
	summaryEnd := len(lines)
	for i, line := range lines {
		if i > 0 && strings.HasPrefix(line, "## ") {
			summaryEnd = i
			break
		}
	}
	if summaryEnd > 20 {
		summaryEnd = 20
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

func buildReviewPrompt(diff, diffStat, pkgContext, fileContents string, changedFiles []string) string {
	var sb strings.Builder

	sb.WriteString(`You are a senior software engineer performing a thorough code review. Review ALL changes with extreme attention to detail. Do not skip anything.

## Your review MUST cover every one of these categories:

### 1. Critical Issues (bugs, logic errors, data loss risks, security vulnerabilities)
### 2. Design & Architecture (coupling, separation of concerns, API design, naming)
### 3. Error Handling (missing checks, swallowed errors, unhelpful messages)
### 4. Testing (missing test coverage, edge cases not covered, brittle tests)
### 5. Performance (unnecessary allocations, N+1 queries, missing caching)
### 6. Style & Consistency (naming conventions, formatting, idioms for this language)
### 7. Documentation (missing/outdated comments, unclear function signatures)
### 8. Nits (typos, dead code, unnecessary imports, minor cleanup opportunities)

For each finding:
- Reference the exact file and line/section
- Explain what's wrong and why it matters
- Suggest a specific fix

Start with a brief summary (2-3 sentences), then go through each category. If a category has no findings, say "No issues found." Do NOT skip categories.

Be thorough. Flag everything — even minor nits. A clean review says "no issues" per category, not nothing at all.

`)

	sb.WriteString("## Changed Files\n\n")
	sb.WriteString("```\n")
	sb.WriteString(diffStat)
	sb.WriteString("```\n\n")

	if pkgContext != "" {
		sb.WriteString("## Affected Packages\n\n")
		sb.WriteString(pkgContext)
		sb.WriteString("\n")
	}

	sb.WriteString("## Diff\n\n")
	sb.WriteString("```diff\n")
	// Cap diff at ~80k chars to leave room for file contents
	if len(diff) > 80_000 {
		sb.WriteString(diff[:80_000])
		sb.WriteString("\n... (diff truncated)\n")
	} else {
		sb.WriteString(diff)
	}
	sb.WriteString("```\n\n")

	if fileContents != "" {
		sb.WriteString("## Full File Contents (for context)\n\n")
		sb.WriteString("```\n")
		sb.WriteString(fileContents)
		sb.WriteString("```\n")
	}

	return sb.String()
}

// ---------------------------------------------------------------------------
// LLM provider abstraction — direct HTTP, no SDK dependencies
// ---------------------------------------------------------------------------

func callLLM(p *llmProvider, prompt string) (string, error) {
	switch p.Name {
	case "anthropic":
		return callAnthropic(p.APIKey, p.Model, prompt)
	case "openai":
		return callOpenAI(p.APIKey, p.Model, prompt)
	default:
		return "", fmt.Errorf("unsupported provider: %s", p.Name)
	}
}

// ── Anthropic ──────────────────────────────────────────────────────────

func callAnthropic(apiKey, model, prompt string) (string, error) {
	reqBody := map[string]any{
		"model":      model,
		"max_tokens": 8192,
		"messages":   []map[string]string{{"role": "user", "content": prompt}},
	}

	jsonData, err := json.Marshal(reqBody)
	if err != nil {
		return "", fmt.Errorf("marshaling request: %w", err)
	}

	req, err := http.NewRequest("POST", "https://api.anthropic.com/v1/messages", bytes.NewReader(jsonData))
	if err != nil {
		return "", fmt.Errorf("creating request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-api-key", apiKey)
	req.Header.Set("anthropic-version", "2023-06-01")

	client := &http.Client{Timeout: 5 * time.Minute}
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("API request failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("reading response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("anthropic API error %d: %s", resp.StatusCode, string(body))
	}

	var apiResp struct {
		Content []struct {
			Type string `json:"type"`
			Text string `json:"text"`
		} `json:"content"`
		Error *struct {
			Type    string `json:"type"`
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal(body, &apiResp); err != nil {
		return "", fmt.Errorf("parsing response: %w", err)
	}

	if apiResp.Error != nil {
		return "", fmt.Errorf("anthropic error: %s: %s", apiResp.Error.Type, apiResp.Error.Message)
	}

	var result strings.Builder
	for _, block := range apiResp.Content {
		if block.Type == "text" {
			result.WriteString(block.Text)
		}
	}

	return result.String(), nil
}

// ── OpenAI ─────────────────────────────────────────────────────────────

func callOpenAI(apiKey, model, prompt string) (string, error) {
	reqBody := map[string]any{
		"model":      model,
		"max_tokens": 8192,
		"messages":   []map[string]string{{"role": "user", "content": prompt}},
	}

	jsonData, err := json.Marshal(reqBody)
	if err != nil {
		return "", fmt.Errorf("marshaling request: %w", err)
	}

	req, err := http.NewRequest("POST", "https://api.openai.com/v1/chat/completions", bytes.NewReader(jsonData))
	if err != nil {
		return "", fmt.Errorf("creating request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+apiKey)

	client := &http.Client{Timeout: 5 * time.Minute}
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("API request failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("reading response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("openai API error %d: %s", resp.StatusCode, string(body))
	}

	var apiResp struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
		Error *struct {
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal(body, &apiResp); err != nil {
		return "", fmt.Errorf("parsing response: %w", err)
	}

	if apiResp.Error != nil {
		return "", fmt.Errorf("openai error: %s", apiResp.Error.Message)
	}

	if len(apiResp.Choices) == 0 {
		return "", fmt.Errorf("openai returned no choices")
	}

	return apiResp.Choices[0].Message.Content, nil
}

func reviewDiff(wsRoot, base string) string {
	// Try staged + unstaged diff against base
	cmd := exec.Command("git", "-C", wsRoot, "diff", base)
	out, err := cmd.Output()
	if err != nil {
		// Fall back to working tree diff
		cmd = exec.Command("git", "-C", wsRoot, "diff")
		out, _ = cmd.Output()
	}

	// Also include staged changes
	staged := exec.Command("git", "-C", wsRoot, "diff", "--cached")
	stagedOut, _ := staged.Output()

	combined := string(out) + string(stagedOut)
	return combined
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

func isBinary(data []byte) bool {
	// Check first 512 bytes for null bytes
	check := data
	if len(check) > 512 {
		check = check[:512]
	}
	for _, b := range check {
		if b == 0 {
			return true
		}
	}
	return false
}
