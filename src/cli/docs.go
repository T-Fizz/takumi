package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	"github.com/tfitz/takumi/src/ui"
)

func init() {
	docsGenerateCmd.Flags().Bool("ai", false, "Also run AI doc-writer skill")
	docsCmd.AddCommand(docsGenerateCmd, docsHookCmd)
	docsHookCmd.AddCommand(docsHookInstallCmd, docsHookRemoveCmd)
	rootCmd.AddCommand(docsCmd)
}

var docsCmd = &cobra.Command{
	Use:   "docs",
	Short: "Auto-generated documentation",
}

var docsGenerateCmd = &cobra.Command{
	Use:   "generate",
	Short: "Regenerate docs from code and config",
	Long: `Extract documentation from Cobra commands, config structs, and skill definitions.
Use --ai to also run the doc-writer skill for enhanced generation.`,
	RunE: runDocsGenerate,
}

var docsHookCmd = &cobra.Command{
	Use:   "hook",
	Short: "Manage git pre-commit hook for doc generation",
}

var docsHookInstallCmd = &cobra.Command{
	Use:   "install",
	Short: "Install pre-commit hook that regenerates docs",
	RunE:  runDocsHookInstall,
}

var docsHookRemoveCmd = &cobra.Command{
	Use:   "remove",
	Short: "Remove the pre-commit hook",
	RunE:  runDocsHookRemove,
}

func runDocsGenerate(cmd *cobra.Command, args []string) error {
	ws := requireWorkspace()
	aiFlag, _ := cmd.Flags().GetBool("ai")

	docsDir := filepath.Join(ws.Root, "docs")
	userDir := filepath.Join(docsDir, "user")
	if err := os.MkdirAll(userDir, 0755); err != nil {
		return fmt.Errorf("creating docs directory: %w", err)
	}

	fmt.Println()
	fmt.Println(ui.SectionHeader.Render("Generating Documentation"))
	fmt.Println()

	// 1. Commands reference
	var cmdBuf strings.Builder
	cmdBuf.WriteString("# Takumi Commands Reference\n\n")
	cmdBuf.WriteString("Auto-generated from CLI definitions.\n\n")
	writeCommandDocs(&cmdBuf, rootCmd, "")

	cmdPath := filepath.Join(userDir, "commands.md")
	if err := os.WriteFile(cmdPath, []byte(cmdBuf.String()), 0644); err != nil {
		return fmt.Errorf("writing commands.md: %w", err)
	}
	fmt.Println(ui.StepDone("Generated " + ui.FilePath("docs/user/commands.md")))

	// 2. Skills reference
	allSkills, _ := loadAllSkills()
	var skillBuf strings.Builder
	skillBuf.WriteString("# Takumi Skills Reference\n\n")
	skillBuf.WriteString("Auto-generated from built-in skill definitions.\n\n")
	for _, s := range allSkills {
		fmt.Fprintf(&skillBuf, "## %s\n\n", s.Name)
		fmt.Fprintf(&skillBuf, "%s\n\n", s.Description)
		if len(s.AutoContext) > 0 {
			skillBuf.WriteString("**Auto-context:** ")
			skillBuf.WriteString(strings.Join(s.AutoContext, ", "))
			skillBuf.WriteString("\n\n")
		}
		if s.MaxTokens > 0 {
			fmt.Fprintf(&skillBuf, "**Max tokens:** %d\n\n", s.MaxTokens)
		}
		skillBuf.WriteString("---\n\n")
	}

	skillPath := filepath.Join(userDir, "skills-reference.md")
	if err := os.WriteFile(skillPath, []byte(skillBuf.String()), 0644); err != nil {
		return fmt.Errorf("writing skills-reference.md: %w", err)
	}
	fmt.Println(ui.StepDone("Generated " + ui.FilePath("docs/user/skills-reference.md")))

	// 3. Config reference
	var cfgBuf strings.Builder
	cfgBuf.WriteString("# Takumi Config Reference\n\n")
	cfgBuf.WriteString("Auto-generated from config structures.\n\n")

	cfgBuf.WriteString("## takumi.yaml (Workspace Config)\n\n")
	cfgBuf.WriteString("```yaml\nworkspace:\n")
	cfgBuf.WriteString("  name: <string>          # Workspace name\n")
	cfgBuf.WriteString("  ignore:                  # Directories to skip during package scan\n")
	cfgBuf.WriteString("    - vendor/\n")
	cfgBuf.WriteString("  sources:                 # Tracked git repos\n")
	cfgBuf.WriteString("    <name>:\n")
	cfgBuf.WriteString("      url: <string>        # Git clone URL\n")
	cfgBuf.WriteString("      branch: <string>     # Branch to track\n")
	cfgBuf.WriteString("      path: <string>       # Local path\n")
	cfgBuf.WriteString("  version-set:\n")
	cfgBuf.WriteString("    file: <string>         # Path to takumi-versions.yaml\n")
	cfgBuf.WriteString("  settings:\n")
	cfgBuf.WriteString("    parallel: <bool>       # Enable parallel builds (default: true)\n")
	cfgBuf.WriteString("  ai:\n")
	cfgBuf.WriteString("    agent: <string>        # AI agent (claude, cursor, copilot, windsurf, cline)\n")
	cfgBuf.WriteString("    instructions: <string> # Path to takumi-ai.yaml\n")
	cfgBuf.WriteString("```\n\n")

	cfgBuf.WriteString("## takumi-pkg.yaml (Package Config)\n\n")
	cfgBuf.WriteString("```yaml\npackage:\n")
	cfgBuf.WriteString("  name: <string>           # Package name\n")
	cfgBuf.WriteString("  version: <string>        # Semver version\n")
	cfgBuf.WriteString("dependencies:              # Other takumi packages this depends on\n")
	cfgBuf.WriteString("  - <package-name>\n")
	cfgBuf.WriteString("runtime:                   # Optional: isolated runtime environment\n")
	cfgBuf.WriteString("  setup:                   # Commands to create the env\n")
	cfgBuf.WriteString("    - <command>\n")
	cfgBuf.WriteString("  env:                     # Env vars injected into all commands\n")
	cfgBuf.WriteString("    KEY: VALUE\n")
	cfgBuf.WriteString("phases:                    # Build phases\n")
	cfgBuf.WriteString("  build:\n")
	cfgBuf.WriteString("    pre: [<command>]\n")
	cfgBuf.WriteString("    commands: [<command>]\n")
	cfgBuf.WriteString("    post: [<command>]\n")
	cfgBuf.WriteString("```\n\n")

	cfgPath := filepath.Join(userDir, "config-reference.md")
	if err := os.WriteFile(cfgPath, []byte(cfgBuf.String()), 0644); err != nil {
		return fmt.Errorf("writing config-reference.md: %w", err)
	}
	fmt.Println(ui.StepDone("Generated " + ui.FilePath("docs/user/config-reference.md")))

	// 4. Package map
	var pkgBuf strings.Builder
	pkgBuf.WriteString("# Package Map\n\n")
	pkgBuf.WriteString("Auto-generated from workspace scan.\n\n")

	var names []string
	for name := range ws.Packages {
		names = append(names, name)
	}
	sort.Strings(names)

	pkgBuf.WriteString("| Package | Version | Dependencies | Phases | Runtime |\n")
	pkgBuf.WriteString("|---------|---------|-------------|--------|---------|\n")
	for _, name := range names {
		pkg := ws.Packages[name]
		deps := strings.Join(pkg.Config.Dependencies, ", ")
		if deps == "" {
			deps = "—"
		}
		var phases []string
		for p := range pkg.Config.Phases {
			phases = append(phases, p)
		}
		sort.Strings(phases)
		phaseStr := strings.Join(phases, ", ")
		if phaseStr == "" {
			phaseStr = "—"
		}
		runtime := "no"
		if pkg.Config.Runtime != nil {
			runtime = "yes"
		}
		fmt.Fprintf(&pkgBuf, "| %s | %s | %s | %s | %s |\n",
			name, pkg.Config.Package.Version, deps, phaseStr, runtime)
	}

	pkgPath := filepath.Join(userDir, "packages.md")
	if err := os.WriteFile(pkgPath, []byte(pkgBuf.String()), 0644); err != nil {
		return fmt.Errorf("writing packages.md: %w", err)
	}
	fmt.Println(ui.StepDone("Generated " + ui.FilePath("docs/user/packages.md")))

	if aiFlag {
		fmt.Println()
		fmt.Println(ui.StepInfo("Running doc-writer skill..."))
		return runAISkillRun(cmd, []string{"doc-writer"})
	}

	fmt.Println()
	return nil
}

func writeCommandDocs(buf *strings.Builder, cmd *cobra.Command, prefix string) {
	if cmd.Hidden {
		return
	}

	name := cmd.Name()
	if prefix != "" {
		name = prefix + " " + name
	}

	if cmd.Runnable() && name != "takumi" {
		fmt.Fprintf(buf, "## `%s`\n\n", name)
		fmt.Fprintf(buf, "%s\n\n", cmd.Short)
		fmt.Fprintf(buf, "**Usage:** `%s`\n\n", cmd.UseLine())

		if cmd.Long != "" {
			fmt.Fprintf(buf, "%s\n\n", cmd.Long)
		}

		flags := cmd.NonInheritedFlags()
		if flags.HasFlags() {
			buf.WriteString("**Flags:**\n\n")
			flags.VisitAll(func(f *pflag.Flag) {
				fmt.Fprintf(buf, "- `--%s` — %s (default: %s)\n", f.Name, f.Usage, f.DefValue)
			})
			buf.WriteString("\n")
		}

		buf.WriteString("---\n\n")
	}

	for _, sub := range cmd.Commands() {
		writeCommandDocs(buf, sub, name)
	}
}

const hookScript = `#!/bin/sh
# Takumi pre-commit hook — regenerate docs and auto-stage changes
takumi docs generate
git add docs/ 2>/dev/null || true
`

func runDocsHookInstall(cmd *cobra.Command, args []string) error {
	ws := requireWorkspace()

	hookDir := filepath.Join(ws.Root, ".git", "hooks")
	if _, err := os.Stat(filepath.Join(ws.Root, ".git")); os.IsNotExist(err) {
		return fmt.Errorf("not a git repository — cannot install hook")
	}

	os.MkdirAll(hookDir, 0755)
	hookPath := filepath.Join(hookDir, "pre-commit")

	if err := os.WriteFile(hookPath, []byte(hookScript), 0755); err != nil {
		return fmt.Errorf("writing hook: %w", err)
	}

	fmt.Println(ui.StepDone("Installed pre-commit hook"))
	fmt.Println(ui.StepInfo("Docs will auto-regenerate on every commit"))
	return nil
}

func runDocsHookRemove(cmd *cobra.Command, args []string) error {
	ws := requireWorkspace()

	hookPath := filepath.Join(ws.Root, ".git", "hooks", "pre-commit")
	if err := os.Remove(hookPath); err != nil {
		if os.IsNotExist(err) {
			fmt.Println(ui.Muted.Render("No pre-commit hook found"))
			return nil
		}
		return fmt.Errorf("removing hook: %w", err)
	}

	fmt.Println(ui.StepDone("Removed pre-commit hook"))
	return nil
}
