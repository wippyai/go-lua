package returnescape

import (
	"sync"
	"testing"

	"github.com/wippyai/go-lua/analysis/engine/execution"
	"github.com/wippyai/go-lua/analysis/schema/structure"
	"github.com/wippyai/go-lua/domain/materialization"
	"github.com/wippyai/go-lua/domain/placement"
	valuedomain "github.com/wippyai/go-lua/domain/value"
)

func TestReturnSparseStackPlacementIsDisplaced(t *testing.T) {
	// A sparse predecessor arrives as the Factor's own declared default, which
	// is the one absence the fold admits; the route it is published at is the
	// relation's answer and not this reducer's.
	current, currentOK := placement.AuthenticateFactCell(placement.DefaultFact(), false, true)
	if !currentOK {
		t.Fatal("sparse Placement default")
	}
	got, outcome := ReturnEscapeFold(1, current)
	want := placement.Fact{Class: placement.OwnedHeap, RetainEscape: placement.EvidenceProven}
	if outcome != structure.Concrete || got != want {
		t.Fatalf("sparse Stack predecessor = %s/%v, want OwnedHeap/Concrete", got, outcome)
	}
	if _, sparseOK := placement.AuthenticateFactCell(placement.BottomFact(), false, true); sparseOK {
		t.Fatal("a sparse non-default predecessor was admitted")
	}
	shared := placement.Fact{Class: placement.SharedHeap, RetainEscape: placement.EvidenceRefuted}
	if got, outcome := ReturnEscapeFold(1, shared); outcome != structure.Concrete ||
		got != (placement.Fact{Class: placement.SharedHeap, RetainEscape: placement.EvidenceProven}) {
		t.Fatalf("known Return policy changed = %s/%v, want SharedHeap/Concrete", got, outcome)
	}
}

// TestReturnExactRoutesStayCanonicalWithSuffixOnlySpill states the shape of
// the generated set: rows arrive in any order and are held ascending by the
// coordinate they address, the inline prefix fills first, and only the excess
// reaches the spill. A row on an address already held adds no ordinal.
func TestReturnExactRoutesStayCanonicalWithSuffixOnlySpill(t *testing.T) {
	var plan derived2Rows
	for _, tag := range []uint64{9, 2, 7, 1, 6, 4, 8, 3, 5} {
		var placed bool
		plan, placed = insertDerived2Row(plan, uint32(tag-1), tag, Route{Tag: tag})
		if !placed {
			t.Fatalf("place route tag %d", tag)
		}
	}
	if derived2Count(plan) != 9 {
		t.Fatalf("exact route count = %d, want 9", derived2Count(plan))
	}
	if got := len(plan.spill); got != 9-len(plan.inline) {
		t.Fatalf("exact spill length = %d, want suffix length %d", got, 9-len(plan.inline))
	}
	for index := 0; index < derived2Count(plan); index++ {
		want := uint64(index + 1)
		route, routeOK := derived2At(plan, index)
		if !routeOK || route.Tag != want {
			t.Fatalf("exact route %d = %#v/%t, want tag %d", index, route, routeOK, want)
		}
	}
	repeated, repeatedOK := insertDerived2Row(plan, 4, 5, Route{Tag: 5})
	if !repeatedOK || derived2Count(repeated) != 9 {
		t.Fatal("one route named twice changed the canonical set")
	}
}

// TestReturnWidenedRoutesAreLazyOwnerSchemaViews is the widening half of the
// allocation bar. A widened answer is the owner's whole directory, which
// already lies in the order this relation is ordered by, so the set records
// that it widened and keeps what one row is resolved from - it copies nothing.
func TestReturnWidenedRoutesAreLazyOwnerSchemaViews(t *testing.T) {
	fixture := newReturnPlanFixture(t)
	plan, ok := returnRoutes(t, fixture, returnCell(fixture.values.Top()))
	if !ok || !plan.widened {
		t.Fatalf("Top route set = %#v/%t, want a widened view", plan, ok)
	}
	if plan.spill != nil || derived2Count(plan) != len(fixture.allocations) {
		t.Fatalf("Top route set copied its rows: spill=%v count=%d", plan.spill, derived2Count(plan))
	}
	if plan.widenPlacementSchema.ContentID() != fixture.placement.ContentID() {
		t.Fatal("Top route set lost the owner Placement schema it resolves through")
	}
	for index := 0; index < derived2Count(plan); index++ {
		route, routeOK := derived2At(plan, index)
		if !routeOK || route.Tag == 0 {
			t.Fatalf("Top route %d unavailable: %#v/%t", index, route, routeOK)
		}
	}
}

func TestReturnRouteSetCommonPathsDoNotAllocate(t *testing.T) {
	fixture := newReturnPlanFixture(t)
	atom, atomOK := fixture.values.Allocation(fixture.allocations[0], materialization.Recent)
	if !atomOK {
		t.Fatal("allocation atom")
	}
	exact, exactOK := fixture.values.Singleton(atom)
	if !exactOK {
		t.Fatal("exact allocation value")
	}
	opaqueAtom, opaqueAtomOK := fixture.values.OpaqueReference(valuedomain.ReferenceTable)
	if !opaqueAtomOK {
		t.Fatal("opaque atom")
	}
	opaque, opaqueOK := fixture.values.Singleton(opaqueAtom)
	if !opaqueOK {
		t.Fatal("opaque value")
	}
	for _, item := range []struct {
		name    string
		fact    valuedomain.Value
		widened bool
		count   int
	}{
		{"exact", exact, false, 1},
		{"top", fixture.values.Top(), true, len(fixture.allocations)},
		{"opaque", opaque, true, len(fixture.allocations)},
	} {
		vector, vectorOK := execution.NewMemberVector([]execution.MemberCell[valuedomain.Value]{returnCell(item.fact)})
		if !vectorOK {
			t.Fatal("member vector")
		}
		var plan derived2Rows
		var planOK bool
		got := testing.AllocsPerRun(100, func() {
			plan, planOK = deriveDerived2Rows(fixture.placement, fixture.values, fixture.boundary, fixture.values.Bottom(), vector)
		})
		if got != 0 || !planOK || plan.widened != item.widened || derived2Count(plan) != item.count {
			t.Fatalf("%s route set allocations=%v plan=%#v/%t", item.name, got, plan, planOK)
		}
	}
}

func TestReturnWidenedRouteSetIsConcurrent(t *testing.T) {
	fixture := newReturnPlanFixture(t)
	plan, planOK := returnRoutes(t, fixture, returnCell(fixture.values.Top()))
	if !planOK || !plan.widened || derived2Count(plan) == 0 {
		t.Fatal("widened return route set")
	}
	// A widened set holds the owner's schema and resolves each member on
	// demand, so reading one concurrently reads that schema concurrently.
	const workers = 8
	const iterations = 100
	failed := make(chan struct{}, 1)
	var wait sync.WaitGroup
	wait.Add(workers)
	for worker := 0; worker < workers; worker++ {
		go func() {
			defer wait.Done()
			for iteration := 0; iteration < iterations; iteration++ {
				route, routeOK := derived2At(plan, iteration%derived2Count(plan))
				if !routeOK || route.Tag == 0 {
					select {
					case failed <- struct{}{}:
					default:
					}
					return
				}
			}
		}()
	}
	wait.Wait()
	select {
	case <-failed:
		t.Fatal("concurrent widened return route set changed")
	default:
	}
}
