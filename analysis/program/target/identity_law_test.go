package target

import (
	"github.com/wippyai/go-lua/analysis/identity"
	flowkind "github.com/wippyai/go-lua/analysis/program/flow/kind"
	"github.com/wippyai/go-lua/analysis/program/target/vocabulary"
	schematype "github.com/wippyai/go-lua/analysis/schema/typecontract"
	"testing"
)

func effectIdentityOperation(name string, effects vocabulary.RowSpec) vocabulary.OperationSpec {
	return vocabulary.OperationSpec{
		Bindings:   []vocabulary.BindingSpec{{Namespace: vocabulary.BindingBuiltin, Member: []string{name}}},
		RowFormals: 1,
		Input:      vocabulary.ValuesSpec{Fixed: []schematype.Type{testAny, testString}, Tail: vocabulary.ValuesClosed},
		Outcomes:   []vocabulary.OutcomeSpec{{Kind: flowkind.OutcomeNormal, Values: vocabulary.ValuesSpec{Tail: vocabulary.ValuesClosed}}},
		Effects:    effects,
	}
}

func effectIdentityCallbackOperation(name string, callbackEffects, operationEffects vocabulary.RowSpec) vocabulary.OperationSpec {
	empty := vocabulary.ValuesSpec{Tail: vocabulary.ValuesClosed}
	terminals := []vocabulary.TerminalSpec{
		{Kind: flowkind.OutcomeNormal, Values: empty},
		{Kind: flowkind.OutcomeReturn, Values: empty},
		{Kind: flowkind.OutcomeThrow, Values: empty},
		{Kind: flowkind.OutcomeYield, Values: empty},
		{Kind: flowkind.OutcomeCancel, Values: empty},
	}
	return vocabulary.OperationSpec{
		Bindings:   []vocabulary.BindingSpec{{Namespace: vocabulary.BindingBuiltin, Member: []string{name}}},
		RowFormals: 1,
		Input:      vocabulary.ValuesSpec{Fixed: []schematype.Type{testAny, testString}, Tail: vocabulary.ValuesClosed},
		Callbacks: []vocabulary.CallbackSpec{{
			Function: vocabulary.InputSource{Kind: vocabulary.InputSourceValueFormal}, Admission: schematype.CallableAdmissionOrdinary,
			Arguments: empty, Outcomes: terminals, Lifecycle: vocabulary.CallbackRetainedOptionalOnce,
			Effects: callbackEffects,
		}},
		Outcomes: []vocabulary.OutcomeSpec{{Kind: flowkind.OutcomeNormal, Values: empty}},
		Effects:  operationEffects,
	}
}

func effectIdentitySpec(operation vocabulary.OperationSpec) Spec {
	target := effectIdentityOperation("effect-target", vocabulary.RowSpec{Tail: vocabulary.RowClosed})
	return Spec{Operations: []vocabulary.OperationSpec{operation, target}}
}

func firstEffectIdentityContract(t *testing.T, spec Spec, name string) (*Contract, vocabulary.Operation, vocabulary.CallbackID) {
	t.Helper()
	contract := mustSeal(t, spec)
	op, ok := contract.Operations.Lookup(vocabulary.BindingSpec{Namespace: vocabulary.BindingBuiltin, Member: []string{name}})
	if !ok {
		t.Fatalf("operation %q absent", name)
	}
	callback, _ := contract.Operations.CallbackAt(op, 0)
	return contract, op, callback
}

func TestEffectIdentityDuplicateDescriptorHasDistinctOccurrences(t *testing.T) {
	effects := vocabulary.RowSpec{Occurrences: []vocabulary.EffectSpec{
		{Target: 2, ValueArgs: []vocabulary.ValueFormal{0, 1}, RowArgs: []vocabulary.RowVar{0}},
		{Target: 2, ValueArgs: []vocabulary.ValueFormal{0, 1}, RowArgs: []vocabulary.RowVar{0}},
	}, Tail: vocabulary.RowClosed}
	contract, owner, _ := firstEffectIdentityContract(t, effectIdentitySpec(effectIdentityOperation("effect-owner", effects)), "effect-owner")
	firstDescriptor, ok := contract.Operations.EffectDescriptorID(owner, 0)
	if !ok {
		t.Fatal("first effect descriptor unavailable")
	}
	secondDescriptor, ok := contract.Operations.EffectDescriptorID(owner, 1)
	if !ok || secondDescriptor != firstDescriptor {
		t.Fatal("duplicate effects did not share a descriptor")
	}
	firstOccurrence, ok := contract.Operations.EffectOccurrenceID(owner, 0)
	if !ok {
		t.Fatal("first effect occurrence unavailable")
	}
	secondOccurrence, ok := contract.Operations.EffectOccurrenceID(owner, 1)
	if !ok || secondOccurrence == firstOccurrence {
		t.Fatal("duplicate effects did not retain distinct occurrences")
	}
	firstFamily, ok := contract.Operations.EffectRowFamilyID(owner)
	if !ok {
		t.Fatal("effect family unavailable")
	}
	withoutDuplicate := vocabulary.RowSpec{Occurrences: effects.Occurrences[:1], Tail: vocabulary.RowClosed}
	withoutContract, withoutOwner, _ := firstEffectIdentityContract(t, effectIdentitySpec(effectIdentityOperation("effect-owner", withoutDuplicate)), "effect-owner")
	withoutDescriptor, ok := withoutContract.Operations.EffectDescriptorID(withoutOwner, 0)
	if !ok || withoutDescriptor != firstDescriptor {
		t.Fatal("duplicate count changed the semantic descriptor")
	}
	withoutFamily, ok := withoutContract.Operations.EffectRowFamilyID(withoutOwner)
	if !ok || withoutFamily == firstFamily {
		t.Fatal("duplicate count did not change the effect family")
	}
}

func TestEffectIdentityExcludesUnrelatedRowsButTracksABI(t *testing.T) {
	baseEffects := vocabulary.RowSpec{Occurrences: []vocabulary.EffectSpec{{Target: 2, ValueArgs: []vocabulary.ValueFormal{0, 1}, RowArgs: []vocabulary.RowVar{0}}}, Tail: vocabulary.RowClosed}
	base, owner, _ := firstEffectIdentityContract(t, effectIdentitySpec(effectIdentityOperation("effect-owner", baseEffects)), "effect-owner")
	baseOperation, ok := base.Operations.EffectOperationID(owner)
	if !ok {
		t.Fatal("effect operation identity unavailable")
	}
	baseDescriptor, ok := base.Operations.EffectDescriptorID(owner, 0)
	if !ok {
		t.Fatal("effect descriptor unavailable")
	}

	mutated := baseEffects
	mutated.Occurrences = append(mutated.Occurrences, vocabulary.EffectSpec{Target: 2, ValueArgs: []vocabulary.ValueFormal{1, 0}, RowArgs: []vocabulary.RowVar{0}})
	withExtra, extraOwner, _ := firstEffectIdentityContract(t, effectIdentitySpec(effectIdentityOperation("effect-owner", mutated)), "effect-owner")
	if got, ok := withExtra.Operations.EffectOperationID(extraOwner); !ok || got != baseOperation {
		t.Fatal("unrelated owner effect changed EffectOperationID")
	}
	if got, ok := withExtra.Operations.EffectDescriptorID(extraOwner, 0); !ok || got != baseDescriptor {
		t.Fatal("unrelated owner effect changed existing descriptor")
	}

	outcomeMutation := effectIdentityOperation("effect-owner", baseEffects)
	outcomeMutation.Outcomes = []vocabulary.OutcomeSpec{{Kind: flowkind.OutcomeNormal, Values: vocabulary.ValuesSpec{Fixed: []schematype.Type{testInteger}, Tail: vocabulary.ValuesClosed}}}
	withOutcome, outcomeOwner, _ := firstEffectIdentityContract(t, effectIdentitySpec(outcomeMutation), "effect-owner")
	if got, ok := withOutcome.Operations.EffectOperationID(outcomeOwner); !ok || got != baseOperation {
		t.Fatal("unrelated owner outcome changed EffectOperationID")
	}
	if got, ok := withOutcome.Operations.EffectDescriptorID(outcomeOwner, 0); !ok || got != baseDescriptor {
		t.Fatal("unrelated owner outcome changed descriptor")
	}

	abiMutation := effectIdentityOperation("effect-owner", baseEffects)
	abiMutation.Input.Fixed[0] = testInteger
	withABI, abiOwner, _ := firstEffectIdentityContract(t, effectIdentitySpec(abiMutation), "effect-owner")
	if got, ok := withABI.Operations.EffectOperationID(abiOwner); !ok || got == baseOperation {
		t.Fatal("input ABI mutation did not change EffectOperationID")
	}
	if got, ok := withABI.Operations.EffectDescriptorID(abiOwner, 0); !ok || got == baseDescriptor {
		t.Fatal("input ABI mutation did not change descriptor")
	}

	bindingMutation := effectIdentityOperation("effect-owner-renamed", baseEffects)
	withBinding, bindingOwner, _ := firstEffectIdentityContract(t, effectIdentitySpec(bindingMutation), "effect-owner-renamed")
	if got, ok := withBinding.Operations.EffectOperationID(bindingOwner); !ok || got == baseOperation {
		t.Fatal("binding mutation did not change EffectOperationID")
	}
}

func TestEffectIdentitySeparatesOrdinaryAndCallbackOccurrences(t *testing.T) {
	effects := vocabulary.RowSpec{Occurrences: []vocabulary.EffectSpec{{Target: 1, ValueArgs: []vocabulary.ValueFormal{0, 1}, RowArgs: []vocabulary.RowVar{0}}}, Tail: vocabulary.RowClosed}
	contract, owner, callback := firstEffectIdentityContract(t, effectIdentitySpec(effectIdentityCallbackOperation("effect-callback", effects, effects)), "effect-callback")
	if callback == 0 {
		t.Fatal("callback absent")
	}
	ordinaryDescriptor, ok := contract.Operations.EffectDescriptorID(owner, 0)
	if !ok {
		t.Fatal("ordinary descriptor unavailable")
	}
	callbackDescriptor, ok := contract.Operations.CallbackEffectDescriptorID(callback, 0)
	if !ok || callbackDescriptor != ordinaryDescriptor {
		t.Fatal("equivalent ordinary and callback effects did not share descriptor")
	}
	ordinaryOccurrence, ok := contract.Operations.EffectOccurrenceID(owner, 0)
	if !ok {
		t.Fatal("ordinary occurrence unavailable")
	}
	callbackOccurrence, ok := contract.Operations.CallbackEffectOccurrenceID(callback, 0)
	if !ok || callbackOccurrence == ordinaryOccurrence {
		t.Fatal("ordinary and callback occurrences were conflated")
	}
	if ordinaryFamily, ok := contract.Operations.EffectRowFamilyID(owner); !ok {
		t.Fatal("ordinary family unavailable")
	} else if callbackFamily, ok := contract.Operations.CallbackEffectRowFamilyID(callback); !ok || callbackFamily == ordinaryFamily {
		t.Fatal("ordinary and callback families were conflated")
	}
}

func TestEffectIdentityCallbackTailChangesFamilyOnly(t *testing.T) {
	closed := vocabulary.RowSpec{Occurrences: []vocabulary.EffectSpec{{Target: 1, ValueArgs: []vocabulary.ValueFormal{0, 1}, RowArgs: []vocabulary.RowVar{0}}}, Tail: vocabulary.RowClosed}
	variable := vocabulary.RowSpec{Occurrences: []vocabulary.EffectSpec{{Target: 1, ValueArgs: []vocabulary.ValueFormal{0, 1}, RowArgs: []vocabulary.RowVar{0}}}, Tail: vocabulary.RowVariable, Var: 0}
	base, _, baseCallback := firstEffectIdentityContract(t, effectIdentitySpec(effectIdentityCallbackOperation("effect-callback", closed, vocabulary.RowSpec{Tail: vocabulary.RowClosed})), "effect-callback")
	changed, _, changedCallback := firstEffectIdentityContract(t, effectIdentitySpec(effectIdentityCallbackOperation("effect-callback", variable, vocabulary.RowSpec{Tail: vocabulary.RowClosed})), "effect-callback")
	baseDescriptor, ok := base.Operations.CallbackEffectDescriptorID(baseCallback, 0)
	if !ok {
		t.Fatal("base callback descriptor unavailable")
	}
	changedDescriptor, ok := changed.Operations.CallbackEffectDescriptorID(changedCallback, 0)
	if !ok || changedDescriptor != baseDescriptor {
		t.Fatal("callback row tail changed descriptor")
	}
	baseOccurrence, ok := base.Operations.CallbackEffectOccurrenceID(baseCallback, 0)
	if !ok {
		t.Fatal("base callback occurrence unavailable")
	}
	changedOccurrence, ok := changed.Operations.CallbackEffectOccurrenceID(changedCallback, 0)
	if !ok || changedOccurrence != baseOccurrence {
		t.Fatal("callback row tail changed occurrence")
	}
	baseFamily, ok := base.Operations.CallbackEffectRowFamilyID(baseCallback)
	if !ok {
		t.Fatal("base callback family unavailable")
	}
	changedFamily, ok := changed.Operations.CallbackEffectRowFamilyID(changedCallback)
	if !ok || changedFamily == baseFamily {
		t.Fatal("callback row tail did not change family")
	}
}

func TestEffectIdentityQueriesAllocateNothingAndReplay(t *testing.T) {
	effects := vocabulary.RowSpec{Occurrences: []vocabulary.EffectSpec{{Target: 2, ValueArgs: []vocabulary.ValueFormal{0, 1}, RowArgs: []vocabulary.RowVar{0}}}, Tail: vocabulary.RowClosed}
	left, owner, _ := firstEffectIdentityContract(t, effectIdentitySpec(effectIdentityOperation("effect-owner", effects)), "effect-owner")
	right, rightOwner, _ := firstEffectIdentityContract(t, effectIdentitySpec(effectIdentityOperation("effect-owner", effects)), "effect-owner")
	leftOperation, ok := left.Operations.EffectOperationID(owner)
	if !ok {
		t.Fatal("left operation identity unavailable")
	}
	if rightOperation, ok := right.Operations.EffectOperationID(rightOwner); !ok || rightOperation != leftOperation {
		t.Fatal("equivalent reseal changed operation identity")
	}
	if allocs := testing.AllocsPerRun(1000, func() {
		_, _ = left.Operations.EffectOperationID(owner)
		_, _ = left.Operations.EffectDescriptorID(owner, 0)
		_, _ = left.Operations.EffectOccurrenceID(owner, 0)
		_, _ = left.Operations.EffectRowFamilyID(owner)
	}); allocs != 0 {
		t.Fatalf("effect identity queries allocated %f times", allocs)
	}
}

func TestEffectIdentityEmptyOrdinaryAndOpaqueFamiliesAreAvailable(t *testing.T) {
	spec := effectIdentitySpec(effectIdentityOperation("effect-empty", vocabulary.RowSpec{Tail: vocabulary.RowClosed}))
	left, owner, _ := firstEffectIdentityContract(t, spec, "effect-empty")
	leftFamily, ok := left.Operations.EffectRowFamilyID(owner)
	if !ok || !leftFamily.Available() {
		t.Fatal("empty ordinary effect family unavailable")
	}
	opaque, ok := left.Operations.Opaque()
	if !ok {
		t.Fatal("opaque operation unavailable")
	}
	opaqueFamily, ok := left.Operations.EffectRowFamilyID(opaque)
	if !ok || !opaqueFamily.Available() {
		t.Fatal("opaque effect family unavailable")
	}
	right, rightOwner, _ := firstEffectIdentityContract(t, effectIdentitySpec(effectIdentityOperation("effect-empty", vocabulary.RowSpec{Tail: vocabulary.RowClosed})), "effect-empty")
	if got, ok := right.Operations.EffectRowFamilyID(rightOwner); !ok || got != leftFamily {
		t.Fatal("empty ordinary family changed across reseal")
	}
	rightOpaque, ok := right.Operations.Opaque()
	if !ok {
		t.Fatal("opaque operation unavailable after reseal")
	}
	if got, ok := right.Operations.EffectRowFamilyID(rightOpaque); !ok || got != opaqueFamily {
		t.Fatal("opaque family changed across reseal")
	}
}

func TestEffectIdentityCallbackEmptyFamilyIgnoresOwnerABI(t *testing.T) {
	empty := vocabulary.RowSpec{Tail: vocabulary.RowClosed}
	baseSpec := effectIdentitySpec(effectIdentityCallbackOperation("effect-callback-empty", empty, empty))
	base, _, baseCallback := firstEffectIdentityContract(t, baseSpec, "effect-callback-empty")
	baseFamily, ok := base.Operations.CallbackEffectRowFamilyID(baseCallback)
	if !ok || !baseFamily.Available() {
		t.Fatal("empty callback effect family unavailable")
	}
	changedOperation := effectIdentityCallbackOperation("effect-callback-empty", empty, empty)
	changedOperation.Input.Fixed[0] = testInteger
	changed, _, changedCallback := firstEffectIdentityContract(t, effectIdentitySpec(changedOperation), "effect-callback-empty")
	if got, ok := changed.Operations.CallbackEffectRowFamilyID(changedCallback); !ok || got != baseFamily {
		t.Fatal("empty callback family churned with owner ABI")
	}
}

func TestEffectIdentityTracksFormalAndOpenABI(t *testing.T) {
	formalSpec := func(constraint schematype.Type) Spec {
		formal := testNewTypeParam("T", constraint)
		declarations, formals := testOperationTypes(formal)
		return Spec{Operations: []vocabulary.OperationSpec{{
			Bindings:    []vocabulary.BindingSpec{{Namespace: vocabulary.BindingBuiltin, Member: []string{"effect-formal-abi"}}},
			TypeFormals: formals,
			Input:       vocabulary.ValuesSpec{Fixed: []schematype.Type{declarations[0]}, Tail: vocabulary.ValuesClosed},
			Outcomes:    []vocabulary.OutcomeSpec{{Kind: flowkind.OutcomeNormal, Values: vocabulary.ValuesSpec{Tail: vocabulary.ValuesClosed}}},
			Effects:     vocabulary.RowSpec{Tail: vocabulary.RowClosed},
		}}}
	}
	formalID := func(spec Spec) (identity.ContentID, bool) {
		contract := mustSeal(t, spec)
		op, ok := contract.Operations.Lookup(vocabulary.BindingSpec{Namespace: vocabulary.BindingBuiltin, Member: []string{"effect-formal-abi"}})
		if !ok {
			t.Fatal("formal ABI operation absent")
		}
		return contract.Operations.EffectOperationID(op)
	}
	formalNumber, ok := formalID(formalSpec(testNumber))
	if !ok {
		t.Fatal("number formal ABI identity unavailable")
	}
	formalString, ok := formalID(formalSpec(testString))
	if !ok || formalString == formalNumber {
		t.Fatal("type formal constraint mutation did not change EffectOperationID")
	}

	valuesSpec := func(tailType schematype.Type) Spec {
		return Spec{Operations: []vocabulary.OperationSpec{{
			Bindings:   []vocabulary.BindingSpec{{Namespace: vocabulary.BindingBuiltin, Member: []string{"effect-values-abi"}}},
			ValuesVars: 1,
			Input:      vocabulary.ValuesSpec{Fixed: []schematype.Type{testAny}, Tail: vocabulary.ValuesVariable, Var: 0, TailType: tailType},
			Outcomes:   []vocabulary.OutcomeSpec{{Kind: flowkind.OutcomeNormal, Values: vocabulary.ValuesSpec{Tail: vocabulary.ValuesClosed}}},
			Effects:    vocabulary.RowSpec{Tail: vocabulary.RowClosed},
		}}}
	}
	valuesID := func(spec Spec) (identity.ContentID, bool) {
		contract := mustSeal(t, spec)
		op, ok := contract.Operations.Lookup(vocabulary.BindingSpec{Namespace: vocabulary.BindingBuiltin, Member: []string{"effect-values-abi"}})
		if !ok {
			t.Fatal("Values ABI operation absent")
		}
		return contract.Operations.EffectOperationID(op)
	}
	valuesString, ok := valuesID(valuesSpec(testString))
	if !ok {
		t.Fatal("string Values ABI identity unavailable")
	}
	valuesInteger, ok := valuesID(valuesSpec(testInteger))
	if !ok || valuesInteger == valuesString {
		t.Fatal("ValuesVar ABI mutation did not change EffectOperationID")
	}

	rowSpec := func(count uint32) Spec {
		return Spec{Operations: []vocabulary.OperationSpec{{
			Bindings:   []vocabulary.BindingSpec{{Namespace: vocabulary.BindingBuiltin, Member: []string{"effect-row-abi"}}},
			RowFormals: count,
			Input:      vocabulary.ValuesSpec{Tail: vocabulary.ValuesClosed},
			Outcomes:   []vocabulary.OutcomeSpec{{Kind: flowkind.OutcomeNormal, Values: vocabulary.ValuesSpec{Tail: vocabulary.ValuesClosed}}},
			Effects:    vocabulary.RowSpec{Tail: vocabulary.RowClosed},
		}}}
	}
	rowOne := mustSeal(t, rowSpec(1))
	rowTwo := mustSeal(t, rowSpec(2))
	rowOneOp, ok := rowOne.Operations.Lookup(vocabulary.BindingSpec{Namespace: vocabulary.BindingBuiltin, Member: []string{"effect-row-abi"}})
	if !ok {
		t.Fatal("one-row-formal operation absent")
	}
	rowTwoOp, ok := rowTwo.Operations.Lookup(vocabulary.BindingSpec{Namespace: vocabulary.BindingBuiltin, Member: []string{"effect-row-abi"}})
	if !ok {
		t.Fatal("two-row-formal operation absent")
	}
	rowOneID, ok := rowOne.Operations.EffectOperationID(rowOneOp)
	if !ok {
		t.Fatal("one-row-formal identity unavailable")
	}
	rowTwoID, ok := rowTwo.Operations.EffectOperationID(rowTwoOp)
	if !ok || rowTwoID == rowOneID {
		t.Fatal("row formal count mutation did not change EffectOperationID")
	}
}

func TestHostPrerequisiteIDsRoundTripAndFence(t *testing.T) {
	contract := mustSeal(t, endpointIdentityTransferDependencies(1))
	op, _ := contract.Operations.Lookup(vocabulary.BindingSpec{Namespace: vocabulary.BindingBuiltin, Member: []string{"transfer-dependencies"}})
	input, ok := contract.InputFormalID(op, 1)
	if !ok || !input.Available() {
		t.Fatal("input formal identity unavailable")
	}
	if foundOp, foundFormal, found := contract.FindInputFormalID(input); !found || foundOp != op || foundFormal != 1 {
		t.Fatalf("input inverse = %d/%d/%v", foundOp, foundFormal, found)
	}
	result, ok := contract.OutcomeResultID(op, 0, 1)
	if !ok || !result.Available() {
		t.Fatal("outcome result identity unavailable")
	}
	if foundOp, outcome, slot, found := contract.FindOutcomeResultID(result); !found || foundOp != op || outcome != 0 || slot != 1 {
		t.Fatalf("result inverse = %d/%d/%d/%v", foundOp, outcome, slot, found)
	}
	if _, ok := contract.InputFormalID(op, 2); ok {
		t.Fatal("out-of-range input formal accepted")
	}
	if _, ok := contract.OutcomeResultID(op, 0, 2); ok {
		t.Fatal("out-of-range outcome result accepted")
	}
	if _, ok := contract.InputFormalID(0, 0); ok {
		t.Fatal("zero operation accepted for input formal")
	}
	if _, ok := contract.OutcomeResultID(0, 0, 0); ok {
		t.Fatal("zero operation accepted for outcome result")
	}
}

func TestHostPrerequisiteIDsAreLocalAndReplayStable(t *testing.T) {
	base := mustSeal(t, endpointIdentityTransferDependencies(1))
	op, _ := base.Operations.Lookup(vocabulary.BindingSpec{Namespace: vocabulary.BindingBuiltin, Member: []string{"transfer-dependencies"}})
	input, _ := base.InputFormalID(op, 0)
	result, _ := base.OutcomeResultID(op, 0, 1)
	withUnrelated := endpointIdentityTransferDependencies(1)
	withUnrelated.Operations = append(withUnrelated.Operations, endpointIdentityOperation("aardvark-host-id"))
	changed := mustSeal(t, withUnrelated)
	op, _ = changed.Operations.Lookup(vocabulary.BindingSpec{Namespace: vocabulary.BindingBuiltin, Member: []string{"transfer-dependencies"}})
	if got, ok := changed.InputFormalID(op, 0); !ok || got != input {
		t.Fatal("unrelated operation changed input formal ID")
	}
	if got, ok := changed.OutcomeResultID(op, 0, 1); !ok || got != result {
		t.Fatal("unrelated operation changed outcome result ID")
	}
	mutation := mustSeal(t, endpointIdentityTransferDependencies(2))
	op, _ = mutation.Operations.Lookup(vocabulary.BindingSpec{Namespace: vocabulary.BindingBuiltin, Member: []string{"transfer-dependencies"}})
	if got, _ := mutation.OutcomeResultID(op, 0, 1); got == result {
		t.Fatal("callback-result mutation did not change outcome result ID")
	}
	if allocations := testing.AllocsPerRun(1000, func() {
		_, _ = base.InputFormalID(1, 0)
		_, _, _ = base.FindInputFormalID(input)
		_, _ = base.OutcomeResultID(1, 0, 1)
		_, _, _, _ = base.FindOutcomeResultID(result)
	}); allocations != 0 {
		t.Fatalf("host prerequisite queries allocated %v times", allocations)
	}
}

func TestInputFormalIDPermutationReplayAndForeignRangeFence(t *testing.T) {
	left := mustSeal(t, Spec{Operations: []vocabulary.OperationSpec{endpointIdentityOperation("alpha-formal"), endpointIdentityOperation("beta-formal")}})
	right := mustSeal(t, Spec{Operations: []vocabulary.OperationSpec{endpointIdentityOperation("beta-formal"), endpointIdentityOperation("alpha-formal")}})
	leftOp, _ := left.Operations.Lookup(vocabulary.BindingSpec{Namespace: vocabulary.BindingBuiltin, Member: []string{"alpha-formal"}})
	rightOp, _ := right.Operations.Lookup(vocabulary.BindingSpec{Namespace: vocabulary.BindingBuiltin, Member: []string{"alpha-formal"}})
	leftID, leftOK := left.InputFormalID(leftOp, 0)
	rightID, rightOK := right.InputFormalID(rightOp, 0)
	if !leftOK || !rightOK || leftID != rightID {
		t.Fatal("input formal ID changed under authoring permutation")
	}
	foreign := mustSeal(t, Spec{Operations: []vocabulary.OperationSpec{endpointIdentityOperation("foreign-formal"), endpointIdentityOperation("foreign-other")}})
	foreignOp, _ := foreign.Operations.Lookup(vocabulary.BindingSpec{Namespace: vocabulary.BindingBuiltin, Member: []string{"foreign-other"}})
	if _, ok := left.InputFormalID(foreignOp, 1); ok {
		t.Fatal("foreign out-of-range scalar coordinate accepted")
	}
	if _, _, ok := left.FindInputFormalID(identity.ContentID{}); ok {
		t.Fatal("zero input ID inverted")
	}
}

func TestBootAndInitialValueIDsStayNarrow(t *testing.T) {
	baseSpec := completeBootSpec("Lua 5.3", vocabulary.InitialMutable)
	base := mustSeal(t, baseSpec)
	boot, ok := base.BootRelationID()
	if !ok {
		t.Fatal("boot relation unavailable")
	}
	var version vocabulary.InitialValue
	for index := 0; index < base.InitialEntryCount(); index++ {
		_, key, value, _, found := base.InitialEntryAt(index)
		literal, keyOK := base.ExactKeyValue(key)
		if found && keyOK && literal.String == "_VERSION" {
			version = value
			break
		}
	}
	versionID, ok := base.InitialValueContentID(version)
	if !ok {
		t.Fatal("version value identity unavailable")
	}

	unrelated := completeBootSpec("Lua 5.3", vocabulary.InitialMutable)
	unrelated.Operations = append(unrelated.Operations, endpointIdentityOperation("aardvark-boot"))
	other := mustSeal(t, unrelated)
	if got, ok := other.BootRelationID(); !ok || got != boot {
		t.Fatal("unrelated operation changed boot relation")
	}
	var otherVersion vocabulary.InitialValue
	for index := 0; index < other.InitialEntryCount(); index++ {
		_, key, value, _, found := other.InitialEntryAt(index)
		literal, keyOK := other.ExactKeyValue(key)
		if found && keyOK && literal.String == "_VERSION" {
			otherVersion = value
			break
		}
	}
	if got, ok := other.InitialValueContentID(otherVersion); !ok || got != versionID {
		t.Fatal("unrelated operation changed initial value identity")
	}
	bodyMutation := completeBootSpec("Lua 5.3", vocabulary.InitialMutable)
	bodyMutation.Operations[0].Outcomes = append(bodyMutation.Operations[0].Outcomes, vocabulary.OutcomeSpec{Kind: flowkind.OutcomeThrow, Values: vocabulary.ValuesSpec{Tail: vocabulary.ValuesClosed}})
	if got, ok := mustSeal(t, bodyMutation).BootRelationID(); !ok || got != boot {
		t.Fatal("unobserved operation body changed boot relation")
	}

	rootMutation := completeBootSpec("Lua 5.3", vocabulary.InitialMutable)
	rootMutation.InitialRoots[0].Shape.Immutable = true
	if mutated, ok := mustSeal(t, rootMutation).BootRelationID(); !ok || mutated == boot {
		t.Fatal("root shape mutation did not change boot relation")
	}
	valueMutation := completeBootSpec("Lua 5.4", vocabulary.InitialMutable)
	mutated := mustSeal(t, valueMutation)
	for index := 0; index < mutated.InitialEntryCount(); index++ {
		_, key, value, _, found := mutated.InitialEntryAt(index)
		literal, keyOK := mutated.ExactKeyValue(key)
		if found && keyOK && literal.String == "_VERSION" {
			if got, ok := mutated.InitialValueContentID(value); !ok || got == versionID {
				t.Fatal("initial value mutation did not change value identity")
			}
			break
		}
	}
}

func TestSemanticIdentityQueriesTrackOperationAndOutcomeOwners(t *testing.T) {
	contract := mustSeal(t, Spec{Operations: []vocabulary.OperationSpec{contentIDOperation("semantic-id", []vocabulary.OutcomeSpec{{
		Kind:   flowkind.OutcomeNormal,
		Values: vocabulary.ValuesSpec{Fixed: []schematype.Type{testString}, Tail: vocabulary.ValuesClosed},
	}})}})
	op, ok := contract.Operations.Lookup(vocabulary.BindingSpec{Namespace: vocabulary.BindingBuiltin, Member: []string{"semantic-id"}})
	if !ok {
		t.Fatal("semantic identity operation missing")
	}
	operationID, ok := contract.OperationContentID(op)
	if !ok || !operationID.Available() {
		t.Fatalf("OperationContentID = %v/%v", operationID, ok)
	}
	outcomeID, ok := contract.outcomeContentID(op, 0)
	if !ok || !outcomeID.Available() || operationID == outcomeID {
		t.Fatalf("OutcomeContentID = %v/%v; operation identity = %v", outcomeID, ok, operationID)
	}
	again, againOK := contract.OperationContentID(op)
	if !againOK || again != operationID {
		t.Fatal("operation identity was not replay-stable")
	}
}

func TestRelationIdentityQueriesRoundTripFormalAndOutcomeRows(t *testing.T) {
	contract := mustSeal(t, Spec{Operations: []vocabulary.OperationSpec{contentIDOperation("relation-id", []vocabulary.OutcomeSpec{{
		Kind:   flowkind.OutcomeNormal,
		Values: vocabulary.ValuesSpec{Fixed: []schematype.Type{testString}, Tail: vocabulary.ValuesClosed},
	}})}})
	op, ok := contract.Operations.Lookup(vocabulary.BindingSpec{Namespace: vocabulary.BindingBuiltin, Member: []string{"relation-id"}})
	if !ok {
		t.Fatal("relation identity operation missing")
	}
	formal, ok := contract.InputFormalID(op, 0)
	if !ok || !formal.Available() {
		t.Fatalf("InputFormalID = %v/%v", formal, ok)
	}
	if owner, coordinate, ok := contract.FindInputFormalID(formal); !ok || owner != op || coordinate != 0 {
		t.Fatalf("FindInputFormalID = %d/%d/%v", owner, coordinate, ok)
	}
	result, ok := contract.OutcomeResultID(op, 0, 0)
	if !ok || !result.Available() {
		t.Fatalf("OutcomeResultID = %v/%v", result, ok)
	}
	if owner, outcome, ordinal, ok := contract.FindOutcomeResultID(result); !ok || owner != op || outcome != 0 || ordinal != 0 {
		t.Fatalf("FindOutcomeResultID = %d/%d/%d/%v", owner, outcome, ordinal, ok)
	}
}

func callbackResumeContentOperation(name string, callbackFormal, resumeCarrier vocabulary.ValueFormal) vocabulary.OperationSpec {
	return vocabulary.OperationSpec{
		Bindings:   []vocabulary.BindingSpec{{Namespace: vocabulary.BindingBuiltin, Member: []string{name}}},
		ValuesVars: 5,
		Input:      vocabulary.ValuesSpec{Fixed: []schematype.Type{testAny, testAny}, Tail: vocabulary.ValuesVariable, Var: 0},
		Outcomes: []vocabulary.OutcomeSpec{
			{Kind: flowkind.OutcomeNormal, Values: callbackTail(0)},
		},
		Callbacks: []vocabulary.CallbackSpec{{
			Function:  vocabulary.InputSource{Kind: vocabulary.InputSourceValueFormal, Ordinal: uint32(callbackFormal)},
			Admission: schematype.CallableAdmissionOrdinary,
			Arguments: callbackTail(1),
			Outcomes:  callbackOutcomes(1, 1, 2, 3, 4),
			Lifecycle: vocabulary.CallbackRetainedOptionalOnce,
			Effects:   vocabulary.RowSpec{Tail: vocabulary.RowClosed},
		}},
		Resumes: []vocabulary.ResumeSpec{{
			Source:    vocabulary.ResumeSourceValueFormal,
			Carrier:   resumeCarrier,
			Arguments: callbackTail(0),
			Outcomes: []vocabulary.ResumeOutcomeSpec{
				{Kind: flowkind.OutcomeNormal, Outcome: 0},
				{Kind: flowkind.OutcomeReturn, Outcome: 0},
				{Kind: flowkind.OutcomeThrow, Outcome: 0},
				{Kind: flowkind.OutcomeYield, Outcome: 0},
				{Kind: flowkind.OutcomeCancel, Outcome: 0},
			},
		}},
		Effects: vocabulary.RowSpec{Tail: vocabulary.RowClosed},
	}
}

func TestCallbackAndResumeContentIDsFenceOwnersAndInvert(t *testing.T) {
	contract := mustSeal(t, Spec{Operations: []vocabulary.OperationSpec{
		callbackResumeContentOperation("alpha", 0, 0),
		callbackResumeContentOperation("beta", 1, 1),
	}})
	alpha, ok := contract.Operations.Lookup(vocabulary.BindingSpec{Namespace: vocabulary.BindingBuiltin, Member: []string{"alpha"}})
	if !ok {
		t.Fatal("alpha operation missing")
	}
	beta, ok := contract.Operations.Lookup(vocabulary.BindingSpec{Namespace: vocabulary.BindingBuiltin, Member: []string{"beta"}})
	if !ok {
		t.Fatal("beta operation missing")
	}
	alphaCallback, _ := contract.Operations.CallbackAt(alpha, 0)
	betaCallback, _ := contract.Operations.CallbackAt(beta, 0)
	alphaResume, _ := contract.Operations.ResumeIDAt(alpha, 0)
	betaResume, _ := contract.Operations.ResumeIDAt(beta, 0)

	callback, ok := contract.CallbackContentID(alpha, alphaCallback)
	if !ok || !callback.Available() {
		t.Fatal("alpha callback content identity unavailable")
	}
	if owner, got, ok := contract.findCallbackContentID(callback); !ok || owner != alpha || got != alphaCallback {
		t.Fatalf("callback inverse = %d/%d/%v", owner, got, ok)
	}
	resume, ok := contract.ResumeContentID(alpha, alphaResume)
	if !ok || !resume.Available() {
		t.Fatal("alpha resume content identity unavailable")
	}
	if owner, got, ok := contract.findResumeContentID(resume); !ok || owner != alpha || got != alphaResume {
		t.Fatalf("resume inverse = %d/%d/%v", owner, got, ok)
	}
	if _, ok := contract.CallbackContentID(beta, alphaCallback); ok {
		t.Fatal("callback content identity accepted a foreign owner")
	}
	if _, ok := contract.ResumeContentID(beta, alphaResume); ok {
		t.Fatal("resume content identity accepted a foreign owner")
	}
	if _, _, ok := contract.findCallbackContentID(identity.ContentID{}); ok {
		t.Fatal("zero callback content identity inverted")
	}
	if _, _, ok := contract.findResumeContentID(identity.ContentID{}); ok {
		t.Fatal("zero resume content identity inverted")
	}
	if _, ok := contract.CallbackContentID(alpha, 0); ok {
		t.Fatal("zero callback handle accepted")
	}
	if _, ok := contract.ResumeContentID(alpha, 0); ok {
		t.Fatal("zero resume handle accepted")
	}
	if allocs := testing.AllocsPerRun(1000, func() {
		if _, ok := contract.CallbackContentID(alpha, alphaCallback); !ok {
			panic("callback identity disappeared")
		}
		if _, _, ok := contract.findCallbackContentID(callback); !ok {
			panic("callback inverse disappeared")
		}
		if _, ok := contract.ResumeContentID(alpha, alphaResume); !ok {
			panic("resume identity disappeared")
		}
		if _, _, ok := contract.findResumeContentID(resume); !ok {
			panic("resume inverse disappeared")
		}
	}); allocs != 0 {
		t.Fatalf("content identity queries allocated %f times", allocs)
	}
	if betaCallback == alphaCallback || betaResume == alphaResume {
		t.Fatal("operation-local handles unexpectedly overlap")
	}
}

func TestCallbackAndResumeContentIDsAreReplayAndPermutationStable(t *testing.T) {
	left := mustSeal(t, Spec{Operations: []vocabulary.OperationSpec{
		callbackResumeContentOperation("beta", 1, 1),
		callbackResumeContentOperation("alpha", 0, 0),
	}})
	right := mustSeal(t, Spec{Operations: []vocabulary.OperationSpec{
		callbackResumeContentOperation("alpha", 0, 0),
		callbackResumeContentOperation("beta", 1, 1),
	}})
	for _, name := range []string{"alpha", "beta"} {
		leftOp, _ := left.Operations.Lookup(vocabulary.BindingSpec{Namespace: vocabulary.BindingBuiltin, Member: []string{name}})
		rightOp, _ := right.Operations.Lookup(vocabulary.BindingSpec{Namespace: vocabulary.BindingBuiltin, Member: []string{name}})
		leftCallback, _ := left.Operations.CallbackAt(leftOp, 0)
		rightCallback, _ := right.Operations.CallbackAt(rightOp, 0)
		leftCallbackID, leftOK := left.CallbackContentID(leftOp, leftCallback)
		rightCallbackID, rightOK := right.CallbackContentID(rightOp, rightCallback)
		if !leftOK || !rightOK || leftCallbackID != rightCallbackID {
			t.Fatalf("%s callback identity changed across replay", name)
		}
		leftResume, _ := left.Operations.ResumeIDAt(leftOp, 0)
		rightResume, _ := right.Operations.ResumeIDAt(rightOp, 0)
		leftResumeID, leftOK := left.ResumeContentID(leftOp, leftResume)
		rightResumeID, rightOK := right.ResumeContentID(rightOp, rightResume)
		if !leftOK || !rightOK || leftResumeID != rightResumeID {
			t.Fatalf("%s resume identity changed across replay", name)
		}
	}
}

func TestCallbackAndResumeContentIDsTrackDescriptorMutation(t *testing.T) {
	base := mustSeal(t, Spec{Operations: []vocabulary.OperationSpec{callbackResumeContentOperation("mutable", 0, 0)}})
	callback, _ := base.Operations.CallbackAt(1, 0)
	resume, _ := base.Operations.ResumeIDAt(1, 0)
	baseCallback, _ := base.CallbackContentID(1, callback)
	baseResume, _ := base.ResumeContentID(1, resume)

	changedCallback := callbackResumeContentOperation("mutable", 1, 0)
	changedResume := callbackResumeContentOperation("mutable", 0, 1)
	callbackContract := mustSeal(t, Spec{Operations: []vocabulary.OperationSpec{changedCallback}})
	resumeContract := mustSeal(t, Spec{Operations: []vocabulary.OperationSpec{changedResume}})
	callbackAfter, _ := callbackContract.CallbackContentID(1, callback)
	resumeAfter, _ := resumeContract.ResumeContentID(1, resume)
	if callbackAfter == baseCallback {
		t.Fatal("callback descriptor mutation reused content identity")
	}
	if resumeAfter == baseResume {
		t.Fatal("resume descriptor mutation reused content identity")
	}
}
