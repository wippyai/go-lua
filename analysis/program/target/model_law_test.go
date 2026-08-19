package target

import (
	flowkind "github.com/wippyai/go-lua/analysis/program/flow/kind"
	"github.com/wippyai/go-lua/analysis/program/target/vocabulary"
	"github.com/wippyai/go-lua/analysis/schema"
	schematype "github.com/wippyai/go-lua/analysis/schema/typecontract"
	"testing"
)

func TestBehaviorDescriptorProjectsOpaqueResultAndPredicateRows(t *testing.T) {
	resultRelation := schema.NewEntryID(schema.SurfaceKindStructure, "behavior/runtime-kind/result")
	predicateRelation := schema.NewEntryID(schema.SurfaceKindStructure, "behavior/runtime-kind/predicate")
	spec := vocabulary.OperationSpec{
		Bindings: []vocabulary.BindingSpec{{Namespace: vocabulary.BindingBuiltin, Member: []string{"behavior"}}},
		Input:    vocabulary.ValuesSpec{Fixed: []schematype.Type{testAny}, Tail: vocabulary.ValuesClosed},
		Outcomes: []vocabulary.OutcomeSpec{
			{Kind: flowkind.OutcomeNormal, Values: vocabulary.ValuesSpec{Fixed: []schematype.Type{testString}, Tail: vocabulary.ValuesClosed}},
			{Kind: flowkind.OutcomeThrow, Values: vocabulary.ValuesSpec{Fixed: []schematype.Type{testAny}, Tail: vocabulary.ValuesClosed}},
		},
		Behavior: &vocabulary.OperationBehaviorSpec{
			Results: []vocabulary.OperationResultSpec{{
				Outcome: 0, Result: 0,
				Source:   vocabulary.InputSource{Kind: vocabulary.InputSourceValueFormal, Ordinal: 0},
				Relation: resultRelation,
			}},
			Predicates: []vocabulary.OperationPredicateSpec{{
				Outcome: 0, Result: 0,
				Subject:  vocabulary.InputSource{Kind: vocabulary.InputSourceValueFormal, Ordinal: 0},
				Relation: predicateRelation,
			}},
		},
		Effects: vocabulary.RowSpec{Tail: vocabulary.RowClosed},
	}
	contract := mustSeal(t, Spec{Operations: []vocabulary.OperationSpec{spec}})
	op, ok := contract.Lookup(vocabulary.BindingSpec{Namespace: vocabulary.BindingBuiltin, Member: []string{"behavior"}})
	if !ok {
		t.Fatal("behavior operation missing")
	}
	if contract.BehaviorResultCount(op) != 1 || contract.BehaviorPredicateCount(op) != 1 {
		t.Fatalf("behavior counts = results:%d predicates:%d", contract.BehaviorResultCount(op), contract.BehaviorPredicateCount(op))
	}
	outcome, result, source, relation, ok := contract.BehaviorResultAt(op, 0)
	if !ok || outcome != 0 || result != 0 || source.Kind != vocabulary.InputSourceValueFormal || source.Ordinal != 0 || relation != resultRelation {
		t.Fatalf("behavior result = outcome:%d result:%d source:%#v relation:%v ok:%v", outcome, result, source, relation, ok)
	}
	predicateOutcome, predicateResult, subject, relation, ok := contract.BehaviorPredicateAt(op, 0)
	if !ok || predicateOutcome != 0 || predicateResult != 0 || subject.Kind != vocabulary.InputSourceValueFormal || subject.Ordinal != 0 || relation != predicateRelation {
		t.Fatalf("behavior predicate = outcome:%d result:%d subject:%#v relation:%v ok:%v", predicateOutcome, predicateResult, subject, relation, ok)
	}

	without := spec
	without.Behavior = nil
	withoutContract := mustSeal(t, Spec{Operations: []vocabulary.OperationSpec{without}})
	if contract.ContentID() == withoutContract.ContentID() {
		t.Fatal("behavior descriptor did not participate in contract identity")
	}
}

func TestBehaviorDescriptorRejectsUnidentifiedRelation(t *testing.T) {
	spec := vocabulary.OperationSpec{
		Bindings: []vocabulary.BindingSpec{{Namespace: vocabulary.BindingBuiltin, Member: []string{"behavior-invalid"}}},
		Input:    vocabulary.ValuesSpec{Fixed: []schematype.Type{testAny}, Tail: vocabulary.ValuesClosed},
		Outcomes: []vocabulary.OutcomeSpec{{Kind: flowkind.OutcomeNormal, Values: vocabulary.ValuesSpec{Fixed: []schematype.Type{testString}, Tail: vocabulary.ValuesClosed}}},
		Behavior: &vocabulary.OperationBehaviorSpec{Results: []vocabulary.OperationResultSpec{{
			Outcome: 0, Result: 0,
			Source: vocabulary.InputSource{Kind: vocabulary.InputSourceValueFormal, Ordinal: 0},
		}}},
		Effects: vocabulary.RowSpec{Tail: vocabulary.RowClosed},
	}
	if _, err := testSeal(&Spec{Operations: []vocabulary.OperationSpec{spec}}); err == nil {
		t.Fatal("behavior row without a relation identity was accepted")
	}
}

func TestModelHandlesRemainDenseAndZeroInvalid(t *testing.T) {
	contract := mustSeal(t, Spec{Operations: []vocabulary.OperationSpec{builtin("model", testString, vocabulary.RowSpec{Tail: vocabulary.RowClosed})}})
	if _, ok := contract.OperationAt(0); !ok {
		t.Fatal("sealed operation did not receive a model handle")
	}
	if _, ok := contract.Input(vocabulary.Operation(1)); !ok {
		t.Fatal("sealed operation did not receive an input Values handle")
	}
	if _, ok := contract.Input(0); ok {
		t.Fatal("zero Operation handle resolved")
	}
	if _, ok := contract.TypeDeclaration(0); ok {
		t.Fatal("zero Type handle resolved")
	}
}

func TestModelActivationCallbackReferencesResolveToOwnedIDs(t *testing.T) {
	contract := mustSeal(t, Spec{Operations: []vocabulary.OperationSpec{callbackOwnerOperation("activation-model")}})
	op, ok := contract.Lookup(vocabulary.BindingSpec{Namespace: vocabulary.BindingBuiltin, Member: []string{"activation-model"}})
	if !ok {
		t.Fatal("activation owner operation missing")
	}
	callback, ok := contract.CallbackAt(op, 0)
	if !ok || callback == 0 {
		t.Fatalf("CallbackAt = %d/%v", callback, ok)
	}
	if got, ok := contract.CallbackOwner(callback); !ok || got != op {
		t.Fatalf("callback owner = %d/%v, want %d/true", got, ok, op)
	}
	if _, ok := contract.CallbackAt(op, -1); ok {
		t.Fatal("negative callback coordinate resolved")
	}
}

func TestModelRowsKeepOperationOwnedRangesCorrelated(t *testing.T) {
	contract := mustSeal(t, Spec{Operations: []vocabulary.OperationSpec{
		builtin("row-a", testString, vocabulary.RowSpec{Tail: vocabulary.RowClosed}),
		builtin("row-b", testBoolean, vocabulary.RowSpec{Tail: vocabulary.RowClosed}),
	}})
	for _, name := range []string{"row-a", "row-b"} {
		op, ok := contract.Lookup(vocabulary.BindingSpec{Namespace: vocabulary.BindingBuiltin, Member: []string{name}})
		if !ok || contract.OutcomeCount(op) != 1 || contract.EffectCount(op) != 0 {
			t.Fatalf("%s row shape = op:%d/%v outcomes:%d effects:%d", name, op, ok, contract.OutcomeCount(op), contract.EffectCount(op))
		}
	}
}

func TestModelValuesPublicationDescriptorGettersRetainTypedAuthority(t *testing.T) {
	contract, owner := publicationEffectContract(t, sendPublication(vocabulary.PublicationMutabilityCopyOnWrite), false)
	descriptor, ok := contract.PublicationEffectDescriptor(owner, 0)
	if !ok {
		t.Fatal("publication descriptor unavailable")
	}
	if descriptor.Kind() != vocabulary.PublicationEffectSendTransfer || descriptor.Subject() != 0 || descriptor.Context() != 1 {
		t.Fatalf("publication descriptor identity = kind:%d subject:%d context:%d", descriptor.Kind(), descriptor.Subject(), descriptor.Context())
	}
	if descriptor.Escape() != vocabulary.PublicationEscapeSendTransfer || descriptor.Mutability() != vocabulary.PublicationMutabilityCopyOnWrite || descriptor.Lifetime() != vocabulary.PublicationLifetimePreserve {
		t.Fatal("publication descriptor consequences changed")
	}
}

func sendPublication(mutability vocabulary.PublicationMutabilityDisposition) *vocabulary.PublicationEffectSpec {
	return &vocabulary.PublicationEffectSpec{
		Kind:        vocabulary.PublicationEffectSendTransfer,
		Subject:     0,
		Destination: vocabulary.PublicationDestinationValueFormal,
		Context:     1,
		Escape:      vocabulary.PublicationEscapeSendTransfer,
		Mutability:  mutability,
		Lifetime:    vocabulary.PublicationLifetimePreserve,
	}
}

func publicationEffectSpec(publication *vocabulary.PublicationEffectSpec, duplicate bool) Spec {
	occurrences := []vocabulary.EffectSpec{{Target: 2, ValueArgs: []vocabulary.ValueFormal{0, 1}, RowArgs: []vocabulary.RowVar{0}, Publication: publication}}
	if duplicate {
		occurrences = append(occurrences, occurrences[0])
	}
	return effectIdentitySpec(effectIdentityOperation("publication-owner", vocabulary.RowSpec{
		Occurrences: occurrences,
		Tail:        vocabulary.RowClosed,
	}))
}

func publicationEffectContract(t *testing.T, publication *vocabulary.PublicationEffectSpec, duplicate bool) (*Contract, vocabulary.Operation) {
	t.Helper()
	contract, owner, _ := firstEffectIdentityContract(t, publicationEffectSpec(publication, duplicate), "publication-owner")
	return contract, owner
}

func distinctPublicationEffectSpec(reverse bool) Spec {
	occurrences := []vocabulary.EffectSpec{
		{Target: 2, ValueArgs: []vocabulary.ValueFormal{0, 1}, RowArgs: []vocabulary.RowVar{0}, Publication: publicationFor(vocabulary.PublicationEffectReturnEscape)},
		{Target: 2, ValueArgs: []vocabulary.ValueFormal{0, 1}, RowArgs: []vocabulary.RowVar{0}, Publication: publicationFor(vocabulary.PublicationEffectFreezeSeal)},
	}
	if reverse {
		occurrences[0], occurrences[1] = occurrences[1], occurrences[0]
	}
	return effectIdentitySpec(effectIdentityOperation("publication-distinct", vocabulary.RowSpec{Occurrences: occurrences, Tail: vocabulary.RowClosed}))
}

func publicationFor(kind vocabulary.PublicationEffectKind) *vocabulary.PublicationEffectSpec {
	spec := &vocabulary.PublicationEffectSpec{
		Kind: kind, Subject: 0, Destination: vocabulary.PublicationDestinationNone,
		Escape: vocabulary.PublicationEscapeNone, Mutability: vocabulary.PublicationMutabilityPreserve, Lifetime: vocabulary.PublicationLifetimePreserve,
	}
	switch kind {
	case vocabulary.PublicationEffectSendTransfer:
		spec.Destination, spec.Context = vocabulary.PublicationDestinationValueFormal, 1
		spec.Escape = vocabulary.PublicationEscapeSendTransfer
	case vocabulary.PublicationEffectReturnEscape:
		spec.Escape = vocabulary.PublicationEscapeReturn
	case vocabulary.PublicationEffectCallbackEscape:
		spec.Escape = vocabulary.PublicationEscapeCallback
	case vocabulary.PublicationEffectFreezeSeal:
		spec.Mutability = vocabulary.PublicationMutabilitySeal
	case vocabulary.PublicationEffectWriteMutation:
		spec.Mutability = vocabulary.PublicationMutabilityWrite
	case vocabulary.PublicationEffectCloseRelease:
		spec.Lifetime = vocabulary.PublicationLifetimeRelease
	}
	return spec
}

func publicationSealError(publication *vocabulary.PublicationEffectSpec) error {
	spec := publicationEffectSpec(publication, false)
	_, err := testSeal(&spec)
	return err
}

func TestPublicationEffectDescriptorOwnerLaw(t *testing.T) {
	for _, kind := range []vocabulary.PublicationEffectKind{
		vocabulary.PublicationEffectSendTransfer,
		vocabulary.PublicationEffectReturnEscape,
		vocabulary.PublicationEffectCallbackEscape,
		vocabulary.PublicationEffectFreezeSeal,
		vocabulary.PublicationEffectWriteMutation,
		vocabulary.PublicationEffectCloseRelease,
	} {
		authored := publicationFor(kind)
		sealed, sealedOwner := publicationEffectContract(t, authored, false)
		descriptor, found := sealed.PublicationEffectDescriptor(sealedOwner, 0)
		if !found || descriptor.Kind() != kind || descriptor.Subject() != authored.Subject ||
			descriptor.DestinationRole() != authored.Destination || descriptor.Context() != authored.Context ||
			descriptor.Escape() != authored.Escape || descriptor.Mutability() != authored.Mutability || descriptor.Lifetime() != authored.Lifetime {
			t.Fatalf("kind %d did not retain its exact typed publication semantics", kind)
		}

		mismatched := *authored
		if mismatched.Escape == vocabulary.PublicationEscapeNone {
			mismatched.Escape = vocabulary.PublicationEscapeReturn
		} else {
			mismatched.Escape = vocabulary.PublicationEscapeNone
		}
		if err := publicationSealError(&mismatched); err == nil {
			t.Fatalf("kind %d sealed with mismatched typed consequences", kind)
		}
	}

	contract, owner := publicationEffectContract(t, sendPublication(vocabulary.PublicationMutabilityCopyOnWrite), true)
	descriptor, ok := contract.PublicationEffectDescriptor(owner, 0)
	if !ok {
		t.Fatal("explicit publication descriptor unavailable")
	}
	if descriptor.Kind() != vocabulary.PublicationEffectSendTransfer || descriptor.Subject() != 0 ||
		descriptor.DestinationRole() != vocabulary.PublicationDestinationValueFormal || descriptor.Context() != 1 ||
		descriptor.Escape() != vocabulary.PublicationEscapeSendTransfer || descriptor.Mutability() != vocabulary.PublicationMutabilityCopyOnWrite ||
		descriptor.Lifetime() != vocabulary.PublicationLifetimePreserve {
		t.Fatal("publication descriptor projection changed its exact authored semantics")
	}
	firstDescriptor, firstDescriptorOK := contract.PublicationEffectDescriptorID(owner, 0)
	secondDescriptor, secondDescriptorOK := contract.PublicationEffectDescriptorID(owner, 1)
	firstOccurrence, firstOccurrenceOK := contract.PublicationEffectOccurrenceID(owner, 0)
	secondOccurrence, secondOccurrenceOK := contract.PublicationEffectOccurrenceID(owner, 1)
	if !firstDescriptorOK || !secondDescriptorOK || firstDescriptor != secondDescriptor {
		t.Fatal("duplicate publication effects did not canonicalize to one descriptor identity")
	}
	if !firstOccurrenceOK || !secondOccurrenceOK || firstOccurrence == secondOccurrence {
		t.Fatal("duplicate publication effects lost their distinct occurrence identities")
	}
	if _, ok := contract.PublicationEffectDescriptor(owner, 2); ok {
		t.Fatal("out-of-range effect position accepted a spliced publication descriptor")
	}
	tampered := *contract
	tampered.effects = append([]effectRow(nil), contract.effects...)
	ownerRow, ownerRowOK := tampered.operation(owner)
	if !ownerRowOK || ownerRow.effects.len() == 0 {
		t.Fatal("publication fixture has no mutable sealed effect row")
	}
	tampered.effects[ownerRow.effects.start].publication.subject = 2
	if _, ok := tampered.PublicationEffectDescriptor(owner, 0); ok {
		t.Fatal("publication query accepted a stale out-of-ABI sealed row")
	}
	if id := tampered.ContentID(); id.Available() {
		t.Fatal("stale out-of-ABI publication row retained a whole-contract identity")
	}
	if _, err := tampered.effectDescriptorID(owner, tampered.effects[ownerRow.effects.start]); err == nil {
		t.Fatal("stale out-of-ABI publication row retained an effect descriptor identity")
	}
	without, withoutOwner := publicationEffectContract(t, nil, false)
	if _, ok := without.PublicationEffectDescriptor(withoutOwner, 0); ok {
		t.Fatal("generic effect inferred publication semantics")
	}
	if _, ok := without.PublicationEffectDescriptorID(withoutOwner, 0); ok {
		t.Fatal("generic effect exposed a publication descriptor identity")
	}

	changed, changedOwner := publicationEffectContract(t, sendPublication(vocabulary.PublicationMutabilityPreserve), false)
	changedDescriptor, changedOK := changed.PublicationEffectDescriptorID(changedOwner, 0)
	if !changedOK || changedDescriptor == firstDescriptor {
		t.Fatal("publication consequence did not participate in canonical descriptor identity")
	}
	if foreign, ok := changed.PublicationEffectDescriptorID(owner, 0); !ok || foreign != changedDescriptor || foreign == firstDescriptor {
		t.Fatal("receiver-local query did not select its own sealed descriptor row")
	}

	firstDistinct, firstDistinctOwner, _ := firstEffectIdentityContract(t, distinctPublicationEffectSpec(false), "publication-distinct")
	secondDistinct, secondDistinctOwner, _ := firstEffectIdentityContract(t, distinctPublicationEffectSpec(true), "publication-distinct")
	if firstDistinct.ContentID() != secondDistinct.ContentID() {
		t.Fatal("permuted distinct publication effects changed Contract identity")
	}
	for index := 0; index < firstDistinct.EffectCount(firstDistinctOwner); index++ {
		firstDescriptorID, firstDescriptorOK := firstDistinct.PublicationEffectDescriptorID(firstDistinctOwner, index)
		secondDescriptorID, secondDescriptorOK := secondDistinct.PublicationEffectDescriptorID(secondDistinctOwner, index)
		firstOccurrenceID, firstOccurrenceOK := firstDistinct.PublicationEffectOccurrenceID(firstDistinctOwner, index)
		secondOccurrenceID, secondOccurrenceOK := secondDistinct.PublicationEffectOccurrenceID(secondDistinctOwner, index)
		if !firstDescriptorOK || !secondDescriptorOK || firstDescriptorID != secondDescriptorID ||
			!firstOccurrenceOK || !secondOccurrenceOK || firstOccurrenceID != secondOccurrenceID {
			t.Fatalf("permuted distinct publication effect %d changed a sealed identity", index)
		}
	}

	badSubject := sendPublication(vocabulary.PublicationMutabilityPreserve)
	badSubject.Subject = 2
	if err := publicationSealError(badSubject); err == nil {
		t.Fatal("out-of-ABI publication subject sealed")
	}
	badDestination := sendPublication(vocabulary.PublicationMutabilityPreserve)
	badDestination.Context = 2
	if err := publicationSealError(badDestination); err == nil {
		t.Fatal("out-of-ABI publication destination sealed")
	}
	badContext := publicationFor(vocabulary.PublicationEffectReturnEscape)
	badContext.Context = 1
	if err := publicationSealError(badContext); err == nil {
		t.Fatal("destination-free publication carried a context formal")
	}

	callbackPublication := publicationFor(vocabulary.PublicationEffectCallbackEscape)
	callbackContract, callbackOwner, callback := firstEffectIdentityContract(t, effectIdentitySpec(effectIdentityCallbackOperation("publication-callback", vocabulary.RowSpec{
		Occurrences: []vocabulary.EffectSpec{{Target: 2, ValueArgs: []vocabulary.ValueFormal{0, 1}, RowArgs: []vocabulary.RowVar{0}, Publication: callbackPublication}},
		Tail:        vocabulary.RowClosed,
	}, vocabulary.RowSpec{Tail: vocabulary.RowClosed})), "publication-callback")
	callbackDescriptor, callbackOK := callbackContract.CallbackPublicationEffectDescriptor(callback, 0)
	callbackID, callbackIDOK := callbackContract.CallbackPublicationEffectOccurrenceID(callback, 0)
	if !callbackOK || !callbackIDOK || !callbackID.Available() || callbackDescriptor.Kind() != vocabulary.PublicationEffectCallbackEscape ||
		callbackOwner == 0 {
		t.Fatal("callback publication descriptor is unavailable")
	}
}

func exactProjectionOperation(name string, source, result vocabulary.ValuesSpec) vocabulary.OperationSpec {
	empty := vocabulary.ValuesSpec{Tail: vocabulary.ValuesClosed}
	valuesVars := uint32(0)
	if source.Tail == vocabulary.ValuesVariable {
		valuesVars = uint32(source.Var) + 1
	}
	return vocabulary.OperationSpec{
		Bindings:   []vocabulary.BindingSpec{{Namespace: vocabulary.BindingBuiltin, Member: []string{name}}},
		ValuesVars: valuesVars,
		Input:      empty,
		Outcomes:   []vocabulary.OutcomeSpec{{Kind: flowkind.OutcomeNormal, Values: empty}},
		Subedges: []vocabulary.SubedgeSpec{{
			Role:      1,
			Family:    vocabulary.SubedgeFamilyLength,
			Admission: schematype.CallableAdmissionOrdinary,
			Arguments: vocabulary.ValuesSpec{Fixed: []schematype.Type{testAny}, Tail: vocabulary.ValuesClosed},
			ArgumentOrigins: []vocabulary.ArgumentOrigin{{
				Segment: vocabulary.ArgumentFixed, Kind: vocabulary.ArgumentSourceRule,
			}},
			Outcomes: []vocabulary.TerminalSpec{
				{Kind: flowkind.OutcomeNormal, Values: source},
				{Kind: flowkind.OutcomeReturn, Values: empty},
				{Kind: flowkind.OutcomeThrow, Values: empty},
				{Kind: flowkind.OutcomeYield, Values: empty},
				{Kind: flowkind.OutcomeCancel, Values: empty},
			},
			AdmissionFailure: vocabulary.AdmissionFailureSpec{
				Values: empty,
				Route:  vocabulary.AdmissionRouteSpec{Route: vocabulary.RouteOutcome, Adjustment: vocabulary.AdjustmentPreserve, Result: empty, Placement: vocabulary.PlacementFixed},
			},
			Routes: []vocabulary.SubedgeRouteSpec{
				{Kind: flowkind.OutcomeNormal, Route: vocabulary.RouteContinue, Adjustment: vocabulary.AdjustmentExact, Result: result},
				{Kind: flowkind.OutcomeReturn, Route: vocabulary.RouteContinue, Adjustment: vocabulary.AdjustmentExact, Result: empty},
				{Kind: flowkind.OutcomeThrow, Route: vocabulary.RouteContinue, Adjustment: vocabulary.AdjustmentExact, Result: empty},
				{Kind: flowkind.OutcomeYield, Route: vocabulary.RoutePropagateYield, Adjustment: vocabulary.AdjustmentPreserve, Result: empty},
				{Kind: flowkind.OutcomeCancel, Route: vocabulary.RouteContinue, Adjustment: vocabulary.AdjustmentExact, Result: empty},
			},
		}},
		Effects: vocabulary.RowSpec{Tail: vocabulary.RowClosed},
	}
}

func sealsExactProjection(t *testing.T, name string, source, result vocabulary.ValuesSpec) bool {
	t.Helper()
	_, err := testSeal(&Spec{Operations: []vocabulary.OperationSpec{exactProjectionOperation(name, source, result)}})
	return err == nil
}

func TestExactProjectionAccountsForNilAndEveryOpenSuffixPosition(t *testing.T) {
	empty := vocabulary.ValuesSpec{Tail: vocabulary.ValuesClosed}
	if sealsExactProjection(t, "exact-closed-missing", vocabulary.ValuesSpec{Fixed: []schematype.Type{testString}, Tail: vocabulary.ValuesClosed}, vocabulary.ValuesSpec{Fixed: []schematype.Type{testString, testString}, Tail: vocabulary.ValuesClosed}) {
		t.Fatal("closed Values Exact projection accepted missing string instead of nil")
	}
	if !sealsExactProjection(t, "exact-nil", empty, vocabulary.ValuesSpec{Fixed: []schematype.Type{testNil}, Tail: vocabulary.ValuesClosed}) {
		t.Fatal("closed empty Values did not project nil")
	}
	if sealsExactProjection(t, "exact-nil-reject", empty, vocabulary.ValuesSpec{Fixed: []schematype.Type{testString}, Tail: vocabulary.ValuesClosed}) {
		t.Fatal("closed empty Values projected nil into string")
	}

	optionalString := testUnion(testString, testNil)
	if sealsExactProjection(t, "exact-open-scalar-reject", vocabulary.ValuesSpec{Tail: vocabulary.ValuesVariable, Var: 0, TailType: testString}, vocabulary.ValuesSpec{Fixed: []schematype.Type{testString}, Tail: vocabulary.ValuesClosed}) {
		t.Fatal("open string tail Exact projection ignored its empty/nil case")
	}
	if !sealsExactProjection(t, "exact-open-scalar", vocabulary.ValuesSpec{Tail: vocabulary.ValuesVariable, Var: 0, TailType: testString}, vocabulary.ValuesSpec{Fixed: []schematype.Type{optionalString}, Tail: vocabulary.ValuesClosed}) {
		t.Fatal("open string tail Exact projection rejected string|nil")
	}

	stringInteger := testUnion(testString, testInteger)
	stringIntegerNil := testUnion(testString, testInteger, testNil)
	source := vocabulary.ValuesSpec{
		Fixed: []schematype.Type{testString}, Tail: vocabulary.ValuesVariable, Var: 0, TailType: testString,
		Suffix: []schematype.Type{testInteger},
	}
	if !sealsExactProjection(t, "exact-open-suffix", source, vocabulary.ValuesSpec{Fixed: []schematype.Type{testString, stringInteger, stringIntegerNil}, Tail: vocabulary.ValuesClosed}) {
		t.Fatal("P·alpha·S Exact projection rejected its complete positional coverage")
	}
	if sealsExactProjection(t, "exact-open-suffix-integer", source, vocabulary.ValuesSpec{Fixed: []schematype.Type{testString, testString, stringIntegerNil}, Tail: vocabulary.ValuesClosed}) {
		t.Fatal("P·alpha·S Exact projection ignored an early suffix position")
	}
	if sealsExactProjection(t, "exact-open-suffix-nil", source, vocabulary.ValuesSpec{Fixed: []schematype.Type{testString, stringInteger, stringInteger}, Tail: vocabulary.ValuesClosed}) {
		t.Fatal("P·alpha·S Exact projection ignored the tail-short nil case")
	}
}

func TestExactProjectionRejectsConcreteTerminalContradiction(t *testing.T) {
	if sealsExactProjection(t, "exact-string-number", vocabulary.ValuesSpec{Fixed: []schematype.Type{testString}, Tail: vocabulary.ValuesClosed}, vocabulary.ValuesSpec{Fixed: []schematype.Type{testNumber}, Tail: vocabulary.ValuesClosed}) {
		t.Fatal("Exact projection accepted String terminal as Number")
	}
}

func TestValuesSuffixCanonicalizationAndQueries(t *testing.T) {
	contract, err := testSeal(&Spec{Operations: []vocabulary.OperationSpec{{
		Bindings:   []vocabulary.BindingSpec{{Namespace: vocabulary.BindingBuiltin, Member: []string{"suffix"}}},
		ValuesVars: 1,
		Input:      vocabulary.ValuesSpec{Fixed: []schematype.Type{testString, testInteger}, Tail: vocabulary.ValuesClosed},
		Outcomes: []vocabulary.OutcomeSpec{
			{Kind: flowkind.OutcomeNormal, Values: vocabulary.ValuesSpec{Fixed: []schematype.Type{testString}, Tail: vocabulary.ValuesClosed, Suffix: []schematype.Type{testInteger}}},
			{Kind: flowkind.OutcomeThrow, Values: vocabulary.ValuesSpec{Fixed: []schematype.Type{testString}, Tail: vocabulary.ValuesVariable, Var: 0, Suffix: []schematype.Type{testInteger}}},
		},
		Effects: vocabulary.RowSpec{Tail: vocabulary.RowClosed},
	}}})
	if err != nil {
		t.Fatal(err)
	}
	op, ok := contract.Lookup(vocabulary.BindingSpec{Namespace: vocabulary.BindingBuiltin, Member: []string{"suffix"}})
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
	base := vocabulary.OperationSpec{
		Input:    vocabulary.ValuesSpec{Tail: vocabulary.ValuesClosed},
		Outcomes: []vocabulary.OutcomeSpec{{Kind: flowkind.OutcomeNormal, Values: vocabulary.ValuesSpec{Tail: vocabulary.ValuesClosed}}},
		Effects:  vocabulary.RowSpec{Tail: vocabulary.RowClosed},
	}
	for _, test := range []struct {
		name string
		edit func(*vocabulary.OperationSpec)
	}{
		{"input suffix", func(op *vocabulary.OperationSpec) { op.Input.Suffix = []schematype.Type{testString} }},
		{"nil suffix type", func(op *vocabulary.OperationSpec) { op.Outcomes[0].Values.Suffix = []schematype.Type{{}} }},
	} {
		t.Run(test.name, func(t *testing.T) {
			op := base
			op.Outcomes = append([]vocabulary.OutcomeSpec(nil), base.Outcomes...)
			test.edit(&op)
			if _, err := testSeal(&Spec{Operations: []vocabulary.OperationSpec{op}}); err == nil {
				t.Fatal("Seal accepted invalid suffix")
			}
		})
	}
}

func TestValuesSuffixOutcomePermutationHasOnePublicContract(t *testing.T) {
	makeContract := func(outcomes []vocabulary.OutcomeSpec) *Contract {
		contract, err := testSeal(&Spec{Operations: []vocabulary.OperationSpec{{
			Bindings:   []vocabulary.BindingSpec{{Namespace: vocabulary.BindingBuiltin, Member: []string{"suffix-permutation"}}},
			ValuesVars: 1,
			Input:      vocabulary.ValuesSpec{Fixed: []schematype.Type{testString}, Tail: vocabulary.ValuesClosed},
			Outcomes:   outcomes,
			Effects:    vocabulary.RowSpec{Tail: vocabulary.RowClosed},
		}}})
		if err != nil {
			t.Fatal(err)
		}
		return contract
	}
	normal := vocabulary.OutcomeSpec{Kind: flowkind.OutcomeNormal, Values: vocabulary.ValuesSpec{Fixed: []schematype.Type{testString}, Tail: vocabulary.ValuesVariable, Var: 0, Suffix: []schematype.Type{testInteger}}}
	throwing := vocabulary.OutcomeSpec{Kind: flowkind.OutcomeThrow, Values: vocabulary.ValuesSpec{Tail: vocabulary.ValuesClosed, Suffix: []schematype.Type{testString}}}
	left := makeContract([]vocabulary.OutcomeSpec{normal, throwing})
	right := makeContract([]vocabulary.OutcomeSpec{throwing, normal})
	assertPublicContractEqual(t, left, right)
}

func TestWideValuesSuffixSealsAndQueries(t *testing.T) {
	const width = 2048
	suffix := make([]schematype.Type, width)
	for index := range suffix {
		suffix[index] = testInteger
	}
	contract, err := testSeal(&Spec{Operations: []vocabulary.OperationSpec{{
		Bindings:   []vocabulary.BindingSpec{{Namespace: vocabulary.BindingBuiltin, Member: []string{"wide-suffix"}}},
		ValuesVars: 1,
		Input:      vocabulary.ValuesSpec{Tail: vocabulary.ValuesVariable, Var: 0},
		Outcomes:   []vocabulary.OutcomeSpec{{Kind: flowkind.OutcomeNormal, Values: vocabulary.ValuesSpec{Tail: vocabulary.ValuesVariable, Var: 0, Suffix: suffix}}},
		Effects:    vocabulary.RowSpec{Tail: vocabulary.RowClosed},
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

func TestValuesVarTailTypesAreTotalSharedAndAllocationFree(t *testing.T) {
	contract := mustSeal(t, Spec{Operations: []vocabulary.OperationSpec{{
		Bindings:   []vocabulary.BindingSpec{{Namespace: vocabulary.BindingBuiltin, Member: []string{"tail-class"}}},
		ValuesVars: 3,
		Input:      vocabulary.ValuesSpec{Tail: vocabulary.ValuesVariable, Var: 0, TailType: testString},
		Outcomes: []vocabulary.OutcomeSpec{
			{Kind: flowkind.OutcomeNormal, Values: vocabulary.ValuesSpec{Tail: vocabulary.ValuesVariable, Var: 0, TailType: testString}},
			{Kind: flowkind.OutcomeNormal, Values: vocabulary.ValuesSpec{Tail: vocabulary.ValuesVariable, Var: 1}},
			{Kind: flowkind.OutcomeThrow, Values: vocabulary.ValuesSpec{Tail: vocabulary.ValuesClosed}},
		},
		Effects: vocabulary.RowSpec{Tail: vocabulary.RowClosed},
	}}})
	op, ok := contract.Lookup(vocabulary.BindingSpec{Namespace: vocabulary.BindingBuiltin, Member: []string{"tail-class"}})
	if !ok {
		t.Fatal("tail-class operation missing")
	}
	input, _ := contract.Input(op)
	if got, found := contract.ValuesTailType(input); !found || !sameFrozenType(t, contract, got, testString) {
		t.Fatalf("input tail type = %d/%v, want string", got, found)
	}
	for _, variable := range []vocabulary.ValuesVar{1, 2} {
		got, found := contract.ValuesVarType(op, variable)
		if !found || !sameFrozenType(t, contract, got, testAny) {
			t.Fatalf("ValuesVarType(%d) = %d/%v, want any", variable, got, found)
		}
	}
	_, closed, _ := contract.OutcomeAt(op, 2)
	if _, found := contract.ValuesTailType(closed); found {
		t.Fatal("closed Values exposed a tail type")
	}
	if allocations := testing.AllocsPerRun(1000, func() {
		_, _ = contract.ValuesVarType(op, 0)
		_, _ = contract.ValuesTailType(input)
	}); allocations != 0 {
		t.Fatalf("Values tail type queries allocated %f times", allocations)
	}
}

func TestValuesTailTypesRejectInvalidAndConflictingClasses(t *testing.T) {
	base := vocabulary.OperationSpec{
		ValuesVars: 1,
		Input:      vocabulary.ValuesSpec{Tail: vocabulary.ValuesVariable, Var: 0, TailType: testString},
		Outcomes:   []vocabulary.OutcomeSpec{{Kind: flowkind.OutcomeNormal, Values: vocabulary.ValuesSpec{Tail: vocabulary.ValuesVariable, Var: 0, TailType: testString}}},
		Effects:    vocabulary.RowSpec{Tail: vocabulary.RowClosed},
	}
	for _, test := range []struct {
		name string
		edit func(*vocabulary.OperationSpec)
	}{
		{"closed tail class", func(op *vocabulary.OperationSpec) {
			op.Input = vocabulary.ValuesSpec{Tail: vocabulary.ValuesClosed, TailType: testString}
		}},
		{"unknown tail class", func(op *vocabulary.OperationSpec) {
			op.Input = vocabulary.ValuesSpec{Tail: vocabulary.ValuesUnknown, TailType: testString}
		}},
		{"conflicting explicit class", func(op *vocabulary.OperationSpec) { op.Outcomes[0].Values.TailType = testNumber }},
		{"omitted any conflicts", func(op *vocabulary.OperationSpec) { op.Outcomes[0].Values.TailType = schematype.Type{} }},
	} {
		t.Run(test.name, func(t *testing.T) {
			op := base
			op.Outcomes = append([]vocabulary.OutcomeSpec(nil), base.Outcomes...)
			test.edit(&op)
			if _, err := testSeal(&Spec{Operations: []vocabulary.OperationSpec{op}}); err == nil {
				t.Fatal("Seal accepted invalid Values tail class")
			}
		})
	}
}

func admissionTailClassOperation(name string, class schematype.Type) vocabulary.OperationSpec {
	empty := vocabulary.ValuesSpec{Tail: vocabulary.ValuesClosed}
	optional := testUnion(class, testNil)
	return vocabulary.OperationSpec{
		Bindings:   []vocabulary.BindingSpec{{Namespace: vocabulary.BindingBuiltin, Member: []string{name}}},
		ValuesVars: 1,
		Input:      empty,
		Outcomes:   []vocabulary.OutcomeSpec{{Kind: flowkind.OutcomeNormal, Values: vocabulary.ValuesSpec{Fixed: []schematype.Type{testAny}, Tail: vocabulary.ValuesClosed}}},
		Subedges: []vocabulary.SubedgeSpec{{
			Role:      1,
			Family:    vocabulary.SubedgeFamilyLength,
			Admission: schematype.CallableAdmissionOrdinary,
			Arguments: vocabulary.ValuesSpec{Fixed: []schematype.Type{testAny}, Tail: vocabulary.ValuesClosed},
			ArgumentOrigins: []vocabulary.ArgumentOrigin{{
				Segment: vocabulary.ArgumentFixed, Kind: vocabulary.ArgumentSourceRule,
			}},
			Outcomes: []vocabulary.TerminalSpec{
				{Kind: flowkind.OutcomeNormal, Values: empty},
				{Kind: flowkind.OutcomeReturn, Values: empty},
				{Kind: flowkind.OutcomeThrow, Values: empty},
				{Kind: flowkind.OutcomeYield, Values: empty},
				{Kind: flowkind.OutcomeCancel, Values: empty},
			},
			AdmissionFailure: vocabulary.AdmissionFailureSpec{
				Values: vocabulary.ValuesSpec{Tail: vocabulary.ValuesVariable, Var: 0, TailType: class},
				Route: vocabulary.AdmissionRouteSpec{
					Route: vocabulary.RouteOutcome, Adjustment: vocabulary.AdjustmentExact,
					Result:    vocabulary.ValuesSpec{Fixed: []schematype.Type{optional}, Tail: vocabulary.ValuesClosed},
					Placement: vocabulary.PlacementFixed, Outcome: 0,
				},
			},
			Routes: []vocabulary.SubedgeRouteSpec{
				{Kind: flowkind.OutcomeNormal, Route: vocabulary.RouteContinue, Adjustment: vocabulary.AdjustmentExact, Result: empty},
				{Kind: flowkind.OutcomeReturn, Route: vocabulary.RouteContinue, Adjustment: vocabulary.AdjustmentExact, Result: empty},
				{Kind: flowkind.OutcomeThrow, Route: vocabulary.RouteContinue, Adjustment: vocabulary.AdjustmentExact, Result: empty},
				{Kind: flowkind.OutcomeYield, Route: vocabulary.RoutePropagateYield, Adjustment: vocabulary.AdjustmentPreserve, Result: empty},
				{Kind: flowkind.OutcomeCancel, Route: vocabulary.RouteContinue, Adjustment: vocabulary.AdjustmentExact, Result: empty},
			},
		}},
		Effects: vocabulary.RowSpec{Tail: vocabulary.RowClosed},
	}
}

func TestAdmissionFailureTailContributesToTheOneValuesVarClassTable(t *testing.T) {
	strings := mustSeal(t, Spec{Operations: []vocabulary.OperationSpec{admissionTailClassOperation("admission-tail-string", testString)}})
	op, found := strings.Lookup(vocabulary.BindingSpec{Namespace: vocabulary.BindingBuiltin, Member: []string{"admission-tail-string"}})
	if !found {
		t.Fatal("admission-tail-string operation missing")
	}
	edge, found := strings.SubedgeAt(op, 0)
	if !found {
		t.Fatal("admission-tail-string Subedge missing")
	}
	failure, found := strings.AdmissionFailure(edge)
	if !found {
		t.Fatal("admission failure source missing")
	}
	if tail, variable, ok := strings.ValuesTail(failure); !ok || tail != vocabulary.ValuesVariable || variable != 0 {
		t.Fatalf("admission failure tail = %d/%d/%v", tail, variable, ok)
	}
	if class, ok := strings.ValuesTailType(failure); !ok || !sameFrozenType(t, strings, class, testString) {
		t.Fatalf("admission failure tail class = %d/%v, want string", class, ok)
	}

	numbers := mustSeal(t, Spec{Operations: []vocabulary.OperationSpec{admissionTailClassOperation("admission-tail-string", testNumber)}})
	if strings.ContentID() == numbers.ContentID() {
		t.Fatal("admission tail class was omitted from ContentID")
	}

	conflict := admissionTailClassOperation("admission-tail-conflict", testString)
	// Input does not participate in this admission transport. If the separate
	// admission source were omitted from ValuesVar closure, this would silently
	// seal as number (or fall back to Any); it must instead reject the conflict.
	conflict.Input = vocabulary.ValuesSpec{Tail: vocabulary.ValuesVariable, Var: 0, TailType: testNumber}
	if _, err := testSeal(&Spec{Operations: []vocabulary.OperationSpec{conflict}}); err == nil {
		t.Fatal("admission failure tail class conflicted with owner input but sealed")
	}
}

func sameFrozenType(t *testing.T, contract *Contract, frozen vocabulary.Type, want schematype.Type) bool {
	t.Helper()
	got, gotOK := contract.TypeDeclaration(frozen)
	return gotOK && got.Equal(want)
}

// TestPublicationEffectRejectsInvalidDestinationAuthority states the two
// destination fences a publication spec must clear before Seal admits it: the
// role is one of the two declared roles, and a destination-free publication
// names no context formal. Both are stated at the Seal boundary rather than at
// any interior phase, so the law holds wherever inside Seal the projection is
// frozen.
func TestPublicationEffectRejectsInvalidDestinationAuthority(t *testing.T) {
	undeclaredRole := sendPublication(vocabulary.PublicationMutabilityPreserve)
	undeclaredRole.Destination = vocabulary.PublicationDestinationRole(7)
	if err := publicationSealError(undeclaredRole); err == nil {
		t.Fatal("Seal admitted a publication with an undeclared destination role")
	}

	destinationFreeWithContext := publicationFor(vocabulary.PublicationEffectReturnEscape)
	destinationFreeWithContext.Context = 1
	if destinationFreeWithContext.Destination != vocabulary.PublicationDestinationNone {
		t.Fatal("fixture no longer authors a destination-free publication")
	}
	if err := publicationSealError(destinationFreeWithContext); err == nil {
		t.Fatal("Seal admitted a destination-free publication carrying a context formal")
	}
}
