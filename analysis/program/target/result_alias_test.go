package target

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/domain/type/typ"
	flowkind "github.com/wippyai/go-lua/analysis/program/flow/kind"
)

func TestResultAliasesCanonicalizeAndRemainConjunctive(t *testing.T) {
	contract, err := Seal(&Spec{Operations: []OperationSpec{
		{
			Bindings: []BindingSpec{{Namespace: BindingBuiltin, Member: []string{"aliases"}}},
			Input:    ValuesSpec{Fixed: []typ.Type{typ.Any, typ.String}, Tail: ValuesClosed},
			Outcomes: []OutcomeSpec{{
				Kind: flowkind.OutcomeNormal, Values: ValuesSpec{Fixed: []typ.Type{typ.Any, typ.String}, Tail: ValuesClosed},
				Produced: []ProducedSpec{{Result: 0, Operation: 2}},
				ResultAliases: []ResultAliasSpec{
					{Result: 1, Source: InputSource{Kind: InputSourceValueFormal, Ordinal: 1}},
					{Result: 0, Source: InputSource{Kind: InputSourceValueFormal, Ordinal: 0}},
				},
			}},
			Effects: RowSpec{Tail: RowClosed},
		},
		{Input: ValuesSpec{Tail: ValuesClosed}, Outcomes: []OutcomeSpec{{Kind: flowkind.OutcomeNormal, Values: ValuesSpec{Tail: ValuesClosed}}}, Effects: RowSpec{Tail: RowClosed}},
	}})
	if err != nil {
		t.Fatal(err)
	}
	op, ok := contract.Lookup(BindingSpec{Namespace: BindingBuiltin, Member: []string{"aliases"}})
	if !ok || contract.ResultAliasCount(op, 0) != 2 {
		t.Fatalf("result aliases missing: op=%d ok=%v count=%d", op, ok, contract.ResultAliasCount(op, 0))
	}
	result, kind, source, ok := contract.ResultAliasAt(op, 0, 0)
	if !ok || result != 0 || kind != InputSourceValueFormal || source != 0 {
		t.Fatalf("first alias = %d/%d/%d/%v", result, kind, source, ok)
	}
	kind, source, row, ok := contract.ResultAliasForResult(op, 0, 1)
	if !ok || kind != InputSourceValueFormal || source != 1 || row != 1 {
		t.Fatalf("alias lookup = %d/%d/%d/%v", kind, source, row, ok)
	}
	if _, _, _, ok := contract.ResultAliasForResult(op, 0, 2); ok {
		t.Fatal("unknown result alias found")
	}
	if got := testing.AllocsPerRun(100, func() { _, _, _, _ = contract.ResultAliasForResult(op, 0, 1) }); got != 0 {
		t.Fatalf("ResultAliasForResult allocated %f times", got)
	}
}

func TestResultAliasCoexistsWithCallbackResult(t *testing.T) {
	contract, err := Seal(&Spec{Operations: []OperationSpec{{
		Bindings:   []BindingSpec{{Namespace: BindingBuiltin, Member: []string{"callback-alias"}}},
		ValuesVars: 5,
		Input:      ValuesSpec{Fixed: []typ.Type{typ.Any}, Tail: ValuesClosed},
		Callbacks:  []CallbackSpec{{Function: InputSource{Kind: InputSourceValueFormal}, Admission: OrdinaryCallable, Arguments: callbackTail(0), Outcomes: callbackOutcomes(1, 1, 2, 3, 4), Lifecycle: CallbackRetainedOptionalOnce, Effects: RowSpec{Tail: RowClosed}}},
		Outcomes: []OutcomeSpec{{
			Kind:            flowkind.OutcomeNormal,
			Values:          ValuesSpec{Fixed: []typ.Type{typ.Any}, Tail: ValuesClosed},
			CallbackResults: []CallbackResultSpec{{Result: 0, Callback: 1}},
			ResultAliases:   []ResultAliasSpec{{Result: 0, Source: InputSource{Kind: InputSourceValueFormal}}},
		}},
		Effects: RowSpec{Tail: RowClosed},
	}}})
	if err != nil {
		t.Fatal(err)
	}
	op, ok := contract.Lookup(BindingSpec{Namespace: BindingBuiltin, Member: []string{"callback-alias"}})
	if !ok || contract.CallbackResultCount(op, 0) != 1 || contract.ResultAliasCount(op, 0) != 1 {
		t.Fatalf("callback/alias conjunction missing: op=%d ok=%v callbacks=%d aliases=%d", op, ok, contract.CallbackResultCount(op, 0), contract.ResultAliasCount(op, 0))
	}
}

func TestResultAliasesRejectInvalidAndDuplicate(t *testing.T) {
	base := OperationSpec{
		Input:    ValuesSpec{Fixed: []typ.Type{typ.String}, Tail: ValuesClosed},
		Outcomes: []OutcomeSpec{{Kind: flowkind.OutcomeNormal, Values: ValuesSpec{Fixed: []typ.Type{typ.String}, Tail: ValuesClosed}}},
		Effects:  RowSpec{Tail: RowClosed},
	}
	for _, test := range []struct {
		name    string
		aliases []ResultAliasSpec
	}{
		{"result outside prefix", []ResultAliasSpec{{Result: 1, Source: InputSource{Kind: InputSourceValueFormal}}}},
		{"Values variable source", []ResultAliasSpec{{Result: 0, Source: InputSource{Kind: InputSourceValuesVar}}}},
		{"formal outside scope", []ResultAliasSpec{{Result: 0, Source: InputSource{Kind: InputSourceValueFormal, Ordinal: 1}}}},
		{"duplicate result", []ResultAliasSpec{{Result: 0, Source: InputSource{Kind: InputSourceValueFormal}}, {Result: 0, Source: InputSource{Kind: InputSourceValueFormal}}}},
	} {
		t.Run(test.name, func(t *testing.T) {
			op := base
			op.Outcomes = append([]OutcomeSpec(nil), base.Outcomes...)
			op.Outcomes[0].ResultAliases = test.aliases
			if _, err := Seal(&Spec{Operations: []OperationSpec{op}}); err == nil {
				t.Fatal("Seal accepted invalid result alias")
			}
		})
	}
}

func TestResultAliasPermutationAndWideLookup(t *testing.T) {
	const width = 1024
	fixed := make([]typ.Type, width)
	aliases := make([]ResultAliasSpec, width)
	for index := range fixed {
		fixed[index] = typ.Any
		aliases[width-index-1] = ResultAliasSpec{
			Result: uint32(width - index - 1), Source: InputSource{Kind: InputSourceValueFormal, Ordinal: uint32(width - index - 1)},
		}
	}
	seal := func(aliasRows []ResultAliasSpec) *Contract {
		contract, err := Seal(&Spec{Operations: []OperationSpec{{
			Bindings: []BindingSpec{{Namespace: BindingBuiltin, Member: []string{"wide-alias"}}},
			Input:    ValuesSpec{Fixed: fixed, Tail: ValuesClosed},
			Outcomes: []OutcomeSpec{{Kind: flowkind.OutcomeNormal, Values: ValuesSpec{Fixed: fixed, Tail: ValuesClosed}, ResultAliases: aliasRows}},
			Effects:  RowSpec{Tail: RowClosed},
		}}})
		if err != nil {
			t.Fatal(err)
		}
		return contract
	}
	left := seal(aliases)
	sorted := append([]ResultAliasSpec(nil), aliases...)
	for index := range sorted {
		sorted[index] = ResultAliasSpec{Result: uint32(index), Source: InputSource{Kind: InputSourceValueFormal, Ordinal: uint32(index)}}
	}
	right := seal(sorted)
	assertPublicContractEqual(t, left, right)
	op, _ := left.Lookup(BindingSpec{Namespace: BindingBuiltin, Member: []string{"wide-alias"}})
	if kind, source, row, ok := left.ResultAliasForResult(op, 0, width-1); !ok || kind != InputSourceValueFormal || source != width-1 || row != width-1 {
		t.Fatalf("wide alias lookup = %d/%d/%d/%v", kind, source, row, ok)
	}
	if got := testing.AllocsPerRun(100, func() { _, _, _, _ = left.ResultAliasForResult(op, 0, width-1) }); got != 0 {
		t.Fatalf("wide ResultAliasForResult allocated %f times", got)
	}
}
