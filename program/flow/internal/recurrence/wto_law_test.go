package recurrence

import "testing"

func TestHierarchyValidationClaimsCyclicHeaderExactlyOnce(t *testing.T) {
	valid := []hierarchyEvent{
		{Kind: HierarchyEnter, vertex: 0},
		{Kind: HierarchyPoint, vertex: 1},
		{Kind: HierarchyExit, vertex: 0},
	}
	if err := validateHierarchy(valid, []bool{true, true}); err != nil {
		t.Fatalf("valid cyclic header hierarchy rejected: %v", err)
	}
	for _, malformed := range [][]hierarchyEvent{
		{{Kind: HierarchyEnter, vertex: 0}, {Kind: HierarchyPoint, vertex: 0}, {Kind: HierarchyExit, vertex: 0}},
		{{Kind: HierarchyEnter, vertex: 0}, {Kind: HierarchyEnter, vertex: 0}, {Kind: HierarchyExit, vertex: 0}, {Kind: HierarchyExit, vertex: 0}},
	} {
		if err := validateHierarchy(malformed, []bool{true}); err == nil {
			t.Fatal("duplicate cyclic header vertex was accepted")
		}
	}
}

func TestHierarchyRegionUsesExactBracketNotCanonicalComponentHead(t *testing.T) {
	// The outer and inner brackets may belong to the same SCC. A carried
	// phase must use the LCA's actual Exit, not an independently selected
	// canonical decision head.
	events := []hierarchyEvent{
		{Kind: HierarchyEnter, vertex: 0},
		{Kind: HierarchyEnter, vertex: 1},
		{Kind: HierarchyPoint, vertex: 2},
		{Kind: HierarchyExit, vertex: 1},
		{Kind: HierarchyExit, vertex: 0},
	}
	regions, err := hierarchyRegionsFor(events, components{of: []uint32{0, 0, 0}})
	if err != nil {
		t.Fatalf("hierarchy regions: %v", err)
	}
	answers := regions.lcas([]regionLCAQuery{
		{left: regions.nodeRegion[1], right: regions.nodeRegion[2]},
		{left: regions.nodeRegion[0], right: regions.nodeRegion[2]},
	})
	inner := answers[0]
	if inner == 0 {
		t.Fatal("inner carrier LCA unavailable")
	}
	if exit, ok := regions.exit(inner); !ok || exit != 3 {
		t.Fatalf("inner placement Exit = %d/%v, want event 3", exit, ok)
	}
	outer := answers[1]
	if outer == 0 || outer == inner {
		t.Fatal("outer carrier did not retain distinct exact bracket")
	}
	if !regions.contains(outer, 2) || regions.contains(inner, 0) {
		t.Fatal("hierarchy region containment is not exact")
	}
}

func TestBreakResumeMayLeaveItsAnchoredLoopRegion(t *testing.T) {
	// A Break's root→Outcome Arc anchors the synthetic Outcome phase inside
	// the loop bracket. Its carrierless, typed Outcome→resume edge may then
	// leave that bracket; recurrence must not turn that control transfer into
	// an artificial containment requirement or a Mu carrier.
	events := []hierarchyEvent{
		{Kind: HierarchyEnter, vertex: 0},
		{Kind: HierarchyPoint, vertex: 1},
		{Kind: HierarchyExit, vertex: 0},
		{Kind: HierarchyPoint, vertex: 2},
	}
	regions, err := hierarchyRegionsFor(events, components{of: []uint32{0, 0, 1}})
	if err != nil {
		t.Fatalf("hierarchy regions: %v", err)
	}
	loop := regions.nodeRegion[1]
	if loop == 0 || regions.contains(loop, 2) {
		t.Fatal("fixture did not model a Break resume outside its loop bracket")
	}
}

func TestOutcomeMultiCarrierLCAUsesExactExitAndRejectsMalformedBracket(t *testing.T) {
	// Three authentic carriers for one Outcome may span nested regions. Their
	// agreement is the exact common LCA bracket, not a component-head proxy.
	events := []hierarchyEvent{
		{Kind: HierarchyEnter, vertex: 0},
		{Kind: HierarchyEnter, vertex: 1},
		{Kind: HierarchyPoint, vertex: 2},
		{Kind: HierarchyExit, vertex: 1},
		{Kind: HierarchyEnter, vertex: 3},
		{Kind: HierarchyPoint, vertex: 4},
		{Kind: HierarchyExit, vertex: 3},
		{Kind: HierarchyExit, vertex: 0},
	}
	regions, err := hierarchyRegionsFor(events, components{of: []uint32{0, 0, 0, 0, 0}})
	if err != nil {
		t.Fatalf("hierarchy regions: %v", err)
	}
	inner, outer, sibling := regions.nodeRegion[2], regions.nodeRegion[0], regions.nodeRegion[4]
	answers := regions.lcas([]regionLCAQuery{{left: inner, right: outer}, {left: outer, right: sibling}})
	if len(answers) != 2 || answers[0] != outer || answers[1] != outer {
		t.Fatalf("multi-carrier LCAs = %v, want outer region %d", answers, outer)
	}
	if exit, ok := regions.exit(outer); !ok || exit != 7 {
		t.Fatalf("multi-carrier exact Exit = %d/%v, want 7", exit, ok)
	}
	malformed := []hierarchyEvent{{Kind: HierarchyEnter, vertex: 0}, {Kind: HierarchyExit, vertex: 1}}
	if _, err := hierarchyRegionsFor(malformed, components{of: []uint32{0, 0}}); err == nil {
		t.Fatal("malformed conflicting hierarchy bracket was accepted")
	}
}
