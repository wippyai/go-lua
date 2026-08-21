package suspension

import "testing"

var (
	suspensionRouteBufferLawLen  int
	suspensionRouteBufferLawOK   bool
	suspensionRouteBufferLawPlan routePlan
)

func TestSuspensionSourceFactBufferUsesBoundedInlineStorage(t *testing.T) {
	var inline [sourceFactInlineWidth]sourceFact
	facts, ok := sourceFactBuffer(sourceFactInlineWidth, inline[:])
	if !ok || len(facts) != sourceFactInlineWidth || cap(facts) != sourceFactInlineWidth {
		t.Fatalf("inline source facts = len %d cap %d/%t, want %d/%d/true", len(facts), cap(facts), ok, sourceFactInlineWidth, sourceFactInlineWidth)
	}
	facts[0].present = true
	if !inline[0].present {
		t.Fatal("inline source-fact buffer did not alias caller storage")
	}

	wide, wideOK := sourceFactBuffer(sourceFactInlineWidth+1, inline[:])
	if !wideOK || len(wide) != sourceFactInlineWidth+1 || cap(wide) < len(wide) {
		t.Fatalf("wide source facts = len %d cap %d/%t, want len %d and invocation-local storage", len(wide), cap(wide), wideOK, sourceFactInlineWidth+1)
	}
}

func TestSuspensionRouteBufferKeepsDenseOrderAndDeduplicates(t *testing.T) {
	var plan routePlan
	if !plan.add(route{tag: routeTag(7)}) || !plan.add(route{tag: routeTag(2)}) || !plan.add(route{tag: routeTag(7)}) {
		t.Fatal("route insertion rejected a valid alias")
	}
	first, firstOK := plan.at(0)
	second, secondOK := plan.at(1)
	if plan.count() != 2 || !firstOK || !secondOK || first.tag != routeTag(2) || second.tag != routeTag(7) {
		t.Fatalf("ordered routes = %#v/%t, %#v/%t count=%d, want tags [2 7]", first, firstOK, second, secondOK, plan.count())
	}
	for index := 1; index <= routeInlineWidth+2; index++ {
		if !plan.add(route{tag: routeTag(index + 10)}) {
			t.Fatalf("overflow route %d rejected", index)
		}
	}
	if plan.count() != routeInlineWidth+4 || len(plan.extra) != 4 {
		t.Fatalf("overflow plan count/extra = %d/%d, want %d/4", plan.count(), len(plan.extra), routeInlineWidth+4)
	}
}

func TestSuspensionRouteBufferInlineReductionAllocatesZeroPerCall(t *testing.T) {
	allocs := testing.AllocsPerRun(100, func() {
		suspensionRouteBufferLawPlan = routePlan{}
		_ = suspensionRouteBufferLawPlan.add(route{tag: routeTag(3)})
		_ = suspensionRouteBufferLawPlan.add(route{tag: routeTag(1)})
		suspensionRouteBufferLawLen = suspensionRouteBufferLawPlan.count()
		suspensionRouteBufferLawOK = suspensionRouteBufferLawLen == 2
	})
	if !suspensionRouteBufferLawOK || allocs != 0 {
		t.Fatalf("inline route reduction = len %d/%t allocations %f, want 2/true and zero", suspensionRouteBufferLawLen, suspensionRouteBufferLawOK, allocs)
	}
}
