package analysis

import (
	"github.com/wippyai/go-lua/analysis/domain/effect/callsite"
	packdomain "github.com/wippyai/go-lua/analysis/domain/pack"
	valuedomain "github.com/wippyai/go-lua/analysis/domain/value"
	"github.com/wippyai/go-lua/analysis/engine"
	"github.com/wippyai/go-lua/analysis/identity"
	programartifact "github.com/wippyai/go-lua/analysis/program/artifact"
)

// selectedDirectAllocationMembershipAttachment is an Analysis-local bridge
// from one exact EffectSelected member to Value's summary surface. The Engine
// observation handle is intentionally private: callers cannot provide a
// shaped ValueSummaryObservation to this path.
type selectedDirectAllocationMembershipAttachment struct {
	observation engine.ReceiptObservation[valueSummaryObservation]
	id          identity.ContentID
	mount       identity.ContentID
	point       identity.ContentID
	call        identity.ContentID
	width       uint32
}

func selectedDirectAllocationMembershipAttachmentID(mount, point, call identity.ContentID, width uint32) (identity.ContentID, bool) {
	if !mount.Available() || !point.Available() || !call.Available() || width == 0 {
		return identity.ContentID{}, false
	}
	widthBytes := [4]byte{byte(width >> 24), byte(width >> 16), byte(width >> 8), byte(width)}
	return identity.DeriveContentID("analysis/direct-allocation-membership-observation/v1", mount[:], point[:], call[:], widthBytes[:])
}

func (attachment selectedDirectAllocationMembershipAttachment) valid() bool {
	want, ok := selectedDirectAllocationMembershipAttachmentID(attachment.mount, attachment.point, attachment.call, attachment.width)
	return ok && attachment.id == want && attachment.observation.MatchesID(attachment.id)
}

func selectedEffectMemberRef(mounts []mountedProgramArtifact, mount, occurrence identity.ContentID) (artifactRuleMemberRef, bool) {
	var found artifactRuleMemberRef
	for _, candidate := range mounts {
		if candidate.moduleKey != mount || !candidate.ruleMembersReady {
			continue
		}
		for _, member := range candidate.ruleMembers {
			if member.role != programartifact.RuleRoleEffectSelected || member.mount != mount || member.occurrence != occurrence {
				continue
			}
			if found.point.Available() {
				return artifactRuleMemberRef{}, false
			}
			found = member
		}
	}
	return found, found.point.Available()
}

// attachSelectedDirectAllocationMembership attaches one Value summary root to
// the exact selected CallEffect member already admitted by this receipt graph.
// It intentionally precedes solving and has no transition, placement, or
// allocation input: one call occurrence shares this private observation across
// any later completed publication candidates.
func attachSelectedDirectAllocationMembership(
	compilation *engine.ReceiptCompilation,
	binding *programBinding,
	graph *engine.ReceiptGraph,
	mounts []mountedProgramArtifact,
	mount, call identity.ContentID,
) (selectedDirectAllocationMembershipAttachment, bool) {
	if compilation == nil || binding == nil || binding.valueQuery == nil || graph == nil || !mount.Available() || !call.Available() {
		return selectedDirectAllocationMembershipAttachment{}, false
	}
	ref, refOK := selectedEffectMemberRef(mounts, mount, call)
	role, roleOK := binding.mountedCapability(programartifact.RuleRoleEffectSelected)
	valueSchema := binding.value.Schema()
	if !refOK || !roleOK || valueSchema == nil || valueSchema.CoordinateCount() <= 0 || uint64(valueSchema.CoordinateCount()) > uint64(^uint32(0)) {
		return selectedDirectAllocationMembershipAttachment{}, false
	}
	width := uint32(valueSchema.CoordinateCount())
	id, idOK := selectedDirectAllocationMembershipAttachmentID(mount, ref.point, call, width)
	if !idOK {
		return selectedDirectAllocationMembershipAttachment{}, false
	}
	member, memberOK := graph.MountedRuleMember(role, mount, ref.point, call)
	if !memberOK {
		return selectedDirectAllocationMembershipAttachment{}, false
	}
	observation, failure := engine.AttachRuleSummaryObservationWithFailure[valuedomain.Value, valueSummaryObservation](compilation, binding.valueQuery, id, member)
	if failure != engine.ReceiptObservationAttachFailureNone || !observation.Available() {
		return selectedDirectAllocationMembershipAttachment{}, false
	}
	attachment := selectedDirectAllocationMembershipAttachment{observation: observation, id: id, mount: mount, point: ref.point, call: call, width: width}
	return attachment, attachment.valid()
}

// directAllocationMembershipProof is detached evidence of one exact solved
// subject cell. It remains deliberately narrower than uniqueness or alias
// evidence and cannot be projected as a placement result.
type directAllocationMembershipProof struct {
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

func (proof directAllocationMembershipProof) valid() bool {
	want, ok := directAllocationMembershipProofID(proof.attachment, proof.correlation, proof.direct, proof.membership)
	return ok && proof.id == want
}

// prove reads the retained private Engine observation only after a completed
// transition correlation reauthenticates the same occurrence, live binding,
// and direct receipt. It rejects absent, Top, mixed, foreign, stale, or
// shaped observation data by never accepting observation values from callers.
func (attachment selectedDirectAllocationMembershipAttachment) prove(
	solver *engine.Solver,
	state *engine.State,
	transition callsite.PublicationTransitionProof,
	correlation callsite.PublicationPlacementCorrelationCandidate,
	subject packdomain.RuntimeAllocationContextBinding,
	direct valuedomain.DirectAllocationSubject,
) (directAllocationMembershipProof, bool) {
	if !attachment.valid() || !transition.MatchesCompletion(solver, state) || !correlation.Valid() || !subject.Valid() || !direct.Valid() || !direct.MatchesRuntimeBinding(subject) {
		return directAllocationMembershipProof{}, false
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
		return directAllocationMembershipProof{}, false
	}
	observation, readable := engine.ReceiptObservationResult[valueSummaryObservation](attachment.observation, solver, state)
	if !readable || !observation.Valid || observation.Rows != 1 || len(observation.Values) != int(attachment.width) || len(observation.Present) != int(attachment.width) {
		return directAllocationMembershipProof{}, false
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
			return directAllocationMembershipProof{}, false
		}
		membership, matched = classification, true
	}
	if !matched {
		return directAllocationMembershipProof{}, false
	}
	id, idOK := directAllocationMembershipProofID(attachment.id, correlationID, directID, membership)
	proof := directAllocationMembershipProof{id: id, attachment: attachment.id, correlation: correlationID, direct: directID, membership: membership}
	return proof, idOK && proof.valid()
}
