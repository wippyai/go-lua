package target

import (
	schematype "github.com/wippyai/go-lua/analysis/schema/typecontract"
	"testing"

	flowkind "github.com/wippyai/go-lua/analysis/program/flow/kind"
)

func TestValuesSuffixCanonicalizationAndQueries(t *testing.T) {
	contract, err := testSeal(&Spec{Operations: []OperationSpec{{
		Bindings:   []BindingSpec{{Namespace: BindingBuiltin, Member: []string{"suffix"}}},
		ValuesVars: 1,
		Input:      ValuesSpec{Fixed: []schematype.Type{testString, testInteger}, Tail: ValuesClosed},
		Outcomes: []OutcomeSpec{
			{Kind: flowkind.OutcomeNormal, Values: ValuesSpec{Fixed: []schematype.Type{testString}, Tail: ValuesClosed, Suffix: []schematype.Type{testInteger}}},
			{Kind: flowkind.OutcomeThrow, Values: ValuesSpec{Fixed: []schematype.Type{testString}, Tail: ValuesVariable, Var: 0, Suffix: []schematype.Type{testInteger}}},
		},
		Effects: RowSpec{Tail: RowClosed},
	}}})
	if err != nil {
		t.Fatal(err)
	}
	op, ok := contract.Lookup(BindingSpec{Namespace: BindingBuiltin, Member: []string{"suffix"}})
	if !ok {
		t.Fatal("missing operation")
	}
	input, _ := contract.Input(op)
	_, closed, ok := contract.OutcomeAt(op, 0)
	if !ok || closed != input || contract.ValuesCount(closed) != 2 || contract.ValuesSuffixCount(closed) != 0 {
		t.Fatalf("closed suffix did not canonicalize into prefix: values=%d count=%d suffix=%d", closed, contract.ValuesCount(closed), contract.ValuesSuffixCount(closed))
	}
	_, open, ok := contract.OutcomeAt(op, 1)
	if !ok || contract.ValuesCount(open) != 1 || contract.ValuesSuffixCount(open) != 1 {
		t.Fatalf("open Values shape = prefix %d suffix %d", contract.ValuesCount(open), contract.ValuesSuffixCount(open))
	}
	prefix, prefixOK := contract.ValuesAt(open, 0)
	suffix, suffixOK := contract.ValuesSuffixAt(open, 0)
	if !prefixOK || !suffixOK || prefix == suffix {
		t.Fatalf("open Values query = prefix %d/%v suffix %d/%v", prefix, prefixOK, suffix, suffixOK)
	}
	if _, ok := contract.ValuesSuffixAt(open, 1); ok {
		t.Fatal("suffix outside scope accepted")
	}
	if got := testing.AllocsPerRun(100, func() {
		_, _ = contract.ValuesAt(open, 0)
		_, _ = contract.ValuesSuffixAt(open, 0)
		_ = contract.ValuesSuffixCount(open)
	}); got != 0 {
		t.Fatalf("Values suffix queries allocated %f times", got)
	}
}

func TestValuesSuffixRejectsInvalidInputAndTypes(t *testing.T) {
	base := OperationSpec{
		Input:    ValuesSpec{Tail: ValuesClosed},
		Outcomes: []OutcomeSpec{{Kind: flowkind.OutcomeNormal, Values: ValuesSpec{Tail: ValuesClosed}}},
		Effects:  RowSpec{Tail: RowClosed},
	}
	for _, test := range []struct {
		name string
		edit func(*OperationSpec)
	}{
		{"input suffix", func(op *OperationSpec) { op.Input.Suffix = []schematype.Type{testString} }},
		{"nil suffix type", func(op *OperationSpec) { op.Outcomes[0].Values.Suffix = []schematype.Type{{}} }},
	} {
		t.Run(test.name, func(t *testing.T) {
			op := base
			op.Outcomes = append([]OutcomeSpec(nil), base.Outcomes...)
			test.edit(&op)
			if _, err := testSeal(&Spec{Operations: []OperationSpec{op}}); err == nil {
				t.Fatal("Seal accepted invalid suffix")
			}
		})
	}
}

func TestValuesSuffixOutcomePermutationHasOnePublicContract(t *testing.T) {
	makeContract := func(outcomes []OutcomeSpec) *Contract {
		contract, err := testSeal(&Spec{Operations: []OperationSpec{{
			Bindings:   []BindingSpec{{Namespace: BindingBuiltin, Member: []string{"suffix-permutation"}}},
			ValuesVars: 1,
			Input:      ValuesSpec{Fixed: []schematype.Type{testString}, Tail: ValuesClosed},
			Outcomes:   outcomes,
			Effects:    RowSpec{Tail: RowClosed},
		}}})
		if err != nil {
			t.Fatal(err)
		}
		return contract
	}
	normal := OutcomeSpec{Kind: flowkind.OutcomeNormal, Values: ValuesSpec{Fixed: []schematype.Type{testString}, Tail: ValuesVariable, Var: 0, Suffix: []schematype.Type{testInteger}}}
	throwing := OutcomeSpec{Kind: flowkind.OutcomeThrow, Values: ValuesSpec{Tail: ValuesClosed, Suffix: []schematype.Type{testString}}}
	left := makeContract([]OutcomeSpec{normal, throwing})
	right := makeContract([]OutcomeSpec{throwing, normal})
	assertPublicContractEqual(t, left, right)
}

func TestWideValuesSuffixSealsAndQueries(t *testing.T) {
	const width = 2048
	suffix := make([]schematype.Type, width)
	for index := range suffix {
		suffix[index] = testInteger
	}
	contract, err := testSeal(&Spec{Operations: []OperationSpec{{
		Bindings:   []BindingSpec{{Namespace: BindingBuiltin, Member: []string{"wide-suffix"}}},
		ValuesVars: 1,
		Input:      ValuesSpec{Tail: ValuesVariable, Var: 0},
		Outcomes:   []OutcomeSpec{{Kind: flowkind.OutcomeNormal, Values: ValuesSpec{Tail: ValuesVariable, Var: 0, Suffix: suffix}}},
		Effects:    RowSpec{Tail: RowClosed},
	}}})
	if err != nil {
		t.Fatal(err)
	}
	op, _ := contract.OperationAt(0)
	_, values, _ := contract.OutcomeAt(op, 0)
	if got := contract.ValuesSuffixCount(values); got != width {
		t.Fatalf("ValuesSuffixCount = %d, want %d", got, width)
	}
	if _, ok := contract.ValuesSuffixAt(values, width-1); !ok {
		t.Fatal("last wide suffix type absent")
	}
}
