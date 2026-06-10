package evidence

import "testing"

func TestGradualTopJoinKeepsOnlyCommonProof(t *testing.T) {
	if got := Join(GradualTop(), GradualTop()); !Equal(got, GradualTop()) {
		t.Fatalf("gradual proof joined with itself = %s, want gradual-top", got)
	}
	if got := Join(GradualTop(), Top()); !Equal(got, Top()) {
		t.Fatalf("gradual proof joined with top = %s, want top", got)
	}
	if got := Join(Bottom(), GradualTop()); !Equal(got, GradualTop()) {
		t.Fatalf("bottom joined with gradual proof = %s, want gradual-top", got)
	}
}

func TestGradualTopOrderAndHash(t *testing.T) {
	if !Top().Covers(GradualTop()) {
		t.Fatal("top/no-evidence must cover gradual-top evidence")
	}
	if GradualTop().Covers(Top()) {
		t.Fatal("gradual-top evidence must not cover no-evidence top")
	}
	if Top().Hash() == GradualTop().Hash() {
		t.Fatal("distinct evidence states should not hash identically")
	}
}
