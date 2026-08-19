package flow

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/program/flow/causal"
)

// TestZeroViewProjectionsFailClosed keeps the dissolved query wrappers honest:
// a View with no component hands out a nil owner, and every published query on
// that owner must fail closed rather than panic.
func TestZeroViewProjectionsFailClosed(t *testing.T) {
	view := View{}
	if view.Executable().Contains(1) || view.Executable().Count() != 0 {
		t.Fatal("nil executable owner answered a membership query")
	}
	if _, ok := view.Pending().Count(1); ok {
		t.Fatal("nil pending owner answered a count query")
	}
	if _, ok := view.Continuation().CellCount(1); ok {
		t.Fatal("nil continuation owner answered a cell count")
	}
	if _, ok := view.DirectFunctions().For(1); ok {
		t.Fatal("nil direct-function owner answered a lookup")
	}
	if view.Candidates().Unary().NumericCount() != 0 || view.Candidates().Binary().OrderCount() != 0 ||
		view.Candidates().Access().GetCount() != 0 {
		t.Fatal("nil candidate owner answered a bucket count")
	}
	if view.AccessGeometry().Available() || view.BinaryPrimitives().Available() {
		t.Fatal("nil geometry owner reported itself available")
	}
	if _, _, ok := view.AccessGeometry().ExactRead(1); ok {
		t.Fatal("nil access geometry answered an exact read")
	}
	if _, _, ok := view.AccessGeometry().DirectCall(1); ok {
		t.Fatal("nil access geometry answered a direct call")
	}
	if _, ok := view.BinaryPrimitives().Primitive(1); ok {
		t.Fatal("nil binary primitive owner answered a primitive lookup")
	}
	if view.Causal().Sites().Count() != 0 || view.Causal().Edges().Count() != 0 ||
		view.Causal().Boundaries().Count() != 0 || view.Causal().Successors().TotalCount() != 0 {
		t.Fatal("nil causal owner answered a row count")
	}
	if _, ok := view.Causal().Successors().FinalAt(0); ok {
		t.Fatal("nil causal owner issued a final route")
	}
	if view.Causal().OwnsFinalRoute(causal.FinalRoute{}) {
		t.Fatal("nil causal owner claimed a zero final route")
	}
	if view.LocalWTO().Count() != 0 || view.LocalWTO().EventCount() != 0 {
		t.Fatal("nil local schedule answered a count")
	}
	if _, ok := view.FunctionBoundaries().Root(); ok {
		t.Fatal("nil boundary owner issued a root")
	}
	if view.Outcomes().Count() != 0 {
		t.Fatal("nil outcome owner answered a count")
	}
	if _, ok := view.Ports().Finish(1); ok {
		t.Fatal("nil port owner answered a finish")
	}
}
