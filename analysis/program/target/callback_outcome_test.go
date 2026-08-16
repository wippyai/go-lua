package target

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/domain/type/typ"
	flowkind "github.com/wippyai/go-lua/analysis/program/flow/kind"
)

func callbackOutcomeOperation(name string, outcomes []TerminalSpec) OperationSpec {
	return OperationSpec{
		Bindings:   []BindingSpec{{Namespace: BindingBuiltin, Member: []string{name}}},
		ValuesVars: 6,
		Input:      ValuesSpec{Fixed: []typ.Type{typ.Any}, Tail: ValuesVariable, Var: 0},
		Callbacks: []CallbackSpec{{
			Function: InputSource{Kind: InputSourceValueFormal}, Admission: OrdinaryCallable,
			Arguments: ValuesSpec{Tail: ValuesVariable, Var: 0},
			Outcomes:  outcomes, Lifecycle: CallbackRetainedOptionalOnce,
			Effects: RowSpec{Tail: RowClosed},
		}},
		Outcomes: []OutcomeSpec{{Kind: flowkind.OutcomeNormal, Values: ValuesSpec{Tail: ValuesClosed}}},
		Effects:  RowSpec{Tail: RowClosed},
	}
}

func TestCallbackOutcomeIsTotalCanonicalAndAllocationFree(t *testing.T) {
	outcomes := callbackOutcomes(1, 2, 3, 4, 5)
	reversed := append([]TerminalSpec(nil), outcomes...)
	for left, right := 0, len(reversed)-1; left < right; left, right = left+1, right-1 {
		reversed[left], reversed[right] = reversed[right], reversed[left]
	}
	first := mustSeal(t, Spec{Operations: []OperationSpec{callbackOutcomeOperation("callback-outcome", outcomes)}})
	second := mustSeal(t, Spec{Operations: []OperationSpec{callbackOutcomeOperation("callback-outcome", reversed)}})
	assertPublicContractEqual(t, first, second)
	op, found := first.Lookup(BindingSpec{Namespace: BindingBuiltin, Member: []string{"callback-outcome"}})
	id, idFound := first.CallbackAt(op, 0)
	if !found || !idFound {
		t.Fatal("callback outcome correspondence missing")
	}
	kinds := [...]flowkind.OutcomeKind{
		flowkind.OutcomeNormal, flowkind.OutcomeReturn, flowkind.OutcomeThrow,
		flowkind.OutcomeYield, flowkind.OutcomeCancel,
	}
	for index, kind := range kinds {
		values, ok := first.CallbackOutcome(id, kind)
		tail, variable, tailOK := first.ValuesTail(values)
		if !ok || !tailOK || tail != ValuesVariable || variable != ValuesVar(index+1) {
			t.Fatalf("callback outcome %d = %d/%d/%d/%v/%v, want tail %d", kind, values, tail, variable, ok, tailOK, index+1)
		}
	}
	for _, kind := range []flowkind.OutcomeKind{flowkind.OutcomeBreak, flowkind.OutcomeGoto} {
		if _, ok := first.CallbackOutcome(id, kind); ok {
			t.Fatalf("non-activation outcome %d resolved", kind)
		}
	}
	if _, ok := first.CallbackOutcome(0, flowkind.OutcomeNormal); ok {
		t.Fatal("zero CallbackID resolved an outcome")
	}
	if allocs := testing.AllocsPerRun(1000, func() {
		for _, kind := range kinds {
			if _, ok := first.CallbackOutcome(id, kind); !ok {
				panic("callback outcome disappeared")
			}
		}
	}); allocs != 0 {
		t.Fatalf("CallbackOutcome allocated %f times", allocs)
	}
}

func TestCallbackOutcomeRejectsIncompleteDuplicateInvalidAndOutOfScope(t *testing.T) {
	valid := callbackOutcomes(1, 2, 3, 4, 5)
	duplicate := append([]TerminalSpec(nil), valid...)
	duplicate[4].Kind = flowkind.OutcomeNormal
	invalid := append([]TerminalSpec(nil), valid...)
	invalid[4].Kind = flowkind.OutcomeBreak
	outOfScope := append([]TerminalSpec(nil), valid...)
	outOfScope[4].Values.Var = 6
	for _, test := range []struct {
		name     string
		outcomes []TerminalSpec
	}{
		{name: "missing", outcomes: valid[:4]},
		{name: "duplicate", outcomes: duplicate},
		{name: "invalid kind", outcomes: invalid},
		{name: "Values outside scope", outcomes: outOfScope},
	} {
		t.Run(test.name, func(t *testing.T) {
			if _, err := Seal(&Spec{Operations: []OperationSpec{callbackOutcomeOperation("invalid-callback-outcome", test.outcomes)}}); err == nil {
				t.Fatal("invalid callback outcome relation accepted")
			}
		})
	}
}
