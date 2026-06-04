package returns

import (
	"testing"

	"github.com/wippyai/go-lua/types/typ"
)

func TestRefineDeclaredReturnVectorStructuralMapRow(t *testing.T) {
	declared := []typ.Type{
		typ.NewOptional(typ.NewArray(typ.NewMap(typ.String, typ.Any))),
	}
	rowEvidence := typ.NewRecord().
		Field("count", typ.Integer).
		Field("exists", typ.Boolean).
		Build()
	evidence := []typ.Type{typ.NewArray(rowEvidence)}

	got, ok := RefineDeclaredReturnVector(declared, evidence)
	if !ok || len(got) != 1 {
		t.Fatalf("RefineDeclaredReturnVector ok=%v got=%v, want one refined slot", ok, got)
	}
	opt, ok := got[0].(*typ.Optional)
	if !ok {
		t.Fatalf("slot = %T %[1]v, want optional array", got[0])
	}
	arr, ok := opt.Inner.(*typ.Array)
	if !ok {
		t.Fatalf("inner = %T %[1]v, want array", opt.Inner)
	}
	row, ok := arr.Element.(*typ.Record)
	if !ok {
		t.Fatalf("element = %T %[1]v, want row record", arr.Element)
	}
	count := row.GetField("count")
	if count == nil || count.Optional || !typ.TypeEquals(count.Type, typ.Integer) {
		t.Fatalf("count field = %v, want required integer", count)
	}
}

func TestRefineDeclaredReturnVectorPreservesTopLevelAny(t *testing.T) {
	evidence := typ.NewRecord().Field("count", typ.Integer).Build()
	got, ok := RefineDeclaredReturnVector([]typ.Type{typ.Any}, []typ.Type{evidence})
	if ok {
		t.Fatalf("RefineDeclaredReturnVector refined top-level any to %v", got)
	}
}

func TestRefineDeclaredReturnVectorRejectsIncompatibleScalar(t *testing.T) {
	got, ok := RefineDeclaredReturnVector([]typ.Type{typ.Number}, []typ.Type{typ.Boolean})
	if ok {
		t.Fatalf("RefineDeclaredReturnVector refined incompatible scalar to %v", got)
	}
}
