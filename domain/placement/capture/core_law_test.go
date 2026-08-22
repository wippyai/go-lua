package capture

import (
	"testing"

	"github.com/wippyai/go-lua/domain/placement"
)

var (
	captureRoutePlanLawPlan routePlan
	captureRoutePlanLawOK   bool
)

func TestCapturePlacementIsTheLeastUpperBoundOfClosureAndSource(t *testing.T) {
	cases := []struct {
		closure placement.Placement
		source  placement.Placement
		want    placement.Placement
	}{
		{placement.Stack, placement.Stack, placement.Stack},
		{placement.Stack, placement.OwnedHeap, placement.OwnedHeap},
		{placement.OwnedHeap, placement.Stack, placement.OwnedHeap},
		{placement.OwnedHeap, placement.SharedHeap, placement.SharedHeap},
		{placement.SharedHeap, placement.OwnedHeap, placement.SharedHeap},
		{placement.Unknown, placement.Stack, placement.Unknown},
	}
	for _, item := range cases {
		if got, ok := captureValue(item.closure, item.source); !ok || got != item.want {
			t.Fatalf("capture(%s,%s) = %s/%t, want %s/true", item.closure, item.source, got, ok, item.want)
		}
	}
}

func TestCapturePlacementDoesNotSynthesizeAnAbsentSource(t *testing.T) {
	// captureValue is only reached after the selected source cell has been
	// authenticated. Its value-level contract therefore has no absent-source
	// compensation branch; an absent cell is refused by route planning.
	if got, ok := captureValue(placement.OwnedHeap, placement.SharedHeap); !ok || got != placement.SharedHeap {
		t.Fatalf("authenticated source capture = %s/%t, want SharedHeap/true", got, ok)
	}
}

func TestCaptureExactRoutePlanKeepsDenseOrderAndDeduplicatesInline(t *testing.T) {
	var plan routePlan
	for _, tag := range []routeTag{4, 1, 3, 2, 3, 1} {
		if !plan.addRoute(route{tag: tag}) {
			t.Fatalf("route tag %d was rejected", tag)
		}
	}
	if plan.routeCount() != 4 || len(plan.spill) != 0 {
		t.Fatalf("inline exact plan count/spill = %d/%d, want 4/0", plan.routeCount(), len(plan.spill))
	}
	for index, want := range []routeTag{1, 2, 3, 4} {
		item, itemOK := plan.routeAt(index)
		if !itemOK || item.tag != want {
			t.Fatalf("inline route %d = %d/%t, want %d/true", index, item.tag, itemOK, want)
		}
	}
	for tag := routeTag(5); tag <= routeTag(captureRouteInlineCapacity+4); tag++ {
		if !plan.addRoute(route{tag: tag}) {
			t.Fatalf("overflow route tag %d was rejected", tag)
		}
	}
	if plan.routeCount() != captureRouteInlineCapacity+4 || len(plan.spill) != 4 {
		t.Fatalf("overflow exact plan count/spill = %d/%d, want %d/4", plan.routeCount(), len(plan.spill), captureRouteInlineCapacity+4)
	}
}

func TestCaptureExactRoutePlanInlineReductionAllocatesZeroPerCall(t *testing.T) {
	allocs := testing.AllocsPerRun(100, func() {
		var plan routePlan
		ok := true
		for tag := routeTag(1); tag <= routeTag(4); tag++ {
			ok = ok && plan.addRoute(route{tag: tag})
		}
		captureRoutePlanLawPlan = plan
		captureRoutePlanLawOK = ok && plan.routeCount() == 4
	})
	if !captureRoutePlanLawOK || captureRoutePlanLawPlan.routeCount() != 4 || allocs != 0 {
		t.Fatalf("inline exact route reduction = count %d/%t allocations %f, want 4/true and zero", captureRoutePlanLawPlan.routeCount(), captureRoutePlanLawOK, allocs)
	}
}
