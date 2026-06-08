package callreturn

import (
	"testing"

	"github.com/wippyai/go-lua/types/typ"
)

func TestReturnVectorPrefersExpressionAdjustedReturns(t *testing.T) {
	returns := []typ.Type{typ.String, typ.Boolean}
	got := ReturnVectorOfCallResult(typ.NewTuple(typ.Number), returns).Types()

	if len(got) != 2 || got[0] != typ.String || got[1] != typ.Boolean {
		t.Fatalf("ReturnVector(Returns) = %#v, want string, boolean", got)
	}
	got[0] = typ.Number
	if returns[0] != typ.String {
		t.Fatal("ReturnVector aliased result.Returns")
	}
}

func TestReturnVectorUnpacksTupleType(t *testing.T) {
	tuple := typ.NewTuple(typ.String, typ.Boolean)
	got := ReturnVectorOfCallResult(tuple, nil).Types()

	if len(got) != 2 || got[0] != typ.String || got[1] != typ.Boolean {
		t.Fatalf("ReturnVector(tuple) = %#v, want string, boolean", got)
	}
	got[0] = typ.Number
	if tuple.Elements[0] != typ.String {
		t.Fatal("ReturnVector aliased tuple elements")
	}
}

func TestReturnVectorSingleAndNil(t *testing.T) {
	if got := ReturnVectorOfCallResult(typ.String, nil).Types(); len(got) != 1 || got[0] != typ.String {
		t.Fatalf("ReturnVector(single) = %#v, want string", got)
	}
	if got := ReturnVectorOfCallResult(nil, nil).Types(); got != nil {
		t.Fatalf("ReturnVector(empty) = %#v, want nil", got)
	}
}

func TestResultTypesUsesReturnVectorCompatibility(t *testing.T) {
	if got := ResultTypes(typ.String, nil); len(got) != 1 || got[0] != typ.String {
		t.Fatalf("ResultTypes(single) = %#v, want string", got)
	}
}
