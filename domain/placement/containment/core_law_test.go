package containment

import "testing"

func TestContainmentOperandSummaryKeysAreCompleteAndScalar(t *testing.T) {
	fixture := newContainmentFixture(t)
	candidate, candidateOK := operandForSchema(fixture.placement)
	if !candidateOK {
		t.Fatal("Placement schema did not issue the containment closure Operand")
	}
	count := candidate.SummaryKeyCount()
	if count != fixture.placement.KeyCount() {
		t.Fatalf("containment summary key count = %d, want %d", count, fixture.placement.KeyCount())
	}
	for index := 0; index < count; index++ {
		key, keyOK := candidate.SummaryKeyAt(index)
		if !keyOK || key != uint64(index) {
			t.Fatalf("containment summary key %d = %d, want %d", index, key, index)
		}
	}
	if count == 0 {
		t.Fatal("non-empty containment fixture issued no allocation summary keys")
	}
	if _, keyOK := candidate.SummaryKeyAt(count); keyOK {
		t.Fatal("containment summary key range exposed an out-of-bounds coordinate")
	}
}

func TestContainmentOperandSummaryKeysRejectForeignAndMalformedCandidates(t *testing.T) {
	fixture := newContainmentFixture(t)
	foreignFixture := newContainmentFixtureNamed(t, "placement-containment-summary-foreign")
	canonical, canonicalOK := operandForSchema(fixture.placement)
	foreign, foreignOK := operandForSchema(foreignFixture.placement)
	if !canonicalOK || !foreignOK {
		t.Fatal("containment Operand setup")
	}
	if _, _, accepted := operandContentForSchema(fixture.placement, foreign); accepted {
		t.Fatal("foreign Placement summary-key Operand crossed the owner fence")
	}

	malformed := canonical
	malformed.summaryKeys = append([]uint64(nil), canonical.summaryKeys...)
	if len(malformed.summaryKeys) == 0 {
		malformed.summaryKeys = []uint64{0}
	} else {
		malformed.summaryKeys[0] = uint64(len(malformed.summaryKeys))
	}
	if _, _, accepted := operandContentForSchema(fixture.placement, malformed); accepted {
		t.Fatal("malformed Placement summary-key Operand was admitted")
	}

	missing := canonical
	missing.summaryKeys = nil
	if _, _, accepted := operandContentForSchema(fixture.placement, missing); accepted {
		t.Fatal("missing Placement summary-key Operand was admitted")
	}
}
