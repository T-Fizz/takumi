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

## Commands (use these, don't guess)

| Command | Purpose |
|---------|---------|
| takumi status | Check workspace health (ALWAYS run first) |
| takumi affected | What packages changed? (scope your work) |
| takumi build | Build packages (deps auto-resolved) |
| takumi test | Run tests (use --affected to skip unchanged) |
| takumi env setup | Fix environment issues |
| takumi graph | See dependency order |
| takumi ai diagnose | Auto-triage any failure |

## Workflow

1. `+"`takumi status`"+` — understand state
2. `+"`takumi affected --since main`"+` — scope changes
3. `+"`takumi build --affected`"+` — build only what changed
4. `+"`takumi test --affected`"+` — test only what changed
5. On failure → `+"`takumi ai diagnose`"+` → read output → fix → repeat from 3

## Config locations

| File | Purpose |
|------|---------|
| takumi.yaml | Workspace config |
| takumi-pkg.yaml | Package config (one per package) |
| takumi-versions.yaml | Version pinning |
| .takumi/ai-context.md | Auto-generated AI context |

## Rules

- Never install dependencies globally — takumi manages isolated envs per package
- Use `+"`takumi checkout <url>`"+` to add repos, not git clone
- Use `+"`takumi remove <pkg>`"+` to remove packages, not rm -rf
- Check .takumi/ai-context.md for the full workspace map
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
