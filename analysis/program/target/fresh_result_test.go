package target

import (
	"testing"

	schematype "github.com/wippyai/go-lua/analysis/schema/typecontract"

	flowkind "github.com/wippyai/go-lua/analysis/program/flow/kind"
)

func TestFreshResultsSealDenselyAndAllowConjunctiveRelations(t *testing.T) {
	child := OperationSpec{Input: ValuesSpec{Tail: ValuesClosed}, Outcomes: []OutcomeSpec{{Kind: flowkind.OutcomeNormal, Values: ValuesSpec{Tail: ValuesClosed}}}, Effects: RowSpec{Tail: RowClosed}}
	contract := mustSeal(t, Spec{Operations: []OperationSpec{{
		Bindings:   []BindingSpec{{Namespace: BindingBuiltin, Member: []string{"fresh"}}},
		ValuesVars: 1,
		Input:      ValuesSpec{Fixed: []schematype.Type{testAny}, Tail: ValuesClosed},
		Callbacks: []CallbackSpec{{
			Function: InputSource{Kind: InputSourceValueFormal}, Admission: schematype.CallableAdmissionOrdinary, Arguments: callbackTail(0),
			Outcomes: callbackOutcomes(0, 0, 0, 0, 0), Lifecycle: CallbackRetainedOptionalOnce,
			Effects: RowSpec{Tail: RowClosed},
		}},
		Outcomes: []OutcomeSpec{
			{Kind: flowkind.OutcomeNormal, Values: ValuesSpec{Fixed: []schematype.Type{testAny, testAny}, Tail: ValuesClosed},
				FreshResults: []FreshResultSpec{{Result: 1, Kind: schematype.FreshClassFunction}, {Result: 0, Kind: schematype.FreshClassTable}},
				Produced:     []ProducedSpec{{Result: 0, Operation: 2}},
			},
			{Kind: flowkind.OutcomeThrow, Values: ValuesSpec{Fixed: []schematype.Type{testAny}, Tail: ValuesClosed},
				FreshResults:    []FreshResultSpec{{Result: 0, Kind: schematype.FreshClassError}},
				CallbackResults: []CallbackResultSpec{{Result: 0, Callback: 1}},
			},
		},
		Effects: RowSpec{Tail: RowClosed},
	}, child}})
	op, _ := contract.Lookup(BindingSpec{Namespace: BindingBuiltin, Member: []string{"fresh"}})
	if count := contract.FreshResultCount(op, 0); count != 2 {
		t.Fatalf("normal fresh count = %d, want 2", count)
	}
	for index, want := range []struct {
		result  uint32
		ordinal uint32
		kind    schematype.FreshClass
	}{{0, 0, schematype.FreshClassTable}, {1, 1, schematype.FreshClassFunction}} {
		result, ordinal, kind, found := contract.FreshResultAt(op, 0, index)
		if !found || result != want.result || ordinal != want.ordinal || kind != want.kind {
			t.Fatalf("FreshResultAt(%d) = %d/%d/%d/%v, want %d/%d/%d/true", index, result, ordinal, kind, found, want.result, want.ordinal, want.kind)
		}
	}
	if ordinal, kind, index, found := contract.FreshResultForResult(op, 0, 1); !found || ordinal != 1 || kind != schematype.FreshClassFunction || index != 1 {
		t.Fatalf("FreshResultForResult = %d/%d/%d/%v, want 1/function/1/true", ordinal, kind, index, found)
	}
	if _, _, _, found := contract.FreshResultAt(op, 0, 2); found {
		t.Fatal("FreshResultAt accepted outside range")
	}
	if allocations := testing.AllocsPerRun(1000, func() {
		_ = contract.FreshResultCount(op, 0)
		_, _, _, _ = contract.FreshResultAt(op, 0, 1)
		_, _, _, _ = contract.FreshResultForResult(op, 0, 1)
	}); allocations != 0 {
		t.Fatalf("FreshResult queries allocated %f times", allocations)
	}
}

func TestFreshResultsRejectAliasesDuplicatesAndInvalidKinds(t *testing.T) {
	base := OperationSpec{
		Input:    ValuesSpec{Tail: ValuesClosed},
		Outcomes: []OutcomeSpec{{Kind: flowkind.OutcomeNormal, Values: ValuesSpec{Fixed: []schematype.Type{testAny, testAny}, Tail: ValuesClosed}, FreshResults: []FreshResultSpec{{Result: 0, Kind: schematype.FreshClassTable}}}},
		Effects:  RowSpec{Tail: RowClosed},
	}
	for _, test := range []struct {
		name string
		edit func(*OperationSpec)
	}{
		{"duplicate fixed result", func(op *OperationSpec) {
			op.Outcomes[0].FreshResults = append(op.Outcomes[0].FreshResults, FreshResultSpec{Result: 0, Kind: schematype.FreshClassFunction})
		}},
		{"outside fixed prefix", func(op *OperationSpec) { op.Outcomes[0].FreshResults[0].Result = 2 }},
		{"invalid kind", func(op *OperationSpec) { op.Outcomes[0].FreshResults[0].Kind = schematype.FreshClassInvalid }},
		{"result alias overlap", func(op *OperationSpec) {
			op.Outcomes[0].ResultAliases = []ResultAliasSpec{{Result: 0, Source: InputSource{Kind: InputSourceValueFormal}}}
			op.Input.Fixed = []schematype.Type{testAny}
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			op := base
			op.Input.Fixed = append([]schematype.Type(nil), base.Input.Fixed...)
			op.Outcomes = append([]OutcomeSpec(nil), base.Outcomes...)
			op.Outcomes[0].FreshResults = append([]FreshResultSpec(nil), base.Outcomes[0].FreshResults...)
			test.edit(&op)
			if _, err := testSeal(&Spec{Operations: []OperationSpec{op}}); err == nil {
				t.Fatal("Seal accepted invalid FreshResult relation")
			}
		})
	}
}

func TestFreshResultDistinguishesSameShapeCasesAndProducedAnchors(t *testing.T) {
	caseResult := func(kind schematype.FreshClass, child SpecRef) OutcomeSpec {
		return OutcomeSpec{
			Kind:         flowkind.OutcomeNormal,
			Values:       ValuesSpec{Fixed: []schematype.Type{testAny}, Tail: ValuesClosed},
			FreshResults: []FreshResultSpec{{Result: 0, Kind: kind}},
			Produced:     []ProducedSpec{{Result: 0, Operation: child}},
		}
	}
	child := func() OperationSpec {
		return OperationSpec{Input: ValuesSpec{Tail: ValuesClosed}, Outcomes: []OutcomeSpec{{Kind: flowkind.OutcomeNormal, Values: ValuesSpec{Tail: ValuesClosed}}}, Effects: RowSpec{Tail: RowClosed}}
	}
	makeSpec := func(outcomes []OutcomeSpec) Spec {
		return Spec{Operations: []OperationSpec{{
			Bindings: []BindingSpec{{Namespace: BindingBuiltin, Member: []string{"fresh-cases"}}},
			Input:    ValuesSpec{Tail: ValuesClosed}, Outcomes: outcomes, Effects: RowSpec{Tail: RowClosed},
		}, child(), child()}}
	}
	left := mustSeal(t, makeSpec([]OutcomeSpec{caseResult(schematype.FreshClassTable, 2), caseResult(schematype.FreshClassFunction, 3)}))
	right := mustSeal(t, makeSpec([]OutcomeSpec{caseResult(schematype.FreshClassFunction, 3), caseResult(schematype.FreshClassTable, 2)}))
	assertPublicContractEqual(t, left, right)
	if left.ContentID() != right.ContentID() {
		t.Fatal("FreshResult outcome permutation changed ContentID")
	}
	op, _ := left.Lookup(BindingSpec{Namespace: BindingBuiltin, Member: []string{"fresh-cases"}})
	if count := left.OutcomeCount(op); count != 2 {
		t.Fatalf("same-shape Fresh outcomes = %d, want 2", count)
	}
}
