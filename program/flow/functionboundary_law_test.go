package flow_test

import (
	"testing"

	"github.com/wippyai/go-lua/program"
	flowkind "github.com/wippyai/go-lua/program/flow/kind"
	"github.com/wippyai/go-lua/program/keyspace"
	programlower "github.com/wippyai/go-lua/program/lower"
)

var (
	functionBoundaryTermSink keyspace.Term
	functionBoundaryBoolSink bool
)

func lowerFunctionBoundaryLaw(t *testing.T, text string) *program.Program {
	t.Helper()
	program, err := programlower.Lower(programlower.Source{Name: "function-boundary.lua", Text: []byte(text)})
	if err != nil {
		t.Fatal(err)
	}
	return program
}

func TestFunctionBoundaryRootOwnsEntryAndCompleteOutcomeRange(t *testing.T) {
	program := lowerFunctionBoundaryLaw(t, `
while test() do break end
if jump() then goto done end
::done::
return finish()
`)
	flow := program.Flow()
	boundaries := flow.FunctionBoundaries()
	entry, entryOK := program.Source().Index().Entry()
	if !entryOK {
		t.Fatal("missing Source entry Body")
	}
	root, rootOK := boundaries.Root()
	body, bodyOK := boundaries.ForBody(entry)
	if !rootOK || !bodyOK || !root.Available() || !body.Available() {
		t.Fatalf("root/body = %v/%v %v/%v", rootOK, root.Available(), bodyOK, body.Available())
	}
	if got, ok := root.Body(); !ok || got != entry {
		t.Fatalf("root Body = %v/%v, want %v", got, ok, entry)
	}
	if got, ok := body.Body(); !ok || got != entry {
		t.Fatalf("body Body = %v/%v, want %v", got, ok, entry)
	}
	portEntry, portOK := flow.Ports().Entry(entry)
	rootEntry, rootEntryOK := root.Entry()
	if !portOK || !rootEntryOK || rootEntry != portEntry {
		t.Fatalf("root Entry = %v/%v, port = %v/%v", rootEntry, rootEntryOK, portEntry, portOK)
	}
	start, end, rangeOK := flow.Outcomes().BodyRange(entry)
	if !rangeOK || root.OutcomeCount() != end-start || body.OutcomeCount() != end-start {
		t.Fatalf("root/body Outcome range = %d/%d, want %d/%d", root.OutcomeCount(), body.OutcomeCount(), start, end)
	}
	found := map[uint8]bool{}
	for index := 0; index < root.OutcomeCount(); index++ {
		exit, exitOK := root.OutcomeAt(index)
		global, globalOK := flow.Outcomes().At(start + index)
		if !exitOK || !globalOK {
			t.Fatalf("root Outcome[%d] = %#v/%v global=%v/%v", index, exit, exitOK, global, globalOK)
		}
		outcome, outcomeOK := flow.Outcomes().Get(global)
		if !outcomeOK || exit.Outcome != global || exit.Body != outcome.Body || exit.Kind != outcome.Kind || exit.Target != outcome.Target || outcome.Body != entry {
			t.Fatalf("root Outcome[%d] = %#v; global = %v/%#v", index, exit, global, outcome)
		}
		found[uint8(outcome.Kind)] = true
		got, gotOK := boundaries.ForOutcome(global)
		gotBody, gotBodyOK := got.Body()
		if !gotOK || !gotBodyOK || gotBody != entry {
			t.Fatalf("ForOutcome(%v) failed", global)
		}
	}
	if !found[uint8(flowkind.OutcomeReturn)] || !found[uint8(flowkind.OutcomeNormal)] ||
		!found[uint8(flowkind.OutcomeThrow)] || !found[uint8(flowkind.OutcomeYield)] || !found[uint8(flowkind.OutcomeCancel)] {
		t.Fatalf("root outcomes lost a terminal disposition: %#v", found)
	}
	if found[uint8(flowkind.OutcomeBreak)] || found[uint8(flowkind.OutcomeGoto)] {
		t.Fatalf("locally discharged break/goto escaped as root terminal outcomes: %#v", found)
	}
	resolved, resolvedOK := boundaries.ResolveBodyContextID(root.ContextID())
	if !resolvedOK || !resolved.Equal(body) {
		t.Fatal("root Body context inverse did not resolve exact Body")
	}
}

func TestFunctionBoundaryJoinsFormalCaptureVarargAndOutcomeOrder(t *testing.T) {
	program := lowerFunctionBoundaryLaw(t, `
local outer = seed()
local function factory(a, b, ...)
  local inner = a
  local function nested(c)
    return outer, inner, c
  end
  return nested
end
return factory(1, 2, 3)
`)
	flow := program.Flow()
	boundaries := flow.FunctionBoundaries()
	authored := flow.Authored().Functions()
	formalOrder := program.Source().Formals()
	for functionIndex := 0; functionIndex < authored.Count(); functionIndex++ {
		function, functionOK := authored.At(functionIndex)
		boundary, boundaryOK := boundaries.For(function)
		if !functionOK || !boundaryOK || !boundary.Available() {
			t.Fatalf("Function boundary[%d] = %v/%v", functionIndex, boundaryOK, boundary.Available())
		}
		owner, body, vararg, rowOK := authored.Get(function)
		gotFunction, gotFunctionOK := boundary.Function()
		gotOwner, gotOwnerOK := boundary.Owner()
		gotBody, gotBodyOK := boundary.Body()
		if !rowOK || !gotFunctionOK || !gotOwnerOK || !gotBodyOK || gotFunction != function || gotOwner != owner || gotBody != body {
			t.Fatalf("Function row = %v/%v owner %v/%v body %v/%v", gotFunction, gotFunctionOK, gotOwner, gotOwnerOK, gotBody, gotBodyOK)
		}
		if ownerBoundary, ownerOK := boundaries.ForFunctionBody(body); !ownerOK || !ownerBoundary.Equal(boundary) {
			t.Fatalf("Function Body inverse[%d] failed", functionIndex)
		}
		entry, entryOK := boundary.Entry()
		portEntry, portOK := flow.Ports().Entry(body)
		if !entryOK || !portOK || entry != portEntry {
			t.Fatalf("Function Entry = %v/%v, port = %v/%v", entry, entryOK, portEntry, portOK)
		}
		gotVararg, gotVarargOK := boundary.Vararg()
		if gotVarargOK != (vararg != 0) || gotVararg != vararg {
			t.Fatalf("Function Vararg = %v/%v, want %v", gotVararg, gotVarargOK, vararg)
		}
		formalCount, formalOK := formalOrder.Len(function)
		if !formalOK || boundary.FormalCount() != formalCount {
			t.Fatalf("Function formal count = %d/%v, boundary %d", formalCount, formalOK, boundary.FormalCount())
		}
		for index := 0; index < formalCount; index++ {
			want, wantOK := formalOrder.At(function, index)
			got, gotOK := boundary.FormalAt(index)
			if !wantOK || !gotOK || got != want {
				t.Fatalf("Formal[%d] = %v/%v, want %v/%v", index, got, gotOK, want, wantOK)
			}
		}
		captureCount, captureOK := authored.CaptureCount(function)
		if !captureOK || boundary.CaptureCount() != captureCount {
			t.Fatalf("Capture count = %d/%v, boundary %d", captureCount, captureOK, boundary.CaptureCount())
		}
		for index := 0; index < captureCount; index++ {
			inner, outer, wantOK := authored.CaptureAt(function, index)
			capture, gotOK := boundary.CaptureAt(index)
			if !wantOK || !gotOK || capture.Inner != inner || capture.Outer != outer {
				t.Fatalf("Capture[%d] = %#v/%v, want %v/%v/%v", index, capture, gotOK, inner, outer, wantOK)
			}
		}
		start, end, rangeOK := flow.Outcomes().BodyRange(body)
		if !rangeOK || boundary.OutcomeCount() != end-start {
			t.Fatalf("Function Outcome count = %d, want %d/%v", boundary.OutcomeCount(), end-start, rangeOK)
		}
		for index := 0; index < boundary.OutcomeCount(); index++ {
			outcome, outcomeOK := boundary.OutcomeAt(index)
			global, globalOK := flow.Outcomes().At(start + index)
			if !outcomeOK || !globalOK || outcome.Outcome != global {
				t.Fatalf("Function Outcome[%d] = %#v/%v, global %v/%v", index, outcome, outcomeOK, global, globalOK)
			}
			if ownerBoundary, ownerOK := boundaries.ForFunctionOutcome(global); !ownerOK || !ownerBoundary.Equal(boundary) {
				t.Fatalf("Function Outcome inverse[%d] failed", index)
			}
			direct, directOK := flow.FunctionBoundaries().ForOutcome(global)
			directExit, directOrdinal, exitOK := direct.OutcomeForTerm(global)
			if !directOK || !exitOK || directOrdinal != index || directExit != outcome {
				t.Fatalf("Body Outcome direct inverse[%d] = %#v/%d/%v", index, directExit, directOrdinal, exitOK)
			}
		}
	}
}

func TestFunctionBoundaryContextReplayAndAllocation(t *testing.T) {
	text := `local function f(a, b, ...) return a, b end return f(1, 2, 3)`
	left, right := lowerFunctionBoundaryLaw(t, text), lowerFunctionBoundaryLaw(t, text)
	leftBoundaries, rightBoundaries := left.Flow().FunctionBoundaries(), right.Flow().FunctionBoundaries()
	if leftBoundaries.Count() != rightBoundaries.Count() {
		t.Fatalf("Function boundary counts = %d/%d", leftBoundaries.Count(), rightBoundaries.Count())
	}
	for index := 0; index < leftBoundaries.Count(); index++ {
		leftBoundary, leftOK := leftBoundaries.At(index)
		rightBoundary, rightOK := rightBoundaries.At(index)
		if !leftOK || !rightOK || leftBoundary.ContextID() != rightBoundary.ContextID() || !leftBoundary.Equal(rightBoundary) {
			t.Fatalf("replay boundary[%d] = %v/%v equal=%v", index, leftBoundary.ContextID(), rightBoundary.ContextID(), leftBoundary.Equal(rightBoundary))
		}
		resolved, resolvedOK := leftBoundaries.ResolveContextID(leftBoundary.ContextID())
		if !resolvedOK || !resolved.Equal(leftBoundary) {
			t.Fatalf("Function context inverse[%d] failed", index)
		}
	}
	leftBoundary, leftOK := leftBoundaries.At(0)
	rightBoundary, rightOK := rightBoundaries.At(0)
	leftBody, leftBodyOK := leftBoundary.Body()
	rightBody, rightBodyOK := rightBoundary.Body()
	leftBodyBoundary, leftBoundaryOK := leftBoundaries.ForBody(leftBody)
	rightBodyBoundary, rightBoundaryOK := rightBoundaries.ForBody(rightBody)
	if !leftOK || !rightOK || !leftBodyOK || !rightBodyOK || !leftBoundaryOK || !rightBoundaryOK ||
		leftBody == 0 || rightBody == 0 || leftBodyBoundary.ContextID() != rightBodyBoundary.ContextID() ||
		!leftBodyBoundary.Equal(rightBodyBoundary) {
		t.Fatal("non-root Body boundary did not replay with equal context")
	}
	leftResolvedBody, leftResolvedOK := leftBoundaries.ResolveBodyContextID(leftBodyBoundary.ContextID())
	rightResolvedBody, rightResolvedOK := rightBoundaries.ResolveBodyContextID(rightBodyBoundary.ContextID())
	if !leftResolvedOK || !rightResolvedOK || !leftResolvedBody.Equal(leftBodyBoundary) || !rightResolvedBody.Equal(rightBodyBoundary) {
		t.Fatal("non-root Body context inverse failed")
	}
	root, rootOK := leftBoundaries.Root()
	if !rootOK {
		t.Fatal("missing replay root")
	}
	if allocations := testing.AllocsPerRun(1000, func() {
		_, functionBoundaryBoolSink = leftBoundaries.Root()
		functionBoundaryTermSink, functionBoundaryBoolSink = root.Body()
		_, functionBoundaryBoolSink = root.OutcomeAt(0)
	}); allocations != 0 {
		t.Fatalf("Function boundary queries allocate %v times", allocations)
	}
}

func TestBodyReturnProjectionThreadsExactBodyOwnerInConstantTime(t *testing.T) {
	text := `return finish()`
	left, right := lowerFunctionBoundaryLaw(t, text), lowerFunctionBoundaryLaw(t, text)
	leftRoot, leftRootOK := left.Flow().FunctionBoundaries().Root()
	rightRoot, rightRootOK := right.Flow().FunctionBoundaries().Root()
	leftBodyTerm, leftTermOK := leftRoot.Body()
	rightBodyTerm, rightTermOK := rightRoot.Body()
	leftBody, leftBodyOK := left.Flow().FunctionBoundaries().ForBody(leftBodyTerm)
	rightBody, rightBodyOK := right.Flow().FunctionBoundaries().ForBody(rightBodyTerm)
	if !leftRootOK || !rightRootOK || !leftTermOK || !rightTermOK || !leftBodyOK || !rightBodyOK || !leftBody.Equal(rightBody) {
		t.Fatal("equivalent replay did not publish equal root Body proofs")
	}
	returned, returnedOK := left.Flow().BodyReturns().ForBody(leftBody)
	if !returnedOK || !returned.Available() || returned.ValuesCount() != 1 {
		t.Fatalf("left Body Return = %#v/%v values=%d", returned, returnedOK, returned.ValuesCount())
	}
	if _, ok := left.Flow().BodyReturns().ForBody(rightBody); ok {
		t.Fatal("Body Return projection accepted an equal foreign-owner Body")
	}
	if allocations := testing.AllocsPerRun(1000, func() {
		_, functionBoundaryBoolSink = left.Flow().BodyReturns().ForBody(leftBody)
		_ = returned.ValuesCount()
		_, functionBoundaryBoolSink = returned.ValueAt(0)
	}); allocations != 0 {
		t.Fatalf("Body Return projection queries allocate %v times", allocations)
	}
}
