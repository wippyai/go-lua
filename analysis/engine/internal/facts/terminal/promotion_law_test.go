package terminal

import (
	"sync"
	"testing"
)

func stringConfig() Config[string] {
	return Config[string]{
		Equal:       func(left, right string) bool { return left == right },
		Fingerprint: func(value string) uint64 { return uint64(len(value)) },
	}
}

// A sealed candidate page publishes its terminals into the one owner intern
// generation.  A later Work therefore resolves an equal value to that exact
// identity instead of minting a second terminal for the same semantic value.
func TestSealPromotesCandidateTerminalsIntoTheOwnerInternGeneration(t *testing.T) {
	base, ok := New(stringConfig())
	if !ok || !base.Seal() {
		t.Fatal("base setup failed")
	}

	first := base.Begin()
	firstID, ok := first.Admit("shared")
	if !ok {
		t.Fatal("first admission failed")
	}
	if _, sealed := first.Seal(); !sealed {
		t.Fatal("first seal failed")
	}

	second := base.Begin()
	secondID, ok := second.Admit("shared")
	if !ok {
		t.Fatal("second admission failed")
	}
	if secondID != firstID {
		t.Fatalf("second work minted terminal %v for a promoted value, want %v", secondID, firstID)
	}
	if _, sealed := second.Seal(); !sealed {
		t.Fatal("second seal failed")
	}
	if !base.Equal(firstID, secondID) {
		t.Fatal("promoted terminals were not one identity")
	}
	if found, ok := base.Lookup("shared"); !ok || found != firstID {
		t.Fatalf("owner lookup = %v/%t, want the promoted identity", found, ok)
	}
}

// Equal terminals resolve to one canonical identity even when independent
// candidate pages admitted the value before either published.  Identity, not
// a value walk, is therefore the exact sealed equality answer.
func TestPromotionCanonicalizesIndependentlyAdmittedEqualTerminals(t *testing.T) {
	base, ok := New(stringConfig())
	if !ok || !base.Seal() {
		t.Fatal("base setup failed")
	}

	left, right := base.Begin(), base.Begin()
	leftID, leftOK := left.Admit("same")
	rightID, rightOK := right.Admit("same")
	if !leftOK || !rightOK {
		t.Fatal("isolated admission failed")
	}
	if leftID == rightID {
		t.Fatal("open sibling candidates shared one page")
	}
	if _, sealed := left.Seal(); !sealed {
		t.Fatal("left seal failed")
	}
	if _, sealed := right.Seal(); !sealed {
		t.Fatal("right seal failed")
	}
	if !base.Equal(leftID, rightID) {
		t.Fatal("published equal terminals were not one identity")
	}
	if base.Canonical(leftID) != base.Canonical(rightID) {
		t.Fatalf("canonical identities differ: %v vs %v", base.Canonical(leftID), base.Canonical(rightID))
	}
	if base.Equal(leftID, ID[string]{}) || base.Equal(ID[string]{}, rightID) {
		t.Fatal("the undefined terminal was merged with a value terminal")
	}
	unequal := base.Begin()
	otherID, ok := unequal.Admit("other")
	if !ok {
		t.Fatal("unequal admission failed")
	}
	if _, sealed := unequal.Seal(); !sealed {
		t.Fatal("unequal seal failed")
	}
	if base.Equal(leftID, otherID) || base.Canonical(leftID) == base.Canonical(otherID) {
		t.Fatal("distinct values collapsed into one canonical terminal")
	}
}

// Candidate isolation survives promotion: an open Work never observes another
// still-open Work's terminals, by identity or by admission.
func TestOpenCandidatesDoNotObserveEachOtherAfterPromotion(t *testing.T) {
	base, ok := New(stringConfig())
	if !ok || !base.Seal() {
		t.Fatal("base setup failed")
	}
	left, right := base.Begin(), base.Begin()
	leftID, ok := left.Admit("private")
	if !ok {
		t.Fatal("left admission failed")
	}
	if right.Valid(leftID) {
		t.Fatal("open sibling accepted a private candidate identity")
	}
	if _, readable := right.Value(leftID); readable {
		t.Fatal("open sibling read a private candidate value")
	}
	if base.Valid(leftID) {
		t.Fatal("unpublished candidate identity entered the immutable owner")
	}
	if _, ok := base.Lookup("private"); ok {
		t.Fatal("owner lookup resolved an unpublished candidate value")
	}
	rightID, ok := right.Admit("private")
	if !ok || rightID == leftID {
		t.Fatal("open sibling admission reused an unpublished candidate identity")
	}
}

// A discarded candidate contributes nothing: its values never join the owner
// intern generation and never become readable identities.
func TestDiscardedCandidateTerminalsNeverPromote(t *testing.T) {
	base, ok := New(stringConfig())
	if !ok || !base.Seal() {
		t.Fatal("base setup failed")
	}
	dropped := base.Begin()
	droppedID, ok := dropped.Admit("dropped")
	if !ok {
		t.Fatal("candidate admission failed")
	}
	dropped.Discard()
	if dropped.Valid(droppedID) || base.Valid(droppedID) {
		t.Fatal("discarded candidate identity remained readable")
	}
	if _, ok := base.Lookup("dropped"); ok {
		t.Fatal("discarded candidate value entered the owner intern generation")
	}
	if _, ok := dropped.Admit("late"); ok {
		t.Fatal("discarded work admitted a terminal")
	}
	if _, sealed := dropped.Seal(); sealed {
		t.Fatal("discarded work published a page")
	}
	surviving := base.Begin()
	survivingID, ok := surviving.Admit("dropped")
	if !ok || survivingID == droppedID {
		t.Fatal("a later work inherited a discarded candidate identity")
	}
	if _, sealed := surviving.Seal(); !sealed {
		t.Fatal("surviving seal failed")
	}
	if found, ok := base.Lookup("dropped"); !ok || found != survivingID {
		t.Fatalf("owner lookup = %v/%t, want the surviving identity", found, ok)
	}
}

// Concurrent seals converge on one canonical identity per semantic value.
func TestConcurrentSealsConvergeOnOneCanonicalTerminal(t *testing.T) {
	base, ok := New(Config[int]{
		Equal:       func(left, right int) bool { return left == right },
		Fingerprint: func(value int) uint64 { return uint64(value % 4) },
	})
	if !ok || !base.Seal() {
		t.Fatal("base setup failed")
	}
	const workers = 32
	ids := make([]ID[int], workers)
	var group sync.WaitGroup
	group.Add(workers)
	for index := range ids {
		go func(index int) {
			defer group.Done()
			work := base.Begin()
			id, admitted := work.Admit(index % 3)
			if _, sealed := work.Seal(); !admitted || !sealed {
				return
			}
			ids[index] = id
		}(index)
	}
	group.Wait()
	canonical := make(map[int]ID[int])
	for index, id := range ids {
		if !base.Valid(id) {
			t.Fatalf("worker %d identity was not published", index)
		}
		value, readable := base.Value(id)
		if !readable || value != index%3 {
			t.Fatalf("worker %d value = %d/%t, want %d", index, value, readable, index%3)
		}
		resolved := base.Canonical(id)
		if prior, seen := canonical[value]; seen {
			if prior != resolved {
				t.Fatalf("value %d resolved to two canonical terminals", value)
			}
			continue
		}
		canonical[value] = resolved
	}
	if len(canonical) != 3 {
		t.Fatalf("canonical terminal count = %d, want 3", len(canonical))
	}
	count := 0
	if !base.Every(func(int) bool { count++; return true }) {
		t.Fatal("sealed universe audit failed")
	}
	if count < 3 {
		t.Fatalf("sealed universe holds %d terminals, want at least the 3 distinct values", count)
	}
}
