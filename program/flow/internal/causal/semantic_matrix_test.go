package causal

import (
	"math"
	"testing"

	"github.com/wippyai/go-lua/program/flow/internal/authored"
	"github.com/wippyai/go-lua/program/flow/kind"
	"github.com/wippyai/go-lua/program/keyspace"
	"github.com/wippyai/go-lua/program/source"
	"github.com/wippyai/go-lua/program/static"
)

func TestSemanticMatrixDirectCallPublishesExactBoundaryDenominator(t *testing.T) {
	body := causalTerm(keyspace.FamilyBody, 1)
	call := causalTerm(keyspace.FamilyCall, 1)
	values := causalTerm(keyspace.FamilyValues, 1)
	nilValue := causalTerm(keyspace.FamilyNil, 1)
	f := openCausalFixture(t, causalSpec{
		counts: causalCounts(
			causalFamilyCount{keyspace.FamilyBody, 1},
			causalFamilyCount{keyspace.FamilyCall, 1},
			causalFamilyCount{keyspace.FamilyValues, 1},
			causalFamilyCount{keyspace.FamilyNil, 1},
		),
		rows:      [][]keyspace.Term{{call}},
		nilOwners: []keyspace.Term{body},
		flow: authored.Input{
			Values: authored.ValuesInput{Rows: []authored.Value{{Owner: body}}},
			Calls:  []authored.Call{{Owner: body, Callee: nilValue, Actuals: values}},
		},
	})

	boundary, ok := f.result.Boundaries().For(call)
	if !ok {
		t.Fatal("direct Call boundary is absent")
	}
	if boundary.Call != call || boundary.Normal == 0 || boundary.TailReturn != 0 || boundary.Other != 0 {
		t.Fatalf("direct Call boundary = %#v", boundary)
	}
	if boundary.Throw == 0 || boundary.Yield == 0 || boundary.Cancel == 0 {
		t.Fatalf("direct Call exceptional arms = %#v", boundary)
	}
	throwExit, throwOK := f.outcomes.BodyExit(body, kind.OutcomeThrow)
	yieldExit, yieldOK := f.outcomes.BodyExit(body, kind.OutcomeYield)
	cancelExit, cancelOK := f.outcomes.BodyExit(body, kind.OutcomeCancel)
	if !throwOK || !yieldOK || !cancelOK || boundary.Throw != throwExit || boundary.Yield != yieldExit || boundary.Cancel != cancelExit {
		t.Fatalf("direct Call exceptional targets = %#v, want %v/%v/%v", boundary, throwExit, yieldExit, cancelExit)
	}
	if entry, ok := f.ports.Entry(body); !ok || entry != body {
		t.Fatalf("Body Entry = %v/%v", entry, ok)
	}
	if finish, ok := f.ports.Finish(body); !ok || finish != body {
		t.Fatalf("Body Finish = %v/%v", finish, ok)
	}
	if finish, ok := f.ports.Finish(call); !ok || finish != call {
		t.Fatalf("root Call Finish = %v/%v", finish, ok)
	}
	if got, want := f.result.Successors().Count(call), 4; got != want {
		t.Fatalf("direct Call successor denominator = %d, want %d", got, want)
	}
	for index := 0; index < f.result.Edges().Count(); index++ {
		edge, ok := f.result.Edges().At(index)
		if !ok {
			t.Fatalf("Edges.At(%d) failed", index)
		}
		if keyspace.TermFamily(edge.From) == keyspace.FamilyCall {
			t.Fatalf("Call-origin local Edge leaked into causal plane: %#v", edge)
		}
	}
}

func TestSemanticMatrixSelectOrLeftCallPublishesGuardedBoundary(t *testing.T) {
	selectTerm := causalTerm(keyspace.FamilySelect, 1)
	call := causalTerm(keyspace.FamilyCall, 1)
	leftNil := causalTerm(keyspace.FamilyNil, 1)
	f := openCausalFixture(t, selectLeftCallSpec(kind.SelectOr))
	boundary, ok := f.result.Boundaries().For(call)
	if !ok || boundary.Normal != selectTerm || boundary.Other != leftNil {
		t.Fatalf("nested Select-or Call boundary = %#v/%v", boundary, ok)
	}
	if got, want := f.result.Successors().Count(call), 5; got != want {
		t.Fatalf("nested Select-or Call successor denominator = %d, want %d", got, want)
	}
	seenTrue, seenFalse := false, false
	for index := 0; index < f.result.Successors().Count(call); index++ {
		successor, ok := f.result.Successors().At(call, index)
		if !ok {
			t.Fatalf("nested Select-or successor %d unavailable", index)
		}
		if successor.Arm == BoundarySelectTrue {
			seenTrue = successor.To == selectTerm && successor.Decision == selectTerm && successor.Truth
		}
		if successor.Arm == BoundarySelectFalse {
			seenFalse = successor.To == leftNil && successor.Decision == selectTerm && !successor.Truth
		}
	}
	if !seenTrue || !seenFalse {
		t.Fatalf("nested Select-or guarded arms = true %v false %v", seenTrue, seenFalse)
	}
}

func selectLeftCallSpec(op kind.SelectOp) causalSpec {
	body := causalTerm(keyspace.FamilyBody, 1)
	returned := causalTerm(keyspace.FamilyReturn, 1)
	selectTerm := causalTerm(keyspace.FamilySelect, 1)
	call := causalTerm(keyspace.FamilyCall, 1)
	returnValues := causalTerm(keyspace.FamilyValues, 1)
	callValues := causalTerm(keyspace.FamilyValues, 2)
	leftNil := causalTerm(keyspace.FamilyNil, 1)
	calleeNil := causalTerm(keyspace.FamilyNil, 2)
	return causalSpec{
		counts: causalCounts(
			causalFamilyCount{keyspace.FamilyBody, 1},
			causalFamilyCount{keyspace.FamilyReturn, 1},
			causalFamilyCount{keyspace.FamilySelect, 1},
			causalFamilyCount{keyspace.FamilyCall, 1},
			causalFamilyCount{keyspace.FamilyValues, 2},
			causalFamilyCount{keyspace.FamilyNil, 2},
		),
		rows:      [][]keyspace.Term{{returned}},
		nilOwners: []keyspace.Term{body, body},
		flow: authored.Input{
			Values: authored.ValuesInput{Rows: []authored.Value{
				{Owner: body, Fixed: authored.Range{End: 1}},
				{Owner: body, Fixed: authored.Range{Start: 1, End: 1}},
			}, Terms: []keyspace.Term{selectTerm}},
			Calls:   []authored.Call{{Owner: body, Callee: calleeNil, Actuals: callValues}},
			Control: authored.ControlInput{Returns: []authored.Return{{Owner: body, Values: returnValues}}},
			Operators: authored.OperatorsInput{Selects: []authored.Select{{
				Owner: body, Op: op, Left: call, Right: leftNil,
			}}},
		},
	}
}

func TestSemanticMatrixSelectLeftCallPublishesGuardedBoundary(t *testing.T) {
	selectTerm := causalTerm(keyspace.FamilySelect, 1)
	call := causalTerm(keyspace.FamilyCall, 1)
	leftNil := causalTerm(keyspace.FamilyNil, 1)
	f := openCausalFixture(t, selectLeftCallSpec(kind.SelectAnd))
	boundary, ok := f.result.Boundaries().For(call)
	if !ok || boundary.Normal != selectTerm || boundary.Other != leftNil {
		t.Fatalf("nested Select-left Call boundary = %#v/%v", boundary, ok)
	}
	if got, want := f.result.Successors().Count(call), 5; got != want {
		t.Fatalf("nested Select-left Call successor denominator = %d, want %d", got, want)
	}
	seenTrue, seenFalse := false, false
	for index := 0; index < f.result.Successors().Count(call); index++ {
		successor, ok := f.result.Successors().At(call, index)
		if !ok {
			t.Fatalf("nested Select-left successor %d unavailable", index)
		}
		if successor.Arm == BoundarySelectTrue {
			seenTrue = successor.To == leftNil && successor.Decision == selectTerm && successor.Truth
		}
		if successor.Arm == BoundarySelectFalse {
			seenFalse = successor.To == selectTerm && successor.Decision == selectTerm && !successor.Truth
		}
	}
	if !seenTrue || !seenFalse {
		t.Fatalf("nested Select-left guarded arms = true %v false %v", seenTrue, seenFalse)
	}
}

func TestSemanticMatrixTailCallPublishesTerminalReturnBoundary(t *testing.T) {
	body := causalTerm(keyspace.FamilyBody, 1)
	returned := causalTerm(keyspace.FamilyReturn, 1)
	call := causalTerm(keyspace.FamilyCall, 1)
	returnValues := causalTerm(keyspace.FamilyValues, 1)
	callValues := causalTerm(keyspace.FamilyValues, 2)
	calleeNil := causalTerm(keyspace.FamilyNil, 1)
	f := openCausalFixture(t, causalSpec{
		counts: causalCounts(
			causalFamilyCount{keyspace.FamilyBody, 1},
			causalFamilyCount{keyspace.FamilyReturn, 1},
			causalFamilyCount{keyspace.FamilyCall, 1},
			causalFamilyCount{keyspace.FamilyValues, 2},
			causalFamilyCount{keyspace.FamilyNil, 1},
		),
		rows:      [][]keyspace.Term{{returned}},
		nilOwners: []keyspace.Term{body},
		flow: authored.Input{
			Values: authored.ValuesInput{Rows: []authored.Value{
				{Owner: body, Tail: call},
				{Owner: body},
			}},
			Calls:   []authored.Call{{Owner: body, Callee: calleeNil, Actuals: callValues}},
			Control: authored.ControlInput{Returns: []authored.Return{{Owner: body, Values: returnValues}}},
		},
	})

	boundary, ok := f.result.Boundaries().For(call)
	if !ok || boundary.Normal != 0 || boundary.Other != 0 || boundary.TailReturn == 0 || boundary.Throw == 0 || boundary.Yield == 0 || boundary.Cancel == 0 {
		t.Fatalf("tail Call boundary = %#v/%v", boundary, ok)
	}
	returnExit, ok := f.outcomes.ReturnExit(returned)
	if !ok || boundary.TailReturn != returnExit {
		t.Fatalf("tail Call Return destination = %v/%v, want ReturnExit %v/%v", boundary.TailReturn, ok, returnExit, ok)
	}
	if got, want := f.result.Successors().Count(call), 4; got != want {
		t.Fatalf("tail Call successor denominator = %d, want %d", got, want)
	}
	tail, ok := f.result.Successors().At(call, 0)
	if !ok || tail.Arm != BoundaryTail || tail.To != returnExit {
		t.Fatalf("tail Call arm = %#v/%v, want terminal Return Outcome %v", tail, ok, returnExit)
	}
	for index := 0; index < f.result.Edges().Count(); index++ {
		edge, ok := f.result.Edges().At(index)
		if !ok {
			t.Fatalf("Edges.At(%d) failed", index)
		}
		if edge.From == call {
			t.Fatalf("tail Call leaked local Edge: %#v", edge)
		}
	}
}

func TestSemanticMatrixDeadTailReturnIsSkippedBeforeTailTopology(t *testing.T) {
	body := causalTerm(keyspace.FamilyBody, 1)
	terminalReturn := causalTerm(keyspace.FamilyReturn, 1)
	deadReturn := causalTerm(keyspace.FamilyReturn, 2)
	call := causalTerm(keyspace.FamilyCall, 1)
	terminalValues := causalTerm(keyspace.FamilyValues, 1)
	deadValues := causalTerm(keyspace.FamilyValues, 2)
	callValues := causalTerm(keyspace.FamilyValues, 3)
	calleeNil := causalTerm(keyspace.FamilyNil, 1)
	f := openCausalFixture(t, causalSpec{
		counts: causalCounts(
			causalFamilyCount{keyspace.FamilyBody, 1},
			causalFamilyCount{keyspace.FamilyReturn, 2},
			causalFamilyCount{keyspace.FamilyCall, 1},
			causalFamilyCount{keyspace.FamilyValues, 3},
			causalFamilyCount{keyspace.FamilyNil, 1},
		),
		rows:      [][]keyspace.Term{{terminalReturn, deadReturn}},
		nilOwners: []keyspace.Term{body},
		flow: authored.Input{
			Values: authored.ValuesInput{Rows: []authored.Value{
				{Owner: body},
				{Owner: body, Tail: call},
				{Owner: body},
			}},
			Calls: []authored.Call{{Owner: body, Callee: calleeNil, Actuals: callValues}},
			Control: authored.ControlInput{Returns: []authored.Return{
				{Owner: body, Values: terminalValues},
				{Owner: body, Values: deadValues},
			}},
		},
	})

	if !f.executable.Executable(terminalReturn) {
		t.Fatal("terminal Return is not executable")
	}
	if f.executable.Executable(deadReturn) || f.executable.Executable(call) {
		t.Fatalf("unreachable tail topology remained executable: Return=%v Call=%v", f.executable.Executable(deadReturn), f.executable.Executable(call))
	}
	if _, ok := f.result.Boundaries().For(call); ok {
		t.Fatal("dead tail Call unexpectedly published a CallBoundary")
	}
	if got := f.result.Successors().Count(call); got != 0 {
		t.Fatalf("dead tail Call successor denominator = %d, want 0", got)
	}
	assertEveryArcDisposition(t, f)
}

func TestSemanticMatrixReturnPropagatesToExactActivationOutcome(t *testing.T) {
	body := causalTerm(keyspace.FamilyBody, 1)
	returned := causalTerm(keyspace.FamilyReturn, 1)
	values := causalTerm(keyspace.FamilyValues, 1)
	f := openCausalFixture(t, causalSpec{
		counts: causalCounts(
			causalFamilyCount{keyspace.FamilyBody, 1},
			causalFamilyCount{keyspace.FamilyReturn, 1},
			causalFamilyCount{keyspace.FamilyValues, 1},
		),
		rows: [][]keyspace.Term{{returned}},
		flow: authored.Input{
			Values:  authored.ValuesInput{Rows: []authored.Value{{Owner: body}}},
			Control: authored.ControlInput{Returns: []authored.Return{{Owner: body, Values: values}}},
		},
	})

	returnExit, ok := f.outcomes.ReturnExit(returned)
	if !ok {
		t.Fatal("Return Outcome is absent")
	}
	if got, want := f.result.Successors().Count(returned), 1; got != want {
		t.Fatalf("Return successor denominator = %d, want %d", got, want)
	}
	successor, ok := f.result.Successors().At(returned, 0)
	if !ok || successor.IsBoundary() || successor.To != returnExit {
		t.Fatalf("Return successor = %#v/%v, want exact Outcome %v", successor, ok, returnExit)
	}
}

func TestSemanticMatrixNestedReturnPublishesTypedOutcomeChain(t *testing.T) {
	parent := causalTerm(keyspace.FamilyBody, 1)
	returnedBody := causalTerm(keyspace.FamilyBody, 2)
	fallthroughBody := causalTerm(keyspace.FamilyBody, 3)
	branch := causalTerm(keyspace.FamilyBranch, 1)
	returned := causalTerm(keyspace.FamilyReturn, 1)
	values := causalTerm(keyspace.FamilyValues, 1)
	condition := causalTerm(keyspace.FamilyNil, 1)
	f := openCausalFixture(t, causalSpec{
		counts: causalCounts(
			causalFamilyCount{keyspace.FamilyBody, 3},
			causalFamilyCount{keyspace.FamilyBranch, 1},
			causalFamilyCount{keyspace.FamilyReturn, 1},
			causalFamilyCount{keyspace.FamilyValues, 1},
			causalFamilyCount{keyspace.FamilyNil, 1},
		),
		rows:      [][]keyspace.Term{{branch}, {returned}, nil},
		nilOwners: []keyspace.Term{parent},
		flow: authored.Input{
			Values: authored.ValuesInput{Rows: []authored.Value{{Owner: returnedBody}}},
			Control: authored.ControlInput{
				Branches: []authored.Branch{{Owner: parent, Condition: condition, WhenTrue: returnedBody, WhenFalse: fallthroughBody}},
				Returns:  []authored.Return{{Owner: returnedBody, Values: values}},
			},
		},
	})

	if !f.executable.Executable(branch) || !f.executable.Executable(returned) {
		t.Fatalf("nested Branch/Return liveness = %v/%v", f.executable.Executable(branch), f.executable.Executable(returned))
	}
	returnExit, ok := f.outcomes.ReturnExit(returned)
	if !ok {
		t.Fatal("nested Return Outcome is absent")
	}
	if got, want := f.result.Successors().Count(returned), 1; got != want {
		t.Fatalf("nested Return successor denominator = %d, want %d", got, want)
	}
	successor, ok := f.result.Successors().At(returned, 0)
	if !ok || successor.IsBoundary() || successor.To != returnExit || keyspace.TermFamily(successor.To) != keyspace.FamilyOutcome {
		t.Fatalf("nested Return successor = %#v/%v, want exact Outcome %v", successor, ok, returnExit)
	}
	owner, outcomeKind, target, ok := f.outcomes.Get(returnExit)
	if !ok || owner != returnedBody || outcomeKind != kind.OutcomeReturn || target != 0 {
		t.Fatalf("nested Return Outcome = %v/%v/%v/%v", owner, outcomeKind, target, ok)
	}
	parentReturn, parentReturnOK := f.outcomes.Find(parent, kind.OutcomeReturn, 0)
	if !parentReturnOK {
		t.Fatal("parent Body Return Outcome is absent")
	}
	if next, propagated := f.outcomes.Propagation(returnExit); !propagated || next != parentReturn {
		t.Fatalf("nested Return propagation = %v/%v, want parent Return Outcome %v", next, propagated, parentReturn)
	}
	if got := f.result.Successors().Count(returnExit); got != 1 {
		t.Fatalf("nested Return Outcome successor denominator = %d, want 1", got)
	}
	propagation, ok := f.result.Successors().At(returnExit, 0)
	if !ok || propagation.IsBoundary() || propagation.To != parentReturn {
		t.Fatalf("nested Return propagation successor = %#v/%v, want %v", propagation, ok, parentReturn)
	}
	if got := f.result.Successors().Count(parentReturn); got != 0 {
		t.Fatalf("terminal parent Return Outcome successor denominator = %d, want 0", got)
	}
}

func TestSemanticMatrixBranchPublishesGuardedAlternatives(t *testing.T) {
	body, whenTrue, whenFalse, functionBody := causalTerm(keyspace.FamilyBody, 1), causalTerm(keyspace.FamilyBody, 2), causalTerm(keyspace.FamilyBody, 3), causalTerm(keyspace.FamilyBody, 4)
	branch := causalTerm(keyspace.FamilyBranch, 1)
	condition := causalTerm(keyspace.FamilyFunction, 1)
	returned := causalTerm(keyspace.FamilyReturn, 1)
	values := causalTerm(keyspace.FamilyValues, 1)
	f := openCausalFixture(t, causalSpec{
		counts: causalCounts(causalFamilyCount{keyspace.FamilyBody, 4}, causalFamilyCount{keyspace.FamilyBranch, 1}, causalFamilyCount{keyspace.FamilyFunction, 1}, causalFamilyCount{keyspace.FamilyReturn, 1}, causalFamilyCount{keyspace.FamilyValues, 1}),
		rows:   [][]keyspace.Term{{branch}, nil, nil, {returned}},
		flow: authored.Input{
			Values:    authored.ValuesInput{Rows: []authored.Value{{Owner: functionBody}}},
			Functions: authored.FunctionsInput{Rows: []authored.Function{{Owner: body, Body: functionBody}}},
			Control:   authored.ControlInput{Branches: []authored.Branch{{Owner: body, Condition: condition, WhenTrue: whenTrue, WhenFalse: whenFalse}}, Returns: []authored.Return{{Owner: functionBody, Values: values}}},
		},
	})
	if !f.executable.Executable(condition) {
		t.Fatalf("Branch condition Function is not executable")
	}
	if !f.executable.Executable(branch) {
		t.Fatalf("Branch term is not executable")
	}
	if !f.executable.Executable(whenTrue) || !f.executable.Executable(whenFalse) {
		t.Fatalf("Branch arm liveness = %v/%v", f.executable.Executable(whenTrue), f.executable.Executable(whenFalse))
	}
	if f.forest.Static(branch) || f.forest.Static(whenTrue) || f.forest.Static(whenFalse) {
		t.Fatalf("Branch staticness = %v/%v/%v", f.forest.Static(branch), f.forest.Static(whenTrue), f.forest.Static(whenFalse))
	}
	if finish, ok := f.ports.Finish(condition); !ok || finish != condition {
		t.Fatalf("Branch condition Finish = %v/%v", finish, ok)
	}
	var trueArm, falseArm bool
	for index := 0; index < f.result.Successors().Count(condition); index++ {
		successor, ok := f.result.Successors().At(condition, index)
		if !ok || successor.IsBoundary() || successor.Decision != branch {
			continue
		}
		trueArm = trueArm || (successor.To == whenTrue && successor.Truth)
		falseArm = falseArm || (successor.To == whenFalse && !successor.Truth)
	}
	if !trueArm || !falseArm {
		for index := 0; index < f.result.Successors().Count(condition); index++ {
			successor, ok := f.result.Successors().At(condition, index)
			t.Logf("Branch successor[%d] = %#v/%v", index, successor, ok)
		}
		t.Fatalf("Branch guarded public arms missing: true=%v false=%v count=%d", trueArm, falseArm, f.result.Successors().Count(condition))
	}
}

func TestSemanticMatrixBranchConditionCallResumesThroughBoundary(t *testing.T) {
	parent := causalTerm(keyspace.FamilyBody, 1)
	whenTrue := causalTerm(keyspace.FamilyBody, 2)
	whenFalse := causalTerm(keyspace.FamilyBody, 3)
	branch := causalTerm(keyspace.FamilyBranch, 1)
	call := causalTerm(keyspace.FamilyCall, 1)
	actuals := causalTerm(keyspace.FamilyValues, 1)
	nilValue := causalTerm(keyspace.FamilyNil, 1)
	f := openCausalFixture(t, causalSpec{
		counts: causalCounts(
			causalFamilyCount{keyspace.FamilyBody, 3},
			causalFamilyCount{keyspace.FamilyBranch, 1},
			causalFamilyCount{keyspace.FamilyCall, 1},
			causalFamilyCount{keyspace.FamilyValues, 1},
			causalFamilyCount{keyspace.FamilyNil, 1},
		),
		rows:      [][]keyspace.Term{{branch}, nil, nil},
		nilOwners: []keyspace.Term{parent},
		flow: authored.Input{
			Values: authored.ValuesInput{Rows: []authored.Value{{Owner: parent}}},
			Calls:  []authored.Call{{Owner: parent, Callee: nilValue, Actuals: actuals}},
			Control: authored.ControlInput{Branches: []authored.Branch{{
				Owner: parent, Condition: call, WhenTrue: whenTrue, WhenFalse: whenFalse,
			}}},
		},
	})
	boundary, ok := f.result.Boundaries().For(call)
	if !ok || boundary.Normal != branch || boundary.Other != 0 || boundary.TailReturn != 0 {
		t.Fatalf("Branch condition Call boundary = %#v/%v", boundary, ok)
	}
	if got := f.result.Successors().Count(call); got != 4 {
		t.Fatalf("Branch condition Call successor denominator = %d, want 4", got)
	}
	seenTrue, seenFalse := false, false
	for index := 0; index < f.result.Successors().Count(branch); index++ {
		successor, ok := f.result.Successors().At(branch, index)
		if !ok || successor.IsBoundary() || successor.Decision != branch {
			continue
		}
		seenTrue = seenTrue || (successor.To == whenTrue && successor.Truth)
		seenFalse = seenFalse || (successor.To == whenFalse && !successor.Truth)
	}
	if !seenTrue || !seenFalse {
		t.Fatalf("Branch condition Call guarded routes = true %v false %v", seenTrue, seenFalse)
	}
	for index := 0; index < f.result.Edges().Count(); index++ {
		edge, ok := f.result.Edges().At(index)
		if ok && edge.From == call {
			t.Fatalf("Branch condition Call leaked local Edge: %#v", edge)
		}
	}
}

func TestSemanticMatrixLoopConditionCallResumesThroughBoundary(t *testing.T) {
	parent := causalTerm(keyspace.FamilyBody, 1)
	loopBody := causalTerm(keyspace.FamilyBody, 2)
	loop := causalTerm(keyspace.FamilyLoop, 1)
	call := causalTerm(keyspace.FamilyCall, 1)
	actuals := causalTerm(keyspace.FamilyValues, 1)
	nilValue := causalTerm(keyspace.FamilyNil, 1)
	f := openCausalFixture(t, causalSpec{
		counts: causalCounts(
			causalFamilyCount{keyspace.FamilyBody, 2},
			causalFamilyCount{keyspace.FamilyLoop, 1},
			causalFamilyCount{keyspace.FamilyCall, 1},
			causalFamilyCount{keyspace.FamilyValues, 1},
			causalFamilyCount{keyspace.FamilyNil, 1},
		),
		rows:      [][]keyspace.Term{{loop}, nil},
		nilOwners: []keyspace.Term{parent},
		flow: authored.Input{
			Values: authored.ValuesInput{Rows: []authored.Value{{Owner: parent}}},
			Calls:  []authored.Call{{Owner: parent, Callee: nilValue, Actuals: actuals}},
			Control: authored.ControlInput{Loops: []authored.Loop{{
				Owner: parent, Body: loopBody, Kind: kind.LoopWhile, Control: call,
			}}},
		},
	})
	boundary, ok := f.result.Boundaries().For(call)
	if !ok || boundary.Normal != loop || boundary.Other != 0 || boundary.TailReturn != 0 {
		t.Fatalf("Loop condition Call boundary = %#v/%v", boundary, ok)
	}
	if got := f.result.Successors().Count(call); got != 4 {
		t.Fatalf("Loop condition Call successor denominator = %d, want 4", got)
	}
	seenTrue, seenFalse := false, false
	for index := 0; index < f.result.Successors().Count(loop); index++ {
		successor, ok := f.result.Successors().At(loop, index)
		if !ok || successor.IsBoundary() || successor.Decision != loop {
			continue
		}
		seenTrue = seenTrue || successor.Truth
		seenFalse = seenFalse || !successor.Truth
	}
	if !seenTrue || !seenFalse {
		t.Fatalf("Loop condition Call guarded routes = true %v false %v", seenTrue, seenFalse)
	}
	for index := 0; index < f.result.Edges().Count(); index++ {
		edge, ok := f.result.Edges().At(index)
		if ok && edge.From == call {
			t.Fatalf("Loop condition Call leaked local Edge: %#v", edge)
		}
	}
}

func TestSemanticMatrixNestedCalleeCallUsesOneBoundaryChain(t *testing.T) {
	body := causalTerm(keyspace.FamilyBody, 1)
	outer := causalTerm(keyspace.FamilyCall, 1)
	inner := causalTerm(keyspace.FamilyCall, 2)
	outerValues := causalTerm(keyspace.FamilyValues, 1)
	innerValues := causalTerm(keyspace.FamilyValues, 2)
	nilValue := causalTerm(keyspace.FamilyNil, 1)
	f := openCausalFixture(t, causalSpec{
		counts: causalCounts(
			causalFamilyCount{keyspace.FamilyBody, 1},
			causalFamilyCount{keyspace.FamilyCall, 2},
			causalFamilyCount{keyspace.FamilyValues, 2},
			causalFamilyCount{keyspace.FamilyNil, 1},
		),
		rows:      [][]keyspace.Term{{outer}},
		nilOwners: []keyspace.Term{body},
		flow: authored.Input{
			Values: authored.ValuesInput{Rows: []authored.Value{{Owner: body}, {Owner: body}}},
			Calls: []authored.Call{
				{Owner: body, Callee: inner, Actuals: outerValues},
				{Owner: body, Callee: nilValue, Actuals: innerValues},
			},
		},
	})
	innerBoundary, innerOK := f.result.Boundaries().For(inner)
	outerBoundary, outerOK := f.result.Boundaries().For(outer)
	if !innerOK || !outerOK || innerBoundary.Normal != outerValues || outerBoundary.Normal == 0 {
		t.Fatalf("nested callee boundaries = inner %#v/%v outer %#v/%v", innerBoundary, innerOK, outerBoundary, outerOK)
	}
	if got := f.result.Successors().Count(inner); got != 4 {
		t.Fatalf("nested callee inner successor denominator = %d, want 4", got)
	}
	for index := 0; index < f.result.Edges().Count(); index++ {
		edge, ok := f.result.Edges().At(index)
		if ok && (edge.From == inner || edge.From == outer) {
			t.Fatalf("nested callee leaked local Call Edge: %#v", edge)
		}
	}
}

func TestSemanticMatrixLoopPublishesGuardedAlternatives(t *testing.T) {
	body, loopBody, functionBody := causalTerm(keyspace.FamilyBody, 1), causalTerm(keyspace.FamilyBody, 2), causalTerm(keyspace.FamilyBody, 3)
	loop := causalTerm(keyspace.FamilyLoop, 1)
	condition := causalTerm(keyspace.FamilyFunction, 1)
	returned := causalTerm(keyspace.FamilyReturn, 1)
	values := causalTerm(keyspace.FamilyValues, 1)
	f := openCausalFixture(t, causalSpec{
		counts: causalCounts(causalFamilyCount{keyspace.FamilyBody, 3}, causalFamilyCount{keyspace.FamilyLoop, 1}, causalFamilyCount{keyspace.FamilyFunction, 1}, causalFamilyCount{keyspace.FamilyReturn, 1}, causalFamilyCount{keyspace.FamilyValues, 1}),
		rows:   [][]keyspace.Term{{loop}, nil, {returned}},
		flow: authored.Input{
			Values:    authored.ValuesInput{Rows: []authored.Value{{Owner: functionBody}}},
			Functions: authored.FunctionsInput{Rows: []authored.Function{{Owner: body, Body: functionBody}}},
			Control:   authored.ControlInput{Loops: []authored.Loop{{Owner: body, Body: loopBody, Kind: kind.LoopWhile, Control: condition}}, Returns: []authored.Return{{Owner: functionBody, Values: values}}},
		},
	})
	if !f.executable.Executable(condition) {
		t.Fatalf("Loop condition Function is not executable")
	}
	if finish, ok := f.ports.Finish(condition); !ok || finish != condition {
		t.Fatalf("Loop condition Finish = %v/%v", finish, ok)
	}
	var trueArm, falseArm bool
	for index := 0; index < f.result.Successors().Count(condition); index++ {
		successor, ok := f.result.Successors().At(condition, index)
		if !ok || successor.IsBoundary() || successor.Decision != loop {
			continue
		}
		trueArm = trueArm || successor.Truth
		falseArm = falseArm || !successor.Truth
	}
	if !trueArm || !falseArm {
		for index := 0; index < f.result.Successors().Count(condition); index++ {
			successor, ok := f.result.Successors().At(condition, index)
			t.Logf("Loop successor[%d] = %#v/%v", index, successor, ok)
		}
		t.Fatalf("Loop guarded public arms missing: true=%v false=%v count=%d", trueArm, falseArm, f.result.Successors().Count(condition))
	}
	seenMu := false
	for index := 0; index < f.result.Edges().Count(); index++ {
		if mu, ok := f.result.Edges().Mu(index); ok && mu == loop {
			seenMu = true
			if count, countOK := f.result.Edges().ResetCount(index); !countOK || count < 0 {
				t.Fatalf("Loop Mu reset %d = %d/%v", index, count, countOK)
			}
		}
	}
	if !seenMu {
		t.Fatal("real Loop fixture did not publish a Mu edge")
	}
}

func TestSemanticMatrixAllLoopKindsPublishTypedRoutes(t *testing.T) {
	cases := []struct {
		name string
		kind kind.LoopKind
	}{
		{name: "while", kind: kind.LoopWhile},
		{name: "repeat", kind: kind.LoopRepeat},
		{name: "numeric", kind: kind.LoopNumericFor},
		{name: "generic", kind: kind.LoopGenericFor},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			f := openCausalFixture(t, loopKindSpec(testCase.kind))
			loop := causalTerm(keyspace.FamilyLoop, 1)
			if !f.executable.Executable(loop) {
				t.Fatalf("%s Loop is not executable", testCase.name)
			}
			seenDecision := false
			seenMu := false
			for index := 0; index < f.result.Edges().Count(); index++ {
				edge, ok := f.result.Edges().At(index)
				if !ok {
					t.Fatalf("Edges.At(%d) failed", index)
				}
				if edge.Decision == loop {
					seenDecision = true
				}
				if mu, muOK := f.result.Edges().Mu(index); muOK && mu == loop {
					seenMu = true
					if reset, resetOK := f.result.Edges().ResetCount(index); !resetOK || reset < 0 {
						t.Fatalf("%s Loop reset = %d/%v", testCase.name, reset, resetOK)
					}
				}
			}
			if !seenDecision {
				t.Fatalf("%s Loop did not publish guarded route", testCase.name)
			}
			if !seenMu {
				t.Fatalf("%s Loop did not publish exact Mu route", testCase.name)
			}
		})
	}
}

// Numeric and generic-for headers are evaluated on activation ingress.  The
// iteration route must return to the Loop anchor rather than Ports.Entry(loop)
// (the first header operand), otherwise a one-shot header Call is re-entered on
// every iteration.
func TestSemanticMatrixNumericHeaderCallUsesIngressThenLoopAnchor(t *testing.T) {
	loop := causalTerm(keyspace.FamilyLoop, 1)
	header := causalTerm(keyspace.FamilyValues, 1)
	call := causalTerm(keyspace.FamilyCall, 1)
	last := causalTerm(keyspace.FamilyNil, 3)
	f := openCausalFixture(t, numericGenericHeaderCallSpec(kind.LoopNumericFor))

	boundary, ok := f.result.Boundaries().For(call)
	if !ok || boundary.Call != call {
		t.Fatalf("numeric header Call boundary = %#v/%v", boundary, ok)
	}
	first, firstOK := f.ports.Entry(loop)
	if !firstOK || first == loop {
		t.Fatalf("numeric Loop Entry = %v/%v, want one-shot header endpoint", first, firstOK)
	}
	if boundary.Normal != last {
		t.Fatalf("numeric header Call normal = %v, want next header endpoint %v", boundary.Normal, last)
	}

	// The last header endpoint is the only local seed into the iteration
	// anchor.  No Body Normal route may point back to the first header Entry.
	lastEndpoint, lastOK := f.ports.Finish(header)
	if !lastOK || !hasLocalEdge(f.result, lastEndpoint, loop) {
		t.Fatalf("numeric last header endpoint %v/%v does not route to Loop anchor %v", lastEndpoint, lastOK, loop)
	}
	if !hasLocalEdge(f.result, causalTerm(keyspace.FamilyBody, 1), first) {
		t.Fatalf("numeric external Body ingress does not route through one-shot header endpoint %v", first)
	}
	normal, normalOK := f.outcomes.BodyExit(causalTerm(keyspace.FamilyBody, 2), kind.OutcomeNormal)
	if !normalOK || !hasLocalEdge(f.result, normal, loop) {
		t.Fatalf("numeric Body Normal does not route to Loop anchor: %v/%v -> %v", normal, normalOK, loop)
	}
	if hasLocalEdge(f.result, normal, first) {
		t.Fatalf("numeric Body Normal re-enters one-shot header Entry %v", first)
	}
	assertLoopResetRows(t, f, loop)
}

func TestSemanticMatrixGenericHeaderCallUsesIngressThenLoopAnchor(t *testing.T) {
	loop := causalTerm(keyspace.FamilyLoop, 1)
	header := causalTerm(keyspace.FamilyValues, 1)
	call := causalTerm(keyspace.FamilyCall, 1)
	f := openCausalFixture(t, numericGenericHeaderCallSpec(kind.LoopGenericFor))

	boundary, ok := f.result.Boundaries().For(call)
	if !ok || boundary.Call != call {
		t.Fatalf("generic header Call boundary = %#v/%v", boundary, ok)
	}
	first, firstOK := f.ports.Entry(loop)
	if !firstOK || first == loop {
		t.Fatalf("generic Loop Entry = %v/%v, want one-shot header endpoint", first, firstOK)
	}
	if boundary.Normal != header {
		t.Fatalf("generic header Call normal = %v, want header Values %v", boundary.Normal, header)
	}
	lastEndpoint, lastOK := f.ports.Finish(header)
	if !lastOK || !hasLocalEdge(f.result, lastEndpoint, loop) {
		t.Fatalf("generic last header endpoint %v/%v does not route to Loop anchor %v", lastEndpoint, lastOK, loop)
	}
	if f.result.Successors().Count(call) == 0 || !hasBoundaryTo(f.result, call, header) {
		t.Fatalf("generic header Call has no normal boundary route to header Values")
	}
	if !hasLocalEdge(f.result, causalTerm(keyspace.FamilyBody, 1), first) {
		t.Fatalf("generic external Body ingress does not route through one-shot header endpoint %v", first)
	}
	normal, normalOK := f.outcomes.BodyExit(causalTerm(keyspace.FamilyBody, 2), kind.OutcomeNormal)
	if !normalOK || !hasLocalEdge(f.result, normal, loop) {
		t.Fatalf("generic Body Normal does not route to Loop anchor: %v/%v -> %v", normal, normalOK, loop)
	}
	if hasLocalEdge(f.result, normal, first) {
		t.Fatalf("generic Body Normal re-enters one-shot header Entry %v", first)
	}
	assertLoopResetRows(t, f, loop)
}

func numericGenericHeaderCallSpec(loopKind kind.LoopKind) causalSpec {
	parent := causalTerm(keyspace.FamilyBody, 1)
	child := causalTerm(keyspace.FamilyBody, 2)
	loop := causalTerm(keyspace.FamilyLoop, 1)
	header := causalTerm(keyspace.FamilyValues, 1)
	call := causalTerm(keyspace.FamilyCall, 1)
	callValues := causalTerm(keyspace.FamilyValues, 2)
	callee := causalTerm(keyspace.FamilyNil, 1)
	actual := causalTerm(keyspace.FamilyNil, 2)
	last := causalTerm(keyspace.FamilyNil, 3)
	spec := causalSpec{
		counts: causalCounts(
			causalFamilyCount{keyspace.FamilyBody, 2},
			causalFamilyCount{keyspace.FamilyLoop, 1},
			causalFamilyCount{keyspace.FamilyCall, 1},
			causalFamilyCount{keyspace.FamilyValues, 2},
			causalFamilyCount{keyspace.FamilyNil, 3},
			causalFamilyCount{keyspace.FamilyCell, 1},
		),
		rows:      [][]keyspace.Term{{loop}, nil},
		nilOwners: []keyspace.Term{parent, parent, parent},
		flow: authored.Input{
			Values: authored.ValuesInput{
				Rows:  []authored.Value{{Owner: parent, Fixed: authored.Range{Start: 0, End: 2}}, {Owner: parent, Fixed: authored.Range{Start: 2, End: 3}}},
				Terms: []keyspace.Term{call, last, actual},
			},
			Calls:   []authored.Call{{Owner: parent, Callee: callee, Actuals: callValues}},
			Storage: authored.StorageInput{Cells: []authored.Cell{{Kind: authored.CellLocal, Body: child}}},
			Control: authored.ControlInput{
				Cells: []keyspace.Term{causalTerm(keyspace.FamilyCell, 1)},
				Loops: []authored.Loop{{Owner: parent, Body: child, Kind: loopKind, Control: header, Cells: authored.Range{Start: 0, End: 1}}},
			},
		},
	}
	if loopKind == kind.LoopGenericFor {
		// Generic headers have one control operand in the authored contract.
		spec.counts[keyspace.FamilyValues] = 2
		spec.counts[keyspace.FamilyNil] = 2
		spec.nilOwners = []keyspace.Term{parent, parent}
		spec.flow.Values = authored.ValuesInput{
			Rows:  []authored.Value{{Owner: parent, Fixed: authored.Range{Start: 0, End: 1}}, {Owner: parent, Fixed: authored.Range{Start: 1, End: 2}}},
			Terms: []keyspace.Term{call, actual},
		}
		spec.flow.Calls = []authored.Call{{Owner: parent, Callee: causalTerm(keyspace.FamilyNil, 1), Actuals: causalTerm(keyspace.FamilyValues, 2)}}
	}
	return spec
}

func hasLocalEdge(result *Result, from, to keyspace.Term) bool {
	for index := 0; index < result.Edges().Count(); index++ {
		edge, ok := result.Edges().At(index)
		if ok && edge.From == from && edge.To == to {
			return true
		}
	}
	return false
}

func hasBoundaryTo(result *Result, call, to keyspace.Term) bool {
	for index := 0; index < result.Successors().Count(call); index++ {
		successor, ok := result.Successors().At(call, index)
		if ok && successor.IsBoundary() && successor.To == to {
			return true
		}
	}
	return false
}

func assertLoopResetRows(t *testing.T, fixture *causalFixture, loop keyspace.Term) {
	t.Helper()
	seen := false
	for index := 0; index < fixture.result.Edges().Count(); index++ {
		mu, ok := fixture.result.Edges().Mu(index)
		if !ok || mu != loop {
			continue
		}
		seen = true
		count, countOK := fixture.result.Edges().ResetCount(index)
		if !countOK || count < 0 {
			t.Fatalf("Loop reset row %d = %d/%v", index, count, countOK)
		}
		for offset := 0; offset < count; offset++ {
			decision, decisionOK := fixture.result.Edges().ResetAt(index, offset)
			if !decisionOK || !fixture.result.Edges().ResetContains(index, decision) {
				t.Fatalf("Loop reset row %d member %d = %v/%v lacks membership", index, offset, decision, decisionOK)
			}
		}
	}
	if !seen {
		t.Fatalf("Loop %v has no recurrence Mu row", loop)
	}
}

func loopKindSpec(loopKind kind.LoopKind) causalSpec {
	parent := causalTerm(keyspace.FamilyBody, 1)
	child := causalTerm(keyspace.FamilyBody, 2)
	loop := causalTerm(keyspace.FamilyLoop, 1)
	condition := causalTerm(keyspace.FamilyNil, 1)
	spec := causalSpec{
		counts: causalCounts(
			causalFamilyCount{keyspace.FamilyBody, 2},
			causalFamilyCount{keyspace.FamilyLoop, 1},
			causalFamilyCount{keyspace.FamilyNil, 1},
		),
		rows:      [][]keyspace.Term{{loop}, nil},
		nilOwners: []keyspace.Term{parent},
	}
	if loopKind == kind.LoopRepeat {
		// Repeat evaluates its condition after the child Body, so the
		// condition literal belongs to that Body's lexical scope.
		spec.nilOwners[0] = child
	}
	spec.flow.Control.Loops = []authored.Loop{{Owner: parent, Body: child, Kind: loopKind, Control: condition}}
	spec.flow.Control.Cells = nil
	if loopKind == kind.LoopNumericFor {
		values := causalTerm(keyspace.FamilyValues, 1)
		spec.counts[keyspace.FamilyValues] = 1
		spec.counts[keyspace.FamilyNil] = 2
		spec.nilOwners = []keyspace.Term{parent, parent}
		spec.flow.Values = authored.ValuesInput{
			Rows:  []authored.Value{{Owner: parent, Fixed: authored.Range{End: 2}}},
			Terms: []keyspace.Term{condition, causalTerm(keyspace.FamilyNil, 2)},
		}
		spec.flow.Control.Loops[0].Control = values
		spec.counts[keyspace.FamilyCell] = 1
		cell := causalTerm(keyspace.FamilyCell, 1)
		spec.flow.Storage.Cells = []authored.Cell{{Kind: authored.CellLocal, Body: child}}
		spec.flow.Control.Cells = []keyspace.Term{cell}
		spec.flow.Control.Loops[0].Cells = authored.Range{End: 1}
	} else if loopKind == kind.LoopGenericFor {
		values := causalTerm(keyspace.FamilyValues, 1)
		spec.counts[keyspace.FamilyValues] = 1
		spec.flow.Values = authored.ValuesInput{
			Rows:  []authored.Value{{Owner: parent, Fixed: authored.Range{End: 1}}},
			Terms: []keyspace.Term{condition},
		}
		spec.flow.Control.Loops[0].Control = values
		spec.counts[keyspace.FamilyCell] = 1
		cell := causalTerm(keyspace.FamilyCell, 1)
		spec.flow.Storage.Cells = []authored.Cell{{Kind: authored.CellLocal, Body: child}}
		spec.flow.Control.Cells = []keyspace.Term{cell}
		spec.flow.Control.Loops[0].Cells = authored.Range{End: 1}
	}
	return spec
}

func TestSemanticMatrixBreakOutcomePropagatesThroughLoopResume(t *testing.T) {
	body, loop, child := causalTerm(keyspace.FamilyBody, 1), causalTerm(keyspace.FamilyLoop, 1), causalTerm(keyspace.FamilyBody, 2)
	breakTerm := causalTerm(keyspace.FamilyBreak, 1)
	nilValue := causalTerm(keyspace.FamilyNil, 1)
	f := openCausalFixture(t, causalSpec{
		counts: causalCounts(
			causalFamilyCount{keyspace.FamilyBody, 2},
			causalFamilyCount{keyspace.FamilyLoop, 1},
			causalFamilyCount{keyspace.FamilyBreak, 1},
			causalFamilyCount{keyspace.FamilyNil, 1},
		),
		rows:      [][]keyspace.Term{{loop}, {breakTerm}},
		nilOwners: []keyspace.Term{body},
		flow: authored.Input{Control: authored.ControlInput{
			Breaks: []authored.Break{{Owner: child}},
			Loops:  []authored.Loop{{Owner: body, Body: child, Kind: kind.LoopWhile, Control: nilValue}},
		}},
	})

	breakExit, ok := f.outcomes.BreakExit(breakTerm)
	if !ok {
		t.Fatal("Break Outcome is absent")
	}
	if got := f.result.Successors().Count(breakTerm); got == 0 {
		t.Fatal("Break has no local Outcome edge")
	}
	var breakRoute, resumeRoute bool
	for index := 0; index < f.result.Successors().Count(breakTerm); index++ {
		successor, ok := f.result.Successors().At(breakTerm, index)
		if !ok || successor.IsBoundary() || successor.From != breakTerm {
			continue
		}
		breakRoute = breakRoute || successor.To == breakExit
	}
	if !breakRoute {
		t.Fatalf("Break did not route to its exact Outcome %v", breakExit)
	}
	normalExit, normalOK := f.outcomes.BodyExit(body, kind.OutcomeNormal)
	if !normalOK {
		t.Fatal("Break target Body Normal Outcome is absent")
	}
	for index := 0; index < f.result.Successors().Count(breakExit); index++ {
		successor, ok := f.result.Successors().At(breakExit, index)
		if ok && !successor.IsBoundary() && successor.To == normalExit && keyspace.TermFamily(successor.To) == keyspace.FamilyOutcome {
			resumeRoute = true
		}
	}
	if !resumeRoute {
		t.Fatalf("Break Outcome did not propagate to Body Normal Outcome %v", normalExit)
	}
}

func TestSemanticMatrixBackwardGotoCarriesLoopReset(t *testing.T) {
	parent := causalTerm(keyspace.FamilyBody, 1)
	loopBody := causalTerm(keyspace.FamilyBody, 2)
	loop := causalTerm(keyspace.FamilyLoop, 1)
	gotoTerm := causalTerm(keyspace.FamilyGoto, 1)
	label := causalTerm(keyspace.FamilyLabel, 1)
	condition := causalTerm(keyspace.FamilyNil, 1)
	f := openCausalFixture(t, causalSpec{
		counts: causalCounts(
			causalFamilyCount{keyspace.FamilyBody, 2},
			causalFamilyCount{keyspace.FamilyLoop, 1},
			causalFamilyCount{keyspace.FamilyGoto, 1},
			causalFamilyCount{keyspace.FamilyLabel, 1},
			causalFamilyCount{keyspace.FamilyNil, 1},
		),
		rows:      [][]keyspace.Term{{loop}, {gotoTerm, label}},
		nilOwners: []keyspace.Term{parent},
		flow: authored.Input{Control: authored.ControlInput{
			Labels: []authored.Label{{Owner: loopBody}},
			Gotos:  []authored.Goto{{Owner: loopBody, Target: label}},
			Loops:  []authored.Loop{{Owner: parent, Body: loopBody, Kind: kind.LoopWhile, Control: condition}},
		}},
	})

	if got := f.result.Successors().Count(gotoTerm); got == 0 {
		t.Fatal("backward Goto has no causal successors")
	}
	seenMu := false
	for index := 0; index < f.result.Edges().Count(); index++ {
		mu, ok := f.result.Edges().Mu(index)
		if !ok || mu != label {
			continue
		}
		seenMu = true
		if reset, resetOK := f.result.Edges().ResetCount(index); !resetOK || reset < 0 {
			t.Fatalf("backward Goto Mu reset = %d/%v", reset, resetOK)
		}
	}
	if !seenMu {
		t.Fatal("backward Goto did not retain exact Label Mu")
	}
	resume, ok := f.control.Resume(label)
	if !ok {
		t.Fatal("backward Goto Label resume is absent")
	}
	if f.result.Successors().Count(resume) == 0 {
		t.Fatalf("backward Goto Label resume %v has no causal route", resume)
	}
	normal, normalOK := f.outcomes.BodyExit(loopBody, kind.OutcomeNormal)
	if !normalOK {
		t.Fatal("nested Body Normal Outcome is absent")
	}
	foundNormal := false
	for index := 0; index < f.result.Successors().Count(gotoTerm); index++ {
		successor, successorOK := f.result.Successors().At(gotoTerm, index)
		if successorOK && !successor.IsBoundary() && successor.To == normal {
			foundNormal = true
		}
		if successorOK && !successor.IsBoundary() && successor.To == loopBody {
			t.Fatalf("backward Goto Outcome re-entered raw nested Body anchor: %#v", successor)
		}
	}
	if !foundNormal {
		t.Fatalf("backward Goto did not target nested Body Normal Outcome %v", normal)
	}
	gotoArcOrdinal := -1
	for index := 0; index < f.control.ArcCount(); index++ {
		arc, arcOK := f.control.ArcAt(index)
		if arcOK && arc.Source == gotoTerm && arc.Target == label && arc.Decision == 0 && !arc.Truth {
			gotoArcOrdinal = index
			break
		}
	}
	if gotoArcOrdinal < 0 {
		t.Fatal("backward Goto sourcecontrol Arc is absent")
	}
	gotoAnnotation, annotationOK := f.recurrence.ArcAt(gotoArcOrdinal)
	if !annotationOK || gotoAnnotation.Head != 0 {
		t.Fatalf("backward Goto recurrence annotation = %#v/%v, want terminal non-Mu Arc", gotoAnnotation, annotationOK)
	}
	gotoEdge := -1
	for index := 0; index < f.result.Edges().Count(); index++ {
		edge, edgeOK := f.result.Edges().At(index)
		if edgeOK && edge.From == gotoTerm && edge.Mu == 0 {
			gotoEdge = index
			break
		}
	}
	if gotoEdge < 0 {
		t.Fatalf("backward Goto causal edge is absent: %v", gotoTerm)
	}
	if _, ok := f.result.Edges().ResetCount(gotoEdge); ok {
		t.Fatal("non-recurrent Goto edge exposed reset membership")
	}
	loopArcOrdinal := -1
	for index := 0; index < f.control.ArcCount(); index++ {
		arc, arcOK := f.control.ArcAt(index)
		if arcOK && arc.Source == loopBody && arc.Target == loop && arc.Decision == 0 && !arc.Truth {
			annotation, annotationOK := f.recurrence.ArcAt(index)
			if annotationOK && annotation.Head == label {
				loopArcOrdinal = index
				break
			}
		}
	}
	if loopArcOrdinal < 0 {
		t.Fatal("backward loop recurrence Arc is absent")
	}
	loopAnnotation, annotationOK := f.recurrence.ArcAt(loopArcOrdinal)
	if !annotationOK || loopAnnotation.Head != label {
		t.Fatalf("backward loop recurrence annotation = %#v/%v", loopAnnotation, annotationOK)
	}
	conditionEntry, conditionEntryOK := f.ports.Entry(condition)
	if !conditionEntryOK {
		t.Fatal("backward loop condition Entry is absent")
	}
	loopEdge := -1
	for index := 0; index < f.result.Edges().Count(); index++ {
		edge, edgeOK := f.result.Edges().At(index)
		if edgeOK && edge.From == normal && edge.To == conditionEntry && edge.Mu == label {
			loopEdge = index
			break
		}
	}
	if loopEdge < 0 {
		t.Fatalf("backward loop causal Mu edge is absent: %v -> %v", normal, conditionEntry)
	}
	wantReset, wantResetOK := f.recurrence.ResetCount(loopArcOrdinal)
	gotReset, gotResetOK := f.result.Edges().ResetCount(loopEdge)
	if !wantResetOK || !gotResetOK || gotReset != wantReset {
		t.Fatalf("backward loop reset count = %d/%v, want recurrence %d/%v", gotReset, gotResetOK, wantReset, wantResetOK)
	}
	for offset := 0; offset < wantReset; offset++ {
		wantDecision, wantDecisionOK := f.recurrence.ResetAt(loopArcOrdinal, offset)
		gotDecision, gotDecisionOK := f.result.Edges().ResetAt(loopEdge, offset)
		if !wantDecisionOK || !gotDecisionOK || gotDecision != wantDecision || !f.result.Edges().ResetContains(loopEdge, gotDecision) {
			t.Fatalf("backward loop reset[%d] = %v/%v, want %v/%v and membership", offset, gotDecision, gotDecisionOK, wantDecision, wantDecisionOK)
		}
	}
}

func TestSemanticMatrixGotoRoutesToSameBodyResume(t *testing.T) {
	body := causalTerm(keyspace.FamilyBody, 1)
	gotoTerm := causalTerm(keyspace.FamilyGoto, 1)
	label := causalTerm(keyspace.FamilyLabel, 1)
	f := openCausalFixture(t, causalSpec{
		counts: causalCounts(
			causalFamilyCount{keyspace.FamilyBody, 1},
			causalFamilyCount{keyspace.FamilyGoto, 1},
			causalFamilyCount{keyspace.FamilyLabel, 1},
		),
		rows: [][]keyspace.Term{{gotoTerm, label}},
		flow: authored.Input{Control: authored.ControlInput{
			Labels: []authored.Label{{Owner: body}},
			Gotos:  []authored.Goto{{Owner: body, Target: label}},
		}},
	})

	gotoExit, ok := f.outcomes.GotoExit(gotoTerm)
	if !ok {
		t.Fatal("Goto Outcome is absent")
	}
	if f.result.Successors().Count(gotoTerm) == 0 {
		t.Fatal("Goto has no local Outcome edge")
	}
	resume, resumeOK := f.control.Resume(label)
	if !resumeOK {
		t.Fatal("same-Body Label resume is absent")
	}
	normal, normalOK := f.outcomes.BodyExit(body, kind.OutcomeNormal)
	if !normalOK {
		t.Fatal("same-Body Label terminal Body Normal Outcome is absent")
	}
	foundResume := false
	for index := 0; index < f.result.Successors().Count(gotoTerm); index++ {
		successor, ok := f.result.Successors().At(gotoTerm, index)
		if !ok || successor.IsBoundary() {
			continue
		}
		foundResume = foundResume || successor.To == normal && keyspace.TermFamily(successor.To) == keyspace.FamilyOutcome
	}
	if !foundResume {
		t.Fatalf("same-Body Goto did not route to Body Normal Outcome %v (Resume anchor %v, typed exit %v)", normal, resume, gotoExit)
	}
}

func TestSemanticMatrixSelfGotoCarriesEmptyLabelReset(t *testing.T) {
	body := causalTerm(keyspace.FamilyBody, 1)
	gotoTerm := causalTerm(keyspace.FamilyGoto, 1)
	label := causalTerm(keyspace.FamilyLabel, 1)
	f := openCausalFixture(t, causalSpec{
		counts: causalCounts(
			causalFamilyCount{keyspace.FamilyBody, 1},
			causalFamilyCount{keyspace.FamilyGoto, 1},
			causalFamilyCount{keyspace.FamilyLabel, 1},
		),
		rows: [][]keyspace.Term{{label, gotoTerm}},
		flow: authored.Input{Control: authored.ControlInput{
			Labels: []authored.Label{{Owner: body}},
			Gotos:  []authored.Goto{{Owner: body, Target: label}},
		}},
	})

	gotoEdges, labelEndpoints := 0, 0
	for index := 0; index < f.result.Edges().Count(); index++ {
		edge, ok := f.result.Edges().At(index)
		if !ok {
			t.Fatalf("local Edge %d is unavailable", index)
		}
		if edge.From == label || edge.To == label {
			labelEndpoints++
		}
		if edge.From != gotoTerm {
			continue
		}
		gotoEdges++
		if edge.To != gotoTerm || edge.Mu != label {
			t.Fatalf("self Goto Edge = %#v, want Goto -> Goto with Label Mu", edge)
		}
		if reset, resetOK := f.result.Edges().ResetCount(index); !resetOK || reset != 0 {
			t.Fatalf("self Goto empty reset = %d/%v, want 0/true", reset, resetOK)
		}
	}
	if gotoEdges != 1 {
		t.Fatalf("self Goto local Edge count = %d, want 1", gotoEdges)
	}
	if labelEndpoints != 0 {
		t.Fatalf("self Goto retained %d local Label endpoints, want none", labelEndpoints)
	}
}

func TestSemanticMatrixOutwardGotoPropagatesThroughTypedOutcome(t *testing.T) {
	parent, child := causalTerm(keyspace.FamilyBody, 1), causalTerm(keyspace.FamilyBody, 2)
	loop := causalTerm(keyspace.FamilyLoop, 1)
	gotoTerm := causalTerm(keyspace.FamilyGoto, 1)
	label := causalTerm(keyspace.FamilyLabel, 1)
	nilValue := causalTerm(keyspace.FamilyNil, 1)
	f := openCausalFixture(t, causalSpec{
		counts: causalCounts(
			causalFamilyCount{keyspace.FamilyBody, 2},
			causalFamilyCount{keyspace.FamilyLoop, 1},
			causalFamilyCount{keyspace.FamilyGoto, 1},
			causalFamilyCount{keyspace.FamilyLabel, 1},
			causalFamilyCount{keyspace.FamilyNil, 1},
		),
		rows:      [][]keyspace.Term{{loop, label}, {gotoTerm}},
		nilOwners: []keyspace.Term{parent},
		flow: authored.Input{Control: authored.ControlInput{
			Labels: []authored.Label{{Owner: parent}},
			Gotos:  []authored.Goto{{Owner: child, Target: label}},
			Loops:  []authored.Loop{{Owner: parent, Body: child, Kind: kind.LoopWhile, Control: nilValue}},
		}},
	})

	gotoExit, ok := f.outcomes.GotoExit(gotoTerm)
	if !ok || keyspace.TermFamily(gotoExit) != keyspace.FamilyOutcome {
		t.Fatalf("outward Goto Exit = %v/%v, want typed Outcome", gotoExit, ok)
	}
	owner, outcomeKind, target, ok := f.outcomes.Get(gotoExit)
	if !ok || owner != child || outcomeKind != kind.OutcomeGoto || target != label {
		t.Fatalf("Goto Outcome = %v/%v/%v/%v", owner, outcomeKind, target, ok)
	}
	if got := f.result.Successors().Count(gotoTerm); got != 1 {
		t.Fatalf("outward Goto successor denominator = %d, want 1", got)
	}
	successor, ok := f.result.Successors().At(gotoTerm, 0)
	if !ok || successor.IsBoundary() || successor.To != gotoExit {
		t.Fatalf("outward Goto successor = %#v/%v, want exact Outcome %v", successor, ok, gotoExit)
	}
	if got := f.result.Successors().Count(gotoExit); got == 0 {
		t.Fatal("Goto Outcome did not propagate to Label resume")
	}
	for index := 0; index < f.result.Successors().Count(gotoExit); index++ {
		successor, ok := f.result.Successors().At(gotoExit, index)
		if ok && !successor.IsBoundary() && keyspace.TermFamily(successor.To) == keyspace.FamilyBody {
			t.Fatalf("Goto Outcome re-entered raw Body anchor: %#v", successor)
		}
	}
}

func TestSemanticMatrixForeignProvenanceFailsClosed(t *testing.T) {
	first := provenanceCallSpec("causal-provenance-a.lua", false)
	second := provenanceCallSpec("causal-provenance-b.lua", true)
	left := openCausalFixture(t, first)
	right := openCausalFixture(t, second)
	if Matches(left.result, right.sourceView.Identity().ContentID(), left.flow.Cold().ContentID(), left.staticFinalize.View().ContentID(), left.moduleFinalize.View().ContentID()) {
		t.Fatal("equal-shape foreign Source identity matched causal Result")
	}
	if _, err := Seal(right.sourceView, left.flow, left.bodies, left.forest, left.outcomes, left.control, left.recurrence, left.ports, left.executable, left.staticFinalize.View().ContentID(), left.moduleFinalize.View().ContentID()); err == nil {
		t.Fatal("Seal accepted a foreign Source against typed prerequisites")
	}
	if _, err := Seal(left.sourceView, right.flow, left.bodies, left.forest, left.outcomes, left.control, left.recurrence, left.ports, left.executable, left.staticFinalize.View().ContentID(), left.moduleFinalize.View().ContentID()); err == nil {
		t.Fatal("Seal accepted a foreign authored Flow against typed prerequisites")
	}
	foreignStatic := left.staticFinalize.View().ContentID()
	foreignStatic[0]++
	if _, err := Seal(left.sourceView, left.flow, left.bodies, left.forest, left.outcomes, left.control, left.recurrence, left.ports, left.executable, foreignStatic, left.moduleFinalize.View().ContentID()); err == nil {
		t.Fatal("Seal accepted a foreign Static identity")
	}
	foreignModule := left.moduleFinalize.View().ContentID()
	foreignModule[0]++
	if _, err := Seal(left.sourceView, left.flow, left.bodies, left.forest, left.outcomes, left.control, left.recurrence, left.ports, left.executable, left.staticFinalize.View().ContentID(), foreignModule); err == nil {
		t.Fatal("Seal accepted a foreign Module identity")
	}
}

func provenanceCallSpec(name string, swapped bool) causalSpec {
	body := causalTerm(keyspace.FamilyBody, 1)
	values := []keyspace.Term{
		causalTerm(keyspace.FamilyValues, 1),
		causalTerm(keyspace.FamilyValues, 2),
	}
	calls := []keyspace.Term{
		causalTerm(keyspace.FamilyCall, 1),
		causalTerm(keyspace.FamilyCall, 2),
	}
	nilValue := causalTerm(keyspace.FamilyNil, 1)
	boolValue := causalTerm(keyspace.FamilyBool, 1)
	left, right := nilValue, boolValue
	if swapped {
		left, right = right, left
	}
	return causalSpec{
		name: name,
		counts: causalCounts(
			causalFamilyCount{keyspace.FamilyBody, 1},
			causalFamilyCount{keyspace.FamilyCall, 2},
			causalFamilyCount{keyspace.FamilyValues, 2},
			causalFamilyCount{keyspace.FamilyNil, 1},
			causalFamilyCount{keyspace.FamilyBool, 1},
		),
		rows: [][]keyspace.Term{{calls[0], calls[1]}},
		flow: authored.Input{
			Values: authored.ValuesInput{Rows: []authored.Value{
				{Owner: body}, {Owner: body},
			}},
			Calls: []authored.Call{
				{Owner: body, Callee: left, Actuals: values[0]},
				{Owner: body, Callee: right, Actuals: values[1]},
			},
		},
	}
}

func TestSemanticMatrixEverySourceControlArcHasOneDisposition(t *testing.T) {
	f := openCausalFixture(t, directCallSpec("causal-arc-ledger.lua"))
	assertEveryArcDisposition(t, f)
}

func assertEveryArcDisposition(t *testing.T, f *causalFixture) {
	t.Helper()
	state, err := newSealState(f.sourceView, f.flow, f.bodies, f.forest, f.outcomes, f.control, f.recurrence, f.ports, f.executable, f.staticFinalize.View().ContentID(), f.moduleFinalize.View().ContentID())
	if err != nil {
		t.Fatalf("newSealState: %v", err)
	}
	phases := []func() error{state.eval.emitEvaluation, state.structure.emitStructure, state.outcomes.emitOutcomes, state.boundary.emitBoundaries, state.index.finish}
	for index, phase := range phases {
		if err := phase(); err != nil {
			t.Fatalf("causal phase %d: %v", index, err)
		}
	}
	if len(state.arc.arcDisposition) != f.control.ArcCount() {
		t.Fatalf("Arc disposition denominator = %d, want %d", len(state.arc.arcDisposition), f.control.ArcCount())
	}
	for index, disposition := range state.arc.arcDisposition {
		if disposition == arcUndisposed {
			t.Fatalf("sourcecontrol Arc %d remained undisposed", index)
		}
	}
}

func TestSemanticMatrixNestedBranchLoopBreakGotoArcLedger(t *testing.T) {
	parent := causalTerm(keyspace.FamilyBody, 1)
	branchBody := causalTerm(keyspace.FamilyBody, 2)
	loopBody := causalTerm(keyspace.FamilyBody, 3)
	fallthroughBody := causalTerm(keyspace.FamilyBody, 4)
	branch := causalTerm(keyspace.FamilyBranch, 1)
	loop := causalTerm(keyspace.FamilyLoop, 1)
	breakTerm := causalTerm(keyspace.FamilyBreak, 1)
	gotoTerm := causalTerm(keyspace.FamilyGoto, 1)
	label := causalTerm(keyspace.FamilyLabel, 1)
	condition := causalTerm(keyspace.FamilyNil, 1)
	loopCondition := causalTerm(keyspace.FamilyNil, 2)
	f := openCausalFixture(t, causalSpec{
		counts: causalCounts(
			causalFamilyCount{keyspace.FamilyBody, 4},
			causalFamilyCount{keyspace.FamilyBranch, 1},
			causalFamilyCount{keyspace.FamilyLoop, 1},
			causalFamilyCount{keyspace.FamilyBreak, 1},
			causalFamilyCount{keyspace.FamilyGoto, 1},
			causalFamilyCount{keyspace.FamilyLabel, 1},
			causalFamilyCount{keyspace.FamilyNil, 2},
		),
		rows:      [][]keyspace.Term{{branch, loop}, {gotoTerm, label}, {breakTerm}, nil},
		nilOwners: []keyspace.Term{parent, parent},
		flow: authored.Input{Control: authored.ControlInput{
			Branches: []authored.Branch{{Owner: parent, Condition: condition, WhenTrue: branchBody, WhenFalse: fallthroughBody}},
			Loops:    []authored.Loop{{Owner: parent, Body: loopBody, Kind: kind.LoopWhile, Control: loopCondition}},
			Breaks:   []authored.Break{{Owner: loopBody}},
			Gotos:    []authored.Goto{{Owner: branchBody, Target: label}},
			Labels:   []authored.Label{{Owner: branchBody}},
		}},
	})
	if !f.executable.Executable(branch) || !f.executable.Executable(loop) || !f.executable.Executable(breakTerm) || !f.executable.Executable(gotoTerm) {
		t.Fatal("nested control ledger fixture did not retain all live control roots")
	}
	assertEveryArcDisposition(t, f)
}

func TestSemanticMatrixStaticCallTypeSubtreeIsExcludedFromCausalPlane(t *testing.T) {
	body := causalTerm(keyspace.FamilyBody, 1)
	call := causalTerm(keyspace.FamilyCall, 1)
	actuals := causalTerm(keyspace.FamilyValues, 1)
	nilValue := causalTerm(keyspace.FamilyNil, 1)
	primitive := causalTerm(keyspace.FamilyTypePrimitive, 1)
	optional := causalTerm(keyspace.FamilyTypeOptional, 1)
	f := openCausalFixture(t, causalSpec{
		counts: causalCounts(
			causalFamilyCount{keyspace.FamilyBody, 1},
			causalFamilyCount{keyspace.FamilyCall, 1},
			causalFamilyCount{keyspace.FamilyValues, 1},
			causalFamilyCount{keyspace.FamilyNil, 1},
			causalFamilyCount{keyspace.FamilyTypePrimitive, 1},
			causalFamilyCount{keyspace.FamilyTypeOptional, 1},
		),
		rows:      [][]keyspace.Term{{call}},
		nilOwners: []keyspace.Term{body},
		static: static.Input{
			Types: static.TypesInput{
				Primitive: []static.Primitive{{Kind: static.PrimitiveAny}},
				Optional:  []static.Optional{{Inner: primitive}},
			},
			Contracts: static.ContractsInput{Call: []static.CallContract{{TypeArguments: []keyspace.Term{optional}}}},
		},
		flow: authored.Input{
			Values: authored.ValuesInput{Rows: []authored.Value{{Owner: body}}},
			Calls:  []authored.Call{{Owner: body, Callee: nilValue, Actuals: actuals}},
		},
	})
	if !f.forest.Static(optional) || !f.forest.Static(primitive) {
		t.Fatalf("Call-owned static subtree marks = optional %v primitive %v", f.forest.Static(optional), f.forest.Static(primitive))
	}
	if f.executable.Executable(optional) || f.executable.Executable(primitive) {
		t.Fatal("static Call type subtree entered executable closure")
	}
	for index := 0; index < f.result.Edges().Count(); index++ {
		edge, ok := f.result.Edges().At(index)
		if !ok {
			t.Fatalf("Edges.At(%d) failed", index)
		}
		if edge.From == optional || edge.To == optional || edge.From == primitive || edge.To == primitive {
			t.Fatalf("static type subtree leaked into causal Edge[%d] = %#v", index, edge)
		}
	}
	assertEveryArcDisposition(t, f)
}

func directCallSpec(name string) causalSpec {
	body := causalTerm(keyspace.FamilyBody, 1)
	call := causalTerm(keyspace.FamilyCall, 1)
	actuals := causalTerm(keyspace.FamilyValues, 1)
	nilValue := causalTerm(keyspace.FamilyNil, 1)
	return causalSpec{
		name: name,
		counts: causalCounts(
			causalFamilyCount{keyspace.FamilyBody, 1},
			causalFamilyCount{keyspace.FamilyCall, 1},
			causalFamilyCount{keyspace.FamilyValues, 1},
			causalFamilyCount{keyspace.FamilyNil, 1},
		),
		rows:      [][]keyspace.Term{{call}},
		nilOwners: []keyspace.Term{body},
		flow: authored.Input{
			Values: authored.ValuesInput{Rows: []authored.Value{{Owner: body}}},
			Calls:  []authored.Call{{Owner: body, Callee: nilValue, Actuals: actuals}},
		},
	}
}

func TestSemanticMatrixTableRecordListAndSpreadRetainTypedFieldClosure(t *testing.T) {
	body := causalTerm(keyspace.FamilyBody, 1)
	returned := causalTerm(keyspace.FamilyReturn, 1)
	table := causalTerm(keyspace.FamilyTable, 1)
	fields := []keyspace.Term{
		causalTerm(keyspace.FamilyTableField, 1),
		causalTerm(keyspace.FamilyTableField, 2),
		causalTerm(keyspace.FamilyTableField, 3),
	}
	values := []keyspace.Term{
		causalTerm(keyspace.FamilyValues, 1), // name field
		causalTerm(keyspace.FamilyValues, 2), // name field
		causalTerm(keyspace.FamilyValues, 3), // list field, open spread Vararg
		causalTerm(keyspace.FamilyValues, 4), // Return table value
	}
	spread := causalTerm(keyspace.FamilyVararg, 1)
	cell := causalTerm(keyspace.FamilyCell, 1)
	nils := []keyspace.Term{
		causalTerm(keyspace.FamilyNil, 1),
		causalTerm(keyspace.FamilyNil, 2),
		causalTerm(keyspace.FamilyNil, 3),
	}
	keys := []keyspace.Term{causalTerm(keyspace.FamilyKey, 1), causalTerm(keyspace.FamilyKey, 2)}
	f := openCausalFixture(t, causalSpec{
		counts: causalCounts(
			causalFamilyCount{keyspace.FamilyBody, 1},
			causalFamilyCount{keyspace.FamilyReturn, 1},
			causalFamilyCount{keyspace.FamilyTable, 1},
			causalFamilyCount{keyspace.FamilyTableField, 3},
			causalFamilyCount{keyspace.FamilyValues, 4},
			causalFamilyCount{keyspace.FamilyVararg, 1},
			causalFamilyCount{keyspace.FamilyCell, 1},
			causalFamilyCount{keyspace.FamilyNil, 3},
			causalFamilyCount{keyspace.FamilyKey, 2},
		),
		rows: [][]keyspace.Term{{returned}},
		keys: []source.KeyInput{
			source.NameKey(body, "name"),
			source.ListKey(body, 1),
		},
		exactAtoms: []keyspace.LiteralValue{
			{Kind: keyspace.LiteralString, String: "name"},
			{Kind: keyspace.LiteralInteger, Integer: 1},
		},
		nilOwners: []keyspace.Term{body, body},
		flow: authored.Input{
			Values: authored.ValuesInput{
				Rows: []authored.Value{
					{Owner: body, Fixed: authored.Range{End: 1}},
					{Owner: body, Fixed: authored.Range{Start: 1, End: 2}},
					{Owner: body, Tail: spread, Fixed: authored.Range{Start: 2, End: 2}},
					{Owner: body, Fixed: authored.Range{Start: 2, End: 3}},
				},
				Terms: []keyspace.Term{nils[0], nils[1], table},
			},
			Tables: authored.TablesInput{
				Rows: []authored.Table{{Owner: body, Fields: authored.Range{End: 3}}},
				Fields: []authored.Field{{
					Table: table, Key: keys[0], Values: values[0], Kind: kind.FieldName,
				}, {
					Table: table, Key: nils[2], Values: values[1], Kind: kind.FieldKey,
				}, {
					Table: table, Key: keys[1], Values: values[2], Kind: kind.FieldList,
				}},
				Order: fields,
			},
			Storage: authored.StorageInput{
				Cells:   []authored.Cell{{Kind: authored.CellLocal, Body: body}},
				Varargs: []authored.Vararg{{Owner: body, Cell: cell}},
			},
			Control: authored.ControlInput{Returns: []authored.Return{{Owner: body, Values: values[3]}}},
		},
	})

	for _, field := range fields {
		if !f.executable.Executable(field) {
			t.Fatalf("canonical TableField %v was excluded from executable closure", field)
		}
	}
	if !f.executable.Executable(table) {
		t.Fatal("canonical Table was excluded from executable closure")
	}
	tableEntry, tableEntryOK := f.ports.Entry(table)
	firstEntry, firstEntryOK := f.ports.Entry(fields[0])
	if !tableEntryOK || !firstEntryOK {
		t.Fatal("Table or first TableField Entry port is absent")
	}
	firstRoute := false
	for index := 0; index < f.result.Edges().Count(); index++ {
		edge, ok := f.result.Edges().At(index)
		if ok && edge.From == tableEntry && edge.To == firstEntry {
			firstRoute = true
		}
	}
	if !firstRoute {
		t.Fatalf("Table Entry did not route directly to first TableField: %v -> %v", tableEntry, firstEntry)
	}
	if finish, ok := f.ports.Finish(table); !ok || finish != fields[2] {
		t.Fatalf("Finish(Table) = %v/%v, want final TableField %v", finish, ok, fields[2])
	}
	for index := 0; index+1 < len(fields); index++ {
		from, fromOK := f.ports.Finish(fields[index])
		to, toOK := f.ports.Entry(fields[index+1])
		if !fromOK || !toOK {
			t.Fatalf("TableField chain ports[%d] = %v/%v -> %v/%v", index, from, fromOK, to, toOK)
		}
		found := false
		for edgeIndex := 0; edgeIndex < f.result.Edges().Count(); edgeIndex++ {
			edge, edgeOK := f.result.Edges().At(edgeIndex)
			if edgeOK && edge.From == from && edge.To == to {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("TableField chain omitted: %v -> %v", from, to)
		}
	}
	if got, ok := f.ports.Finish(spread); !ok || got != spread {
		t.Fatalf("spread Vararg finish = %v/%v", got, ok)
	}
	finalValueFinish, finalValueOK := f.ports.Finish(values[3])
	returnFinish, returnFinishOK := f.ports.Finish(returned)
	if !finalValueOK || !returnFinishOK {
		t.Fatal("final table value or Return Finish port is absent")
	}
	finalReturnRoute := false
	for index := 0; index < f.result.Edges().Count(); index++ {
		edge, ok := f.result.Edges().At(index)
		if ok && edge.From == finalValueFinish && edge.To == returnFinish {
			finalReturnRoute = true
		}
	}
	if !finalReturnRoute {
		t.Fatalf("final table value did not route to Return: %v -> %v", finalValueFinish, returnFinish)
	}
}

func TestSemanticMatrixInvalidNormalizedExactKeyThrowsExactBodyOutcome(t *testing.T) {
	body := causalTerm(keyspace.FamilyBody, 1)
	returned := causalTerm(keyspace.FamilyReturn, 1)
	table := causalTerm(keyspace.FamilyTable, 1)
	field := causalTerm(keyspace.FamilyTableField, 1)
	fieldValues := causalTerm(keyspace.FamilyValues, 1)
	returnValues := causalTerm(keyspace.FamilyValues, 2)
	integer := causalTerm(keyspace.FamilyInteger, 1)
	unary := causalTerm(keyspace.FamilyUnary, 1)
	nilValue := causalTerm(keyspace.FamilyNil, 1)
	f := openCausalFixture(t, causalSpec{
		counts: causalCounts(
			causalFamilyCount{keyspace.FamilyBody, 1},
			causalFamilyCount{keyspace.FamilyReturn, 1},
			causalFamilyCount{keyspace.FamilyTable, 1},
			causalFamilyCount{keyspace.FamilyTableField, 1},
			causalFamilyCount{keyspace.FamilyValues, 2},
			causalFamilyCount{keyspace.FamilyInteger, 1},
			causalFamilyCount{keyspace.FamilyUnary, 1},
			causalFamilyCount{keyspace.FamilyNil, 1},
		),
		rows: [][]keyspace.Term{{returned}},
		exactAtoms: []keyspace.LiteralValue{
			{Kind: keyspace.LiteralInteger, Integer: 1},
		},
		intOwners: []keyspace.Term{body},
		nilOwners: []keyspace.Term{body},
		flow: authored.Input{
			Values: authored.ValuesInput{
				Rows:  []authored.Value{{Owner: body, Fixed: authored.Range{End: 1}}, {Owner: body, Fixed: authored.Range{Start: 1, End: 2}}},
				Terms: []keyspace.Term{nilValue, table},
			},
			Tables: authored.TablesInput{
				Rows:   []authored.Table{{Owner: body, Fields: authored.Range{End: 1}}},
				Fields: []authored.Field{{Table: table, Key: unary, Values: fieldValues, Kind: kind.FieldExact}},
				Order:  []keyspace.Term{field},
			},
			Control:   authored.ControlInput{Returns: []authored.Return{{Owner: body, Values: returnValues}}},
			Operators: authored.OperatorsInput{Unaries: []authored.Unary{{Owner: body, Op: kind.UnaryNeg, Operand: integer}}},
		},
	})

	throwExit, ok := f.outcomes.BodyExit(body, kind.OutcomeThrow)
	if !ok {
		t.Fatal("Body Throw Outcome is absent")
	}
	found := false
	for index := 0; index < f.result.Successors().Count(field); index++ {
		successor, ok := f.result.Successors().At(field, index)
		if ok && !successor.IsBoundary() && successor.To == throwExit {
			found = true
		}
	}
	if !found {
		t.Fatalf("invalid exact key did not throw to Body Outcome %v", throwExit)
	}
	tableEntry, tableEntryOK := f.ports.Entry(table)
	if !tableEntryOK || !hasLocalEdge(f.result, tableEntry, field) {
		t.Fatalf("Return-owned Table ingress did not enter invalid Field directly: %v/%v -> %v", tableEntry, tableEntryOK, field)
	}
	if got := f.result.Successors().Count(unary); got != 0 {
		t.Fatalf("invalid exact key evaluated its unary normalization: successor denominator %d", got)
	}
	for index := 0; index < f.result.Edges().Count(); index++ {
		edge, edgeOK := f.result.Edges().At(index)
		if edgeOK && (edge.From == unary || edge.To == unary) {
			t.Fatalf("invalid exact key retained unary evaluation Edge[%d] = %#v", index, edge)
		}
	}
	assertEveryArcDisposition(t, f)
}

func TestSemanticMatrixValidNormalizedExactKeyRetainsUnaryValuesRoute(t *testing.T) {
	body := causalTerm(keyspace.FamilyBody, 1)
	returned := causalTerm(keyspace.FamilyReturn, 1)
	table := causalTerm(keyspace.FamilyTable, 1)
	field := causalTerm(keyspace.FamilyTableField, 1)
	metadataField := causalTerm(keyspace.FamilyTableField, 2)
	fieldValues := causalTerm(keyspace.FamilyValues, 1)
	metadataValues := causalTerm(keyspace.FamilyValues, 2)
	returnValues := causalTerm(keyspace.FamilyValues, 3)
	float := causalTerm(keyspace.FamilyFloat, 1)
	unary := causalTerm(keyspace.FamilyUnary, 1)
	keyTerm := causalTerm(keyspace.FamilyKey, 1)
	nilValue := causalTerm(keyspace.FamilyNil, 1)
	metadataNil := causalTerm(keyspace.FamilyNil, 2)
	f := openCausalFixture(t, causalSpec{
		counts: causalCounts(
			causalFamilyCount{keyspace.FamilyBody, 1},
			causalFamilyCount{keyspace.FamilyReturn, 1},
			causalFamilyCount{keyspace.FamilyTable, 1},
			causalFamilyCount{keyspace.FamilyTableField, 2},
			causalFamilyCount{keyspace.FamilyValues, 3},
			causalFamilyCount{keyspace.FamilyFloat, 1},
			causalFamilyCount{keyspace.FamilyUnary, 1},
			causalFamilyCount{keyspace.FamilyNil, 2},
			causalFamilyCount{keyspace.FamilyKey, 1},
		),
		rows:        [][]keyspace.Term{{returned}},
		keys:        []source.KeyInput{source.ListKey(body, 1)},
		exactAtoms:  []keyspace.LiteralValue{{Kind: keyspace.LiteralInteger, Integer: 1}},
		floatOwners: []keyspace.Term{body},
		floatBits:   []uint64{math.Float64bits(-1)},
		nilOwners:   []keyspace.Term{body},
		flow: authored.Input{
			Values: authored.ValuesInput{
				Rows:  []authored.Value{{Owner: body, Fixed: authored.Range{End: 1}}, {Owner: body, Fixed: authored.Range{Start: 1, End: 2}}, {Owner: body, Fixed: authored.Range{Start: 2, End: 3}}},
				Terms: []keyspace.Term{nilValue, metadataNil, table},
			},
			Tables: authored.TablesInput{
				Rows: []authored.Table{{Owner: body, Fields: authored.Range{End: 2}}},
				Fields: []authored.Field{{Table: table, Key: unary, Values: fieldValues, Kind: kind.FieldExact}, {
					Table: table, Key: keyTerm, Values: metadataValues, Kind: kind.FieldName,
				}},
				Order: []keyspace.Term{field, metadataField},
			},
			Control:   authored.ControlInput{Returns: []authored.Return{{Owner: body, Values: returnValues}}},
			Operators: authored.OperatorsInput{Unaries: []authored.Unary{{Owner: body, Op: kind.UnaryNeg, Operand: float}}},
		},
	})

	tableEntry, tableEntryOK := f.ports.Entry(table)
	fieldEntry, fieldEntryOK := f.ports.Entry(field)
	if !tableEntryOK || !fieldEntryOK || !hasLocalEdge(f.result, tableEntry, fieldEntry) {
		t.Fatalf("valid exact Table ingress = %v/%v -> %v/%v, want field Entry", tableEntry, tableEntryOK, fieldEntry, fieldEntryOK)
	}
	if got := f.result.Successors().Count(unary); got == 0 {
		t.Fatal("valid exact key dropped Unary normalization")
	}
	fieldValuesEntry, valuesEntryOK := f.ports.Entry(fieldValues)
	if !valuesEntryOK || !hasLocalEdge(f.result, unary, fieldValuesEntry) {
		t.Fatalf("valid exact key omitted Unary -> Values route: %v/%v", fieldValuesEntry, valuesEntryOK)
	}
	throwExit, throwOK := f.outcomes.BodyExit(body, kind.OutcomeThrow)
	if !throwOK {
		t.Fatal("Body Throw Outcome is absent")
	}
	if hasLocalEdge(f.result, field, throwExit) {
		t.Fatalf("valid exact key incorrectly retained Field -> Throw route")
	}
	assertEveryArcDisposition(t, f)
}

func TestSemanticMatrixInvalidMiddleTableFieldStopsNormalChain(t *testing.T) {
	body := causalTerm(keyspace.FamilyBody, 1)
	returned := causalTerm(keyspace.FamilyReturn, 1)
	table := causalTerm(keyspace.FamilyTable, 1)
	firstField := causalTerm(keyspace.FamilyTableField, 1)
	invalidField := causalTerm(keyspace.FamilyTableField, 2)
	laterField := causalTerm(keyspace.FamilyTableField, 3)
	firstValues := causalTerm(keyspace.FamilyValues, 1)
	invalidValues := causalTerm(keyspace.FamilyValues, 2)
	laterValues := causalTerm(keyspace.FamilyValues, 3)
	returnValues := causalTerm(keyspace.FamilyValues, 4)
	integer := causalTerm(keyspace.FamilyInteger, 1)
	unary := causalTerm(keyspace.FamilyUnary, 1)
	keyOne := causalTerm(keyspace.FamilyKey, 1)
	keyTwo := causalTerm(keyspace.FamilyKey, 2)
	nilOne := causalTerm(keyspace.FamilyNil, 1)
	nilTwo := causalTerm(keyspace.FamilyNil, 2)
	nilThree := causalTerm(keyspace.FamilyNil, 3)
	f := openCausalFixture(t, causalSpec{
		counts: causalCounts(
			causalFamilyCount{keyspace.FamilyBody, 1},
			causalFamilyCount{keyspace.FamilyReturn, 1},
			causalFamilyCount{keyspace.FamilyTable, 1},
			causalFamilyCount{keyspace.FamilyTableField, 3},
			causalFamilyCount{keyspace.FamilyValues, 4},
			causalFamilyCount{keyspace.FamilyInteger, 1},
			causalFamilyCount{keyspace.FamilyUnary, 1},
			causalFamilyCount{keyspace.FamilyKey, 2},
			causalFamilyCount{keyspace.FamilyNil, 3},
		),
		rows:       [][]keyspace.Term{{returned}},
		keys:       []source.KeyInput{source.NameKey(body, "first"), source.NameKey(body, "last")},
		exactAtoms: []keyspace.LiteralValue{{Kind: keyspace.LiteralString, String: "first"}, {Kind: keyspace.LiteralString, String: "last"}},
		intOwners:  []keyspace.Term{body},
		nilOwners:  []keyspace.Term{body, body, body},
		flow: authored.Input{
			Values: authored.ValuesInput{
				Rows:  []authored.Value{{Owner: body, Fixed: authored.Range{End: 1}}, {Owner: body, Fixed: authored.Range{Start: 1, End: 2}}, {Owner: body, Fixed: authored.Range{Start: 2, End: 3}}, {Owner: body, Fixed: authored.Range{Start: 3, End: 4}}},
				Terms: []keyspace.Term{nilOne, nilTwo, nilThree, table},
			},
			Tables: authored.TablesInput{
				Rows: []authored.Table{{Owner: body, Fields: authored.Range{End: 3}}},
				Fields: []authored.Field{{Table: table, Key: keyOne, Values: firstValues, Kind: kind.FieldName}, {
					Table: table, Key: unary, Values: invalidValues, Kind: kind.FieldExact,
				}, {Table: table, Key: keyTwo, Values: laterValues, Kind: kind.FieldName}},
				Order: []keyspace.Term{firstField, invalidField, laterField},
			},
			Control:   authored.ControlInput{Returns: []authored.Return{{Owner: body, Values: returnValues}}},
			Operators: authored.OperatorsInput{Unaries: []authored.Unary{{Owner: body, Op: kind.UnaryNeg, Operand: integer}}},
		},
	})

	tableEntry, tableEntryOK := f.ports.Entry(table)
	firstEntry, firstEntryOK := f.ports.Entry(firstField)
	if !tableEntryOK || !firstEntryOK || !hasLocalEdge(f.result, tableEntry, firstEntry) {
		t.Fatalf("table ingress did not reach valid first Field: %v/%v -> %v/%v", tableEntry, tableEntryOK, firstEntry, firstEntryOK)
	}
	if !hasLocalEdge(f.result, firstField, invalidField) {
		t.Fatalf("valid first Field did not enter invalid middle Field")
	}
	throwExit, throwOK := f.outcomes.BodyExit(body, kind.OutcomeThrow)
	if !throwOK || !hasLocalEdge(f.result, invalidField, throwExit) {
		t.Fatalf("invalid middle Field did not route to Body Throw: %v/%v", throwExit, throwOK)
	}
	laterEntry, laterEntryOK := f.ports.Entry(laterField)
	tableFinish, tableFinishOK := f.ports.Finish(table)
	if !laterEntryOK || !tableFinishOK || tableFinish != laterField {
		t.Fatalf("Table Finish = %v/%v, later Field Entry = %v/%v", tableFinish, tableFinishOK, laterEntry, laterEntryOK)
	}
	for index := 0; index < f.result.Edges().Count(); index++ {
		edge, edgeOK := f.result.Edges().At(index)
		if edgeOK && edge.From == invalidField && edge.To != throwExit {
			t.Fatalf("invalid middle Field retained non-Throw route: %#v", edge)
		}
	}
	if hasLocalEdge(f.result, invalidField, laterEntry) || hasLocalEdge(f.result, invalidField, tableFinish) {
		t.Fatal("invalid middle Field reached later Field/Table Finish")
	}
	assertEveryArcDisposition(t, f)
}

func TestSemanticMatrixDeepBodyAndWideCallQueries(t *testing.T) {
	const depth = 96
	deepCounts := causalCounts(causalFamilyCount{keyspace.FamilyBody, depth})
	deepRows := make([][]keyspace.Term, depth)
	for index := 0; index+1 < depth; index++ {
		deepRows[index] = []keyspace.Term{causalTerm(keyspace.FamilyBody, uint32(index+2))}
	}
	deep := openCausalFixture(t, causalSpec{counts: deepCounts, rows: deepRows})
	if deep.result.Edges().Count() == 0 {
		t.Fatal("deep Body fixture published no causal routes")
	}
	deepBody := causalTerm(keyspace.FamilyBody, depth)
	if got, ok := deep.result.Edges().BodyCount(deepBody); !ok || got == 0 {
		t.Fatalf("deep leaf Body edge range = %d/%v", got, ok)
	}

	const width = 96
	wide := openCausalFixture(t, wideCallSpec(width))
	view := wide.result.Successors()
	allocs := testing.AllocsPerRun(1000, func() {
		for ordinal := uint32(1); ordinal <= width; ordinal++ {
			call := causalTerm(keyspace.FamilyCall, ordinal)
			_ = view.Count(call)
			_, _ = view.At(call, 0)
		}
	})
	if allocs != 0 {
		t.Fatalf("wide Successors queries allocate %v times", allocs)
	}
	for ordinal := uint32(1); ordinal <= width; ordinal++ {
		call := causalTerm(keyspace.FamilyCall, ordinal)
		if got := view.Count(call); got != 4 {
			t.Fatalf("wide Call %v successor denominator = %d, want 4", call, got)
		}
	}
}

func wideCallSpec(width int) causalSpec {
	counts := causalCounts(
		causalFamilyCount{keyspace.FamilyBody, 1},
		causalFamilyCount{keyspace.FamilyCall, uint32(width)},
		causalFamilyCount{keyspace.FamilyValues, uint32(width)},
		causalFamilyCount{keyspace.FamilyNil, uint32(width)},
	)
	body := causalTerm(keyspace.FamilyBody, 1)
	rows := make([]keyspace.Term, width)
	values := make([]authored.Value, width)
	calls := make([]authored.Call, width)
	nilOwners := make([]keyspace.Term, width)
	for index := 0; index < width; index++ {
		ordinal := uint32(index + 1)
		rows[index] = causalTerm(keyspace.FamilyCall, ordinal)
		values[index] = authored.Value{Owner: body}
		calls[index] = authored.Call{
			Owner:   body,
			Callee:  causalTerm(keyspace.FamilyNil, ordinal),
			Actuals: causalTerm(keyspace.FamilyValues, ordinal),
		}
		nilOwners[index] = body
	}
	return causalSpec{
		name:      "causal-wide.lua",
		counts:    counts,
		rows:      [][]keyspace.Term{rows},
		nilOwners: nilOwners,
		flow: authored.Input{
			Values: authored.ValuesInput{Rows: values},
			Calls:  calls,
		},
	}
}
