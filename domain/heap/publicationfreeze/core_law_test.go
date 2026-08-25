package publicationfreeze

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/program/target/vocabulary"
	effectfactor "github.com/wippyai/go-lua/domain/effect/factor"
	heapdomain "github.com/wippyai/go-lua/domain/heap"
)

func TestPublicationFreezeOperationGateProjectsOnlySelectedOperations(t *testing.T) {
	firstID := identity.ContentID([32]byte{1})
	secondID := identity.ContentID([32]byte{2})
	firstRaw, firstOK := effectfactor.PublicationMemberTag(firstID)
	secondRaw, secondOK := effectfactor.PublicationMemberTag(secondID)
	if !firstOK || !secondOK {
		t.Fatal("source tag setup")
	}
	firstTag, secondTag := sourceTag(firstRaw), sourceTag(secondRaw)
	var prepared preparedCall
	if !prepared.sources.add(sourceSpec{tag: firstTag, rowID: firstID, operation: vocabulary.Operation(1)}) ||
		!prepared.sources.add(sourceSpec{tag: secondTag, rowID: secondID, operation: vocabulary.Operation(2)}) {
		t.Fatal("source setup")
	}
	var gate operationGate
	if !gate.add(vocabulary.Operation(1)) {
		t.Fatal("operation gate setup")
	}
	sources := prepared.sourcesForGate(gate)
	first, firstFound := sources.at(0)
	if sources.len() != 1 || !firstFound || first.rowID != firstID {
		t.Fatalf("selected source projection = %d, want first row only", sources.len())
	}
	if _, found := sources.find(secondTag); found {
		t.Fatal("unselected publication source crossed the Call gate")
	}
	opaqueSources := prepared.sourcesForGate(operationGate{inline: gate.inline, count: gate.count, opaque: true})
	if opaqueSources.len() != 0 {
		t.Fatal("opaque Call gate requested exact Value sources")
	}
	unsupportedSources := prepared.sourcesForGate(operationGate{inline: gate.inline, count: gate.count, unsupported: true})
	if unsupportedSources.len() != 0 {
		t.Fatal("unsupported Call alternative requested exact Value sources")
	}
}

func TestPublicationFreezeRoutePlanIntersectsEveryAlternative(t *testing.T) {
	var left, right routePlan
	for _, tag := range []heapdomain.RawRouteTag{9, 3, 9, 5} {
		if !left.Add(route{Tag: tag}) {
			t.Fatalf("left route tag %d", tag)
		}
	}
	for _, tag := range []heapdomain.RawRouteTag{7, 5, 3} {
		if !right.Add(route{Tag: tag}) {
			t.Fatalf("right route tag %d", tag)
		}
	}
	if left.Count() != 3 {
		t.Fatalf("left route count = %d, want 3", left.Count())
	}
	for index, want := range []heapdomain.RawRouteTag{3, 5, 9} {
		got, ok := left.At(index)
		if !ok || got.Tag != want {
			t.Fatalf("left route %d = %d/%t, want %d/true", index, got.Tag, ok, want)
		}
	}
	intersection, ok := left.Intersection(right)
	if !ok || intersection.Count() != 2 {
		t.Fatalf("intersection = %d/%t, want 2/true", intersection.Count(), ok)
	}
	for index, want := range []heapdomain.RawRouteTag{3, 5} {
		got, routeOK := intersection.At(index)
		if !routeOK || got.Tag != want {
			t.Fatalf("intersection route %d = %d/%t, want %d/true", index, got.Tag, routeOK, want)
		}
	}
	if _, ok := routeForTag(intersection, 9); ok {
		t.Fatal("non-common route survived intersection")
	}
}
