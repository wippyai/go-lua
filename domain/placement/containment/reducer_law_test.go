package containment

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/engine/operand"
	"github.com/wippyai/go-lua/analysis/schema/structure"
	heapdomain "github.com/wippyai/go-lua/domain/heap"
	placementdomain "github.com/wippyai/go-lua/domain/placement"
)

// The judgment itself: a contained child takes its parent's placement through
// the container, and declines where the evidence does not prove containment.
func TestContainmentValueUsesOnlyAuthenticatedPlacementFacts(t *testing.T) {
	fixture := newContainmentFixture(t)
	held := mustValue(t, fixture, fixture.roots[0], mustNone(t, fixture.heap), mustUnknown(t, fixture.heap), mustNone(t, fixture.heap))

	result, ok := containmentValue(placementdomain.DefaultFact(), placementdomain.Fact{Class: placementdomain.OwnedHeap, RetainEscape: placementdomain.EvidenceProven}, held)
	if !ok || result.Class != placementdomain.OwnedHeap || result.RetainEscape != placementdomain.EvidenceProven {
		t.Fatalf("contained child result=%v ok=%t", result, ok)
	}
	if result, ok := containmentValue(placementdomain.BottomFact(), placementdomain.DefaultFact(), held); ok || result != placementdomain.BottomFact() {
		t.Fatalf("invalid current result=%v ok=%t", result, ok)
	}
	if result, ok := containmentValue(placementdomain.DefaultFact(), placementdomain.BottomFact(), held); ok || result != placementdomain.BottomFact() {
		t.Fatalf("invalid parent result=%v ok=%t", result, ok)
	}
	if result, ok := containmentValue(placementdomain.DefaultFact(), placementdomain.DefaultFact(), heapdomain.Value{}); ok || result != placementdomain.BottomFact() {
		t.Fatalf("unauthenticated child value result=%v ok=%t", result, ok)
	}
	if result, ok := containmentValue(placementdomain.DefaultFact(), placementdomain.DefaultFact(), fixture.heap.Bottom()); ok || result != placementdomain.BottomFact() {
		t.Fatalf("a parent holding nothing result=%v ok=%t", result, ok)
	}
}

// The declared fold is that same judgment reached through what the reads
// deliver: it recovers the parent this child was routed from out of the tag
// the route owner packed, and refuses evidence it cannot address.
func TestContainmentFoldAddressesItsParentThroughTheRouteTag(t *testing.T) {
	fixture := newContainmentFixture(t)
	root := fixture.roots[0]
	if root.Kind() != heapdomain.RootAllocation {
		t.Fatal("containment law fixture root")
	}
	tag, tagOK := routeTag(0, 1)
	if !tagOK {
		t.Fatal("containment law route tag")
	}

	// A vector pair the fold cannot address refuses before any policy is
	// applied: an unpacked tag names no parent, and an unissued coordinate is
	// not a destination.
	empty := operand.SummaryVector[placementdomain.Fact]{}
	emptyHeaps := operand.SummaryVector[heapdomain.Value]{}
	if result, outcome := ContainmentFold(empty, emptyHeaps, root, tag, placementdomain.DefaultFact()); outcome != structure.Refuse || result != placementdomain.BottomFact() {
		t.Fatalf("closed vectors result=%v outcome=%v", result, outcome)
	}
	placements, placementsOK := operand.NewMemberVector([]operand.MemberCell[placementdomain.Fact]{
		{Value: placementdomain.Fact{Class: placementdomain.OwnedHeap, RetainEscape: placementdomain.EvidenceProven}, Present: true},
		{Value: placementdomain.DefaultFact(), Present: true},
	})
	held := mustValue(t, fixture, root, mustNone(t, fixture.heap), mustUnknown(t, fixture.heap), mustNone(t, fixture.heap))
	heaps, heapsOK := operand.NewMemberVector([]operand.MemberCell[heapdomain.Value]{
		{Value: held, Present: true},
		{Value: fixture.heap.Bottom(), Present: true},
	})
	if !placementsOK || !heapsOK {
		t.Fatal("containment law vectors")
	}
	if result, outcome := ContainmentFold(placements, heaps, heapdomain.Key{}, tag, placementdomain.DefaultFact()); outcome != structure.Refuse || result != placementdomain.BottomFact() {
		t.Fatalf("unissued route result=%v outcome=%v", result, outcome)
	}
	if result, outcome := ContainmentFold(placements, heaps, root, 0, placementdomain.DefaultFact()); outcome != structure.Refuse || result != placementdomain.BottomFact() {
		t.Fatalf("unpacked tag result=%v outcome=%v", result, outcome)
	}

	// Addressed, the fold answers what the judgment answers for the parent the
	// tag names.
	result, outcome := ContainmentFold(placements, heaps, root, tag, placementdomain.DefaultFact())
	if outcome != structure.Concrete || result.Class != placementdomain.OwnedHeap || result.RetainEscape != placementdomain.EvidenceProven {
		t.Fatalf("routed child result=%v outcome=%v", result, outcome)
	}

	// The second parent of the same vectors holds nothing, so the child it
	// would reach is not contained and the fold declines.
	secondTag, secondTagOK := routeTag(1, 0)
	if !secondTagOK {
		t.Fatal("containment law second route tag")
	}
	if result, outcome := ContainmentFold(placements, heaps, root, secondTag, placementdomain.DefaultFact()); outcome != structure.Refuse || result != placementdomain.BottomFact() {
		t.Fatalf("parent holding nothing result=%v outcome=%v", result, outcome)
	}
}
