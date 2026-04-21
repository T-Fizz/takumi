package cli

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"

	"github.com/spf13/cobra"
	cobradoc "github.com/spf13/cobra/doc"
	"github.com/tfitz/takumi/src/ui"
)

func init() {
	docsCmd.AddCommand(docsGenerateCmd, docsCheckCmd, docsHookCmd)
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
	Long: `Extract documentation from Cobra commands, config structs, and workspace state.`,
	RunE: runDocsGenerate,
}

var docsCheckCmd = &cobra.Command{
	Use:   "check",
	Short: "Validate that generated docs are up-to-date with source code",
	Long: `Run docs generation into a temp directory and compare against existing docs.
Exits non-zero if any generated file differs from what's on disk.
Use in CI or pre-commit hooks to catch doc drift.`,
	RunE: runDocsCheck,
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

	docsDir := filepath.Join(ws.Root, "docs")
	userDir := filepath.Join(docsDir, "user")
	if err := os.MkdirAll(userDir, 0755); err != nil {
		return fmt.Errorf("creating docs directory: %w", err)
	}

	fmt.Println()
	fmt.Println(ui.SectionHeader.Render("Generating Documentation"))
	fmt.Println()

	// 1. Commands reference (via cobra/doc)
	cmdDir := filepath.Join(userDir, "commands")
	if err := os.MkdirAll(cmdDir, 0755); err != nil {
		return fmt.Errorf("creating commands dir: %w", err)
	}

	linkHandler := func(name string) string {
		// Convert "takumi_build.md" to "takumi_build.md" (keep flat)
		return name
	}
	filePrepender := func(filename string) string {
		return ""
	}
	if err := cobradoc.GenMarkdownTreeCustom(rootCmd, cmdDir, filePrepender, linkHandler); err != nil {
		return fmt.Errorf("generating command docs: %w", err)
	}

	// Also generate a single-file index
	var cmdBuf strings.Builder
	cmdBuf.WriteString("# Commands Reference\n\n")
	cmdBuf.WriteString("Auto-generated from CLI definitions via `cobra/doc`.\n\n")
	cmdBuf.WriteString("Individual command pages are in [commands/](commands/).\n\n")
	genCommandIndex(&cmdBuf, rootCmd, "")

	cmdPath := filepath.Join(userDir, "commands.md")
	if err := os.WriteFile(cmdPath, []byte(cmdBuf.String()), 0644); err != nil {
		return fmt.Errorf("writing commands.md: %w", err)
	}
	fmt.Println(ui.StepDone("Generated " + ui.FilePath("docs/user/commands/") + " (per-command pages)"))
	fmt.Println(ui.StepDone("Generated " + ui.FilePath("docs/user/commands.md") + " (index)"))

	// 2. Config reference
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
	cfgBuf.WriteString("    agent: <string>        # AI agent (claude, cursor, copilot, windsurf, cline, kiro)\n")
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

	// 5. Dev: package reference (from go doc)
	devDir := filepath.Join(docsDir, "dev")
	if err := os.MkdirAll(devDir, 0755); err != nil {
		return fmt.Errorf("creating dev docs directory: %w", err)
	}

	devPkgDoc, err := generateDevPackages(ws.Root)
	if err != nil {
		fmt.Println(ui.Warn("Skipping dev/packages.md: " + err.Error()))
	} else {
		devPkgPath := filepath.Join(devDir, "packages.md")
		if err := os.WriteFile(devPkgPath, []byte(devPkgDoc), 0644); err != nil {
			return fmt.Errorf("writing dev/packages.md: %w", err)
		}
		fmt.Println(ui.StepDone("Generated " + ui.FilePath("docs/dev/packages.md")))
	}

	// 6. Dev: architecture directory layout
	archPath := filepath.Join(devDir, "architecture.md")
	if _, err := os.Stat(archPath); err == nil {
		if updated, patchErr := patchArchitectureLayout(archPath, ws.Root); patchErr != nil {
			fmt.Println(ui.Warn("Skipping architecture.md layout update: " + patchErr.Error()))
		} else if updated {
			fmt.Println(ui.StepDone("Updated directory layout in " + ui.FilePath("docs/dev/architecture.md")))
		}
	}

	fmt.Println()
	return nil
}

// genCommandIndex writes a table-of-contents linking to per-command pages.
func genCommandIndex(buf *strings.Builder, cmd *cobra.Command, prefix string) {
	if cmd.Hidden {
		return
	}

	name := cmd.Name()
	if prefix != "" {
		name = prefix + " " + name
	}

	if cmd.Runnable() && name != "takumi" && cmd.Name() != "help" {
		// Link to the cobra/doc generated file
		filename := strings.ReplaceAll(name, " ", "_") + ".md"
		fmt.Fprintf(buf, "- [`%s`](commands/%s) — %s\n", name, filename, cmd.Short)
	}

	for _, sub := range cmd.Commands() {
		genCommandIndex(buf, sub, name)
	}
}

// ---------------------------------------------------------------------------
// Docs check (drift detection)
// ---------------------------------------------------------------------------

func runDocsCheck(cmd *cobra.Command, args []string) error {
	ws := requireWorkspace()

	docsDir := filepath.Join(ws.Root, "docs")

	// Generate into a temp directory
	tmpDir, err := os.MkdirTemp("", "takumi-docs-check-*")
	if err != nil {
		return fmt.Errorf("creating temp dir: %w", err)
	}
	defer os.RemoveAll(tmpDir)

	// We'll compare specific generated files
	type docFile struct {
		name     string
		generate func() (string, error)
	}

	files := []docFile{
		{
			name: "docs/user/commands.md",
			generate: func() (string, error) {
				var buf strings.Builder
				buf.WriteString("# Commands Reference\n\n")
				buf.WriteString("Auto-generated from CLI definitions via `cobra/doc`.\n\n")
				buf.WriteString("Individual command pages are in [commands/](commands/).\n\n")
				genCommandIndex(&buf, rootCmd, "")
				return buf.String(), nil
			},
		},
		{
			name: "docs/dev/packages.md",
			generate: func() (string, error) {
				return generateDevPackages(ws.Root)
			},
		},
	}

	fmt.Println()
	fmt.Println(ui.SectionHeader.Render("Checking Documentation"))
	fmt.Println()

	drifted := 0
	for _, f := range files {
		generated, err := f.generate()
		if err != nil {
			fmt.Println(ui.Warn(fmt.Sprintf("Skipping %s: %s", f.name, err)))
			continue
		}

		existing, err := os.ReadFile(filepath.Join(ws.Root, f.name))
		if os.IsNotExist(err) {
			fmt.Println(ui.Cross(f.name + " — missing (run takumi docs generate)"))
			drifted++
			continue
		} else if err != nil {
			fmt.Println(ui.Warn(fmt.Sprintf("Skipping %s: %s", f.name, err)))
			continue
		}

		if string(existing) != generated {
			fmt.Println(ui.Cross(f.name + " — out of date"))
			drifted++
		} else {
			fmt.Println(ui.Check(f.name + " — up to date"))
		}
	}

	// Check architecture.md layout section
	archPath := filepath.Join(docsDir, "dev", "architecture.md")
	if data, err := os.ReadFile(archPath); err == nil {
		content := string(data)
		startMarker := "## Directory Layout\n\n```\n"
		if idx := strings.Index(content, startMarker); idx >= 0 {
			blockStart := idx + len(startMarker)
			endMarker := "```\n"
			if blockEnd := strings.Index(content[blockStart:], endMarker); blockEnd >= 0 {
				currentLayout := content[blockStart : blockStart+blockEnd]
				expectedLayout := generateDirectoryLayout(ws.Root)
				if currentLayout != expectedLayout {
					fmt.Println(ui.Cross("docs/dev/architecture.md — directory layout out of date"))
					drifted++
				} else {
					fmt.Println(ui.Check("docs/dev/architecture.md — directory layout up to date"))
				}
			}
		}
	}

	fmt.Println()
	if drifted > 0 {
		return fmt.Errorf("%d file(s) out of date — run: takumi docs generate", drifted)
	}
	fmt.Println(ui.StepDone("All generated docs are up to date"))
	return nil
}

// ---------------------------------------------------------------------------
// Dev doc generators
// ---------------------------------------------------------------------------

// generateDevPackages produces docs/dev/packages.md from `go doc` output.
func generateDevPackages(wsRoot string) (string, error) {
	srcDir := filepath.Join(wsRoot, "src")
	entries, err := os.ReadDir(srcDir)
	if err != nil {
		return "", fmt.Errorf("reading src/: %w", err)
	}

	var pkgDirs []string
	for _, e := range entries {
		if e.IsDir() && e.Name() != "cli" {
			pkgDirs = append(pkgDirs, e.Name())
		}
	}
	sort.Strings(pkgDirs)

	var buf strings.Builder
	buf.WriteString("# Package Reference\n\n")
	buf.WriteString("Auto-generated from Go source via `takumi docs generate`.\n\n")
	buf.WriteString("Import path: `github.com/tfitz/takumi/src/<package>`.\n\n")

	for _, pkg := range pkgDirs {
		doc, err := goDocOutput(wsRoot, "./src/"+pkg+"/")
		if err != nil {
			continue
		}

		// Parse the go doc output
		lines := strings.Split(doc, "\n")
		if len(lines) == 0 {
			continue
		}

		// Extract package description (lines after "package <name>" until first blank or type)
		var desc []string
		var api []string
		inDesc := false
		for _, line := range lines {
			if strings.HasPrefix(line, "package ") {
				inDesc = true
				continue
			}
			if inDesc {
				if line == "" {
					inDesc = false
					continue
				}
				desc = append(desc, strings.TrimSpace(line))
			} else if line != "" {
				api = append(api, line)
			}
		}

		fmt.Fprintf(&buf, "## %s\n\n", pkg)
		if len(desc) > 0 {
			buf.WriteString(strings.Join(desc, " "))
			buf.WriteString("\n\n")
		}

		// List files in the package
		pkgFiles, _ := listGoFiles(filepath.Join(srcDir, pkg))
		if len(pkgFiles) > 0 {
			buf.WriteString("**Files:** ")
			buf.WriteString(strings.Join(pkgFiles, ", "))
			buf.WriteString("\n\n")
		}

		// Write API summary
		if len(api) > 0 {
			buf.WriteString("```\n")
			for _, line := range api {
				buf.WriteString(line)
				buf.WriteString("\n")
			}
			buf.WriteString("```\n\n")
		}

		buf.WriteString("---\n\n")
	}

	return buf.String(), nil
}

func goDocOutput(wsRoot, importPath string) (string, error) {
	cmd := exec.Command("go", "doc", importPath)
	cmd.Dir = wsRoot
	out, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return string(out), nil
}

func listGoFiles(dir string) ([]string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	var files []string
	for _, e := range entries {
		name := e.Name()
		if !e.IsDir() && strings.HasSuffix(name, ".go") && !strings.HasSuffix(name, "_test.go") {
			files = append(files, name)
		}
	}
	return files, nil
}

// patchArchitectureLayout updates the directory layout block in architecture.md.
func patchArchitectureLayout(archPath, wsRoot string) (bool, error) {
	data, err := os.ReadFile(archPath)
	if err != nil {
		return false, err
	}
	content := string(data)

	// Find the directory layout code block
	startMarker := "## Directory Layout\n\n```\n"
	endMarker := "```\n"

	startIdx := strings.Index(content, startMarker)
	if startIdx < 0 {
		return false, nil // no layout section to patch
	}

	blockStart := startIdx + len(startMarker)
	blockEnd := strings.Index(content[blockStart:], endMarker)
	if blockEnd < 0 {
		return false, nil
	}
	blockEnd += blockStart

	// Generate new layout
	newLayout := generateDirectoryLayout(wsRoot)

	// Check if anything changed
	oldLayout := content[blockStart:blockEnd]
	if oldLayout == newLayout {
		return false, nil
	}

	// Patch
	updated := content[:blockStart] + newLayout + content[blockEnd:]
	if err := os.WriteFile(archPath, []byte(updated), 0644); err != nil {
		return false, err
	}
	return true, nil
}

func generateDirectoryLayout(wsRoot string) string {
	var buf strings.Builder

	buf.WriteString("cmd/\n")
	buf.WriteString("  takumi/\n")
	buf.WriteString("    main.go                    # Entry point — calls cli.Execute()\n")
	buf.WriteString("\n")
	buf.WriteString("src/\n")

	srcDir := filepath.Join(wsRoot, "src")
	entries, err := os.ReadDir(srcDir)
	if err != nil {
		return buf.String()
	}

	// Descriptions for known packages
	descs := map[string]string{
		"agent":    "Multi-turn LLM agent loop (Anthropic, OpenAI)",
		"cache":    "Content-addressed build cache",
		"cli":      "Cobra commands (one per command group)",
		"config":   "YAML config parsing and validation",
		"executor": "Phase execution, parallelism, logging",
		"graph":    "Dependency DAG, topological sort",
		"mcp":      "MCP server (Model Context Protocol)",
		"ui":       "Terminal styling (lipgloss)",
		"workspace": "Workspace detection, package discovery, git utilities",
	}

	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		name := e.Name()
		desc := descs[name]
		if desc == "" {
			desc = name
		}

		// Check for subdirs
		subEntries, _ := os.ReadDir(filepath.Join(srcDir, name))
		var subdirs []string
		for _, sub := range subEntries {
			if sub.IsDir() {
				subdirs = append(subdirs, sub.Name())
			}
		}

		padding := strings.Repeat(" ", 27-len(name))
		fmt.Fprintf(&buf, "  %s/%s# %s\n", name, padding, desc)

		for _, sub := range subdirs {
			fmt.Fprintf(&buf, "    %s/\n", sub)
		}
	}

	return buf.String()
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
