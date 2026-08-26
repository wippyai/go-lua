package capture

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/schema/structure"
	heapdomain "github.com/wippyai/go-lua/domain/heap"
	"github.com/wippyai/go-lua/domain/placement"
)

var (
	captureRoutePlanLawPlan RoutePlan
	captureRoutePlanLawOK   bool
)

func TestCapturePlacementIsTheLeastUpperBoundOfClosureAndSource(t *testing.T) {
	cases := []struct {
		closure placement.Fact
		Source  placement.Fact
		want    placement.Fact
	}{
		{placement.DefaultFact(), placement.DefaultFact(), placement.DefaultFact()},
		{placement.DefaultFact(), placement.Fact{Class: placement.OwnedHeap, RetainEscape: placement.EvidenceRefuted}, placement.Fact{Class: placement.OwnedHeap, RetainEscape: placement.EvidenceRefuted}},
		{placement.Fact{Class: placement.OwnedHeap, RetainEscape: placement.EvidenceProven}, placement.DefaultFact(), placement.Fact{Class: placement.OwnedHeap, RetainEscape: placement.EvidenceProven}},
		{placement.Fact{Class: placement.OwnedHeap, RetainEscape: placement.EvidenceProven}, placement.Fact{Class: placement.SharedHeap, RetainEscape: placement.EvidenceRefuted}, placement.Fact{Class: placement.SharedHeap, RetainEscape: placement.EvidenceProven}},
		{placement.Fact{Class: placement.SharedHeap, RetainEscape: placement.EvidenceProven}, placement.Fact{Class: placement.OwnedHeap, RetainEscape: placement.EvidenceRefuted}, placement.Fact{Class: placement.SharedHeap, RetainEscape: placement.EvidenceProven}},
		{placement.UnknownFact(), placement.DefaultFact(), placement.UnknownFact()},
	}
	route := captureLawRoute(t)
	for _, item := range cases {
		got, outcome := CaptureFold(item.closure, route, 1, item.Source)
		if outcome != structure.Concrete || got != item.want {
			t.Fatalf("capture(%s,%s) = %s/%v, want %s/Concrete", item.closure, item.Source, got, outcome, item.want)
		}
	}
}

func TestCapturePlacementDoesNotSynthesizeAnAbsentSource(t *testing.T) {
	// The fold is only reached after the selected source cell has been
	// authenticated. Its value-level contract therefore has no absent-source
	// compensation branch; an absent cell is refused by route planning.
	closure := placement.Fact{Class: placement.OwnedHeap, RetainEscape: placement.EvidenceProven}
	Source := placement.Fact{Class: placement.SharedHeap, RetainEscape: placement.EvidenceRefuted}
	want := placement.Fact{Class: placement.SharedHeap, RetainEscape: placement.EvidenceProven}
	got, outcome := CaptureFold(closure, captureLawRoute(t), 1, Source)
	if outcome != structure.Concrete || got != want {
		t.Fatalf("authenticated Source capture = %s/%v, want SharedHeap/Concrete", got, outcome)
	}
}

// captureLawRoute is one owner-issued root allocation coordinate: the
// destination a capture route publishes at. The fold authenticates the route
// it is handed, so a law over the judgment states it with a real one.
func captureLawRoute(t testing.TB) heapdomain.Key {
	t.Helper()
	fixture := newCaptureRoutePlanBenchmarkFixture(t)
	for index := 0; index < fixture.placement.KeyCount(); index++ {
		key, keyOK := fixture.placement.KeyAt(index)
		if keyOK && key.Kind() == heapdomain.RootAllocation {
			return key
		}
	}
	t.Fatal("capture law fixture publishes no root allocation")
	return heapdomain.Key{}
}

func TestCaptureExactRoutePlanKeepsDenseOrderAndDeduplicatesInline(t *testing.T) {
	var plan RoutePlan
	for _, tag := range []RouteTag{4, 1, 3, 2, 3, 1} {
		if !plan.addRoute(Route{tag: tag}) {
			t.Fatalf("Route tag %d was rejected", tag)
		}
	}
	if plan.RouteCount() != 4 || len(plan.spill) != 0 {
		t.Fatalf("inline exact plan count/spill = %d/%d, want 4/0", plan.RouteCount(), len(plan.spill))
	}
	for index, want := range []RouteTag{1, 2, 3, 4} {
		item, itemOK := plan.RouteAt(index)
		if !itemOK || item.tag != want {
			t.Fatalf("inline Route %d = %d/%t, want %d/true", index, item.tag, itemOK, want)
		}
	}
	for tag := RouteTag(5); tag <= RouteTag(captureRouteInlineCapacity+4); tag++ {
		if !plan.addRoute(Route{tag: tag}) {
			t.Fatalf("overflow Route tag %d was rejected", tag)
		}
	}
	if plan.RouteCount() != captureRouteInlineCapacity+4 || len(plan.spill) != 4 {
		t.Fatalf("overflow exact plan count/spill = %d/%d, want %d/4", plan.RouteCount(), len(plan.spill), captureRouteInlineCapacity+4)
	}
}

func TestCaptureExactRoutePlanInlineReductionAllocatesZeroPerCall(t *testing.T) {
	allocs := testing.AllocsPerRun(100, func() {
		var plan RoutePlan
		ok := true
		for tag := RouteTag(1); tag <= RouteTag(4); tag++ {
			ok = ok && plan.addRoute(Route{tag: tag})
		}
		captureRoutePlanLawPlan = plan
		captureRoutePlanLawOK = ok && plan.RouteCount() == 4
	})
	if !captureRoutePlanLawOK || captureRoutePlanLawPlan.RouteCount() != 4 || allocs != 0 {
		t.Fatalf("inline exact Route reduction = count %d/%t allocations %f, want 4/true and zero", captureRoutePlanLawPlan.RouteCount(), captureRoutePlanLawOK, allocs)
	}
}
