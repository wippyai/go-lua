package flow

import (
	"testing"

	"github.com/wippyai/go-lua/types/domain/value/product"
	"github.com/wippyai/go-lua/types/typ"
)

func TestSetReturnRelations(t *testing.T) {
	rel := ReturnCorrelation{ValueIndex: 0, ErrorIndex: 1}
	rels := ReturnRelationsOfErrorReturns([]ReturnCorrelation{rel})
	ps := PointState{}

	if !SetReturnRelations(&ps, rels) {
		t.Fatal("SetReturnRelations reported unchanged for new relations")
	}
	if !ps.ReturnRel.HasErrorReturn(rel) {
		t.Fatalf("ReturnRel = %#v, want %#v", ps.ReturnRel.ErrorReturns(), rel)
	}
	if SetReturnRelations(&ps, rels) {
		t.Fatal("SetReturnRelations reported changed for equal relations")
	}
}

func TestReturnSlotValueAccessors(t *testing.T) {
	value := product.FromType(typ.String)
	ps := PointState{}

	if !WriteReturnSlotValue(&ps, 0, value) {
		t.Fatal("WriteReturnSlotValue reported unchanged for new slot")
	}
	if got, ok := PointFactsOf(ps).ValueKeyValue(ReturnSlotValueKey(0)); !ok || !product.Domain.Equal(got, value) {
		t.Fatalf("return slot value = %v/%v, want %v/true", got.ProjectValue(), ok, value.ProjectValue())
	}
	if WriteReturnSlotValue(&ps, 0, value) {
		t.Fatal("WriteReturnSlotValue reported changed for equal slot")
	}
	if !ClearReturnSlotValue(&ps, 0) {
		t.Fatal("ClearReturnSlotValue reported unchanged for existing slot")
	}
	if _, ok := ps.Env[ReturnSlotValueKey(0)]; ok {
		t.Fatalf("ClearReturnSlotValue left Env[%s]", ReturnSlotValueKey(0))
	}
	if ClearReturnSlotValue(&ps, 0) {
		t.Fatal("ClearReturnSlotValue reported changed for absent slot")
	}
}
