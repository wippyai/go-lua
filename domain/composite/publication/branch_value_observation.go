package publication

import (
	"github.com/wippyai/go-lua/analysis/engine"
	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/schema"
	"github.com/wippyai/go-lua/analysis/schema/executioncontext"
	valuedomain "github.com/wippyai/go-lua/domain/value"
)

// BranchValueObservationAttachment is the bridge from one mounted branch
// evidence point to Value's summary surface. The declared observation row is
// intentionally private: a caller cannot hand a shaped ValueSummaryObservation
// to this path, and cannot substitute a row that this attachment's identity
// did not authorize. One evidence point carries one attachment, shared by every
// later reader of that point.
type BranchValueObservationAttachment struct {
	observation engine.ProgramObservationAdmission
	id          identity.ContentID
	mount       identity.ContentID
	point       identity.ContentID
	producer    schema.Key
	context     executioncontext.Context
}

// BranchValueObservationID is the Snapshot row address of one mounted branch
// evidence observation owned by one exact execution Context. Attach and
// detach both derive this identity; neither retains a second publication
// table. Context is part of the canonical preimage so same-module actors can
// never collapse into one row.
func BranchValueObservationID(mount, point identity.ContentID, producer schema.Key, context executioncontext.Context) (identity.ContentID, bool) {
	if !mount.Available() || !point.Available() || !producer.Available() || !context.Available() || context.ModuleKey() != mount {
		return identity.ContentID{}, false
	}
	contextID := context.ID()
	return identity.DeriveContentID("analysis/branch-value-observation/v2", mount[:], point[:], contextID[:], []byte(producer))
}

func (attachment BranchValueObservationAttachment) valid() bool {
	want, ok := BranchValueObservationID(attachment.mount, attachment.point, attachment.producer, attachment.context)
	return ok && attachment.id == want && attachment.observation.Available() && attachment.observation.ID == attachment.id
}

func (attachment BranchValueObservationAttachment) Valid() bool { return attachment.valid() }

func (attachment BranchValueObservationAttachment) ContentID() (identity.ContentID, bool) {
	return attachment.id, attachment.valid()
}

// DeclareBranchValueObservation states one Value summary root over the exact
// mounted rule member that produces this evidence point. It precedes solving
// and reads no solved state: the caller names the member by role capability and
// authored coordinates, and the committed program, not this relation, decides
// whether that member exists.
//
// The failure is the Engine observation classification, so a caller that
// declares a whole receipt of evidence points reports which coordinate
// rejected the row rather than a single opaque refusal.
func DeclareBranchValueObservation(
	committed *engine.CommittedProgram,
	query *engine.SummaryQueryImplementation[valuedomain.Value, valuedomain.ValueSummaryObservation],
	role engine.RuleSlotCapability,
	producer schema.Key,
	mount, point, occurrence identity.ContentID,
	context executioncontext.Context,
) (BranchValueObservationAttachment, engine.SolveFailure, bool) {
	if committed == nil || query == nil {
		return BranchValueObservationAttachment{}, engine.ObservationSealArguments(), false
	}
	id, idOK := BranchValueObservationID(mount, point, producer, context)
	if !idOK {
		return BranchValueObservationAttachment{}, engine.ObservationSealArguments(), false
	}
	if !MemberPublished(committed, role, mount, point, occurrence) {
		return BranchValueObservationAttachment{}, engine.ObservationSealPoint(), false
	}
	observation, declared := engine.NewSummaryObservationAdmission[valuedomain.Value, valuedomain.ValueSummaryObservation](query, id, role, mount, point, occurrence, context)
	if !declared {
		return BranchValueObservationAttachment{}, engine.ObservationSealArguments(), false
	}
	attachment := BranchValueObservationAttachment{observation: observation, id: id, mount: mount, point: point, producer: producer, context: context}
	if !attachment.valid() {
		return BranchValueObservationAttachment{}, engine.ObservationSealArguments(), false
	}
	return attachment, engine.SolveFailure{}, true
}

// Observation is the declared observation row this attachment answers on. The
// seal that mints the Solver binds it; nothing else can.
func (attachment BranchValueObservationAttachment) Observation() (engine.ProgramObservationAdmission, bool) {
	return attachment.observation, attachment.valid()
}

// MemberPublished reports whether the committed program publishes a member at
// these exact coordinates. A receipt that names one evidence point from a
// second producer holds one attachment for it and reauthenticates the
// additional coordinates here, so a producer that names no published member is
// rejected rather than silently folded into an attachment it never authorized.
func MemberPublished(committed *engine.CommittedProgram, role engine.RuleSlotCapability, mount, point, occurrence identity.ContentID) bool {
	if committed == nil {
		return false
	}
	_, published := committed.MountedRuleMember(role, mount, point, occurrence)
	return published
}
