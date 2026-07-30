package pathevidence

import (
	"testing"

	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/path/keyspace"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/presence"
)

func TestCloseBranchProofsAcrossKnownEqualitiesClosesFiniteObservedCarrier(t *testing.T) {
	ks := keyspace.New()
	left := mustClosureKey(t, ks, "sym1001@1")
	middle := mustClosureKey(t, ks, "sym1002@1")
	right := mustClosureKey(t, ks, "sym1003@1")
	leftField := mustClosureKey(t, ks, "sym1001@1.field")
	middleField := mustClosureKey(t, ks, "sym1002@1.field")
	rightField := mustClosureKey(t, ks, "sym1003@1.field")

	closed := CloseBranchProofsAcrossKnownEqualities(ks, []BranchProof{
		{Kind: BranchProofPathEqual, Path: left, Other: middle},
		{Kind: BranchProofPathEqual, Path: middle, Other: right},
		{Kind: BranchProofPathPresence, Path: leftField, Presence: presence.Present()},
	})
	for _, want := range []BranchProof{
		{Kind: BranchProofPathPresence, Path: leftField, Presence: presence.Present()},
		{Kind: BranchProofPathPresence, Path: middleField, Presence: presence.Present()},
		{Kind: BranchProofPathPresence, Path: rightField, Presence: presence.Present()},
	} {
		if !containsClosedBranchProof(closed, want) {
			t.Fatalf("closed proofs omitted %s", ks.Format(want.Path))
		}
	}
}

func TestCloseBranchProofsAcrossKnownEqualitiesDoesNotInternCyclicTerms(t *testing.T) {
	ks := keyspace.New()
	a := mustClosureKey(t, ks, "sym1011@1")
	aChild := mustClosureKey(t, ks, "sym1011@1.child")
	b := mustClosureKey(t, ks, "sym1012@1")
	bChild := mustClosureKey(t, ks, "sym1012@1.child")
	aLabel := mustClosureKey(t, ks, "sym1011@1.label")
	bChildLabel := mustClosureKey(t, ks, "sym1012@1.child.label")
	forbidden := mustClosureKey(t, ks, "sym1011@1.child.child.label")

	closed := CloseBranchProofsAcrossKnownEqualities(ks, []BranchProof{
		{Kind: BranchProofPathEqual, Path: a, Other: bChild},
		{Kind: BranchProofPathEqual, Path: b, Other: aChild},
		{Kind: BranchProofPathPresence, Path: aLabel, Presence: presence.Present()},
	})
	if !containsClosedBranchProof(closed, BranchProof{Kind: BranchProofPathPresence, Path: bChildLabel, Presence: presence.Present()}) {
		t.Fatalf("finite carrier omitted direct observed-shape consequence %s", ks.Format(bChildLabel))
	}
	if containsClosedBranchProof(closed, BranchProof{Kind: BranchProofPathPresence, Path: forbidden, Presence: presence.Present()}) {
		t.Fatalf("closure invented cyclic term %s", ks.Format(forbidden))
	}
	if got, want := len(closed), 4; got != want {
		t.Fatalf("closed proof count = %d, want %d", got, want)
	}
}

func TestCloseBranchProofsAcrossKnownEqualitiesClosesIndexBoundsAtBothKeys(t *testing.T) {
	ks := keyspace.New()
	left := mustClosureKey(t, ks, "sym1021@1")
	right := mustClosureKey(t, ks, "sym1022@1")
	leftIndex := mustClosureKey(t, ks, "sym1021@1.index")
	rightIndex := mustClosureKey(t, ks, "sym1022@1.index")
	leftLimit := mustClosureKey(t, ks, "sym1021@1.limit")
	rightLimit := mustClosureKey(t, ks, "sym1022@1.limit")

	closed := CloseBranchProofsAcrossKnownEqualities(ks, []BranchProof{
		{Kind: BranchProofPathEqual, Path: left, Other: right},
		{Kind: BranchProofIndexInRange, Path: leftIndex, Other: leftLimit},
	})
	want := BranchProof{Kind: BranchProofIndexInRange, Path: rightIndex, Other: rightLimit}
	if !containsClosedBranchProof(closed, want) {
		t.Fatalf("closed proofs omitted rebased index bounds %s/%s", ks.Format(rightIndex), ks.Format(rightLimit))
	}
}

func mustClosureKey(t *testing.T, ks *keyspace.KeySpace, path pathdom.PathKey) keyspace.Key {
	t.Helper()
	key, ok := ks.FromStateKey(path)
	if !ok {
		t.Fatalf("keyspace key %q", path)
	}
	return key
}

func containsClosedBranchProof(proofs []BranchProof, want BranchProof) bool {
	for _, proof := range proofs {
		if proof == want {
			return true
		}
	}
	return false
}
