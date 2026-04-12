package graph

import (
	"sort"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNew(t *testing.T) {
	g := New()
	assert.NotNil(t, g)
	assert.Empty(t, g.Nodes())
}

func TestAddNode_And_Nodes(t *testing.T) {
	g := New()
	g.AddNode("a", nil)
	g.AddNode("b", []string{"a"})
	g.AddNode("c", []string{"a", "b"})

	nodes := g.Nodes()
	sort.Strings(nodes)
	assert.Equal(t, []string{"a", "b", "c"}, nodes)
}

func TestDepsOf(t *testing.T) {
	g := New()
	g.AddNode("a", nil)
	g.AddNode("b", []string{"a"})

	assert.Nil(t, g.DepsOf("a"))
	assert.Equal(t, []string{"a"}, g.DepsOf("b"))
	assert.Nil(t, g.DepsOf("nonexistent"))
}

func TestDependents(t *testing.T) {
	g := New()
	g.AddNode("a", nil)
	g.AddNode("b", []string{"a"})
	g.AddNode("c", []string{"a"})
	g.AddNode("d", []string{"b", "c"})

	depA := g.Dependents("a")
	sort.Strings(depA)
	assert.Equal(t, []string{"b", "c"}, depA)

	depB := g.Dependents("b")
	assert.Equal(t, []string{"d"}, depB)

	assert.Empty(t, g.Dependents("d"))
	assert.Empty(t, g.Dependents("nonexistent"))
}

func TestSort_Linear(t *testing.T) {
	// a → b → c (c depends on b, b depends on a)
	g := New()
	g.AddNode("a", nil)
	g.AddNode("b", []string{"a"})
	g.AddNode("c", []string{"b"})

	levels, err := g.Sort()
	require.NoError(t, err)
	require.Len(t, levels, 3)

	assert.Equal(t, 0, levels[0].Index)
	assert.Equal(t, []string{"a"}, levels[0].Packages)

	assert.Equal(t, 1, levels[1].Index)
	assert.Equal(t, []string{"b"}, levels[1].Packages)

	assert.Equal(t, 2, levels[2].Index)
	assert.Equal(t, []string{"c"}, levels[2].Packages)
}

func TestSort_Parallel(t *testing.T) {
	// a and b are independent, c depends on both
	g := New()
	g.AddNode("a", nil)
	g.AddNode("b", nil)
	g.AddNode("c", []string{"a", "b"})

	levels, err := g.Sort()
	require.NoError(t, err)
	require.Len(t, levels, 2)

	// Level 0: a and b (parallel)
	l0 := levels[0].Packages
	sort.Strings(l0)
	assert.Equal(t, []string{"a", "b"}, l0)

	// Level 1: c
	assert.Equal(t, []string{"c"}, levels[1].Packages)
}

func TestSort_Diamond(t *testing.T) {
	//     a
	//    / \
	//   b   c
	//    \ /
	//     d
	g := New()
	g.AddNode("a", nil)
	g.AddNode("b", []string{"a"})
	g.AddNode("c", []string{"a"})
	g.AddNode("d", []string{"b", "c"})

	levels, err := g.Sort()
	require.NoError(t, err)
	require.Len(t, levels, 3)

	assert.Equal(t, []string{"a"}, levels[0].Packages)

	l1 := levels[1].Packages
	sort.Strings(l1)
	assert.Equal(t, []string{"b", "c"}, l1)

	assert.Equal(t, []string{"d"}, levels[2].Packages)
}

func TestSort_SingleNode(t *testing.T) {
	g := New()
	g.AddNode("solo", nil)

	levels, err := g.Sort()
	require.NoError(t, err)
	require.Len(t, levels, 1)
	assert.Equal(t, []string{"solo"}, levels[0].Packages)
}

func TestSort_AllIndependent(t *testing.T) {
	g := New()
	g.AddNode("a", nil)
	g.AddNode("b", nil)
	g.AddNode("c", nil)

	levels, err := g.Sort()
	require.NoError(t, err)
	require.Len(t, levels, 1)

	pkgs := levels[0].Packages
	sort.Strings(pkgs)
	assert.Equal(t, []string{"a", "b", "c"}, pkgs)
}

func TestSort_Empty(t *testing.T) {
	g := New()

	levels, err := g.Sort()
	require.NoError(t, err)
	assert.Empty(t, levels)
}

func TestSort_CycleDetected(t *testing.T) {
	// a → b → c → a
	g := New()
	g.AddNode("a", []string{"c"})
	g.AddNode("b", []string{"a"})
	g.AddNode("c", []string{"b"})

	_, err := g.Sort()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "dependency cycle detected")
}

func TestSort_SelfCycle(t *testing.T) {
	g := New()
	g.AddNode("a", []string{"a"})

	_, err := g.Sort()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "dependency cycle detected")
}

func TestSort_PartialCycle(t *testing.T) {
	// a is fine, b↔c is a cycle
	g := New()
	g.AddNode("a", nil)
	g.AddNode("b", []string{"c"})
	g.AddNode("c", []string{"b"})

	_, err := g.Sort()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "dependency cycle detected")
}

func TestSort_ExternalDepsIgnored(t *testing.T) {
	// b depends on "external" which is not in the graph — should be ignored
	g := New()
	g.AddNode("a", nil)
	g.AddNode("b", []string{"a", "external-lib"})

	levels, err := g.Sort()
	require.NoError(t, err)
	require.Len(t, levels, 2)
	assert.Equal(t, []string{"a"}, levels[0].Packages)
	assert.Equal(t, []string{"b"}, levels[1].Packages)
}

func TestFlatten(t *testing.T) {
	g := New()
	g.AddNode("a", nil)
	g.AddNode("b", []string{"a"})
	g.AddNode("c", []string{"b"})

	order, err := g.Flatten()
	require.NoError(t, err)
	assert.Equal(t, []string{"a", "b", "c"}, order)
}

func TestFlatten_CycleError(t *testing.T) {
	g := New()
	g.AddNode("a", []string{"b"})
	g.AddNode("b", []string{"a"})

	_, err := g.Flatten()
	require.Error(t, err)
}

func TestTransitiveDependents(t *testing.T) {
	//     a
	//    / \
	//   b   c
	//   |
	//   d
	g := New()
	g.AddNode("a", nil)
	g.AddNode("b", []string{"a"})
	g.AddNode("c", []string{"a"})
	g.AddNode("d", []string{"b"})

	// Changing a affects b, c, d
	td := g.TransitiveDependents("a")
	sort.Strings(td)
	assert.Equal(t, []string{"b", "c", "d"}, td)

	// Changing b affects d only
	td2 := g.TransitiveDependents("b")
	assert.Equal(t, []string{"d"}, td2)

	// Changing d affects nothing
	assert.Empty(t, g.TransitiveDependents("d"))
}

func TestTransitiveDependents_Deep(t *testing.T) {
	// a → b → c → d → e
	g := New()
	g.AddNode("a", nil)
	g.AddNode("b", []string{"a"})
	g.AddNode("c", []string{"b"})
	g.AddNode("d", []string{"c"})
	g.AddNode("e", []string{"d"})

	td := g.TransitiveDependents("a")
	sort.Strings(td)
	assert.Equal(t, []string{"b", "c", "d", "e"}, td)
}

func TestSort_Complex(t *testing.T) {
	// Real-world-ish:
	//   shared-utils (no deps)
	//   data-models (no deps)
	//   api-service (depends on shared-utils, data-models)
	//   frontend (depends on shared-utils)
	//   integration-tests (depends on api-service, frontend)
	g := New()
	g.AddNode("shared-utils", nil)
	g.AddNode("data-models", nil)
	g.AddNode("api-service", []string{"shared-utils", "data-models"})
	g.AddNode("frontend", []string{"shared-utils"})
	g.AddNode("integration-tests", []string{"api-service", "frontend"})

	levels, err := g.Sort()
	require.NoError(t, err)
	require.Len(t, levels, 3)

	// Level 0: shared-utils, data-models (parallel)
	l0 := levels[0].Packages
	sort.Strings(l0)
	assert.Equal(t, []string{"data-models", "shared-utils"}, l0)

	// Level 1: api-service, frontend (parallel)
	l1 := levels[1].Packages
	sort.Strings(l1)
	assert.Equal(t, []string{"api-service", "frontend"}, l1)

	// Level 2: integration-tests
	assert.Equal(t, []string{"integration-tests"}, levels[2].Packages)
}
