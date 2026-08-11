package target

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/type/typ"
	flowkind "github.com/wippyai/go-lua/program/flow/kind"
)

func TestProducedOperationUsesOneOrdinaryOperationIdentity(t *testing.T) {
	contract := mustSeal(t, Spec{Operations: []OperationSpec{
		{
			Bindings: []BindingSpec{{Namespace: BindingBuiltin, Member: []string{"factory"}}},
			Input:    ValuesSpec{Fixed: []typ.Type{typ.String}, Tail: ValuesClosed},
			Outcomes: []OutcomeSpec{{
				Kind:   flowkind.OutcomeNormal,
				Values: ValuesSpec{Fixed: []typ.Type{typ.Any}, Tail: ValuesClosed},
				Produced: []ProducedSpec{{
					Result: 0, Operation: 2,
					Captures: []CaptureSpec{{Kind: CaptureValueFormal, Ordinal: 0}},
				}},
			}},
			Effects: RowSpec{Tail: RowClosed},
		},
		{
			ValuesVars: 1,
			Input:      ValuesSpec{Tail: ValuesVariable, Var: 0},
			Outcomes: []OutcomeSpec{{
				Kind:   flowkind.OutcomeNormal,
				Values: ValuesSpec{Fixed: []typ.Type{typ.Any}, Tail: ValuesClosed},
				Produced: []ProducedSpec{{
					Result: 0, Operation: 3,
					Captures: []CaptureSpec{{Kind: CaptureValuesVar, Ordinal: 0}},
				}},
			}},
			Effects: RowSpec{Tail: RowClosed},
		},
		{
			Input:    ValuesSpec{Tail: ValuesClosed},
			Outcomes: []OutcomeSpec{{Kind: flowkind.OutcomeNormal, Values: ValuesSpec{Tail: ValuesClosed}}},
			Effects:  RowSpec{Tail: RowClosed},
		},
	}})
	factory, ok := contract.Lookup(BindingSpec{Namespace: BindingBuiltin, Member: []string{"factory"}})
	if !ok || factory != 1 || contract.BoundOperationCount() != 1 {
		t.Fatalf("factory/bound = %d/%v/%d", factory, ok, contract.BoundOperationCount())
	}
	first, row, ok := contract.ProducedForResult(factory, 0, 0)
	if !ok || first != 2 || row != 0 {
		t.Fatalf("factory result = %d/%d/%v, want 2/0/true", first, row, ok)
	}
	if kind, source, ok := contract.ProducedCaptureAt(factory, 0, row, 0); !ok || kind != CaptureValueFormal || source != 0 {
		t.Fatalf("factory capture = %d/%d/%v", kind, source, ok)
	}
	second, _, ok := contract.ProducedForResult(first, 0, 0)
	if !ok || second != 3 {
		t.Fatalf("produced chain = %d/%v, want 3/true", second, ok)
	}
}

func TestProducedCallbackCaptureRemapsToSealedID(t *testing.T) {
	contract := mustSeal(t, Spec{Operations: []OperationSpec{
		{
			Bindings:   []BindingSpec{{Namespace: BindingBuiltin, Member: []string{"wrap"}}},
			ValuesVars: 5,
			Input:      ValuesSpec{Fixed: []typ.Type{typ.String}, Tail: ValuesVariable, Var: 0},
			Callbacks: []CallbackSpec{{
				Function:  InputSource{Kind: InputSourceValueFormal, Ordinal: 0},
				Admission: OrdinaryCallable, Arguments: callbackTail(0), Outcomes: callbackOutcomes(1, 1, 2, 3, 4), Lifecycle: CallbackRetainedOptionalOnce,
				Effects: RowSpec{Tail: RowClosed},
			}},
			Outcomes: []OutcomeSpec{{
				Kind:   flowkind.OutcomeNormal,
				Values: ValuesSpec{Fixed: []typ.Type{typ.Any}, Tail: ValuesClosed},
				Produced: []ProducedSpec{{
					Result: 0, Operation: 2,
					Captures: []CaptureSpec{{Kind: CaptureCallback, Ordinal: 1}},
				}},
			}},
			Effects: RowSpec{Tail: RowClosed},
		},
		{Input: ValuesSpec{Tail: ValuesClosed}, Outcomes: []OutcomeSpec{{Kind: flowkind.OutcomeNormal, Values: ValuesSpec{Tail: ValuesClosed}}}, Effects: RowSpec{Tail: RowClosed}},
	}})
	wrap, _ := contract.Lookup(BindingSpec{Namespace: BindingBuiltin, Member: []string{"wrap"}})
	callback, ok := contract.CallbackAt(wrap, 0)
	if !ok || callback == 0 {
		t.Fatal("missing sealed callback")
	}
	functionSource, ok := contract.CallbackFunction(callback)
	if !ok || functionSource.Kind != InputSourceValueFormal || functionSource.Ordinal != 0 {
		t.Fatalf("callback function = %#v/%v", functionSource, ok)
	}
	_, row, ok := contract.ProducedForResult(wrap, 0, 0)
	if !ok {
		t.Fatal("missing wrap produced operation")
	}
	kind, source, ok := contract.ProducedCaptureAt(wrap, 0, row, 0)
	if !ok || kind != CaptureCallback || CallbackID(source) != callback {
		t.Fatalf("callback capture = %d/%d/%v, want %d", kind, source, ok, callback)
	}
}

func TestBindingAliasesShareOneOperation(t *testing.T) {
	contract := mustSeal(t, Spec{Operations: []OperationSpec{{
		Bindings: []BindingSpec{
			{Namespace: BindingModule, Owner: []string{"string"}, Member: []string{"gfind"}},
			{Namespace: BindingModule, Owner: []string{"string"}, Member: []string{"gmatch"}},
		},
		Input:    ValuesSpec{Tail: ValuesClosed},
		Outcomes: []OutcomeSpec{{Kind: flowkind.OutcomeNormal, Values: ValuesSpec{Tail: ValuesClosed}}},
		Effects:  RowSpec{Tail: RowClosed},
	}}})
	gfind, _ := contract.Lookup(BindingSpec{Namespace: BindingModule, Owner: []string{"string"}, Member: []string{"gfind"}})
	gmatch, _ := contract.Lookup(BindingSpec{Namespace: BindingModule, Owner: []string{"string"}, Member: []string{"gmatch"}})
	if gfind == 0 || gfind != gmatch || contract.BindingCount(gfind) != 2 {
		t.Fatalf("aliases = %d/%d, binding count %d", gfind, gmatch, contract.BindingCount(gfind))
	}
}

func TestBoundOperationsStayCanonicalPrefixWithProducedChildren(t *testing.T) {
	makeSpec := func(alphaRef SpecRef, input []OperationSpec) *Contract {
		input[0].Outcomes[0].Produced = []ProducedSpec{{Result: 0, Operation: alphaRef}}
		return mustSeal(t, Spec{Operations: input})
	}
	alpha := OperationSpec{
		Bindings: []BindingSpec{{Namespace: BindingBuiltin, Member: []string{"alpha"}}},
		Input:    ValuesSpec{Tail: ValuesClosed},
		Outcomes: []OutcomeSpec{{Kind: flowkind.OutcomeNormal, Values: ValuesSpec{Fixed: []typ.Type{typ.Any}, Tail: ValuesClosed}}},
		Effects:  RowSpec{Tail: RowClosed},
	}
	beta := OperationSpec{
		Bindings: []BindingSpec{{Namespace: BindingBuiltin, Member: []string{"beta"}}},
		Input:    ValuesSpec{Tail: ValuesClosed},
		Outcomes: []OutcomeSpec{{Kind: flowkind.OutcomeNormal, Values: ValuesSpec{Tail: ValuesClosed}}},
		Effects:  RowSpec{Tail: RowClosed},
	}
	child := OperationSpec{Input: ValuesSpec{Tail: ValuesClosed}, Outcomes: []OutcomeSpec{{Kind: flowkind.OutcomeNormal, Values: ValuesSpec{Tail: ValuesClosed}}}, Effects: RowSpec{Tail: RowClosed}}
	first := makeSpec(3, []OperationSpec{alpha, beta, child})
	second := mustSeal(t, Spec{Operations: []OperationSpec{beta, child, func() OperationSpec {
		copy := alpha
		copy.Outcomes[0].Produced = []ProducedSpec{{Result: 0, Operation: 2}}
		return copy
	}()}})
	for _, contract := range []*Contract{first, second} {
		if contract.BoundOperationCount() != 2 {
			t.Fatalf("bound operation count = %d, want 2", contract.BoundOperationCount())
		}
		for index := 0; index < contract.BoundOperationCount(); index++ {
			op, ok := contract.BoundOperationAt(index)
			if !ok || op != Operation(index+1) {
				t.Fatalf("BoundOperationAt(%d) = %d/%v, want %d/true", index, op, ok, index+1)
			}
		}
		alphaOp, _ := contract.Lookup(BindingSpec{Namespace: BindingBuiltin, Member: []string{"alpha"}})
		betaOp, _ := contract.Lookup(BindingSpec{Namespace: BindingBuiltin, Member: []string{"beta"}})
		childOp, _, found := contract.ProducedForResult(alphaOp, 0, 0)
		if alphaOp != 1 || betaOp != 2 || !found || childOp != 3 {
			t.Fatalf("canonical prefix = alpha:%d beta:%d child:%d/%v", alphaOp, betaOp, childOp, found)
		}
	}
}

func TestProducedAnchorsRejectAmbiguityAndCycles(t *testing.T) {
	plain := func(name string, target SpecRef) OperationSpec {
		outcome := OutcomeSpec{Kind: flowkind.OutcomeNormal, Values: ValuesSpec{Fixed: []typ.Type{typ.Any}, Tail: ValuesClosed}}
		if target != 0 {
			outcome.Produced = []ProducedSpec{{Result: 0, Operation: target}}
		}
		return OperationSpec{Bindings: []BindingSpec{{Namespace: BindingBuiltin, Member: []string{name}}}, Input: ValuesSpec{Tail: ValuesClosed}, Outcomes: []OutcomeSpec{outcome}, Effects: RowSpec{Tail: RowClosed}}
	}
	if _, err := Seal(&Spec{Operations: []OperationSpec{{Input: ValuesSpec{Tail: ValuesClosed}, Outcomes: []OutcomeSpec{{Kind: flowkind.OutcomeNormal, Values: ValuesSpec{Tail: ValuesClosed}}}, Effects: RowSpec{Tail: RowClosed}}}}); err == nil {
		t.Fatal("unanchored produced-only operation accepted")
	}
	if _, err := Seal(&Spec{Operations: []OperationSpec{plain("left", 3), plain("right", 3), OperationSpec{Input: ValuesSpec{Tail: ValuesClosed}, Outcomes: []OutcomeSpec{{Kind: flowkind.OutcomeNormal, Values: ValuesSpec{Tail: ValuesClosed}}}, Effects: RowSpec{Tail: RowClosed}}}}); err == nil {
		t.Fatal("multiply anchored produced-only operation accepted")
	}
	if _, err := Seal(&Spec{Operations: []OperationSpec{
		{Input: ValuesSpec{Tail: ValuesClosed}, Outcomes: []OutcomeSpec{{Kind: flowkind.OutcomeNormal, Values: ValuesSpec{Fixed: []typ.Type{typ.Any}, Tail: ValuesClosed}, Produced: []ProducedSpec{{Result: 0, Operation: 2}}}}, Effects: RowSpec{Tail: RowClosed}},
		{Input: ValuesSpec{Tail: ValuesClosed}, Outcomes: []OutcomeSpec{{Kind: flowkind.OutcomeNormal, Values: ValuesSpec{Fixed: []typ.Type{typ.Any}, Tail: ValuesClosed}, Produced: []ProducedSpec{{Result: 0, Operation: 1}}}}, Effects: RowSpec{Tail: RowClosed}},
	}}); err == nil {
		t.Fatal("produced cycle accepted")
	}
}

func TestDeepProducedChainSealsIteratively(t *testing.T) {
	const depth = 4096
	operations := make([]OperationSpec, depth)
	for index := range operations {
		operations[index] = OperationSpec{
			Input:    ValuesSpec{Tail: ValuesClosed},
			Outcomes: []OutcomeSpec{{Kind: flowkind.OutcomeNormal, Values: ValuesSpec{Fixed: []typ.Type{typ.Any}, Tail: ValuesClosed}}},
			Effects:  RowSpec{Tail: RowClosed},
		}
		if index == 0 {
			operations[index].Bindings = []BindingSpec{{Namespace: BindingBuiltin, Member: []string{"root"}}}
		}
		if index+1 < len(operations) {
			operations[index].Outcomes[0].Produced = []ProducedSpec{{Result: 0, Operation: SpecRef(index + 2)}}
		}
	}
	contract := mustSeal(t, Spec{Operations: operations})
	if contract.OperationCount() != depth+1 || contract.BoundOperationCount() != 1 {
		t.Fatalf("deep chain operations = %d/%d", contract.OperationCount(), contract.BoundOperationCount())
	}
	current, ok := contract.Lookup(BindingSpec{Namespace: BindingBuiltin, Member: []string{"root"}})
	if !ok {
		t.Fatal("root binding missing")
	}
	for index := 1; index < depth; index++ {
		next, _, found := contract.ProducedForResult(current, 0, 0)
		if !found {
			t.Fatalf("chain ended at %d, want step %d", index, depth)
		}
		current = next
	}
	if contract.ProducedCount(current, 0) != 0 {
		t.Fatal("terminal produced operation has a successor")
	}
	if allocs := testing.AllocsPerRun(1000, func() {
		if _, _, found := contract.ProducedForResult(current, 0, 0); found {
			panic("terminal unexpectedly produced")
		}
	}); allocs != 0 {
		t.Fatalf("ProducedForResult allocated %f times", allocs)
	}
}
