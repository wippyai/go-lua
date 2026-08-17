package target

import (
	schematype "github.com/wippyai/go-lua/analysis/schema/typecontract"
	"testing"

	flowkind "github.com/wippyai/go-lua/analysis/program/flow/kind"
)

func TestEffectRowsCarryTotalRowFormalSubstitutions(t *testing.T) {
	contract := mustSeal(t, rowBoundarySpec(RowVariable, CallbackReleaseOne))
	owner, ok := contract.Lookup(BindingSpec{Namespace: BindingBuiltin, Member: []string{"row-owner"}})
	if !ok || contract.EffectCount(owner) != 1 {
		t.Fatalf("owner/effects = %d/%v/%d", owner, ok, contract.EffectCount(owner))
	}
	target, targetOK := contract.Lookup(BindingSpec{Namespace: BindingBuiltin, Member: []string{"row-target"}})
	effectTarget, effectTargetOK := contract.EffectTarget(owner, 0)
	if !targetOK || !effectTargetOK || effectTarget != target || contract.EffectRowArgumentCount(owner, 0) != 1 {
		t.Fatalf("effect row = target:%d/%v want:%d rows:%d", effectTarget, effectTargetOK, target, contract.EffectRowArgumentCount(owner, 0))
	}
	row, rowOK := contract.EffectRowArgumentAt(owner, 0, 0)
	if !rowOK || row != 0 {
		t.Fatalf("effect row argument = %d/%v", row, rowOK)
	}
	if _, ok := contract.EffectRowArgumentAt(owner, 0, 1); ok {
		t.Fatal("out-of-range effect row argument resolved")
	}

	badScope := rowBoundarySpec(RowVariable, CallbackReleaseOne)
	badScope.Operations[0].Effects.Occurrences[0].RowArgs[0] = 1
	if contract, err := testSeal(&badScope); err == nil || contract != nil {
		t.Fatal("effect row argument outside source scope was published")
	}
	badABI := rowBoundarySpec(RowVariable, CallbackReleaseOne)
	badABI.Operations[1].RowFormals = 2
	if contract, err := testSeal(&badABI); err == nil || contract != nil {
		t.Fatal("incomplete effect row substitution was published")
	}
}

func TestCallbackExpectedRowsAndRetainedReleaseAreDirectAndCanonical(t *testing.T) {
	first := mustSeal(t, rowBoundarySpec(RowVariable, CallbackReleaseOne))
	second := mustSeal(t, rowBoundarySpec(RowVariable, CallbackReleaseAll))
	if first.ContentID() == second.ContentID() {
		t.Fatal("callback release mode was omitted from canonical artifact")
	}
	owner, ownerOK := first.Lookup(BindingSpec{Namespace: BindingBuiltin, Member: []string{"row-owner"}})
	releaseOp, releaseOK := first.Lookup(BindingSpec{Namespace: BindingBuiltin, Member: []string{"row-target"}})
	callback, callbackOK := first.CallbackAt(owner, 0)
	if !ownerOK || !releaseOK || !callbackOK {
		t.Fatal("callback boundary fixture failed to resolve")
	}
	tail, variable, tailOK := first.CallbackEffectTail(callback)
	if !tailOK || tail != RowVariable || variable != 0 || first.CallbackEffectCount(callback) != 1 {
		t.Fatalf("callback expected row = %d/%d/%v effects:%d", tail, variable, tailOK, first.CallbackEffectCount(callback))
	}
	effectTarget, effectOK := first.CallbackEffectTarget(callback, 0)
	row, rowOK := first.CallbackEffectRowArgumentAt(callback, 0, 0)
	if !effectOK || effectTarget != releaseOp || !rowOK || row != 0 {
		t.Fatalf("callback effect = target:%d/%v row:%d/%v", effectTarget, effectOK, row, rowOK)
	}
	operation, input, outcome, mode, releaseFound := first.CallbackRelease(callback)
	if !releaseFound || operation != releaseOp || input != 0 || outcome != 0 || mode != CallbackReleaseOne {
		t.Fatalf("callback release = %d/%d/%d/%d/%v", operation, input, outcome, mode, releaseFound)
	}
	if first.CallbackReleaseCount(releaseOp) != 1 {
		t.Fatalf("release reverse range = %d", first.CallbackReleaseCount(releaseOp))
	}
	released, releaseInput, releaseOutcome, releaseMode, reverseFound := first.CallbackReleaseAt(releaseOp, 0)
	if !reverseFound || released != callback || releaseInput != 0 || releaseOutcome != 0 || releaseMode != CallbackReleaseOne {
		t.Fatalf("reverse release = %d/%d/%d/%d/%v", released, releaseInput, releaseOutcome, releaseMode, reverseFound)
	}

	missingRow := rowBoundarySpec(RowVariable, CallbackReleaseOne)
	missingRow.Operations[0].Callbacks[0].Effects = RowSpec{}
	if contract, err := testSeal(&missingRow); err == nil || contract != nil {
		t.Fatal("callback without an expected row was published")
	}
	syncRelease := rowBoundarySpec(RowVariable, CallbackReleaseOne)
	syncRelease.Operations[0].Callbacks[0].Lifecycle = CallbackSyncOptionalOnce
	if contract, err := testSeal(&syncRelease); err == nil || contract != nil {
		t.Fatal("sync callback release was published")
	}
	badRelease := rowBoundarySpec(RowVariable, CallbackReleaseOne)
	badRelease.Operations[0].Callbacks[0].Release.Outcome = 1
	if contract, err := testSeal(&badRelease); err == nil || contract != nil {
		t.Fatal("release outcome outside target operation was published")
	}
}

func rowBoundarySpec(callbackRowTail RowTail, mode CallbackReleaseMode) Spec {
	return Spec{Operations: []OperationSpec{
		{
			Bindings:   []BindingSpec{{Namespace: BindingBuiltin, Member: []string{"row-owner"}}},
			ValuesVars: 5,
			RowFormals: 1,
			Input:      ValuesSpec{Fixed: []schematype.Type{testAny}, Tail: ValuesVariable, Var: 0},
			Outcomes:   []OutcomeSpec{{Kind: flowkind.OutcomeNormal, Values: ValuesSpec{Tail: ValuesClosed}}},
			Effects:    RowSpec{Occurrences: []EffectSpec{{Target: 2, ValueArgs: []ValueFormal{0}, RowArgs: []RowVar{0}}}, Tail: RowVariable, Var: 0},
			Callbacks: []CallbackSpec{{
				Function: InputSource{Kind: InputSourceValueFormal}, Admission: OrdinaryCallable, Arguments: callbackTail(0),
				Outcomes: callbackOutcomes(1, 1, 2, 3, 4), Lifecycle: CallbackRetainedOptionalOnce,
				Effects: RowSpec{Occurrences: []EffectSpec{{Target: 2, ValueArgs: []ValueFormal{0}, RowArgs: []RowVar{0}}}, Tail: callbackRowTail, Var: 0},
				Release: &CallbackReleaseSpec{Operation: 2, Input: 0, Outcome: 0, Mode: mode, Zero: CallbackReleaseZeroSpec{Behavior: CallbackReleaseZeroIdempotent, Outcome: 0}},
			}},
		},
		{
			Bindings:   []BindingSpec{{Namespace: BindingBuiltin, Member: []string{"row-target"}}},
			RowFormals: 1,
			Input:      ValuesSpec{Fixed: []schematype.Type{testAny}, Tail: ValuesClosed},
			Outcomes:   []OutcomeSpec{{Kind: flowkind.OutcomeNormal, Values: ValuesSpec{Tail: ValuesClosed}}},
			Effects:    RowSpec{Tail: RowClosed},
		},
	}}
}
