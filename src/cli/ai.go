package cli

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"

	"github.com/spf13/cobra"
	"github.com/tfitz/takumi/src/skills"
	"github.com/tfitz/takumi/src/ui"
	"github.com/tfitz/takumi/src/workspace"
)

func init() {
	aiCmd.AddCommand(aiContextCmd, aiDiagnoseCmd, aiReviewCmd, aiOptimizeCmd, aiOnboardCmd, aiSkillCmd)
	aiSkillCmd.AddCommand(aiSkillListCmd, aiSkillShowCmd, aiSkillRunCmd)
	rootCmd.AddCommand(aiCmd)
}

var aiCmd = &cobra.Command{
	Use:   "ai",
	Short: "AI skills and context generation",
}

var aiContextCmd = &cobra.Command{
	Use:   "context",
	Short: "Regenerate AI context and integrations",
	Long:  `Regenerate .takumi/TAKUMI.md with the operator skill and workspace snapshot.`,
	RunE:  runAIContext,
}

var aiDiagnoseCmd = &cobra.Command{
	Use:   "diagnose <package>",
	Short: "Triage a build/test failure for a package",
	Args:  cobra.ExactArgs(1),
	RunE:  runAIDiagnose,
}

var aiReviewCmd = &cobra.Command{
	Use:   "review",
	Short: "Summarize workspace changes for code review",
	RunE:  runAIReview,
}

var aiOptimizeCmd = &cobra.Command{
	Use:   "optimize",
	Short: "Analyze build performance and suggest improvements",
	RunE:  runAIOptimize,
}

var aiOnboardCmd = &cobra.Command{
	Use:   "onboard",
	Short: "Generate a workspace briefing for new developers",
	RunE:  runAIOnboard,
}

var aiSkillCmd = &cobra.Command{
	Use:   "skill",
	Short: "Manage AI skills",
}

var aiSkillListCmd = &cobra.Command{
	Use:   "list",
	Short: "List all available skills",
	RunE:  runAISkillList,
}

var aiSkillShowCmd = &cobra.Command{
	Use:   "show <name>",
	Short: "Print a skill's prompt template",
	Args:  cobra.ExactArgs(1),
	RunE:  runAISkillShow,
}

var aiSkillRunCmd = &cobra.Command{
	Use:   "run <name>",
	Short: "Collect context, render skill template, and output",
	Args:  cobra.ExactArgs(1),
	RunE:  runAISkillRun,
}

// --- Implementations ---

func runAIContext(cmd *cobra.Command, args []string) error {
	ws := requireWorkspace()

	if err := writeTakumiMD(ws.Root, ws.Config.Workspace.Name); err != nil {
		return err
	}

	if ws.Config.Workspace.AI.Agent != "" {
		agent := AgentByName(ws.Config.Workspace.AI.Agent)
		if agent != nil {
			setupAgentConfig(ws.Root, agent)
		}
	}

	fmt.Println(ui.StepDone("Regenerated AI context"))
	return nil
}

func runAIDiagnose(cmd *cobra.Command, args []string) error {
	ws := requireWorkspace()
	pkgName := args[0]

	if _, exists := ws.Packages[pkgName]; !exists {
		return fmt.Errorf("package %q not found in workspace", pkgName)
	}

	logDir := filepath.Join(ws.Root, workspace.MarkerDir, "logs")
	var logContent, logPhase string

	for _, phase := range []string{"build", "test"} {
		logPath := filepath.Join(logDir, fmt.Sprintf("%s.%s.log", pkgName, phase))
		if data, err := os.ReadFile(logPath); err == nil {
			logContent = string(data)
			logPhase = phase
			break
		}
	}

	if logContent == "" {
		fmt.Println(ui.Warn("No log file found for " + ui.Bold.Render(pkgName)))
		fmt.Println(ui.StepInfo("Run " + ui.Command("takumi build "+pkgName) + " first"))
		return nil
	}

	skill := findSkill("diagnose")
	if skill == nil {
		return fmt.Errorf("built-in skill 'diagnose' not found")
	}

	changedFiles, _ := gitChangedFiles(ws.Root, "HEAD")
	g := buildGraph(ws)
	deps := g.DepsOf(pkgName)

	vars := map[string]string{
		"package_name":     pkgName,
		"phase":            logPhase,
		"exit_code":        "non-zero",
		"error_output":     logContent,
		"changed_files":    strings.Join(changedFiles, "\n"),
		"dependency_chain": strings.Join(deps, ", "),
		"env_status":       envStatus(ws, pkgName),
	}

	fmt.Println()
	fmt.Println(ui.SectionHeader.Render("Diagnostic: " + pkgName))
	fmt.Println()
	fmt.Println(skills.Render(skill.Prompt, vars))
	return nil
}

func runAIReview(cmd *cobra.Command, args []string) error {
	ws := requireWorkspace()

	skill := findSkill("review")
	if skill == nil {
		return fmt.Errorf("built-in skill 'review' not found")
	}

	diff := gitDiffOutput(ws.Root)
	changedFiles, _ := gitChangedFiles(ws.Root, "HEAD")
	affected := mapFilesToPackages(ws, changedFiles)

	var names []string
	for name := range affected {
		names = append(names, name)
	}
	sort.Strings(names)

	vars := map[string]string{
		"affected_packages": strings.Join(names, ", "),
		"git_diff":          diff,
		"test_results":      "(run `takumi test` to generate)",
	}

	fmt.Println()
	fmt.Println(ui.SectionHeader.Render("Review Prompt"))
	fmt.Println()
	fmt.Println(skills.Render(skill.Prompt, vars))
	return nil
}

func runAIOptimize(cmd *cobra.Command, args []string) error {
	ws := requireWorkspace()

	skill := findSkill("optimize")
	if skill == nil {
		return fmt.Errorf("built-in skill 'optimize' not found")
	}

	metricsPath := filepath.Join(ws.Root, workspace.MarkerDir, "metrics.json")
	metricsData := "(no build metrics yet)"
	if data, err := os.ReadFile(metricsPath); err == nil {
		metricsData = string(data)
	}

	g := buildGraph(ws)
	levels, _ := g.Sort()
	var graphBuf strings.Builder
	for _, level := range levels {
		fmt.Fprintf(&graphBuf, "Level %d: %s\n", level.Index, strings.Join(level.Packages, ", "))
	}

	vars := map[string]string{
		"build_metrics": metricsData,
		"package_graph": graphBuf.String(),
	}

	fmt.Println()
	fmt.Println(ui.SectionHeader.Render("Optimization Prompt"))
	fmt.Println()
	fmt.Println(skills.Render(skill.Prompt, vars))
	return nil
}

func runAIOnboard(cmd *cobra.Command, args []string) error {
	ws := requireWorkspace()

	skill := findSkill("onboard")
	if skill == nil {
		return fmt.Errorf("built-in skill 'onboard' not found")
	}

	wsCfgData, _ := ws.Config.Marshal()

	var pkgBuf strings.Builder
	for name, pkg := range ws.Packages {
		data, _ := pkg.Config.Marshal()
		fmt.Fprintf(&pkgBuf, "--- %s ---\n%s\n", name, string(data))
	}

	g := buildGraph(ws)
	levels, _ := g.Sort()
	var graphBuf strings.Builder
	for _, level := range levels {
		fmt.Fprintf(&graphBuf, "Level %d: %s\n", level.Index, strings.Join(level.Packages, ", "))
	}

	vars := map[string]string{
		"workspace_config":    string(wsCfgData),
		"all_package_configs": pkgBuf.String(),
		"dependency_graph":    graphBuf.String(),
		"version_set":         "(not configured)",
	}

	fmt.Println()
	fmt.Println(ui.SectionHeader.Render("Onboarding Prompt"))
	fmt.Println()
	fmt.Println(skills.Render(skill.Prompt, vars))
	return nil
}

func runAISkillList(cmd *cobra.Command, args []string) error {
	_ = requireWorkspace()

	allSkills, err := loadAllSkills()
	if err != nil {
		return err
	}

	sourceLabel := map[skills.Source]string{
		skills.SourceBuiltin:   "built-in",
		skills.SourceWorkspace: "workspace",
		skills.SourcePackage:   "package",
	}

	fmt.Println()
	fmt.Println(ui.SectionHeader.Render("Available Skills"))
	fmt.Println()

	for _, s := range allSkills {
		fmt.Printf("  %s %s %s\n",
			ui.Bold.Render(s.Name),
			ui.Muted.Render("["+sourceLabel[s.Source]+"]"),
			ui.Muted.Render(s.Description),
		)
	}
	fmt.Println()
	return nil
}

func runAISkillShow(cmd *cobra.Command, args []string) error {
	_ = requireWorkspace()

	skill := findSkill(args[0])
	if skill == nil {
		return fmt.Errorf("skill %q not found", args[0])
	}

	fmt.Println()
	fmt.Println(ui.SectionHeader.Render("Skill: " + skill.Name))
	fmt.Println(ui.Muted.Render("  " + skill.Description))
	fmt.Println()

	if len(skill.AutoContext) > 0 {
		fmt.Println(ui.Bold.Render("  Auto-context:"))
		for _, ctx := range skill.AutoContext {
			fmt.Println("    " + ui.Bullet(ctx))
		}
		fmt.Println()
	}

	fmt.Println(ui.Bold.Render("  Prompt template:"))
	fmt.Println(ui.Divider())
	fmt.Println(skill.Prompt)
	return nil
}

func runAISkillRun(cmd *cobra.Command, args []string) error {
	ws := requireWorkspace()
	name := args[0]

	switch name {
	case "diagnose":
		return fmt.Errorf("use 'takumi ai diagnose <package>' instead")
	case "review":
		return runAIReview(cmd, nil)
	case "optimize":
		return runAIOptimize(cmd, nil)
	case "onboard":
		return runAIOnboard(cmd, nil)
	}

	skill := findSkill(name)
	if skill == nil {
		return fmt.Errorf("skill %q not found", name)
	}

	vars := map[string]string{
		"workspace_name": ws.Config.Workspace.Name,
	}

	fmt.Println()
	fmt.Println(ui.SectionHeader.Render("Running: " + name))
	fmt.Println()
	fmt.Println(skills.Render(skill.Prompt, vars))
	return nil
}

// --- Helpers ---

func loadAllSkills() ([]skills.LoadedSkill, error) {
	return skills.LoadBuiltins()
}

func findSkill(name string) *skills.LoadedSkill {
	all, err := loadAllSkills()
	if err != nil {
		return nil
	}
	for i := range all {
		if all[i].Name == name {
			return &all[i]
		}
	}
	return nil
}

func envStatus(ws *workspace.Info, pkgName string) string {
	pkg, exists := ws.Packages[pkgName]
	if !exists || pkg.Config.Runtime == nil {
		return "no runtime defined"
	}
	envDir := filepath.Join(ws.Root, workspace.MarkerDir, "envs", pkgName)
	if _, err := os.Stat(envDir); os.IsNotExist(err) {
		return "not set up"
	}
	return "ready (" + envDir + ")"
}

func gitDiffOutput(wsRoot string) string {
	cmd := exec.Command("git", "-C", wsRoot, "diff")
	out, err := cmd.Output()
	if err != nil {
		return "(git diff unavailable)"
	}
	return string(out)
}
