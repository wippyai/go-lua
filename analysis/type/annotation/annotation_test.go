package annotation

import "testing"

func TestAnnotationPayloadAccessors(t *testing.T) {
	got := Annotation{Name: "min", Arg: Int64Arg(1)}
	if got.Name != "min" {
		t.Fatalf("annotation name = %q", got.Name)
	}
	if v, ok := got.Arg.AsInt64(); !ok || v != 1 {
		t.Fatalf("annotation payload = (%d, %v), want (1, true)", v, ok)
	}
}

func TestAnnotationIdentityIncludesPayload(t *testing.T) {
	minOne := Annotation{Name: "min", Arg: Int64Arg(1)}
	alsoMinOne := Annotation{Name: "min", Arg: Int64Arg(1)}
	minTwo := Annotation{Name: "min", Arg: Int64Arg(2)}

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

func TestAnnotationPayloadKindsEqualAndHash(t *testing.T) {
	tests := []struct {
		name      string
		a         Payload
		b         Payload
		different Payload
	}{
		{name: "none", a: Payload{}, b: Payload{}, different: StringArg("")},
		{name: "string", a: StringArg("x"), b: StringArg("x"), different: StringArg("y")},
		{name: "bool", a: BoolArg(true), b: BoolArg(true), different: BoolArg(false)},
		{name: "int", a: IntArg(1), b: IntArg(1), different: IntArg(2)},
		{name: "int64", a: Int64Arg(1), b: Int64Arg(1), different: Int64Arg(2)},
		{name: "float64", a: Float64Arg(1.5), b: Float64Arg(1.5), different: Float64Arg(2.5)},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if !tt.a.Equal(tt.b) {
				t.Fatal("same payload should be equal")
			}
			if tt.a.Hash() != tt.b.Hash() {
				t.Fatal("same payload should have same hash")
			}
			if tt.a.Equal(tt.different) {
				t.Fatal("different payload should not be equal")
			}
			if tt.a.Hash() == tt.different.Hash() {
				t.Fatal("different scalar payload should produce different hash")
			}
		})
	}
}

func TestPayloadFromSupportsClosedSet(t *testing.T) {
	tests := []struct {
		name string
		in   any
		want Payload
	}{
		{name: "nil", in: nil, want: Payload{}},
		{name: "string", in: "x", want: StringArg("x")},
		{name: "bool", in: true, want: BoolArg(true)},
		{name: "int", in: int(1), want: IntArg(1)},
		{name: "int64", in: int64(1), want: Int64Arg(1)},
		{name: "float64", in: float64(1.5), want: Float64Arg(1.5)},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := PayloadFrom(tt.in)
			if !ok {
				t.Fatal("supported payload was rejected")
			}
			if !got.Equal(tt.want) {
				t.Fatalf("payload = %#v, want %#v", got, tt.want)
			}
			if got.Hash() != tt.want.Hash() {
				t.Fatal("converted payload hash differs from constructor payload hash")
			}
		})
	}
}

func TestPayloadFromRejectsUnsupportedPayload(t *testing.T) {
	if got, ok := PayloadFrom(struct{ Value int }{Value: 1}); ok {
		t.Fatalf("unsupported payload was accepted: %#v", got)
	}
}
