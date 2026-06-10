package relation

import (
	"testing"

	. "github.com/wippyai/go-lua/analysis/type/typ"
)

func TestNormalizeUnionForProjectionUsesAccessProjectionPolicy(t *testing.T) {
	tests := []struct {
		name    string
		members []Type
		want    Type
	}{
		{
			name:    "same scalar remains scalar",
			members: []Type{String, String},
			want:    String,
		},
		{
			name:    "unknown refines to concrete projection",
			members: []Type{Unknown, String},
			want:    String,
		},
		{
			name:    "unknown remains when no concrete projection exists",
			members: []Type{Unknown, Never},
			want:    Unknown,
		},
		{
			name:    "any absorbs concrete projection",
			members: []Type{Any, String},
			want:    Any,
		},
		{
			name:    "never is ignored as an impossible projection",
			members: []Type{Never, String},
			want:    String,
		},
		{
			name:    "nil and optional projections preserve nilability",
			members: []Type{NewOptional(String), Number},
			want:    NewUnion(Nil, String, Number),
		},
		{
			name:    "unknown plus nil remains optional unknown",
			members: []Type{Unknown, Nil},
			want:    NewOptional(Unknown),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := NormalizeUnionForProjection(tt.members...)
			if !TypeEquals(got, tt.want) {
				t.Fatalf("NormalizeUnionForProjection(%v) = %v, want %v", tt.members, got, tt.want)
			}
		})
	}
}
