package returnescape

import (
	"sync"
	"testing"

	"github.com/wippyai/go-lua/domain/materialization"
	"github.com/wippyai/go-lua/domain/placement"
	valuedomain "github.com/wippyai/go-lua/domain/value"
)

func TestReturnSparseStackPlacementIsDisplaced(t *testing.T) {
	got, ok := returnValue(placement.DefaultFact(), false, routePlan{class: routeExact})
	want := placement.Fact{Class: placement.OwnedHeap, RetainEscape: placement.EvidenceProven}
	if !ok || got != want {
		t.Fatalf("sparse Stack predecessor = %s/%t, want OwnedHeap/true", got, ok)
	}
	if got, ok := returnValue(placement.BottomFact(), false, routePlan{class: routeExact}); ok || got != placement.BottomFact() {
		t.Fatalf("sparse non-default predecessor = %s/%t, want Bottom/false", got, ok)
	}
	shared := placement.Fact{Class: placement.SharedHeap, RetainEscape: placement.EvidenceRefuted}
	if got, ok := returnValue(shared, true, routePlan{class: routeWidened}); !ok || got != (placement.Fact{Class: placement.SharedHeap, RetainEscape: placement.EvidenceProven}) {
		t.Fatalf("widened identity changed known Return policy = %s/%t, want SharedHeap/true", got, ok)
	}
}

func TestReturnFactsRejectDuplicateIndexedCell(t *testing.T) {
	facts, ok := newReturnFacts(2)
	if !ok || !facts.set(1, returnFact{available: true}) {
		t.Fatal("initial indexed fact")
	}
	if facts.set(1, returnFact{available: true}) {
		t.Fatal("duplicate indexed fact was admitted")
	}
	if item, itemOK := facts.at(0); !itemOK || item.available {
		t.Fatalf("missing indexed fact was not preserved: %#v/%t", item, itemOK)
	}
}

func TestReturnExactRoutesStayCanonicalWithSuffixOnlySpill(t *testing.T) {
	var plan routePlan
	for _, tag := range []routeTag{9, 2, 7, 1, 6, 4, 8, 3, 5} {
		if !plan.addRoute(route{tag: tag}) {
			t.Fatalf("add route tag %d", tag)
		}
	}
	if plan.routeCount() != 9 {
		t.Fatalf("exact route count = %d, want 9", plan.routeCount())
	}
	if got := len(plan.spill); got != 9-len(plan.inline) {
		t.Fatalf("exact spill length = %d, want suffix length %d", got, 9-len(plan.inline))
	}
	for index := 0; index < plan.routeCount(); index++ {
		want := routeTag(index + 1)
		candidate, candidateOK := plan.routeAt(index)
		if !candidateOK || candidate.tag != want {
			t.Fatalf("exact route %d = %#v/%t, want tag %d", index, candidate, candidateOK, want)
		}
		byTag, byTagOK := routeAtTag(plan, want)
		if !byTagOK || byTag != candidate {
			t.Fatalf("exact routeAtTag(%d) = %#v/%t, want %#v/true", want, byTag, byTagOK, candidate)
		}
	}
	if !plan.addRoute(route{tag: 5}) || plan.routeCount() != 9 {
		t.Fatal("duplicate exact route changed the canonical set")
	}
}

func TestReturnWidenedRoutesAreLazyOwnerSchemaViews(t *testing.T) {
	fixture := newReturnPlanFixture(t)
	plan, ok := routePlanFor(fixture.placement, fixture.values, fixture.values.Top())
	if !ok || plan.class != routeWidened || !plan.allRoot {
		t.Fatalf("Top plan = %#v/%t, want widened all-root view", plan, ok)
	}
	if plan.spill != nil || plan.allRootDenseSize != fixture.placement.DenseKeyCount() {
		t.Fatalf("Top plan retained copied routes: spill=%v dense=%d", plan.spill, plan.allRootDenseSize)
	}
	if plan.allRootSchema.ContentID() != fixture.placement.ContentID() {
		t.Fatal("Top plan lost its owner Placement schema")
	}
	for index := 0; index < plan.routeCount(); index++ {
		candidate, candidateOK := plan.routeAt(index)
		if !candidateOK {
			t.Fatalf("Top route %d unavailable", index)
		}
		byTag, byTagOK := routeAtTag(plan, candidate.tag)
		if !byTagOK || byTag != candidate {
			t.Fatalf("Top routeAtTag(%d) = %#v/%t, want %#v/true", candidate.tag, byTag, byTagOK, candidate)
		}
	}
}

func TestReturnRoutePlanCommonPathsDoNotAllocateRouteScratch(t *testing.T) {
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
	var plan routePlan
	var planOK bool
	if got := testing.AllocsPerRun(100, func() {
		plan, planOK = routePlanFor(fixture.placement, fixture.values, exact)
	}); got != 0 || !planOK || plan.class != routeExact || plan.routeCount() != 1 {
		t.Fatalf("exact route planner allocations=%v plan=%#v/%t", got, plan, planOK)
	}
	if got := testing.AllocsPerRun(100, func() {
		plan, planOK = routePlanFor(fixture.placement, fixture.values, fixture.values.Top())
	}); got != 0 || !planOK || plan.class != routeWidened || !plan.allRoot {
		t.Fatalf("Top route planner allocations=%v plan=%#v/%t", got, plan, planOK)
	}
	if got := testing.AllocsPerRun(100, func() {
		plan, planOK = routePlanFor(fixture.placement, fixture.values, opaque)
	}); got != 0 || !planOK || plan.class != routeWidened || !plan.allRoot {
		t.Fatalf("opaque route planner allocations=%v plan=%#v/%t", got, plan, planOK)
	}
}

func TestReturnWidenedRoutePlanIsConcurrent(t *testing.T) {
	fixture := newReturnPlanFixture(t)
	plan, planOK := routePlanFor(fixture.placement, fixture.values, fixture.values.Top())
	if !planOK || !plan.allRoot || plan.routeCount() == 0 {
		t.Fatal("widened return plan")
	}
	const workers = 8
	const iterations = 100
	failed := make(chan struct{}, 1)
	var wait sync.WaitGroup
	wait.Add(workers)
	for worker := 0; worker < workers; worker++ {
		go func() {
			defer wait.Done()
			for iteration := 0; iteration < iterations; iteration++ {
				candidate, candidateOK := plan.routeAt(iteration % plan.routeCount())
				byTag, byTagOK := routeAtTag(plan, candidate.tag)
				if !candidateOK || !byTagOK || byTag != candidate {
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
		t.Fatal("concurrent widened return plan changed")
	default:
	}
}
