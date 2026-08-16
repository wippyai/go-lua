package keyspace

import "testing"

func TestTermFamiliesAreStableCheckedAndRoundTrip(t *testing.T) {
	if FamilyNil != 1 || FamilyImport != 57 || FamilyCount != 58 {
		t.Fatalf("stable family codes changed: nil=%d import=%d count=%d", FamilyNil, FamilyImport, FamilyCount)
	}
	for family := FamilyNil; family < FamilyCount; family++ {
		for _, ordinal := range []uint32{1, 7, MaxTermOrdinal} {
			term := MakeTerm(family, ordinal)
			if term == 0 || TermFamily(term) != family || TermOrdinal(term) != ordinal {
				t.Fatalf("family %d ordinal %d did not round-trip", family, ordinal)
			}
		}
	}
	for _, term := range []Term{
		MakeTerm(FamilyInvalid, 1),
		MakeTerm(FamilyCount, 1),
		MakeTerm(FamilyNil, 0),
	} {
		if term != 0 {
			t.Fatalf("invalid construction produced %08x", uint32(term))
		}
	}
	if TermFamily(Term(uint32(FamilyCount))) != FamilyInvalid {
		t.Fatal("malformed family did not fail closed")
	}
	if TermFamily(Term(uint32(FamilyNil))) != FamilyInvalid {
		t.Fatal("ordinal-zero family did not fail closed")
	}
	if !ValidTerm(MakeTerm(FamilyString, 3), FamilyString, 3) ||
		ValidTerm(MakeTerm(FamilyString, 3), FamilyString, 2) ||
		ValidTerm(MakeTerm(FamilyBool, 3), FamilyString, 3) {
		t.Fatal("family membership validation is not exact")
	}
}
