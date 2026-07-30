package state

import (
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/engine/state/escapeevent"
)

const LaneEscapeEvents LaneID = "escape-events"

var escapeEventsLaneSpec = laneSpec{
	id:                       LaneEscapeEvents,
	keySpaceMode:             laneKeySpaceFree,
	formalRekey:              formalRekeyStructural(),
	valueDependencies:        independentValueDependencies(),
	identitySupport:          independentIdentitySupport(),
	numericConsistency:       numericConsistencyIndependent(),
	semanticLaws:             []laneSemanticLaw{pathSubtreeMutationIndependent(), pathDescendantMutationIndependent(), pathResolutionIndependent(), pathEqualityQuotientIndependent(), genericForBindingIndependent(), pathReplacementIndependent(), effectFactorIndependent(), callBoundaryIndependent()},
	boundaryClosureCompanion: noBoundaryClosureCompanion(),
	rootAssignment:           rootAssignmentUnchanged(false, false, false),
	dynamicRead:              dynamicReadIndependent(),
	fingerprint:              fingerprintEscapeEvents,
	boundary:                 boundaryLaneOps{project: projectEscapeEventsBoundary, rebase: rebaseEscapeEventsBoundary, postRebase: postRebaseBoundaryNoop, equal: equalEscapeEventsBoundary},
	markReachable: func(s State) State {
		s.escapeEvents = s.escapeEvents.Reachable()
		return s
	},
	build: func(reg *axis.Registry, _ DomainOptions) laneOps {
		domain := escapeevent.Domain()
		return stateLaneWithBoundary(domain,
			func(s State) escapeevent.Lane { return s.escapeEvents },
			func(out *State, lane escapeevent.Lane) { out.escapeEvents = lane },
			typedLaneFactorRepresentation[escapeevent.Lane]{equal: domain.Equal},
			typedBoundaryFactorOps[escapeevent.Lane]{apply: applyEscapeEventsBoundaryLane, roots: boundaryRootsReachable(func(lane escapeevent.Lane) escapeevent.Lane { return lane.Reachable() }), project: projectEscapeEventsBoundaryFactor, rebase: rebaseEscapeEventsBoundaryFactor, postRebase: boundaryPostRebaseUnchanged[escapeevent.Lane], reachability: emitEscapeEventsReachability},
		)
	},
}

func emitEscapeEventsReachability(program *boundaryReachabilityProgramBuilder, lane escapeevent.Lane) {
	for _, fact := range lane.Snapshot().Facts {
		program.pathCone(false, program.addStateKey(fact.Target))
	}
}
