package callboundary

import (
	"testing"

	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/presence"
	"github.com/wippyai/go-lua/analysis/engine/state/pathevidence"
	"github.com/wippyai/go-lua/analysis/symbol"
)

// These tests pin the concrete spec wiring of the keyed-fact-set lanes (Key,
// Valid, Prefer, Dominates), on top of the generic combinator law tests in
// analysis/domain/lattice/factset.

func TestEscapeEventLaneNormalizeFiltersAndPrefersStrongestKind(t *testing.T) {
	pa := pathdom.NewPlaceholder(0).Field("a")
	in := []EscapeEventFact{
		{Target: pa, Kind: EscapeEventStore, Recursive: false},
		{Target: pa, Kind: EscapeEventSend, Recursive: false}, // same key: stronger kind wins
		{Target: pa, Kind: EscapeEventNone, Recursive: false}, // dropped by Valid
	}
	got := escapeEventLane.Normalize(in)
	if len(got) != 1 || got[0].Kind != EscapeEventSend {
		t.Fatalf("normalize = %+v, want single Send fact", got)
	}
}

func TestEscapeEventLaneRecursiveTargetSubsumesDescendant(t *testing.T) {
	pa := pathdom.NewPlaceholder(0).Field("a")
	pab := pa.Field("b")
	ancestor := []EscapeEventFact{{Target: pa, Kind: EscapeEventSend, Recursive: true}}
	descendant := []EscapeEventFact{{Target: pab, Kind: EscapeEventSend, Recursive: false}}

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
	a := []StoreRelationFact{{Source: pa, Into: pb}, {Source: pb, Into: pc}}
	b := []StoreRelationFact{{Source: pa, Into: pb}, {Source: pc, Into: pa}}

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

func TestBranchProofLaneAdmitCanonicalizesPerKind(t *testing.T) {
	ph := pathdom.NewPlaceholder(0).Field("x")
	other := pathdom.NewPlaceholder(1)
	concrete := pathdom.NewPath(symbol.ID(7), "arg").Field("f")

	in := []BranchProof{
		{Kind: pathevidence.BranchProofPathPresence, Path: ph, Presence: presence.Present(), Other: other}, // Other cleared
		{Kind: pathevidence.BranchProofPathPresence, Path: concrete, Presence: presence.Present()},         // dropped: non-placeholder
		{Kind: pathevidence.BranchProofPathEqual, Path: ph, Other: other, Presence: presence.Present()},    // Presence cleared
		{Kind: pathevidence.BranchProofIndexInRange, Path: ph, Other: other, Presence: presence.Present()}, // Presence cleared
		{Kind: pathevidence.BranchProofIndexInRange, Path: ph, Other: concrete},                            // dropped: non-placeholder Other
	}
	got := branchProofLane.Normalize(in)
	if len(got) != 3 {
		t.Fatalf("expected 3 admitted proofs, got %d: %+v", len(got), got)
	}
	for _, p := range got {
		switch p.Kind {
		case pathevidence.BranchProofPathPresence:
			if !p.Other.IsEmpty() {
				t.Fatalf("presence proof must clear Other, got %+v", p)
			}
		case pathevidence.BranchProofPathEqual, pathevidence.BranchProofIndexInRange:
			if !p.Presence.IsBottom() {
				t.Fatalf("relational proof must clear Presence, got %+v", p)
			}
			if !p.Other.IsPlaceholder() {
				t.Fatalf("relational proof must keep a placeholder Other, got %+v", p)
			}
		}
	}
}

func TestNumFloorFactsUseMustFloorSemantics(t *testing.T) {
	p0 := pathdom.NewPlaceholder(0)
	p1 := pathdom.NewPlaceholder(1)
	concrete := pathdom.NewPath(symbol.ID(7), "arg")

	normalized := numFloorLane.Normalize([]NumFloorFact{
		{Path: concrete, Floor: 9}, // dropped: not a placeholder boundary path
		{Path: p0, Floor: 1},
		{Path: p0, Floor: 4}, // same path: stronger local duplicate wins
	})
	if len(normalized) != 1 || !normalized[0].Path.Equal(p0) || normalized[0].Floor != 4 {
		t.Fatalf("normalize num floors = %#v, want $0 >= 4", normalized)
	}

	left := []NumFloorFact{
		{Path: p0, Floor: 4},
		{Path: p1, Floor: 1},
	}
	right := []NumFloorFact{
		{Path: p0, Floor: 2},
	}
	joined := numFloorLane.Join(left, right)
	if len(joined) != 1 || !joined[0].Path.Equal(p0) || joined[0].Floor != 2 {
		t.Fatalf("join num floors = %#v, want common path with weaker floor 2", joined)
	}
	if !numFloorLane.LessOrEq(left, right) {
		t.Fatalf("stronger floor must be <= weaker common floor")
	}
	if numFloorLane.LessOrEq(right, left) {
		t.Fatalf("weaker/missing floors must not be <= stronger floor set")
	}
	if !numFloorLane.LessOrEq(
		[]NumFloorFact{{Path: p0, Floor: 1}, {Path: p0, Floor: 4}},
		[]NumFloorFact{{Path: p0, Floor: 3}},
	) {
		t.Fatalf("LessOrEq must compare strongest duplicate floor without requiring normalized inputs")
	}
}

func TestNumCeilFactsUseMustCeilSemantics(t *testing.T) {
	p0 := pathdom.NewPlaceholder(0)
	p1 := pathdom.NewPlaceholder(1)
	concrete := pathdom.NewPath(symbol.ID(7), "arg")

	normalized := numCeilLane.Normalize([]NumCeilFact{
		{Path: concrete, Ceil: 9}, // dropped: not a placeholder boundary path
		{Path: p0, Ceil: 4},
		{Path: p0, Ceil: 1}, // same path: stronger (smaller) local duplicate wins
	})
	if len(normalized) != 1 || !normalized[0].Path.Equal(p0) || normalized[0].Ceil != 1 {
		t.Fatalf("normalize num ceils = %#v, want $0 <= 1", normalized)
	}

	left := []NumCeilFact{
		{Path: p0, Ceil: 4},
		{Path: p1, Ceil: 1},
	}
	right := []NumCeilFact{
		{Path: p0, Ceil: 9},
	}
	joined := numCeilLane.Join(left, right)
	if len(joined) != 1 || !joined[0].Path.Equal(p0) || joined[0].Ceil != 9 {
		t.Fatalf("join num ceils = %#v, want common path with weaker ceiling 9", joined)
	}
	if !numCeilLane.LessOrEq(left, right) {
		t.Fatalf("stronger ceiling must be <= weaker common ceiling")
	}
	if numCeilLane.LessOrEq(right, left) {
		t.Fatalf("weaker/missing ceilings must not be <= stronger ceiling set")
	}
	if !numCeilLane.LessOrEq(
		[]NumCeilFact{{Path: p0, Ceil: 4}, {Path: p0, Ceil: 1}},
		[]NumCeilFact{{Path: p0, Ceil: 3}},
	) {
		t.Fatalf("LessOrEq must compare strongest duplicate ceiling without requiring normalized inputs")
	}
}

func TestNumCeilFactsNormalizeOwnedMayReusePathPayload(t *testing.T) {
	path := pathdom.NewPlaceholder(0).Field("index")

	defensiveInput := []NumCeilFact{{Path: path, Ceil: 9}}
	defensive := numCeilLane.Normalize(defensiveInput)
	defensiveInput[0].Path.Segments[0].Name = "changed"
	if defensive[0].Path.Segments[0].Name != "index" {
		t.Fatalf("Normalize reused caller path payload: got %q, want index", defensive[0].Path.Segments[0].Name)
	}

	ownedInput := []NumCeilFact{{Path: path, Ceil: 9}}
	owned := numCeilLane.NormalizeOwned(ownedInput)
	ownedInput[0].Path.Segments[0].Name = "changed"
	if owned[0].Path.Segments[0].Name != "changed" {
		t.Fatalf("NormalizeOwned cloned caller-owned path payload: got %q, want changed", owned[0].Path.Segments[0].Name)
	}
}

func TestNumFloorFactsNormalizeOwnedMayReusePathPayload(t *testing.T) {
	path := pathdom.NewPlaceholder(0).Field("index")

	defensiveInput := []NumFloorFact{{Path: path, Floor: 1}}
	defensive := numFloorLane.Normalize(defensiveInput)
	defensiveInput[0].Path.Segments[0].Name = "changed"
	if defensive[0].Path.Segments[0].Name != "index" {
		t.Fatalf("Normalize reused caller path payload: got %q, want index", defensive[0].Path.Segments[0].Name)
	}

	ownedInput := []NumFloorFact{{Path: path, Floor: 1}}
	owned := numFloorLane.NormalizeOwned(ownedInput)
	ownedInput[0].Path.Segments[0].Name = "changed"
	if owned[0].Path.Segments[0].Name != "changed" {
		t.Fatalf("NormalizeOwned cloned caller-owned path payload: got %q, want changed", owned[0].Path.Segments[0].Name)
	}
}

func TestRelConstraintFactsNormalizeOwnedMayReusePathPayload(t *testing.T) {
	fact := RelConstraintFact{
		CoA: 1,
		A:   RelOperand{Path: pathdom.NewPlaceholder(0).Field("i")},
		C:   RelOperand{Path: pathdom.NewPlaceholder(1).Field("n")},
	}

	defensiveInput := []RelConstraintFact{fact}
	defensive := relConstraintLane.Normalize(defensiveInput)
	defensiveInput[0].A.Path.Segments[0].Name = "changed"
	if defensive[0].A.Path.Segments[0].Name != "i" {
		t.Fatalf("Normalize reused caller path payload: got %q, want i", defensive[0].A.Path.Segments[0].Name)
	}

	ownedInput := []RelConstraintFact{fact}
	owned := relConstraintLane.NormalizeOwned(ownedInput)
	ownedInput[0].A.Path.Segments[0].Name = "changed"
	if owned[0].A.Path.Segments[0].Name != "changed" {
		t.Fatalf("NormalizeOwned cloned caller-owned path payload: got %q, want changed", owned[0].A.Path.Segments[0].Name)
	}
}

func TestPathInvalidationLaneAncestorSubsumesDescendant(t *testing.T) {
	pa := pathdom.NewPlaceholder(0).Field("a")
	pab := pa.Field("b")
	captured := pathdom.NewPath(symbol.ID(31), "captured").Field("value")
	ancestor := []PathInvalidationFact{{Path: pa}}
	descendant := []PathInvalidationFact{{Path: pab}}

	if got := pathInvalidationLane.Join(ancestor, descendant); len(got) != 1 || !got[0].Path.Equal(pa) {
		t.Fatalf("ancestor path must subsume descendant, got %+v", got)
	}
	if !pathInvalidationLane.LessOrEq(descendant, ancestor) {
		t.Fatalf("descendant invalidation must be below the ancestor invalidation")
	}
	normalized := pathInvalidationLane.Normalize([]PathInvalidationFact{
		{Path: pathdom.Path{}}, // empty identity is invalid
		{Path: pab},            // placeholder boundary invalidation
		{Path: captured},       // captured concrete invalidation
	})
	if len(normalized) != 2 {
		t.Fatalf("normalize invalidations = %+v, want placeholder and captured concrete paths", normalized)
	}
	if !hasPathInvalidation(normalized, pab) {
		t.Fatalf("normalize invalidations = %+v, want placeholder descendant", normalized)
	}
	if !hasPathInvalidation(normalized, captured) {
		t.Fatalf("normalize invalidations = %+v, want captured concrete path", normalized)
	}
	conflict := pathInvalidationLane.Normalize([]PathInvalidationFact{
		{Path: pa, PreserveStructuralWitness: true},
		{Path: pa},
	})
	if len(conflict) != 1 || !conflict[0].Path.Equal(pa) || conflict[0].PreserveStructuralWitness {
		t.Fatalf("normalize same-path invalidations = %+v, want stronger structural-clearing fact", conflict)
	}
}

func hasPathInvalidation(facts []PathInvalidationFact, target pathdom.Path) bool {
	for _, fact := range facts {
		if fact.Path.Equal(target) {
			return true
		}
	}
	return false
}
