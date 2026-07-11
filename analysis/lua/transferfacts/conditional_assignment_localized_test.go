package transferfacts

import (
	"math/rand"
	"testing"

	"github.com/wippyai/go-lua/analysis/domain/path"
	factflow "github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/symbol"
	"github.com/wippyai/go-lua/analysis/test/value/standard"
)

func TestAssignmentStatesBeforePointsMatchesWholeGraphOracleAcrossSmallCFGs(t *testing.T) {
	rng := rand.New(rand.NewSource(0x5cc17))
	for sample := 0; sample < 240; sample++ {
		graph := cfg.New()
		count := 2 + rng.Intn(9)
		points := make([]cfg.Point, count)
		for i := range points {
			points[i] = graph.AddNode(cfg.NodeNoop)
		}
		// A spine makes every sampled point reachable. Extra arbitrary edges
		// produce self-loops, nested/reducible cycles, multiple SCCs, and
		// irreducible SCCs with multiple entries.
		graph.AddEdge(graph.Entry(), points[0], true)
		for i := 0; i+1 < len(points); i++ {
			graph.AddEdge(points[i], points[i+1], true)
		}
		graph.AddEdge(points[len(points)-1], graph.Exit(), true)
		for i := 0; i < count+rng.Intn(count*2+1); i++ {
			from := points[rng.Intn(len(points))]
			toChoice := rng.Intn(len(points) + 1)
			to := graph.Exit()
			if toChoice < len(points) {
				to = points[toChoice]
			}
			graph.AddEdge(from, to, rng.Intn(2) == 0)
		}

		rootAssignments := make(map[cfg.Point]factflow.RootAssignment)
		pathAssignments := make(map[cfg.Point]factflow.PathAssignment)
		for _, point := range points {
			sym := symbol.ID(1 + rng.Intn(3))
			target := path.NewPath(sym, string(rune('x'+sym-1)))
			switch rng.Intn(4) {
			case 0, 1:
				rootAssignments[point] = boolRootAssignment(target, rng.Intn(2) == 0)
			case 2:
				// Unknown member writes exercise overlapping invalidation without
				// introducing new evidence.
				pathAssignments[point] = factflow.NewPathAssignment(
					target.Field("member"),
					factflow.ValueSource{Kind: factflow.ValueSourceUnknown},
				)
			}
		}
		input := &factflow.FactsInput{
			RootAssignments: rootAssignments,
			PathAssignments: pathAssignments,
		}
		t.Run("sample", func(t *testing.T) {
			assertLocalizedAssignmentStatesMatchOracle(t, graph, input)
		})
	}
}

func TestAssignmentStatesBeforePointsHandlesSelfLoopAndIgnoresUnreachableComponent(t *testing.T) {
	graph := cfg.New()
	seed := graph.AddNode(cfg.NodeAssign)
	loop := graph.AddNode(cfg.NodeBranch)
	unreachableA := graph.AddNode(cfg.NodeBranch)
	unreachableB := graph.AddNode(cfg.NodeBranch)
	graph.AddEdge(graph.Entry(), seed, true)
	graph.AddEdge(seed, loop, true)
	graph.AddEdge(loop, loop, true)
	graph.AddEdge(loop, graph.Exit(), false)
	graph.AddEdge(unreachableA, unreachableB, true)
	graph.AddEdge(unreachableB, unreachableA, true)

	x := path.NewPath(symbol.ID(1), "x")
	input := &factflow.FactsInput{RootAssignments: map[cfg.Point]factflow.RootAssignment{
		seed:         boolRootAssignment(x, true),
		unreachableA: boolRootAssignment(x, false),
	}}
	lower := &lowerer{registry: standard.Registry()}
	localized := lower.assignmentStatesBeforePoints(input, graph)
	if _, ok := localized[unreachableA]; ok {
		t.Fatal("unreachable SCC unexpectedly received an incoming assignment state")
	}
	want := lower.assignmentStateBeforePoint(input, graph, loop)
	if got := localized[loop]; !presentAssignmentStateEqual(got, want) {
		t.Fatalf("self-loop state differs from whole-graph oracle\nwant: %#v\n got: %#v", want.entries, got.entries)
	}
}

func TestAssignmentStatesBeforePointsMatchesWholeGraphOracleOnNestedLoop(t *testing.T) {
	graph := cfg.New()
	seed := graph.AddNode(cfg.NodeAssign)
	outer := graph.AddNode(cfg.NodeBranch)
	inner := graph.AddNode(cfg.NodeBranch)
	write := graph.AddNode(cfg.NodeAssign)
	graph.AddEdge(graph.Entry(), seed, true)
	graph.AddEdge(seed, outer, true)
	graph.AddEdge(outer, inner, true)
	graph.AddEdge(outer, graph.Exit(), false)
	graph.AddEdge(inner, write, true)
	graph.AddEdge(inner, outer, false)
	graph.AddEdge(write, inner, true)

	x := path.NewPath(symbol.ID(1), "x")
	input := &factflow.FactsInput{RootAssignments: map[cfg.Point]factflow.RootAssignment{
		seed:  boolRootAssignment(x, true),
		write: boolRootAssignment(x, false),
	}}
	assertLocalizedAssignmentStatesMatchOracle(t, graph, input)
}

func TestAssignmentStatesBeforePointsMatchesWholeGraphOracleOnIrreducibleLoop(t *testing.T) {
	graph := cfg.New()
	seed := graph.AddNode(cfg.NodeBranch)
	left := graph.AddNode(cfg.NodeBranch)
	right := graph.AddNode(cfg.NodeBranch)
	leftWrite := graph.AddNode(cfg.NodeAssign)
	rightWrite := graph.AddNode(cfg.NodeAssign)
	graph.AddEdge(graph.Entry(), seed, true)
	graph.AddEdge(seed, left, true)
	graph.AddEdge(seed, right, false)
	graph.AddEdge(left, leftWrite, true)
	graph.AddEdge(left, graph.Exit(), false)
	graph.AddEdge(leftWrite, right, true)
	graph.AddEdge(right, rightWrite, true)
	graph.AddEdge(right, graph.Exit(), false)
	graph.AddEdge(rightWrite, left, true)

	x := path.NewPath(symbol.ID(1), "x")
	input := &factflow.FactsInput{RootAssignments: map[cfg.Point]factflow.RootAssignment{
		seed:       boolRootAssignment(x, true),
		leftWrite:  boolRootAssignment(x, false),
		rightWrite: boolRootAssignment(x, true),
	}}
	assertLocalizedAssignmentStatesMatchOracle(t, graph, input)
}

func TestAssignmentStatesBeforePointsDoesNotRescanAcyclicPrefixPerCyclicBranch(t *testing.T) {
	graph := cfg.New()
	previous := graph.Entry()
	for range 128 {
		point := graph.AddNode(cfg.NodeNoop)
		graph.AddEdge(previous, point, true)
		previous = point
	}
	left := graph.AddNode(cfg.NodeBranch)
	right := graph.AddNode(cfg.NodeBranch)
	graph.AddEdge(previous, left, true)
	graph.AddEdge(left, right, true)
	graph.AddEdge(left, graph.Exit(), false)
	graph.AddEdge(right, left, true)
	graph.AddEdge(right, graph.Exit(), false)

	input := &factflow.FactsInput{}
	stats := &presentAssignmentTransferStats{}
	lower := &lowerer{registry: standard.Registry(), presentAssignmentStats: stats}
	for _, branch := range []cfg.Point{left, right} {
		lower.assignmentStateBeforePoint(input, graph, branch)
	}
	oracleTransfers := stats.transfers
	stats.transfers = 0
	lower.assignmentStatesBeforePoints(input, graph)
	localizedTransfers := stats.transfers
	if localizedTransfers >= oracleTransfers {
		t.Fatalf("localized transfers = %d, want fewer than whole-graph oracle %d", localizedTransfers, oracleTransfers)
	}
}

func assertLocalizedAssignmentStatesMatchOracle(
	t *testing.T,
	graph cfg.Graph,
	input *factflow.FactsInput,
) {
	t.Helper()
	lower := &lowerer{registry: standard.Registry()}
	localized := lower.assignmentStatesBeforePoints(input, graph)
	for _, point := range cfg.RPOReadOnly(graph) {
		if !graph.IsBranch(point) {
			continue
		}
		want := lower.assignmentStateBeforePoint(input, graph, point)
		got, ok := localized[point]
		if !ok {
			t.Fatalf("branch %d missing localized state", point)
		}
		if !presentAssignmentStateEqual(got, want) {
			t.Fatalf("branch %d localized state differs from whole-graph oracle\nwant: %#v\n got: %#v", point, want.entries, got.entries)
		}
	}
}

func boolRootAssignment(target path.Path, value bool) factflow.RootAssignment {
	return factflow.NewRootAssignment(
		factflow.RootAssignmentOrdinaryRootWrite,
		target.Symbol,
		target,
		factflow.ValueSource{
			Kind:        factflow.ValueSourceLiteral,
			LiteralKind: factflow.ValueSourceLiteralBool,
			Bool:        value,
		},
	)
}
