package target

import (
	flowkind "github.com/wippyai/go-lua/analysis/program/flow/kind"
	"github.com/wippyai/go-lua/analysis/program/target/vocabulary"
	schematype "github.com/wippyai/go-lua/analysis/schema/typecontract"
	"github.com/wippyai/go-lua/domain/type/annotation"
	"github.com/wippyai/go-lua/domain/type/typeexpr"
	"testing"
)

func TestSealDeterministicAcrossOperationOrder(t *testing.T) {
	first := mustSeal(t, Spec{Operations: []vocabulary.OperationSpec{
		builtin("alpha", testString, effect(2, []vocabulary.ValueFormal{0})),
		builtin("beta", testString, vocabulary.RowSpec{Tail: vocabulary.RowClosed}),
	}})
	second := mustSeal(t, Spec{Operations: []vocabulary.OperationSpec{
		builtin("beta", testString, vocabulary.RowSpec{Tail: vocabulary.RowClosed}),
		builtin("alpha", testString, effect(1, []vocabulary.ValueFormal{0})),
	}})
	assertContractShapeEqual(t, first, second)
	alpha := vocabulary.BindingSpec{Namespace: vocabulary.BindingBuiltin, Member: []string{"alpha"}}
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
	leftContract := mustSeal(t, Spec{Operations: []vocabulary.OperationSpec{genericBuiltin("identity", left)}})
	rightContract := mustSeal(t, Spec{Operations: []vocabulary.OperationSpec{genericBuiltin("identity", right)}})
	assertPublicContractEqual(t, leftContract, rightContract)
	leftOp, _ := leftContract.Lookup(vocabulary.BindingSpec{Namespace: vocabulary.BindingBuiltin, Member: []string{"identity"}})
	rightOp, _ := rightContract.Lookup(vocabulary.BindingSpec{Namespace: vocabulary.BindingBuiltin, Member: []string{"identity"}})
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
	contract := mustSeal(t, Spec{Operations: []vocabulary.OperationSpec{{
		Bindings:    []vocabulary.BindingSpec{{Namespace: vocabulary.BindingProvider, Owner: []string{"channel"}, Member: []string{"new"}}},
		TypeFormals: []vocabulary.TypeFormalSpec{{}},
		Input:       vocabulary.ValuesSpec{Fixed: []schematype.Type{channelOfOuterDeclaration, recursiveDeclaration}, Tail: vocabulary.ValuesClosed},
		Outcomes:    []vocabulary.OutcomeSpec{{Kind: flowkind.OutcomeNormal, Values: vocabulary.ValuesSpec{Fixed: []schematype.Type{channelOfOuterDeclaration}, Tail: vocabulary.ValuesClosed}}},
		Effects:     vocabulary.RowSpec{Tail: vocabulary.RowClosed},
	}}})
	op, _ := contract.Lookup(vocabulary.BindingSpec{Namespace: vocabulary.BindingProvider, Owner: []string{"channel"}, Member: []string{"new"}})
	input, _ := contract.Input(op)
	for index := 0; index < contract.ValuesCount(input); index++ {
		frozen, _ := contract.ValuesAt(input, index)
		if data, ok := contract.TypeDeclaration(frozen); !ok || !data.Available() {
			t.Fatalf("frozen type %d unavailable", index)
		}
	}
}

func TestDeepAuthoringTypeUsesNoGoRecursion(t *testing.T) {
	deep := testRawString
	for index := 0; index < 20000; index++ {
		deep = testRawArray(deep)
	}
	contract := mustSeal(t, Spec{Operations: []vocabulary.OperationSpec{builtin("deep", deep, vocabulary.RowSpec{Tail: vocabulary.RowClosed})}})
	op, _ := contract.Lookup(vocabulary.BindingSpec{Namespace: vocabulary.BindingBuiltin, Member: []string{"deep"}})
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
	operations := []vocabulary.OperationSpec{builtin("owned", record, vocabulary.RowSpec{Tail: vocabulary.RowClosed})}
	spec := Spec{Operations: operations}
	contract, err := testSeal(&spec)
	if err != nil {
		t.Fatalf("Seal: %v", err)
	}
	contentID := contract.ContentID()
	op, _ := contract.Lookup(vocabulary.BindingSpec{Namespace: vocabulary.BindingBuiltin, Member: []string{"owned"}})
	input, _ := contract.Input(op)
	frozen, _ := contract.ValuesAt(input, 0)
	before, _ := contract.TypeDeclaration(frozen)
	record.Fields[0].Type = testRawNumber
	operations[0].Bindings[0].Member[0] = "mutated"
	if _, found := contract.Lookup(vocabulary.BindingSpec{Namespace: vocabulary.BindingBuiltin, Member: []string{"owned"}}); !found {
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

	bad := Spec{Operations: []vocabulary.OperationSpec{{Bindings: []vocabulary.BindingSpec{{Namespace: vocabulary.BindingBuiltin, Member: []string{"bad"}}}, Input: vocabulary.ValuesSpec{Tail: vocabulary.ValuesUnknown}}}}
	if _, err := testSeal(&bad); err == nil {
		t.Fatal("bad Seal unexpectedly succeeded")
	}
	bad.Operations = []vocabulary.OperationSpec{builtin("replacement", testString, vocabulary.RowSpec{Tail: vocabulary.RowClosed})}
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
			if _, err := testSeal(&Spec{Operations: []vocabulary.OperationSpec{builtin("reject", test.value, vocabulary.RowSpec{Tail: vocabulary.RowClosed})}}); err == nil {
				t.Fatalf("%s type was accepted", test.name)
			}
		})
	}
}

func TestBindingInputAndValuesTails(t *testing.T) {
	contract := mustSeal(t, Spec{Operations: []vocabulary.OperationSpec{{
		Bindings:   []vocabulary.BindingSpec{{Namespace: vocabulary.BindingProvider, Owner: []string{"channel"}, Member: []string{"send"}}},
		ValuesVars: 1,
		Input:      vocabulary.ValuesSpec{Fixed: []schematype.Type{testString, testString}, Tail: vocabulary.ValuesVariable, Var: 0},
		Outcomes:   []vocabulary.OutcomeSpec{{Kind: flowkind.OutcomeNormal, Values: vocabulary.ValuesSpec{Tail: vocabulary.ValuesVariable, Var: 0}}},
		Effects:    vocabulary.RowSpec{Tail: vocabulary.RowClosed},
	}}})
	binding := vocabulary.BindingSpec{Namespace: vocabulary.BindingProvider, Owner: []string{"channel"}, Member: []string{"send"}}
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
	if tail, variable, ok := contract.ValuesTail(input); !ok || tail != vocabulary.ValuesVariable || variable != 0 {
		t.Fatalf("input tail = %d/%d/%v", tail, variable, ok)
	}
}

func TestCorrelatedOutcomesRejectDuplicates(t *testing.T) {
	base := builtin("receive", testString, vocabulary.RowSpec{Tail: vocabulary.RowClosed})
	base.Outcomes = []vocabulary.OutcomeSpec{
		{Kind: flowkind.OutcomeNormal, Values: vocabulary.ValuesSpec{Fixed: []schematype.Type{testString}, Tail: vocabulary.ValuesClosed}},
		{Kind: flowkind.OutcomeNormal, Values: vocabulary.ValuesSpec{Fixed: []schematype.Type{testBoolean}, Tail: vocabulary.ValuesClosed}},
		{Kind: flowkind.OutcomeThrow, Values: vocabulary.ValuesSpec{Fixed: []schematype.Type{testString}, Tail: vocabulary.ValuesClosed}},
	}
	contract := mustSeal(t, Spec{Operations: []vocabulary.OperationSpec{base}})
	op, _ := contract.Lookup(vocabulary.BindingSpec{Namespace: vocabulary.BindingBuiltin, Member: []string{"receive"}})
	if got := contract.OutcomeCount(op); got != 3 {
		t.Fatalf("outcome count = %d, want 3", got)
	}
	dup := base
	dup.Outcomes = append(append([]vocabulary.OutcomeSpec(nil), base.Outcomes...), base.Outcomes[0])
	if _, err := testSeal(&Spec{Operations: []vocabulary.OperationSpec{dup}}); err == nil {
		t.Fatal("duplicate correlated outcome accepted")
	}
}

func TestEffectMultiplicityAndArgumentValidation(t *testing.T) {
	contract := mustSeal(t, Spec{Operations: []vocabulary.OperationSpec{
		builtin("store", testString, vocabulary.RowSpec{Tail: vocabulary.RowClosed}),
		builtin("send", testString, vocabulary.RowSpec{Tail: vocabulary.RowClosed}),
	}})
	_ = contract
	withEffects := []vocabulary.OperationSpec{
		builtin("store", testString, vocabulary.RowSpec{Tail: vocabulary.RowClosed}),
		builtin("send", testString, vocabulary.RowSpec{Occurrences: []vocabulary.EffectSpec{
			{Target: 1, ValueArgs: []vocabulary.ValueFormal{0}},
			{Target: 1, ValueArgs: []vocabulary.ValueFormal{0}},
		}, Tail: vocabulary.RowClosed}),
	}
	sealed := mustSeal(t, Spec{Operations: withEffects})
	send, _ := sealed.Lookup(vocabulary.BindingSpec{Namespace: vocabulary.BindingBuiltin, Member: []string{"send"}})
	if got := sealed.EffectCount(send); got != 2 {
		t.Fatalf("effect multiplicity = %d, want 2", got)
	}
	bad := withEffects
	bad[1].Effects.Occurrences = []vocabulary.EffectSpec{{Target: 1}}
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
	if tail, _, ok := contract.ValuesTail(input); !ok || tail != vocabulary.ValuesUnknown {
		t.Fatalf("opaque input = %d/%v", tail, ok)
	}
	if tail, _, ok := contract.EffectTail(opaque); !ok || tail != vocabulary.RowUnknownOpen {
		t.Fatalf("opaque effect tail = %d/%v", tail, ok)
	}
	if got := contract.OutcomeCount(opaque); got != 4 {
		t.Fatalf("opaque outcomes = %d, want 4", got)
	}
	if _, ok := contract.Lookup(vocabulary.BindingSpec{Namespace: vocabulary.BindingBuiltin, Member: []string{"missing"}}); ok {
		t.Fatal("missing binding resolved to opaque")
	}

	bound := mustSeal(t, Spec{Operations: []vocabulary.OperationSpec{builtin("hot", testString, vocabulary.RowSpec{Tail: vocabulary.RowClosed})}})
	binding := vocabulary.BindingSpec{Namespace: vocabulary.BindingBuiltin, Member: []string{"hot"}}
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

func TestSealConsumesAuthoringSpecOnEveryFirstAttempt(t *testing.T) {
	spec := Spec{Operations: []vocabulary.OperationSpec{builtin("seal-once", testString, vocabulary.RowSpec{Tail: vocabulary.RowClosed})}}
	if _, err := testSeal(&spec); err != nil {
		t.Fatal(err)
	}
	if _, err := testSeal(&spec); err == nil {
		t.Fatal("successful Seal left the authoring spec reusable")
	}
	bad := Spec{Operations: []vocabulary.OperationSpec{{Bindings: []vocabulary.BindingSpec{{Namespace: vocabulary.BindingBuiltin, Member: []string{"bad"}}}, Input: vocabulary.ValuesSpec{Tail: vocabulary.ValuesUnknown}}}}
	if _, err := testSeal(&bad); err == nil {
		t.Fatal("invalid spec unexpectedly sealed")
	}
	bad.Operations = []vocabulary.OperationSpec{builtin("replacement", testString, vocabulary.RowSpec{Tail: vocabulary.RowClosed})}
	if _, err := testSeal(&bad); err == nil {
		t.Fatal("failed Seal left the authoring spec reusable")
	}
}

func TestSealDraftsCanonicalizeIndependentOperationAuthoring(t *testing.T) {
	contract := mustSeal(t, Spec{Operations: []vocabulary.OperationSpec{
		builtin("draft-b", testBoolean, vocabulary.RowSpec{Tail: vocabulary.RowClosed}),
		builtin("draft-a", testString, vocabulary.RowSpec{Tail: vocabulary.RowClosed}),
	}})
	left, leftOK := contract.Lookup(vocabulary.BindingSpec{Namespace: vocabulary.BindingBuiltin, Member: []string{"draft-a"}})
	right, rightOK := contract.Lookup(vocabulary.BindingSpec{Namespace: vocabulary.BindingBuiltin, Member: []string{"draft-b"}})
	if !leftOK || !rightOK || left >= right {
		t.Fatalf("draft canonical order = %d/%v before %d/%v", left, leftOK, right, rightOK)
	}
	if contract.OperationCount() != 3 {
		t.Fatalf("draft operation count = %d, want two bound plus opaque", contract.OperationCount())
	}
}

func TestSealOperationRequiresCompleteOutcomeAuthority(t *testing.T) {
	valid := Spec{Operations: []vocabulary.OperationSpec{builtin("complete", testString, vocabulary.RowSpec{Tail: vocabulary.RowClosed})}}
	if _, err := testSeal(&valid); err != nil {
		t.Fatal(err)
	}
	invalid := Spec{Operations: []vocabulary.OperationSpec{{
		Bindings: []vocabulary.BindingSpec{{Namespace: vocabulary.BindingBuiltin, Member: []string{"incomplete"}}},
		Input:    vocabulary.ValuesSpec{Tail: vocabulary.ValuesClosed},
	}}}
	if _, err := testSeal(&invalid); err == nil {
		t.Fatal("operation without outcomes was accepted")
	}
}

func TestSealRelationsRejectDuplicateBindingsAndCanonicalizeOrder(t *testing.T) {
	bindings, err := freezeBindings([]vocabulary.BindingSpec{
		{Namespace: vocabulary.BindingBuiltin, Member: []string{"z"}},
		{Namespace: vocabulary.BindingBuiltin, Member: []string{"a"}},
	})
	if err != nil || len(bindings) != 2 || bindings[0].Member[0] != "a" {
		t.Fatalf("freezeBindings = %#v/%v", bindings, err)
	}
	if _, err := freezeBindings([]vocabulary.BindingSpec{
		{Namespace: vocabulary.BindingBuiltin, Member: []string{"same"}},
		{Namespace: vocabulary.BindingBuiltin, Member: []string{"same"}},
	}); err == nil {
		t.Fatal("duplicate bindings were accepted")
	}
}

func TestSealResolutionRetainsProducedOperationAnchors(t *testing.T) {
	contract := mustSeal(t, deltaProduced(0))
	parent, ok := contract.Lookup(vocabulary.BindingSpec{Namespace: vocabulary.BindingBuiltin, Member: []string{"produced"}})
	if !ok || contract.producedCount(parent, 0) != 1 {
		t.Fatalf("produced resolution = op:%d/%v count:%d", parent, ok, contract.producedCount(parent, 0))
	}
	_, child, ok := contract.producedAt(parent, 0, 0)
	if !ok || child == 0 || child == parent {
		t.Fatalf("produced child = %d/%v, parent=%d", child, ok, parent)
	}
}

func TestSealAppendPublishesDenseOperationRelations(t *testing.T) {
	contract := mustSeal(t, Spec{Operations: []vocabulary.OperationSpec{builtin("append-relations", testString, vocabulary.RowSpec{Tail: vocabulary.RowClosed})}})
	op, ok := contract.Lookup(vocabulary.BindingSpec{Namespace: vocabulary.BindingBuiltin, Member: []string{"append-relations"}})
	if !ok {
		t.Fatal("appended operation binding missing")
	}
	input, ok := contract.Input(op)
	if !ok || input == 0 || contract.ValuesCount(input) != 1 {
		t.Fatalf("appended input = %d/%v count=%d", input, ok, contract.ValuesCount(input))
	}
	if got := contract.OutcomeCount(op); got != 1 {
		t.Fatalf("appended outcome count = %d, want 1", got)
	}
	if got := contract.BindingCount(op); got != 1 {
		t.Fatalf("appended binding count = %d, want 1", got)
	}
}

func TestSealValidationAdmitsOnlyNormalizedBindingRows(t *testing.T) {
	if !vocabulary.ValidBinding(vocabulary.BindingSpec{Namespace: vocabulary.BindingBuiltin, Member: []string{"ok"}}) {
		t.Fatal("valid direct binding rejected")
	}
	if vocabulary.ValidBinding(vocabulary.BindingSpec{Namespace: vocabulary.BindingBuiltin}) {
		t.Fatal("binding without a member rejected law")
	}
	if vocabulary.ValidBinding(vocabulary.BindingSpec{Namespace: vocabulary.BindingNamespace(0), Member: []string{"bad"}}) {
		t.Fatal("invalid binding namespace admitted")
	}
	if !validValuesTail(vocabulary.ValuesClosed, 0, 0, false) || validValuesTail(vocabulary.ValuesUnknown, 0, 0, false) {
		t.Fatal("Values tail validation admitted an invalid closed/unknown combination")
	}
}

func TestStoredRepresentationChecksRejectUnrepresentableRanges(t *testing.T) {
	if _, err := checkedStoredRange("test pool", vocabulary.MaxIndex(), 1); err == nil {
		t.Fatal("native or uint32 range overflow was accepted")
	}
	if _, err := checkedStoredHandle("test handle", vocabulary.MaxIndex()); err == nil {
		t.Fatal("one-based handle overflow was accepted")
	}
	if _, err := vocabulary.CheckedStoredTotal("test pool", vocabulary.MaxIndex(), 1); err == nil {
		t.Fatal("aggregate range overflow was accepted")
	}
}

func TestExactKeyPoolIsCanonicalAndContractLocal(t *testing.T) {
	contract := mustSeal(t, completeBootSpec("Lua", vocabulary.InitialMutable))
	if contract.ExactKeyCount() == 0 {
		t.Fatal("boot contract has no exact keys")
	}
	for index := 0; index < contract.ExactKeyCount(); index++ {
		key, ok := contract.ExactKeyAt(index)
		if !ok || key == 0 {
			t.Fatalf("ExactKeyAt(%d) = %d/%v", index, key, ok)
		}
		if _, ok := contract.ExactKeyValue(key); !ok {
			t.Fatalf("ExactKeyValue(%d) unavailable", key)
		}
	}
	if _, ok := contract.ExactKeyAt(contract.ExactKeyCount()); ok {
		t.Fatal("out-of-range exact key resolved")
	}
}

func TestEffectRowsCarryTotalRowFormalSubstitutions(t *testing.T) {
	contract := mustSeal(t, rowBoundarySpec(vocabulary.RowVariable, vocabulary.CallbackReleaseOne))
	owner, ok := contract.Lookup(vocabulary.BindingSpec{Namespace: vocabulary.BindingBuiltin, Member: []string{"row-owner"}})
	if !ok || contract.EffectCount(owner) != 1 {
		t.Fatalf("owner/effects = %d/%v/%d", owner, ok, contract.EffectCount(owner))
	}
	target, targetOK := contract.Lookup(vocabulary.BindingSpec{Namespace: vocabulary.BindingBuiltin, Member: []string{"row-target"}})
	effectTarget, effectTargetOK := contract.EffectTarget(owner, 0)
	if !targetOK || !effectTargetOK || effectTarget != target || contract.EffectRowArgumentCount(owner, 0) != 1 {
		t.Fatalf("effect row = target:%d/%v want:%d rows:%d", effectTarget, effectTargetOK, target, contract.EffectRowArgumentCount(owner, 0))
	}
	row, rowOK := contract.effectRowArgumentAt(owner, 0, 0)
	if !rowOK || row != 0 {
		t.Fatalf("effect row argument = %d/%v", row, rowOK)
	}
	if _, ok := contract.effectRowArgumentAt(owner, 0, 1); ok {
		t.Fatal("out-of-range effect row argument resolved")
	}

	badScope := rowBoundarySpec(vocabulary.RowVariable, vocabulary.CallbackReleaseOne)
	badScope.Operations[0].Effects.Occurrences[0].RowArgs[0] = 1
	if contract, err := testSeal(&badScope); err == nil || contract != nil {
		t.Fatal("effect row argument outside source scope was published")
	}
	badABI := rowBoundarySpec(vocabulary.RowVariable, vocabulary.CallbackReleaseOne)
	badABI.Operations[1].RowFormals = 2
	if contract, err := testSeal(&badABI); err == nil || contract != nil {
		t.Fatal("incomplete effect row substitution was published")
	}
}

func TestCallbackExpectedRowsAndRetainedReleaseAreDirectAndCanonical(t *testing.T) {
	first := mustSeal(t, rowBoundarySpec(vocabulary.RowVariable, vocabulary.CallbackReleaseOne))
	second := mustSeal(t, rowBoundarySpec(vocabulary.RowVariable, vocabulary.CallbackReleaseAll))
	if first.ContentID() == second.ContentID() {
		t.Fatal("callback release mode was omitted from canonical artifact")
	}
	owner, ownerOK := first.Lookup(vocabulary.BindingSpec{Namespace: vocabulary.BindingBuiltin, Member: []string{"row-owner"}})
	releaseOp, releaseOK := first.Lookup(vocabulary.BindingSpec{Namespace: vocabulary.BindingBuiltin, Member: []string{"row-target"}})
	callback, callbackOK := first.CallbackAt(owner, 0)
	if !ownerOK || !releaseOK || !callbackOK {
		t.Fatal("callback boundary fixture failed to resolve")
	}
	tail, variable, tailOK := first.CallbackEffectTail(callback)
	if !tailOK || tail != vocabulary.RowVariable || variable != 0 || first.CallbackEffectCount(callback) != 1 {
		t.Fatalf("callback expected row = %d/%d/%v effects:%d", tail, variable, tailOK, first.CallbackEffectCount(callback))
	}
	effectTarget, effectOK := first.CallbackEffectTarget(callback, 0)
	row, rowOK := first.callbackEffectRowArgumentAt(callback, 0, 0)
	if !effectOK || effectTarget != releaseOp || !rowOK || row != 0 {
		t.Fatalf("callback effect = target:%d/%v row:%d/%v", effectTarget, effectOK, row, rowOK)
	}
	operation, input, outcome, mode, releaseFound := first.callbackRelease(callback)
	if !releaseFound || operation != releaseOp || input != 0 || outcome != 0 || mode != vocabulary.CallbackReleaseOne {
		t.Fatalf("callback release = %d/%d/%d/%d/%v", operation, input, outcome, mode, releaseFound)
	}
	if first.callbackReleaseCount(releaseOp) != 1 {
		t.Fatalf("release reverse range = %d", first.callbackReleaseCount(releaseOp))
	}
	released, releaseInput, releaseOutcome, releaseMode, reverseFound := first.callbackReleaseAt(releaseOp, 0)
	if !reverseFound || released != callback || releaseInput != 0 || releaseOutcome != 0 || releaseMode != vocabulary.CallbackReleaseOne {
		t.Fatalf("reverse release = %d/%d/%d/%d/%v", released, releaseInput, releaseOutcome, releaseMode, reverseFound)
	}

	missingRow := rowBoundarySpec(vocabulary.RowVariable, vocabulary.CallbackReleaseOne)
	missingRow.Operations[0].Callbacks[0].Effects = vocabulary.RowSpec{}
	if contract, err := testSeal(&missingRow); err == nil || contract != nil {
		t.Fatal("callback without an expected row was published")
	}
	syncRelease := rowBoundarySpec(vocabulary.RowVariable, vocabulary.CallbackReleaseOne)
	syncRelease.Operations[0].Callbacks[0].Lifecycle = vocabulary.CallbackSyncOptionalOnce
	if contract, err := testSeal(&syncRelease); err == nil || contract != nil {
		t.Fatal("sync callback release was published")
	}
	badRelease := rowBoundarySpec(vocabulary.RowVariable, vocabulary.CallbackReleaseOne)
	badRelease.Operations[0].Callbacks[0].Release.Outcome = 1
	if contract, err := testSeal(&badRelease); err == nil || contract != nil {
		t.Fatal("release outcome outside target operation was published")
	}
}

func rowBoundarySpec(callbackRowTail vocabulary.RowTail, mode vocabulary.CallbackReleaseMode) Spec {
	return Spec{Operations: []vocabulary.OperationSpec{
		{
			Bindings:   []vocabulary.BindingSpec{{Namespace: vocabulary.BindingBuiltin, Member: []string{"row-owner"}}},
			ValuesVars: 5,
			RowFormals: 1,
			Input:      vocabulary.ValuesSpec{Fixed: []schematype.Type{testAny}, Tail: vocabulary.ValuesVariable, Var: 0},
			Outcomes:   []vocabulary.OutcomeSpec{{Kind: flowkind.OutcomeNormal, Values: vocabulary.ValuesSpec{Tail: vocabulary.ValuesClosed}}},
			Effects:    vocabulary.RowSpec{Occurrences: []vocabulary.EffectSpec{{Target: 2, ValueArgs: []vocabulary.ValueFormal{0}, RowArgs: []vocabulary.RowVar{0}}}, Tail: vocabulary.RowVariable, Var: 0},
			Callbacks: []vocabulary.CallbackSpec{{
				Function: vocabulary.InputSource{Kind: vocabulary.InputSourceValueFormal}, Admission: schematype.CallableAdmissionOrdinary, Arguments: callbackTail(0),
				Outcomes: callbackOutcomes(1, 1, 2, 3, 4), Lifecycle: vocabulary.CallbackRetainedOptionalOnce,
				Effects: vocabulary.RowSpec{Occurrences: []vocabulary.EffectSpec{{Target: 2, ValueArgs: []vocabulary.ValueFormal{0}, RowArgs: []vocabulary.RowVar{0}}}, Tail: callbackRowTail, Var: 0},
				Release: &vocabulary.CallbackReleaseSpec{Operation: 2, Input: 0, Outcome: 0, Mode: mode, Zero: vocabulary.CallbackReleaseZeroSpec{Behavior: vocabulary.CallbackReleaseZeroIdempotent, Outcome: 0}},
			}},
		},
		{
			Bindings:   []vocabulary.BindingSpec{{Namespace: vocabulary.BindingBuiltin, Member: []string{"row-target"}}},
			RowFormals: 1,
			Input:      vocabulary.ValuesSpec{Fixed: []schematype.Type{testAny}, Tail: vocabulary.ValuesClosed},
			Outcomes:   []vocabulary.OutcomeSpec{{Kind: flowkind.OutcomeNormal, Values: vocabulary.ValuesSpec{Tail: vocabulary.ValuesClosed}}},
			Effects:    vocabulary.RowSpec{Tail: vocabulary.RowClosed},
		},
	}}
}
