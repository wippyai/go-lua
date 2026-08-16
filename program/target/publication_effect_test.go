package target

import "testing"

func sendPublication(mutability PublicationMutabilityDisposition) *PublicationEffectSpec {
	return &PublicationEffectSpec{
		Kind:        PublicationEffectSendTransfer,
		Subject:     0,
		Destination: PublicationDestinationValueFormal,
		Context:     1,
		Escape:      PublicationEscapeSendTransfer,
		Mutability:  mutability,
		Lifetime:    PublicationLifetimePreserve,
	}
}

func publicationEffectSpec(publication *PublicationEffectSpec, duplicate bool) Spec {
	occurrences := []EffectSpec{{Target: 2, ValueArgs: []ValueFormal{0, 1}, RowArgs: []RowVar{0}, Publication: publication}}
	if duplicate {
		occurrences = append(occurrences, occurrences[0])
	}
	return effectIdentitySpec(effectIdentityOperation("publication-owner", RowSpec{
		Occurrences: occurrences,
		Tail:        RowClosed,
	}))
}

func publicationEffectContract(t *testing.T, publication *PublicationEffectSpec, duplicate bool) (*Contract, Operation) {
	t.Helper()
	contract, owner, _ := firstEffectIdentityContract(t, publicationEffectSpec(publication, duplicate), "publication-owner")
	return contract, owner
}

func distinctPublicationEffectSpec(reverse bool) Spec {
	occurrences := []EffectSpec{
		{Target: 2, ValueArgs: []ValueFormal{0, 1}, RowArgs: []RowVar{0}, Publication: publicationFor(PublicationEffectReturnEscape)},
		{Target: 2, ValueArgs: []ValueFormal{0, 1}, RowArgs: []RowVar{0}, Publication: publicationFor(PublicationEffectFreezeSeal)},
	}
	if reverse {
		occurrences[0], occurrences[1] = occurrences[1], occurrences[0]
	}
	return effectIdentitySpec(effectIdentityOperation("publication-distinct", RowSpec{Occurrences: occurrences, Tail: RowClosed}))
}

func publicationFor(kind PublicationEffectKind) *PublicationEffectSpec {
	spec := &PublicationEffectSpec{
		Kind: kind, Subject: 0, Destination: PublicationDestinationNone,
		Escape: PublicationEscapeNone, Mutability: PublicationMutabilityPreserve, Lifetime: PublicationLifetimePreserve,
	}
	switch kind {
	case PublicationEffectSendTransfer:
		spec.Destination, spec.Context = PublicationDestinationValueFormal, 1
		spec.Escape = PublicationEscapeSendTransfer
	case PublicationEffectReturnEscape:
		spec.Escape = PublicationEscapeReturn
	case PublicationEffectCallbackEscape:
		spec.Escape = PublicationEscapeCallback
	case PublicationEffectFreezeSeal:
		spec.Mutability = PublicationMutabilitySeal
	case PublicationEffectWriteMutation:
		spec.Mutability = PublicationMutabilityWrite
	case PublicationEffectCloseRelease:
		spec.Lifetime = PublicationLifetimeRelease
	}
	return spec
}

func publicationSealError(publication *PublicationEffectSpec) error {
	spec := publicationEffectSpec(publication, false)
	_, err := Seal(&spec)
	return err
}

func TestPublicationEffectDescriptorOwnerLaw(t *testing.T) {
	for _, kind := range []PublicationEffectKind{
		PublicationEffectSendTransfer,
		PublicationEffectReturnEscape,
		PublicationEffectCallbackEscape,
		PublicationEffectFreezeSeal,
		PublicationEffectWriteMutation,
		PublicationEffectCloseRelease,
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
		if mismatched.Escape == PublicationEscapeNone {
			mismatched.Escape = PublicationEscapeReturn
		} else {
			mismatched.Escape = PublicationEscapeNone
		}
		if err := publicationSealError(&mismatched); err == nil {
			t.Fatalf("kind %d sealed with mismatched typed consequences", kind)
		}
	}

	contract, owner := publicationEffectContract(t, sendPublication(PublicationMutabilityCopyOnWrite), true)
	descriptor, ok := contract.PublicationEffectDescriptor(owner, 0)
	if !ok {
		t.Fatal("explicit publication descriptor unavailable")
	}
	if descriptor.Kind() != PublicationEffectSendTransfer || descriptor.Subject() != 0 ||
		descriptor.DestinationRole() != PublicationDestinationValueFormal || descriptor.Context() != 1 ||
		descriptor.Escape() != PublicationEscapeSendTransfer || descriptor.Mutability() != PublicationMutabilityCopyOnWrite ||
		descriptor.Lifetime() != PublicationLifetimePreserve {
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
	views, viewsOK := contract.SemanticSourceViews()
	if !viewsOK || views.PublicationEffect().OwnerID() != contract.ContentID() || views.PublicationEffect().Count() != 2 {
		t.Fatal("publication semantic-source receipt lost its Target owner fence or duplicate occurrences")
	}

	without, withoutOwner := publicationEffectContract(t, nil, false)
	if _, ok := without.PublicationEffectDescriptor(withoutOwner, 0); ok {
		t.Fatal("generic effect inferred publication semantics")
	}
	if _, ok := without.PublicationEffectDescriptorID(withoutOwner, 0); ok {
		t.Fatal("generic effect exposed a publication descriptor identity")
	}

	changed, changedOwner := publicationEffectContract(t, sendPublication(PublicationMutabilityPreserve), false)
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
	firstViews, firstViewsOK := firstDistinct.SemanticSourceViews()
	secondViews, secondViewsOK := secondDistinct.SemanticSourceViews()
	if !firstViewsOK || !secondViewsOK || firstViews.PublicationEffect().OwnerID() != secondViews.PublicationEffect().OwnerID() ||
		firstViews.PublicationEffect().Count() != secondViews.PublicationEffect().Count() {
		t.Fatal("permuted distinct publication effects changed semantic receipt owner or cardinality")
	}
	for index := 0; index < firstDistinct.EffectCount(firstDistinctOwner); index++ {
		firstDescriptorID, firstDescriptorOK := firstDistinct.PublicationEffectDescriptorID(firstDistinctOwner, index)
		secondDescriptorID, secondDescriptorOK := secondDistinct.PublicationEffectDescriptorID(secondDistinctOwner, index)
		firstOccurrenceID, firstOccurrenceOK := firstDistinct.PublicationEffectOccurrenceID(firstDistinctOwner, index)
		secondOccurrenceID, secondOccurrenceOK := secondDistinct.PublicationEffectOccurrenceID(secondDistinctOwner, index)
		firstRow, firstRowOK := firstViews.PublicationEffect().DigestAt(index)
		secondRow, secondRowOK := secondViews.PublicationEffect().DigestAt(index)
		if !firstDescriptorOK || !secondDescriptorOK || firstDescriptorID != secondDescriptorID ||
			!firstOccurrenceOK || !secondOccurrenceOK || firstOccurrenceID != secondOccurrenceID ||
			!firstRowOK || !secondRowOK || firstRow != secondRow {
			t.Fatalf("permuted distinct publication effect %d changed a sealed identity or receipt row", index)
		}
	}

	badSubject := sendPublication(PublicationMutabilityPreserve)
	badSubject.Subject = 2
	if err := publicationSealError(badSubject); err == nil {
		t.Fatal("out-of-ABI publication subject sealed")
	}
	badDestination := sendPublication(PublicationMutabilityPreserve)
	badDestination.Context = 2
	if err := publicationSealError(badDestination); err == nil {
		t.Fatal("out-of-ABI publication destination sealed")
	}
	badContext := publicationFor(PublicationEffectReturnEscape)
	badContext.Context = 1
	if err := publicationSealError(badContext); err == nil {
		t.Fatal("destination-free publication carried a context formal")
	}

	callbackPublication := publicationFor(PublicationEffectCallbackEscape)
	callbackContract, callbackOwner, callback := firstEffectIdentityContract(t, effectIdentitySpec(effectIdentityCallbackOperation("publication-callback", RowSpec{
		Occurrences: []EffectSpec{{Target: 2, ValueArgs: []ValueFormal{0, 1}, RowArgs: []RowVar{0}, Publication: callbackPublication}},
		Tail:        RowClosed,
	}, RowSpec{Tail: RowClosed})), "publication-callback")
	callbackDescriptor, callbackOK := callbackContract.CallbackPublicationEffectDescriptor(callback, 0)
	callbackID, callbackIDOK := callbackContract.CallbackPublicationEffectOccurrenceID(callback, 0)
	callbackViews, callbackViewsOK := callbackContract.SemanticSourceViews()
	if !callbackOK || !callbackIDOK || !callbackID.Available() || callbackDescriptor.Kind() != PublicationEffectCallbackEscape ||
		!callbackViewsOK || callbackViews.PublicationEffect().OwnerID() != callbackContract.ContentID() || callbackViews.PublicationEffect().Count() != 1 || callbackOwner == 0 {
		t.Fatal("callback publication descriptor or its Contract-owned semantic receipt is unavailable")
	}
}
