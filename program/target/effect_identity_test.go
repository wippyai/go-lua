package target

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/type/typ"
	flowkind "github.com/wippyai/go-lua/program/flow/kind"
	"github.com/wippyai/go-lua/program/keyspace"
)

func effectIdentityOperation(name string, effects RowSpec) OperationSpec {
	return OperationSpec{
		Bindings:   []BindingSpec{{Namespace: BindingBuiltin, Member: []string{name}}},
		RowFormals: 1,
		Input:      ValuesSpec{Fixed: []typ.Type{typ.Any, typ.String}, Tail: ValuesClosed},
		Outcomes:   []OutcomeSpec{{Kind: flowkind.OutcomeNormal, Values: ValuesSpec{Tail: ValuesClosed}}},
		Effects:    effects,
	}
}

func effectIdentityCallbackOperation(name string, callbackEffects, operationEffects RowSpec) OperationSpec {
	empty := ValuesSpec{Tail: ValuesClosed}
	terminals := []TerminalSpec{
		{Kind: flowkind.OutcomeNormal, Values: empty},
		{Kind: flowkind.OutcomeReturn, Values: empty},
		{Kind: flowkind.OutcomeThrow, Values: empty},
		{Kind: flowkind.OutcomeYield, Values: empty},
		{Kind: flowkind.OutcomeCancel, Values: empty},
	}
	return OperationSpec{
		Bindings:   []BindingSpec{{Namespace: BindingBuiltin, Member: []string{name}}},
		RowFormals: 1,
		Input:      ValuesSpec{Fixed: []typ.Type{typ.Any, typ.String}, Tail: ValuesClosed},
		Callbacks: []CallbackSpec{{
			Function: InputSource{Kind: InputSourceValueFormal}, Admission: OrdinaryCallable,
			Arguments: empty, Outcomes: terminals, Lifecycle: CallbackRetainedOptionalOnce,
			Effects: callbackEffects,
		}},
		Outcomes: []OutcomeSpec{{Kind: flowkind.OutcomeNormal, Values: empty}},
		Effects:  operationEffects,
	}
}

func effectIdentitySpec(operation OperationSpec) Spec {
	target := effectIdentityOperation("effect-target", RowSpec{Tail: RowClosed})
	return Spec{Operations: []OperationSpec{operation, target}}
}

func firstEffectIdentityContract(t *testing.T, spec Spec, name string) (*Contract, Operation, CallbackID) {
	t.Helper()
	contract := mustSeal(t, spec)
	op, ok := contract.Lookup(BindingSpec{Namespace: BindingBuiltin, Member: []string{name}})
	if !ok {
		t.Fatalf("operation %q absent", name)
	}
	callback, _ := contract.CallbackAt(op, 0)
	return contract, op, callback
}

func TestEffectIdentityDuplicateDescriptorHasDistinctOccurrences(t *testing.T) {
	effects := RowSpec{Occurrences: []EffectSpec{
		{Target: 2, ValueArgs: []ValueFormal{0, 1}, RowArgs: []RowVar{0}},
		{Target: 2, ValueArgs: []ValueFormal{0, 1}, RowArgs: []RowVar{0}},
	}, Tail: RowClosed}
	contract, owner, _ := firstEffectIdentityContract(t, effectIdentitySpec(effectIdentityOperation("effect-owner", effects)), "effect-owner")
	firstDescriptor, ok := contract.EffectDescriptorID(owner, 0)
	if !ok {
		t.Fatal("first effect descriptor unavailable")
	}
	secondDescriptor, ok := contract.EffectDescriptorID(owner, 1)
	if !ok || secondDescriptor != firstDescriptor {
		t.Fatal("duplicate effects did not share a descriptor")
	}
	firstOccurrence, ok := contract.EffectOccurrenceID(owner, 0)
	if !ok {
		t.Fatal("first effect occurrence unavailable")
	}
	secondOccurrence, ok := contract.EffectOccurrenceID(owner, 1)
	if !ok || secondOccurrence == firstOccurrence {
		t.Fatal("duplicate effects did not retain distinct occurrences")
	}
	firstFamily, ok := contract.EffectRowFamilyID(owner)
	if !ok {
		t.Fatal("effect family unavailable")
	}
	withoutDuplicate := RowSpec{Occurrences: effects.Occurrences[:1], Tail: RowClosed}
	withoutContract, withoutOwner, _ := firstEffectIdentityContract(t, effectIdentitySpec(effectIdentityOperation("effect-owner", withoutDuplicate)), "effect-owner")
	withoutDescriptor, ok := withoutContract.EffectDescriptorID(withoutOwner, 0)
	if !ok || withoutDescriptor != firstDescriptor {
		t.Fatal("duplicate count changed the semantic descriptor")
	}
	withoutFamily, ok := withoutContract.EffectRowFamilyID(withoutOwner)
	if !ok || withoutFamily == firstFamily {
		t.Fatal("duplicate count did not change the effect family")
	}
}

func TestEffectIdentityExcludesUnrelatedRowsButTracksABI(t *testing.T) {
	baseEffects := RowSpec{Occurrences: []EffectSpec{{Target: 2, ValueArgs: []ValueFormal{0, 1}, RowArgs: []RowVar{0}}}, Tail: RowClosed}
	base, owner, _ := firstEffectIdentityContract(t, effectIdentitySpec(effectIdentityOperation("effect-owner", baseEffects)), "effect-owner")
	baseOperation, ok := base.EffectOperationID(owner)
	if !ok {
		t.Fatal("effect operation identity unavailable")
	}
	baseDescriptor, ok := base.EffectDescriptorID(owner, 0)
	if !ok {
		t.Fatal("effect descriptor unavailable")
	}

	mutated := baseEffects
	mutated.Occurrences = append(mutated.Occurrences, EffectSpec{Target: 2, ValueArgs: []ValueFormal{1, 0}, RowArgs: []RowVar{0}})
	withExtra, extraOwner, _ := firstEffectIdentityContract(t, effectIdentitySpec(effectIdentityOperation("effect-owner", mutated)), "effect-owner")
	if got, ok := withExtra.EffectOperationID(extraOwner); !ok || got != baseOperation {
		t.Fatal("unrelated owner effect changed EffectOperationID")
	}
	if got, ok := withExtra.EffectDescriptorID(extraOwner, 0); !ok || got != baseDescriptor {
		t.Fatal("unrelated owner effect changed existing descriptor")
	}

	outcomeMutation := effectIdentityOperation("effect-owner", baseEffects)
	outcomeMutation.Outcomes = []OutcomeSpec{{Kind: flowkind.OutcomeNormal, Values: ValuesSpec{Fixed: []typ.Type{typ.Integer}, Tail: ValuesClosed}}}
	withOutcome, outcomeOwner, _ := firstEffectIdentityContract(t, effectIdentitySpec(outcomeMutation), "effect-owner")
	if got, ok := withOutcome.EffectOperationID(outcomeOwner); !ok || got != baseOperation {
		t.Fatal("unrelated owner outcome changed EffectOperationID")
	}
	if got, ok := withOutcome.EffectDescriptorID(outcomeOwner, 0); !ok || got != baseDescriptor {
		t.Fatal("unrelated owner outcome changed descriptor")
	}

	abiMutation := effectIdentityOperation("effect-owner", baseEffects)
	abiMutation.Input.Fixed[0] = typ.Integer
	withABI, abiOwner, _ := firstEffectIdentityContract(t, effectIdentitySpec(abiMutation), "effect-owner")
	if got, ok := withABI.EffectOperationID(abiOwner); !ok || got == baseOperation {
		t.Fatal("input ABI mutation did not change EffectOperationID")
	}
	if got, ok := withABI.EffectDescriptorID(abiOwner, 0); !ok || got == baseDescriptor {
		t.Fatal("input ABI mutation did not change descriptor")
	}

	bindingMutation := effectIdentityOperation("effect-owner-renamed", baseEffects)
	withBinding, bindingOwner, _ := firstEffectIdentityContract(t, effectIdentitySpec(bindingMutation), "effect-owner-renamed")
	if got, ok := withBinding.EffectOperationID(bindingOwner); !ok || got == baseOperation {
		t.Fatal("binding mutation did not change EffectOperationID")
	}
}

func TestEffectIdentitySeparatesOrdinaryAndCallbackOccurrences(t *testing.T) {
	effects := RowSpec{Occurrences: []EffectSpec{{Target: 1, ValueArgs: []ValueFormal{0, 1}, RowArgs: []RowVar{0}}}, Tail: RowClosed}
	contract, owner, callback := firstEffectIdentityContract(t, effectIdentitySpec(effectIdentityCallbackOperation("effect-callback", effects, effects)), "effect-callback")
	if callback == 0 {
		t.Fatal("callback absent")
	}
	ordinaryDescriptor, ok := contract.EffectDescriptorID(owner, 0)
	if !ok {
		t.Fatal("ordinary descriptor unavailable")
	}
	callbackDescriptor, ok := contract.CallbackEffectDescriptorID(callback, 0)
	if !ok || callbackDescriptor != ordinaryDescriptor {
		t.Fatal("equivalent ordinary and callback effects did not share descriptor")
	}
	ordinaryOccurrence, ok := contract.EffectOccurrenceID(owner, 0)
	if !ok {
		t.Fatal("ordinary occurrence unavailable")
	}
	callbackOccurrence, ok := contract.CallbackEffectOccurrenceID(callback, 0)
	if !ok || callbackOccurrence == ordinaryOccurrence {
		t.Fatal("ordinary and callback occurrences were conflated")
	}
	if ordinaryFamily, ok := contract.EffectRowFamilyID(owner); !ok {
		t.Fatal("ordinary family unavailable")
	} else if callbackFamily, ok := contract.CallbackEffectRowFamilyID(callback); !ok || callbackFamily == ordinaryFamily {
		t.Fatal("ordinary and callback families were conflated")
	}
}

func TestEffectIdentityCallbackTailChangesFamilyOnly(t *testing.T) {
	closed := RowSpec{Occurrences: []EffectSpec{{Target: 1, ValueArgs: []ValueFormal{0, 1}, RowArgs: []RowVar{0}}}, Tail: RowClosed}
	variable := RowSpec{Occurrences: []EffectSpec{{Target: 1, ValueArgs: []ValueFormal{0, 1}, RowArgs: []RowVar{0}}}, Tail: RowVariable, Var: 0}
	base, _, baseCallback := firstEffectIdentityContract(t, effectIdentitySpec(effectIdentityCallbackOperation("effect-callback", closed, RowSpec{Tail: RowClosed})), "effect-callback")
	changed, _, changedCallback := firstEffectIdentityContract(t, effectIdentitySpec(effectIdentityCallbackOperation("effect-callback", variable, RowSpec{Tail: RowClosed})), "effect-callback")
	baseDescriptor, ok := base.CallbackEffectDescriptorID(baseCallback, 0)
	if !ok {
		t.Fatal("base callback descriptor unavailable")
	}
	changedDescriptor, ok := changed.CallbackEffectDescriptorID(changedCallback, 0)
	if !ok || changedDescriptor != baseDescriptor {
		t.Fatal("callback row tail changed descriptor")
	}
	baseOccurrence, ok := base.CallbackEffectOccurrenceID(baseCallback, 0)
	if !ok {
		t.Fatal("base callback occurrence unavailable")
	}
	changedOccurrence, ok := changed.CallbackEffectOccurrenceID(changedCallback, 0)
	if !ok || changedOccurrence != baseOccurrence {
		t.Fatal("callback row tail changed occurrence")
	}
	baseFamily, ok := base.CallbackEffectRowFamilyID(baseCallback)
	if !ok {
		t.Fatal("base callback family unavailable")
	}
	changedFamily, ok := changed.CallbackEffectRowFamilyID(changedCallback)
	if !ok || changedFamily == baseFamily {
		t.Fatal("callback row tail did not change family")
	}
}

func TestEffectIdentityQueriesAllocateNothingAndReplay(t *testing.T) {
	effects := RowSpec{Occurrences: []EffectSpec{{Target: 2, ValueArgs: []ValueFormal{0, 1}, RowArgs: []RowVar{0}}}, Tail: RowClosed}
	left, owner, _ := firstEffectIdentityContract(t, effectIdentitySpec(effectIdentityOperation("effect-owner", effects)), "effect-owner")
	right, rightOwner, _ := firstEffectIdentityContract(t, effectIdentitySpec(effectIdentityOperation("effect-owner", effects)), "effect-owner")
	leftOperation, ok := left.EffectOperationID(owner)
	if !ok {
		t.Fatal("left operation identity unavailable")
	}
	if rightOperation, ok := right.EffectOperationID(rightOwner); !ok || rightOperation != leftOperation {
		t.Fatal("equivalent reseal changed operation identity")
	}
	if allocs := testing.AllocsPerRun(1000, func() {
		_, _ = left.EffectOperationID(owner)
		_, _ = left.EffectDescriptorID(owner, 0)
		_, _ = left.EffectOccurrenceID(owner, 0)
		_, _ = left.EffectRowFamilyID(owner)
	}); allocs != 0 {
		t.Fatalf("effect identity queries allocated %f times", allocs)
	}
}

func TestEffectIdentityEmptyOrdinaryAndOpaqueFamiliesAreAvailable(t *testing.T) {
	spec := effectIdentitySpec(effectIdentityOperation("effect-empty", RowSpec{Tail: RowClosed}))
	left, owner, _ := firstEffectIdentityContract(t, spec, "effect-empty")
	leftFamily, ok := left.EffectRowFamilyID(owner)
	if !ok || !leftFamily.Available() {
		t.Fatal("empty ordinary effect family unavailable")
	}
	opaque, ok := left.Opaque()
	if !ok {
		t.Fatal("opaque operation unavailable")
	}
	opaqueFamily, ok := left.EffectRowFamilyID(opaque)
	if !ok || !opaqueFamily.Available() {
		t.Fatal("opaque effect family unavailable")
	}
	right, rightOwner, _ := firstEffectIdentityContract(t, effectIdentitySpec(effectIdentityOperation("effect-empty", RowSpec{Tail: RowClosed})), "effect-empty")
	if got, ok := right.EffectRowFamilyID(rightOwner); !ok || got != leftFamily {
		t.Fatal("empty ordinary family changed across reseal")
	}
	rightOpaque, ok := right.Opaque()
	if !ok {
		t.Fatal("opaque operation unavailable after reseal")
	}
	if got, ok := right.EffectRowFamilyID(rightOpaque); !ok || got != opaqueFamily {
		t.Fatal("opaque family changed across reseal")
	}
}

func TestEffectIdentityCallbackEmptyFamilyIgnoresOwnerABI(t *testing.T) {
	empty := RowSpec{Tail: RowClosed}
	baseSpec := effectIdentitySpec(effectIdentityCallbackOperation("effect-callback-empty", empty, empty))
	base, _, baseCallback := firstEffectIdentityContract(t, baseSpec, "effect-callback-empty")
	baseFamily, ok := base.CallbackEffectRowFamilyID(baseCallback)
	if !ok || !baseFamily.Available() {
		t.Fatal("empty callback effect family unavailable")
	}
	changedOperation := effectIdentityCallbackOperation("effect-callback-empty", empty, empty)
	changedOperation.Input.Fixed[0] = typ.Integer
	changed, _, changedCallback := firstEffectIdentityContract(t, effectIdentitySpec(changedOperation), "effect-callback-empty")
	if got, ok := changed.CallbackEffectRowFamilyID(changedCallback); !ok || got != baseFamily {
		t.Fatal("empty callback family churned with owner ABI")
	}
}

func TestEffectIdentityTracksFormalAndOpenABI(t *testing.T) {
	formalSpec := func(constraint typ.Type) Spec {
		formal := typ.NewTypeParam("T", constraint)
		return Spec{Operations: []OperationSpec{{
			Bindings:    []BindingSpec{{Namespace: BindingBuiltin, Member: []string{"effect-formal-abi"}}},
			TypeFormals: []*typ.TypeParam{formal},
			Input:       ValuesSpec{Fixed: []typ.Type{formal}, Tail: ValuesClosed},
			Outcomes:    []OutcomeSpec{{Kind: flowkind.OutcomeNormal, Values: ValuesSpec{Tail: ValuesClosed}}},
			Effects:     RowSpec{Tail: RowClosed},
		}}}
	}
	formalID := func(spec Spec) (keyspace.ContentID, bool) {
		contract := mustSeal(t, spec)
		op, ok := contract.Lookup(BindingSpec{Namespace: BindingBuiltin, Member: []string{"effect-formal-abi"}})
		if !ok {
			t.Fatal("formal ABI operation absent")
		}
		return contract.EffectOperationID(op)
	}
	formalNumber, ok := formalID(formalSpec(typ.Number))
	if !ok {
		t.Fatal("number formal ABI identity unavailable")
	}
	formalString, ok := formalID(formalSpec(typ.String))
	if !ok || formalString == formalNumber {
		t.Fatal("type formal constraint mutation did not change EffectOperationID")
	}

	valuesSpec := func(tailType typ.Type) Spec {
		return Spec{Operations: []OperationSpec{{
			Bindings:   []BindingSpec{{Namespace: BindingBuiltin, Member: []string{"effect-values-abi"}}},
			ValuesVars: 1,
			Input:      ValuesSpec{Fixed: []typ.Type{typ.Any}, Tail: ValuesVariable, Var: 0, TailType: tailType},
			Outcomes:   []OutcomeSpec{{Kind: flowkind.OutcomeNormal, Values: ValuesSpec{Tail: ValuesClosed}}},
			Effects:    RowSpec{Tail: RowClosed},
		}}}
	}
	valuesID := func(spec Spec) (keyspace.ContentID, bool) {
		contract := mustSeal(t, spec)
		op, ok := contract.Lookup(BindingSpec{Namespace: BindingBuiltin, Member: []string{"effect-values-abi"}})
		if !ok {
			t.Fatal("Values ABI operation absent")
		}
		return contract.EffectOperationID(op)
	}
	valuesString, ok := valuesID(valuesSpec(typ.String))
	if !ok {
		t.Fatal("string Values ABI identity unavailable")
	}
	valuesInteger, ok := valuesID(valuesSpec(typ.Integer))
	if !ok || valuesInteger == valuesString {
		t.Fatal("ValuesVar ABI mutation did not change EffectOperationID")
	}

	rowSpec := func(count uint32) Spec {
		return Spec{Operations: []OperationSpec{{
			Bindings:   []BindingSpec{{Namespace: BindingBuiltin, Member: []string{"effect-row-abi"}}},
			RowFormals: count,
			Input:      ValuesSpec{Tail: ValuesClosed},
			Outcomes:   []OutcomeSpec{{Kind: flowkind.OutcomeNormal, Values: ValuesSpec{Tail: ValuesClosed}}},
			Effects:    RowSpec{Tail: RowClosed},
		}}}
	}
	rowOne := mustSeal(t, rowSpec(1))
	rowTwo := mustSeal(t, rowSpec(2))
	rowOneOp, ok := rowOne.Lookup(BindingSpec{Namespace: BindingBuiltin, Member: []string{"effect-row-abi"}})
	if !ok {
		t.Fatal("one-row-formal operation absent")
	}
	rowTwoOp, ok := rowTwo.Lookup(BindingSpec{Namespace: BindingBuiltin, Member: []string{"effect-row-abi"}})
	if !ok {
		t.Fatal("two-row-formal operation absent")
	}
	rowOneID, ok := rowOne.EffectOperationID(rowOneOp)
	if !ok {
		t.Fatal("one-row-formal identity unavailable")
	}
	rowTwoID, ok := rowTwo.EffectOperationID(rowTwoOp)
	if !ok || rowTwoID == rowOneID {
		t.Fatal("row formal count mutation did not change EffectOperationID")
	}
}
