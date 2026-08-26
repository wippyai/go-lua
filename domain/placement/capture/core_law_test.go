package capture

import (
	"testing"

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
	for _, item := range cases {
		if got, ok := captureValue(item.closure, item.Source); !ok || got != item.want {
			t.Fatalf("capture(%s,%s) = %s/%t, want %s/true", item.closure, item.Source, got, ok, item.want)
		}
	}
}

func TestCapturePlacementDoesNotSynthesizeAnAbsentSource(t *testing.T) {
	// captureValue is only reached after the selected source cell has been
	// authenticated. Its value-level contract therefore has no absent-source
	// compensation branch; an absent cell is refused by route planning.
	closure := placement.Fact{Class: placement.OwnedHeap, RetainEscape: placement.EvidenceProven}
	Source := placement.Fact{Class: placement.SharedHeap, RetainEscape: placement.EvidenceRefuted}
	want := placement.Fact{Class: placement.SharedHeap, RetainEscape: placement.EvidenceProven}
	if got, ok := captureValue(closure, Source); !ok || got != want {
		t.Fatalf("authenticated Source capture = %s/%t, want SharedHeap/true", got, ok)
	}
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
