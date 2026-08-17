package target

import (
	"testing"

	schematype "github.com/wippyai/go-lua/analysis/schema/typecontract"

	"github.com/wippyai/go-lua/domain/type/annotation"
	"github.com/wippyai/go-lua/domain/type/typeexpr"
	flowkind "github.com/wippyai/go-lua/analysis/program/flow/kind"
)

func TestSealDeterministicAcrossOperationOrder(t *testing.T) {
	first := mustSeal(t, Spec{Operations: []OperationSpec{
		builtin("alpha", testString, effect(2, []ValueFormal{0})),
		builtin("beta", testString, RowSpec{Tail: RowClosed}),
	}})
	second := mustSeal(t, Spec{Operations: []OperationSpec{
		builtin("beta", testString, RowSpec{Tail: RowClosed}),
		builtin("alpha", testString, effect(1, []ValueFormal{0})),
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
	left := testNewTypeParam("T", testString)
	right := testNewTypeParam("Value", testString)
	leftContract := mustSeal(t, Spec{Operations: []OperationSpec{genericBuiltin("identity", left)}})
	rightContract := mustSeal(t, Spec{Operations: []OperationSpec{genericBuiltin("identity", right)}})
	assertPublicContractEqual(t, leftContract, rightContract)
	leftOp, _ := leftContract.Lookup(BindingSpec{Namespace: BindingBuiltin, Member: []string{"identity"}})
	rightOp, _ := rightContract.Lookup(BindingSpec{Namespace: BindingBuiltin, Member: []string{"identity"}})
	leftInput, _ := leftContract.Input(leftOp)
	rightInput, _ := rightContract.Input(rightOp)
	leftType, _ := leftContract.ValuesAt(leftInput, 0)
	rightType, _ := rightContract.ValuesAt(rightInput, 0)
	leftDeclaration, leftOK := leftContract.TypeDeclaration(leftType)
	rightDeclaration, rightOK := rightContract.TypeDeclaration(rightType)
	if !leftOK || !rightOK || !leftDeclaration.Equal(rightDeclaration) {
		t.Fatal("alpha-renamed type formal changed frozen target type bytes")
	}
	leftConstraint, ok := leftContract.TypeFormalConstraint(leftOp, 0)
	if !ok || leftConstraint == 0 {
		t.Fatal("missing frozen formal constraint")
	}
}

func TestGenericAndRecursiveTypesFreezeWithoutRawRetention(t *testing.T) {
	outer := testNewTypeParam("T", schematype.Type{})
	inner := testNewTypeParam("Element", schematype.Type{})
	channel := testRawGeneric("Channel", []*testRawTypeParam{inner}, testRawArray(inner))
	channelOfOuter := testRawInstantiate(channel, outer)
	recursive := testRawRecursive("Node", func(self testRawType) testRawType { return typeexpr.Optional(self) })
	channelOfOuterDeclaration := testEncode(channelOfOuter, outer)
	recursiveDeclaration := testEncode(recursive)
	contract := mustSeal(t, Spec{Operations: []OperationSpec{{
		Bindings:    []BindingSpec{{Namespace: BindingProvider, Owner: []string{"channel"}, Member: []string{"new"}}},
		TypeFormals: []TypeFormalSpec{{}},
		Input:       ValuesSpec{Fixed: []schematype.Type{channelOfOuterDeclaration, recursiveDeclaration}, Tail: ValuesClosed},
		Outcomes:    []OutcomeSpec{{Kind: flowkind.OutcomeNormal, Values: ValuesSpec{Fixed: []schematype.Type{channelOfOuterDeclaration}, Tail: ValuesClosed}}},
		Effects:     RowSpec{Tail: RowClosed},
	}}})
	op, _ := contract.Lookup(BindingSpec{Namespace: BindingProvider, Owner: []string{"channel"}, Member: []string{"new"}})
	input, _ := contract.Input(op)
	for index := 0; index < contract.ValuesCount(input); index++ {
		frozen, _ := contract.ValuesAt(input, index)
		if data, ok := contract.TypeDeclaration(frozen); !ok || !data.Available() {
			t.Fatalf("frozen type %d unavailable", index)
		}
	}
}

func TestDeepAuthoringTypeUsesNoGoRecursion(t *testing.T) {
	var deep testRawType = testRawString
	for index := 0; index < 20000; index++ {
		deep = testRawArray(deep)
	}
	contract := mustSeal(t, Spec{Operations: []OperationSpec{builtin("deep", deep, RowSpec{Tail: RowClosed})}})
	op, _ := contract.Lookup(BindingSpec{Namespace: BindingBuiltin, Member: []string{"deep"}})
	input, _ := contract.Input(op)
	frozen, _ := contract.ValuesAt(input, 0)
	if data, ok := contract.TypeDeclaration(frozen); !ok || !data.Available() {
		t.Fatal("deep type did not freeze")
	}
	if !contract.ContentID().Available() {
		t.Fatal("deep sealed type has no ContentID")
	}
}

func TestSealOwnsInputsAndConsumesFailedSpec(t *testing.T) {
	record := testMutableRecord(testRawRecordParts{Fields: []testRawField{{Name: "value", Type: testRawString}}})
	operations := []OperationSpec{builtin("owned", record, RowSpec{Tail: RowClosed})}
	spec := Spec{Operations: operations}
	contract, err := testSeal(&spec)
	if err != nil {
		t.Fatalf("Seal: %v", err)
	}
	contentID := contract.ContentID()
	op, _ := contract.Lookup(BindingSpec{Namespace: BindingBuiltin, Member: []string{"owned"}})
	input, _ := contract.Input(op)
	frozen, _ := contract.ValuesAt(input, 0)
	before, _ := contract.TypeDeclaration(frozen)
	record.Fields[0].Type = testRawNumber
	operations[0].Bindings[0].Member[0] = "mutated"
	if _, found := contract.Lookup(BindingSpec{Namespace: BindingBuiltin, Member: []string{"owned"}}); !found {
		t.Fatal("caller mutation changed sealed binding")
	}
	after, _ := contract.TypeDeclaration(frozen)
	if !before.Equal(after) || !before.Available() {
		t.Fatal("caller type mutation changed frozen bytes")
	}
	if contentID != contract.ContentID() {
		t.Fatal("caller mutation changed sealed ContentID")
	}
	if _, err := testSeal(&spec); err == nil {
		t.Fatal("successful Seal did not consume spec")
	}

	bad := Spec{Operations: []OperationSpec{{Bindings: []BindingSpec{{Namespace: BindingBuiltin, Member: []string{"bad"}}}, Input: ValuesSpec{Tail: ValuesUnknown}}}}
	if _, err := testSeal(&bad); err == nil {
		t.Fatal("bad Seal unexpectedly succeeded")
	}
	bad.Operations = []OperationSpec{builtin("replacement", testString, RowSpec{Tail: RowClosed})}
	if _, err := testSeal(&bad); err == nil || err.Error() != "target: consumed spec" {
		t.Fatalf("failed Seal did not consume replacement input: %v", err)
	}
}

func TestTargetTypeValidationRejectsErasureAndUnresolvedAuthority(t *testing.T) {
	foreign := testNewTypeParam("Foreign", nil)
	cases := []struct {
		name  string
		value testRawType
	}{
		{name: "annotated", value: testRawAnnotated(testRawString, []annotation.Annotation{{Name: "min", Arg: annotation.IntArg(1)}})},
		{name: "unknown", value: testRawUnknown},
		{name: "self", value: testRawSelf},
		{name: "ref", value: testRawRef("module", "T")},
		{name: "foreign formal", value: foreign},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			if _, err := testSeal(&Spec{Operations: []OperationSpec{builtin("reject", test.value, RowSpec{Tail: RowClosed})}}); err == nil {
				t.Fatalf("%s type was accepted", test.name)
			}
		})
	}
}

func TestBindingInputAndValuesTails(t *testing.T) {
	contract := mustSeal(t, Spec{Operations: []OperationSpec{{
		Bindings:   []BindingSpec{{Namespace: BindingProvider, Owner: []string{"channel"}, Member: []string{"send"}}},
		ValuesVars: 1,
		Input:      ValuesSpec{Fixed: []schematype.Type{testString, testString}, Tail: ValuesVariable, Var: 0},
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
	base := builtin("receive", testString, RowSpec{Tail: RowClosed})
	base.Outcomes = []OutcomeSpec{
		{Kind: flowkind.OutcomeNormal, Values: ValuesSpec{Fixed: []schematype.Type{testString}, Tail: ValuesClosed}},
		{Kind: flowkind.OutcomeNormal, Values: ValuesSpec{Fixed: []schematype.Type{testBoolean}, Tail: ValuesClosed}},
		{Kind: flowkind.OutcomeThrow, Values: ValuesSpec{Fixed: []schematype.Type{testString}, Tail: ValuesClosed}},
	}
	contract := mustSeal(t, Spec{Operations: []OperationSpec{base}})
	op, _ := contract.Lookup(BindingSpec{Namespace: BindingBuiltin, Member: []string{"receive"}})
	if got := contract.OutcomeCount(op); got != 3 {
		t.Fatalf("outcome count = %d, want 3", got)
	}
	dup := base
	dup.Outcomes = append(append([]OutcomeSpec(nil), base.Outcomes...), base.Outcomes[0])
	if _, err := testSeal(&Spec{Operations: []OperationSpec{dup}}); err == nil {
		t.Fatal("duplicate correlated outcome accepted")
	}
}

func TestEffectMultiplicityAndArgumentValidation(t *testing.T) {
	contract := mustSeal(t, Spec{Operations: []OperationSpec{
		builtin("store", testString, RowSpec{Tail: RowClosed}),
		builtin("send", testString, RowSpec{Tail: RowClosed}),
	}})
	_ = contract
	withEffects := []OperationSpec{
		builtin("store", testString, RowSpec{Tail: RowClosed}),
		builtin("send", testString, RowSpec{Occurrences: []EffectSpec{
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
	if _, err := testSeal(&Spec{Operations: bad}); err == nil {
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

	bound := mustSeal(t, Spec{Operations: []OperationSpec{builtin("hot", testString, RowSpec{Tail: RowClosed})}})
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

func builtin(name string, input interface{}, effects RowSpec) OperationSpec {
	declaration := testDeclaration(input)
	return OperationSpec{
		Bindings: []BindingSpec{{Namespace: BindingBuiltin, Member: []string{name}}},
		Input:    ValuesSpec{Fixed: []schematype.Type{declaration}, Tail: ValuesClosed},
		Outcomes: []OutcomeSpec{{Kind: flowkind.OutcomeNormal, Values: ValuesSpec{
			Fixed: []schematype.Type{declaration}, Tail: ValuesClosed,
		}}},
		Effects: effects,
	}
}

func genericBuiltin(name string, formal *testRawTypeParam) OperationSpec {
	declaration := testEncode(formal, formal)
	constraint := schematype.Type{}
	if formal.Constraint != nil {
		constraint = testEncode(formal.Constraint)
	}
	return OperationSpec{
		Bindings:    []BindingSpec{{Namespace: BindingBuiltin, Member: []string{name}}},
		TypeFormals: []TypeFormalSpec{{Constraint: constraint}},
		Input:       ValuesSpec{Fixed: []schematype.Type{declaration}, Tail: ValuesClosed},
		Outcomes:    []OutcomeSpec{{Kind: flowkind.OutcomeNormal, Values: ValuesSpec{Fixed: []schematype.Type{declaration}, Tail: ValuesClosed}}},
		Effects:     RowSpec{Tail: RowClosed},
	}
}

func testDeclaration(value interface{}) schematype.Type {
	switch value := value.(type) {
	case schematype.Type:
		return value
	case testRawType:
		return testEncodeOrUnavailable(value)
	default:
		panic("unsupported target test declaration")
	}
}

func effect(target SpecRef, values []ValueFormal) RowSpec {
	return RowSpec{Occurrences: []EffectSpec{{Target: target, ValueArgs: values}}, Tail: RowClosed}
}

func mustSeal(t *testing.T, spec Spec) *Contract {
	t.Helper()
	contract, err := testSeal(&spec)
	if err != nil {
		t.Fatalf("Seal: %v", err)
	}
	return contract
}

func assertContractShapeEqual(t *testing.T, left, right *Contract) {
	t.Helper()
	assertPublicContractEqual(t, left, right)
}
