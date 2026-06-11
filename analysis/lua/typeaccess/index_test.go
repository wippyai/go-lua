package typeaccess

import (
	"testing"

	typetable "github.com/wippyai/go-lua/analysis/type/table"
	"github.com/wippyai/go-lua/analysis/type/typ"
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

func TestIndexRecordMapComponentAndOpenRecord(t *testing.T) {
	rec := typetable.NewRecord().
		MapComponent(typ.String, typ.Boolean).
		Build()

	got, ok := Index(rec, typ.LiteralString("dynamic"))
	if !ok {
		t.Fatal("Index(record map component, string key) failed")
	}
	assertType(t, got, typ.NewOptional(typ.Boolean))

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
	assertType(t, got, typ.NewOptional(typ.Number))

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
	assertType(t, got, typ.NewOptional(typ.Number))

	if _, ok := Index(rec, typ.String); ok {
		t.Fatal("Index(record literal map component, broad string key) succeeded")
	}
	if _, ok := Index(rec, typ.NewUnion(typ.LiteralString("raw"), typ.LiteralString("other"))); ok {
		t.Fatal("Index(record literal map component, partially admitted union key) succeeded")
	}
}

func TestIndexMapReadonlyMapCompatibleKeys(t *testing.T) {
	m := typetable.NewMap(typ.String, typ.Number)

	got, ok := Index(m, typ.LiteralString("name"))
	if !ok {
		t.Fatal("Index(map, literal string key) failed")
	}
	assertType(t, got, typ.NewOptional(typ.Number))

	got, ok = Index(m, typ.String)
	if !ok {
		t.Fatal("Index(map, string key) failed")
	}
	assertType(t, got, typ.NewOptional(typ.Number))

	ro := typetable.NewReadonlyMap(typ.Integer, typ.Boolean)
	got, ok = Index(ro, typ.LiteralInt(1))
	if !ok {
		t.Fatal("Index(readonly map, integer literal key) failed")
	}
	assertType(t, got, typ.NewOptional(typ.Boolean))
}

func TestIndexArrayAndTupleIntegerReads(t *testing.T) {
	arr := typ.NewArray(typ.String)

	got, ok := Index(arr, typ.Integer)
	if !ok {
		t.Fatal("Index(array, integer key) failed")
	}
	assertType(t, got, typ.NewOptional(typ.String))

	tup := typ.NewTuple(typ.Number, typ.Boolean)
	got, ok = Index(tup, typ.LiteralInt(2))
	if !ok {
		t.Fatal("Index(tuple, literal integer key) failed")
	}
	assertType(t, got, typ.Boolean)

	if _, ok := Index(tup, typ.LiteralInt(3)); ok {
		t.Fatal("Index(tuple, out-of-range literal key) succeeded")
	}
}

func TestIndexUnionDistributionAndRejection(t *testing.T) {
	left := typetable.NewRecord().Field("id", typ.String).Build()
	right := typetable.NewRecord().Field("id", typ.Number).Build()

	got, ok := Index(typ.NewUnion(left, right), typ.LiteralString("id"))
	if !ok {
		t.Fatal("Index(union, common key) failed")
	}
	assertType(t, got, typ.NewUnion(typ.String, typ.Number))

	missing := typetable.NewRecord().Field("other", typ.Boolean).Build()
	got, ok = Index(typ.NewUnion(left, missing), typ.LiteralString("id"))
	if !ok {
		t.Fatal("Index(union, missing table member key) failed")
	}
	assertType(t, got, typ.NewOptional(typ.String))

	if _, ok := Index(typ.NewUnion(left, typ.String), typ.LiteralString("id")); ok {
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

	got, ok = RuntimeIndex(rec, typ.NewUnion(typ.LiteralString("name"), typ.LiteralString("missing")))
	if !ok {
		t.Fatal("RuntimeIndex(record, union present-or-missing key) failed")
	}
	assertType(t, got, typ.NewOptional(typ.String))

	got, ok = RuntimeIndex(typetable.NewMap(typ.String, typ.Number), typ.LiteralString("dynamic"))
	if !ok {
		t.Fatal("RuntimeIndex(map, string key) failed")
	}
	assertType(t, got, typ.NewOptional(typ.Number))

	if _, ok := RuntimeIndex(typ.String, typ.LiteralString("field")); ok {
		t.Fatal("RuntimeIndex(non-table, key) succeeded")
	}
}

func TestRuntimeIndexUnionMissingSlot(t *testing.T) {
	present := typetable.NewRecord().Field("id", typ.String).Build()
	missing := typetable.NewRecord().Field("other", typ.Number).Build()

	got, ok := RuntimeIndex(typ.NewUnion(present, missing), typ.LiteralString("id"))
	if !ok {
		t.Fatal("RuntimeIndex(union, missing table member key) failed")
	}
	assertType(t, got, typ.NewOptional(typ.String))
}

func TestIndexOptionalAliasInstantiatedContainer(t *testing.T) {
	t.Run("optional", func(t *testing.T) {
		rec := typetable.NewRecord().Field("value", typ.String).Build()

		got, ok := Index(typ.NewOptional(rec), typ.LiteralString("value"))
		if !ok {
			t.Fatal("Index(optional record, value) failed")
		}
		assertType(t, got, typ.NewOptional(typ.String))
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
}
