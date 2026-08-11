package coverage

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/engine"
)

func TestConclusionIdentityIsFactorScopedAndRuleIndependent(t *testing.T) {
	owner := lawKey(1)
	first, firstOK := DeriveConclusion(owner, 1, 1)
	replay, replayOK := DeriveConclusion(owner, 1, 1)
	otherOrdinal, ordinalOK := DeriveConclusion(owner, 2, 1)
	otherRevision, revisionOK := DeriveConclusion(owner, 1, 2)
	otherOwner, ownerOK := DeriveConclusion(lawKey(2), 1, 1)
	if !firstOK || !replayOK || !ordinalOK || !revisionOK || !ownerOK {
		t.Fatal("derive conclusion identity")
	}
	if first != replay {
		t.Fatal("conclusion identity is not deterministic")
	}
	if first == otherOrdinal || first == otherRevision || first == otherOwner {
		t.Fatal("conclusion identity collapsed owner, ordinal, or revision")
	}
}

func TestConclusionIdentityRejectsIncompleteAuthority(t *testing.T) {
	if _, ok := DeriveConclusion(engine.SemanticKey{}, 1, 1); ok {
		t.Fatal("zero Factor owner admitted")
	}
	if _, ok := DeriveConclusion(lawKey(1), 0, 1); ok {
		t.Fatal("zero conclusion ordinal admitted")
	}
	if _, ok := DeriveConclusion(lawKey(1), 1, 0); ok {
		t.Fatal("zero conclusion revision admitted")
	}
}
