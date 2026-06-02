package value

import (
	"testing"

	querycore "github.com/wippyai/go-lua/types/query/core"
	"github.com/wippyai/go-lua/types/typ"
)

func TestAdmitMapArrayElementMutation_RefinesLazySoftArraySlot(t *testing.T) {
	entry := typ.NewRecord().Field("id", typ.String).Build()
	base := typ.NewMap(typ.String, typ.NewUnion(typ.NewArray(typ.Any), typ.NewRecord().Build()))

	got := AdmitMapArrayElementMutation(base, typ.String, entry)
	want := typ.NewMap(typ.String, typ.NewArray(entry))
	if !typ.TypeEquals(got, want) {
		t.Fatalf("AdmitMapArrayElementMutation(lazy slot) = %v, want %v", got, want)
	}
}

func TestAdmitArrayElementMutation_AdmitsAnyTargetAsObservedArray(t *testing.T) {
	entry := typ.NewRecord().Field("id", typ.String).Build()
	got := AdmitArrayElementMutation(typ.Any, entry, typ.JoinPreferNonSoft)
	want := typ.NewArray(entry)
	if !typ.TypeEquals(got, want) {
		t.Fatalf("AdmitArrayElementMutation(any, entry) = %v, want %v", got, want)
	}
}

func TestAdmitContainerElementUnion_WidensGenericFirstTypeArg(t *testing.T) {
	tp := typ.NewTypeParam("T", nil)
	channel := typ.NewGeneric("Channel", []*typ.TypeParam{tp}, typ.NewRecord().Build())

	got := AdmitContainerElementUnion(typ.Instantiate(channel, typ.Unknown), typ.String)
	want := typ.Instantiate(channel, typ.String)
	if !typ.TypeEquals(got, want) {
		t.Fatalf("AdmitContainerElementUnion(Channel<unknown>, string) = %v, want %v", got, want)
	}
}

func TestAdmitContainerElementUnion_WidensArrayAndMapSlots(t *testing.T) {
	if got, want := AdmitContainerElementUnion(typ.NewArray(typ.String), typ.Number), typ.NewArray(typ.NewUnion(typ.String, typ.Number)); !typ.TypeEquals(got, want) {
		t.Fatalf("AdmitContainerElementUnion(string[], number) = %v, want %v", got, want)
	}
	if got, want := AdmitContainerElementUnion(typ.NewMap(typ.String, typ.Unknown), typ.Boolean), typ.NewMap(typ.String, typ.Boolean); !typ.TypeEquals(got, want) {
		t.Fatalf("AdmitContainerElementUnion({[string]: unknown}, boolean) = %v, want %v", got, want)
	}
}

func TestAdmitContainerElementUnion_WidensContainersInsideUnion(t *testing.T) {
	tp := typ.NewTypeParam("T", nil)
	channel := typ.NewGeneric("Channel", []*typ.TypeParam{tp}, typ.NewRecord().Build())
	base := typ.NewUnion(typ.Nil, typ.Instantiate(channel, typ.Number), typ.NewArray(typ.String))
	want := typ.NewUnion(typ.Nil, typ.Instantiate(channel, typ.NewUnion(typ.Number, typ.Boolean)), typ.NewArray(typ.NewUnion(typ.String, typ.Boolean)))

	got := AdmitContainerElementUnion(base, typ.Boolean)
	if !typ.TypeEquals(got, want) {
		t.Fatalf("AdmitContainerElementUnion(union, boolean) = %v, want %v", got, want)
	}
}

func TestMergeForConvergence_PreservesDynamicArrayElementRefinement(t *testing.T) {
	entry := typ.NewRecord().Field("id", typ.String).Build()
	base := typ.NewArray(typ.Any)
	admitted := AdmitArrayElementMutation(base, entry, typ.JoinPreferNonSoft)

	got := MergeForConvergence(base, admitted)
	want := typ.NewArray(entry)
	if !typ.TypeEquals(got, want) {
		t.Fatalf("MergeForConvergence(any[], admitted) = %v, want %v", got, want)
	}
}

func TestAdmitIndexedValueMutation_MergesRecordSlotShape(t *testing.T) {
	baseValue := typ.NewRecord().
		Field("created_at", typ.String).
		Field("pid", typ.String).
		Build()
	updateValue := typ.NewRecord().
		Field("last_activity", typ.String).
		Build()

	got := AdmitIndexedValueMutation(typ.NewMap(typ.String, baseValue), typ.String, updateValue)
	wantValue := typ.NewRecord().
		OptField("created_at", typ.String).
		OptField("last_activity", typ.String).
		OptField("pid", typ.String).
		Build()
	want := typ.NewMap(typ.String, wantValue)
	if !typ.TypeEquals(got, want) {
		t.Fatalf("AdmitIndexedValueMutation() = %v, want %v", got, want)
	}
}

func TestIndexedWriteAdmits_AdmitsMutableHeterogeneousRecordByWeakening(t *testing.T) {
	rec := typ.NewRecord().
		Field("count", typ.Integer).
		Field("name", typ.String).
		Build()

	if !IndexedWriteAdmits(rec, typ.String, typ.String) {
		t.Fatal("mutable dynamic replacement should be admitted by weakening reachable fields")
	}
	if !IndexedWriteAdmits(rec, typ.LiteralString("name"), typ.String) {
		t.Fatal("literal name key should admit string replacement")
	}
}

func TestSealedIndexedWriteAdmits_RejectsHeterogeneousDynamicRecordWrite(t *testing.T) {
	rec := typ.NewRecord().
		Field("count", typ.Integer).
		Field("name", typ.String).
		Build()

	if SealedIndexedWriteAdmits(rec, typ.String, typ.Integer) {
		t.Fatal("sealed heterogeneous record must reject integer write through broad string key")
	}
	if !SealedIndexedWriteAdmits(rec, typ.LiteralString("count"), typ.Integer) {
		t.Fatal("sealed record must admit compatible exact-key field write")
	}
}

func TestSealedIndexedWriteObligation_MissingClosedRecordKeyIsNever(t *testing.T) {
	rec := typ.NewRecord().
		Field("count", typ.Integer).
		Field("name", typ.String).
		Build()

	got := SealedIndexedWriteObligation(rec, typ.LiteralString("missing"))
	if !typ.IsNever(got) {
		t.Fatalf("missing sealed key obligation = %v, want never", got)
	}
	if SealedIndexedWriteAdmits(rec, typ.LiteralString("missing"), typ.Integer) {
		t.Fatal("sealed closed record must reject writes to absent exact keys")
	}
}

func TestIndexedWriteAdmits_AdmitsExistingMapSlotWithoutShapeChange(t *testing.T) {
	if !IndexedWriteAdmits(typ.NewMap(typ.String, typ.Number), typ.String, typ.Number) {
		t.Fatal("map write matching existing key/value domain should be admitted")
	}
}

func TestAdmitIndexedWrite_ExactStringKeyInstallsStaticMember(t *testing.T) {
	got := AdmitIndexedWrite(typ.NewMap(typ.String, typ.Number), typ.LiteralString("foo"), typ.Number)
	rec, ok := got.(*typ.Record)
	if !ok {
		t.Fatalf("AdmitIndexedWrite() = %T %v, want record with static member", got, got)
	}
	member := rec.GetStaticStringIndex("foo")
	if member == nil || !typ.TypeEquals(member.Type, typ.Number) {
		t.Fatalf("static foo member = %#v, want number", member)
	}
	if !rec.HasMapComponent() || !typ.TypeEquals(rec.MapKey, typ.String) || !typ.TypeEquals(rec.MapValue, typ.Number) {
		t.Fatalf("map tail = [%v]%v, want [string]number", rec.MapKey, rec.MapValue)
	}
}

func TestAdmitForeignIndexedWrite_ExactKeyWeakensDotFieldAndInstallsStaticMember(t *testing.T) {
	base := typ.NewRecord().Field("foo", typ.String).Build()

	got := AdmitForeignIndexedWrite(base, typ.LiteralString("foo"), typ.Number)
	rec, ok := got.(*typ.Record)
	if !ok {
		t.Fatalf("AdmitForeignIndexedWrite() = %T %v, want record", got, got)
	}
	field := rec.GetField("foo")
	if field == nil || !typ.TypeEquals(field.Type, typ.NewUnion(typ.String, typ.Number)) {
		t.Fatalf("dot field foo = %#v, want string|number", field)
	}
	member := rec.GetStaticStringIndex("foo")
	if member == nil || !typ.TypeEquals(member.Type, typ.Number) {
		t.Fatalf("static foo member = %#v, want number", member)
	}
}

func TestAdmitForeignIndexedWrite_FreshEmptyRecordDoesNotInferIteratorTailFromOneWrite(t *testing.T) {
	payload := typ.NewRecord().
		Field("created_at", typ.String).
		Field("last_activity", typ.NewOptional(typ.String)).
		Build()

	got := AdmitForeignIndexedWrite(typ.NewFreshEmptyRecord(), typ.LiteralString("s1"), payload)
	rec, ok := got.(*typ.Record)
	if !ok {
		t.Fatalf("AdmitForeignIndexedWrite(fresh{}, \"s1\", payload) = %T %v, want record", got, got)
	}
	member := rec.GetStaticStringIndex("s1")
	if member == nil || !typ.TypeEquals(member.Type, payload) {
		t.Fatalf("static [\"s1\"] member = %#v, want exact payload", member)
	}
	iter := querycore.EntryValueType(got)
	if !typ.TypeEquals(iter, typ.Any) {
		t.Fatalf("EntryValueType(after fresh foreign write) = %v, want any", iter)
	}
}

func TestAdmitForeignIndexedWrite_FreshEmptyRecordDynamicKeyLearnsIteratorTail(t *testing.T) {
	payload := typ.NewRecord().
		Field("created_at", typ.Number).
		Field("last_activity", typ.Number).
		Build()

	got := AdmitForeignIndexedWrite(typ.NewFreshEmptyRecord(), typ.String, payload)
	iter := querycore.EntryValueType(got)
	if !typ.TypeEquals(iter, payload) {
		t.Fatalf("EntryValueType(after fresh dynamic write) = %v, want %v; got=%v", iter, payload, got)
	}
}

func TestAdmitForeignIndexedWrite_OrdinaryEmptyRecordStillLearnsIteratorTail(t *testing.T) {
	payload := typ.NewRecord().Field("id", typ.String).Build()

	got := AdmitForeignIndexedWrite(typ.NewRecord().Build(), typ.LiteralString("s1"), payload)
	iter := querycore.EntryValueType(got)
	if !typ.TypeEquals(iter, payload) {
		t.Fatalf("EntryValueType(after ordinary empty record write) = %v, want %v", iter, payload)
	}
}

func TestIndexedValueMutationAdmits_AllowsRecordElementPatch(t *testing.T) {
	elem := typ.NewRecord().
		Field("created_at", typ.String).
		Field("pid", typ.String).
		Build()
	patch := typ.NewRecord().
		Field("last_activity", typ.String).
		Build()

	if !IndexedValueMutationAdmits(typ.NewMap(typ.String, elem), typ.String, patch) {
		t.Fatal("record map element should admit structural field patch")
	}
	if IndexedValueMutationAdmits(typ.NewMap(typ.String, typ.String), typ.String, patch) {
		t.Fatal("primitive map element must not admit structural field patch")
	}
}

func TestMergeForConvergence_PreservesLazySoftArraySlotRefinement(t *testing.T) {
	entry := typ.NewRecord().Field("id", typ.String).Build()
	base := typ.NewMap(typ.String, typ.NewUnion(typ.NewArray(typ.Any), typ.NewRecord().Build()))
	admitted := AdmitMapArrayElementMutation(base, typ.String, entry)

	got := MergeForConvergence(base, admitted)
	want := typ.NewMap(typ.String, typ.NewArray(entry))
	if !typ.TypeEquals(got, want) {
		t.Fatalf("MergeForConvergence(lazy slot, admitted) = %v, want %v", got, want)
	}
}

func TestMergeForConvergence_PreservesMapArrayUnknownSlotRefinement(t *testing.T) {
	entry := typ.NewRecord().
		Field("id", typ.String).
		Field("meta", typ.NewUnion(typ.NewRecord().Field("suite", typ.NewOptional(typ.String)).Build(), typ.False)).
		Build()
	base := typ.NewMap(typ.String, typ.NewArray(typ.Unknown))
	admitted := AdmitMapArrayElementMutation(base, typ.String, entry)

	got := MergeForConvergence(base, admitted)
	want := typ.NewMap(typ.String, typ.NewArray(entry))
	if !typ.TypeEquals(got, want) {
		t.Fatalf("MergeForConvergence(map unknown[], admitted) = %v, want %v", got, want)
	}
}
