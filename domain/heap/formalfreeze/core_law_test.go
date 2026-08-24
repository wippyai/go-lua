package formalfreeze

import (
	"testing"

	"github.com/wippyai/go-lua/domain/heap"
	"github.com/wippyai/go-lua/domain/heap/internal/recentplan"
)

func TestFormalFreezeParamSetCanonicalizesAndDeduplicates(t *testing.T) {
	var set freezeParamSet
	for _, param := range []int{4, 1, 4, 3, 0, 2} {
		if !set.add(param) {
			t.Fatalf("add param %d", param)
		}
	}
	if got := set.count(); got != 5 {
		t.Fatalf("parameter count = %d, want 5", got)
	}
	for index, want := range []int{0, 1, 2, 3, 4} {
		got, ok := set.at(index)
		if !ok || got != want {
			t.Fatalf("parameter %d = %d/%t, want %d/true", index, got, ok, want)
		}
	}
	if set.add(-1) {
		t.Fatal("negative parameter admitted")
	}
}

func TestFormalFreezeRoutePlanCanonicalizesAliasesAndIntersects(t *testing.T) {
	var left, right recentplan.Plan
	for _, tag := range []heap.RawRouteTag{9, 3, 9, 5} {
		if !left.Add(recentplan.Route{Tag: tag}) {
			t.Fatalf("left route tag %d", tag)
		}
	}
	for _, tag := range []heap.RawRouteTag{7, 5, 3} {
		if !right.Add(recentplan.Route{Tag: tag}) {
			t.Fatalf("right route tag %d", tag)
		}
	}
	if got := left.Count(); got != 3 {
		t.Fatalf("left route count = %d, want 3", got)
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
	if _, ok := recentplan.RouteForTag(intersection, 9); ok {
		t.Fatal("non-common route survived intersection")
	}
	if allocations := testing.AllocsPerRun(1000, func() {
		var plan recentplan.Plan
		if !plan.Add(recentplan.Route{Tag: 1}) || !plan.Add(recentplan.Route{Tag: 2}) || !plan.Add(recentplan.Route{Tag: 1}) {
			t.Fatal("inline route plan")
		}
	}); allocations != 0 {
		t.Fatalf("ordinary route plan allocations = %v", allocations)
	}
}

func TestFormalFreezeRoutePlanInlineOverflowRemainsCanonical(t *testing.T) {
	var plan recentplan.Plan
	for tag := formalFreezeInlineWidth + 3; tag > 0; tag-- {
		if !plan.Add(recentplan.Route{Tag: heap.RawRouteTag(tag)}) {
			t.Fatalf("route tag %d", tag)
		}
	}
	if got := plan.Count(); got != formalFreezeInlineWidth+3 {
		t.Fatalf("route count = %d, want %d", got, formalFreezeInlineWidth+3)
	}
	for index := 0; index < plan.Count(); index++ {
		route, ok := plan.At(index)
		if !ok || route.Tag != heap.RawRouteTag(index+1) {
			t.Fatalf("route %d = %d/%t, want %d/true", index, route.Tag, ok, index+1)
		}
	}
}
