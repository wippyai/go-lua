package publicationescape

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/program/target/vocabulary"
	heapdomain "github.com/wippyai/go-lua/domain/heap"
	"github.com/wippyai/go-lua/domain/placement"
)

var lazyAllRootAllocationSink int

func TestAllRootsWithRequirementIsLazyAndOwnerBacked(t *testing.T) {
	fixture := newPublicationEscapeFixture(t)
	routes, ok := broadcastAllRoots(fixture.placement, placement.SharedHeap)
	if !ok || !routes.allRoot {
		t.Fatalf("broadcast all-root plan = %#v/%t, want lazy owner plan", routes, ok)
	}
	if routes.count != 0 || routes.overflow != nil {
		t.Fatalf("lazy all-root plan copied exact routes: count=%d overflow=%v", routes.count, routes.overflow)
	}
	allocationCount := 0
	for dense := 0; dense < fixture.placement.DenseKeyCount(); dense++ {
		key, keyOK := fixture.placement.KeyAt(dense)
		if keyOK && key.Kind() == heapdomain.RootAllocation {
			allocationCount++
		}
	}
	if allocationCount == 0 || routes.len() != allocationCount {
		t.Fatalf("lazy all-root length=%d, owner allocation roots=%d", routes.len(), allocationCount)
	}
	for index := 0; index < routes.len(); index++ {
		route, routeOK := routes.at(index)
		if !routeOK || route.unknown || route.required != placement.SharedHeap || route.key.Kind() != heapdomain.RootAllocation {
			t.Fatalf("lazy all-root route[%d]=%#v/%t, want SharedHeap root", index, route, routeOK)
		}
	}
	allocations := testing.AllocsPerRun(100, func() {
		planned, plannedOK := broadcastAllRoots(fixture.placement, placement.SharedHeap)
		if plannedOK {
			lazyAllRootAllocationSink = planned.len()
		}
	})
	if allocations != 0 {
		t.Fatalf("lazy all-root planning allocations = %v, want zero", allocations)
	}
}

func TestAllRootAtRetainsDefensiveNonPrefixScan(t *testing.T) {
	fixture := newPublicationEscapeFixture(t)
	routes, ok := broadcastAllRoots(fixture.placement, placement.SharedHeap)
	if !ok || !routes.allRoot || !routes.allRootPrefix {
		t.Fatalf("fixture did not establish validated allocation prefix: %#v/%t", routes, ok)
	}
	prefixRoutes := routes
	routes.allRootPrefix = false
	for index := 0; index < routes.len(); index++ {
		want, wantOK := prefixRoutes.at(index)
		got, gotOK := routes.at(index)
		if !wantOK || !gotOK || got.key != want.key || got.tag != want.tag || got.required != want.required || got.unknown != want.unknown {
			t.Fatalf("non-prefix fallback route[%d]=%#v/%t, prefix route=%#v/%t", index, got, gotOK, want, wantOK)
		}
	}
}

func TestAllRootRouteSetIsAllocationFree(t *testing.T) {
	fixture := newPublicationEscapeFixture(t)
	prepared := &PreparedBatch{
		rows:     []publicationRow{{id: identity.ContentID{3}, requirement: placement.SharedHeap, operation: 1, subjectOpen: true}},
		byTag:    map[sourceTag]sourceSpec{},
		prepared: true,
	}
	gate := operationGateForTest(1)
	allocations := testing.AllocsPerRun(100, func() {
		routes, routesOK := routeSetFor(fixture.placement, fixture.values, prepared, gate, factBuffer{})
		if routesOK {
			lazyAllRootAllocationSink = routes.len()
		}
	})
	if allocations != 0 {
		t.Fatalf("all-root routeSet allocations = %v, want zero", allocations)
	}
}

func TestRequirementForEscapeIsNarrowAndConservative(t *testing.T) {
	cases := []struct {
		name      string
		escape    vocabulary.PublicationEscapeDisposition
		require   placement.Placement
		available bool
	}{
		{"return", vocabulary.PublicationEscapeReturn, placement.OwnedHeap, true},
		{"callback", vocabulary.PublicationEscapeCallback, placement.OwnedHeap, true},
		{"send", vocabulary.PublicationEscapeSendTransfer, placement.SharedHeap, true},
		{"none", vocabulary.PublicationEscapeNone, placement.Bottom, false},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			got, ok := requirementForEscape(test.escape)
			if got != test.require || ok != test.available {
				t.Fatalf("requirementForEscape(%v) = %v/%v, want %v/%v", test.escape, got, ok, test.require, test.available)
			}
		})
	}
}

func TestApplyRouteUsesOnlyEscapeDisplacement(t *testing.T) {
	if got, ok := applyRoute(plannedRoute{required: placement.OwnedHeap}, placement.DefaultFact()); !ok || got != (placement.Fact{Class: placement.OwnedHeap, RetainEscape: placement.EvidenceProven}) {
		t.Fatalf("owned route from stack = %v, want owned heap", got)
	}
	owned := placement.Fact{Class: placement.OwnedHeap, RetainEscape: placement.EvidenceRefuted}
	if got, ok := applyRoute(plannedRoute{required: placement.SharedHeap}, owned); !ok || got != (placement.Fact{Class: placement.SharedHeap, RetainEscape: placement.EvidenceProven}) {
		t.Fatalf("send route from owned heap = %v, want shared heap", got)
	}
	if got, ok := applyRoute(plannedRoute{required: placement.Unknown, unknown: true}, placement.DefaultFact()); !ok || got != placement.UnknownFact() {
		t.Fatalf("unknown route from stack = %v, want unknown", got)
	}
}

func TestOperationGateExcludesUnselectedPublicationSources(t *testing.T) {
	firstID := identity.ContentID([32]byte{1})
	secondID := identity.ContentID([32]byte{2})
	firstTag, firstOK := sourceTagFor(firstID)
	secondTag, secondOK := sourceTagFor(secondID)
	if !firstOK || !secondOK {
		t.Fatal("source tag setup")
	}
	prepared := &PreparedBatch{
		sources: []sourceSpec{
			{tag: firstTag, rowID: firstID, operation: vocabulary.Operation(1)},
			{tag: secondTag, rowID: secondID, operation: vocabulary.Operation(2)},
		},
	}
	gate := operationGateForTest(vocabulary.Operation(1))
	sources := prepared.sourcesForGate(gate)
	first, firstFound := sources.at(0)
	if sources.len() != 1 || !firstFound || first.rowID != firstID {
		t.Fatalf("selected source projection = %d, want first row only", sources.len())
	}
	if _, found := sources.find(secondTag); found {
		t.Fatal("unselected publication source crossed the Call gate")
	}
}

func TestOpaqueCallGateLeavesNonKnownRowsWithoutPublicationAuthority(t *testing.T) {
	gate := operationGateForTest(vocabulary.Operation(1))
	gate.opaque = true
	if !gate.admits(vocabulary.Operation(1)) || gate.admits(vocabulary.Operation(2)) || !gate.opaque {
		t.Fatal("opaque Call gate did not preserve exact/unknown distinction")
	}
	prepared := &PreparedBatch{sources: []sourceSpec{{operation: vocabulary.Operation(2)}}}
	sources := prepared.sourcesForGate(gate)
	if sources.len() != 0 {
		t.Fatal("opaque non-known row incorrectly requested an exact publication Value read")
	}
}

func operationGateForTest(operations ...vocabulary.Operation) operationGate {
	var gate operationGate
	for _, operation := range operations {
		gate.add(operation)
	}
	return gate
}

func TestSameRootSendDominatesReturnRequirement(t *testing.T) {
	merged := mergeRoute(
		plannedRoute{required: placement.OwnedHeap, tag: routeTag(1)},
		plannedRoute{required: placement.SharedHeap, tag: routeTag(1)},
	)
	if merged.required != placement.SharedHeap || merged.unknown {
		t.Fatalf("same-root Send/Return merge = %v (unknown=%t), want shared", merged.required, merged.unknown)
	}
}

func TestPublicationSourceTagIsStableAndReceiptScoped(t *testing.T) {
	var first, second [32]byte
	first[0] = 1
	second[0] = 2
	firstID := identity.ContentID(first)
	secondID := identity.ContentID(second)
	left, leftOK := sourceTagFor(firstID)
	repeat, repeatOK := sourceTagFor(firstID)
	right, rightOK := sourceTagFor(secondID)
	if !leftOK || !repeatOK || !rightOK || left != repeat || left == right {
		t.Fatalf("source tags are not stable and receipt-scoped: %v/%v/%v", left, repeat, right)
	}
}
