package formalfreeze

import (
	"testing"

	"github.com/wippyai/go-lua/domain/heap"
)

func TestFormalFreezeObservationBufferKeepsOrdinaryCallsAllocationFree(t *testing.T) {
	var inline [formalFreezeInlineWidth]actualObservation
	for count := 0; count <= len(inline); count++ {
		observations, ok := formalFreezeObservationBuffer(count, inline[:])
		if !ok || len(observations) != count {
			t.Fatalf("inline buffer width %d = %d/%t", count, len(observations), ok)
		}
		if count != 0 && &observations[0] != &inline[0] {
			t.Fatalf("inline width %d allocated a second image", count)
		}
	}
	if allocations := testing.AllocsPerRun(1000, func() {
		var local [formalFreezeInlineWidth]actualObservation
		if observations, ok := formalFreezeObservationBuffer(len(local), local[:]); !ok || len(observations) != len(local) {
			t.Fatal("inline observation buffer")
		}
	}); allocations != 0 {
		t.Fatalf("ordinary formal-freeze observation buffer allocations = %v", allocations)
	}
	fallback, ok := formalFreezeObservationBuffer(formalFreezeInlineWidth+1, inline[:])
	if !ok || len(fallback) != formalFreezeInlineWidth+1 || &fallback[0] == &inline[0] {
		t.Fatal("wide-call fallback buffer")
	}
	if _, ok := formalFreezeObservationBuffer(-1, inline[:]); ok {
		t.Fatal("negative observation width admitted")
	}
}

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
	var left, right routePlan
	for _, tag := range []heap.RawRouteTag{9, 3, 9, 5} {
		if !left.Add(route{Tag: tag}) {
			t.Fatalf("left route tag %d", tag)
		}
	}
	for _, tag := range []heap.RawRouteTag{7, 5, 3} {
		if !right.Add(route{Tag: tag}) {
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
	if _, ok := routeForTag(intersection, 9); ok {
		t.Fatal("non-common route survived intersection")
	}
	if allocations := testing.AllocsPerRun(1000, func() {
		var plan routePlan
		if !plan.Add(route{Tag: 1}) || !plan.Add(route{Tag: 2}) || !plan.Add(route{Tag: 1}) {
			t.Fatal("inline route plan")
		}
	}); allocations != 0 {
		t.Fatalf("ordinary route plan allocations = %v", allocations)
	}
}

func TestFormalFreezeRoutePlanInlineOverflowRemainsCanonical(t *testing.T) {
	var plan routePlan
	for tag := formalFreezeInlineWidth + 3; tag > 0; tag-- {
		if !plan.Add(route{Tag: heap.RawRouteTag(tag)}) {
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
