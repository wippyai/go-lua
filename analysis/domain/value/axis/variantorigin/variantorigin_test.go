package variantorigin

import "testing"

func TestJoinUnionsCasesWithinFamily(t *testing.T) {
	got := Join(Singleton(7, 2), Of(7, []int{1, 2}))
	want := Of(7, []int{1, 2})
	if !Equal(got, want) {
		t.Fatalf("Join same family = %#v, want %#v", got, want)
	}
}

func TestJoinDifferentFamiliesIsTop(t *testing.T) {
	got := Join(Singleton(7, 0), Singleton(8, 0))
	if !got.IsTop() {
		t.Fatalf("Join different families = %#v, want Top", got)
	}
}

func TestNarrowCase(t *testing.T) {
	v := Of(7, []int{0, 1, 2})
	got := v.NarrowCase(7, 1, true)
	if !Equal(got, Singleton(7, 1)) {
		t.Fatalf("Narrow equal = %#v, want singleton case", got)
	}

	got = v.NarrowCase(7, 1, false)
	if !Equal(got, Of(7, []int{0, 2})) {
		t.Fatalf("Narrow not-equal = %#v, want excluded case", got)
	}

	got = Singleton(7, 1).NarrowCase(7, 1, false)
	if !got.IsBottom() {
		t.Fatalf("exclude only case = %#v, want Bottom", got)
	}
}

func TestNarrowCaseDifferentFamily(t *testing.T) {
	v := Of(8, []int{0, 1})

	got := v.NarrowCase(7, 1, true)
	if !got.IsBottom() {
		t.Fatalf("equal against different family = %#v, want Bottom", got)
	}

	got = v.NarrowCase(7, 1, false)
	if !Equal(got, v) {
		t.Fatalf("not-equal against different family = %#v, want unchanged %#v", got, v)
	}
}

func TestHashFollowsCanonicalCaseOrder(t *testing.T) {
	a := Of(7, []int{2, 1, 1})
	b := Of(7, []int{1, 2})
	if !Equal(a, b) {
		t.Fatalf("canonical values not equal: %#v vs %#v", a, b)
	}
	if a.Hash() != b.Hash() {
		t.Fatalf("equal values hash differently: %d vs %d", a.Hash(), b.Hash())
	}
}
