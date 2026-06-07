package constraint

import (
	"reflect"
	"testing"
)

func TestDirectPathRelationNormalizesEqualityConstraints(t *testing.T) {
	left := Path{Root: "left"}
	right := Path{Root: "right"}

	tests := []struct {
		name string
		in   Constraint
		kind PathRelationKind
	}{
		{
			name: "equal",
			in:   NewEqPath(left, right),
			kind: PathRelationEqual,
		},
		{
			name: "not equal",
			in:   NewNotEqPath(left, right),
			kind: PathRelationNotEqual,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := DirectPathRelation(tt.in)
			if !ok {
				t.Fatalf("DirectPathRelation(%T) returned false", tt.in)
			}
			if !reflect.DeepEqual(got.Left, left) || !reflect.DeepEqual(got.Right, right) || got.Kind != tt.kind {
				t.Fatalf("DirectPathRelation(%T) = %#v, want left=%#v right=%#v kind=%v", tt.in, got, left, right, tt.kind)
			}
		})
	}
}

func TestDirectPathRelationRejectsNonDirectRelations(t *testing.T) {
	path := Path{Root: "value"}

	tests := []Constraint{
		FieldEqualsPath{Target: path, Field: "id", Value: Path{Root: "other"}},
		IndexEqualsPath{Target: path, Value: Path{Root: "other"}},
		Truthy{Path: path},
		HasField{Path: path, Field: "id"},
	}

	for _, in := range tests {
		if got, ok := DirectPathRelation(in); ok {
			t.Fatalf("DirectPathRelation(%T) = %#v, want rejected", in, got)
		}
	}
}

func TestPathRelationIsEquality(t *testing.T) {
	tests := []struct {
		kind PathRelationKind
		want bool
		ok   bool
	}{
		{kind: PathRelationEqual, want: true, ok: true},
		{kind: PathRelationNotEqual, want: false, ok: true},
		{kind: PathRelationInvalid, ok: false},
	}

	for _, tt := range tests {
		got, ok := (PathRelation{Kind: tt.kind}).IsEquality()
		if got != tt.want || ok != tt.ok {
			t.Fatalf("IsEquality(%v) = (%v, %v), want (%v, %v)", tt.kind, got, ok, tt.want, tt.ok)
		}
	}
}
