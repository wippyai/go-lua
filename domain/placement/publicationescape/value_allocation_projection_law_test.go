package publicationescape

import (
	"testing"

	"github.com/wippyai/go-lua/domain/materialization"
	placementdomain "github.com/wippyai/go-lua/domain/placement"
	valuedomain "github.com/wippyai/go-lua/domain/value"
)

func TestValueAllocationProjectionIsCanonicalOwnerFencedAndExplicit(t *testing.T) {
	fixture := newPublicationEscapeFixture(t)
	if len(fixture.allocations) < 2 {
		t.Fatal("projection fixture has fewer than two allocation roots")
	}

	first, firstOK := fixture.values.Allocation(fixture.allocations[0], materialization.Recent)
	second, secondOK := fixture.values.Allocation(fixture.allocations[1], materialization.Summary)
	if !firstOK || !secondOK {
		t.Fatal("allocation atoms")
	}
	// Repeating the first root through another materialization role must not
	// duplicate its Placement route.
	relation, relationOK := fixture.values.Alternatives(first, second, first)
	if !relationOK {
		t.Fatal("allocation relation")
	}
	projection, projectionOK := placementdomain.ProjectValueAllocations(fixture.placement, fixture.values, relation)
	if !projectionOK || !projection.Valid() || projection.IsBottom() || projection.IsTop() || projection.HasOpaque() || projection.Widened() {
		t.Fatalf("exact projection = %#v/%t, want finite exact roots", projection, projectionOK)
	}
	if projection.ExactCount() != 2 {
		t.Fatalf("exact root count = %d, want 2", projection.ExactCount())
	}
	for index, want := range fixture.allocations[:2] {
		got, gotOK := projection.ExactAt(index)
		if !gotOK || got != want {
			t.Fatalf("exact root[%d] = %#v/%t, want %#v", index, got, gotOK, want)
		}
	}

	opaqueAtom, opaqueOK := fixture.values.OpaqueReference(valuedomain.ReferenceTable)
	if !opaqueOK {
		t.Fatal("opaque atom")
	}
	opaque, opaqueValueOK := fixture.values.Singleton(opaqueAtom)
	if !opaqueValueOK {
		t.Fatal("opaque value")
	}
	opaqueProjection, opaqueProjectionOK := placementdomain.ProjectValueAllocations(fixture.placement, fixture.values, opaque)
	if !opaqueProjectionOK || !opaqueProjection.Valid() || !opaqueProjection.HasOpaque() || !opaqueProjection.Widened() || opaqueProjection.IsTop() {
		t.Fatalf("opaque projection = %#v/%t, want explicit opaque widening", opaqueProjection, opaqueProjectionOK)
	}

	topProjection, topOK := placementdomain.ProjectValueAllocations(fixture.placement, fixture.values, fixture.values.Top())
	if !topOK || !topProjection.IsTop() || !topProjection.Widened() || topProjection.HasOpaque() {
		t.Fatalf("Top projection = %#v/%t, want authenticated Top widening", topProjection, topOK)
	}
	bottomProjection, bottomOK := placementdomain.ProjectValueAllocations(fixture.placement, fixture.values, fixture.values.Bottom())
	if !bottomOK || !bottomProjection.IsBottom() || bottomProjection.Widened() || bottomProjection.ExactCount() != 0 {
		t.Fatalf("Bottom projection = %#v/%t, want empty non-widened projection", bottomProjection, bottomOK)
	}
}

func TestValueAllocationProjectionRefusesForeignAndMalformedFacts(t *testing.T) {
	local := newPublicationEscapeFixture(t)
	foreign := newPublicationEscapeFixture(t)

	if _, ok := placementdomain.ProjectValueAllocations(local.placement, foreign.values, foreign.values.Top()); ok {
		t.Fatal("foreign Value authority crossed the Placement/Heap fence")
	}
	if _, ok := placementdomain.ProjectValueAllocations(local.placement, local.values, valuedomain.Value{}); ok {
		t.Fatal("malformed/unavailable Value fact was accepted")
	}
}
