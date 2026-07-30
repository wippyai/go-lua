package cfg

import "testing"

// nested builds `if a then if b then X end end`: X is reached only through the
// true edge of both branches.
func nested() (graph *CFG, outer, inner, guarded Point) {
	graph = New()
	outer = graph.AddNode(NodeBranch)
	inner = graph.AddNode(NodeBranch)
	guarded = graph.AddNode(NodeAssign)
	join := graph.AddNode(NodeJoin)
	graph.AddEdge(graph.Entry(), outer, true)
	graph.AddEdge(outer, inner, true)
	graph.AddEdge(outer, join, false)
	graph.AddEdge(inner, guarded, true)
	graph.AddEdge(inner, join, false)
	graph.AddEdge(guarded, join, true)
	graph.AddEdge(join, graph.Exit(), true)
	return graph, outer, inner, guarded
}

// sequential builds `if a then Y end if b then X end`: the second branch is
// reached whichever edge the first one takes.
func sequential() (graph *CFG, first, second, guarded Point) {
	graph = New()
	first = graph.AddNode(NodeBranch)
	body := graph.AddNode(NodeAssign)
	middle := graph.AddNode(NodeJoin)
	second = graph.AddNode(NodeBranch)
	guarded = graph.AddNode(NodeAssign)
	join := graph.AddNode(NodeJoin)
	graph.AddEdge(graph.Entry(), first, true)
	graph.AddEdge(first, body, true)
	graph.AddEdge(first, middle, false)
	graph.AddEdge(body, middle, true)
	graph.AddEdge(middle, second, true)
	graph.AddEdge(second, guarded, true)
	graph.AddEdge(second, join, false)
	graph.AddEdge(guarded, join, true)
	graph.AddEdge(join, graph.Exit(), true)
	return graph, first, second, guarded
}

// parallelArms builds a branch whose two edges land on the same point, the
// shape an empty then-arm can lower to. Neither edge is a guard: the successor
// is reached whichever condition holds.
func parallelArms() (graph *CFG, branch, target Point) {
	graph = New()
	branch = graph.AddNode(NodeBranch)
	target = graph.AddNode(NodeJoin)
	graph.AddEdge(graph.Entry(), branch, true)
	graph.AddEdge(branch, target, true)
	graph.AddEdge(branch, target, false)
	graph.AddEdge(target, graph.Exit(), true)
	return graph, branch, target
}

func TestEveryPathTakesEdgeAcceptsNestedGuard(t *testing.T) {
	graph, outer, inner, guarded := nested()
	for _, successor := range graph.Successors(outer) {
		if cond, _ := graph.EdgeCond(outer, successor); !cond {
			continue
		}
		if !EveryPathTakesEdge(graph, graph.Entry(), inner, outer, successor) {
			t.Fatalf("inner branch is reachable around the outer true edge")
		}
		if !EveryPathTakesEdge(graph, graph.Entry(), guarded, outer, successor) {
			t.Fatalf("guarded point is reachable around the outer true edge")
		}
	}
	for _, successor := range graph.Successors(inner) {
		cond, _ := graph.EdgeCond(inner, successor)
		if cond && !EveryPathTakesEdge(graph, graph.Entry(), guarded, inner, successor) {
			t.Fatalf("guarded point is reachable around the inner true edge")
		}
		if !cond && EveryPathTakesEdge(graph, graph.Entry(), guarded, inner, successor) {
			t.Fatalf("guarded point reported as needing the inner false edge")
		}
	}
}

func TestEveryPathTakesEdgeRejectsSequentialGuard(t *testing.T) {
	graph, first, second, guarded := sequential()
	for _, successor := range graph.Successors(first) {
		if cond, _ := graph.EdgeCond(first, successor); !cond {
			continue
		}
		if EveryPathTakesEdge(graph, graph.Entry(), second, first, successor) {
			t.Fatalf("a sibling branch was reported as guarded by the first branch's true edge")
		}
		if EveryPathTakesEdge(graph, graph.Entry(), guarded, first, successor) {
			t.Fatalf("a sibling arm was reported as guarded by the first branch's true edge")
		}
	}
}

func TestEveryPathTakesEdgeRejectsParallelArms(t *testing.T) {
	graph, branch, target := parallelArms()
	if EveryPathTakesEdge(graph, graph.Entry(), target, branch, target) {
		t.Fatalf("a point both arms reach was reported as guarded by one of them")
	}
}

func TestEveryPathTakesEdgeRejectsUnplaceableCoordinates(t *testing.T) {
	graph, outer, inner, _ := nested()
	outside := Point(graph.Size() + 5)
	if EveryPathTakesEdge(graph, graph.Entry(), outside, outer, inner) {
		t.Fatalf("a point outside the graph was answered as guarded")
	}
	if EveryPathTakesEdge(graph, graph.Entry(), inner, outside, inner) {
		t.Fatalf("an edge outside the graph was answered as guarded")
	}
	if EveryPathTakesEdge(nil, 0, 1, 0, 1) {
		t.Fatalf("a nil graph was answered as guarded")
	}
	if EveryPathTakesEdge(graph, inner, inner, outer, inner) {
		t.Fatalf("a point reached itself through a cut edge")
	}
}
