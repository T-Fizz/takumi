package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/charmbracelet/huh"
	"github.com/tfitz/takumi/src/ui"
)

// AgentType identifies a supported AI agent.
type AgentType struct {
	Name     string // key used in --agent flag and takumi.yaml
	Label    string // display name for menu
	FilePath string // config file path relative to workspace root
}

// SupportedAgents lists all AI agents Takumi can onboard.
var SupportedAgents = []AgentType{
	{Name: "claude", Label: "Claude Code", FilePath: "CLAUDE.md"},
	{Name: "cursor", Label: "Cursor", FilePath: ".cursor/rules"},
	{Name: "copilot", Label: "GitHub Copilot", FilePath: ".github/copilot-instructions.md"},
	{Name: "windsurf", Label: "Windsurf", FilePath: ".windsurfrules"},
	{Name: "cline", Label: "Cline", FilePath: ".clinerules"},
	{Name: "kiro", Label: "Kiro", FilePath: "AGENTS.md"},
	{Name: "none", Label: "Skip (no AI agent)", FilePath: ""},
}

// AgentByName returns the AgentType for the given name, or nil if not found.
func AgentByName(name string) *AgentType {
	for i := range SupportedAgents {
		if SupportedAgents[i].Name == name {
			return &SupportedAgents[i]
		}
	}
	return nil
}

// agentNames returns all valid agent names for flag help text.
func agentNames() string {
	names := make([]string, len(SupportedAgents))
	for i, a := range SupportedAgents {
		names[i] = a.Name
	}
	return strings.Join(names, ", ")
}

// promptAgentSelection shows an interactive select menu and returns the pick.
// Replaced in tests with a mock.
var promptAgentSelection = func() (*AgentType, error) {
	options := make([]huh.Option[string], len(SupportedAgents))
	for i, a := range SupportedAgents {
		options[i] = huh.NewOption(a.Label, a.Name)
	}

	var selected string
	err := huh.NewSelect[string]().
		Title("Select your AI agent").
		Description("Takumi will create the config file for your chosen agent.").
		Options(options...).
		Value(&selected).
		Run()

	if err != nil {
		return nil, fmt.Errorf("agent selection: %w", err)
	}

	return AgentByName(selected), nil
}

// includeLine is what gets appended to the agent's config file.
const includeLine = "Read .takumi/TAKUMI.md for Takumi build tool instructions."

// setupAgentConfig creates the agent's config file with an include line pointing
// to .takumi/TAKUMI.md. If the file already exists, appends the include line
// only if it's not already present.
func setupAgentConfig(wsRoot string, agent *AgentType) error {
	if agent.FilePath == "" {
		return nil // "none" selected
	}

	filePath := filepath.Join(wsRoot, agent.FilePath)

	// Ensure parent directory exists
	dir := filepath.Dir(filePath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("creating directory %s: %w", dir, err)
	}

	// Check if file exists and already has the include line
	if data, err := os.ReadFile(filePath); err == nil {
		if strings.Contains(string(data), includeLine) {
			return nil // already set up
		}
		// Append include line
		f, err := os.OpenFile(filePath, os.O_APPEND|os.O_WRONLY, 0644)
		if err != nil {
			return fmt.Errorf("opening %s: %w", agent.FilePath, err)
		}
		defer f.Close()
		_, err = fmt.Fprintf(f, "\n%s\n", includeLine)
		return err
	}

	// Create new file with include line
	return os.WriteFile(filePath, []byte(includeLine+"\n"), 0644)
}

// takumiMDContent returns the content for .takumi/TAKUMI.md.
func takumiMDContent(wsName string) string {
	return fmt.Sprintf(`# Takumi (匠) — Workspace: %s

You are working in a Takumi workspace — an AI-aware, language-agnostic package builder.

## Commands (use these instead of raw shell commands)

| Command | Purpose |
|---------|---------|
| takumi status | Workspace health dashboard (ALWAYS run first in a new session) |
| takumi build | Build packages in dependency order (not go build, npm run build, etc.) |
| takumi test | Run tests in dependency order (not pytest, go test, vitest, etc.) |
| takumi run <phase> | Run any custom phase: deploy, lint, dev, etc. (not raw commands) |
| takumi affected | List packages affected by changes (scope your work before building) |
| takumi graph | Dependency DAG with topological levels (not grep for imports) |
| takumi validate | Check all configs for errors and cycles (run after editing configs) |
| takumi env setup | Install dependencies and set up isolated runtime environments |
| takumi review | Run AI code review of workspace changes |
| takumi init | Scaffold a new workspace or package config |

## Workflow

1. `+"`takumi status`"+` — understand workspace state and packages
2. `+"`takumi affected --since main`"+` — scope what changed
3. `+"`takumi build --affected`"+` — build only what changed
4. `+"`takumi test --affected`"+` — test only what changed
5. On failure → read logs in .takumi/logs/ → fix → repeat from 3

## When NOT to use raw commands

- See go.mod/package.json/pytest.ini? Use takumi build / takumi test, not language tools
- Need to deploy? Use takumi run deploy, not fly deploy / vercel deploy
- Need to lint? Use takumi run lint, not eslint / ruff
- Need to install dependencies for an interpreted language (Python, Node, Ruby)? Edit the manifest, then takumi env setup to sync into the managed env. Prefer this over raw pip install / npm install
- New source directory? Create a takumi-pkg.yaml before building, or run takumi init
- Changed a config? Run takumi validate to check for errors
- Adding a dependency between packages? Add to dependencies in takumi-pkg.yaml, verify with takumi validate

## When raw tools ARE appropriate

- Interactive work: REPLs (python, node), debuggers (dlv, pdb), one-off scripts
- Git operations: commits, branches, merges — VCS is outside Takumi's scope
- User explicitly requests a specific raw command with custom flags
- No takumi.yaml exists yet — raw tools until takumi init is run

## Config locations

| File | Purpose |
|------|---------|
| takumi.yaml | Workspace config |
| takumi-pkg.yaml | Package config (one per package directory) |
| takumi-versions.yaml | Version pinning |
| .takumi/TAKUMI.md | AI context (this file) |

## Rules

- For interpreted languages (Python, Node, Ruby), prefer takumi env setup over raw pip install / npm install — Takumi manages isolated envs in .takumi/envs/<pkg>/. Compiled languages (Go, Rust, C) fetch deps at build time and don't need this.
- Never build/test with raw language commands — takumi handles dependency order and caching
- Use `+"`takumi checkout <url>`"+` to add repos, not git clone
- Use `+"`takumi remove <pkg>`"+` to remove packages, not rm -rf
- Check `+"`takumi status`"+` at the start of every new session
- After modifying source, check `+"`takumi affected`"+` before building everything
`, wsName)
}

// writeTakumiMD writes .takumi/TAKUMI.md in the workspace.
func writeTakumiMD(wsRoot, wsName string) error {
	path := filepath.Join(wsRoot, ".takumi", "TAKUMI.md")
	if err := os.WriteFile(path, []byte(takumiMDContent(wsName)), 0644); err != nil {
		return err
	}
	fmt.Println(ui.StepDone("Created " + ui.FilePath(".takumi/TAKUMI.md")))
	return nil
}
