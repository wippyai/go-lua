package state

import (
	"github.com/wippyai/go-lua/analysis/domain/path/keyspace"
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/engine/state/numbound"
)

const LaneNumCeils LaneID = "num-ceils"

var numCeilsLaneSpec = laneSpec{
	dynamicRead:              dynamicReadIndependent(),
	id:                       LaneNumCeils,
	keySpaceMode:             laneKeySpaceOwned,
	valueDependencies:        independentValueDependencies(),
	identitySupport:          independentIdentitySupport(),
	numericConsistency:       numericConsistencyContributor(contributeNumCeils),
	semanticLaws:             []laneSemanticLaw{pathSubtreeMutationIndependent(), pathDescendantMutationIndependent(), pathResolutionIndependent(), pathEqualityQuotientIndependent(), genericForBindingIndependent(), pathReplacementIndependent(), effectFactorIndependent(), callBoundaryIndependent()},
	boundaryClosureCompanion: noBoundaryClosureCompanion(),
	rootAssignment:           rootAssignmentUnchanged(true, true, true),
	coordinateFamilies:       []coordinateFamilySpec{numBoundCoordinateFamilySpec(numCeilCoordinateFamilyID, numbound.Upper, dynamicReadNumCeilCoordinates())},
	boundary:                 boundaryLaneOps{project: projectNumCeilsBoundary, rebase: rebaseNumCeilsBoundary, postRebase: postRebaseBoundaryNoop, equal: equalNumCeilsBoundary},
	rekey: func(s State, from, to *keyspace.KeySpace) (State, bool) {
		lane, ok := numBoundRekey(s.numCeils, from, to)
		if !ok {
			return s, false
		}
		setStateNumCeils(&s, lane)
		return s, true
	},
	fingerprint: fingerprintNumCeils,
	markReachable: func(s State) State {
		if s.numCeils.lane.Bottom() {
			setStateNumCeils(&s, s.numCeils.Reachable())
		}
		return s
	},
	build: func(_ *axis.Registry, options DomainOptions) laneOps {
		domain := numBoundLaneDomain(numbound.Upper, options.WidenThresholds)
		return stateLaneWithBoundary(domain,
			func(s State) numBoundLane { return s.numCeils },
			setStateNumCeils,
			typedLaneFactorRepresentation[numBoundLane]{equal: domain.Equal},
			typedBoundaryFactorOps[numBoundLane]{apply: applyNumBoundBoundary, roots: boundaryRootsReachable(func(lane numBoundLane) numBoundLane { return lane.Reachable() }), reachability: emitNumBoundReachability, project: func(ctx *boundaryProjectContext, lane numBoundLane) (numBoundLane, bool) {
				return projectNumBoundBoundary(ctx, lane), true
			}, rebase: func(ctx *boundaryRebaseContext, lane numBoundLane) (numBoundLane, bool) {
				return rebaseNumBoundBoundary(ctx, lane, numbound.Upper)
			}, postRebase: boundaryPostRebaseUnchanged[numBoundLane]},
		)
	},
}
