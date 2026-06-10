package coalesce

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/type/typ"
)

func TestJoinClosedCompatibleRecordSetMergesMissingFieldsAndStaticMembers(t *testing.T) {
	left := typ.NewRecord().
		Field("name", typ.String).
		StaticStringIndex("raw-key", typ.Number).
		Build()
	right := typ.NewRecord().
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

func TestCoalesceClosedCompatibleRecordsPreservesDiscriminatedUnion(t *testing.T) {
	a := typ.NewRecord().
		Field("kind", typ.LiteralString("a")).
		Field("x", typ.Number).
		Build()
	b := typ.NewRecord().
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
