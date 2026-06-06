package callreturn

import (
	"testing"

	"github.com/wippyai/go-lua/types/typ"
)

func TestResultTypesPrefersExpressionAdjustedReturns(t *testing.T) {
	returns := []typ.Type{typ.String, typ.Boolean}
	got := ResultTypes(typ.NewTuple(typ.Number), returns)

	if len(got) != 2 || got[0] != typ.String || got[1] != typ.Boolean {
		t.Fatalf("ResultTypes(Returns) = %#v, want string, boolean", got)
	}
	got[0] = typ.Number
	if returns[0] != typ.String {
		t.Fatal("ResultTypes aliased result.Returns")
	}
}

func TestResultTypesUnpacksTupleType(t *testing.T) {
	tuple := typ.NewTuple(typ.String, typ.Boolean)
	got := ResultTypes(tuple, nil)

	if len(got) != 2 || got[0] != typ.String || got[1] != typ.Boolean {
		t.Fatalf("ResultTypes(tuple) = %#v, want string, boolean", got)
	}
	got[0] = typ.Number
	if tuple.Elements[0] != typ.String {
		t.Fatal("ResultTypes aliased tuple elements")
	}
}

func TestResultTypesSingleAndNil(t *testing.T) {
	if got := ResultTypes(typ.String, nil); len(got) != 1 || got[0] != typ.String {
		t.Fatalf("ResultTypes(single) = %#v, want string", got)
	}
	if got := ResultTypes(nil, nil); got != nil {
		t.Fatalf("ResultTypes(empty) = %#v, want nil", got)
	}
}
