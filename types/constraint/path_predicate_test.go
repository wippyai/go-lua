package constraint

import (
	"reflect"
	"testing"

	"github.com/wippyai/go-lua/types/narrow"
)

func TestSinglePathPredicateNormalizesPredicateConstraints(t *testing.T) {
	path := Path{Root: "value"}
	key := narrow.BuiltinTypeKey("string")

	tests := []struct {
		name string
		in   Constraint
		want PathPredicate
	}{
		{
			name: "truthy",
			in:   Truthy{Path: path},
			want: PathPredicate{Path: path, Kind: PathPredicateTruthy},
		},
		{
			name: "falsy",
			in:   Falsy{Path: path},
			want: PathPredicate{Path: path, Kind: PathPredicateFalsy},
		},
		{
			name: "nil",
			in:   IsNil{Path: path},
			want: PathPredicate{Path: path, Kind: PathPredicateIsNil},
		},
		{
			name: "not nil",
			in:   NotNil{Path: path},
			want: PathPredicate{Path: path, Kind: PathPredicateNotNil},
		},
		{
			name: "has type",
			in:   HasType{Path: path, Type: key},
			want: PathPredicate{Path: path, Kind: PathPredicateHasType, Type: key},
		},
		{
			name: "not has type",
			in:   NotHasType{Path: path, Type: key},
			want: PathPredicate{Path: path, Kind: PathPredicateNotHasType, Type: key},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := SinglePathPredicate(tt.in)
			if !ok {
				t.Fatalf("SinglePathPredicate(%T) returned false", tt.in)
			}
			if !reflect.DeepEqual(got.Path, tt.want.Path) || got.Kind != tt.want.Kind || !got.Type.Equal(tt.want.Type) {
				t.Fatalf("SinglePathPredicate(%T) = %#v, want %#v", tt.in, got, tt.want)
			}
		})
	}
}

func TestSinglePathPredicateRejectsNonPredicatesAndZeroTypeKeys(t *testing.T) {
	path := Path{Root: "value"}

	tests := []Constraint{
		EqPath{Left: path, Right: Path{Root: "other"}},
		HasField{Path: path, Field: "kind"},
		HasType{Path: path},
		NotHasType{Path: path},
	}

	for _, in := range tests {
		if got, ok := SinglePathPredicate(in); ok {
			t.Fatalf("SinglePathPredicate(%T) = %#v, want rejected", in, got)
		}
	}
}

func TestPathPredicateNonNilBranch(t *testing.T) {
	tests := []struct {
		kind PathPredicateKind
		want bool
		ok   bool
	}{
		{kind: PathPredicateTruthy, want: true, ok: true},
		{kind: PathPredicateNotNil, want: true, ok: true},
		{kind: PathPredicateFalsy, want: false, ok: true},
		{kind: PathPredicateIsNil, want: false, ok: true},
		{kind: PathPredicateHasType, ok: false},
		{kind: PathPredicateNotHasType, ok: false},
	}

	for _, tt := range tests {
		got, ok := (PathPredicate{Kind: tt.kind}).NonNilBranch()
		if ok != tt.ok || got != tt.want {
			t.Fatalf("NonNilBranch(%v) = (%v, %v), want (%v, %v)", tt.kind, got, ok, tt.want, tt.ok)
		}
	}
}
