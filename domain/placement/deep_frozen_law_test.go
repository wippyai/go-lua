package placement

import "testing"

func TestFiniteDeepFrozenStatesProvesFrozenCycle(t *testing.T) {
	states := finiteDeepFrozenStates(
		[]EvidenceState{EvidenceProven, EvidenceProven},
		[][]int{{1}, {0}},
	)
	for index, state := range states {
		if state != EvidenceProven {
			t.Fatalf("frozen cycle state[%d] = %v, want proven", index, state)
		}
	}
}

func TestFiniteDeepFrozenStatesRefutesMutableDescendant(t *testing.T) {
	states := finiteDeepFrozenStates(
		[]EvidenceState{EvidenceProven, EvidenceRefuted},
		[][]int{{1}, {}},
	)
	if states[0] != EvidenceRefuted || states[1] != EvidenceRefuted {
		t.Fatalf("mutable descendant states = %#v, want both refuted", states)
	}
}

func TestFiniteDeepFrozenStatesDeduplicatesNoncontiguousSCCEdges(t *testing.T) {
	// Nodes 0 and 2 form one SCC but are not adjacent in node order. Both
	// point at component 1; the condensation must contain one edge, without
	// relying on SCC members being contiguous in the source vector.
	states := finiteDeepFrozenStates(
		[]EvidenceState{EvidenceProven, EvidenceRefuted, EvidenceProven},
		[][]int{{1, 2}, {}, {0, 1}},
	)
	for index, state := range states {
		if state != EvidenceRefuted {
			t.Fatalf("noncontiguous SCC state[%d] = %v, want refuted", index, state)
		}
	}
}

func TestFiniteDeepFrozenStatesRefusesMalformedEdge(t *testing.T) {
	states := finiteDeepFrozenStates(
		[]EvidenceState{EvidenceProven, EvidenceProven},
		[][]int{{99}, {}},
	)
	if states != nil {
		t.Fatalf("malformed edge was compensated as %#v; want refusal", states)
	}
}

func TestFiniteDeepFrozenStatesRefusesEveryMalformedGraphShape(t *testing.T) {
	cases := []struct {
		name      string
		local     []EvidenceState
		adjacency [][]int
	}{
		{name: "row-count", local: []EvidenceState{EvidenceProven}, adjacency: nil},
		{name: "negative-child", local: []EvidenceState{EvidenceProven}, adjacency: [][]int{{-1}}},
		{name: "out-of-range-child", local: []EvidenceState{EvidenceProven}, adjacency: [][]int{{1}}},
		{name: "unsorted-children", local: []EvidenceState{EvidenceProven, EvidenceProven, EvidenceProven}, adjacency: [][]int{{2, 1}, {}, {}}},
		{name: "duplicate-child", local: []EvidenceState{EvidenceProven, EvidenceProven}, adjacency: [][]int{{1, 1}, {}}},
		{name: "invalid-local", local: []EvidenceState{EvidenceState(99)}, adjacency: [][]int{{}}},
	}
	for _, item := range cases {
		t.Run(item.name, func(t *testing.T) {
			if got := finiteDeepFrozenStates(item.local, item.adjacency); got != nil {
				t.Fatalf("malformed graph produced %#v; want refusal", got)
			}
		})
	}
}

func TestDeepFrozenLocalStateRejectsMixedHeader(t *testing.T) {
	if got := deepFrozenLocalState(deepFrozenLocalFacts{unknown: true}); got != EvidenceUnknown {
		t.Fatalf("mixed header state = %v, want unknown", got)
	}
	if got := deepFrozenLocalState(deepFrozenLocalFacts{mutable: true, frozen: true}); got != EvidenceRefuted {
		t.Fatalf("exact mutable witness state = %v, want refuted", got)
	}
}

func TestDeepFrozenLocalStateRefusesMalformedHeader(t *testing.T) {
	if got := deepFrozenLocalState(deepFrozenLocalFacts{invalid: true}); got.Valid() {
		t.Fatalf("malformed header produced valid state %v; want refusal sentinel", got)
	}
}
