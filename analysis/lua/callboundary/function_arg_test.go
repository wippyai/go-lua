package callboundary

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/type/typ"
)

func TestProjectContextualFunctionArgKeepsExpectedParamsAndSynthesizedReturns(t *testing.T) {
	node := typ.NewRecord().
		Field("id", typ.String).
		Field("children", typ.NewArray(typ.String)).
		Build()
	bodyDemand := typ.NewRecord().Field("children", typ.String).Build()
	mapped := typ.NewRecord().
		Field("id", typ.String).
		Field("child_count", typ.Integer).
		Build()

	got := ProjectContextualFunctionArg(
		typ.Func().Param("value", node).Returns(typ.Any).Build(),
		typ.Func().Param("decoded", bodyDemand).Returns(mapped).Build(),
	)

	fn, ok := got.(*typ.Function)
	if !ok || fn == nil {
		t.Fatalf("projected = %v (%T), want function", got, got)
	}
	if len(fn.Params) != 1 || fn.Params[0].Name != "decoded" || !typ.TypeEquals(fn.Params[0].Type, node) {
		t.Fatalf("params = %v, want decoded node", fn.Params)
	}
	if len(fn.Returns) != 1 || !typ.TypeEquals(fn.Returns[0], mapped) {
		t.Fatalf("returns = %v, want mapped record", fn.Returns)
	}
}
