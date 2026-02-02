package core

import (
	"testing"

	"github.com/wippyai/go-lua/types/typ"
)

func TestIndex(t *testing.T) {
	arr := typ.NewArray(typ.String)
	m := typ.NewMap(typ.String, typ.Number)
	tuple := typ.NewTuple(typ.String, typ.Integer, typ.Boolean)
	rec := typ.NewRecord().
		Field("a", typ.String).
		Field("b", typ.Integer).
		Build()

	tests := []struct {
		name    string
		t       typ.Type
		keyType typ.Type
		found   bool
		checker func(typ.Type) bool
	}{
		{"nil type", nil, typ.Integer, false, nil},
		{"array with integer key", arr, typ.Integer, true, func(t typ.Type) bool { return t == typ.String }},
		{"array with number key", arr, typ.Number, true, func(t typ.Type) bool { return t == typ.String }},
		{"array with string key", arr, typ.String, false, nil},
		{"map with matching key", m, typ.String, true, func(t typ.Type) bool {
			_, ok := t.(*typ.Optional)
			return ok
		}},
		{"map with wrong key type", m, typ.Integer, false, nil},
		{"map with any key", typ.NewMap(typ.Any, typ.Boolean), typ.String, true, func(t typ.Type) bool {
			_, ok := t.(*typ.Optional)
			return ok
		}},
		{"tuple with literal index", tuple, typ.LiteralInt(1), true, func(t typ.Type) bool { return t == typ.String }},
		{"tuple with literal index 2", tuple, typ.LiteralInt(2), true, func(t typ.Type) bool { return t == typ.Integer }},
		{"tuple with out of bounds index", tuple, typ.LiteralInt(4), true, func(t typ.Type) bool { return t == typ.Nil }},
		{"tuple with zero index", tuple, typ.LiteralInt(0), true, func(t typ.Type) bool { return t == typ.Nil }},
		{"tuple with negative index", tuple, typ.LiteralInt(-1), true, func(t typ.Type) bool { return t == typ.Nil }},
		{"tuple with generic integer", tuple, typ.Integer, true, func(t typ.Type) bool {
			return ContainsNil(t)
		}},
		{"empty tuple with integer", typ.NewTuple(), typ.Integer, false, nil},
		{"record with string literal key", rec, typ.LiteralString("a"), true, func(t typ.Type) bool { return t == typ.String }},
		{"record with generic string key", rec, typ.String, true, func(t typ.Type) bool {
			return ContainsNil(t)
		}},
		{"empty record with string", typ.NewRecord().Build(), typ.String, true, func(t typ.Type) bool { return t == typ.Nil }},
		{"any type", typ.Any, typ.String, true, func(t typ.Type) bool { return t == typ.Any }},
		{"never type", typ.Never, typ.String, true, func(t typ.Type) bool { return t == typ.Never }},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, ok := Index(tt.t, tt.keyType)
			if ok != tt.found {
				t.Errorf("expected found=%v, got %v", tt.found, ok)
			}

			if tt.found && tt.checker != nil && !tt.checker(result) {
				t.Errorf("checker failed for result %v", result)
			}
		})
	}
}

func TestIndexUnion(t *testing.T) {
	arr1 := typ.NewArray(typ.String)
	arr2 := typ.NewArray(typ.Integer)
	union := typ.NewUnion(arr1, arr2)

	t.Run("union of arrays", func(t *testing.T) {
		result, ok := Index(union, typ.Integer)
		if !ok {
			t.Error("expected to index union of arrays")
		}

		if result == nil {
			t.Error("expected non-nil result")
		}
	})

	t.Run("empty union", func(t *testing.T) {
		emptyUnion := &typ.Union{Members: []typ.Type{}}

		_, ok := Index(emptyUnion, typ.Integer)
		if ok {
			t.Error("expected not to index empty union")
		}
	})

	t.Run("union with non-indexable member", func(t *testing.T) {
		mixedUnion := typ.NewUnion(arr1, typ.String)

		_, ok := Index(mixedUnion, typ.Integer)
		if ok {
			t.Error("expected not to index union with non-indexable member")
		}
	})
}

func TestIndexIntersection(t *testing.T) {
	arr := typ.NewArray(typ.String)
	rec := typ.NewRecord().Field("x", typ.Number).Build()
	inter := typ.NewIntersection(arr, rec)

	t.Run("intersection with integer key", func(t *testing.T) {
		result, ok := Index(inter, typ.Integer)
		if !ok {
			t.Error("expected to index intersection with integer key")
		}

		if result == nil {
			t.Error("expected non-nil result")
		}
	})

	t.Run("empty intersection", func(t *testing.T) {
		emptyInter := &typ.Intersection{Members: []typ.Type{}}

		_, ok := Index(emptyInter, typ.Integer)
		if ok {
			t.Error("expected not to index empty intersection")
		}
	})
}

func TestIndexOptional(t *testing.T) {
	arr := typ.NewArray(typ.String)
	opt := typ.NewOptional(arr)

	t.Run("optional array", func(t *testing.T) {
		result, ok := Index(opt, typ.Integer)
		if !ok {
			t.Error("expected to index optional array")
		}

		if _, isOpt := result.(*typ.Optional); !isOpt {
			t.Errorf("expected optional result type, got %v", result)
		}
	})
}

func TestIndexAlias(t *testing.T) {
	arr := typ.NewArray(typ.Number)
	alias := typ.NewAlias("Numbers", arr)

	t.Run("alias to array", func(t *testing.T) {
		result, ok := Index(alias, typ.Integer)
		if !ok {
			t.Error("expected to index alias")
		}

		if result != typ.Number {
			t.Errorf("expected number, got %v", result)
		}
	})
}

func TestIsNumericKey(t *testing.T) {
	tests := []struct {
		name   string
		key    typ.Type
		expect bool
	}{
		{"integer", typ.Integer, true},
		{"number", typ.Number, true},
		{"string", typ.String, false},
		{"boolean", typ.Boolean, false},
		{"literal integer", typ.LiteralInt(5), true},
		{"literal number", typ.LiteralNumber(3.14), true},
		{"literal string", typ.LiteralString("x"), false},
		{"nil", typ.Nil, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if isNumeric(tt.key) != tt.expect {
				t.Errorf("expected %v", tt.expect)
			}
		})
	}
}
