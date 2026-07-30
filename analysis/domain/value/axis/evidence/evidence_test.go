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

func TestEvidenceJoinCarriesBoundedOriginsForCommonProof(t *testing.T) {
	left := GradualTop().WithOrigin(Origin{Kind: OriginSource, ID: 20})
	right := GradualTop().WithOrigin(Origin{Kind: OriginBranch, ID: 10})

	got := Join(left, right)
	if !got.IsGradualTop() {
		t.Fatalf("joined evidence = %s, want gradual-top proof", got)
	}
	want := []Origin{
		{Kind: OriginSource, ID: 20},
		{Kind: OriginBranch, ID: 10},
	}
	if origins := got.Origins(); !sameOrigins(origins, want) {
		t.Fatalf("joined origins = %#v, want %#v", origins, want)
	}
}

func TestEvidenceJoinDropsOriginsWhenProofKindDiverges(t *testing.T) {
	gradual := GradualTop().WithOrigin(Origin{Kind: OriginSource, ID: 1})
	explicit := ExplicitTop().WithOrigin(Origin{Kind: OriginAnnotation, ID: 2})

	got := Join(gradual, explicit)
	if !Equal(got, Top()) {
		t.Fatalf("divergent proof join = %s, want top", got)
	}
	if origins := got.Origins(); len(origins) != 0 {
		t.Fatalf("divergent proof join kept origins %#v", origins)
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

func TestEvidenceMeetIntersectsOrigins(t *testing.T) {
	common := Origin{Kind: OriginSource, ID: 1}
	leftOnly := Origin{Kind: OriginBranch, ID: 2}
	rightOnly := Origin{Kind: OriginCall, ID: 3}

	left := GradualTop().WithOrigin(common).WithOrigin(leftOnly)
	right := GradualTop().WithOrigin(common).WithOrigin(rightOnly)

	got := Meet(left, right)
	if !got.IsGradualTop() {
		t.Fatalf("meet evidence = %s, want gradual-top proof", got)
	}
	if origins := got.Origins(); !sameOrigins(origins, []Origin{common}) {
		t.Fatalf("meet origins = %#v, want common origin only", origins)
	}
}

func TestEvidenceWidenBoundsOriginGrowth(t *testing.T) {
	got := GradualTop()
	for i := 10; i > 0; i-- {
		next := GradualTop().WithOrigin(Origin{Kind: OriginSource, ID: uint64(i)})
		got = Widen(got, next)
	}
	if got := len(got.Origins()); got != maxOrigins {
		t.Fatalf("widen kept %d origins, want bounded set of %d", got, maxOrigins)
	}
	if !got.OriginsTruncated() {
		t.Fatal("widen over more than max origins did not mark truncation")
	}
	want := []Origin{
		{Kind: OriginSource, ID: 1},
		{Kind: OriginSource, ID: 2},
		{Kind: OriginSource, ID: 3},
		{Kind: OriginSource, ID: 4},
	}
	if origins := got.Origins(); !sameOrigins(origins, want) {
		t.Fatalf("bounded origins = %#v, want deterministic lowest origins %#v", origins, want)
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
	withOrigin := GradualTop().WithOrigin(Origin{Kind: OriginSource, ID: 1})
	if !Join(withOrigin, GradualTop()).Covers(withOrigin) {
		t.Fatal("joined origin evidence must cover the originating proof")
	}
	if GradualTop().Hash() == withOrigin.Hash() {
		t.Fatal("origin-bearing evidence should not hash identically to origin-free evidence")
	}
}

func sameOrigins(a, b []Origin) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
