package value

import (
	"testing"

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

func TestIndexedWriteAdmits_RejectsHeterogeneousRecordDynamicReplacement(t *testing.T) {
	rec := typ.NewRecord().
		Field("count", typ.Integer).
		Field("name", typ.String).
		Build()

	if IndexedWriteAdmits(rec, typ.String, typ.String) {
		t.Fatal("string replacement through dynamic key must not be admitted for heterogeneous closed record")
	}
	if !IndexedWriteAdmits(rec, typ.LiteralString("name"), typ.String) {
		t.Fatal("literal name key should admit string replacement")
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
