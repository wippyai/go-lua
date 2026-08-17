package parserproducts

import "testing"

func TestReturnedLawOrderingAndGuardAlgebraAreCanonical(t *testing.T) {
	left := Guard{Atoms: []GuardAtom{{Kind: GuardAtomKind(1), Term: ActionTermID(2)}}}
	right := Guard{Atoms: []GuardAtom{{Kind: GuardAtomKind(2), Term: ActionTermID(3)}}}
	merged, err := mergeGuards(left, right)
	if err != nil || len(merged.Atoms) != 2 {
		t.Fatalf("mergeGuards = %#v/%v, want both atoms", merged, err)
	}
	negated, err := negatedGuard(left)
	if err != nil || len(negated.Atoms) != 1 || !negated.Atoms[0].Negated {
		t.Fatalf("negatedGuard = %#v/%v, want one negated atom", negated, err)
	}
	if compareGuards(left, left) != 0 || compareGuards(left, right) == 0 {
		t.Fatal("guard ordering does not distinguish equal and distinct rows")
	}
}
