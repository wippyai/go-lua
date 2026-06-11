package normalize

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/type/identity"
	"github.com/wippyai/go-lua/analysis/type/typ"
)

func TestUnionForJoin(t *testing.T) {
	tests := []struct {
		name    string
		members []typ.Type
		want    typ.Type
	}{
		{
			name:    "flattens union inputs",
			members: []typ.Type{typ.NewUnion(typ.String, typ.Number), typ.Boolean},
			want:    typ.NewUnion(typ.String, typ.Number, typ.Boolean),
		},
		{
			name:    "flattens optional inputs",
			members: []typ.Type{typ.NewOptional(typ.String), typ.Number},
			want:    typ.NewUnion(typ.Nil, typ.String, typ.Number),
		},
		{
			name:    "filters unknown under concrete evidence",
			members: []typ.Type{typ.Unknown, typ.String},
			want:    typ.String,
		},
		{
			name:    "preserves unknown with nil",
			members: []typ.Type{typ.Unknown, typ.Nil},
			want:    typ.NewOptional(typ.Unknown),
		},
		{
			name:    "any absorbs",
			members: []typ.Type{typ.Any, typ.String},
			want:    typ.Any,
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
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := UnionForJoin(tt.members...)
			if !identity.TypeEquals(got, tt.want) {
				t.Fatalf("UnionForJoin(%v) = %v, want %v", tt.members, got, tt.want)
			}
		})
	}
}

func TestUnionForProjection(t *testing.T) {
	tests := []struct {
		name    string
		members []typ.Type
		want    typ.Type
	}{
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
			name:    "any absorbs concrete projection",
			members: []typ.Type{typ.Any, typ.String},
			want:    typ.Any,
		},
		{
			name:    "never is ignored as an impossible projection",
			members: []typ.Type{typ.Never, typ.String},
			want:    typ.String,
		},
		{
			name:    "nil and optional projections preserve nilability",
			members: []typ.Type{typ.NewOptional(typ.String), typ.Number},
			want:    typ.NewUnion(typ.Nil, typ.String, typ.Number),
		},
		{
			name:    "unknown plus nil remains optional unknown",
			members: []typ.Type{typ.Unknown, typ.Nil},
			want:    typ.NewOptional(typ.Unknown),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := UnionForProjection(tt.members...)
			if !identity.TypeEquals(got, tt.want) {
				t.Fatalf("UnionForProjection(%v) = %v, want %v", tt.members, got, tt.want)
			}
		})
	}
}

func TestIntersectionForMeet(t *testing.T) {
	tests := []struct {
		name    string
		members []typ.Type
		want    typ.Type
	}{
		{name: "flattens intersection inputs", members: []typ.Type{typ.NewIntersection(typ.Any, typ.String), typ.Boolean}, want: typ.NewIntersection(typ.String, typ.Boolean)},
		{name: "any identity left", members: []typ.Type{typ.Any, typ.String}, want: typ.String},
		{name: "any identity right", members: []typ.Type{typ.String, typ.Any}, want: typ.String},
		{name: "never absorbs left", members: []typ.Type{typ.Never, typ.String}, want: typ.Never},
		{name: "never absorbs right", members: []typ.Type{typ.String, typ.Never}, want: typ.Never},
		{name: "nil accepted by optional", members: []typ.Type{typ.Nil, typ.NewOptional(typ.String)}, want: typ.Nil},
		{name: "nil accepted by unknown", members: []typ.Type{typ.Nil, typ.Unknown}, want: typ.Nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := IntersectionForMeet(tt.members...)
			if !identity.TypeEquals(got, tt.want) {
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

func intersectionHasMember(inter *typ.Intersection, want typ.Type) bool {
	for _, member := range inter.Members {
		if identity.TypeEquals(member, want) {
			return true
		}
	}
	return false
}
