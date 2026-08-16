package analysis

import "testing"

func TestSemanticBundleIsExplicitDistinctAndGlobal(t *testing.T) {
	first, firstOK := newSemanticBundle()
	replayed, replayedOK := newSemanticBundle()
	if !firstOK || !replayedOK {
		t.Fatal("semantic bundle unavailable")
	}
	if first.ValueFactor != replayed.ValueFactor || first.RawGetRule.Rule != replayed.RawGetRule.Rule {
		t.Fatal("equivalent semantic bundle did not replay exactly")
	}
	if first.ValueFactor == first.CallFactor || first.RawGetRule.Rule == first.RawGetRule.Operand {
		t.Fatal("distinct typed semantic roles collided")
	}
	if first.ValueQuery != replayed.ValueQuery || first.ValueCodec != replayed.ValueCodec ||
		first.EffectQuery != replayed.EffectQuery || first.EffectCodec != replayed.EffectCodec ||
		first.ValueQuery == first.ValueCodec || first.EffectQuery == first.EffectCodec {
		t.Fatal("fixed query-family semantics are not exact, distinct, and replayable")
	}
}
