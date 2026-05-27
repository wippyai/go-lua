package propagate

import (
	"testing"

	"github.com/wippyai/go-lua/types/cfg"
	"github.com/wippyai/go-lua/types/constraint"
)

// TestPropagate_AcyclicHighFanIn_NoWidening models the §10.5 regression: a
// purely acyclic CFG with a single merge point of N predecessors, each
// carrying a distinct disjunct on a distinct path literal. The merge point
// is NOT in the FVS (no cycle contains it), so widening MUST NOT fire — the
// precise N-disjunct condition is preserved through the merge.
//
// DOMAIN_DESIGN.md §10.5 / §7.4.
func TestPropagate_AcyclicHighFanIn_NoWidening(t *testing.T) {
	const N = 8

	// Path per predecessor branch.
	branches := make([]constraint.Path, N)
	for i := 0; i < N; i++ {
		branches[i] = constraint.Path{Root: "p", Symbol: cfg.SymbolID(200 + i)}
	}

	// CFG: entry(1) → split(2) → branches[3 .. 2+N] → merge(2+N+1) → end(2+N+2).
	const splitPoint cfg.Point = 2
	mergePoint := cfg.Point(2 + N + 1)
	endPoint := cfg.Point(2 + N + 2)

	g := &mockGraph{
		entry: 1,
		nodes: map[cfg.Point]*cfg.Node{
			1:          {Kind: cfg.NodeEntry, Point: 1},
			splitPoint: {Kind: cfg.NodeBranch, Point: splitPoint},
			mergePoint: {Kind: cfg.NodeJoin, Point: mergePoint},
			endPoint:   {Kind: cfg.NodeAssign, Point: endPoint},
		},
		preds: map[cfg.Point][]cfg.Point{
			1:          {},
			splitPoint: {1},
			endPoint:   {mergePoint},
		},
		succs: map[cfg.Point][]cfg.Point{
			1:        {splitPoint},
			endPoint: {},
		},
		rpo: []cfg.Point{1, splitPoint},
	}

	edgeConds := EdgeConditions{}
	mergePreds := make([]cfg.Point, 0, N)
	for i := 0; i < N; i++ {
		bp := cfg.Point(3 + i)
		g.nodes[bp] = &cfg.Node{Kind: cfg.NodeAssign, Point: bp}
		g.preds[bp] = []cfg.Point{splitPoint}
		g.succs[bp] = []cfg.Point{mergePoint}
		edgeConds[EdgeKey{From: splitPoint, To: bp}] = constraint.FromConstraints(constraint.Truthy{Path: branches[i]})
		mergePreds = append(mergePreds, bp)
		g.rpo = append(g.rpo, bp)
	}
	g.preds[mergePoint] = mergePreds
	g.succs[splitPoint] = mergePreds
	g.succs[mergePoint] = []cfg.Point{endPoint}
	g.rpo = append(g.rpo, mergePoint, endPoint)

	inputs := &Inputs{
		Graph:          g,
		EdgeConditions: edgeConds,
	}
	result := Propagate(inputs)

	merged := result.PointConditions[mergePoint]
	if merged.IsFalse() {
		t.Fatalf("merge point should be reachable; got ⊥")
	}
	if merged.IsTrue() {
		t.Fatalf("merge point should carry N distinct disjuncts; widening collapsed it to ⊤")
	}

	gotDisjuncts := merged.NumDisjuncts()
	if gotDisjuncts != N {
		t.Fatalf("expected %d disjuncts at merge (one per branch, widening must NOT fire); got %d\n  merged=%v",
			N, gotDisjuncts, merged)
	}

	// Verify every branch literal is represented.
	keys := map[string]struct{}{}
	for _, d := range merged.Disjuncts {
		for _, lit := range d {
			keys[constraintKey(lit)] = struct{}{}
		}
	}
	for i, bp := range branches {
		want := constraint.Truthy{Path: bp}
		if _, ok := keys[constraintKey(want)]; !ok {
			t.Errorf("merge missing branch %d literal %v", i, want)
		}
	}
}
