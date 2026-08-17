package callsite

import (
	"crypto/sha256"
	"testing"

	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/program/target"
	"github.com/wippyai/go-lua/domain/pack"
)

func publicationCorrelationLawID(label string) identity.ContentID {
	return identity.ContentID(sha256.Sum256([]byte(label)))
}

// This law deliberately exercises only the detached scalar seal. The positive
// issuer path is covered where an exact solved PublicationTransitionProof and
// same-seal Pack bindings coexist; this package must not fabricate either
// owner capability merely to test the post-issuance candidate.
func TestPublicationPlacementCorrelationCandidateDetachedSealLaw(t *testing.T) {
	candidate := PublicationPlacementCorrelationCandidate{
		proof:       publicationCorrelationLawID("proof"),
		descriptor:  publicationCorrelationLawID("descriptor"),
		occurrence:  publicationCorrelationLawID("occurrence"),
		mount:       publicationCorrelationLawID("mount"),
		call:        publicationCorrelationLawID("call"),
		subject:     publicationCorrelationLawID("subject-binding"),
		destination: publicationCorrelationLawID("destination-binding"),
		hasContext:  true,
		kind:        target.PublicationEffectSendTransfer,
		escape:      target.PublicationEscapeSendTransfer,
		mutability:  target.PublicationMutabilityCopyOnWrite,
		lifetime:    target.PublicationLifetimePreserve,
		sealed:      true,
	}
	candidate.id = publicationPlacementCorrelationID(candidate.proof, candidate.descriptor, candidate.occurrence, candidate.mount, candidate.call, candidate.subject, candidate.destination, candidate.hasContext, candidate.kind, candidate.escape, candidate.mutability, candidate.lifetime)
	if !candidate.Valid() {
		t.Fatal("detached scalar correlation candidate invalid")
	}
	if destination, ok := candidate.DestinationBindingID(); !ok || destination != candidate.destination {
		t.Fatal("detached destination binding identity")
	}
	splicedSubject := candidate
	splicedSubject.subject = publicationCorrelationLawID("foreign-subject-binding")
	if splicedSubject.Valid() {
		t.Fatal("spliced subject binding survived scalar seal")
	}
	splicedDestination := candidate
	splicedDestination.destination = publicationCorrelationLawID("foreign-destination-binding")
	if splicedDestination.Valid() {
		t.Fatal("spliced destination binding survived scalar seal")
	}
	splicedConsequence := candidate
	splicedConsequence.mutability = target.PublicationMutabilitySeal
	if splicedConsequence.Valid() {
		t.Fatal("spliced typed consequence survived scalar seal")
	}

	if _, ok := NewPublicationPlacementCorrelationCandidate(PublicationTransitionProof{}, pack.RuntimeAllocationContextBinding{}, pack.RuntimeDestinationContextBinding{}, false); ok {
		t.Fatal("unproved transition issued a correlation candidate")
	}
}

// The scalar derivation runs once, where the row seals. This law states that
// the identity a projection hands back is the identity that derivation
// produces, and that reading it repeatedly is the same read: a sealed row's
// scalars are the derivation's own inputs, so a projection that returned
// anything else would be handing out an identity no derivation ever agreed to.
func TestPublicationPlacementCorrelationSealedIdentityIsTheDerivedIdentityLaw(t *testing.T) {
	candidate := PublicationPlacementCorrelationCandidate{
		proof:      publicationCorrelationLawID("proof"),
		descriptor: publicationCorrelationLawID("descriptor"),
		occurrence: publicationCorrelationLawID("occurrence"),
		mount:      publicationCorrelationLawID("mount"),
		call:       publicationCorrelationLawID("call"),
		subject:    publicationCorrelationLawID("subject-binding"),
		hasContext: false,
		kind:       target.PublicationEffectSendTransfer,
		escape:     target.PublicationEscapeSendTransfer,
		mutability: target.PublicationMutabilityCopyOnWrite,
		lifetime:   target.PublicationLifetimePreserve,
	}
	candidate.id = candidate.derivedID()
	candidate.sealed = true

	derived := candidate.derivedID()
	first, firstOK := candidate.ContentID()
	second, secondOK := candidate.ContentID()
	if !firstOK || !secondOK || first != derived || second != derived {
		t.Fatalf("sealed identity %x/%v read as %x/%v then %x/%v", derived, true, first, firstOK, second, secondOK)
	}
	if !candidate.Valid() || !candidate.valid() {
		t.Fatal("sealed row rejected by its own seal")
	}
	for _, projection := range []struct {
		name string
		read func() (identity.ContentID, bool)
	}{
		{"proof", candidate.ProofID},
		{"descriptor", candidate.DescriptorID},
		{"occurrence", candidate.OccurrenceID},
		{"subject", candidate.SubjectBindingID},
	} {
		id, ok := projection.read()
		if !ok || !id.Available() {
			t.Fatalf("sealed row withheld its %s identity", projection.name)
		}
	}
	if candidate.Kind() != target.PublicationEffectSendTransfer || candidate.Escape() != target.PublicationEscapeSendTransfer ||
		candidate.Mutability() != target.PublicationMutabilityCopyOnWrite || candidate.Lifetime() != target.PublicationLifetimePreserve {
		t.Fatal("sealed row withheld a typed consequence it carries")
	}
	if destination, ok := candidate.DestinationBindingID(); ok || destination.Available() {
		t.Fatal("context-free row published a destination binding")
	}
}

// The seal is stamped by issuance, not recomputed by a reader. This law states
// that a row carrying a correctly derived identity but no issuance seal reads
// as nothing at all, so no path outside NewPublicationPlacementCorrelationCandidate
// can assemble a row a consumer would accept.
func TestPublicationPlacementCorrelationUnsealedRowReadsNothingLaw(t *testing.T) {
	unsealed := PublicationPlacementCorrelationCandidate{
		proof:      publicationCorrelationLawID("proof"),
		descriptor: publicationCorrelationLawID("descriptor"),
		occurrence: publicationCorrelationLawID("occurrence"),
		mount:      publicationCorrelationLawID("mount"),
		call:       publicationCorrelationLawID("call"),
		subject:    publicationCorrelationLawID("subject-binding"),
		kind:       target.PublicationEffectSendTransfer,
		escape:     target.PublicationEscapeSendTransfer,
		mutability: target.PublicationMutabilityCopyOnWrite,
		lifetime:   target.PublicationLifetimePreserve,
	}
	unsealed.id = unsealed.derivedID()
	if !unsealed.id.Available() || unsealed.id != unsealed.derivedID() {
		t.Fatal("law fixture did not derive an identity to withhold the seal from")
	}
	if unsealed.Valid() {
		t.Fatal("unissued row passed the scalar seal")
	}
	if _, ok := unsealed.ContentID(); ok {
		t.Fatal("unissued row published its identity")
	}
	if _, ok := unsealed.ProofID(); ok {
		t.Fatal("unissued row published its proof identity")
	}
	if mount, call, ok := unsealed.CallProvenance(); ok || mount.Available() || call.Available() {
		t.Fatal("unissued row published its call provenance")
	}
	if unsealed.Kind() != target.PublicationEffectInvalid || unsealed.Escape() != target.PublicationEscapeInvalid ||
		unsealed.Mutability() != target.PublicationMutabilityInvalid || unsealed.Lifetime() != target.PublicationLifetimeInvalid {
		t.Fatal("unissued row published a typed consequence")
	}

	var absent PublicationPlacementCorrelationCandidate
	if absent.Valid() {
		t.Fatal("zero-valued row passed the scalar seal")
	}
	if _, ok := absent.ContentID(); ok {
		t.Fatal("zero-valued row published an identity")
	}
}
