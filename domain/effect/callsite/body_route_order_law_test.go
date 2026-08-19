package callsite

import "testing"

// The body route table addresses staged Selection ordinals. A Selection is
// published in the engine's canonical exact-Unit then numeric-tag order, and
// this rule tags every route with the Effect root coordinate its exact Ref is
// issued from, so the sealed table must be ordered by that tag. Call's own
// body-target order is a separate authority and never fixes the ordinal.
func TestBodyRouteTableSealsInCanonicalSelectionOrder(t *testing.T) {
	declared := []bodyRoute{{tag: 1}, {tag: 5}, {tag: 3}}
	ordered, slots, ok := orderBodyRoutes(declared)
	if !ok {
		t.Fatal("seal the declared body route table")
	}
	for index, want := range []uint64{1, 3, 5} {
		if ordered[index].tag != want {
			t.Fatalf("route ordinal %d carries tag %d, canonical Selection order wants %d", index, ordered[index].tag, want)
		}
	}
	if len(slots) != len(declared) {
		t.Fatalf("sealed %d slots for %d declared routes", len(slots), len(declared))
	}
	// Every declared route keeps its own slot across the reordering, so a
	// role resolved through the table still reaches the route it declared.
	for index, route := range declared {
		slot := slots[index]
		if slot == 0 || uint64(slot) > uint64(len(ordered)) || ordered[slot-1].tag != route.tag {
			t.Fatalf("declared route %d with tag %d resolved to slot %d", index, route.tag, slot)
		}
	}
}

// Two routes on one tag address a single Selection ordinal, so the table
// cannot be sealed rather than folding one route's effects twice.
func TestBodyRouteTableRefusesDuplicateTag(t *testing.T) {
	if _, _, ok := orderBodyRoutes([]bodyRoute{{tag: 2}, {tag: 2}}); ok {
		t.Fatal("duplicate route tag sealed a body route table")
	}
}

// A Call algebra with no executable body target seals an empty table.
func TestBodyRouteTableSealsEmptyDeclaration(t *testing.T) {
	ordered, slots, ok := orderBodyRoutes(nil)
	if !ok || len(ordered) != 0 || len(slots) != 0 {
		t.Fatalf("empty body route declaration sealed as ordered=%d slots=%d ok=%v", len(ordered), len(slots), ok)
	}
}
