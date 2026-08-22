package dispatch

import (
	"github.com/wippyai/go-lua/analysis/engine"
	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/schema/executioncontext"
	programissuance "github.com/wippyai/go-lua/analysis/schema/program/issuance"
	calldomain "github.com/wippyai/go-lua/domain/call"
)

const callDispatchObservationDomain = "wippy.analysis.call.dispatch-observation.v1\x00"

// MountedCalleeSetObservation attaches Call's exact observation query to one
// mounted call-dispatch occurrence and execution Context.  Dispatch is an
// observation population: the row is attached for every authenticated native
// dispatch stage, even when the solved Call cell is Bottom.  The exact query
// turns Bottom into absence; this boundary never fabricates Unknown.
//
// The mounted inverse and application key are both recovered from Call's
// Algebra.  In particular, a callback or resume key cannot be reinterpreted as
// an application merely because it carries a ContentID.  Program contributes
// only the committed native stage/member proof; it does not provide the row
// identity or a second occurrence directory.  The Engine's exact-observation
// binder reauthenticates that member's one strong, unrouted exact write against
// the query Factor; a routed, summary, or multiple-write member therefore
// refuses at observation seal rather than acquiring a fallback admission.
func (rule *HotRule) MountedCalleeSetObservation(
	committed *engine.CommittedProgram,
	query *engine.ExactQueryImplementation[calldomain.Value, CalleeSet],
	mount, occurrence identity.ContentID,
	context executioncontext.Context,
) (admission engine.ProgramObservationAdmission, ok bool) {
	if rule == nil || !rule.valid() || rule.implementation == nil || committed == nil || query == nil ||
		!mount.Available() || !occurrence.Available() || !context.Available() || context.ModuleKey() != mount {
		return engine.ProgramObservationAdmission{}, false
	}

	algebra := rule.calls.Algebra()
	linkID := algebra.LinkOwner().ContentID()
	if !linkID.Available() || context.LinkID() != linkID {
		return engine.ProgramObservationAdmission{}, false
	}

	// Resolve the occurrence through Call's exact owner.  The application key
	// check is intentional: callback and resume arms have ContentIDs too, but
	// neither is an ordinary mounted call application.
	mounted, mountedOK := algebra.MountedCallForOccurrence(mount, occurrence)
	applicationID, callID, moduleID, _, _, identityOK := algebra.MountedCallIdentity(mounted)
	key, keyOK := algebra.KeyForMountedCall(mounted)
	keyApplicationID, keyApplicationOK := key.ApplicationID()
	row, rowOK := rule.rowForMounted(mounted)
	if !mountedOK || !identityOK || !keyOK || !key.IsApplication() || !keyApplicationOK ||
		!applicationID.Available() || !callID.Available() || !moduleID.Available() || !rowOK ||
		callID != occurrence || moduleID != mount || keyApplicationID != applicationID || !row.contentID.Available() {
		return engine.ProgramObservationAdmission{}, false
	}

	capability, capabilityOK := rule.implementation.MountedCapability()
	stage, stageOK := committed.MountedNativeCallStage(capability, mount, occurrence)
	if !capabilityOK || !stageOK || !stage.Available() || stage.Kind() != programissuance.StageCallDispatch ||
		stage.MountID() != mount || stage.OccurrenceID() != occurrence || !stage.PointID().Available() ||
		!stage.HasMember() {
		return engine.ProgramObservationAdmission{}, false
	}

	observationID, idOK := callDispatchObservationID(linkID, applicationID, mount, occurrence, context.ID())
	if !idOK {
		return engine.ProgramObservationAdmission{}, false
	}
	admission, admitted := engine.NewExactObservationAdmission[calldomain.Value, CalleeSet](
		query, observationID, capability, mount, stage.PointID(), occurrence, context,
	)
	return admission, admitted
}

// callDispatchObservationID derives the immutable Snapshot row address for a
// mounted dispatch observation.  Every identity is owner-issued: the Link
// owner fences equivalent content from another Link, application distinguishes
// the ordinary Call source arm, and mount/occurrence/context distinguish the
// exact mounted stage and actor-qualified execution cell.
func callDispatchObservationID(linkID, applicationID, mount, occurrence, contextID identity.ContentID) (identity.ContentID, bool) {
	if !linkID.Available() || !applicationID.Available() || !mount.Available() || !occurrence.Available() || !contextID.Available() {
		return identity.ContentID{}, false
	}
	return identity.DeriveContentID(callDispatchObservationDomain, linkID[:], applicationID[:], mount[:], occurrence[:], contextID[:])
}
