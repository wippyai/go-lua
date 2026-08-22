package call

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/identity"
)

// TestDispatchValuePreservesExactAuthenticatedTargetSet pins the Call
// completion seam: a dispatch proof carrying one owner-issued Target may not
// widen into the Algebra's global target universe. The opaque arm is an
// independent uncertainty bit and must not add sibling known targets.
func TestDispatchValuePreservesExactAuthenticatedTargetSet(t *testing.T) {
	_, _, algebra := targetOperationLawAlgebra(t)
	applicationID := identity.ContentID{70}
	algebra.keys = []keyRow{{kind: keyApplication, id: applicationID}}
	algebra.keyIndex = map[identity.ContentID]uint32{applicationID: 1}
	key, keyOK := algebra.KeyForApplicationID(applicationID)
	first, firstOK := algebra.TargetForSeedID(identity.ContentID{1})
	sibling, siblingOK := algebra.TargetForSeedID(identity.ContentID{2})
	if !keyOK || !firstOK || !siblingOK {
		t.Fatal("dispatch target-selection fixture")
	}

	complete, completeOK := algebra.DispatchValue(key, []Target{first}, false)
	if !completeOK || !complete.IsComplete() || complete.HasOpaqueAlternative() || complete.KnownTargetCount() != 1 || !complete.HasTarget(first) || complete.HasTarget(sibling) {
		t.Fatalf("complete dispatch = valid:%t opaque:%t known:%d first:%t sibling:%t", completeOK, complete.HasOpaqueAlternative(), complete.KnownTargetCount(), complete.HasTarget(first), complete.HasTarget(sibling))
	}

	open, openOK := algebra.DispatchValue(key, []Target{first}, true)
	if !openOK || !open.IsOpen() || !open.HasOpaqueAlternative() || open.KnownTargetCount() != 1 || !open.HasTarget(first) || open.HasTarget(sibling) {
		t.Fatalf("opaque dispatch = valid:%t open:%t opaque:%t known:%d first:%t sibling:%t", openOK, open.IsOpen(), open.HasOpaqueAlternative(), open.KnownTargetCount(), open.HasTarget(first), open.HasTarget(sibling))
	}
}
