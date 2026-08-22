package formalfreeze

import "testing"

// TestFormalFreezeActualTagsRankInAuthoredOrder is what remains of this rule's
// side of the selection contract once the engine owns member order. The rule
// mints one dense tag per authored actual; because those tags strictly increase
// with the authored ordinal, the engine's declared ReadOrderByTag ranking is the
// authored order, and the rule needs no decode of its own.
func TestFormalFreezeActualTagsRankInAuthoredOrder(t *testing.T) {
	const count = 8
	previous, seen := actualTag(0), make(map[actualTag]int, count)
	for ordinal := 0; ordinal < count; ordinal++ {
		tag, tagOK := canonicalActualTag(ordinal)
		if !tagOK || tag == 0 {
			t.Fatalf("authored ordinal %d minted tag %d/%t", ordinal, tag, tagOK)
		}
		if ordinal > 0 && tag <= previous {
			t.Fatalf("authored ordinal %d minted tag %d after %d", ordinal, tag, previous)
		}
		if earlier, repeated := seen[tag]; repeated {
			t.Fatalf("tag %d names both authored ordinal %d and %d", tag, earlier, ordinal)
		}
		seen[tag], previous = ordinal, tag
	}
	if _, ok := canonicalActualTag(-1); ok {
		t.Fatal("a negative authored ordinal minted a tag")
	}
}
