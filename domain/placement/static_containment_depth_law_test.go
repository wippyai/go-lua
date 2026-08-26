package placement

import (
	"reflect"
	"testing"

	"github.com/wippyai/go-lua/analysis/identity"
	heapdomain "github.com/wippyai/go-lua/domain/heap"
)

func TestSharedStaticHeapGraphProjectsAllocationDepthAndBootFreezeIndependently(t *testing.T) {
	// Dense roots are allocation 0, Boot 1, allocation 2. Depth excludes the
	// Boot edge, while DeepFrozen must retain it: the mutable Boot descendant
	// refutes allocation 0 without changing either allocation's finite depth.
	graph := staticHeapGraph{
		evidence: []AllocationEvidence{
			{Kind: AllocationKindTable, HasKind: true},
			{},
			{Kind: AllocationKindTable, HasKind: true},
		},
		allocationDense:     []int{0, 2},
		allocationOrdinal:   []int{0, -1, 1},
		adjacency:           [][]int{{1}, {}, {}},
		allocationAdjacency: [][]int{{}, {}},
		deepLocal:           []EvidenceState{EvidenceProven, EvidenceRefuted, EvidenceProven},
		cellsComplete:       true,
		depthComplete:       true,
	}
	wantAdjacency := [][]int{{1}, {}, {}}
	wantAllocationAdjacency := [][]int{{}, {}}

	var scratch containmentSCCScratch
	depth, known := graph.depthStatesWithScratch(&scratch)
	deep := graph.deepStatesWithScratch(&scratch)
	if !known[0] || depth[0] != 0 || !known[1] || depth[1] != 0 {
		t.Fatalf("allocation depth = %#v/%#v, want two exact depth-zero roots", depth, known)
	}
	if deep[0] != EvidenceRefuted || deep[2] != EvidenceProven {
		t.Fatalf("deep-frozen projection = %#v, want Boot refutation/proven", deep)
	}
	if !reflect.DeepEqual(graph.adjacency, wantAdjacency) || !reflect.DeepEqual(graph.allocationAdjacency, wantAllocationAdjacency) {
		t.Fatalf("shared graph mutated: dense=%#v allocation=%#v", graph.adjacency, graph.allocationAdjacency)
	}
}

func TestStaticContainmentEvidenceReducesAlternativeWorldsAtItsOwner(t *testing.T) {
	base := AllocationEvidence{
		Kind:             AllocationKindTable,
		HasKind:          true,
		OwnerIdentity:    identity.ContentID{1},
		HasOwnerIdentity: true,
	}
	first := base
	first.Depth, first.HasDepth = 1, true
	first.DeepFrozen = EvidenceProven
	second := base
	second.Depth, second.HasDepth = 3, true
	second.DeepFrozen = EvidenceRefuted

	merged, ok := mergeStaticContainmentEvidence(first, second)
	if !ok || !merged.Valid() || !merged.HasDepth || merged.Depth != 3 || merged.DeepFrozen != EvidenceRefuted {
		t.Fatalf("alternative Heap worlds = %#v/%t, want maximum depth and conjunctive refutation", merged, ok)
	}
	otherProducer := base
	otherProducer.DiesBeforeSuspension = EvidenceProven
	merged, ok = mergeStaticContainmentEvidence(otherProducer, second)
	if !ok || merged.DiesBeforeSuspension != EvidenceProven || merged.Depth != second.Depth || merged.DeepFrozen != second.DeepFrozen {
		t.Fatalf("first containment row did not preserve independent evidence: %#v/%t", merged, ok)
	}

	unknownDepth := base
	unknownDepth.DeepFrozen = EvidenceUnknown
	merged, ok = mergeStaticContainmentEvidence(first, unknownDepth)
	if !ok || merged.HasDepth || merged.Depth != 0 || merged.DeepFrozen != EvidenceUnknown {
		t.Fatalf("world with unknown depth = %#v/%t, want absent exact depth and authenticated Unknown", merged, ok)
	}

	foreign := second
	foreign.OwnerIdentity = identity.ContentID{2}
	if merged, ok = mergeStaticContainmentEvidence(first, foreign); ok || merged.Valid() {
		t.Fatalf("foreign containment evidence crossed owner reduction: %#v/%t", merged, ok)
	}
}

func TestFiniteContainmentDepthsChainAndSharedDescendant(t *testing.T) {
	// The graph is already the authenticated Heap projection produced by the
	// public producer.  This law isolates the deterministic longest-path
	// solver: 0 -> 1 -> 2 and 0 -> 3, 1 -> 3 gives the shared node depth 2.
	depths, known := finiteContainmentDepths([][]int{{1, 3}, {2, 3}, {}, {}})
	wantDepths := []uint32{0, 1, 2, 2}
	for index := range wantDepths {
		if !known[index] || depths[index] != wantDepths[index] {
			t.Fatalf("depth[%d] = %d/%v, want %d/true", index, depths[index], known[index], wantDepths[index])
		}
	}
}

func TestFiniteContainmentDepthsRecursiveSCCIsConservative(t *testing.T) {
	// 0 <-> 1 is recursive and 1 -> 2 is downstream.  An unrelated tree
	// remains exact, so a cycle does not erase evidence for every allocation.
	depths, known := finiteContainmentDepths([][]int{{1}, {0, 2}, {}, {}})
	if known[0] || known[1] || known[2] {
		t.Fatalf("recursive SCC/downstream depths = %#v/%#v, want unknown", depths, known)
	}
	if !known[3] || depths[3] != 0 {
		t.Fatalf("unrelated acyclic depth = %d/%v, want 0/true", depths[3], known[3])
	}

	depths, known = finiteContainmentDepths([][]int{{0}})
	if known[0] || depths[0] != 0 {
		t.Fatalf("self-cycle depth = %d/%v, want 0/false", depths[0], known[0])
	}
}

func TestStaticContainmentDepthEvidenceRejectsUnavailableSchemaAndMissingRootInput(t *testing.T) {
	if graph, ok := buildStaticHeapGraphRows(Schema{}, 0, absentHeapValueAt); ok || len(graph.evidence) != 0 {
		t.Fatal("unavailable Placement schema produced a static Heap graph")
	}

	// Applying against an unavailable schema cannot manufacture a complete
	// relation or turn a missing root into a negative fact.
	if _, ok := AccumulatePlacementSummaryContainmentCached(nil, Schema{}, PlacementSummaryObservation{}, 0, absentHeapValueAt); ok {
		t.Fatal("unavailable schema accepted a depth application")
	}
	depths, known := finiteContainmentDepths([][]int{{1}, {99}})
	if depths != nil || known != nil {
		t.Fatalf("missing root edge produced %#v/%#v; want refusal", depths, known)
	}
}

func TestStaticContainmentProjectionRefusesIncompleteGraph(t *testing.T) {
	cases := []staticHeapGraph{
		{cellsComplete: false},
		{
			cellsComplete:       true,
			evidence:            []AllocationEvidence{{}},
			allocationOrdinal:   []int{0},
			allocationDense:     []int{0},
			adjacency:           [][]int{{}},
			deepLocal:           nil,
			allocationAdjacency: [][]int{{}},
		},
	}
	for index, graph := range cases {
		if projection, ok := projectStaticContainmentGraph(graph); ok || projection.deepStates != nil {
			t.Fatalf("malformed graph %d produced projection %#v/%t; want refusal", index, projection, ok)
		}
	}
}

func TestFiniteContainmentDepthsZeroAllocationGraph(t *testing.T) {
	depths, known := finiteContainmentDepths(nil)
	if len(depths) != 0 || len(known) != 0 {
		t.Fatalf("zero allocation graph = %d/%d, want empty", len(depths), len(known))
	}
}

func TestFiniteContainmentDepthsSparseBottomRowsAreKnownRoots(t *testing.T) {
	// Heap's identity summary represents an absent cell as its admitted
	// Default, and Heap's Default is Bottom. A Bottom row has no worlds and
	// therefore no containment edges; it is a known depth-zero allocation,
	// rather than an unavailable relation that suppresses every depth column.
	depths, known := finiteContainmentDepths([][]int{{}, {}, {}})
	for index := range known {
		if !known[index] || depths[index] != 0 {
			t.Fatalf("sparse Bottom row %d = %d/%v, want 0/true", index, depths[index], known[index])
		}
	}
}

func TestFiniteContainmentDepthsRejectsNonCanonicalEdges(t *testing.T) {
	for _, adjacency := range [][][]int{
		{{1, 1, 2}, {2}, {}},
		{{2, 1}, {}, {}},
	} {
		depths, known := finiteContainmentDepths(adjacency)
		if depths != nil || known != nil {
			t.Fatalf("non-canonical edges %#v produced %#v/%#v; want refusal", adjacency, depths, known)
		}
	}
}

func TestFiniteContainmentDepthsLongChainIsIterative(t *testing.T) {
	const count = 16_384
	adjacency := make([][]int, count)
	for node := 0; node+1 < count; node++ {
		adjacency[node] = []int{node + 1}
	}
	depths, known := finiteContainmentDepths(adjacency)
	if !known[0] || depths[0] != 0 || !known[count-1] || depths[count-1] != count-1 {
		t.Fatalf("long chain endpoints = %d/%v and %d/%v, want 0/true and %d/true", depths[0], known[0], depths[count-1], known[count-1], count-1)
	}
	for index := range known {
		if !known[index] || depths[index] != uint32(index) {
			t.Fatalf("long chain depth[%d] = %d/%v, want %d/true", index, depths[index], known[index], index)
		}
	}
}

// absentHeapValueAt is the row accessor of a delivery that answers no
// coordinate: the vector a missing Heap predecessor delivers.
func absentHeapValueAt(int) (heapdomain.Value, bool, bool) {
	var none heapdomain.Value
	return none, false, false
}
