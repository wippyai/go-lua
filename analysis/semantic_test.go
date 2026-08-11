package analysis

import (
	"testing"

	"github.com/wippyai/go-lua/program/keyspace"
)

func TestSemanticBundleIsExplicitDistinctAndLinkScoped(t *testing.T) {
	var firstID, secondID keyspace.ContentID
	firstID[31] = 1
	secondID[31] = 2

	first, firstOK := newSemanticBundle(firstID)
	replayed, replayedOK := newSemanticBundle(firstID)
	second, secondOK := newSemanticBundle(secondID)
	if !firstOK || !replayedOK || !secondOK {
		t.Fatal("semantic bundle unavailable")
	}
	if first.valueFactor != replayed.valueFactor || first.rawGetRule.rule != replayed.rawGetRule.rule {
		t.Fatal("equivalent semantic bundle did not replay exactly")
	}
	if first.valueFactor == first.callFactor || first.rawGetRule.rule == first.rawGetRule.operand {
		t.Fatal("distinct typed semantic roles collided")
	}
	if first.valueFactor == second.valueFactor || first.rawGetRule.rule == second.rawGetRule.rule {
		t.Fatal("foreign Link semantic bundle crossed its provenance boundary")
	}
	if first.valueQuery != replayed.valueQuery || first.valueCodec != replayed.valueCodec ||
		first.effectQuery != replayed.effectQuery || first.effectCodec != replayed.effectCodec ||
		first.valueQuery == first.valueCodec || first.effectQuery == first.effectCodec ||
		first.valueQuery == second.valueQuery || first.effectQuery == second.effectQuery {
		t.Fatal("fixed query-family semantics are not exact, distinct, and replayable")
	}
	if _, ok := newSemanticBundle(keyspace.ContentID{}); ok {
		t.Fatal("unavailable Link identity admitted")
	}
}
