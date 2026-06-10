package annotation

import "testing"

func TestAnnotationPayload(t *testing.T) {
	got := Annotation{Name: "min", Arg: int64(1)}
	if got.Name != "min" || got.Arg != int64(1) {
		t.Fatalf("annotation payload = %#v", got)
	}
}

func TestAnnotationIdentityIncludesPayload(t *testing.T) {
	minOne := Annotation{Name: "min", Arg: int64(1)}
	alsoMinOne := Annotation{Name: "min", Arg: int64(1)}
	minTwo := Annotation{Name: "min", Arg: int64(2)}

	if !minOne.Equal(alsoMinOne) {
		t.Fatal("same annotation payload should be equal")
	}
	if minOne.Hash() != alsoMinOne.Hash() {
		t.Fatal("same annotation payload should have same hash")
	}
	if minOne.Equal(minTwo) {
		t.Fatal("different annotation args should not be equal")
	}
	if minOne.Hash() == minTwo.Hash() {
		t.Fatal("different scalar annotation args should produce different hashes")
	}
}
