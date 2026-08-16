package typeexpr

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/type/annotation"
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

func TestUnionCanonicalizesNestedUnion(t *testing.T) {
	u1 := Union(typ.Number, Union(typ.String, typ.Boolean))
	u2 := Union(Union(typ.Boolean, typ.Number), typ.String)
	u3 := Union(typ.String, typ.Boolean, typ.Number)

	if !u1.Equals(u2) {
		t.Fatalf("u1 = %v should equal u2 = %v", u1, u2)
	}
	if !u2.Equals(u3) {
		t.Fatalf("u2 = %v should equal u3 = %v", u2, u3)
	}
	if u1.Hash() != u2.Hash() || u2.Hash() != u3.Hash() {
		t.Fatalf("canonical unions should have same hash: %d %d %d", u1.Hash(), u2.Hash(), u3.Hash())
	}
}

func TestUnionIdempotentAcrossNestedUnion(t *testing.T) {
	base := Union(typ.Number, typ.String)
	extended := Union(base, typ.Number)

	if !base.Equals(extended) {
		t.Fatalf("adding existing member should not change union: %v vs %v", base, extended)
	}
}

func TestUnionExpandsOptionalMember(t *testing.T) {
	optionalString := typ.MaterializeOptional(typ.String)

	got := Union(optionalString, typ.Number)

	requireUnionMembers(t, got, typ.Nil, typ.String, typ.Number)
}

func TestUnionWithNilCreatesOptional(t *testing.T) {
	got := Union(typ.Number, typ.Nil)

	if got.Kind() != kind.Optional {
		t.Fatalf("number | nil should become number?, got %v", got.Kind())
	}

	opt := got.(*typ.Optional)
	if opt.Inner != typ.Number {
		t.Fatalf("Inner = %v, want number", opt.Inner)
	}
}

func TestUnionNilFoldingDoesNotAbsorbAny(t *testing.T) {
	got := Union(typ.Any, typ.Nil)
	opt, ok := got.(*typ.Optional)
	if !ok {
		t.Fatalf("any | nil should be represented as optional any, got %T %[1]v", got)
	}
	if opt.Inner != typ.Any {
		t.Fatalf("optional inner = %v, want any", opt.Inner)
	}
}

func TestUnionPreservesAnyNeverAndUnknownMembers(t *testing.T) {
	requireUnionMembers(t, Union(typ.Number, typ.String, typ.Any), typ.Number, typ.String, typ.Any)
	requireUnionMembers(t, Union(typ.Number, typ.Never, typ.String), typ.Number, typ.Never, typ.String)
	requireUnionMembers(t, Union(typ.Unknown, typ.String), typ.Unknown, typ.String)

	if got := Union(typ.Unknown); got != typ.Unknown {
		t.Fatalf("unknown alone should remain unknown, got %v", got)
	}

	withNil := Union(typ.Unknown, typ.Nil)
	opt, ok := withNil.(*typ.Optional)
	if !ok {
		t.Fatalf("unknown | nil should be optional, got %T %[1]v", withNil)
	}
	if opt.Inner != typ.Unknown {
		t.Fatalf("unknown | nil inner should be unknown, got %v", opt.Inner)
	}
}

func TestUnionPreservesLiteralMembersWithBaseTypes(t *testing.T) {
	tests := []struct {
		name  string
		input []typ.Type
		want  []typ.Type
	}{
		{
			name:  "string literal with string",
			input: []typ.Type{typ.String, typ.LiteralString("")},
			want:  []typ.Type{typ.String, typ.LiteralString("")},
		},
		{
			name:  "number literal with number",
			input: []typ.Type{typ.Number, typ.LiteralNumber(42)},
			want:  []typ.Type{typ.Number, typ.LiteralNumber(42)},
		},
		{
			name:  "boolean literal with boolean",
			input: []typ.Type{typ.Boolean, typ.LiteralBool(true)},
			want:  []typ.Type{typ.Boolean, typ.LiteralBool(true)},
		},
		{
			name:  "integer literal with integer",
			input: []typ.Type{typ.Integer, typ.LiteralInt(7)},
			want:  []typ.Type{typ.Integer, typ.LiteralInt(7)},
		},
		{
			name:  "integer with number",
			input: []typ.Type{typ.Number, typ.Integer},
			want:  []typ.Type{typ.Number, typ.Integer},
		},
		{
			name:  "integer literal with number",
			input: []typ.Type{typ.Number, typ.LiteralInt(7)},
			want:  []typ.Type{typ.Number, typ.LiteralInt(7)},
		},
		{
			name:  "multiple string literals with string",
			input: []typ.Type{typ.String, typ.LiteralString("a"), typ.LiteralString("b")},
			want:  []typ.Type{typ.String, typ.LiteralString("a"), typ.LiteralString("b")},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			requireUnionMembers(t, Union(tt.input...), tt.want...)
		})
	}

	got := Union(Optional(typ.String), typ.LiteralString(""))
	requireUnionMembers(t, got, typ.Nil, typ.String, typ.LiteralString(""))
}

func TestUnionNestedOptionalAndUnionNilDedups(t *testing.T) {
	inner := Union(typ.Nil, typ.Number, typ.String)
	outer := Union(Optional(typ.Boolean), inner)

	requireUnionMembers(t, outer, typ.Nil, typ.Number, typ.String, typ.Boolean)
}

func TestUnionAnnotatedMembersDoNotPanic(t *testing.T) {
	annotatedOpt := typ.NewAnnotated(Optional(typ.String), []annotation.Annotation{{Name: "min_len", Arg: annotation.Int64Arg(1)}})
	if got := Union(annotatedOpt, typ.Number); got == nil {
		t.Fatal("union should not be nil")
	}

	inner := Union(typ.String, typ.Number)
	annotatedUnion := typ.NewAnnotated(inner, []annotation.Annotation{{Name: "max_len", Arg: annotation.Int64Arg(255)}})
	if got := Union(annotatedUnion, typ.Boolean); got == nil {
		t.Fatal("union should not be nil")
	}
}

func TestOptionalNilAndNestedIdentities(t *testing.T) {
	if got := Optional(typ.Nil); got != typ.Nil {
		t.Fatalf("Optional(nil) = %v, want nil", got)
	}

	inner := Optional(typ.Number)
	if got := Optional(inner); got != inner {
		t.Fatalf("Optional(optional) = %v, want original optional %v", got, inner)
	}
}

func TestOptionalAnyCollapsesToAny(t *testing.T) {
	if got := Optional(typ.Any); got != typ.Any {
		t.Fatalf("Optional(any) = %v, want any", got)
	}
}

func TestOptionalUnionWithNilIsIdempotent(t *testing.T) {
	inner := Union(typ.Number, typ.String, typ.Nil)

	got := Optional(inner)

	if !got.Equals(inner) {
		t.Fatalf("Optional(union containing nil) = %v, want %v", got, inner)
	}
	if got.Hash() != inner.Hash() {
		t.Fatalf("hash = %d, want %d", got.Hash(), inner.Hash())
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

func TestOptionalAnnotatedUnionDoesNotPanic(t *testing.T) {
	inner := Union(typ.String, typ.Number)
	annotated := typ.NewAnnotated(inner, []annotation.Annotation{{Name: "max_len", Arg: annotation.Int64Arg(255)}})
	if got := Optional(annotated); got == nil {
		t.Fatal("optional should not be nil")
	}
}

func TestOptionalUnionPreservesRecursiveMemberHashes(t *testing.T) {
	recA := typ.NewRecursive("Node", func(self typ.Type) typ.Type {
		return typ.NewArray(self)
	})
	recB := typ.NewRecursive("Node", func(self typ.Type) typ.Type {
		return typ.NewMap(typ.String, self)
	})
	u := Union(recA, recB)

	got := Optional(u)
	want := Union(typ.Nil, recA, recB)
	if !got.Equals(want) {
		t.Fatalf("Optional(union) = %v, want %v", got, want)
	}
	if got.Hash() != want.Hash() {
		t.Fatalf("hash = %d, want %d", got.Hash(), want.Hash())
	}
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

func TestIntersectionPreservesMembersWithoutMeetPolicy(t *testing.T) {
	requireIntersectionMembers(t, Intersection(typ.Number, typ.Never), typ.Number, typ.Never)
	requireIntersectionMembers(t, Intersection(typ.Number, typ.Any, typ.String), typ.Number, typ.Any, typ.String)
	requireIntersectionMembers(t, Intersection(typ.Nil, Optional(typ.String)), typ.Nil, Optional(typ.String))
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
