package functionboundary_test

import "testing"

func TestSealRetainsCanonicalOutcomeRowsWithoutChangingBoundaryQueries(t *testing.T) {
	program := lowerFunctionBoundary(t, "local function f(a) return a end return f(1)")
	view := program.Flow()
	boundaries := view.FunctionBoundaries()
	function, ok := view.Authored().Functions().At(0)
	if !ok {
		t.Fatal("fixture did not publish a Function")
	}
	boundary, ok := boundaries.For(function)
	if !ok {
		t.Fatal("sealed FunctionBoundary did not resolve Function")
	}
	_, body, _, ok := view.Authored().Functions().Get(function)
	if !ok {
		t.Fatal("authored Function row was unavailable")
	}
	start, end, rangeOK := view.Outcomes().BodyRange(body)
	if !rangeOK || boundary.OutcomeCount() != end-start {
		t.Fatalf("Boundary Outcome count = %d, want canonical range %d/%d", boundary.OutcomeCount(), start, end)
	}
	for index := start; index < end; index++ {
		term, termOK := view.Outcomes().At(index)
		exit, exitOK := boundary.OutcomeAt(index - start)
		if !termOK || !exitOK || exit.Outcome != term {
			t.Fatalf("Boundary Outcome[%d] = %#v/%v, canonical = %v/%v", index-start, exit, exitOK, term, termOK)
		}
	}
}
