package call

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/type/typ"
	flowkind "github.com/wippyai/go-lua/program/flow/kind"
	"github.com/wippyai/go-lua/program/target"
)

func closedKeySumContract(t *testing.T, reverse bool) *target.Contract {
	t.Helper()
	alpha := closedKeySumOperation("alpha")
	beta := closedKeySumOperation("beta")
	operations := []target.OperationSpec{alpha, beta}
	if reverse {
		operations[0], operations[1] = operations[1], operations[0]
	}
	contract, err := target.Seal(&target.Spec{Operations: operations})
	if err != nil {
		t.Fatal(err)
	}
	return contract
}

func closedKeySumOperation(name string) target.OperationSpec {
	return target.OperationSpec{
		Bindings:   []target.BindingSpec{{Namespace: target.BindingBuiltin, Member: []string{name}}},
		ValuesVars: 5,
		Input:      target.ValuesSpec{Fixed: []typ.Type{typ.Any, typ.Any}, Tail: target.ValuesVariable, Var: 0},
		Outcomes: []target.OutcomeSpec{
			{Kind: flowkind.OutcomeNormal, Values: target.ValuesSpec{Tail: target.ValuesClosed}},
		},
		Callbacks: []target.CallbackSpec{{
			Function:  target.InputSource{Kind: target.InputSourceValueFormal, Ordinal: 0},
			Admission: target.OrdinaryCallable,
			Arguments: target.ValuesSpec{Tail: target.ValuesVariable, Var: 0},
			Outcomes: []target.TerminalSpec{
				{Kind: flowkind.OutcomeNormal, Values: target.ValuesSpec{Tail: target.ValuesVariable, Var: 0}},
				{Kind: flowkind.OutcomeReturn, Values: target.ValuesSpec{Tail: target.ValuesVariable, Var: 1}},
				{Kind: flowkind.OutcomeThrow, Values: target.ValuesSpec{Tail: target.ValuesVariable, Var: 2}},
				{Kind: flowkind.OutcomeYield, Values: target.ValuesSpec{Tail: target.ValuesVariable, Var: 3}},
				{Kind: flowkind.OutcomeCancel, Values: target.ValuesSpec{Tail: target.ValuesVariable, Var: 4}},
			},
			Lifecycle: target.CallbackRetainedOptionalOnce,
			Effects:   target.RowSpec{Tail: target.RowClosed},
		}},
		Resumes: []target.ResumeSpec{{
			Source:    target.ResumeSourceValueFormal,
			Carrier:   0,
			Arguments: target.ValuesSpec{Tail: target.ValuesVariable, Var: 0},
			Outcomes: []target.ResumeOutcomeSpec{
				{Kind: flowkind.OutcomeNormal, Outcome: 0},
				{Kind: flowkind.OutcomeReturn, Outcome: 0},
				{Kind: flowkind.OutcomeThrow, Outcome: 0},
				{Kind: flowkind.OutcomeYield, Outcome: 0},
				{Kind: flowkind.OutcomeCancel, Outcome: 0},
			},
		}},
		Effects: target.RowSpec{Tail: target.RowClosed},
	}
}

func TestCallClosedKeySumCoverageAndNoProduct(t *testing.T) {
	contract := closedKeySumContract(t, false)
	link, _ := callSource(t, "call_keysum", `local function invoke(f) return f() end; invoke(invoke)`, contract)
	algebra, ok := New(link)
	if !ok {
		t.Fatal("Call algebra")
	}

	applications := link.Project().Applications()
	baseCount := 0
	for index := 0; index < applications.Count(); index++ {
		application, present := applications.At(index)
		if !present {
			t.Fatal("application")
		}
		if !applications.IsBase(application) {
			continue
		}
		baseCount++
		key, keyed := algebra.KeyForApplication(application)
		projected, projectedOK := key.Application()
		if !keyed || !key.IsApplication() || !projectedOK || projected != application {
			t.Fatal("base application key")
		}
		id, idOK := key.ContentID()
		if !idOK {
			t.Fatal("base application identity")
		}
		if restored, restoredOK := algebra.FindKey(id); !restoredOK || restored != key {
			t.Fatal("base application inverse")
		}
	}

	targetContract, targetOK := link.Boundary().Target()
	if !targetOK {
		t.Fatal("Target")
	}
	callbackCount, resumeCount := 0, 0
	for operationIndex := 0; operationIndex < targetContract.OperationCount(); operationIndex++ {
		operation, present := targetContract.OperationAt(operationIndex)
		if !present {
			t.Fatal("operation")
		}
		for callbackIndex := 0; callbackIndex < targetContract.CallbackCount(operation); callbackIndex++ {
			callback, present := targetContract.CallbackAt(operation, callbackIndex)
			if !present {
				t.Fatal("callback")
			}
			key, keyed := algebra.KeyForCallback(targetContract, operation, callback)
			if !keyed || !key.IsCallback() {
				t.Fatal("callback key")
			}
			gotOperation, gotCallback, projected := key.Callback()
			if !projected || gotOperation != operation || gotCallback != callback {
				t.Fatal("callback projection")
			}
			callbackCount++
		}
		for resumeIndex := 0; resumeIndex < targetContract.ResumeCount(operation); resumeIndex++ {
			resume, present := targetContract.ResumeIDAt(operation, resumeIndex)
			if !present {
				t.Fatal("resume")
			}
			key, keyed := algebra.KeyForResume(targetContract, operation, resume)
			if !keyed || !key.IsResume() {
				t.Fatal("resume key")
			}
			gotOperation, gotResume, projected := key.Resume()
			if !projected || gotOperation != operation || gotResume != resume {
				t.Fatal("resume projection")
			}
			resumeCount++
		}
	}

	if want := baseCount + callbackCount + resumeCount; algebra.KeyCount() != want {
		t.Fatalf("closed Call key sum = %d, want B+C+R=%d (B=%d C=%d R=%d)", algebra.KeyCount(), want, baseCount, callbackCount, resumeCount)
	}
	for index := 0; index < algebra.KeyCount(); index++ {
		key, present := algebra.KeyAt(index)
		arms := 0
		if key.IsApplication() {
			arms++
		}
		if key.IsCallback() {
			arms++
		}
		if key.IsResume() {
			arms++
		}
		if !present || arms != 1 {
			t.Fatal("key sum arm is not exactly one arm")
		}
	}
}

func TestCallClosedKeySumReplayPermutationAndForeignFences(t *testing.T) {
	contract := closedKeySumContract(t, false)
	leftLink, _ := callSource(t, "call_keysum_replay", `local function invoke(f) return f() end; invoke(invoke)`, contract)
	rightLink, _ := callSource(t, "call_keysum_replay", `local function invoke(f) return f() end; invoke(invoke)`, contract)
	left, leftOK := New(leftLink)
	right, rightOK := New(rightLink)
	if !leftOK || !rightOK || !left.Equivalent(right) || left.ContentID() != right.ContentID() {
		t.Fatal("equivalent Call replay")
	}
	for index := 0; index < left.KeyCount(); index++ {
		key, present := left.KeyAt(index)
		id, idOK := key.ContentID()
		rebound, reboundOK := right.FindKey(id)
		if !present || !idOK || !reboundOK || !rebound.IsApplication() && !rebound.IsCallback() && !rebound.IsResume() {
			t.Fatal("portable key replay")
		}
	}

	permutedContract := closedKeySumContract(t, true)
	permutedLink, _ := callSource(t, "call_keysum_replay", `local function invoke(f) return f() end; invoke(invoke)`, permutedContract)
	permuted, permutedOK := New(permutedLink)
	if !permutedOK || !left.Equivalent(permuted) {
		t.Fatal("Target operation permutation changed Call key algebra")
	}
	permutedTarget, targetOK := permutedLink.Boundary().Target()
	if !targetOK || permutedTarget == nil {
		t.Fatal("permuted Target")
	}
	permutedOperation, operationOK := permutedTarget.OperationAt(0)
	permutedCallback, callbackOK := permutedTarget.CallbackAt(permutedOperation, 0)
	if !operationOK || !callbackOK {
		t.Fatal("permuted Target callback")
	}
	if _, accepted := left.KeyForCallback(permutedTarget, permutedOperation, permutedCallback); accepted {
		t.Fatal("foreign equivalent Target callback crossed owner fence")
	}
	permutedResume, resumeOK := permutedTarget.ResumeIDAt(permutedOperation, 0)
	if !resumeOK {
		t.Fatal("permuted Target resume")
	}
	if _, accepted := left.KeyForResume(permutedTarget, permutedOperation, permutedResume); accepted {
		t.Fatal("foreign equivalent Target resume crossed owner fence")
	}

	foreignLink, foreignProgram := callSource(t, "call_keysum_foreign", `local function invoke(f) return f() end`, contract)
	_, foreignOK := New(foreignLink)
	if !foreignOK {
		t.Fatal("foreign Call algebra")
	}
	foreignApps := foreignLink.Project().Applications()
	for index := 0; index < foreignApps.Count(); index++ {
		application, present := foreignApps.At(index)
		if !present || !foreignApps.IsBase(application) {
			continue
		}
		if _, accepted := left.KeyForApplication(application); accepted {
			t.Fatal("foreign Project application crossed owner fence")
		}
	}
	_ = foreignProgram
}

func TestCallClosedKeySumHotLookupsAllocateNothing(t *testing.T) {
	contract := closedKeySumContract(t, false)
	link, _ := callSource(t, "call_keysum_alloc", `local function invoke(f) return f() end; invoke(invoke)`, contract)
	algebra, ok := New(link)
	if !ok {
		t.Fatal("Call algebra")
	}
	targetContract, ok := link.Boundary().Target()
	if !ok {
		t.Fatal("Target")
	}
	op, ok := targetContract.OperationAt(0)
	if !ok {
		t.Fatal("operation")
	}
	callback, ok := targetContract.CallbackAt(op, 0)
	if !ok {
		t.Fatal("callback")
	}
	resume, ok := targetContract.ResumeIDAt(op, 0)
	if !ok {
		t.Fatal("resume")
	}
	key, ok := algebra.KeyForCallback(targetContract, op, callback)
	if !ok {
		t.Fatal("callback key")
	}
	id, ok := key.ContentID()
	if !ok {
		t.Fatal("callback identity")
	}
	if allocations := testing.AllocsPerRun(1000, func() {
		if _, ok := algebra.KeyForCallback(targetContract, op, callback); !ok {
			panic("callback lookup")
		}
		if _, ok := algebra.KeyForResume(targetContract, op, resume); !ok {
			panic("resume lookup")
		}
		if _, ok := algebra.FindKey(id); !ok {
			panic("identity lookup")
		}
		if _, _, ok := key.Callback(); !ok {
			panic("callback projection")
		}
	}); allocations != 0 {
		t.Fatalf("closed key lookups allocated %g times", allocations)
	}
}
