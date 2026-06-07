package constraint

import (
	"reflect"
	"testing"

	"github.com/wippyai/go-lua/types/typ"
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

func TestSpecializePathRelationConstraintFieldRelations(t *testing.T) {
	base := Path{Root: "record"}
	field := base.Field("id")
	value := Path{Root: "value"}

	tests := []struct {
		name string
		in   Constraint
		want Constraint
	}{
		{
			name: "left equality field",
			in:   NewEqPath(field, value),
			want: FieldEqualsPath{Target: base, Field: "id", Value: value},
		},
		{
			name: "right equality field",
			in:   NewEqPath(value, field),
			want: FieldEqualsPath{Target: base, Field: "id", Value: value},
		},
		{
			name: "left inequality field",
			in:   NewNotEqPath(field, value),
			want: FieldNotEqualsPath{Target: base, Field: "id", Value: value},
		},
		{
			name: "right inequality field",
			in:   NewNotEqPath(value, field),
			want: FieldNotEqualsPath{Target: base, Field: "id", Value: value},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := SpecializePathRelationConstraint(tt.in)
			if !got.Equals(tt.want) {
				t.Fatalf("SpecializePathRelationConstraint(%T) = %#v, want %#v", tt.in, got, tt.want)
			}
		})
	}
}

func TestSpecializePathRelationConstraintIndexRelations(t *testing.T) {
	base := Path{Root: "items"}
	index := base.IndexInt(2)
	value := Path{Root: "value"}
	key := typ.LiteralInt(2)

	tests := []struct {
		name string
		in   Constraint
		want Constraint
	}{
		{
			name: "left equality index",
			in:   NewEqPath(index, value),
			want: IndexEqualsPath{Target: base, Key: key, Value: value},
		},
		{
			name: "right equality index",
			in:   NewEqPath(value, index),
			want: IndexEqualsPath{Target: base, Key: key, Value: value},
		},
		{
			name: "left inequality index",
			in:   NewNotEqPath(index, value),
			want: IndexNotEqualsPath{Target: base, Key: key, Value: value},
		},
		{
			name: "right inequality index",
			in:   NewNotEqPath(value, index),
			want: IndexNotEqualsPath{Target: base, Key: key, Value: value},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := SpecializePathRelationConstraint(tt.in)
			if !got.Equals(tt.want) {
				t.Fatalf("SpecializePathRelationConstraint(%T) = %#v, want %#v", tt.in, got, tt.want)
			}
		})
	}
}

func TestSplitIndexPath(t *testing.T) {
	base := Path{Root: "items"}.Field("nested")
	path := base.IndexStr("id")

	parent, key, ok := SplitIndexPath(path)
	if !ok {
		t.Fatal("SplitIndexPath returned false")
	}
	if !reflect.DeepEqual(parent, base) {
		t.Fatalf("parent = %#v, want %#v", parent, base)
	}
	if key == nil || !typ.TypeEquals(key, typ.LiteralString("id")) {
		t.Fatalf("key = %v, want literal string id", key)
	}

	intParent, intKey, ok := SplitIndexPath(Path{Root: "items"}.IndexInt(3))
	if !ok {
		t.Fatal("SplitIndexPath int index returned false")
	}
	if intParent.Root != "items" || len(intParent.Segments) != 0 {
		t.Fatalf("int parent = %#v, want root-only items", intParent)
	}
	if intKey == nil || !typ.TypeEquals(intKey, typ.LiteralInt(3)) {
		t.Fatalf("int key = %v, want literal int 3", intKey)
	}
}

func TestSplitIndexPathRejectsNonIndexPaths(t *testing.T) {
	tests := []Path{
		{},
		{Root: "value"},
		Path{Root: "value"}.Field("field"),
	}

	for _, path := range tests {
		if parent, key, ok := SplitIndexPath(path); ok {
			t.Fatalf("SplitIndexPath(%#v) = (%#v, %v, true), want rejected", path, parent, key)
		}
	}
}
