package typeexpr

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/type/kind"
	"github.com/wippyai/go-lua/analysis/type/typ"
)

func TestUnionFlattensNestedUnion(t *testing.T) {
	inner := typ.MaterializeUnion([]typ.Type{typ.Number, typ.String})

	got := Union(inner, typ.Boolean)
	want := typ.MaterializeUnion([]typ.Type{typ.Number, typ.String, typ.Boolean})

	if !got.Equals(want) {
		t.Fatalf("Union(nested, boolean) = %v, want %v", got, want)
	}
	requireUnionMembers(t, got, typ.Number, typ.String, typ.Boolean)
}

func TestUnionExpandsOptionalMember(t *testing.T) {
	optionalString := typ.MaterializeOptional(typ.String)

	got := Union(optionalString, typ.Number)

	requireUnionMembers(t, got, typ.Nil, typ.String, typ.Number)
}

func TestOptionalAnyCollapsesToAny(t *testing.T) {
	if got := Optional(typ.Any); got != typ.Any {
		t.Fatalf("Optional(any) = %v, want any", got)
	}
}

func TestOptionalUnionAddsNilAndMaterializesUnion(t *testing.T) {
	inner := typ.MaterializeUnion([]typ.Type{typ.String, typ.Number})

	got := Optional(inner)
	want := typ.MaterializeUnion([]typ.Type{typ.Nil, typ.String, typ.Number})

	if got.Kind() != kind.Union {
		t.Fatalf("Optional(union) kind = %v, want union", got.Kind())
	}
	if !got.Equals(want) {
		t.Fatalf("Optional(union) = %v, want %v", got, want)
	}
	requireUnionMembers(t, got, typ.Nil, typ.String, typ.Number)
}

func TestIntersectionFlattensNestedIntersection(t *testing.T) {
	inner := typ.MaterializeIntersection([]typ.Type{typ.Number, typ.String})

	got := Intersection(inner, typ.Boolean)
	want := typ.MaterializeIntersection([]typ.Type{typ.Number, typ.String, typ.Boolean})

	if !got.Equals(want) {
		t.Fatalf("Intersection(nested, boolean) = %v, want %v", got, want)
	}
	requireIntersectionMembers(t, got, typ.Number, typ.String, typ.Boolean)
}

func TestEmptyUnionAndIntersectionIdentities(t *testing.T) {
	if got := Union(); got != typ.Never {
		t.Fatalf("Union() = %v, want never", got)
	}
	if got := Intersection(); got != typ.Any {
		t.Fatalf("Intersection() = %v, want any", got)
	}
}

func requireUnionMembers(t *testing.T, got typ.Type, wants ...typ.Type) {
	t.Helper()

	union, ok := got.(*typ.Union)
	if !ok {
		t.Fatalf("got %T %[1]v, want union", got)
	}
	if len(union.Members) != len(wants) {
		t.Fatalf("union members = %v, want %v", union.Members, wants)
	}
	for _, want := range wants {
		if !union.Contains(want) {
			t.Fatalf("union members = %v, missing %v", union.Members, want)
		}
	}
}

func requireIntersectionMembers(t *testing.T, got typ.Type, wants ...typ.Type) {
	t.Helper()

	intersection, ok := got.(*typ.Intersection)
	if !ok {
		t.Fatalf("got %T %[1]v, want intersection", got)
	}
	if len(intersection.Members) != len(wants) {
		t.Fatalf("intersection members = %v, want %v", intersection.Members, wants)
	}
	for _, want := range wants {
		if !hasType(intersection.Members, want) {
			t.Fatalf("intersection members = %v, missing %v", intersection.Members, want)
		}
	}
}

func hasType(types []typ.Type, want typ.Type) bool {
	for _, candidate := range types {
		if candidate.Equals(want) {
			return true
		}
	}
	return false
}
