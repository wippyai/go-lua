package coalesce

import (
	"testing"

	typetable "github.com/wippyai/go-lua/analysis/type/table"
	"github.com/wippyai/go-lua/analysis/type/typ"
)

func TestJoinClosedCompatibleRecordSetMergesMissingFieldsAndStaticMembers(t *testing.T) {
	left := typetable.NewRecord().
		Field("name", typ.String).
		StaticStringIndex("raw-key", typ.Number).
		Build()
	right := typetable.NewRecord().
		Field("name", typ.String).
		Field("count", typ.Integer).
		Build()

	got, ok := JoinClosedCompatibleRecordSet([]*typ.Record{left, right}, RecordPolicy{})
	if !ok {
		t.Fatal("JoinClosedCompatibleRecordSet ok=false")
	}
	rec, ok := got.(*typ.Record)
	if !ok {
		t.Fatalf("JoinClosedCompatibleRecordSet = %T %[1]v, want record", got)
	}
	name := rec.GetField("name")
	if name == nil || name.Optional || !typ.TypeEquals(name.Type, typ.String) {
		t.Fatalf("name field = %#v, want required string", name)
	}
	count := rec.GetField("count")
	if count == nil || !count.Optional || !typ.TypeEquals(count.Type, typ.Integer) {
		t.Fatalf("count field = %#v, want optional integer", count)
	}
	member := rec.GetStaticStringIndex("raw-key")
	if member == nil || !member.Optional || !typ.TypeEquals(member.Type, typ.Number) {
		t.Fatalf("static raw-key = %#v, want optional number", member)
	}
}

func TestJoinClosedCompatibleRecordSetUsesInjectedSlotJoinForSharedField(t *testing.T) {
	left := typetable.NewRecord().Field("value", typ.Number).Build()
	right := typetable.NewRecord().Field("value", typ.String).Build()
	sentinel := typ.NewArray(typ.Boolean)
	called := false

	got, ok := JoinClosedCompatibleRecordSet([]*typ.Record{left, right}, RecordPolicy{
		SlotJoin: func(a, b typ.Type) typ.Type {
			called = true
			if !typ.TypeEquals(a, typ.Number) || !typ.TypeEquals(b, typ.String) {
				t.Fatalf("slot join inputs = (%v, %v), want number and string", a, b)
			}
			return sentinel
		},
	})
	if !ok {
		t.Fatal("JoinClosedCompatibleRecordSet ok=false")
	}
	if !called {
		t.Fatal("slot join callback was not called")
	}
	rec, ok := got.(*typ.Record)
	if !ok {
		t.Fatalf("JoinClosedCompatibleRecordSet = %T %[1]v, want record", got)
	}
	field := rec.GetField("value")
	if field == nil || !typ.TypeEquals(field.Type, sentinel) {
		t.Fatalf("merged value field = %v, want sentinel %v", field, sentinel)
	}
}

func TestCoalesceClosedCompatibleRecordsPreservesDiscriminatedUnion(t *testing.T) {
	a := typetable.NewRecord().
		Field("kind", typ.LiteralString("a")).
		Field("x", typ.Number).
		Build()
	b := typetable.NewRecord().
		Field("kind", typ.LiteralString("b")).
		Field("y", typ.String).
		Build()

	got, ok, complete := CoalesceClosedCompatibleRecords([]typ.Type{a, b}, RecordPolicy{})
	if !ok || !complete {
		t.Fatalf("CoalesceClosedCompatibleRecords = ok %v complete %v, want true true", ok, complete)
	}
	if len(got) != 2 || got[0] != a || got[1] != b {
		t.Fatalf("discriminated variants changed: %v", got)
	}
}

func TestJoinFieldContainerSlotUsesInjectedSlotAndKeyJoins(t *testing.T) {
	left := typ.NewMap(typ.String, typ.Number)
	right := typ.NewMap(typ.Integer, typ.Boolean)
	slotCalls := 0
	keyCalls := 0

	got, ok := JoinFieldContainerSlot(left, right, RecordPolicy{
		SlotJoin: func(a, b typ.Type) typ.Type {
			slotCalls++
			if !typ.TypeEquals(a, typ.Number) || !typ.TypeEquals(b, typ.Boolean) {
				t.Fatalf("slot join inputs = (%v, %v), want number and boolean", a, b)
			}
			return typ.Boolean
		},
		KeyJoin: func(a, b typ.Type) typ.Type {
			keyCalls++
			if !typ.TypeEquals(a, typ.String) || !typ.TypeEquals(b, typ.Integer) {
				t.Fatalf("key join inputs = (%v, %v), want string and integer", a, b)
			}
			return typ.String
		},
	})
	if !ok {
		t.Fatal("JoinFieldContainerSlot ok=false")
	}
	if slotCalls != 1 || keyCalls != 1 {
		t.Fatalf("slot/key calls = %d/%d, want 1/1", slotCalls, keyCalls)
	}
	m, ok := got.(*typ.Map)
	if !ok {
		t.Fatalf("JoinFieldContainerSlot = %T %[1]v, want map", got)
	}
	if !typ.TypeEquals(m.Key, typ.String) || !typ.TypeEquals(m.Value, typ.Boolean) {
		t.Fatalf("joined map = %v, want {[string]: boolean}", m)
	}
}

func TestJoinFieldContainerSlotJoinsArrayElementsPointwise(t *testing.T) {
	left := typ.NewArray(typetable.NewRecord().
		Field("name", typ.LiteralString("a")).
		Field("line", typ.LiteralInt(1)).
		Build())
	right := typ.NewArray(typetable.NewRecord().
		Field("name", typ.LiteralString("b")).
		Field("line", typ.LiteralInt(2)).
		Build())

	slotJoin := func(a, b typ.Type) typ.Type {
		ar, aOK := a.(*typ.Record)
		br, bOK := b.(*typ.Record)
		if aOK && bOK {
			joined, ok := JoinClosedCompatibleRecordSet([]*typ.Record{ar, br}, RecordPolicy{})
			if ok {
				return joined
			}
		}
		return typ.NewUnion(a, b)
	}

	got, ok := JoinFieldContainerSlot(left, right, RecordPolicy{SlotJoin: slotJoin})
	if !ok {
		t.Fatal("JoinFieldContainerSlot ok=false")
	}
	arr, ok := got.(*typ.Array)
	if !ok {
		t.Fatalf("JoinFieldContainerSlot = %T %[1]v, want array", got)
	}
	elem, ok := arr.Element.(*typ.Record)
	if !ok {
		t.Fatalf("joined array element = %T %[1]v, want record", arr.Element)
	}
	if name := elem.GetField("name"); name == nil || !typ.TypeEquals(name.Type, typ.String) {
		t.Fatalf("joined name field = %v, want string", name)
	}
	if line := elem.GetField("line"); line == nil || !typ.TypeEquals(line.Type, typ.Integer) {
		t.Fatalf("joined line field = %v, want integer", line)
	}
}

func TestCompatibleRecordMetatablesRequireMatchingPresenceAndShape(t *testing.T) {
	meta := typetable.NewRecord().
		Field("__index", typetable.NewRecord().Field("run", typ.Func().Build()).Build()).
		Build()
	withMeta := typetable.NewRecord().Field("id", typ.String).Metatable(meta).Build()
	withSameMeta := typetable.NewRecord().Field("name", typ.String).Metatable(meta).Build()
	withoutMeta := typetable.NewRecord().Field("id", typ.String).Build()
	otherMeta := typetable.NewRecord().
		Field("__index", typetable.NewRecord().Field("stop", typ.Func().Build()).Build()).
		Build()
	withOtherMeta := typetable.NewRecord().Field("id", typ.String).Metatable(otherMeta).Build()

	if !CompatibleRecordMetatables(withMeta, withSameMeta) {
		t.Fatal("records sharing the same metatable should be compatible")
	}
	if CompatibleRecordMetatables(withMeta, withoutMeta) {
		t.Fatal("record with metatable should not merge with record without metatable")
	}
	if CompatibleRecordMetatables(withMeta, withOtherMeta) {
		t.Fatal("records with different metatables should not be compatible")
	}
}
