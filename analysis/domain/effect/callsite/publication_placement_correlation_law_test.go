package callsite

import (
	"crypto/sha256"
	"testing"

	"github.com/wippyai/go-lua/analysis/domain/pack"
	"github.com/wippyai/go-lua/program/keyspace"
	"github.com/wippyai/go-lua/program/target"
)

func publicationCorrelationLawID(label string) keyspace.ContentID {
	return keyspace.ContentID(sha256.Sum256([]byte(label)))
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
