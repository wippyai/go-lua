package placement

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/identity"
)

func TestEvidenceStateJoinIsExhaustive(t *testing.T) {
	states := []EvidenceState{EvidenceAbsent, EvidenceUnknown, EvidenceRefuted, EvidenceProven}
	// The composition is stated independently of the implementation: absence
	// is the identity, exact authenticated repeats are idempotent, and every
	// distinct pair of written verdicts is refused. In particular, Unknown is
	// a completed producer verdict and cannot be erased by Proven or Refuted.
	expect := func(left, right EvidenceState) (EvidenceState, bool) {
		switch {
		case left == EvidenceAbsent:
			return right, true
		case right == EvidenceAbsent:
			return left, true
		case left == right:
			return left, true
		default:
			return invalidEvidenceState, false
		}
	}
	for _, left := range states {
		for _, right := range states {
			got, ok := left.JoinChecked(right)
			want, wantOK := expect(left, right)
			if got != want || ok != wantOK || got.Valid() != want.Valid() {
				t.Fatalf("evidence join(%v,%v)=%v/%t, want %v/%t", left, right, got, ok, want, wantOK)
			}
			// The join is commutative: no producer ordering may change a
			// published proof column.
			mirrored, mirroredOK := right.JoinChecked(left)
			if mirrored != got || mirroredOK != ok {
				t.Fatalf("evidence join(%v,%v)=%v/%t is not commutative with %v/%t", right, left, mirrored, mirroredOK, got, ok)
			}
		}
	}
	if EvidenceAbsent.Known() || EvidenceAbsent != (EvidenceState(0)) || !EvidenceAbsent.Absent() {
		t.Fatal("absence is not the zero value of the proof state")
	}
	if EvidenceState(99).Valid() {
		t.Fatal("invalid evidence state admitted")
	}
	if got, ok := EvidenceState(99).JoinChecked(EvidenceProven); ok || got.Valid() || got == EvidenceUnknown {
		t.Fatalf("invalid evidence state crossed the checked join: %v/%t", got, ok)
	}
}

// TestEvidenceAbsenceIsNotUnknown is the tri-state publication law. A proof
// column that no producer wrote is absence; a column a producer authenticated
// as undecidable is Unknown. The two must remain distinguishable at every
// boundary, and absence must be the join identity so that composing an
// unwritten column can neither erase nor weaken an authenticated one.
func TestEvidenceAbsenceIsNotUnknown(t *testing.T) {
	unwritten := AllocationEvidence{}
	authenticated := AllocationEvidence{DeepFrozen: EvidenceUnknown}
	if unwritten.DeepFrozen == authenticated.DeepFrozen {
		t.Fatal("an unwritten proof column is indistinguishable from producer-authenticated Unknown")
	}
	if unwritten.DeepFrozen.Known() || !unwritten.Valid() {
		t.Fatalf("absent proof column = %v, want a valid state with no polarity", unwritten.DeepFrozen)
	}
	for _, state := range []EvidenceState{EvidenceUnknown, EvidenceRefuted, EvidenceProven} {
		joined, ok := unwritten.DeepFrozen.JoinChecked(state)
		if !ok || joined != state {
			t.Fatalf("absence join %v = %v/%t, want %v/true", state, joined, ok, state)
		}
		joined, ok = state.JoinChecked(unwritten.DeepFrozen)
		if !ok || joined != state {
			t.Fatalf("%v join absence = %v/%t, want %v/true", state, joined, ok, state)
		}
	}
	// Absence is the unique identity: joining it into an authenticated Unknown
	// yields Unknown, so Unknown is strictly above absence rather than equal.
	if joined, ok := EvidenceUnknown.JoinChecked(unwritten.DeepFrozen); !ok || joined != EvidenceUnknown {
		t.Fatalf("Unknown join absence = %v/%t, want Unknown/true", joined, ok)
	}
	// Composing an unwritten producer row must retain every base proof, and an
	// unwritten base must adopt the producer's authenticated states.
	base := AllocationEvidence{FrameLocal: EvidenceProven, DiesBeforeSuspension: EvidenceRefuted, DeepFrozen: EvidenceUnknown}
	merged, ok := ComposeAllocationEvidence(base, unwritten)
	if !ok || merged != base {
		t.Fatalf("composing an unwritten producer row = %#v/%t, want the base unchanged", merged, ok)
	}
	merged, ok = ComposeAllocationEvidence(unwritten, base)
	if !ok || merged != base {
		t.Fatalf("composing onto an unwritten base = %#v/%t, want the producer row", merged, ok)
	}
}

func TestAllocationEvidenceRejectsImplicitPresence(t *testing.T) {
	if !AllocationKindManifest.Valid() || AllocationKindManifest.String() != "manifest.allocation" {
		t.Fatal("Target fresh allocation kind is not in the public vocabulary")
	}
	owner := identity.ContentID{1}
	if unknown := (AllocationEvidence{}); unknown.HasKind || unknown.HasDepth {
		t.Fatal("unknown evidence unexpectedly carried presence")
	}
	if (AllocationEvidence{Depth: 1}).Valid() {
		t.Fatal("optional scalar was admitted without its presence bit")
	}
	if !(AllocationEvidence{OwnerIdentity: owner, HasOwnerIdentity: true}).Valid() {
		t.Fatal("available owner identity was rejected")
	}
	if (AllocationEvidence{OwnerIdentity: identity.ContentID{}, HasOwnerIdentity: true}).Valid() {
		t.Fatal("unavailable owner identity was admitted")
	}
	for index := range owner {
		var nonzero identity.ContentID
		nonzero[index] = 1
		if (AllocationEvidence{OwnerIdentity: nonzero}).Valid() {
			t.Fatalf("nonzero owner identity byte %d was admitted without presence", index)
		}
	}
	if (AllocationEvidence{Kind: AllocationKindUnknown, HasKind: true}).Valid() {
		t.Fatal("unknown kind was admitted with presence")
	}
	for name, evidence := range map[string]AllocationEvidence{
		"table without presence":        {Kind: AllocationKindTable},
		"closure without presence":      {Kind: AllocationKindClosure},
		"invalid kind without presence": {Kind: AllocationKind(0xff)},
		"owner without presence":        {OwnerIdentity: owner},
	} {
		t.Run(name, func(t *testing.T) {
			if evidence.Valid() {
				t.Fatalf("hostile evidence %#v was admitted without its presence bit", evidence)
			}
		})
	}

	// These are the two ordinary producer shapes: Heap-derived records carry
	// both the owner identity and a known Heap kind, while fresh Target roots
	// carry only the owner identity until a neutral kind producer exists.
	if !(AllocationEvidence{
		Kind:             AllocationKindTable,
		HasKind:          true,
		OwnerIdentity:    owner,
		HasOwnerIdentity: true,
	}).Valid() {
		t.Fatal("ordinary Heap-derived evidence was rejected")
	}
	if !(AllocationEvidence{OwnerIdentity: owner, HasOwnerIdentity: true}).Valid() {
		t.Fatal("fresh-root evidence without a kind was rejected")
	}
}

func TestNewAllocationEvidenceProjectsAuthenticatedFact(t *testing.T) {
	owner := identity.ContentID{1}
	for _, test := range []struct {
		name       string
		kind       AllocationKind
		fact       Fact
		frameLocal EvidenceState
		hasKind    bool
	}{
		{name: "stack table", kind: AllocationKindTable, fact: Fact{Class: Stack, RetainEscape: EvidenceRefuted}, frameLocal: EvidenceProven, hasKind: true},
		{name: "owned closure", kind: AllocationKindClosure, fact: Fact{Class: OwnedHeap, RetainEscape: EvidenceProven}, frameLocal: EvidenceRefuted, hasKind: true},
		{name: "shared unknown kind", kind: AllocationKindUnknown, fact: Fact{Class: SharedHeap, RetainEscape: EvidenceUnknown}, frameLocal: EvidenceRefuted},
	} {
		t.Run(test.name, func(t *testing.T) {
			got, ok := NewAllocationEvidence(owner, test.kind, test.fact)
			if !ok || !got.Valid() {
				t.Fatalf("NewAllocationEvidence = %#v/%t, want valid", got, ok)
			}
			if !got.HasOwnerIdentity || got.OwnerIdentity != owner || !got.HasClass || got.Class != test.fact.Class || got.RetainEscape != test.fact.RetainEscape {
				t.Fatalf("base identity/fact projection = %#v, want owner and %#v", got, test.fact)
			}
			if got.Kind != test.kind || got.HasKind != test.hasKind || got.FrameLocal != test.frameLocal {
				t.Fatalf("base kind/frame projection = %#v, want kind %v/%t and frame %v", got, test.kind, test.hasKind, test.frameLocal)
			}
			if got.DiesBeforeSuspension != EvidenceAbsent || got.DeepFrozen != EvidenceAbsent || got.HasDepth {
				t.Fatalf("constructor invented producer refinements: %#v", got)
			}
		})
	}
}

func TestNewAllocationEvidenceRejectsUnauthenticatedInputs(t *testing.T) {
	owner := identity.ContentID{1}
	fact := Fact{Class: Stack, RetainEscape: EvidenceRefuted}
	for name, test := range map[string]struct {
		owner identity.ContentID
		kind  AllocationKind
		fact  Fact
	}{
		"unavailable owner": {owner: identity.ContentID{}, kind: AllocationKindTable, fact: fact},
		"invalid kind":      {owner: owner, kind: AllocationKind(0xff), fact: fact},
		"bottom fact":       {owner: owner, kind: AllocationKindTable, fact: BottomFact()},
		"absent retain":     {owner: owner, kind: AllocationKindTable, fact: Fact{Class: Stack, RetainEscape: EvidenceAbsent}},
	} {
		t.Run(name, func(t *testing.T) {
			if got, ok := NewAllocationEvidence(test.owner, test.kind, test.fact); ok || got.Valid() {
				t.Fatalf("unauthenticated constructor input produced %#v/%t", got, ok)
			}
		})
	}
}

func TestAllocationEvidenceCompositionRejectsConflictingOptionalScalars(t *testing.T) {
	leftOwner := identity.ContentID{1}
	rightOwner := identity.ContentID{2}
	left := AllocationEvidence{
		Kind:             AllocationKindTable,
		HasKind:          true,
		OwnerIdentity:    leftOwner,
		HasOwnerIdentity: true,
	}
	right := AllocationEvidence{
		Kind:             AllocationKindClosure,
		HasKind:          true,
		OwnerIdentity:    rightOwner,
		HasOwnerIdentity: true,
	}
	if merged, ok := ComposeAllocationEvidence(left, right); ok || merged.Valid() {
		t.Fatalf("conflicting evidence was composed: %#v/%t", merged, ok)
	}
}

func TestAllocationEvidenceCompositionRejectsInvalidAndConflictingProofs(t *testing.T) {
	base := AllocationEvidence{FrameLocal: EvidenceProven}
	if merged, ok := ComposeAllocationEvidence(base, AllocationEvidence{FrameLocal: EvidenceRefuted}); ok || merged.Valid() {
		t.Fatalf("conflicting proof was composed: %#v/%t", merged, ok)
	}
	if merged, ok := ComposeAllocationEvidence(base, AllocationEvidence{FrameLocal: EvidenceUnknown}); ok || merged.Valid() {
		t.Fatalf("authenticated Unknown was erased by a decided proof: %#v/%t", merged, ok)
	}
	if merged, ok := ComposeAllocationEvidence(AllocationEvidence{FrameLocal: EvidenceUnknown}, base); ok || merged.Valid() {
		t.Fatalf("decided proof erased authenticated Unknown: %#v/%t", merged, ok)
	}
	if merged, ok := ComposeAllocationEvidence(base, AllocationEvidence{DeepFrozen: EvidenceState(99)}); ok || merged.Valid() {
		t.Fatalf("invalid proof was composed: %#v/%t", merged, ok)
	}
	refinement := AllocationEvidence{DiesBeforeSuspension: EvidenceProven}
	merged, ok := ComposeAllocationEvidence(base, refinement)
	if !ok || !merged.Valid() || merged.FrameLocal != EvidenceProven || merged.DiesBeforeSuspension != EvidenceProven {
		t.Fatalf("independent proof refinement = %#v/%t", merged, ok)
	}
}
