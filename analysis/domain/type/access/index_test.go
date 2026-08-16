package access

import (
	"testing"

	typetable "github.com/wippyai/go-lua/analysis/domain/type/table"
	"github.com/wippyai/go-lua/analysis/domain/type/typ"
	"github.com/wippyai/go-lua/analysis/domain/type/typeexpr"
)

func TestIndexRecordStringKeys(t *testing.T) {
	rec := typetable.NewRecord().
		Field("name", typ.String).
		StaticStringIndex("raw-key", typ.Number).
		Build()

	got, ok := Index(rec, typ.LiteralString("name"))
	if !ok {
		t.Fatal("Index(record, literal field key) failed")
	}
	assertType(t, got, typ.String)

	got, ok = Index(rec, typ.LiteralString("raw-key"))
	if !ok {
		t.Fatal("Index(record, static string key) failed")
	}
	assertType(t, got, typ.Number)
}

func TestIndexRecordStaticIntMember(t *testing.T) {
	rec := typetable.NewRecord().
		StaticIntIndex(7, typ.Boolean).
		Build()

	got, ok := Index(rec, typ.LiteralInt(7))
	if !ok {
		t.Fatal("Index(record, static int key) failed")
	}
	assertType(t, got, typ.Boolean)
}

func TestRuntimeIndexRecordDynamicIntegerUnionsStaticIntMembers(t *testing.T) {
	rec := typetable.NewRecord().
		StaticIntIndex(1, typ.String).
		StaticIntIndex(2, typ.Number).
		Build()

	got, ok := RuntimeIndex(rec, typ.Integer)
	if !ok {
		t.Fatal("RuntimeIndex(record static int members, integer key) failed")
	}
	assertType(t, got, typeexpr.Optional(typeexpr.Union(typ.String, typ.Number)))

	got, ok = RuntimeIndex(rec, typ.Number)
	if !ok {
		t.Fatal("RuntimeIndex(record static int members, number key) failed")
	}
	assertType(t, got, typeexpr.Optional(typeexpr.Union(typ.String, typ.Number)))
}

func TestIndexRecordMapComponentAndOpenRecord(t *testing.T) {
	rec := typetable.NewRecord().
		MapComponent(typ.String, typ.Boolean).
		Build()

	got, ok := Index(rec, typ.LiteralString("dynamic"))
	if !ok {
		t.Fatal("Index(record map component, string key) failed")
	}
	assertType(t, got, typeexpr.Optional(typ.Boolean))

	open := typetable.NewRecord().SetOpen(true).Build()
	got, ok = Index(open, typ.LiteralString("anything"))
	if !ok {
		t.Fatal("Index(open record, string key) failed")
	}
	assertType(t, got, typ.Unknown)
}

func TestIndexRecordMapComponentRequiresStrictKeyAdmission(t *testing.T) {
	m := typetable.NewMap(typ.LiteralString("raw"), typ.Number)

	got, ok := Index(m, typ.LiteralString("raw"))
	if !ok {
		t.Fatal("Index(literal-key map, exact key) failed")
	}
	assertType(t, got, typeexpr.Optional(typ.Number))

	if _, ok := Index(m, typ.String); ok {
		t.Fatal("Index(literal-key map, broad string key) succeeded")
	}

	rec := typetable.NewRecord().
		MapComponent(typ.LiteralString("raw"), typ.Number).
		Build()

	got, ok = Index(rec, typ.LiteralString("raw"))
	if !ok {
		t.Fatal("Index(record literal map component, exact key) failed")
	}
	assertType(t, got, typeexpr.Optional(typ.Number))

	if _, ok := Index(rec, typ.String); ok {
		t.Fatal("Index(record literal map component, broad string key) succeeded")
	}
	if _, ok := Index(rec, typeexpr.Union(typ.LiteralString("raw"), typ.LiteralString("other"))); ok {
		t.Fatal("Index(record literal map component, partially admitted union key) succeeded")
	}
}

func TestRuntimeIndexMapAllowsOverlappingKey(t *testing.T) {
	m := typetable.NewMap(typ.LiteralString("raw"), typ.Number)

	got, ok := RuntimeIndex(m, typ.String)
	if !ok {
		t.Fatal("RuntimeIndex(literal-key map, broad string key) failed")
	}
	assertType(t, got, typeexpr.Optional(typ.Number))

	anyMap := typetable.NewMap(typ.Any, typ.String)
	got, ok = RuntimeIndex(anyMap, typ.Any)
	if !ok {
		t.Fatal("RuntimeIndex(any-key map, any key) failed")
	}
	assertType(t, got, typeexpr.Optional(typ.String))

	anyRecordMap := typetable.NewRecord().MapComponent(typ.Any, typ.String).Build()
	got, ok = RuntimeIndex(anyRecordMap, typ.Any)
	if !ok {
		t.Fatal("RuntimeIndex(any-key record map, any key) failed")
	}
	assertType(t, got, typeexpr.Optional(typ.String))

	stringMap := typetable.NewMap(typ.String, typ.Number)
	got, ok = RuntimeIndex(stringMap, typeexpr.Optional(typ.String))
	if !ok {
		t.Fatal("RuntimeIndex(string map, optional string key) failed")
	}
	assertType(t, got, typeexpr.Optional(typ.Number))
}

func TestIndexMapReadonlyMapCompatibleKeys(t *testing.T) {
	m := typetable.NewMap(typ.String, typ.Number)

	got, ok := Index(m, typ.LiteralString("name"))
	if !ok {
		t.Fatal("Index(map, literal string key) failed")
	}
	assertType(t, got, typeexpr.Optional(typ.Number))

	got, ok = Index(m, typ.String)
	if !ok {
		t.Fatal("Index(map, string key) failed")
	}
	assertType(t, got, typeexpr.Optional(typ.Number))

	ro := typetable.NewReadonlyMap(typ.Integer, typ.Boolean)
	got, ok = Index(ro, typ.LiteralInt(1))
	if !ok {
		t.Fatal("Index(readonly map, integer literal key) failed")
	}
	assertType(t, got, typeexpr.Optional(typ.Boolean))
}

func TestWritableIndexDropsReadMissOptionality(t *testing.T) {
	m := typetable.NewMap(typ.String, typ.Number)
	got, ok := WritableIndex(m, typ.String)
	if !ok {
		t.Fatal("WritableIndex(map, string key) failed")
	}
	assertType(t, got, typ.Number)

	optionalValue := typetable.NewMap(typ.String, typ.MaterializeOptional(typ.Number))
	got, ok = WritableIndex(optionalValue, typ.String)
	if !ok {
		t.Fatal("WritableIndex(map, optional value) failed")
	}
	assertType(t, got, typeexpr.Optional(typ.Number))

	arr := typ.NewArray(typ.String)
	got, ok = WritableIndex(arr, typ.Integer)
	if !ok {
		t.Fatal("WritableIndex(array, integer key) failed")
	}
	assertType(t, got, typ.String)
}

func TestIndexRecordMapComponentIntegerAdmission(t *testing.T) {
	rec := typetable.NewRecord().
		MapComponent(typ.Integer, typ.Boolean).
		Build()

	got, ok := Index(rec, typ.LiteralInt(7))
	if !ok {
		t.Fatal("Index(record integer map component, literal int key) failed")
	}
	assertType(t, got, typeexpr.Optional(typ.Boolean))

	got, ok = Index(rec, typ.Integer)
	if !ok {
		t.Fatal("Index(record integer map component, integer key) failed")
	}
	assertType(t, got, typeexpr.Optional(typ.Boolean))
}

func TestRuntimeIndexRecordIntegerMapComponentMayOverlapNumericRuntimeKeys(t *testing.T) {
	rec := typetable.NewRecord().
		MapComponent(typ.Integer, typ.Boolean).
		Build()

	got, ok := RuntimeIndex(rec, typ.Number)
	if !ok {
		t.Fatal("RuntimeIndex(record integer map component, broad number key) failed")
	}
	assertType(t, got, typeexpr.Optional(typ.Boolean))

	got, ok = RuntimeIndex(rec, typ.LiteralNumber(2))
	if !ok {
		t.Fatal("RuntimeIndex(record integer map component, integer-valued number literal key) failed")
	}
	assertType(t, got, typeexpr.Optional(typ.Boolean))

	got, ok = RuntimeIndex(rec, typ.LiteralNumber(1.5))
	if !ok {
		t.Fatal("RuntimeIndex(record integer map component, fractional number literal key) failed")
	}
	assertType(t, got, typ.Nil)
}

func TestRuntimeIndexRecordDynamicIntegerUnionsStaticIntMembersAndMapComponent(t *testing.T) {
	rec := typetable.NewRecord().
		StaticIntIndex(1, typ.String).
		MapComponent(typ.Integer, typ.Boolean).
		Build()

	got, ok := RuntimeIndex(rec, typ.Integer)
	if !ok {
		t.Fatal("RuntimeIndex(record static int members plus map component, integer key) failed")
	}
	assertType(t, got, typeexpr.Optional(typeexpr.Union(typ.String, typ.Boolean)))

	got, ok = RuntimeIndex(rec, typ.Number)
	if !ok {
		t.Fatal("RuntimeIndex(record static int members plus map component, broad number key) failed")
	}
	assertType(t, got, typeexpr.Optional(typeexpr.Union(typ.String, typ.Boolean)))
}

func TestIndexArrayAndTupleIntegerReads(t *testing.T) {
	arr := typ.NewArray(typ.String)

	got, ok := Index(arr, typ.Integer)
	if !ok {
		t.Fatal("Index(array, integer key) failed")
	}
	assertType(t, got, typeexpr.Optional(typ.String))

	got, ok = RuntimeIndex(arr, typ.Number)
	if !ok {
		t.Fatal("RuntimeIndex(array, broad number key) failed")
	}
	assertType(t, got, typeexpr.Optional(typ.String))

	got, ok = RuntimeIndex(arr, typ.LiteralNumber(2))
	if !ok {
		t.Fatal("RuntimeIndex(array, integer-valued number literal key) failed")
	}
	assertType(t, got, typeexpr.Optional(typ.String))

	got, ok = RuntimeIndex(arr, typ.LiteralNumber(1.5))
	if !ok {
		t.Fatal("RuntimeIndex(array, non-integer number literal key) failed")
	}
	assertType(t, got, typ.Nil)

	tup := typ.NewTuple(typ.Number, typ.Boolean)
	got, ok = Index(tup, typ.LiteralInt(2))
	if !ok {
		t.Fatal("Index(tuple, literal integer key) failed")
	}
	assertType(t, got, typ.Boolean)

	if _, ok := Index(tup, typ.LiteralInt(3)); ok {
		t.Fatal("Index(tuple, out-of-range literal key) succeeded")
	}

	got, ok = RuntimeIndex(tup, typ.LiteralInt(3))
	if !ok {
		t.Fatal("RuntimeIndex(tuple, out-of-range literal key) failed")
	}
	assertType(t, got, typ.Nil)

	got, ok = RuntimeIndex(tup, typeexpr.Union(typ.LiteralInt(1), typ.LiteralInt(3)))
	if !ok {
		t.Fatal("RuntimeIndex(tuple, union in-range-or-missing key) failed")
	}
	assertType(t, got, typeexpr.Optional(typ.Number))

	got, ok = RuntimeIndex(tup, typ.Integer)
	if !ok {
		t.Fatal("RuntimeIndex(tuple, dynamic integer key) failed")
	}
	assertType(t, got, typeexpr.Optional(typeexpr.Union(typ.Number, typ.Boolean)))

	got, ok = RuntimeIndex(tup, typ.Number)
	if !ok {
		t.Fatal("RuntimeIndex(tuple, broad number key) failed")
	}
	assertType(t, got, typeexpr.Optional(typeexpr.Union(typ.Number, typ.Boolean)))
}

func TestIndexUnionDistributionAndRejection(t *testing.T) {
	left := typetable.NewRecord().Field("id", typ.String).Build()
	right := typetable.NewRecord().Field("id", typ.Number).Build()

	got, ok := Index(typeexpr.Union(left, right), typ.LiteralString("id"))
	if !ok {
		t.Fatal("Index(union, common key) failed")
	}
	assertType(t, got, typeexpr.Union(typ.String, typ.Number))

	missing := typetable.NewRecord().Field("other", typ.Boolean).Build()
	got, ok = Index(typeexpr.Union(left, missing), typ.LiteralString("id"))
	if !ok {
		t.Fatal("Index(union, missing table member key) failed")
	}
	assertType(t, got, typeexpr.Optional(typ.String))

	if _, ok := Index(typeexpr.Union(left, typ.String), typ.LiteralString("id")); ok {
		t.Fatal("Index(union with non-table member) succeeded")
	}
}

func TestRuntimeIndexMissingSlotAndNonTable(t *testing.T) {
	rec := typetable.NewRecord().
		Field("name", typ.String).
		Build()

	got, ok := RuntimeIndex(rec, typ.LiteralString("missing"))
	if !ok {
		t.Fatal("RuntimeIndex(record, missing key) failed")
	}
	assertType(t, got, typ.Nil)

	got, ok = RuntimeIndex(rec, typ.LiteralString("name"))
	if !ok {
		t.Fatal("RuntimeIndex(record, present key) failed")
	}
	assertType(t, got, typ.String)

	got, ok = RuntimeIndex(rec, typeexpr.Union(typ.LiteralString("name"), typ.LiteralString("missing")))
	if !ok {
		t.Fatal("RuntimeIndex(record, union present-or-missing key) failed")
	}
	assertType(t, got, typeexpr.Optional(typ.String))

	got, ok = RuntimeIndex(typetable.NewMap(typ.String, typ.Number), typ.LiteralString("dynamic"))
	if !ok {
		t.Fatal("RuntimeIndex(map, string key) failed")
	}
	assertType(t, got, typeexpr.Optional(typ.Number))

	if _, ok := RuntimeIndex(typ.String, typ.LiteralString("field")); ok {
		t.Fatal("RuntimeIndex(non-table, key) succeeded")
	}
}

func TestRuntimeIndexUnionMissingSlot(t *testing.T) {
	present := typetable.NewRecord().Field("id", typ.String).Build()
	missing := typetable.NewRecord().Field("other", typ.Number).Build()

	got, ok := RuntimeIndex(typeexpr.Union(present, missing), typ.LiteralString("id"))
	if !ok {
		t.Fatal("RuntimeIndex(union, missing table member key) failed")
	}
	assertType(t, got, typeexpr.Optional(typ.String))
}

func TestIndexDeepCompositeProjectionUsesExplicitWorkStack(t *testing.T) {
	const depth = 4097
	leaf := typetable.NewRecord().Field("value", typ.String).Build()
	var value typ.Type = leaf
	for index := 0; index < depth; index++ {
		value = &typ.Union{Members: []typ.Type{value, leaf}}
	}
	got, ok := Index(value, typ.LiteralString("value"))
	if !ok {
		t.Fatal("deep composite index projection failed")
	}
	assertType(t, got, typ.String)
}

func TestIndexRecursiveCompositeCycleUsesExactCoinductiveIdentity(t *testing.T) {
	leaf := typetable.NewRecord().Field("value", typ.String).Build()
	loop := typ.NewRecursivePlaceholder("Loop")
	loop.SetBody(&typ.Union{Members: []typ.Type{loop, leaf}})
	got, ok := Index(loop, typ.LiteralString("value"))
	if !ok {
		t.Fatal("productive recursive index projection failed")
	}
	assertType(t, got, typ.String)

	bad := typ.NewRecursivePlaceholder("Bad")
	bad.SetBody(bad)
	if got, ok := Index(bad, typ.LiteralString("value")); ok || got != nil {
		t.Fatalf("invalid recursive index projection = (%v, %v)", got, ok)
	}
}

func TestIndexOptionalAliasInstantiatedContainer(t *testing.T) {
	t.Run("optional", func(t *testing.T) {
		rec := typetable.NewRecord().Field("value", typ.String).Build()

		got, ok := Index(typeexpr.Optional(rec), typ.LiteralString("value"))
		if !ok {
			t.Fatal("Index(optional record, value) failed")
		}
		assertType(t, got, typeexpr.Optional(typ.String))
	})

	t.Run("alias", func(t *testing.T) {
		rec := typetable.NewRecord().Field("value", typ.Boolean).Build()

		got, ok := Index(typ.NewAlias("Row", rec), typ.LiteralString("value"))
		if !ok {
			t.Fatal("Index(alias record, value) failed")
		}
		assertType(t, got, typ.Boolean)
	})

	t.Run("instantiated", func(t *testing.T) {
		param := typ.NewTypeParam("T", nil)
		box := typ.NewGeneric("Box", []*typ.TypeParam{param},
			typetable.NewRecord().Field("value", param).Build())

		got, ok := Index(typ.Instantiate(box, typ.Number), typ.LiteralString("value"))
		if !ok {
			t.Fatal("Index(Box<number>, value) failed")
		}
		assertType(t, got, typ.Number)
	})

	t.Run("constrained type parameter", func(t *testing.T) {
		constraint := typetable.NewRecord().Field("value", typ.String).Build()
		param := typ.NewTypeParam("T", constraint)

		got, ok := Index(param, typ.LiteralString("value"))
		if !ok {
			t.Fatal("Index(constrained T, value) failed")
		}
		assertType(t, got, typ.String)
	})

	t.Run("unconstrained type parameter", func(t *testing.T) {
		param := typ.NewTypeParam("T", nil)

		if got, ok := Index(param, typ.LiteralString("value")); ok {
			t.Fatalf("Index(unconstrained T, value) = %v/true, want missing", got)
		}
	})
}

func TestIndexWrappedTopTypesPreserveAccessTopSemantics(t *testing.T) {
	got, ok := Index(typ.NewAlias("Dynamic", typ.Any), typ.LiteralString("value"))
	if !ok {
		t.Fatal("Index(alias any, value) failed")
	}
	assertType(t, got, typ.Any)

	got, ok = Index(typ.NewAlias("Opaque", typ.Unknown), typ.LiteralString("value"))
	if !ok {
		t.Fatal("Index(alias unknown, value) failed")
	}
	assertType(t, got, typ.Unknown)
}

func TestRuntimeIndexArrayWrappedDynamicKeyMayBeInteger(t *testing.T) {
	got, ok := RuntimeIndex(typ.NewArray(typ.String), typ.NewAlias("DynamicKey", typ.Any))
	if !ok {
		t.Fatal("RuntimeIndex(array, alias any key) failed")
	}
	assertType(t, got, typeexpr.Optional(typ.String))
}

// TestRuntimeIndexArrayDeepAliasKeyReachesExactLeaf verifies that a finite
// wrapper chain is no longer truncated by a semantic depth budget.
func TestRuntimeIndexArrayDeepAliasKeyReachesExactLeaf(t *testing.T) {
	var key typ.Type = typ.String
	for i := 0; i < 10000; i++ {
		key = &typ.Alias{Name: "K", Target: key}
	}

	got, ok := RuntimeIndex(typ.NewArray(typ.String), key)
	if !ok {
		t.Fatal("RuntimeIndex(array, deep alias key) failed to resolve")
	}
	if !typ.TypeEquals(got, typ.Nil) {
		t.Fatalf("RuntimeIndex(array, deep string alias key) = %v, want nil", got)
	}
}
