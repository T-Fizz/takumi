// Package graph provides a dependency DAG with topological sort, cycle detection,
// and parallel execution levels using Kahn's algorithm.
package graph

import "fmt"

// Graph represents a directed acyclic graph of package dependencies.
type Graph struct {
	nodes map[string][]string // node → list of dependencies (what it depends ON)
}

// New creates an empty graph.
func New() *Graph {
	return &Graph{nodes: make(map[string][]string)}
}

// AddNode registers a node (package) with its dependencies.
// Dependencies that are not themselves registered as nodes are silently ignored
// during sort (they may be external/unmanaged).
func (g *Graph) AddNode(name string, deps []string) {
	g.nodes[name] = deps
}

// Nodes returns all registered node names.
func (g *Graph) Nodes() []string {
	names := make([]string, 0, len(g.nodes))
	for n := range g.nodes {
		names = append(names, n)
	}
	return names
}

// DepsOf returns the direct dependencies of a node, or nil if not found.
func (g *Graph) DepsOf(name string) []string {
	return g.nodes[name]
}

// Dependents returns all nodes that directly depend on the given node.
func (g *Graph) Dependents(name string) []string {
	var result []string
	for node, deps := range g.nodes {
		for _, d := range deps {
			if d == name {
				result = append(result, node)
				break
			}
		}
	}
	return result
}

// Level represents a group of packages that can be built in parallel.
type Level struct {
	Index    int      // 0-based level number
	Packages []string // packages at this level (no interdependencies)
}

// Sort performs a topological sort using Kahn's algorithm.
// Returns ordered levels (each level's packages can run in parallel)
// and an error if a cycle is detected.
//
// Edge semantics: if A depends on B, then B must be built before A.
// in-degree[A] = number of A's dependencies that exist in the graph.
func (g *Graph) Sort() ([]Level, error) {
	// Compute in-degree: count how many in-graph deps each node has
	inDegree := make(map[string]int, len(g.nodes))
	for name, deps := range g.nodes {
		count := 0
		for _, d := range deps {
			if _, exists := g.nodes[d]; exists {
				count++
			}
		}
		inDegree[name] = count
	}

	// Seed queue with zero-dependency nodes
	var queue []string
	for name, deg := range inDegree {
		if deg == 0 {
			queue = append(queue, name)
		}
	}

	var levels []Level
	processed := 0

	for len(queue) > 0 {
		levels = append(levels, Level{
			Index:    len(levels),
			Packages: queue,
		})
		processed += len(queue)

		// For each completed node, decrement in-degree of its dependents
		var nextQueue []string
		for _, done := range queue {
			for name, deps := range g.nodes {
				if inDegree[name] <= 0 {
					continue // already processed
				}
				for _, d := range deps {
					if d == done {
						inDegree[name]--
						if inDegree[name] == 0 {
							nextQueue = append(nextQueue, name)
						}
						break
					}
				}
			}
		}
		queue = nextQueue
	}

	if processed < len(g.nodes) {
		var cycleNodes []string
		for name, deg := range inDegree {
			if deg > 0 {
				cycleNodes = append(cycleNodes, name)
			}
		}
		return nil, fmt.Errorf("dependency cycle detected involving: %v", cycleNodes)
	}

	return levels, nil
}

// Flatten returns all nodes in build order (flattened from levels).
func (g *Graph) Flatten() ([]string, error) {
	levels, err := g.Sort()
	if err != nil {
		return nil, err
	}
	var result []string
	for _, level := range levels {
		result = append(result, level.Packages...)
	}
	return result, nil
}

// TransitiveDependents returns all nodes that transitively depend on the given
// node (i.e., all downstream consumers). Uses BFS.
func (g *Graph) TransitiveDependents(name string) []string {
	visited := make(map[string]bool)
	queue := []string{name}
	visited[name] = true

	var result []string
	for len(queue) > 0 {
		current := queue[0]
		queue = queue[1:]

		for _, dep := range g.Dependents(current) {
			if !visited[dep] {
				visited[dep] = true
				result = append(result, dep)
				queue = append(queue, dep)
			}
		}
	}
	return result
}
