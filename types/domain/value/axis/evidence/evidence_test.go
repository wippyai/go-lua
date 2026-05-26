package evidence

import "testing"

// TestEvidenceAxisLaws verifies the lattice laws for the current one-point
// evidence lattice. The discriminant/correlation/predicate carriers and their
// reducers arrive in Phase 5; the suite expands with them. For now it pins that
// every operation is total and law-consistent over the sole element.
func TestEvidenceAxisLaws(t *testing.T) {
	vs := []Value{Bottom(), Top()}
	for _, a := range vs {
		if !Equal(a, a) {
			t.Fatal("Equal not reflexive")
		}
		if !a.Covers(a) {
			t.Fatal("Covers not reflexive")
		}
		for _, b := range vs {
			if Equal(a, b) && a.Hash() != b.Hash() {
				t.Fatal("Equal values must hash identically")
			}
			if !Equal(Join(a, b), Join(b, a)) {
				t.Fatal("Join not commutative")
			}
			if !Equal(Join(a, a), a) {
				t.Fatal("Join not idempotent")
			}
			j := Join(a, b)
			if !j.Covers(a) || !j.Covers(b) {
				t.Fatal("Join not an upper bound")
			}
			if !Equal(Widen(a, b), Join(a, b)) {
				t.Fatal("Widen must equal Join")
			}
		}
	}
}
