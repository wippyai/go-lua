package target

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/domain/type/typ"
	flowkind "github.com/wippyai/go-lua/analysis/program/flow/kind"
)

func TestFreshResultsSealDenselyAndAllowConjunctiveRelations(t *testing.T) {
	child := OperationSpec{Input: ValuesSpec{Tail: ValuesClosed}, Outcomes: []OutcomeSpec{{Kind: flowkind.OutcomeNormal, Values: ValuesSpec{Tail: ValuesClosed}}}, Effects: RowSpec{Tail: RowClosed}}
	contract := mustSeal(t, Spec{Operations: []OperationSpec{{
		Bindings:   []BindingSpec{{Namespace: BindingBuiltin, Member: []string{"fresh"}}},
		ValuesVars: 1,
		Input:      ValuesSpec{Fixed: []typ.Type{typ.Any}, Tail: ValuesClosed},
		Callbacks: []CallbackSpec{{
			Function: InputSource{Kind: InputSourceValueFormal}, Admission: OrdinaryCallable, Arguments: callbackTail(0),
			Outcomes: callbackOutcomes(0, 0, 0, 0, 0), Lifecycle: CallbackRetainedOptionalOnce,
			Effects: RowSpec{Tail: RowClosed},
		}},
		Outcomes: []OutcomeSpec{
			{Kind: flowkind.OutcomeNormal, Values: ValuesSpec{Fixed: []typ.Type{typ.Any, typ.Any}, Tail: ValuesClosed},
				FreshResults: []FreshResultSpec{{Result: 1, Kind: FreshFunction}, {Result: 0, Kind: FreshTable}},
				Produced:     []ProducedSpec{{Result: 0, Operation: 2}},
			},
			{Kind: flowkind.OutcomeThrow, Values: ValuesSpec{Fixed: []typ.Type{typ.Any}, Tail: ValuesClosed},
				FreshResults:    []FreshResultSpec{{Result: 0, Kind: FreshError}},
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
		kind    FreshKind
	}{{0, 0, FreshTable}, {1, 1, FreshFunction}} {
		result, ordinal, kind, found := contract.FreshResultAt(op, 0, index)
		if !found || result != want.result || ordinal != want.ordinal || kind != want.kind {
			t.Fatalf("FreshResultAt(%d) = %d/%d/%d/%v, want %d/%d/%d/true", index, result, ordinal, kind, found, want.result, want.ordinal, want.kind)
		}
	}
	if ordinal, kind, index, found := contract.FreshResultForResult(op, 0, 1); !found || ordinal != 1 || kind != FreshFunction || index != 1 {
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
		Outcomes: []OutcomeSpec{{Kind: flowkind.OutcomeNormal, Values: ValuesSpec{Fixed: []typ.Type{typ.Any, typ.Any}, Tail: ValuesClosed}, FreshResults: []FreshResultSpec{{Result: 0, Kind: FreshTable}}}},
		Effects:  RowSpec{Tail: RowClosed},
	}
	for _, test := range []struct {
		name string
		edit func(*OperationSpec)
	}{
		{"duplicate fixed result", func(op *OperationSpec) {
			op.Outcomes[0].FreshResults = append(op.Outcomes[0].FreshResults, FreshResultSpec{Result: 0, Kind: FreshFunction})
		}},
		{"outside fixed prefix", func(op *OperationSpec) { op.Outcomes[0].FreshResults[0].Result = 2 }},
		{"invalid kind", func(op *OperationSpec) { op.Outcomes[0].FreshResults[0].Kind = FreshInvalid }},
		{"result alias overlap", func(op *OperationSpec) {
			op.Outcomes[0].ResultAliases = []ResultAliasSpec{{Result: 0, Source: InputSource{Kind: InputSourceValueFormal}}}
			op.Input.Fixed = []typ.Type{typ.Any}
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			op := base
			op.Input.Fixed = append([]typ.Type(nil), base.Input.Fixed...)
			op.Outcomes = append([]OutcomeSpec(nil), base.Outcomes...)
			op.Outcomes[0].FreshResults = append([]FreshResultSpec(nil), base.Outcomes[0].FreshResults...)
			test.edit(&op)
			if _, err := Seal(&Spec{Operations: []OperationSpec{op}}); err == nil {
				t.Fatal("Seal accepted invalid FreshResult relation")
			}
		})
	}
}

func TestFreshResultDistinguishesSameShapeCasesAndProducedAnchors(t *testing.T) {
	caseResult := func(kind FreshKind, child SpecRef) OutcomeSpec {
		return OutcomeSpec{
			Kind:         flowkind.OutcomeNormal,
			Values:       ValuesSpec{Fixed: []typ.Type{typ.Any}, Tail: ValuesClosed},
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
	left := mustSeal(t, makeSpec([]OutcomeSpec{caseResult(FreshTable, 2), caseResult(FreshFunction, 3)}))
	right := mustSeal(t, makeSpec([]OutcomeSpec{caseResult(FreshFunction, 3), caseResult(FreshTable, 2)}))
	assertPublicContractEqual(t, left, right)
	if left.ContentID() != right.ContentID() {
		t.Fatal("FreshResult outcome permutation changed ContentID")
	}
	op, _ := left.Lookup(BindingSpec{Namespace: BindingBuiltin, Member: []string{"fresh-cases"}})
	if count := left.OutcomeCount(op); count != 2 {
		t.Fatalf("same-shape Fresh outcomes = %d, want 2", count)
	}
}
