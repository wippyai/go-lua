package publication

import (
	"github.com/wippyai/go-lua/analysis/engine"
	"github.com/wippyai/go-lua/analysis/identity"
	valuedomain "github.com/wippyai/go-lua/domain/value"
)

// BranchValueObservationAttachment is the bridge from one mounted branch
// evidence point to Value's summary surface. The Engine observation handle is
// intentionally private: a caller cannot hand a shaped ValueSummaryObservation
// to this path, and cannot substitute a handle that this attachment's identity
// did not authorize. One evidence point carries one attachment, shared by every
// later reader of that point.
type BranchValueObservationAttachment struct {
	observation engine.ReceiptObservation[valuedomain.ValueSummaryObservation]
	id          identity.ContentID
	mount       identity.ContentID
	point       identity.ContentID
}

func branchValueObservationAttachmentID(mount, point identity.ContentID) (identity.ContentID, bool) {
	if !mount.Available() || !point.Available() {
		return identity.ContentID{}, false
	}
	return identity.DeriveContentID("analysis/branch-value-observation/v1", mount[:], point[:], []byte("value-summary"))
}

func (attachment BranchValueObservationAttachment) valid() bool {
	want, ok := branchValueObservationAttachmentID(attachment.mount, attachment.point)
	return ok && attachment.id == want && attachment.observation.MatchesID(attachment.id)
}

func (attachment BranchValueObservationAttachment) Valid() bool { return attachment.valid() }

func (attachment BranchValueObservationAttachment) ContentID() (identity.ContentID, bool) {
	return attachment.id, attachment.valid()
}

// AttachBranchValueObservation attaches one Value summary root to the exact
// mounted rule member that produces this evidence point. It precedes solving
// and reads no solved state: the caller names the member by role capability and
// authored coordinates, and the graph, not this relation, decides whether that
// member exists.
//
// The failure is the Engine attach classification, so a caller that binds a
// whole receipt of evidence points reports which coordinate rejected the
// binding rather than a single opaque refusal.
func AttachBranchValueObservation(
	compilation *engine.ReceiptCompilation,
	query *engine.SummaryQueryImplementation[valuedomain.Value, valuedomain.ValueSummaryObservation],
	graph *engine.ReceiptGraph,
	role engine.RuleSlotCapability,
	mount, point, occurrence identity.ContentID,
) (BranchValueObservationAttachment, engine.ReceiptObservationAttachFailure, bool) {
	if compilation == nil || query == nil || graph == nil {
		return BranchValueObservationAttachment{}, engine.ReceiptObservationAttachFailureArguments, false
	}
	id, idOK := branchValueObservationAttachmentID(mount, point)
	if !idOK {
		return BranchValueObservationAttachment{}, engine.ReceiptObservationAttachFailureArguments, false
	}
	member, memberOK := graph.MountedRuleMember(role, mount, point, occurrence)
	if !memberOK {
		return BranchValueObservationAttachment{}, engine.ReceiptObservationAttachFailurePoint, false
	}
	observation, failure := engine.AttachRuleSummaryObservationWithFailure[valuedomain.Value, valuedomain.ValueSummaryObservation](compilation, query, id, member)
	if failure != engine.ReceiptObservationAttachFailureNone || !observation.Available() {
		return BranchValueObservationAttachment{}, failure, false
	}
	attachment := BranchValueObservationAttachment{observation: observation, id: id, mount: mount, point: point}
	if !attachment.valid() {
		return BranchValueObservationAttachment{}, engine.ReceiptObservationAttachFailureArguments, false
	}
	return attachment, engine.ReceiptObservationAttachFailureNone, true
}

// MemberAdmitted asks the graph whether the named role and occurrence still
// resolve to an admitted member at this attachment's own evidence point. A
// caller whose receipt names the same point from a second producer holds one
// attachment for it and reauthenticates the additional coordinates here, so a
// producer that names no admitted member is rejected rather than silently
// folded into an attachment it never authorized.
func (attachment BranchValueObservationAttachment) MemberAdmitted(graph *engine.ReceiptGraph, role engine.RuleSlotCapability, occurrence identity.ContentID) bool {
	if graph == nil || !attachment.valid() {
		return false
	}
	_, memberOK := graph.MountedRuleMember(role, attachment.mount, attachment.point, occurrence)
	return memberOK
}

// Observe reads the retained private Engine observation for this evidence
// point. It reports readability separately from the result's own validity, so a
// caller distinguishes a handle this solver and state cannot read from a read
// that returned an invalid summary.
func (attachment BranchValueObservationAttachment) Observe(solver *engine.Solver, state *engine.State) (valuedomain.ValueSummaryObservation, bool) {
	if !attachment.valid() {
		return valuedomain.ValueSummaryObservation{}, false
	}
	return engine.ReceiptObservationResult[valuedomain.ValueSummaryObservation](attachment.observation, solver, state)
}
