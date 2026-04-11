package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/spf13/cobra"
	"github.com/tfitz/takumi/src/config"
	"github.com/tfitz/takumi/src/ui"
	"github.com/tfitz/takumi/src/workspace"
)

func init() {
	rootCmd.AddCommand(validateCmd)
}

var validateCmd = &cobra.Command{
	Use:   "validate",
	Short: "Check workspace and package configs for errors",
	Long: `Validate all configuration files in the workspace.

Checks takumi.yaml, all takumi-pkg.yaml files, and takumi-versions.yaml
for structural errors, missing fields, invalid values, unresolved
dependencies, and dependency cycles.`,
	RunE: runValidate,
}

func runValidate(cmd *cobra.Command, args []string) error {
	ws := requireWorkspace()

	fmt.Println()
	fmt.Println(ui.SectionHeader.Render("Validating workspace"))
	fmt.Println()

	var errors, warnings int

	// 1. Workspace config
	wsFindings := config.ValidateWorkspace(ws.Config)
	errors, warnings = printFindings("takumi.yaml", wsFindings, errors, warnings)

	// 2. Package configs
	var names []string
	for name := range ws.Packages {
		names = append(names, name)
	}
	sort.Strings(names)

	for _, name := range names {
		pkg := ws.Packages[name]
		relPath, _ := filepath.Rel(ws.Root, filepath.Join(pkg.Dir, workspace.PackageFile))
		if relPath == "" {
			relPath = name + "/" + workspace.PackageFile
		}
		findings := config.ValidatePackage(pkg.Config)
		errors, warnings = printFindings(relPath, findings, errors, warnings)
	}

	// 3. Version-set config (if configured)
	if vsFile := ws.Config.Workspace.VersionSet.File; vsFile != "" {
		vsPath := filepath.Join(ws.Root, vsFile)
		if _, err := os.Stat(vsPath); os.IsNotExist(err) {
			fmt.Printf("  %s %s\n", ui.Cross(""), ui.FilePath(vsFile))
			fmt.Printf("    %s file not found\n", ui.Bullet(""))
			errors++
		} else {
			vsCfg, err := config.LoadVersionSetConfig(vsPath)
			if err != nil {
				fmt.Printf("  %s %s\n", ui.Cross(""), ui.FilePath(vsFile))
				fmt.Printf("    %s %s\n", ui.Bullet(""), err)
				errors++
			} else {
				vsFindings := config.ValidateVersionSet(vsCfg)
				errors, warnings = printFindings(vsFile, vsFindings, errors, warnings)
			}
		}
	}

	// 4. Cross-validation: dependency resolution
	for _, name := range names {
		pkg := ws.Packages[name]
		for _, dep := range pkg.Config.Dependencies {
			if _, exists := ws.Packages[dep]; !exists {
				fmt.Printf("  %s %s depends on %s which is not in the workspace\n",
					ui.Warn(""), ui.Bold.Render(name), ui.Bold.Render(dep))
				warnings++
			}
		}
	}

	// 5. Cross-validation: cycle detection
	g := buildGraph(ws)
	if _, err := g.Sort(); err != nil {
		fmt.Printf("  %s Dependency cycle detected: %s\n", ui.Cross(""), err)
		errors++
	} else {
		fmt.Println(ui.StepDone("Dependency graph — no cycles"))
	}

	// Summary
	fmt.Println()
	if errors == 0 && warnings == 0 {
		fmt.Println(ui.StepDone("All configs valid"))
	} else {
		parts := []string{}
		if errors > 0 {
			parts = append(parts, fmt.Sprintf("%s", ui.FormatCount(errors, "error", "errors")))
		}
		if warnings > 0 {
			parts = append(parts, fmt.Sprintf("%s", ui.FormatCount(warnings, "warning", "warnings")))
		}
		fmt.Printf("  Validation complete: %s\n", strings.Join(parts, ", "))
	}
	fmt.Println()

	if errors > 0 {
		return fmt.Errorf("validation failed with %d error(s)", errors)
	}
	return nil
}

func printFindings(label string, findings []config.Finding, errors, warnings int) (int, int) {
	if len(findings) == 0 {
		fmt.Println(ui.StepDone(ui.FilePath(label) + " — valid"))
		return errors, warnings
	}

	hasError := false
	for _, f := range findings {
		if f.Severity == config.SeverityError {
			hasError = true
			break
		}
	}

	if hasError {
		fmt.Printf("  %s %s\n", ui.Cross(""), ui.FilePath(label))
	} else {
		fmt.Printf("  %s %s\n", ui.Warn(""), ui.FilePath(label))
	}

	for _, f := range findings {
		switch f.Severity {
		case config.SeverityError:
			fmt.Printf("    %s %s — %s\n", ui.Cross(""), ui.Muted.Render(f.Field), f.Message)
			errors++
		case config.SeverityWarning:
			fmt.Printf("    %s %s — %s\n", ui.Warn(""), ui.Muted.Render(f.Field), f.Message)
			warnings++
		}
	}
	return errors, warnings
}
