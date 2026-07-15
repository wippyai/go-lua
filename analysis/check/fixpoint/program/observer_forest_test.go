package program

import (
	"context"
	"reflect"
	"testing"

	"github.com/wippyai/go-lua/analysis/check/fixpoint/transformer"
	"github.com/wippyai/go-lua/analysis/engine/observation"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/ir/wir"
	"github.com/wippyai/go-lua/analysis/lexicalidentity"
)

func TestLexicalObserverForestAssemblesAcyclicGraphCanonically(t *testing.T) {
	nodes := observerTestNodes(4, [][2]int{{0, 1}, {1, 2}})
	want := assembleObserverTestForest(t, nodes, 0)
	if len(want.Diagnostic) != 3 || len(want.Uncalled) != 1 || want.Work.DiagnosticEdges != 2 {
		t.Fatalf("diagnostic/uncalled/edges = %d/%d/%d, want 3/1/2", len(want.Diagnostic), len(want.Uncalled), want.Work.DiagnosticEdges)
	}
	if want.Work.RetainedGraphUnits() != len(want.Diagnostic)+want.Work.DiagnosticEdges {
		t.Fatalf("retained units = %d, want templates+calls", want.Work.RetainedGraphUnits())
	}
	for _, scc := range want.SCCs {
		if scc.Recursive {
			t.Fatalf("acyclic catalog produced recursive SCC at %v", scc.Anchor.Cell)
		}
	}

	// Canonical identity and edge order cannot depend on catalog insertion.
	permuted, permutedRoot := permuteObserverTestNodes(nodes, 0, []int{2, 0, 3, 1})
	got := assembleObserverTestForest(t, permuted, permutedRoot)
	if !reflect.DeepEqual(got, want) {
		t.Fatal("observer forest depends on catalog insertion order")
	}
}

func TestLexicalObserverForestMutualRecursionUsesMuReferences(t *testing.T) {
	nodes := observerTestNodes(3, [][2]int{{0, 1}, {1, 2}, {2, 1}})
	forest := assembleObserverTestForest(t, nodes, 0)
	if len(forest.SCCs) != 2 || countRecursiveObserverSCCs(forest) != 1 {
		t.Fatalf("SCCs/recursive = %d/%d, want 2/1", len(forest.SCCs), countRecursiveObserverSCCs(forest))
	}
	if countObserverTargets(forest, lexicalObserverMuTarget) != 2 || countObserverTargets(forest, lexicalObserverTemplateTarget) != 1 {
		t.Fatalf("mu/template edges = %d/%d, want 2/1", countObserverTargets(forest, lexicalObserverMuTarget), countObserverTargets(forest, lexicalObserverTemplateTarget))
	}
}

func TestLexicalObserverForestNestedRecursiveFamilyStaysFinite(t *testing.T) {
	// root -> outer -> (left <-> right) -> leaf, plus one uncalled body.
	nodes := observerTestNodes(6, [][2]int{{0, 1}, {1, 2}, {2, 3}, {3, 2}, {3, 4}})
	forest := assembleObserverTestForest(t, nodes, 0)
	if len(forest.Diagnostic) != 5 || len(forest.Uncalled) != 1 || countRecursiveObserverSCCs(forest) != 1 {
		t.Fatalf("diagnostic/uncalled/recursive = %d/%d/%d, want 5/1/1", len(forest.Diagnostic), len(forest.Uncalled), countRecursiveObserverSCCs(forest))
	}
	if forest.Work.RetainedGraphUnits() != 10 {
		t.Fatalf("retained graph units = %d, want 5 bodies + 5 calls", forest.Work.RetainedGraphUnits())
	}
}

func TestLexicalObserverForestFigureEightIsLinearInBodiesAndCalls(t *testing.T) {
	// a<->b and a<->c form one SCC. An invocation-path tree is infinite; the
	// observer forest must retain the three equations and four recursive edges
	// exactly once, plus the root ingress.
	nodes := observerTestNodes(4, [][2]int{{0, 1}, {1, 2}, {2, 1}, {1, 3}, {3, 1}})
	forest := assembleObserverTestForest(t, nodes, 0)
	if len(forest.Diagnostic) != 4 || forest.Work.DiagnosticEdges != 5 || forest.Work.RetainedGraphUnits() != 9 {
		t.Fatalf("nodes/edges/units = %d/%d/%d, want 4/5/9", len(forest.Diagnostic), forest.Work.DiagnosticEdges, forest.Work.RetainedGraphUnits())
	}
	if countRecursiveObserverSCCs(forest) != 1 || countObserverTargets(forest, lexicalObserverMuTarget) != 4 {
		t.Fatalf("recursive SCC/mu edges = %d/%d, want 1/4", countRecursiveObserverSCCs(forest), countObserverTargets(forest, lexicalObserverMuTarget))
	}
	var recursive lexicalObserverSCC
	for _, scc := range forest.SCCs {
		if scc.Recursive {
			recursive = scc
		}
	}
	for _, template := range forest.Diagnostic {
		for _, edge := range template.Calls {
			if edge.Target.Kind == lexicalObserverMuTarget && edge.Target.Mu.Anchor != recursive.Anchor {
				t.Fatalf("mu anchor = %v, want canonical SCC anchor %v", edge.Target.Mu.Anchor.Cell, recursive.Anchor.Cell)
			}
		}
	}
}

func observerTestNodes(count int, edges [][2]int) []lexicalObserverCatalogNode {
	nodes := make([]lexicalObserverCatalogNode, count)
	for index := range nodes {
		var bodyID lexicalidentity.StableLexicalBodyID
		bodyID[len(bodyID)-1] = byte(index + 1)
		nodes[index].ref = lexicalObserverTemplateRef{Body: bodyID, Cell: transformer.CellRef{Function: uint64(index + 1)}}
	}
	for ordinal, pair := range edges {
		point := cfg.Point(ordinal + 1)
		nodes[pair[0]].callRefs = append(nodes[pair[0]].callRefs, lexicalObserverCatalogEdge{
			point: point,
			occurrence: observation.Occurrence{
				Point: wir.DebugPointID{Ordinal: uint32(ordinal + 1), Phase: wir.DebugPhaseCall},
				Kind:  observation.CallInvocation,
			},
			target: pair[1],
		})
	}
	return nodes
}

func permuteObserverTestNodes(nodes []lexicalObserverCatalogNode, root int, order []int) ([]lexicalObserverCatalogNode, int) {
	remap := make([]int, len(nodes))
	for target, source := range order {
		remap[source] = target
	}
	out := make([]lexicalObserverCatalogNode, len(nodes))
	for target, source := range order {
		out[target] = nodes[source]
		out[target].callRefs = append([]lexicalObserverCatalogEdge(nil), nodes[source].callRefs...)
		for edge := range out[target].callRefs {
			out[target].callRefs[edge].target = remap[out[target].callRefs[edge].target]
		}
	}
	return out, remap[root]
}

func assembleObserverTestForest(t *testing.T, nodes []lexicalObserverCatalogNode, root int) lexicalObserverForest {
	t.Helper()
	calls := 0
	for index := range nodes {
		calls += len(nodes[index].callRefs)
	}
	forest, err := assembleLexicalObserverForest(context.Background(), nodes, root, lexicalObserverWork{CatalogTemplates: len(nodes), CallSitesScanned: calls})
	if err != nil {
		t.Fatal(err)
	}
	return forest
}

func countRecursiveObserverSCCs(forest lexicalObserverForest) int {
	count := 0
	for _, scc := range forest.SCCs {
		if scc.Recursive {
			count++
		}
	}
	return count
}

func countObserverTargets(forest lexicalObserverForest, kind lexicalObserverCallTargetKind) int {
	count := 0
	for _, template := range forest.Diagnostic {
		for _, edge := range template.Calls {
			if edge.Target.Kind == kind {
				count++
			}
		}
	}
	return count
}
