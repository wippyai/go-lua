package publicationescape

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/program/target/vocabulary"
	"github.com/wippyai/go-lua/domain/placement"
)

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
	if got := applyRoute(plannedRoute{required: placement.OwnedHeap}, placement.Stack); got != placement.OwnedHeap {
		t.Fatalf("owned route from stack = %v, want owned heap", got)
	}
	if got := applyRoute(plannedRoute{required: placement.SharedHeap}, placement.OwnedHeap); got != placement.SharedHeap {
		t.Fatalf("send route from owned heap = %v, want shared heap", got)
	}
	if got := applyRoute(plannedRoute{required: placement.Unknown, unknown: true}, placement.Stack); got != placement.Unknown {
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
	prepared := &preparedBatch{
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

func TestOpaqueCallGateLeavesNonKnownRowsForCanonicalWidening(t *testing.T) {
	gate := operationGateForTest(vocabulary.Operation(1))
	gate.opaque = true
	if !gate.admits(vocabulary.Operation(1)) || gate.admits(vocabulary.Operation(2)) || !gate.opaque {
		t.Fatal("opaque Call gate did not preserve exact/unknown distinction")
	}
	prepared := &preparedBatch{sources: []sourceSpec{{operation: vocabulary.Operation(2)}}}
	sources := prepared.sourcesForGate(gate)
	if sources.len() != 0 {
		t.Fatal("opaque non-known row incorrectly requested an exact Value read")
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
