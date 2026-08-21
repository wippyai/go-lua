package publication

import (
	"github.com/wippyai/go-lua/analysis/engine"
	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/snapshot"
	"github.com/wippyai/go-lua/domain/effect/callsite"
	packdomain "github.com/wippyai/go-lua/domain/pack"
	valuedomain "github.com/wippyai/go-lua/domain/value"
)

// DirectAllocationMembershipAttachment is the bridge from one exact
// EffectSelected member to Value's summary surface. The declared observation
// row is intentionally private: callers cannot provide a shaped
// ValueSummaryObservation to this path, and cannot substitute a row that this
// attachment's identity did not authorize.
type DirectAllocationMembershipAttachment struct {
	observation engine.ProgramObservationAdmission
	id          identity.ContentID
	mount       identity.ContentID
	point       identity.ContentID
	call        identity.ContentID
	width       uint32
}

func directAllocationMembershipAttachmentID(mount, point, call identity.ContentID, width uint32) (identity.ContentID, bool) {
	if !mount.Available() || !point.Available() || !call.Available() || width == 0 {
		return identity.ContentID{}, false
	}
	widthBytes := [4]byte{byte(width >> 24), byte(width >> 16), byte(width >> 8), byte(width)}
	return identity.DeriveContentID("analysis/direct-allocation-membership-observation/v1", mount[:], point[:], call[:], widthBytes[:])
}

func (attachment DirectAllocationMembershipAttachment) valid() bool {
	want, ok := directAllocationMembershipAttachmentID(attachment.mount, attachment.point, attachment.call, attachment.width)
	return ok && attachment.id == want && attachment.observation.Available() && attachment.observation.ID == attachment.id
}

func (attachment DirectAllocationMembershipAttachment) Valid() bool { return attachment.valid() }

func (attachment DirectAllocationMembershipAttachment) ContentID() (identity.ContentID, bool) {
	return attachment.id, attachment.valid()
}

// DeclareSelectedDirectAllocationMembership states one Value summary root over
// the exact selected CallEffect member the committed program published. It
// precedes solving and has no transition, placement, or allocation input: one
// call occurrence shares this private observation across any later completed
// publication candidates.
//
// The caller names the member by the role capability and its authored
// coordinates; the committed program, not this relation, decides whether that
// member exists. A coordinate triple that is not a published member of the
// named role therefore has no attachment.
func DeclareSelectedDirectAllocationMembership(
	committed *engine.CommittedProgram,
	query *engine.SummaryQueryImplementation[valuedomain.Value, valuedomain.ValueSummaryObservation],
	role engine.RuleSlotCapability,
	mount, point, call identity.ContentID,
	width uint32,
) (DirectAllocationMembershipAttachment, bool) {
	if committed == nil || query == nil {
		return DirectAllocationMembershipAttachment{}, false
	}
	id, idOK := directAllocationMembershipAttachmentID(mount, point, call, width)
	if !idOK {
		return DirectAllocationMembershipAttachment{}, false
	}
	if _, published := committed.MountedRuleMember(role, mount, point, call); !published {
		return DirectAllocationMembershipAttachment{}, false
	}
	observation, declared := engine.NewSummaryObservationAdmission[valuedomain.Value, valuedomain.ValueSummaryObservation](query, id, role, mount, point, call)
	if !declared {
		return DirectAllocationMembershipAttachment{}, false
	}
	attachment := DirectAllocationMembershipAttachment{observation: observation, id: id, mount: mount, point: point, call: call, width: width}
	return attachment, attachment.valid()
}

// Observation is the declared observation row this attachment answers on. The
// seal that mints the Solver binds it; nothing else can.
func (attachment DirectAllocationMembershipAttachment) Observation() (engine.ProgramObservationAdmission, bool) {
	return attachment.observation, attachment.valid()
}

// DirectAllocationMembershipProof is detached evidence of one exact solved
// subject cell. It remains deliberately narrower than uniqueness or alias
// evidence and cannot be projected as a placement result.
type DirectAllocationMembershipProof struct {
	id          identity.ContentID
	attachment  identity.ContentID
	correlation identity.ContentID
	direct      identity.ContentID
	membership  valuedomain.AllocationMembership
}

func directAllocationMembershipProofID(attachment, correlation, direct identity.ContentID, membership valuedomain.AllocationMembership) (identity.ContentID, bool) {
	if !attachment.Available() || !correlation.Available() || !direct.Available() || membership != valuedomain.MembershipRecent && membership != valuedomain.MembershipSummary {
		return identity.ContentID{}, false
	}
	return identity.DeriveContentID("analysis/direct-allocation-membership/v1", attachment[:], correlation[:], direct[:], []byte{byte(membership)})
}

func (proof DirectAllocationMembershipProof) valid() bool {
	want, ok := directAllocationMembershipProofID(proof.attachment, proof.correlation, proof.direct, proof.membership)
	return ok && proof.id == want
}

func (proof DirectAllocationMembershipProof) Valid() bool { return proof.valid() }

func (proof DirectAllocationMembershipProof) ContentID() (identity.ContentID, bool) {
	return proof.id, proof.valid()
}

func (proof DirectAllocationMembershipProof) Membership() valuedomain.AllocationMembership {
	return proof.membership
}

func (proof DirectAllocationMembershipProof) CorrelationID() (identity.ContentID, bool) {
	return proof.correlation, proof.valid()
}

// DirectAllocationSubjectID reports the exact direct-identity receipt this
// proof was issued against. It is the relation's one published link between a
// solved membership cell and the Value/Pack coordinate join that named it.
func (proof DirectAllocationMembershipProof) DirectAllocationSubjectID() (identity.ContentID, bool) {
	return proof.direct, proof.valid()
}

// Prove reads the completed Snapshot only after a completed transition
// correlation reauthenticates the same occurrence, live binding, and direct
// receipt. It rejects absent, Top, mixed, foreign, stale, or shaped observation
// data by never accepting observation values from callers.
//
// This is also the relation's one cross-owner admission of the direct receipt:
// MatchesRuntimeBinding reauthenticates the live Pack seal, the binding's own
// semantic source, and the mounted requirement key behind the receipt's Heap
// key, so no separate detached admission carrier stands between the direct
// receipt and this proof.
func (attachment DirectAllocationMembershipAttachment) Prove(
	solver *engine.Solver,
	state *engine.State,
	transition callsite.PublicationTransitionProof,
	correlation callsite.PublicationPlacementCorrelationCandidate,
	subject packdomain.RuntimeAllocationContextBinding,
	direct DirectAllocationSubject,
) (DirectAllocationMembershipProof, bool) {
	if !attachment.valid() || !transition.MatchesCompletion(solver, state) || !correlation.Valid() || !subject.Valid() || !direct.Valid() || !direct.MatchesRuntimeBinding(subject) {
		return DirectAllocationMembershipProof{}, false
	}
	transitionID, transitionOK := transition.ContentID()
	transitionDescriptor, descriptorOK := transition.DescriptorID()
	transitionOccurrence, occurrenceOK := transition.OccurrenceID()
	correlationID, correlationOK := correlation.ContentID()
	correlationProof, correlationProofOK := correlation.ProofID()
	correlationDescriptor, correlationDescriptorOK := correlation.DescriptorID()
	correlationOccurrence, correlationOccurrenceOK := correlation.OccurrenceID()
	subjectID, subjectOK := correlation.SubjectBindingID()
	directID, directOK := direct.ContentID()
	mount, call, provenanceOK := correlation.CallProvenance()
	subjectMount, subjectCall, subjectProvenanceOK := subject.CallProvenance()
	if !transitionOK || !descriptorOK || !occurrenceOK || !correlationOK || !correlationProofOK || !correlationDescriptorOK || !correlationOccurrenceOK || !subjectOK || !directOK || !provenanceOK || !subjectProvenanceOK ||
		transitionID != correlationProof || transitionDescriptor != correlationDescriptor || transitionOccurrence != correlationOccurrence || transition.MountID() != mount || transition.CallOccurrenceID() != call ||
		subject.ID() != subjectID || mount != attachment.mount || call != attachment.call || subjectMount != mount || subjectCall != call {
		return DirectAllocationMembershipProof{}, false
	}
	sealed, publishedOK := solver.PublishedSnapshot(state)
	if !publishedOK {
		return DirectAllocationMembershipProof{}, false
	}
	published := sealed.Snapshot()
	plan, opened := snapshot.OpenQuery[identity.ContentID, engine.Answer](&published, sealed.ObservationFamily())
	if !opened {
		return DirectAllocationMembershipProof{}, false
	}
	answer, status := snapshot.Query(&published, plan, attachment.id)
	if status != snapshot.ReadHit || !answer.Available() {
		return DirectAllocationMembershipProof{}, false
	}
	observation, readable := engine.AnswerValue[valuedomain.ValueSummaryObservation](answer)
	if !readable || !observation.Valid || observation.Rows != 1 || len(observation.Values) != int(attachment.width) || len(observation.Present) != int(attachment.width) {
		return DirectAllocationMembershipProof{}, false
	}
	var membership valuedomain.AllocationMembership
	matched := false
	for index, present := range observation.Present {
		if !present {
			continue
		}
		classification, cellMatches := direct.ClassifySummaryCell(index, observation.Values[index])
		if !cellMatches {
			continue
		}
		if matched || classification != valuedomain.MembershipRecent && classification != valuedomain.MembershipSummary {
			return DirectAllocationMembershipProof{}, false
		}
		membership, matched = classification, true
	}
	if !matched {
		return DirectAllocationMembershipProof{}, false
	}
	id, idOK := directAllocationMembershipProofID(attachment.id, correlationID, directID, membership)
	proof := DirectAllocationMembershipProof{id: id, attachment: attachment.id, correlation: correlationID, direct: directID, membership: membership}
	return proof, idOK && proof.valid()
}
