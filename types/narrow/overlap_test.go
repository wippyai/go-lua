package narrow

import (
	"testing"

	"github.com/wippyai/go-lua/types/typ"
)

func TestTypesOverlap_DiscriminatedRecords(t *testing.T) {
	left := typ.NewRecord().
		Field("kind", typ.LiteralString("left")).
		Field("value", typ.String).
		Build()
	right := typ.NewRecord().
		Field("kind", typ.LiteralString("right")).
		Field("value", typ.String).
		Build()

	if TypesOverlap(left, right) {
		t.Fatalf("records with incompatible literal discriminants must not overlap")
	}
}

func TestTypesOverlap_RecordWidthCanOverlap(t *testing.T) {
	base := typ.NewRecord().Field("kind", typ.LiteralString("item")).Build()
	wide := typ.NewRecord().
		Field("kind", typ.LiteralString("item")).
		Field("payload", typ.Number).
		Build()

	if !TypesOverlap(base, wide) {
		t.Fatalf("record width variants with compatible common fields should overlap")
	}
}

func TestTypesOverlap_RecursiveArrayProductsAreCoinductive(t *testing.T) {
	left := typ.NewRecursive("Suite", func(self typ.Type) typ.Type {
		return typ.NewRecord().
			Field("name", typ.String).
			Field("children", typ.NewArray(self)).
			Build()
	})
	right := typ.NewRecursive("Suite", func(self typ.Type) typ.Type {
		return typ.NewRecord().
			Field("name", typ.String).
			Field("children", typ.NewArray(self)).
			Build()
	})

	if !TypesOverlap(typ.NewArray(left), typ.NewArray(right)) {
		t.Fatalf("structurally equal recursive arrays should overlap")
	}
}

func TestIntersect_FiltersUnionBeforeSubtype(t *testing.T) {
	left := typ.NewRecord().
		Field("kind", typ.LiteralString("left")).
		Field("value", typ.String).
		Build()
	right := typ.NewRecord().
		Field("kind", typ.LiteralString("right")).
		Field("value", typ.String).
		Build()
	union := typ.NewUnion(left, right)
	want := typ.NewRecord().Field("kind", typ.LiteralString("left")).Build()

	got := Intersect(union, want)
	if !typ.TypeEquals(got, left) {
		t.Fatalf("Intersect(discriminated union, left predicate) = %v, want %v", got, left)
	}
}
