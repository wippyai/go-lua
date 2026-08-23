package placement

import "testing"

func TestAuthenticateFactCellPinsCanonicalDefault(t *testing.T) {
	facts := []Fact{
		DefaultFact(),
		{Class: OwnedHeap, RetainEscape: EvidenceRefuted},
		{Class: SharedHeap, RetainEscape: EvidenceProven},
		UnknownFact(),
	}
	for _, fact := range facts {
		if got, ok := AuthenticateFactCell(fact, true, true); !ok || got != fact {
			t.Fatalf("present %#v = %#v/%t", fact, got, ok)
		}
	}
	if got, ok := AuthenticateFactCell(DefaultFact(), false, true); !ok || got != DefaultFact() {
		t.Fatalf("sparse default = %#v/%t", got, ok)
	}
	for _, input := range []struct {
		fact      Fact
		present   bool
		available bool
	}{
		{BottomFact(), true, true},
		{BottomFact(), false, true},
		{Fact{Class: OwnedHeap, RetainEscape: EvidenceRefuted}, false, true},
		{Fact{Class: Interpreter, RetainEscape: EvidenceRefuted}, true, true},
		{Fact{Class: Stack, RetainEscape: EvidenceAbsent}, true, true},
		{DefaultFact(), false, false},
	} {
		if got, ok := AuthenticateFactCell(input.fact, input.present, input.available); ok || got.Valid() {
			t.Fatalf("malformed cell %#v/%t/%t = %#v/%t", input.fact, input.present, input.available, got, ok)
		}
	}
}

func TestDisplaceFactMatrix(t *testing.T) {
	facts := []Fact{
		DefaultFact(),
		{Class: OwnedHeap, RetainEscape: EvidenceRefuted},
		{Class: SharedHeap, RetainEscape: EvidenceProven},
		UnknownFact(),
		BottomFact(),
		{Class: Register, RetainEscape: EvidenceRefuted},
	}
	for _, current := range facts {
		for _, escape := range allEscapeValues() {
			got, ok := DisplaceFactChecked(current, escape)
			validInput := current.Valid() && current.Class != Bottom && current.RetainEscape != EvidenceAbsent && validAnalysisEscape(escape)
			if !validInput {
				if ok || got.Valid() {
					t.Fatalf("DisplaceFact(%#v, %v) = %#v/%t, want refusal", current, escape.Name(), got, ok)
				}
				continue
			}
			if !ok || !got.Valid() {
				t.Fatalf("DisplaceFact(%#v, %v) = %#v/%t", current, escape.Name(), got, ok)
			}
			if escape == None || escape == Borrow {
				if got != current {
					t.Fatalf("non-retaining escape changed fact: %#v -> %#v", current, got)
				}
			} else if got.RetainEscape != EvidenceProven || !LessOrEq(current.Class, got.Class) {
				t.Fatalf("retaining escape result = %#v, want monotone class and proven provenance", got)
			}
		}
	}
}

func TestDisplaceFactIsIdempotentAfterRetain(t *testing.T) {
	for _, escape := range []Escape{Retain, Store, Send, Export, Opaque, Return} {
		first, ok := DisplaceFactChecked(DefaultFact(), escape)
		if !ok {
			t.Fatalf("first %v displacement", escape)
		}
		second, ok := DisplaceFactChecked(first, escape)
		if !ok || second != first {
			t.Fatalf("%v displacement = %#v then %#v/%t", escape, first, second, ok)
		}
	}
}

func allEscapeValues() []Escape {
	values := make([]Escape, 0, 256)
	for raw := 0; raw <= 255; raw++ {
		values = append(values, Escape(raw))
	}
	return values
}
