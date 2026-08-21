package recentplan

import (
	"testing"

	"github.com/wippyai/go-lua/domain/heap"
)

func TestPlanCanonicalizesAliasesAndIntersects(t *testing.T) {
	var left, right Plan
	for _, tag := range []heap.RawRouteTag{9, 3, 9, 5} {
		if !left.Add(Route{Tag: tag}) {
			t.Fatalf("left route tag %d", tag)
		}
	}
	for _, tag := range []heap.RawRouteTag{7, 5, 3} {
		if !right.Add(Route{Tag: tag}) {
			t.Fatalf("right route tag %d", tag)
		}
	}
	if left.Count() != 3 {
		t.Fatalf("left route count = %d, want 3", left.Count())
	}
	for index, want := range []heap.RawRouteTag{3, 5, 9} {
		got, ok := left.At(index)
		if !ok || got.Tag != want {
			t.Fatalf("left route %d = %d/%t, want %d/true", index, got.Tag, ok, want)
		}
	}
	intersection, ok := left.Intersection(right)
	if !ok || intersection.Count() != 2 {
		t.Fatalf("intersection = %d/%t, want 2/true", intersection.Count(), ok)
	}
	for index, want := range []heap.RawRouteTag{3, 5} {
		got, routeOK := intersection.At(index)
		if !routeOK || got.Tag != want {
			t.Fatalf("intersection route %d = %d/%t, want %d/true", index, got.Tag, routeOK, want)
		}
	}
	if _, ok := RouteForTag(intersection, 9); ok {
		t.Fatal("non-common route survived intersection")
	}
	if _, ok := RouteForTag(left, 4); ok {
		t.Fatal("ordered lookup found absent route")
	}
}

func TestPlanInlineOverflowRemainsCanonical(t *testing.T) {
	var plan Plan
	for tag := InlineWidth + 3; tag > 0; tag-- {
		if !plan.Add(Route{Tag: heap.RawRouteTag(tag)}) {
			t.Fatalf("route tag %d", tag)
		}
	}
	if plan.Count() != InlineWidth+3 {
		t.Fatalf("route count = %d, want %d", plan.Count(), InlineWidth+3)
	}
	for index := 0; index < plan.Count(); index++ {
		route, ok := plan.At(index)
		if !ok || route.Tag != heap.RawRouteTag(index+1) {
			t.Fatalf("route %d = %d/%t, want %d/true", index, route.Tag, ok, index+1)
		}
	}
}

func TestPlanOrdinaryPathIsAllocationFree(t *testing.T) {
	if allocations := testing.AllocsPerRun(1000, func() {
		var plan Plan
		if !plan.Add(Route{Tag: 1}) || !plan.Add(Route{Tag: 2}) || !plan.Add(Route{Tag: 1}) {
			t.Fatal("inline route plan")
		}
	}); allocations != 0 {
		t.Fatalf("ordinary route plan allocations = %v", allocations)
	}
}
