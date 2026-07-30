package state

import (
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/engine/state/channelselectfact"
)

const LaneChannelSelect LaneID = "channel-select"

var channelSelectLaneSpec = laneSpec{
	id:                       LaneChannelSelect,
	keySpaceMode:             laneKeySpaceFree,
	formalRekey:              formalRekeyStructural(),
	valueDependencies:        independentValueDependencies(),
	identitySupport:          enumeratedIdentitySupport(visitChannelSelectLaneIdentities, func(s State) channelselectfact.Lane { return s.channelSelect }, IdentityImageEmbeddedValue),
	numericConsistency:       numericConsistencyIndependent(),
	semanticLaws:             []laneSemanticLaw{pathSubtreeMutationIndependent(), pathDescendantMutationIndependent(), pathResolutionIndependent(), pathEqualityQuotientIndependent(), genericForBindingIndependent(), pathReplacementIndependent(), effectFactorIndependent(), callBoundaryIndependent()},
	boundaryClosureCompanion: noBoundaryClosureCompanion(),
	rootAssignment:           rootAssignmentUnchanged(false, false, false),
	dynamicRead:              dynamicReadIndependent(),
	fingerprint:              fingerprintChannelSelect,
	boundary:                 boundaryLaneOps{project: projectChannelSelectBoundary, rebase: rebaseChannelSelectBoundary, postRebase: postRebaseBoundaryNoop, equal: equalChannelSelectBoundary},
	markReachable: func(s State) State {
		s.channelSelect = s.channelSelect.Reachable()
		return s
	},
	build: func(reg *axis.Registry, _ DomainOptions) laneOps {
		domain := channelselectfact.Domain()
		return stateLaneWithBoundary(domain,
			func(s State) channelselectfact.Lane { return s.channelSelect },
			func(out *State, lane channelselectfact.Lane) { out.channelSelect = lane },
			typedLaneFactorRepresentation[channelselectfact.Lane]{equal: domain.Equal},
			typedBoundaryFactorOps[channelselectfact.Lane]{apply: applyChannelSelectBoundaryLane, roots: boundaryRootsReachable(func(lane channelselectfact.Lane) channelselectfact.Lane { return lane.Reachable() }), project: projectChannelSelectBoundaryFactor, rebase: rebaseChannelSelectBoundaryFactor, postRebase: boundaryPostRebaseUnchanged[channelselectfact.Lane], reachability: emitChannelSelectReachability},
		)
	},
}

func emitChannelSelectReachability(program *boundaryReachabilityProgramBuilder, lane channelselectfact.Lane) {
	for _, fact := range lane.Snapshot().Facts {
		if program.pathCone(false, program.addStateKey(fact.Result), program.addStateKey(fact.Case)) && fact.HasPayload {
			program.addValue(fact.Payload)
		}
	}
}
