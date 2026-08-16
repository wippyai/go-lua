package normalize

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/type/typ"
)

func TestUnionForEvidence(t *testing.T) {
	tests := []struct {
		name    string
		members []typ.Type
		want    typ.Type
	}{
		{
			name:    "empty evidence is never",
			members: nil,
			want:    typ.Never,
		},
		{
			name:    "flattens union inputs",
			members: []typ.Type{typ.MaterializeUnion([]typ.Type{typ.String, typ.Number}), typ.Boolean},
			want:    typ.MaterializeUnion([]typ.Type{typ.String, typ.Number, typ.Boolean}),
		},
		{
			name:    "nil plus scalar remains optional",
			members: []typ.Type{typ.Nil, typ.String},
			want:    typ.MaterializeOptional(typ.String),
		},
		{
			name:    "flattens optional inputs",
			members: []typ.Type{typ.MaterializeOptional(typ.String), typ.Number},
			want:    typ.MaterializeUnion([]typ.Type{typ.Nil, typ.String, typ.Number}),
		},
		{
			name:    "filters unknown under concrete evidence",
			members: []typ.Type{typ.Unknown, typ.String},
			want:    typ.String,
		},
		{
			name:    "preserves unknown with nil",
			members: []typ.Type{typ.Unknown, typ.Nil},
			want:    typ.MaterializeOptional(typ.Unknown),
		},
		{
			name:    "any alone collapses by cardinality",
			members: []typ.Type{typ.Any},
			want:    typ.Any,
		},
		{
			name:    "any preserves concrete evidence",
			members: []typ.Type{typ.Any, typ.String},
			want:    typ.MaterializeUnion([]typ.Type{typ.Any, typ.String}),
		},
		{
			name:    "any plus nil preserves nilability",
			members: []typ.Type{typ.Any, typ.Nil},
			want:    typ.MaterializeOptional(typ.Any),
		},
		{
			name:    "never identity",
			members: []typ.Type{typ.Never, typ.String},
			want:    typ.String,
		},
		{
			name:    "literal base subsumption",
			members: []typ.Type{typ.LiteralString("ready"), typ.String},
			want:    typ.String,
		},
		{
			name:    "integer number subsumption",
			members: []typ.Type{typ.Integer, typ.Number},
			want:    typ.Number,
		},
		{
			name:    "same scalar remains scalar",
			members: []typ.Type{typ.String, typ.String},
			want:    typ.String,
		},
		{
			name:    "unknown refines to concrete projection",
			members: []typ.Type{typ.Unknown, typ.String},
			want:    typ.String,
		},
		{
			name:    "unknown remains when no concrete projection exists",
			members: []typ.Type{typ.Unknown, typ.Never},
			want:    typ.Unknown,
		},
		{
			name:    "any does not absorb concrete projection",
			members: []typ.Type{typ.Any, typ.String},
			want:    typ.MaterializeUnion([]typ.Type{typ.Any, typ.String}),
		},
		{
			name:    "never is ignored as an impossible projection",
			members: []typ.Type{typ.Never, typ.String},
			want:    typ.String,
		},
		{
			name:    "nil and optional projections preserve nilability",
			members: []typ.Type{typ.MaterializeOptional(typ.String), typ.Number},
			want:    typ.MaterializeUnion([]typ.Type{typ.Nil, typ.String, typ.Number}),
		},
		{
			name:    "unknown plus nil remains optional unknown",
			members: []typ.Type{typ.Unknown, typ.Nil},
			want:    typ.MaterializeOptional(typ.Unknown),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := UnionForEvidence(tt.members...)
			if !typ.TypeEquals(got, tt.want) {
				t.Fatalf("UnionForEvidence(%v) = %v, want %v", tt.members, got, tt.want)
			}
		})
	}
}

func TestUnionForEvidenceAnyEvidenceShape(t *testing.T) {
	got := UnionForEvidence(typ.Any, typ.String)
	union, ok := got.(*typ.Union)
	if !ok {
		t.Fatalf("UnionForEvidence(any, string) = %T %[1]v, want explicit union", got)
	}
	assertExactMembers(t, "any/string evidence members", union.Members, typ.MaterializeUnion([]typ.Type{typ.Any, typ.String}).(*typ.Union).Members)

	got = UnionForEvidence(typ.Any, typ.Nil)
	optional, ok := got.(*typ.Optional)
	if !ok {
		t.Fatalf("UnionForEvidence(any, nil) = %T %[1]v, want explicit optional", got)
	}
	if !typ.TypeEquals(optional.Inner, typ.Any) {
		t.Fatalf("optional inner = %v, want any", optional.Inner)
	}
}

func TestUnionForEvidenceRawContainerPolicy(t *testing.T) {
	rawNested := &typ.Union{
		Members: []typ.Type{
			typ.Number,
			typ.Integer,
			typ.LiteralString("ready"),
			typ.String,
			typ.String,
			nil,
			typ.Never,
			typ.Unknown,
			typ.MaterializeUnion([]typ.Type{typ.Boolean, typ.Boolean}),
			typ.MaterializeOptional(typ.Number),
		},
	}

	got := UnionForEvidence(rawNested)
	union, ok := got.(*typ.Union)
	if !ok {
		t.Fatalf("UnionForEvidence(raw union) = %T %[1]v, want concrete union", got)
	}

	assertExactMembers(t, "raw union policy members", union.Members, []typ.Type{
		typ.Nil,
		typ.Boolean,
		typ.Number,
		typ.String,
	})
}

func TestUnionForEvidenceRawUnknownNilPolicy(t *testing.T) {
	rawNested := &typ.Union{
		Members: []typ.Type{
			nil,
			typ.Never,
			typ.Unknown,
			typ.Nil,
		},
	}

	got := UnionForEvidence(rawNested)
	if !typ.TypeEquals(got, typ.MaterializeOptional(typ.Unknown)) {
		t.Fatalf("UnionForEvidence(raw unknown/nil union) = %T %[1]v, want optional unknown", got)
	}

	optional, ok := got.(*typ.Optional)
	if !ok {
		t.Fatalf("UnionForEvidence(raw unknown/nil union) = %T %[1]v, want optional", got)
	}
	if !typ.TypeEquals(optional.Inner, typ.Unknown) {
		t.Fatalf("optional inner = %v, want unknown", optional.Inner)
	}
}

func TestOptionalSemanticPolicy(t *testing.T) {
	if got := Optional(nil); !typ.TypeEquals(got, typ.Nil) {
		t.Fatalf("Optional(nil) = %v, want nil", got)
	}
	if got := Optional(typ.Any); !typ.TypeEquals(got, typ.Any) {
		t.Fatalf("Optional(any) = %v, want any", got)
	}
	optionalString := typ.MaterializeOptional(typ.String)
	if got := Optional(optionalString); !typ.TypeEquals(got, optionalString) {
		t.Fatalf("Optional(string?) = %v, want string?", got)
	}
	union := typ.MaterializeUnion([]typ.Type{typ.String, typ.Number})
	got := Optional(union)
	want := typ.MaterializeUnion([]typ.Type{typ.Nil, typ.String, typ.Number})
	if !typ.TypeEquals(got, want) {
		t.Fatalf("Optional(union) = %v, want union with nil %v", got, want)
	}
}

func TestIntersectionForMeet(t *testing.T) {
	tests := []struct {
		name    string
		members []typ.Type
		want    typ.Type
	}{
		{name: "empty meet is any", members: nil, want: typ.Any},
		{name: "flattens intersection inputs", members: []typ.Type{typ.MaterializeIntersection([]typ.Type{typ.Any, typ.String}), typ.Boolean}, want: typ.MaterializeIntersection([]typ.Type{typ.String, typ.Boolean})},
		{name: "any identity left", members: []typ.Type{typ.Any, typ.String}, want: typ.String},
		{name: "any identity right", members: []typ.Type{typ.String, typ.Any}, want: typ.String},
		{name: "never absorbs left", members: []typ.Type{typ.Never, typ.String}, want: typ.Never},
		{name: "never absorbs right", members: []typ.Type{typ.String, typ.Never}, want: typ.Never},
		{name: "nil accepted by optional", members: []typ.Type{typ.Nil, typ.MaterializeOptional(typ.String)}, want: typ.Nil},
		{name: "nil accepted by unknown", members: []typ.Type{typ.Nil, typ.Unknown}, want: typ.Nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := IntersectionForMeet(tt.members...)
			if !typ.TypeEquals(got, tt.want) {
				t.Fatalf("IntersectionForMeet(%v) = %v, want %v", tt.members, got, tt.want)
			}
		})
	}

	got := IntersectionForMeet(typ.Nil, typ.String)
	inter, ok := got.(*typ.Intersection)
	if !ok {
		t.Fatalf("intersection meet nil & string = %T %[1]v, want explicit intersection", got)
	}
	if !intersectionHasMember(inter, typ.Nil) || !intersectionHasMember(inter, typ.String) {
		t.Fatalf("intersection meet nil & string members = %v, want nil and string", inter.Members)
	}
}

func TestIntersectionForMeetCollapsesContradictoryBooleanScalars(t *testing.T) {
	if got := IntersectionForMeet(typ.LiteralBool(false), typ.LiteralBool(true)); got != typ.Never {
		t.Fatalf("IntersectionForMeet(false, true) = %v, want never", got)
	}
}

func TestIntersectionForMeetRawContainerPolicy(t *testing.T) {
	rawNested := &typ.Intersection{
		Members: []typ.Type{
			typ.Any,
			typ.String,
			typ.String,
			nil,
			&typ.Intersection{
				Members: []typ.Type{
					typ.Boolean,
					typ.Any,
				},
			},
		},
	}

	got := IntersectionForMeet(rawNested, typ.Nil)
	inter, ok := got.(*typ.Intersection)
	if !ok {
		t.Fatalf("IntersectionForMeet(raw intersection, nil) = %T %[1]v, want concrete intersection", got)
	}

	assertExactMembers(t, "raw intersection policy members", inter.Members, []typ.Type{
		typ.Nil,
		typ.Boolean,
		typ.String,
	})
}

func TestIntersectionForMeetRawNilAcceptancePolicy(t *testing.T) {
	rawNested := &typ.Intersection{
		Members: []typ.Type{
			nil,
			typ.Any,
			typ.MaterializeOptional(typ.String),
			typ.Nil,
		},
	}

	got := IntersectionForMeet(rawNested)
	if !typ.TypeEquals(got, typ.Nil) {
		t.Fatalf("IntersectionForMeet(raw optional/nil intersection) = %T %[1]v, want nil", got)
	}
}

func intersectionHasMember(inter *typ.Intersection, want typ.Type) bool {
	for _, member := range inter.Members {
		if typ.TypeEquals(member, want) {
			return true
		}
	}
	return false
}

func assertExactMembers(t *testing.T, label string, got, want []typ.Type) {
	t.Helper()

	if len(got) != len(want) {
		t.Fatalf("%s length = %d (%v), want %d (%v)", label, len(got), got, len(want), want)
	}
	for i := range want {
		if !typ.TypeEquals(got[i], want[i]) {
			t.Fatalf("%s[%d] = %v, want %v; full members = %v", label, i, got[i], want[i], got)
		}
	}
}
