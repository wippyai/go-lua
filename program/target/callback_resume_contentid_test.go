package target

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/type/typ"
	flowkind "github.com/wippyai/go-lua/program/flow/kind"
	"github.com/wippyai/go-lua/program/keyspace"
)

func callbackResumeContentOperation(name string, callbackFormal, resumeCarrier ValueFormal) OperationSpec {
	return OperationSpec{
		Bindings:   []BindingSpec{{Namespace: BindingBuiltin, Member: []string{name}}},
		ValuesVars: 5,
		Input:      ValuesSpec{Fixed: []typ.Type{typ.Any, typ.Any}, Tail: ValuesVariable, Var: 0},
		Outcomes: []OutcomeSpec{
			{Kind: flowkind.OutcomeNormal, Values: callbackTail(0)},
		},
		Callbacks: []CallbackSpec{{
			Function:  InputSource{Kind: InputSourceValueFormal, Ordinal: uint32(callbackFormal)},
			Admission: OrdinaryCallable,
			Arguments: callbackTail(1),
			Outcomes:  callbackOutcomes(1, 1, 2, 3, 4),
			Lifecycle: CallbackRetainedOptionalOnce,
			Effects:   RowSpec{Tail: RowClosed},
		}},
		Resumes: []ResumeSpec{{
			Source:    ResumeSourceValueFormal,
			Carrier:   resumeCarrier,
			Arguments: callbackTail(0),
			Outcomes: []ResumeOutcomeSpec{
				{Kind: flowkind.OutcomeNormal, Outcome: 0},
				{Kind: flowkind.OutcomeReturn, Outcome: 0},
				{Kind: flowkind.OutcomeThrow, Outcome: 0},
				{Kind: flowkind.OutcomeYield, Outcome: 0},
				{Kind: flowkind.OutcomeCancel, Outcome: 0},
			},
		}},
		Effects: RowSpec{Tail: RowClosed},
	}
}

func TestCallbackAndResumeContentIDsFenceOwnersAndInvert(t *testing.T) {
	contract := mustSeal(t, Spec{Operations: []OperationSpec{
		callbackResumeContentOperation("alpha", 0, 0),
		callbackResumeContentOperation("beta", 1, 1),
	}})
	alpha, ok := contract.Lookup(BindingSpec{Namespace: BindingBuiltin, Member: []string{"alpha"}})
	if !ok {
		t.Fatal("alpha operation missing")
	}
	beta, ok := contract.Lookup(BindingSpec{Namespace: BindingBuiltin, Member: []string{"beta"}})
	if !ok {
		t.Fatal("beta operation missing")
	}
	alphaCallback, _ := contract.CallbackAt(alpha, 0)
	betaCallback, _ := contract.CallbackAt(beta, 0)
	alphaResume, _ := contract.ResumeIDAt(alpha, 0)
	betaResume, _ := contract.ResumeIDAt(beta, 0)

	callback, ok := contract.CallbackContentID(alpha, alphaCallback)
	if !ok || !callback.Available() {
		t.Fatal("alpha callback content identity unavailable")
	}
	if owner, got, ok := contract.FindCallbackContentID(callback); !ok || owner != alpha || got != alphaCallback {
		t.Fatalf("callback inverse = %d/%d/%v", owner, got, ok)
	}
	resume, ok := contract.ResumeContentID(alpha, alphaResume)
	if !ok || !resume.Available() {
		t.Fatal("alpha resume content identity unavailable")
	}
	if owner, got, ok := contract.FindResumeContentID(resume); !ok || owner != alpha || got != alphaResume {
		t.Fatalf("resume inverse = %d/%d/%v", owner, got, ok)
	}
	if _, ok := contract.CallbackContentID(beta, alphaCallback); ok {
		t.Fatal("callback content identity accepted a foreign owner")
	}
	if _, ok := contract.ResumeContentID(beta, alphaResume); ok {
		t.Fatal("resume content identity accepted a foreign owner")
	}
	if _, _, ok := contract.FindCallbackContentID(keyspace.ContentID{}); ok {
		t.Fatal("zero callback content identity inverted")
	}
	if _, _, ok := contract.FindResumeContentID(keyspace.ContentID{}); ok {
		t.Fatal("zero resume content identity inverted")
	}
	if _, ok := contract.CallbackContentID(alpha, 0); ok {
		t.Fatal("zero callback handle accepted")
	}
	if _, ok := contract.ResumeContentID(alpha, 0); ok {
		t.Fatal("zero resume handle accepted")
	}
	if allocs := testing.AllocsPerRun(1000, func() {
		if _, ok := contract.CallbackContentID(alpha, alphaCallback); !ok {
			panic("callback identity disappeared")
		}
		if _, _, ok := contract.FindCallbackContentID(callback); !ok {
			panic("callback inverse disappeared")
		}
		if _, ok := contract.ResumeContentID(alpha, alphaResume); !ok {
			panic("resume identity disappeared")
		}
		if _, _, ok := contract.FindResumeContentID(resume); !ok {
			panic("resume inverse disappeared")
		}
	}); allocs != 0 {
		t.Fatalf("content identity queries allocated %f times", allocs)
	}
	if betaCallback == alphaCallback || betaResume == alphaResume {
		t.Fatal("operation-local handles unexpectedly overlap")
	}
}

func TestCallbackAndResumeContentIDsAreReplayAndPermutationStable(t *testing.T) {
	left := mustSeal(t, Spec{Operations: []OperationSpec{
		callbackResumeContentOperation("beta", 1, 1),
		callbackResumeContentOperation("alpha", 0, 0),
	}})
	right := mustSeal(t, Spec{Operations: []OperationSpec{
		callbackResumeContentOperation("alpha", 0, 0),
		callbackResumeContentOperation("beta", 1, 1),
	}})
	for _, name := range []string{"alpha", "beta"} {
		leftOp, _ := left.Lookup(BindingSpec{Namespace: BindingBuiltin, Member: []string{name}})
		rightOp, _ := right.Lookup(BindingSpec{Namespace: BindingBuiltin, Member: []string{name}})
		leftCallback, _ := left.CallbackAt(leftOp, 0)
		rightCallback, _ := right.CallbackAt(rightOp, 0)
		leftCallbackID, leftOK := left.CallbackContentID(leftOp, leftCallback)
		rightCallbackID, rightOK := right.CallbackContentID(rightOp, rightCallback)
		if !leftOK || !rightOK || leftCallbackID != rightCallbackID {
			t.Fatalf("%s callback identity changed across replay", name)
		}
		leftResume, _ := left.ResumeIDAt(leftOp, 0)
		rightResume, _ := right.ResumeIDAt(rightOp, 0)
		leftResumeID, leftOK := left.ResumeContentID(leftOp, leftResume)
		rightResumeID, rightOK := right.ResumeContentID(rightOp, rightResume)
		if !leftOK || !rightOK || leftResumeID != rightResumeID {
			t.Fatalf("%s resume identity changed across replay", name)
		}
	}
}

func TestCallbackAndResumeContentIDsTrackDescriptorMutation(t *testing.T) {
	base := mustSeal(t, Spec{Operations: []OperationSpec{callbackResumeContentOperation("mutable", 0, 0)}})
	callback, _ := base.CallbackAt(1, 0)
	resume, _ := base.ResumeIDAt(1, 0)
	baseCallback, _ := base.CallbackContentID(1, callback)
	baseResume, _ := base.ResumeContentID(1, resume)

	changedCallback := callbackResumeContentOperation("mutable", 1, 0)
	changedResume := callbackResumeContentOperation("mutable", 0, 1)
	callbackContract := mustSeal(t, Spec{Operations: []OperationSpec{changedCallback}})
	resumeContract := mustSeal(t, Spec{Operations: []OperationSpec{changedResume}})
	callbackAfter, _ := callbackContract.CallbackContentID(1, callback)
	resumeAfter, _ := resumeContract.ResumeContentID(1, resume)
	if callbackAfter == baseCallback {
		t.Fatal("callback descriptor mutation reused content identity")
	}
	if resumeAfter == baseResume {
		t.Fatal("resume descriptor mutation reused content identity")
	}
}
