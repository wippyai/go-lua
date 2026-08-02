package evidence

import "testing"

func TestFourStateSemanticLattice(t *testing.T) {
	if got := Join(GradualTop(), GradualTop()); !Equal(got, GradualTop()) {
		t.Fatalf("gradual proof joined with itself = %s, want gradual-top", got)
	}
	if got := Join(ExplicitTop(), ExplicitTop()); !Equal(got, ExplicitTop()) {
		t.Fatalf("explicit proof joined with itself = %s, want explicit-top", got)
	}
	if got := Join(GradualTop(), Top()); !Equal(got, Top()) {
		t.Fatalf("gradual proof joined with top = %s, want top", got)
	}
	if got := Join(GradualTop(), ExplicitTop()); !Equal(got, Top()) {
		t.Fatalf("gradual proof joined with explicit proof = %s, want top", got)
	}
	if got := Join(Bottom(), GradualTop()); !Equal(got, GradualTop()) {
		t.Fatalf("bottom joined with gradual proof = %s, want gradual-top", got)
	}
	if got := Meet(GradualTop(), Top()); !Equal(got, GradualTop()) {
		t.Fatalf("gradual proof met with top = %s, want gradual-top", got)
	}
	if got := Meet(ExplicitTop(), Top()); !Equal(got, ExplicitTop()) {
		t.Fatalf("explicit proof met with top = %s, want explicit-top", got)
	}
	if got := Meet(GradualTop(), ExplicitTop()); !Equal(got, Bottom()) {
		t.Fatalf("gradual proof met with explicit proof = %s, want bottom", got)
	}
}

func TestFourStateOrderHashAndWiden(t *testing.T) {
	if !Top().Covers(GradualTop()) || !Top().Covers(ExplicitTop()) {
		t.Fatal("top must cover both nonterminal proof states")
	}
	if GradualTop().Covers(Top()) || ExplicitTop().Covers(GradualTop()) || GradualTop().Covers(ExplicitTop()) {
		t.Fatal("semantic lattice order is incorrect")
	}
	if got := Widen(GradualTop(), GradualTop()); !Equal(got, GradualTop()) {
		t.Fatalf("widen identical gradual proof = %s", got)
	}
	if got := Widen(GradualTop(), ExplicitTop()); !Equal(got, Top()) {
		t.Fatalf("widen incompatible proofs = %s, want top", got)
	}
	values := []Value{Bottom(), GradualTop(), ExplicitTop(), Top()}
	for left, value := range values {
		for right := range values[:left] {
			if value.Hash() == values[right].Hash() {
				t.Fatalf("distinct semantic states %s and %s hash identically", value, values[right])
			}
		}
	}
}
