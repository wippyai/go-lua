package target

import (
	"bytes"
	"testing"

	"github.com/wippyai/go-lua/analysis/domain/type/annotation"
	"github.com/wippyai/go-lua/analysis/domain/type/typ"
	"github.com/wippyai/go-lua/analysis/domain/type/typeexpr"
	flowkind "github.com/wippyai/go-lua/analysis/program/flow/kind"
)

func TestSealDeterministicAcrossOperationOrder(t *testing.T) {
	first := mustSeal(t, Spec{Operations: []OperationSpec{
		builtin("alpha", typ.String, effect(2, []ValueFormal{0})),
		builtin("beta", typ.String, RowSpec{Tail: RowClosed}),
	}})
	second := mustSeal(t, Spec{Operations: []OperationSpec{
		builtin("beta", typ.String, RowSpec{Tail: RowClosed}),
		builtin("alpha", typ.String, effect(1, []ValueFormal{0})),
	}})
	assertContractShapeEqual(t, first, second)
	alpha := BindingSpec{Namespace: BindingBuiltin, Member: []string{"alpha"}}
	for _, contract := range []*Contract{first, second} {
		op, ok := contract.Lookup(alpha)
		if !ok || op != 1 {
			t.Fatalf("Lookup(alpha) = %d/%v, want 1/true", op, ok)
		}
		if got := contract.EffectCount(op); got != 1 {
			t.Fatalf("alpha effect count = %d, want 1", got)
		}
		target, ok := contract.EffectTarget(op, 0)
		if !ok || target != 2 {
			t.Fatalf("alpha effect target = %d/%v, want 2/true", target, ok)
		}
	}
}

func TestTypeFormalsAreAlphaInvariant(t *testing.T) {
	left := typ.NewTypeParam("T", typ.String)
	right := typ.NewTypeParam("Value", typ.String)
	leftContract := mustSeal(t, Spec{Operations: []OperationSpec{genericBuiltin("identity", left)}})
	rightContract := mustSeal(t, Spec{Operations: []OperationSpec{genericBuiltin("identity", right)}})
	assertPublicContractEqual(t, leftContract, rightContract)
	leftOp, _ := leftContract.Lookup(BindingSpec{Namespace: BindingBuiltin, Member: []string{"identity"}})
	rightOp, _ := rightContract.Lookup(BindingSpec{Namespace: BindingBuiltin, Member: []string{"identity"}})
	leftInput, _ := leftContract.Input(leftOp)
	rightInput, _ := rightContract.Input(rightOp)
	leftType, _ := leftContract.ValuesAt(leftInput, 0)
	rightType, _ := rightContract.ValuesAt(rightInput, 0)
	leftBytes, _ := leftContract.TypeBytes(leftType)
	rightBytes, _ := rightContract.TypeBytes(rightType)
	if !bytes.Equal(leftBytes, rightBytes) {
		t.Fatal("alpha-renamed type formal changed frozen target type bytes")
	}
	leftConstraint, ok := leftContract.TypeFormalConstraint(leftOp, 0)
	if !ok || leftConstraint == 0 {
		t.Fatal("missing frozen formal constraint")
	}
}

func TestGenericAndRecursiveTypesFreezeWithoutRawRetention(t *testing.T) {
	outer := typ.NewTypeParam("T", nil)
	inner := typ.NewTypeParam("Element", nil)
	channel := typ.NewGeneric("Channel", []*typ.TypeParam{inner}, typ.NewArray(inner))
	channelOfOuter := typ.Instantiate(channel, outer)
	recursive := typ.NewRecursive("Node", func(self typ.Type) typ.Type { return typeexpr.Optional(self) })
	contract := mustSeal(t, Spec{Operations: []OperationSpec{{
		Bindings:    []BindingSpec{{Namespace: BindingProvider, Owner: []string{"channel"}, Member: []string{"new"}}},
		TypeFormals: []*typ.TypeParam{outer},
		Input:       ValuesSpec{Fixed: []typ.Type{channelOfOuter, recursive}, Tail: ValuesClosed},
		Outcomes:    []OutcomeSpec{{Kind: flowkind.OutcomeNormal, Values: ValuesSpec{Fixed: []typ.Type{channelOfOuter}, Tail: ValuesClosed}}},
		Effects:     RowSpec{Tail: RowClosed},
	}}})
	op, _ := contract.Lookup(BindingSpec{Namespace: BindingProvider, Owner: []string{"channel"}, Member: []string{"new"}})
	input, _ := contract.Input(op)
	for index := 0; index < contract.ValuesCount(input); index++ {
		frozen, _ := contract.ValuesAt(input, index)
		if data, ok := contract.TypeBytes(frozen); !ok || len(data) == 0 {
			t.Fatalf("frozen type %d unavailable", index)
		}
	}
}

func TestDeepAuthoringTypeUsesNoGoRecursion(t *testing.T) {
	var deep typ.Type = typ.String
	for index := 0; index < 20000; index++ {
		deep = typ.NewArray(deep)
	}
	contract := mustSeal(t, Spec{Operations: []OperationSpec{builtin("deep", deep, RowSpec{Tail: RowClosed})}})
	op, _ := contract.Lookup(BindingSpec{Namespace: BindingBuiltin, Member: []string{"deep"}})
	input, _ := contract.Input(op)
	frozen, _ := contract.ValuesAt(input, 0)
	if data, ok := contract.TypeBytes(frozen); !ok || len(data) == 0 {
		t.Fatal("deep type did not freeze")
	}
	if !contract.ContentID().Available() {
		t.Fatal("deep sealed type has no ContentID")
	}
}

func TestSealOwnsInputsAndConsumesFailedSpec(t *testing.T) {
	record := typ.RebuildRecord(typ.RecordParts{Fields: []typ.Field{{Name: "value", Type: typ.String}}})
	operations := []OperationSpec{builtin("owned", record, RowSpec{Tail: RowClosed})}
	spec := Spec{Operations: operations}
	contract, err := Seal(&spec)
	if err != nil {
		t.Fatalf("Seal: %v", err)
	}
	contentID := contract.ContentID()
	op, _ := contract.Lookup(BindingSpec{Namespace: BindingBuiltin, Member: []string{"owned"}})
	input, _ := contract.Input(op)
	frozen, _ := contract.ValuesAt(input, 0)
	before, _ := contract.TypeBytes(frozen)
	record.Fields[0].Type = typ.Number
	operations[0].Bindings[0].Member[0] = "mutated"
	if _, found := contract.Lookup(BindingSpec{Namespace: BindingBuiltin, Member: []string{"owned"}}); !found {
		t.Fatal("caller mutation changed sealed binding")
	}
	after, _ := contract.TypeBytes(frozen)
	if !bytes.Equal(before, after) || len(before) == 0 {
		t.Fatal("caller type mutation changed frozen bytes")
	}
	if contentID != contract.ContentID() {
		t.Fatal("caller mutation changed sealed ContentID")
	}
	if _, err := Seal(&spec); err == nil {
		t.Fatal("successful Seal did not consume spec")
	}

	bad := Spec{Operations: []OperationSpec{{Bindings: []BindingSpec{{Namespace: BindingBuiltin, Member: []string{"bad"}}}, Input: ValuesSpec{Tail: ValuesUnknown}}}}
	if _, err := Seal(&bad); err == nil {
		t.Fatal("bad Seal unexpectedly succeeded")
	}
	bad.Operations = []OperationSpec{builtin("replacement", typ.String, RowSpec{Tail: RowClosed})}
	if _, err := Seal(&bad); err == nil || err.Error() != "target: consumed spec" {
		t.Fatalf("failed Seal did not consume replacement input: %v", err)
	}
}

func TestTargetTypeValidationRejectsErasureAndUnresolvedAuthority(t *testing.T) {
	foreign := typ.NewTypeParam("Foreign", nil)
	cases := []struct {
		name  string
		value typ.Type
	}{
		{name: "annotated", value: typ.NewAnnotated(typ.String, []annotation.Annotation{{Name: "min", Arg: annotation.IntArg(1)}})},
		{name: "unknown", value: typ.Unknown},
		{name: "self", value: typ.Self},
		{name: "ref", value: typ.NewRef("module", "T")},
		{name: "foreign formal", value: foreign},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			if _, err := Seal(&Spec{Operations: []OperationSpec{builtin("reject", test.value, RowSpec{Tail: RowClosed})}}); err == nil {
				t.Fatalf("%s type was accepted", test.name)
			}
		})
	}
}

func TestBindingInputAndValuesTails(t *testing.T) {
	contract := mustSeal(t, Spec{Operations: []OperationSpec{{
		Bindings:   []BindingSpec{{Namespace: BindingProvider, Owner: []string{"channel"}, Member: []string{"send"}}},
		ValuesVars: 1,
		Input:      ValuesSpec{Fixed: []typ.Type{typ.String, typ.String}, Tail: ValuesVariable, Var: 0},
		Outcomes:   []OutcomeSpec{{Kind: flowkind.OutcomeNormal, Values: ValuesSpec{Tail: ValuesVariable, Var: 0}}},
		Effects:    RowSpec{Tail: RowClosed},
	}}})
	binding := BindingSpec{Namespace: BindingProvider, Owner: []string{"channel"}, Member: []string{"send"}}
	op, ok := contract.Lookup(binding)
	if !ok {
		t.Fatal("method binding not found")
	}
	if got := contract.ValueFormalCount(op); got != 2 {
		t.Fatalf("ValueFormalCount = %d, want two fixed inputs", got)
	}
	if got := contract.ValuesVarCount(op); got != 1 {
		t.Fatalf("ValuesVarCount = %d, want 1", got)
	}
	input, _ := contract.Input(op)
	if tail, variable, ok := contract.ValuesTail(input); !ok || tail != ValuesVariable || variable != 0 {
		t.Fatalf("input tail = %d/%d/%v", tail, variable, ok)
	}
}

func TestCorrelatedOutcomesRejectDuplicates(t *testing.T) {
	base := builtin("receive", typ.String, RowSpec{Tail: RowClosed})
	base.Outcomes = []OutcomeSpec{
		{Kind: flowkind.OutcomeNormal, Values: ValuesSpec{Fixed: []typ.Type{typ.String}, Tail: ValuesClosed}},
		{Kind: flowkind.OutcomeNormal, Values: ValuesSpec{Fixed: []typ.Type{typ.Boolean}, Tail: ValuesClosed}},
		{Kind: flowkind.OutcomeThrow, Values: ValuesSpec{Fixed: []typ.Type{typ.String}, Tail: ValuesClosed}},
	}
	contract := mustSeal(t, Spec{Operations: []OperationSpec{base}})
	op, _ := contract.Lookup(BindingSpec{Namespace: BindingBuiltin, Member: []string{"receive"}})
	if got := contract.OutcomeCount(op); got != 3 {
		t.Fatalf("outcome count = %d, want 3", got)
	}
	dup := base
	dup.Outcomes = append(append([]OutcomeSpec(nil), base.Outcomes...), base.Outcomes[0])
	if _, err := Seal(&Spec{Operations: []OperationSpec{dup}}); err == nil {
		t.Fatal("duplicate correlated outcome accepted")
	}
}

func TestEffectMultiplicityAndArgumentValidation(t *testing.T) {
	contract := mustSeal(t, Spec{Operations: []OperationSpec{
		builtin("store", typ.String, RowSpec{Tail: RowClosed}),
		builtin("send", typ.String, RowSpec{Tail: RowClosed}),
	}})
	_ = contract
	withEffects := []OperationSpec{
		builtin("store", typ.String, RowSpec{Tail: RowClosed}),
		builtin("send", typ.String, RowSpec{Occurrences: []EffectSpec{
			{Target: 1, ValueArgs: []ValueFormal{0}},
			{Target: 1, ValueArgs: []ValueFormal{0}},
		}, Tail: RowClosed}),
	}
	sealed := mustSeal(t, Spec{Operations: withEffects})
	send, _ := sealed.Lookup(BindingSpec{Namespace: BindingBuiltin, Member: []string{"send"}})
	if got := sealed.EffectCount(send); got != 2 {
		t.Fatalf("effect multiplicity = %d, want 2", got)
	}
	bad := withEffects
	bad[1].Effects.Occurrences = []EffectSpec{{Target: 1}}
	if _, err := Seal(&Spec{Operations: bad}); err == nil {
		t.Fatal("effect ABI mismatch accepted")
	}
}

func TestSynthesizedOpaqueAndHotQueriesAllocateNothing(t *testing.T) {
	contract := mustSeal(t, Spec{})
	if got := contract.OperationCount(); got != 1 {
		t.Fatalf("operation count = %d, want opaque row", got)
	}
	opaque, ok := contract.Opaque()
	if !ok || opaque != 1 {
		t.Fatalf("opaque = %d/%v", opaque, ok)
	}
	input, _ := contract.Input(opaque)
	if tail, _, ok := contract.ValuesTail(input); !ok || tail != ValuesUnknown {
		t.Fatalf("opaque input = %d/%v", tail, ok)
	}
	if tail, _, ok := contract.EffectTail(opaque); !ok || tail != RowUnknownOpen {
		t.Fatalf("opaque effect tail = %d/%v", tail, ok)
	}
	if got := contract.OutcomeCount(opaque); got != 4 {
		t.Fatalf("opaque outcomes = %d, want 4", got)
	}
	if _, ok := contract.Lookup(BindingSpec{Namespace: BindingBuiltin, Member: []string{"missing"}}); ok {
		t.Fatal("missing binding resolved to opaque")
	}

	bound := mustSeal(t, Spec{Operations: []OperationSpec{builtin("hot", typ.String, RowSpec{Tail: RowClosed})}})
	binding := BindingSpec{Namespace: BindingBuiltin, Member: []string{"hot"}}
	if allocs := testing.AllocsPerRun(1000, func() {
		op, ok := bound.Lookup(binding)
		if !ok {
			panic("missing hot binding")
		}
		input, ok := bound.Input(op)
		if !ok {
			panic("missing input")
		}
		if _, ok = bound.ValuesAt(input, 0); !ok {
			panic("missing input type")
		}
		if _, _, ok = bound.OutcomeAt(op, 0); !ok {
			panic("missing outcome")
		}
	}); allocs != 0 {
		t.Fatalf("hot queries allocated %f times", allocs)
	}
}

func builtin(name string, input typ.Type, effects RowSpec) OperationSpec {
	return OperationSpec{
		Bindings: []BindingSpec{{Namespace: BindingBuiltin, Member: []string{name}}},
		Input:    ValuesSpec{Fixed: []typ.Type{input}, Tail: ValuesClosed},
		Outcomes: []OutcomeSpec{{Kind: flowkind.OutcomeNormal, Values: ValuesSpec{
			Fixed: []typ.Type{input}, Tail: ValuesClosed,
		}}},
		Effects: effects,
	}
}

func genericBuiltin(name string, formal *typ.TypeParam) OperationSpec {
	return OperationSpec{
		Bindings:    []BindingSpec{{Namespace: BindingBuiltin, Member: []string{name}}},
		TypeFormals: []*typ.TypeParam{formal},
		Input:       ValuesSpec{Fixed: []typ.Type{formal}, Tail: ValuesClosed},
		Outcomes:    []OutcomeSpec{{Kind: flowkind.OutcomeNormal, Values: ValuesSpec{Fixed: []typ.Type{formal}, Tail: ValuesClosed}}},
		Effects:     RowSpec{Tail: RowClosed},
	}
}

func effect(target SpecRef, values []ValueFormal) RowSpec {
	return RowSpec{Occurrences: []EffectSpec{{Target: target, ValueArgs: values}}, Tail: RowClosed}
}

func mustSeal(t *testing.T, spec Spec) *Contract {
	t.Helper()
	contract, err := Seal(&spec)
	if err != nil {
		t.Fatalf("Seal: %v", err)
	}
	return contract
}

func assertContractShapeEqual(t *testing.T, left, right *Contract) {
	t.Helper()
	assertPublicContractEqual(t, left, right)
}
