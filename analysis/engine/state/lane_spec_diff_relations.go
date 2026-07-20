package state

import "github.com/wippyai/go-lua/analysis/domain/value/axis"

const LaneDiffRelations LaneID = "diff-relations"

var diffRelationsLaneSpec = laneSpec{
	dynamicRead:              dynamicReadIndependent(),
	id:                       LaneDiffRelations,
	keySpaceMode:             laneKeySpaceFree,
	valueDependencies:        independentValueDependencies(),
	identitySupport:          independentIdentitySupport(),
	numericConsistency:       numericConsistencyContributor(contributeDiffRelations),
	semanticLaws:             []laneSemanticLaw{pathSubtreeMutationIndependent(), pathDescendantMutationIndependent(), pathResolutionIndependent(), pathEqualityQuotientIndependent(), genericForBindingIndependent(), pathReplacementIndependent(), effectFactorIndependent(), callBoundaryIndependent()},
	boundaryClosureCompanion: noBoundaryClosureCompanion(),
	rootAssignment:           rootAssignmentUnchanged(false, true, true),
	coordinateFamilies:       []coordinateFamilySpec{diffRelationCoordinateFamilySpec},
	fingerprint:              fingerprintDiffRelations,
	boundary:                 boundaryLaneOps{project: projectDiffRelationsBoundary, rebase: rebaseDiffRelationsBoundary, postRebase: postRebaseBoundaryNoop, equal: equalDiffRelationsBoundary},
	markReachable: func(s State) State {
		if s.diffRelations.bottom {
			setStateDiffRelations(&s, s.diffRelations.reachable())
		}
		return s
	},
	build: func(reg *axis.Registry, _ DomainOptions) laneOps {
		domain := diffRelationDomain()
		return stateLaneWithBoundary(domain,
			func(s State) diffRelationLane { return s.diffRelations },
			setStateDiffRelations,
			typedLaneFactorRepresentation[diffRelationLane]{equal: domain.Equal},
			typedBoundaryFactorOps[diffRelationLane]{apply: applyDiffRelationsBoundaryLane, roots: boundaryRootsReachable(func(lane diffRelationLane) diffRelationLane { return lane.reachable() }), project: projectDiffRelationsBoundaryFactor, rebase: rebaseDiffRelationsBoundaryFactor, postRebase: boundaryPostRebaseUnchanged[diffRelationLane], reachability: emitDiffRelationsReachability},
		)
	},
}

func emitDiffRelationsReachability(program *boundaryReachabilityProgramBuilder, lane diffRelationLane) {
	if lane.bottom {
		return
	}
	for relation := range lane.values {
		a := program.addStateKey(relation.A.Key)
		c := program.addStateKey(relation.C.Key)
		if relation.B.valid() {
			program.pathCone(false, a, program.addStateKey(relation.B.Key), c)
		} else {
			program.pathCone(false, a, c)
		}
	}
}
