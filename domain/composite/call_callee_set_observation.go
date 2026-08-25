package composite

import (
	"github.com/wippyai/go-lua/analysis/engine"
	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/schema"
	"github.com/wippyai/go-lua/analysis/schema/executioncontext"
	programissuance "github.com/wippyai/go-lua/analysis/schema/program/issuance"
	"github.com/wippyai/go-lua/analysis/schema/programmount"
	"github.com/wippyai/go-lua/analysis/schema/query"
	calldomain "github.com/wippyai/go-lua/domain/call"
	calldispatch "github.com/wippyai/go-lua/domain/call/dispatch"
	dispatchobservation "github.com/wippyai/go-lua/domain/call/dispatch/observation"
	calldispatchprogram "github.com/wippyai/go-lua/domain/call/dispatch/program"
	callowner "github.com/wippyai/go-lua/domain/call/owner"
	callquery "github.com/wippyai/go-lua/domain/call/query"
)

// CallCalleeSetObservations enumerates Call's producer-only exact observation
// population over the canonical mounted dispatch placements.  Query
// registration owns the family and its typed fold; this seam only recovers
// that producer from the sealed query cell, resolves Dispatch's owner rule,
// and expands each mounted module over its exact Link-owned Context rows.
//
// The supplied directory must describe the complete Context set carried by
// this binding.  A partial, duplicate, or foreign directory is refused before
// any observation is admitted.  No selected QuerySite or Result publication
// is involved: Call dispatch owns the observation row and its identity.
func (bound *ProgramBinding) CallCalleeSetObservations(
	committed *engine.CommittedProgram,
	mounts []programmount.MountedArtifact,
	contexts executioncontext.Directory,
) ([]engine.ProgramObservationAdmission, bool) {
	if bound == nil || !bound.Available() || committed == nil || len(mounts) == 0 || !contexts.Available() {
		return nil, false
	}
	if !boundOwnsContextDirectory(bound, contexts) {
		return nil, false
	}

	cell, cellOK := bound.Query(callquery.CalleeSetResultFamily)
	fragment, fragmentOK := query.Payload[*callquery.ExactQueryFragment](cell)
	calleeQuery, queryOK := callquery.RecoverQuery(bound.binding, query.Sealed[*callquery.ExactQueryFragment]{Fragment: fragment})
	if !cellOK || !fragmentOK || !queryOK || calleeQuery == nil {
		return nil, false
	}

	capability, capabilityOK := bound.rules.CapabilityByKey(calldispatchprogram.RuleKey)
	algebra := bound.call.Algebra()
	callAuthority := bound.CallAuthority()
	if !capabilityOK || algebra == nil || !algebra.Valid() || callAuthority == nil || callAuthority.Algebra() != algebra {
		return nil, false
	}
	linkID := algebra.LinkOwner().ContentID()
	if !linkID.Available() {
		return nil, false
	}

	observations := make([]engine.ProgramObservationAdmission, 0)
	seen := make(map[identity.ContentID]struct{})
	_, walked := WalkSealedPlacements(mounts, func(ruleKey schema.Key, mount, _, occurrence identity.ContentID) bool {
		if ruleKey != calldispatchprogram.RuleKey {
			return true
		}
		mountedContexts, contextsOK := contexts.ContextsForModule(mount)
		if !contextsOK {
			return false
		}
		for _, context := range mountedContexts {
			if !bound.contextSchema.OwnsContext(context) {
				return false
			}
			admission, admitted := mountedCalleeSetObservation(
				committed, calleeQuery, capability, callAuthority, algebra, linkID, mount, occurrence, context,
			)
			if !admitted || !admission.Available() {
				return false
			}
			if _, duplicate := seen[admission.ID]; duplicate {
				return false
			}
			seen[admission.ID] = struct{}{}
			observations = append(observations, admission)
		}
		return true
	})
	if !walked {
		return nil, false
	}
	return observations, true
}

// mountedCalleeSetObservation is the composite consumer-side admission for
// Call's observation query. The generated dispatch family owns the Call fact;
// this seam only addresses its committed native stage and binds the query's
// exact Snapshot row. It therefore carries no legacy rule payload or second
// dispatch directory.
func mountedCalleeSetObservation(
	committed *engine.CommittedProgram,
	query *callquery.ExactQueryImplementation,
	capability engine.RuleSlotCapability,
	callAuthority *callowner.HotOwner,
	algebra *calldomain.Algebra,
	linkID, mount, occurrence identity.ContentID,
	context executioncontext.Context,
) (engine.ProgramObservationAdmission, bool) {
	if committed == nil || query == nil || callAuthority == nil || algebra == nil || callAuthority.Algebra() != algebra || !algebra.Valid() ||
		!capability.Available() || !linkID.Available() || !mount.Available() ||
		!occurrence.Available() || !context.Available() || context.ModuleKey() != mount ||
		context.LinkID() != linkID {
		return engine.ProgramObservationAdmission{}, false
	}
	projected, projectedOK := algebra.CallCoordinateForOccurrence(mount, occurrence)
	applicationID, callID, moduleID, _, _, identityOK := projected.Identity()
	if !projectedOK || !identityOK || !projected.Valid() || callID != occurrence || moduleID != mount {
		return engine.ProgramObservationAdmission{}, false
	}
	key, keyOK := projected.Key()
	ref, refOK := callAuthority.Ref(key)
	if !keyOK || !refOK {
		return engine.ProgramObservationAdmission{}, false
	}
	stage, stageOK := committed.MountedNativeCallStage(capability, mount, occurrence)
	if !stageOK || !stage.Available() || stage.Kind() != programissuance.StageCallDispatch ||
		stage.MountID() != mount || stage.OccurrenceID() != occurrence ||
		!stage.PointID().Available() || !stage.HasMember() {
		return engine.ProgramObservationAdmission{}, false
	}
	observationID, idOK := dispatchobservation.ID(linkID, applicationID, mount, occurrence, context.ID())
	if !idOK {
		return engine.ProgramObservationAdmission{}, false
	}
	return engine.NewExactObservationAdmissionAt[calldomain.DenseCoordinate, calldomain.Value, calldispatch.CalleeSet](
		query, ref, observationID, capability, mount, stage.PointID(), occurrence, context,
	)
}

// boundOwnsContextDirectory authenticates the complete Context population
// against this binding's exact contextual authority.  ContextsForModule is a
// useful per-mount lookup, but by itself it would permit a same-Link subset;
// the cardinality and ID checks below make missing rows fail closed as well.
func boundOwnsContextDirectory(bound *ProgramBinding, contexts executioncontext.Directory) bool {
	if bound == nil || !contexts.Available() {
		return false
	}
	expected := bound.contextSchema.Directory()
	if !expected.Available() || contexts.LinkID() != expected.LinkID() || contexts.ContextCount() != expected.ContextCount() {
		return false
	}
	seen := make(map[identity.ContentID]struct{}, contexts.ContextCount())
	for index := 0; index < contexts.ContextCount(); index++ {
		context, contextOK := contexts.ContextAt(index)
		if !contextOK || !bound.contextSchema.OwnsContext(context) {
			return false
		}
		if _, duplicate := seen[context.ID()]; duplicate {
			return false
		}
		seen[context.ID()] = struct{}{}
	}
	for index := 0; index < expected.ContextCount(); index++ {
		context, contextOK := expected.ContextAt(index)
		if !contextOK {
			return false
		}
		if _, present := seen[context.ID()]; !present {
			return false
		}
	}
	return true
}
