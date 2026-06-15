package evidence

import "testing"

func TestGradualTopJoinKeepsOnlyCommonProof(t *testing.T) {
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
}

func TestEvidenceMeetTreatsTopOriginsAsIncomparable(t *testing.T) {
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

func TestGradualTopOrderAndHash(t *testing.T) {
	if !Top().Covers(GradualTop()) {
		t.Fatal("top/no-evidence must cover gradual-top evidence")
	}
	if !Top().Covers(ExplicitTop()) {
		t.Fatal("top/no-evidence must cover explicit-top evidence")
	}
	if GradualTop().Covers(Top()) {
		t.Fatal("gradual-top evidence must not cover no-evidence top")
	}
	if ExplicitTop().Covers(GradualTop()) || GradualTop().Covers(ExplicitTop()) {
		t.Fatal("gradual-top and explicit-top evidence must be incomparable")
	}
	if Top().Hash() == GradualTop().Hash() || Top().Hash() == ExplicitTop().Hash() || GradualTop().Hash() == ExplicitTop().Hash() {
		t.Fatal("distinct evidence states should not hash identically")
	}
}
