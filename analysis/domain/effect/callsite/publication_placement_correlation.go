package callsite

import (
	"crypto/sha256"

	"github.com/wippyai/go-lua/analysis/domain/pack"
	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/program/target"
)

const publicationPlacementCorrelationDomain = "wippy.analysis.effect.publication-placement-correlation.v1\x00"

// PublicationPlacementCorrelationCandidate is the detached, post-convergence
// join between one proved publication transition and its exact call-local Pack
// bindings. It is deliberately not a placement result: alias, freeze/COW,
// lifetime and residence proofs remain required before any placement lattice
// conclusion can be emitted.
//
// The candidate retains scalar proof and binding identities only. In
// particular, it retains no Plan, Link, Target, Solver, State, Heap, Pack, or
// runtime capability. That makes it safe to carry as a static correlation
// input without extending either result lifetime or placement authority.
type PublicationPlacementCorrelationCandidate struct {
	id          identity.ContentID
	proof       identity.ContentID
	descriptor  identity.ContentID
	occurrence  identity.ContentID
	mount       identity.ContentID
	call        identity.ContentID
	subject     identity.ContentID
	destination identity.ContentID
	hasContext  bool
	kind        target.PublicationEffectKind
	escape      target.PublicationEscapeDisposition
	mutability  target.PublicationMutabilityDisposition
	lifetime    target.PublicationLifetimeDisposition
}

func publicationPlacementCorrelationID(proof, descriptor, occurrence, mount, call, subject, destination identity.ContentID, hasContext bool, kind target.PublicationEffectKind, escape target.PublicationEscapeDisposition, mutability target.PublicationMutabilityDisposition, lifetime target.PublicationLifetimeDisposition) identity.ContentID {
	if !proof.Available() || !descriptor.Available() || !occurrence.Available() || !mount.Available() || !call.Available() || !subject.Available() ||
		hasContext && !destination.Available() || !hasContext && destination.Available() || !publicationPlacementConsequencesValid(kind, escape, mutability, lifetime) {
		return identity.ContentID{}
	}
	hash := sha256.New()
	_, _ = hash.Write([]byte(publicationPlacementCorrelationDomain))
	for _, value := range [...]identity.ContentID{proof, descriptor, occurrence, mount, call, subject, destination} {
		_, _ = hash.Write(value[:])
	}
	if hasContext {
		_, _ = hash.Write([]byte{1})
	} else {
		_, _ = hash.Write([]byte{0})
	}
	_, _ = hash.Write([]byte{byte(kind), byte(escape), byte(mutability), byte(lifetime)})
	var id identity.ContentID
	copy(id[:], hash.Sum(nil))
	return id
}

// publicationPlacementConsequencesValid validates only the closed enum
// domains carried from the already-authenticated Target descriptor. It does
// not restate Target's kind-to-consequence matrix: that remains Target's sole
// semantic authority and was checked by PublicationTransitionProof.
func publicationPlacementConsequencesValid(kind target.PublicationEffectKind, escape target.PublicationEscapeDisposition, mutability target.PublicationMutabilityDisposition, lifetime target.PublicationLifetimeDisposition) bool {
	// The fields are private and can enter only through the proven Target-owned
	// transition. Do not duplicate Target's closed kind/consequence inventory
	// here merely to validate a detached transport row.
	return kind != target.PublicationEffectInvalid && escape != target.PublicationEscapeInvalid &&
		mutability != target.PublicationMutabilityInvalid && lifetime != target.PublicationLifetimeInvalid
}

func (candidate PublicationPlacementCorrelationCandidate) valid() bool {
	if !candidate.id.Available() {
		return false
	}
	return candidate.id == publicationPlacementCorrelationID(
		candidate.proof, candidate.descriptor, candidate.occurrence, candidate.mount, candidate.call,
		candidate.subject, candidate.destination, candidate.hasContext, candidate.kind, candidate.escape,
		candidate.mutability, candidate.lifetime,
	)
}

// Valid reports that the detached candidate's scalar seal has not been
// spliced or mutated. It intentionally does not re-read its original proof:
// the proof was consumed at issuance and is not retained.
func (candidate PublicationPlacementCorrelationCandidate) Valid() bool { return candidate.valid() }

// NewPublicationPlacementCorrelationCandidate admits one exact completed
// PublicationTransitionProof. The subject must be the identical Pack selector
// bound to a subject allocation; when the proof declares a context selector,
// the destination must be an independently-issued destination-context binding
// at the same mounted call. destinationPresent is explicit: a stale or
// malformed non-zero destination capability is never treated as absence.
// Destination bindings never carry an allocation requirement and cannot be
// mistaken for subject placement evidence.
func NewPublicationPlacementCorrelationCandidate(proof PublicationTransitionProof, subject pack.RuntimeAllocationContextBinding, destination pack.RuntimeDestinationContextBinding, destinationPresent bool) (PublicationPlacementCorrelationCandidate, bool) {
	proofID, proofOK := proof.ContentID()
	descriptor, descriptorOK := proof.DescriptorID()
	occurrence, occurrenceOK := proof.OccurrenceID()
	subjectSelector, subjectSelectorOK := proof.SubjectSelector()
	if !proofOK || !descriptorOK || !occurrenceOK || !subjectSelectorOK || !subject.Valid() || !subject.MatchesSelector(subjectSelector) {
		return PublicationPlacementCorrelationCandidate{}, false
	}
	mount, call := proof.MountID(), proof.CallOccurrenceID()
	subjectModule, subjectCall, subjectProvenanceOK := subject.CallProvenance()
	if !mount.Available() || !call.Available() || !subjectProvenanceOK || subjectModule != mount || subjectCall != call {
		return PublicationPlacementCorrelationCandidate{}, false
	}
	subjectID := subject.ID()
	if !subjectID.Available() {
		return PublicationPlacementCorrelationCandidate{}, false
	}

	hasContext := false
	destinationID := identity.ContentID{}
	if contextSelector, contextRequired := proof.ContextSelector(); contextRequired {
		destinationModule, destinationCall, destinationProvenanceOK := destination.CallProvenance()
		if !destinationPresent || !pack.SameRuntimeAllocationContextBindingIssuer(subject, destination) || !destination.MatchesSelector(contextSelector) || !destinationProvenanceOK || destinationModule != mount || destinationCall != call {
			return PublicationPlacementCorrelationCandidate{}, false
		}
		destinationID = destination.ID()
		if !destinationID.Available() {
			return PublicationPlacementCorrelationCandidate{}, false
		}
		hasContext = true
	} else if destinationPresent || !pack.RuntimeDestinationContextBindingAbsent(destination) {
		// A destination-free transition must receive the canonical absent value,
		// not a live, stale, foreign, or malformed extra capability.
		return PublicationPlacementCorrelationCandidate{}, false
	}

	candidate := PublicationPlacementCorrelationCandidate{
		proof: proofID, descriptor: descriptor, occurrence: occurrence, mount: mount, call: call,
		subject: subjectID, destination: destinationID, hasContext: hasContext,
		kind: proof.Kind(), escape: proof.Escape(), mutability: proof.Mutability(), lifetime: proof.Lifetime(),
	}
	candidate.id = publicationPlacementCorrelationID(candidate.proof, candidate.descriptor, candidate.occurrence, candidate.mount, candidate.call, candidate.subject, candidate.destination, candidate.hasContext, candidate.kind, candidate.escape, candidate.mutability, candidate.lifetime)
	return candidate, candidate.valid()
}

func (candidate PublicationPlacementCorrelationCandidate) ContentID() (identity.ContentID, bool) {
	return candidate.id, candidate.valid()
}

func (candidate PublicationPlacementCorrelationCandidate) ProofID() (identity.ContentID, bool) {
	return candidate.proof, candidate.valid()
}

func (candidate PublicationPlacementCorrelationCandidate) DescriptorID() (identity.ContentID, bool) {
	return candidate.descriptor, candidate.valid()
}

func (candidate PublicationPlacementCorrelationCandidate) OccurrenceID() (identity.ContentID, bool) {
	return candidate.occurrence, candidate.valid()
}

func (candidate PublicationPlacementCorrelationCandidate) CallProvenance() (mount, call identity.ContentID, ok bool) {
	if !candidate.valid() {
		return identity.ContentID{}, identity.ContentID{}, false
	}
	return candidate.mount, candidate.call, true
}

func (candidate PublicationPlacementCorrelationCandidate) SubjectBindingID() (identity.ContentID, bool) {
	return candidate.subject, candidate.valid()
}

func (candidate PublicationPlacementCorrelationCandidate) DestinationBindingID() (identity.ContentID, bool) {
	return candidate.destination, candidate.valid() && candidate.hasContext
}

func (candidate PublicationPlacementCorrelationCandidate) Kind() target.PublicationEffectKind {
	if !candidate.valid() {
		return target.PublicationEffectInvalid
	}
	return candidate.kind
}

func (candidate PublicationPlacementCorrelationCandidate) Escape() target.PublicationEscapeDisposition {
	if !candidate.valid() {
		return target.PublicationEscapeInvalid
	}
	return candidate.escape
}

func (candidate PublicationPlacementCorrelationCandidate) Mutability() target.PublicationMutabilityDisposition {
	if !candidate.valid() {
		return target.PublicationMutabilityInvalid
	}
	return candidate.mutability
}

func (candidate PublicationPlacementCorrelationCandidate) Lifetime() target.PublicationLifetimeDisposition {
	if !candidate.valid() {
		return target.PublicationLifetimeInvalid
	}
	return candidate.lifetime
}
