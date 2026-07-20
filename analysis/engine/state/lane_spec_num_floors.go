package state

import (
	"github.com/wippyai/go-lua/analysis/domain/path/keyspace"
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/engine/state/numbound"
)

const LaneNumFloors LaneID = "num-floors"

var numFloorsLaneSpec = laneSpec{
	dynamicRead:              dynamicReadIndependent(),
	id:                       LaneNumFloors,
	keySpaceMode:             laneKeySpaceOwned,
	valueDependencies:        independentValueDependencies(),
	identitySupport:          independentIdentitySupport(),
	numericConsistency:       numericConsistencyContributor(contributeNumFloors),
	semanticLaws:             []laneSemanticLaw{pathSubtreeMutationIndependent(), pathDescendantMutationIndependent(), pathResolutionIndependent(), pathEqualityQuotientIndependent(), genericForBindingIndependent(), pathReplacementIndependent(), effectFactorIndependent(), callBoundaryIndependent()},
	boundaryClosureCompanion: noBoundaryClosureCompanion(),
	rootAssignment:           rootAssignmentUnchanged(true, true, true),
	coordinateFamilies:       []coordinateFamilySpec{numBoundCoordinateFamilySpec(numFloorCoordinateFamilyID, numbound.Lower, dynamicReadNumFloorCoordinates())},
	boundary:                 boundaryLaneOps{project: projectNumFloorsBoundary, rebase: rebaseNumFloorsBoundary, postRebase: postRebaseBoundaryNoop, equal: equalNumFloorsBoundary},
	rekey: func(s State, from, to *keyspace.KeySpace) (State, bool) {
		lane, ok := numBoundRekey(s.numFloors, from, to)
		if !ok {
			return s, false
		}
		setStateNumFloors(&s, lane)
		return s, true
	},
	fingerprint: fingerprintNumFloors,
	markReachable: func(s State) State {
		if s.numFloors.lane.Bottom() {
			setStateNumFloors(&s, s.numFloors.Reachable())
		}
		return s
	},
	build: func(_ *axis.Registry, _ DomainOptions) laneOps {
		domain := numBoundLaneDomain(numbound.Lower, nil)
		return stateLaneWithBoundary(domain,
			func(s State) numBoundLane { return s.numFloors },
			setStateNumFloors,
			typedLaneFactorRepresentation[numBoundLane]{equal: domain.Equal},
			typedBoundaryFactorOps[numBoundLane]{apply: applyNumBoundBoundary, roots: boundaryRootsReachable(func(lane numBoundLane) numBoundLane { return lane.Reachable() }), reachability: emitNumBoundReachability, project: func(ctx *boundaryProjectContext, lane numBoundLane) (numBoundLane, bool) {
				return projectNumBoundBoundary(ctx, lane), true
			}, rebase: func(ctx *boundaryRebaseContext, lane numBoundLane) (numBoundLane, bool) {
				return rebaseNumBoundBoundary(ctx, lane, numbound.Lower)
			}, postRebase: boundaryPostRebaseUnchanged[numBoundLane]},
		)
	},
}
