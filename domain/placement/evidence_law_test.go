package placement

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/identity"
)

func TestEvidenceStateJoinIsExhaustive(t *testing.T) {
	states := []EvidenceState{EvidenceUnknown, EvidenceRefuted, EvidenceProven}
	for _, left := range states {
		for _, right := range states {
			got := left.Join(right)
			want := EvidenceUnknown
			if left == right && left != EvidenceUnknown {
				want = left
			}
			if got != want || !got.Valid() {
				t.Fatalf("evidence join(%v,%v)=%v, want %v", left, right, got, want)
			}
		}
	}
	if EvidenceState(99).Valid() || EvidenceState(99).Join(EvidenceProven) != EvidenceUnknown {
		t.Fatal("invalid evidence state crossed the conservative join")
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

func TestAllocationEvidenceMergeClearsConflictingOptionalScalarsToZero(t *testing.T) {
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
	merged := left.Merge(right)
	if !merged.Valid() {
		t.Fatalf("conflicting evidence merge became invalid: %#v", merged)
	}
	if merged.HasKind || merged.Kind != AllocationKindUnknown {
		t.Fatalf("kind conflict retained stale scalar: %#v", merged)
	}
	if merged.HasOwnerIdentity || merged.OwnerIdentity.Available() {
		t.Fatalf("owner conflict retained stale identity: %#v", merged)
	}
}
