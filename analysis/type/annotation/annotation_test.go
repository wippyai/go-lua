package annotation

import "testing"

func TestAnnotationPayload(t *testing.T) {
	got := Annotation{Name: "min", Arg: int64(1)}
	if got.Name != "min" || got.Arg != int64(1) {
		t.Fatalf("annotation payload = %#v", got)
	}
}
