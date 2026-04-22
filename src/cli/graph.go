package cli

import (
	"fmt"
	"sort"
	"strings"

	"github.com/spf13/cobra"
	"github.com/tfitz/takumi/src/graph"
	"github.com/tfitz/takumi/src/ui"
	"github.com/tfitz/takumi/src/workspace"
)

var graphPhases bool

func init() {
	graphCmd.Flags().BoolVar(&graphPhases, "phases", false, "Show phase commands for each package")
	rootCmd.AddCommand(graphCmd)
}

var graphCmd = &cobra.Command{
	Use:   "graph",
	Short: "Print the dependency graph",
	Long:  `Display the package dependency graph as an ASCII tree with parallel level annotations.`,
	RunE:  runGraph,
}

func runGraph(cmd *cobra.Command, args []string) error {
	ws := requireWorkspace()
	g := buildGraph(ws)

	levels, err := g.Sort()
	if err != nil {
		return err
	}

	if len(levels) == 0 {
		fmt.Println(ui.Warn("No packages found in workspace"))
		return nil
	}

	fmt.Println()
	fmt.Println(ui.SectionHeader.Render("Dependency Graph"))
	fmt.Println()

	for _, level := range levels {
		sort.Strings(level.Packages)
		label := ui.Muted.Render(fmt.Sprintf("Level %d", level.Index))
		if level.Index == 0 {
			label += ui.Muted.Render(" (no deps)")
		}
		fmt.Printf("  %s\n", label)

		for _, pkg := range level.Packages {
			deps := g.DepsOf(pkg)
			line := "    " + ui.Bold.Render(pkg)
			if len(deps) > 0 {
				// Only show in-graph deps
				var inGraph []string
				for _, d := range deps {
					if g.DepsOf(d) != nil || contains(g.Nodes(), d) {
						inGraph = append(inGraph, d)
					}
				}
				if len(inGraph) > 0 {
					sort.Strings(inGraph)
					line += ui.Muted.Render(" ← " + strings.Join(inGraph, ", "))
				}
			}
			fmt.Println(line)

			// Show phase commands when --phases flag is set
			if graphPhases {
				if wsPkg, ok := ws.Packages[pkg]; ok {
					var phaseNames []string
					for name := range wsPkg.Config.Phases {
						phaseNames = append(phaseNames, name)
					}
					sort.Strings(phaseNames)
					for _, name := range phaseNames {
						phase := wsPkg.Config.Phases[name]
						for _, c := range phase.Commands {
							fmt.Println("      " + ui.Muted.Render(name+": "+c))
						}
					}
				}
			}
		}
		fmt.Println()
	}

	fmt.Println(ui.Divider())
	fmt.Printf("%s in %s\n",
		ui.FormatCount(len(g.Nodes()), "package", "packages"),
		ui.FormatCount(len(levels), "level", "levels"),
	)
	fmt.Println()

	return nil
}

// buildGraph constructs a graph.Graph from workspace packages.
func buildGraph(ws *workspace.Info) *graph.Graph {
	g := graph.New()
	for name, pkg := range ws.Packages {
		g.AddNode(name, pkg.Config.Dependencies)
	}
	return g
}

func contains(list []string, item string) bool {
	for _, s := range list {
		if s == item {
			return true
		}
	}
	return false
}
