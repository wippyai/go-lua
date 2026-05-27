package returnsummary

import (
	"testing"

	"github.com/wippyai/go-lua/types/typ"
)

// A whole-slot bare any is an unverified dynamic outcome that both the slot law
// and the deep law demote to unknown.
func TestDemoteSlot_WholeSlotAnyBecomesUnknown(t *testing.T) {
	in := []typ.Type{typ.Any}

	slot := DemoteInferredDynamicAnySlot(in)
	if len(slot) != 1 || !typ.IsUnknown(slot[0]) {
		t.Fatalf("slot demote = %v, want [unknown]", slot)
	}
	deep := DemoteInferredDynamicAny(in)
	if len(deep) != 1 || !typ.IsUnknown(deep[0]) {
		t.Fatalf("deep demote = %v, want [unknown]", deep)
	}
}

// An any[] array is a proven container contract; the slot law keeps the array
// shape with its any element, while the deep export law rewrites the element.
func TestDemoteSlot_AnyArrayElementStaysAny(t *testing.T) {
	in := []typ.Type{typ.NewArray(typ.Any)}

	slot := DemoteInferredDynamicAnySlot(in)
	arr, ok := slot[0].(*typ.Array)
	if !ok || !typ.IsAny(arr.Element) {
		t.Fatalf("slot demote = %v, want any[]", slot)
	}

	deep := DemoteInferredDynamicAny(in)
	deepArr, ok := deep[0].(*typ.Array)
	if !ok || !typ.IsUnknown(deepArr.Element) {
		t.Fatalf("deep demote = %v, want unknown[]", deep)
	}
}

// A record element field typed any is genuine gradual evidence; the slot law
// preserves it, the deep export law demotes it.
func TestDemoteSlot_RecordAnyFieldStaysAny(t *testing.T) {
	in := []typ.Type{
		typ.NewArray(typ.NewRecord().
			Field("id", typ.String).
			Field("order", typ.Any).
			Build()),
	}

	slot := DemoteInferredDynamicAnySlot(in)
	slotArr, ok := slot[0].(*typ.Array)
	if !ok {
		t.Fatalf("slot demote = %v, want array", slot)
	}
	slotRec, ok := slotArr.Element.(*typ.Record)
	if !ok {
		t.Fatalf("slot element = %v, want record", slotArr.Element)
	}
	if f := slotRec.GetField("order"); f == nil || !typ.IsAny(f.Type) {
		t.Fatalf("slot order field = %v, want any", f)
	}
	if f := slotRec.GetField("id"); f == nil || !typ.TypeEquals(f.Type, typ.String) {
		t.Fatalf("slot id field = %v, want string", f)
	}

	deep := DemoteInferredDynamicAny(in)
	deepArr := deep[0].(*typ.Array)
	deepRec := deepArr.Element.(*typ.Record)
	if f := deepRec.GetField("order"); f == nil || !typ.IsUnknown(f.Type) {
		t.Fatalf("deep order field = %v, want unknown", f)
	}
}

// An optional any[] keeps its proven array shape under the slot law: the
// optional structure and the array element any are preserved, only a deeper
// export demote rewrites the element.
func TestDemoteSlot_OptionalAnyArrayStaysArray(t *testing.T) {
	in := []typ.Type{typ.NewOptional(typ.NewArray(typ.Any))}

	slot := DemoteInferredDynamicAnySlot(in)
	opt, ok := slot[0].(*typ.Optional)
	if !ok {
		t.Fatalf("slot demote = %v, want any[]?", slot)
	}
	arr, ok := opt.Inner.(*typ.Array)
	if !ok || !typ.IsAny(arr.Element) {
		t.Fatalf("slot inner = %v, want any[]", opt.Inner)
	}

	deep := DemoteInferredDynamicAny(in)
	deepOpt := deep[0].(*typ.Optional)
	deepArr := deepOpt.Inner.(*typ.Array)
	if !typ.IsUnknown(deepArr.Element) {
		t.Fatalf("deep inner = %v, want unknown[]", deepOpt.Inner)
	}
}
