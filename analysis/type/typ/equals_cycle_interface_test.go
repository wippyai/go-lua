package typ

import "testing"

// TestEqualsRecursiveThroughInterfaceTerminates proves the coinductive cycle
// guard holds when a recursive type cycles through an interface method signature.
// Before the Interface/Meta cases were added to typeEqualsGuard, the seen-set was
// dropped at the Interface.Equals -> Function.Equals -> typeEquals(nil seen)
// boundary, so this comparison recursed forever (stack overflow).
func TestEqualsRecursiveThroughInterfaceTerminates(t *testing.T) {
	build := func() Type {
		return NewRecursive("Node", func(self Type) Type {
			return NewInterface("Node", []Method{
				{Name: "next", Type: Func().Returns(self).Build()},
			})
		})
	}
	a, b := build(), build()
	if !typeEquals(a, b) {
		t.Fatalf("structurally identical recursive-through-interface types must be equal")
	}
	// A differing method name must compare not-equal (and still terminate).
	c := NewRecursive("Node", func(self Type) Type {
		return NewInterface("Node", []Method{
			{Name: "prev", Type: Func().Returns(self).Build()},
		})
	})
	if typeEquals(a, c) {
		t.Fatalf("recursive-through-interface types with different methods must differ")
	}
}

// TestEqualsRecursiveThroughMetaTerminates is the Meta analogue.
func TestEqualsRecursiveThroughMetaTerminates(t *testing.T) {
	build := func() Type {
		return NewRecursive("M", func(self Type) Type {
			return RebuildRecord(RecordParts{Fields: []Field{
				{Name: "meta", Type: NewMeta(self)},
			}})
		})
	}
	a, b := build(), build()
	if !typeEquals(a, b) {
		t.Fatalf("structurally identical recursive-through-meta types must be equal")
	}
}

// TestEqualsRecursiveThroughEveryCompoundKindTerminates is a drift guard: for
// each structural carrier that can host a recursion, a recursive type cycling
// through it must compare equal and terminate. A new compound kind added without
// an explicit seen-threading case in typeEqualsGuard would fall to the default
// arm, drop the coinductive guard, and stack-overflow here.
func TestEqualsRecursiveThroughEveryCompoundKindTerminates(t *testing.T) {
	carriers := map[string]func(self Type) Type{
		"record":      func(self Type) Type { return RebuildRecord(RecordParts{Fields: []Field{{Name: "n", Type: self}}}) },
		"array":       func(self Type) Type { return NewArray(self) },
		"map":         func(self Type) Type { return NewMap(String, self) },
		"readonlymap": func(self Type) Type { return NewReadonlyMap(String, self) },
		"optional":    func(self Type) Type { return MaterializeOptional(self) },
		"union":       func(self Type) Type { return MaterializeUnion([]Type{Number, self}) },
		"tuple":       func(self Type) Type { return NewTuple(Number, self) },
		"function":    func(self Type) Type { return Func().Returns(self).Build() },
		"interface": func(self Type) Type {
			return NewInterface("I", []Method{{Name: "m", Type: Func().Returns(self).Build()}})
		},
		"meta": func(self Type) Type {
			return RebuildRecord(RecordParts{Fields: []Field{{Name: "m", Type: NewMeta(self)}}})
		},
		"intersection": func(self Type) Type {
			return MaterializeIntersection([]Type{
				NewInterface("A", []Method{{Name: "a", Type: Func().Returns(self).Build()}}),
				NewInterface("B", []Method{{Name: "b", Type: Func().Build()}}),
			})
		},
	}
	for name, wrap := range carriers {
		build := func() Type { return NewRecursive("R", wrap) }
		a, b := build(), build()
		if !typeEquals(a, b) {
			t.Fatalf("%s: identical recursive-through-%s types must be equal", name, name)
		}
	}
}
