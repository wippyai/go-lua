package summary

import (
	"testing"

	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/engine/callboundary"
)

// These tests pin the concrete spec wiring of the keyed-fact-set lanes (Key,
// Valid, Prefer, Dominates), on top of the generic combinator law tests in
// analysis/domain/lattice/factset.

func TestEscapeEventLaneNormalizeFiltersAndPrefersStrongestKind(t *testing.T) {
	pa := pathdom.NewPlaceholder(0).Field("a")
	in := []callboundary.EscapeEventFact{
		{Target: pa, Kind: callboundary.EscapeEventStore, Recursive: false},
		{Target: pa, Kind: callboundary.EscapeEventSend, Recursive: false}, // same key: stronger kind wins
		{Target: pa, Kind: callboundary.EscapeEventNone, Recursive: false}, // dropped by Valid
	}
	got := escapeEventLane.Normalize(in)
	if len(got) != 1 || got[0].Kind != callboundary.EscapeEventSend {
		t.Fatalf("normalize = %+v, want single Send fact", got)
	}
}

func TestEscapeEventLaneRecursiveTargetSubsumesDescendant(t *testing.T) {
	pa := pathdom.NewPlaceholder(0).Field("a")
	pab := pa.Field("b")
	ancestor := []callboundary.EscapeEventFact{{Target: pa, Kind: callboundary.EscapeEventSend, Recursive: true}}
	descendant := []callboundary.EscapeEventFact{{Target: pab, Kind: callboundary.EscapeEventSend, Recursive: false}}

	joined := escapeEventLane.Join(ancestor, descendant)
	if len(joined) != 1 || !joined[0].Target.Equal(pa) {
		t.Fatalf("recursive ancestor must subsume descendant, got %+v", joined)
	}
	if !escapeEventLane.LessOrEq(descendant, ancestor) {
		t.Fatalf("descendant set must be below the recursive-ancestor set")
	}
	if escapeEventLane.LessOrEq(ancestor, descendant) {
		t.Fatalf("recursive-ancestor set must not be below the descendant set")
	}
	if !escapeEventLane.Equal(escapeEventLane.Widen(ancestor, descendant), joined) {
		t.Fatalf("widen must equal join")
	}
}

func TestStoreRelationLaneJoinKeepsOnlyShared(t *testing.T) {
	pa := pathdom.NewPlaceholder(0).Field("a")
	pb := pathdom.NewPlaceholder(1).Field("b")
	pc := pathdom.NewPlaceholder(2).Field("c")
	a := []callboundary.StoreRelationFact{{Source: pa, Into: pb}, {Source: pb, Into: pc}}
	b := []callboundary.StoreRelationFact{{Source: pa, Into: pb}, {Source: pc, Into: pa}}

	got := storeRelationLane.Join(a, b)
	if len(got) != 1 || !got[0].Source.Equal(pa) || !got[0].Into.Equal(pb) {
		t.Fatalf("must join must keep only the shared relation, got %+v", got)
	}
	if !storeRelationLane.LessOrEq(a, got) {
		t.Fatalf("a must be below the intersection (a guarantees at least it)")
	}
	if storeRelationLane.LessOrEq(got, a) {
		t.Fatalf("the intersection must not be below the larger set a")
	}
}

func TestPathInvalidationLaneAncestorSubsumesDescendant(t *testing.T) {
	pa := pathdom.NewPlaceholder(0).Field("a")
	pab := pa.Field("b")
	ancestor := []callboundary.PathInvalidationFact{{Path: pa}}
	descendant := []callboundary.PathInvalidationFact{{Path: pab}}

	if got := pathInvalidationLane.Join(ancestor, descendant); len(got) != 1 || !got[0].Path.Equal(pa) {
		t.Fatalf("ancestor path must subsume descendant, got %+v", got)
	}
	if !pathInvalidationLane.LessOrEq(descendant, ancestor) {
		t.Fatalf("descendant invalidation must be below the ancestor invalidation")
	}
	// A non-placeholder path is dropped by Valid.
	if got := pathInvalidationLane.Normalize(descendant); len(got) != 1 {
		t.Fatalf("placeholder descendant should survive normalize alone, got %+v", got)
	}
}
